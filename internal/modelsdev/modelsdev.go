// Package modelsdev reads the models.dev catalog, a community-maintained
// database of model parameters and reference pricing across providers
// (https://models.dev). The CLI uses it to enrich OpenRouter lookups and to
// resolve approximate model ids that OpenRouter does not list.
package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// APIURL is the full catalog endpoint. It can be overridden with the
// MODELSDEV_API_URL environment variable (useful for tests and restricted
// networks).
const APIURL = "https://models.dev/api.json"

const (
	cacheTTL    = 24 * time.Hour
	maxBodySize = 128 << 20 // the catalog is a few MB today; leave headroom
)

// Catalog is the parsed models.dev data plus fetch metadata.
type Catalog struct {
	Providers map[string]Provider `json:"providers"`
	FetchedAt time.Time           `json:"fetched_at"`
	// Stale is true when the download failed and an outdated cached copy was
	// served instead.
	Stale bool `json:"-"`
}

// Provider is one models.dev provider entry.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	NPM    string           `json:"npm"`
	API    string           `json:"api"`
	Doc    string           `json:"doc"`
	Env    []string         `json:"env"`
	Models map[string]Model `json:"models"`
}

// ProviderInfo is the slim provider metadata used in JSON output, so full
// provider model maps are never marshaled.
type ProviderInfo struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	NPM  string   `json:"npm"`
	API  string   `json:"api"`
	Doc  string   `json:"doc"`
	Env  []string `json:"env"`
}

// Info returns the slim metadata for a provider stored in the catalog.
func (c *Catalog) Info(providerID string) ProviderInfo {
	p, ok := c.Providers[providerID]
	if !ok {
		return ProviderInfo{ID: providerID}
	}
	return ProviderInfo{ID: p.ID, Name: p.Name, NPM: p.NPM, API: p.API, Doc: p.Doc, Env: p.Env}
}

// Model is one models.dev model entry. Prices are USD per 1M tokens; nil
// means the source does not list the value.
type Model struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	Family           string            `json:"family,omitempty"`
	Attachment       bool              `json:"attachment,omitempty"`
	Reasoning        bool              `json:"reasoning,omitempty"`
	ReasoningOptions []ReasoningOption `json:"reasoning_options,omitempty"`
	ToolCall         *bool             `json:"tool_call,omitempty"`
	StructuredOutput *bool             `json:"structured_output,omitempty"`
	Temperature      *bool             `json:"temperature,omitempty"`
	Knowledge        string            `json:"knowledge,omitempty"`
	ReleaseDate      string            `json:"release_date,omitempty"`
	LastUpdated      string            `json:"last_updated,omitempty"`
	Modalities       Modalities        `json:"modalities,omitempty"`
	OpenWeights      *bool             `json:"open_weights,omitempty"`
	Limit            Limit             `json:"limit,omitempty"`
	Cost             Cost              `json:"cost,omitempty"`
	CanonicalModelID string            `json:"canonical_model_id,omitempty"`
	Status           string            `json:"status,omitempty"`
	Experimental     json.RawMessage   `json:"experimental,omitempty"`
	Interleaved      json.RawMessage   `json:"interleaved,omitempty"`
}

