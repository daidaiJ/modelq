package modelsdev

import (
	"sort"
	"strings"
)

// Match is one catalog entry that answered a query, ranked by Source.
type Match struct {
	ProviderID string `json:"provider_id"`
	Model      Model  `json:"model"`
	Source     string `json:"source"`
	Score      int    `json:"score"`
}

// maxMatches caps how many fuzzy candidates a single query can return.
const maxMatches = 32

// Match resolves an approximate model id against the catalog. Confidence
// tiers, highest first:
//
//	100  exact id in the openrouter provider (OpenRouter-style id)
//	 95  exact id minus any ":variant" suffix (e.g. ":free") in openrouter
//	 90  exact id in any other provider
//	 86  "vendor/model" where vendor maps to a provider and model is exact
//	 85  canonical_model_id equals the query
//	 84  same, comparing model ids with separators normalized
//	 60  provider-qualified model id prefix
//	 40  substring of a model id anywhere in the catalog
//	 30  substring of a display name
//
// Results are sorted by score, then provider and model id, capped at
// maxMatches.
func (c *Catalog) Match(query string) []Match {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	base := q
	if i := strings.Index(q, ":"); i >= 0 {
		base = q[:i]
	}
	qn, bn := normID(q), normID(base)

	type key struct{ provider, id string }
	seen := make(map[key]int)
	var out []Match
	add := func(pid string, p Provider, m Model, source string, score int) {
		k := key{pid, m.ID}
		if prev, ok := seen[k]; ok {
			if score > prev {
				for i := range out {
					if out[i].ProviderID == pid && out[i].Model.ID == m.ID {
						out[i].Source, out[i].Score = source, score
					}
				}
				seen[k] = score
			}
			return
		}
		seen[k] = score
		out = append(out, Match{ProviderID: pid, Model: m, Source: source, Score: score})
	}

	// 1/2: OpenRouter provider, with and without the ":variant" suffix.
	if p, ok := c.Providers["openrouter"]; ok {
		if m, ok := p.Models[q]; ok {
			add("openrouter", p, m, "openrouter", 100)
		}
		if base != q {
			if m, ok := p.Models[base]; ok {
				add("openrouter", p, m, "openrouter", 95)
			}
		}
	}

	// 3: exact id in any provider (variant-stripped too).
	for pid, p := range c.Providers {
		if m, ok := p.Models[q]; ok {
			add(pid, p, m, "id", 90)
		}
		if base != q {
			if m, ok := p.Models[base]; ok {
				add(pid, p, m, "id", 89)
			}
		}
	}

	// 4: canonical_model_id reverse lookup.
	for pid, p := range c.Providers {
		for _, m := range p.Models {
			if m.CanonicalModelID != "" && normID(m.CanonicalModelID) == bn {
				add(pid, p, m, "canonical", 85)
			}
		}
	}

	// 5: "vendor/model" with dash-collapsed provider aliases.
	if vendor, model, ok := strings.Cut(base, "/"); ok {
		vn, mn := normID(vendor), normID(model)
		for pid, p := range c.Providers {
			pidN := normID(pid)
			if pidN != vn && collapse(pid) != collapse(vendor) {
				continue
			}
			if m, ok := p.Models[model]; ok {
				add(pid, p, m, "vendor", 86)
				continue
			}
			for mid, m := range p.Models {
				switch {
				case normID(mid) == mn:
					add(pid, p, m, "vendor", 84)
				case len(mn) >= 4 && strings.HasPrefix(normID(mid), mn):
					add(pid, p, m, "vendor-prefix", 60)
				}
			}
		}
	}

	// 6: substring fallback over ids and display names.
	for pid, p := range c.Providers {
		for mid, m := range p.Models {
			if qn != "" && strings.Contains(normID(mid), qn) {
				add(pid, p, m, "substring", 40)
				continue
			}
			if len(qn) >= 3 && strings.Contains(strings.ToLower(m.Name), strings.ToLower(q)) {
				add(pid, p, m, "substring", 30)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].ProviderID != out[j].ProviderID {
			return out[i].ProviderID < out[j].ProviderID
		}
		return out[i].Model.ID < out[j].Model.ID
	})
	if len(out) > maxMatches {
		out = out[:maxMatches]
	}
	return out
}
