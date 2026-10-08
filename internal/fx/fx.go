// Package fx resolves the USD→CNY exchange rate used to annotate CNY
// prices in Chinese output. Sources are keyless public endpoints chosen to
// answer from both inside and outside China; the resolved rate is cached on
// disk for 24 hours, mirroring the models.dev catalog strategy.
package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/daidaiJ/modelq/internal/locale"
)

// sources lists USD→CNY endpoints in fallback order: the ExchangeRate-API
// open endpoint, the ECB-backed Frankfurter API, and the fawazahmed0
// currency-api served from two CDN mirrors.
var sources = []string{
	"https://open.er-api.com/v6/latest/USD",
	"https://api.frankfurter.dev/v1/latest?base=USD&symbols=CNY",
	"https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.json",
	"https://latest.currency-api.pages.dev/v1/currencies/usd.json",
}

const (
	cacheTTL    = 24 * time.Hour
	maxBodySize = 1 << 20 // responses are small; the widest source lists every currency
	// Plausible bounds for CNY per USD; a value outside is treated as a
	// broken response instead of a real rate.
	minRate = 1.0
	maxRate = 20.0
)

// Rate is the resolved USD→CNY conversion plus provenance.
type Rate struct {
	USDCNY    float64   `json:"usd_cny"`
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetched_at"`
	// Stale is true when every source failed and an outdated cached rate
	// was served instead.
	Stale bool `json:"-"`
}

// Fetch returns the USD→CNY rate, serving it from an on-disk cache for 24
// hours. With refresh set it re-downloads even when the cache is fresh.
// When every source fails but any cached copy exists (even expired), the
// cache is returned with Stale set. httpClient may be nil.
func Fetch(ctx context.Context, refresh bool, httpClient *http.Client) (*Rate, error) {
	path, cacheOK := cachePathFn()
	if !refresh && cacheOK {
		if r, ok := readCache(path, cacheTTL); ok {
			return r, nil
		}
	}

	urls := sources
	if v := os.Getenv("MQX_FX_URL"); v != "" {
		urls = []string{v}
	}
	r, err := fetchFrom(ctx, urls, httpClient)
	if err != nil {
		if cacheOK {
			if r, ok := readCache(path, 0); ok {
				r.Stale = true
				return r, nil
			}
		}
		return nil, err
	}

	if cacheOK {
		writeCache(path, r) // best effort; a failed write only costs a re-download
	}
	return r, nil
}

// fetchFrom tries each endpoint in order and returns the first valid rate;
// all failures are joined so the error names every source that was tried.
func fetchFrom(ctx context.Context, urls []string, httpClient *http.Client) (*Rate, error) {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	var errs []error
	for _, u := range urls {
		r, err := fetchOne(ctx, client, u)
		if err == nil {
			return r, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

func fetchOne(ctx context.Context, client *http.Client, raw string) (*Rate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "modelq")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", locale.T("fetch "+raw+": "+err.Error(), "拉取 "+raw+" 失败: "+err.Error()))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("%s", locale.T("read "+raw+": "+err.Error(), "读取 "+raw+" 失败: "+err.Error()))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", locale.T(
			fmt.Sprintf("%s returned HTTP %d: %s", raw, resp.StatusCode, truncateBody(string(body), 200)),
			fmt.Sprintf("%s 返回 HTTP %d: %s", raw, resp.StatusCode, truncateBody(string(body), 200))))
	}

	rate, err := parseCNY(body)
	if err != nil {
		return nil, fmt.Errorf("%s", locale.T("decode "+raw+": "+err.Error(), "解析 "+raw+" 失败: "+err.Error()))
	}
	return &Rate{USDCNY: rate, Source: host(raw), FetchedAt: time.Now()}, nil
}

// payload tolerates the response shapes of the supported sources: a
// top-level rates map (ExchangeRate-API, Frankfurter) or a keyed currency
// map (fawazahmed0 currency-api, "usd": {"cny": ...}).
type payload struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
	USD    struct {
		CNY float64 `json:"cny"`
	} `json:"usd"`
}

// parseCNY extracts the USD→CNY rate, checking both common key spellings
// and bounding the value so a layout change fails loudly instead of
// poisoning every annotated price.
func parseCNY(body []byte) (float64, error) {
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return 0, err
	}
	if p.Result == "error" {
		return 0, errors.New(locale.T("source reported result=error", "源返回 result=error"))
	}
	r := p.USD.CNY
	if r == 0 {
		r = max(p.Rates["CNY"], p.Rates["cny"])
	}
	if r < minRate || r > maxRate {
		return 0, fmt.Errorf("%s", locale.T(
			fmt.Sprintf("CNY rate %.4f outside plausible range [%g, %g]", r, minRate, maxRate),
			fmt.Sprintf("CNY 汇率 %.4f 超出合理范围 [%g, %g]", r, minRate, maxRate)))
	}
	return r, nil
}

// host extracts the host for display; a URL that fails to parse is shown
// verbatim.
func host(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

type cacheFile struct {
	FetchedAt time.Time `json:"fetched_at"`
	USDCNY    float64   `json:"usd_cny"`
	Source    string    `json:"source"`
}

// cachePathFn is swapped in tests to point the cache at a temp dir.
var cachePathFn = cachePath

// cachePath returns the cache file location; ok is false when the platform
// has no user cache dir or it cannot be created.
func cachePath() (path string, ok bool) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(root, "modelq")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	return filepath.Join(dir, "fx.json"), true
}

// readCache loads the cached rate; ttl <= 0 accepts any age.
func readCache(path string, ttl time.Duration) (*Rate, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(body, &cf); err != nil || cf.USDCNY <= 0 {
		return nil, false
	}
	if ttl > 0 && time.Since(cf.FetchedAt) > ttl {
		return nil, false
	}
	return &Rate{USDCNY: cf.USDCNY, Source: cf.Source, FetchedAt: cf.FetchedAt}, true
}

func writeCache(path string, r *Rate) {
	body, err := json.Marshal(cacheFile{FetchedAt: r.FetchedAt, USDCNY: r.USDCNY, Source: r.Source})
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
