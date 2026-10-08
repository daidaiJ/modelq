package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// Client talks to the OpenRouter public API. All endpoints used are public,
// so no authentication is involved.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "openrouter-cli")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openrouter returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Models fetches the full model catalog.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Data []Model `json:"data"`
	}
	if err := c.get(ctx, "/models", &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Model is a single catalog entry.
type Model struct {
	ID               string          `json:"id"`
	CanonicalSlug    string          `json:"canonical_slug"`
	Name             string          `json:"name"`
	Created          int64           `json:"created"`
	Description      string          `json:"description"`
	ContextLength    int64           `json:"context_length"`
	Architecture     Architecture    `json:"architecture"`
	Pricing          Pricing         `json:"pricing"`
	TopProvider      TopProvider     `json:"top_provider"`
	SupportedParams  []string        `json:"supported_parameters"`
	DefaultParams    json.RawMessage `json:"default_parameters"`
	KnowledgeCutoff  *string         `json:"knowledge_cutoff"`
	ExpirationDate   *string         `json:"expiration_date"`
	Reasoning        *Reasoning      `json:"reasoning"`
	Benchmarks       json.RawMessage `json:"benchmarks"`
	AliasTarget      *AliasTarget    `json:"alias_target"`
	HuggingFaceID    *string         `json:"hugging_face_id"`
	SupportedVoices  json.RawMessage `json:"supported_voices"`
	PerRequestLimits json.RawMessage `json:"per_request_limits"`
}

// AliasTarget names the model a "~latest"-style alias resolves to.
type AliasTarget struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Architecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Tokenizer        string   `json:"tokenizer"`
	InstructType     *string  `json:"instruct_type"`
}

type TopProvider struct {
	ContextLength       int64 `json:"context_length"`
	MaxCompletionTokens int64 `json:"max_completion_tokens"`
	IsModerated         bool  `json:"is_moderated"`
}

type Reasoning struct {
	Mandatory bool `json:"mandatory"`
}

// Pricing holds per-token USD prices as strings; OpenRouter returns them as decimal strings.
type Pricing struct {
	Prompt            string `json:"prompt"`
	Completion        string `json:"completion"`
	InputCacheRead    string `json:"input_cache_read"`
	InputCacheWrite   string `json:"input_cache_write"`
	InternalReasoning string `json:"internal_reasoning"`
	WebSearch         string `json:"web_search"`
	Image             string `json:"image"`
	Audio             string `json:"audio"`
	ImageOutput       string `json:"image_output"`
	AudioOutput       string `json:"audio_output"`
}

// InputPerM returns the prompt price in USD per 1M tokens, or -1 when unknown.
func (p Pricing) InputPerM() float64 { return perMillion(p.Prompt) }

// OutputPerM returns the completion price in USD per 1M tokens, or -1 when unknown.
func (p Pricing) OutputPerM() float64 { return perMillion(p.Completion) }

// CacheReadPerM returns the cache-read price in USD per 1M tokens, or -1 when unknown.
func (p Pricing) CacheReadPerM() float64 { return perMillion(p.InputCacheRead) }

// IsFree reports whether both prompt and completion are zero-priced.
func (p Pricing) IsFree() bool {
	in, out := p.InputPerM(), p.OutputPerM()
	return in == 0 && out == 0
}

func perMillion(s string) float64 {
	if s == "" {
		return -1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v * 1_000_000
}
