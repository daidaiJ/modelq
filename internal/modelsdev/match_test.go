package modelsdev

import "testing"

func testCatalog() *Catalog {
	f := func(v float64) *float64 { return &v }
	return &Catalog{Providers: map[string]Provider{
		"openrouter": {ID: "openrouter", Name: "OpenRouter", Models: map[string]Model{
			"anthropic/claude-sonnet-4.5":      {ID: "anthropic/claude-sonnet-4.5", Name: "Claude Sonnet 4.5"},
			"deepseek/deepseek-chat-v3.1":      {ID: "deepseek/deepseek-chat-v3.1", Name: "DeepSeek V3.1"},
			"deepseek/deepseek-chat-v3.1:free": {ID: "deepseek/deepseek-chat-v3.1:free", Name: "DeepSeek V3.1 (free)"},
		}},
		"zai": {ID: "zai", Name: "Z.ai", Models: map[string]Model{
			"glm-5.3-flash": {ID: "glm-5.3-flash", Name: "GLM-5.3-Flash", Cost: Cost{Input: f(0.15)}},
		}},
		"anthropic": {ID: "anthropic", Name: "Anthropic", Models: map[string]Model{
			"claude-sonnet-4-5": {ID: "claude-sonnet-4-5", CanonicalModelID: "anthropic/claude-sonnet-4-5"},
		}},
	}}
}

func TestMatch(t *testing.T) {
	cat := testCatalog()

	top := func(q string) Match {
		t.Helper()
		matches := cat.Match(q)
		if len(matches) == 0 {
			t.Fatalf("Match(%q): no results", q)
		}
		return matches[0]
	}

	t.Run("exact openrouter id", func(t *testing.T) {
		m := top("anthropic/claude-sonnet-4.5")
		if m.ProviderID != "openrouter" || m.Score != 100 {
			t.Errorf("got %s/%d, want openrouter/100", m.ProviderID, m.Score)
		}
	})

	t.Run("variant suffix stripped", func(t *testing.T) {
		m := top("deepseek/deepseek-chat-v3.1:batch")
		if m.Model.ID != "deepseek/deepseek-chat-v3.1" {
			t.Errorf("got %s, want the base entry", m.Model.ID)
		}
	})

	t.Run("vendor alias with separator folding", func(t *testing.T) {
		m := top("z-ai/glm-5.3-flash")
		if m.ProviderID != "zai" {
			t.Errorf("got provider %s, want zai", m.ProviderID)
		}
	})

	t.Run("canonical_model_id with normalized separators", func(t *testing.T) {
		m := top("anthropic/claude_sonnet_4_5")
		if m.ProviderID != "anthropic" || m.Source != "canonical" {
			t.Errorf("got %s/%s, want anthropic/canonical", m.ProviderID, m.Source)
		}
	})

	t.Run("substring fallback", func(t *testing.T) {
		m := top("glm-5.3")
		if m.ProviderID != "zai" || m.Model.ID != "glm-5.3-flash" {
			t.Errorf("got %s/%s, want zai/glm-5.3-flash", m.ProviderID, m.Model.ID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		if got := cat.Match("zzz-qqq-xxx"); len(got) != 0 {
			t.Errorf("expected no matches, got %d", len(got))
		}
	})

	t.Run("empty query", func(t *testing.T) {
		if got := cat.Match("   "); len(got) != 0 {
			t.Errorf("expected no matches, got %d", len(got))
		}
	})
}