// ReasoningOption describes how reasoning can be controlled: "toggle",
// "effort" with allowed values, or "budget_tokens" with a min/max range.
type ReasoningOption struct {
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// Modalities lists accepted input and produced output types.
type Modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// Limit holds token caps. Input can be lower than Context on some providers.
type Limit struct {
	Context int64 `json:"context,omitempty"`
	Input   int64 `json:"input,omitempty"`
	Output  int64 `json:"output,omitempty"`
}

// Cost is the vendor reference pricing per 1M tokens.
type Cost struct {
	Input       *float64   `json:"input,omitempty"`
	Output      *float64   `json:"output,omitempty"`
	CacheRead   *float64   `json:"cache_read,omitempty"`
	CacheWrite  *float64   `json:"cache_write,omitempty"`
	Reasoning   *float64   `json:"reasoning,omitempty"`
	InputAudio  *float64   `json:"input_audio,omitempty"`
	OutputAudio *float64   `json:"output_audio,omitempty"`
	Tiers       []CostTier `json:"tiers,omitempty"`
}

// CostTier is a price bracket that applies once the prompt passes the tier
// threshold (tier.type = "context", tier.size in tokens).
type CostTier struct {
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
	Tier       struct {
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tier"`
}

// ExperimentalModes extracts mode names when experimental carries a modes
// map (e.g. {"fast": {...}}); ok is false for booleans or absent fields.
func (m Model) ExperimentalModes() (names []string, ok bool) {
	if len(m.Experimental) == 0 {
		return nil, false
	}
	var withModes struct {
		Modes map[string]json.RawMessage `json:"modes"`
	}
	if err := json.Unmarshal(m.Experimental, &withModes); err != nil || len(withModes.Modes) == 0 {
		return nil, false
	}
	for name := range withModes.Modes {
		names = append(names, name)
	}
	return names, true
}

// InterleavedField extracts the field name when interleaved is
// {"field": "reasoning_content"}.
func (m Model) InterleavedField() (field string, ok bool) {
	if len(m.Interleaved) == 0 {
		return "", false
	}
	var withField struct {
		Field string `json:"field"`
	}
	if err := json.Unmarshal(m.Interleaved, &withField); err != nil || withField.Field == "" {
		return "", false
	}
	return withField.Field, true
}

// Fetch returns the catalog, serving it from an on-disk cache for 24 hours.
// With refresh set it re-downloads even when the cache is fresh. When the
// download fails but any cached copy exists (even expired), the cache is
// returned with Stale set. httpClient may be nil.
func Fetch(ctx context.Context, refresh bool, httpClient *http.Client) (*Catalog, error) {
	path, cacheOK := cachePath()
	if !refresh && cacheOK {
		if cat, ok := readCache(path, cacheTTL); ok {
			return cat, nil
		}
	}

	cat, err := download(ctx, httpClient)
	if err != nil {
		if cacheOK {
			if cat, ok := readCache(path, 0); ok {
				cat.Stale = true
				return cat, nil
			}
		}
		return nil, err
	}

	if cacheOK {
		writeCache(path, cat) // best effort; a failed write only costs a re-download
	}
	return cat, nil
}

func download(ctx context.Context, httpClient *http.Client) (*Catalog, error) {
	url := os.Getenv("MODELSDEV_API_URL")
	if url == "" {
		url = APIURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "openrouter-cli")

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned HTTP %d: %s", resp.StatusCode, truncateBody(string(body), 200))
	}

	cat := Catalog{FetchedAt: time.Now()}
	if err := json.Unmarshal(body, &cat.Providers); err != nil {
		return nil, fmt.Errorf("decode models.dev catalog: %w", err)
	}
	if len(cat.Providers) == 0 {
		return nil, fmt.Errorf("models.dev catalog is empty")
	}
	return &cat, nil
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type cacheFile struct {
	FetchedAt time.Time           `json:"fetched_at"`
	Providers map[string]Provider `json:"providers"`
}

// cachePath returns the cache file location; ok is false when the platform
// has no user cache dir or it cannot be created.
func cachePath() (path string, ok bool) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(root, "openrouter-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	return filepath.Join(dir, "models-dev.json"), true
}

// readCache loads the cached catalog; ttl <= 0 accepts any age.
func readCache(path string, ttl time.Duration) (*Catalog, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(body, &cf); err != nil || len(cf.Providers) == 0 {
		return nil, false
	}
	if ttl > 0 && time.Since(cf.FetchedAt) > ttl {
		return nil, false
	}
	return &Catalog{Providers: cf.Providers, FetchedAt: cf.FetchedAt}, true
}

func writeCache(path string, cat *Catalog) {
	body, err := json.Marshal(cacheFile{FetchedAt: cat.FetchedAt, Providers: cat.Providers})
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// normID folds id separators so "claude-sonnet-4.5", "claude_sonnet_4_5" and
// "claude-sonnet-4-5" compare equal across catalogs that use different
// conventions.
func normID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ReplaceAll(s, ".", "-")
}

// collapse additionally removes dashes, mapping vendor aliases such as
// "z-ai"/"zai" or "moonshot-ai"/"moonshotai" onto each other.
func collapse(s string) string {
	return strings.ReplaceAll(normID(s), "-", "")
}
