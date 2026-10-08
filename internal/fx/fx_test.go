package fx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCNY(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    float64
		wantErr bool
	}{
		{"er-api shape", `{"result":"success","rates":{"CNY":6.712457}}`, 6.712457, false},
		{"frankfurter shape", `{"amount":1.0,"base":"USD","rates":{"CNY":6.7023}}`, 6.7023, false},
		{"currency-api shape", `{"date":"2026-10-08","usd":{"cny":6.7053}}`, 6.7053, false},
		{"lowercase rates key", `{"rates":{"cny":6.7}}`, 6.7, false},
		{"result error", `{"result":"error","error-type":"plan-upgrade-required"}`, 0, true},
		{"out of bounds", `{"rates":{"CNY":900}}`, 0, true},
		{"missing CNY", `{"rates":{"EUR":0.9}}`, 0, true},
		{"garbage", `not json`, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCNY([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCNY(%s) = %v, want an error", tc.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseCNY(%s) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// withTempCache points the cache at a fresh file and restores the hook when
// the test ends; it returns the cache path.
func withTempCache(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fx.json")
	cachePathFn = func() (string, bool) { return path, true }
	t.Cleanup(func() { cachePathFn = cachePath })
	return path
}

func writeCacheFile(t *testing.T, path string, age time.Duration, rate float64, source string) {
	t.Helper()
	cf := cacheFile{FetchedAt: time.Now().Add(-age), USDCNY: rate, Source: source}
	body, err := json.Marshal(cf)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func deadSource() string { return "http://127.0.0.1:1" }

func TestFetchFallsBackToNextSource(t *testing.T) {
	withTempCache(t)
	ctx := t.Context()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"base":"USD","rates":{"CNY":6.7023}}`)
	}))
	defer good.Close()

	old := sources
	sources = []string{bad.URL, good.URL}
	t.Cleanup(func() { sources = old })

	r, err := Fetch(ctx, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.USDCNY != 6.7023 || r.Stale {
		t.Errorf("rate = %v (stale=%t), want 6.7023 fresh", r.USDCNY, r.Stale)
	}
	if r.Source != host(good.URL) {
		t.Errorf("source = %q, want %q", r.Source, host(good.URL))
	}

	// The rate was cached, so a run whose sources are all dead still
	// answers from the fresh cache.
	sources = []string{deadSource()}
	r2, err := Fetch(ctx, false, nil)
	if err != nil {
		t.Fatalf("cached fetch failed: %v", err)
	}
	if r2.USDCNY != 6.7023 || r2.Stale {
		t.Errorf("cached rate = %v (stale=%t), want 6.7023 fresh", r2.USDCNY, r2.Stale)
	}
}

func TestFetchStaleCacheFallback(t *testing.T) {
	path := withTempCache(t)
	writeCacheFile(t, path, 72*time.Hour, 7.09, "example.test")
	ctx := t.Context()

	old := sources
	sources = []string{deadSource()}
	t.Cleanup(func() { sources = old })

	r, err := Fetch(ctx, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.USDCNY != 7.09 || !r.Stale {
		t.Errorf("rate = %v (stale=%t), want stale 7.09", r.USDCNY, r.Stale)
	}

	// refresh re-downloads and still falls back to the stale copy.
	if r, err := Fetch(ctx, true, nil); err != nil || !r.Stale || r.USDCNY != 7.09 {
		t.Errorf("refreshed rate = %v, %v (stale=%t), want stale 7.09", r, err, r.Stale)
	}
}

func TestFetchErrorWithoutCache(t *testing.T) {
	cachePathFn = func() (string, bool) { return "", false }
	t.Cleanup(func() { cachePathFn = cachePath })

	old := sources
	sources = []string{deadSource()}
	t.Cleanup(func() { sources = old })

	if _, err := Fetch(t.Context(), false, nil); err == nil {
		t.Fatal("expected an error when every source is dead and caching is unavailable")
	}
}

func TestFetchEnvURLOverride(t *testing.T) {
	withTempCache(t)
	ctx := t.Context()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"date":"2026-10-08","usd":{"cny":6.7053}}`)
	}))
	defer good.Close()

	old := sources
	sources = []string{deadSource()}
	t.Cleanup(func() { sources = old })
	t.Setenv("MQX_FX_URL", good.URL)

	r, err := Fetch(ctx, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.USDCNY != 6.7053 || r.Source != host(good.URL) {
		t.Errorf("rate = %v from %q, want 6.7053 from %q", r.USDCNY, r.Source, host(good.URL))
	}
}

func TestFetchJoinsSourceErrors(t *testing.T) {
	withTempCache(t)
	bad1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad1.Close()

	old := sources
	sources = []string{bad1.URL, deadSource()}
	t.Cleanup(func() { sources = old })

	_, err := Fetch(t.Context(), true, nil)
	if err == nil {
		t.Fatal("expected a joined error")
	}
	if !strings.Contains(err.Error(), bad1.URL) || !strings.Contains(err.Error(), deadSource()) {
		t.Errorf("joined error should name every source tried, got: %v", err)
	}
}
