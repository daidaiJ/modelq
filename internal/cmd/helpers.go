package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/daidaiJ/openrouter-cli/internal/api"
	"github.com/daidaiJ/openrouter-cli/internal/format"
	"github.com/daidaiJ/openrouter-cli/internal/modelsdev"
)

// loadModelsDev fetches the models.dev catalog and warns on stderr when a
// stale cached copy had to be served after a failed refresh.
func loadModelsDev(ctx context.Context, refresh bool) (*modelsdev.Catalog, error) {
	cat, err := modelsdev.Fetch(ctx, refresh, nil)
	if err != nil {
		return nil, err
	}
	if cat.Stale {
		fmt.Fprintf(os.Stderr, "models.dev: refresh failed, using cached copy from %s\n",
			cat.FetchedAt.Format("2006-01-02 15:04"))
	}
	return cat, nil
}

// modelsDevRef picks the most confident models.dev match for an OpenRouter
// id (exact, canonical, or vendor-level); nil when only fuzzy substring
// matches exist, which would be too noisy to enrich with.
func modelsDevRef(cat *modelsdev.Catalog, openrouterID string) *modelsdev.Match {
	if cat == nil {
		return nil
	}
	for _, m := range cat.Match(openrouterID) {
		if m.Score >= 80 {
			mm := m
			return &mm
		}
	}
	return nil
}

// fetchModels loads the catalog using the resolved client.
func fetchModels(ctx context.Context, c *api.Client) ([]api.Model, error) {
	models, err := c.Models(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("openrouter returned an empty model catalog")
	}
	return models, nil
}

// matchModel resolves a user-supplied id: exact match first, then prefix, then substring.
func matchModel(models []api.Model, q string) (*api.Model, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("empty model id")
	}
	lower := strings.ToLower(q)
	var prefix, substr []*api.Model
	for i := range models {
		id := strings.ToLower(models[i].ID)
		switch {
		case id == lower:
			return &models[i], nil
		case strings.HasPrefix(id, lower):
			prefix = append(prefix, &models[i])
		case strings.Contains(id, lower):
			substr = append(substr, &models[i])
		}
	}
	pick := prefix
	if len(pick) == 0 {
		pick = substr
	}
	switch len(pick) {
	case 0:
		return nil, fmt.Errorf("no model matches %q (try: orx search %s)", q, q)
	case 1:
		return pick[0], nil
	default:
		ids := make([]string, 0, len(pick))
		for _, m := range pick {
			ids = append(ids, m.ID)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("%q is ambiguous, candidates:\n  %s", q, strings.Join(ids, "\n  "))
	}
}

// sortModels orders models by the given key. Unknown keys fall back to id.
func sortModels(models []api.Model, key string, desc bool) {
	less := func(i, j int) bool { return models[i].ID < models[j].ID }
	switch strings.ToLower(key) {
	case "input", "in", "prompt":
		less = func(i, j int) bool { return lessFloat(models[i].Pricing.InputPerM(), models[j].Pricing.InputPerM()) }
	case "output", "out", "completion":
		less = func(i, j int) bool { return lessFloat(models[i].Pricing.OutputPerM(), models[j].Pricing.OutputPerM()) }
	case "ctx", "context":
		less = func(i, j int) bool { return models[i].ContextLength < models[j].ContextLength }
	case "maxout", "max_completion":
		less = func(i, j int) bool {
			return models[i].TopProvider.MaxCompletionTokens < models[j].TopProvider.MaxCompletionTokens
		}
	case "name":
		less = func(i, j int) bool { return models[i].Name < models[j].Name }
	}
	sort.SliceStable(models, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}

// lessFloat compares prices, pushing unknown (-1) to the end.
func lessFloat(a, b float64) bool {
	if a < 0 {
		return false
	}
	if b < 0 {
		return true
	}
	return a < b
}

func filterModels(models []api.Model, opts filterOpts) []api.Model {
	out := models[:0:0]
	for _, m := range models {
		if opts.FreeOnly && !m.Pricing.IsFree() {
			continue
		}
		if opts.MinContext > 0 && m.ContextLength < opts.MinContext {
			continue
		}
		if opts.Modality != "" && !strings.Contains(strings.ToLower(m.Architecture.Modality), strings.ToLower(opts.Modality)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func searchModels(models []api.Model, query string) []api.Model {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return models
	}
	var out []api.Model
	for _, m := range models {
		hay := strings.ToLower(m.ID + " " + m.Name)
		ok := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, m)
		}
	}
	return out
}

type filterOpts struct {
	FreeOnly   bool
	MinContext int64
	Modality   string
}

type listOpts struct {
	filterOpts
	Sort  string
	Desc  bool
	Limit int
	JSON  bool
}

// renderList prints the compact table used by list and search.
func renderList(models []api.Model, limit int) string {
	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}
	headers := []string{"MODEL", "CTX", "MAX OUT", "INPUT/M", "OUTPUT/M", "CACHE/M"}
	rows := make([][]string, 0, len(models))
	for _, m := range models {
		rows = append(rows, []string{
			format.Truncate(m.ID, 46),
			format.Tokens(m.ContextLength),
			format.Tokens(m.TopProvider.MaxCompletionTokens),
			format.Price(m.Pricing.InputPerM()),
			format.Price(m.Pricing.OutputPerM()),
			format.Price(m.Pricing.CacheReadPerM()),
		})
	}
	return format.Table(headers, rows)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
