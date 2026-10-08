package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/daidaiJ/modelq/internal/api"
	"github.com/daidaiJ/modelq/internal/format"
	"github.com/daidaiJ/modelq/internal/locale"
	"github.com/daidaiJ/modelq/internal/modelsdev"
)

// loadModelsDev fetches the models.dev catalog and warns on stderr when a
// stale cached copy had to be served after a failed refresh.
func loadModelsDev(ctx context.Context, refresh bool) (*modelsdev.Catalog, error) {
	cat, err := modelsdev.Fetch(ctx, refresh, nil)
	if err != nil {
		return nil, err
	}
	if cat.Stale {
		stale := fmt.Sprintf("models.dev: refresh failed, using cached copy from %s", cat.FetchedAt.Format("2006-01-02 15:04"))
		fmt.Fprintln(os.Stderr, locale.T(stale, "models.dev: 刷新失败，使用 "+cat.FetchedAt.Format("2006-01-02 15:04")+" 的缓存副本"))
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

// sourceSel selects which catalogs a command touches.
type sourceSel struct {
	openrouter bool
	modelsdev  bool
}

// parseSource validates the -s/--source value; empty means both catalogs.
func parseSource(v string) (sourceSel, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "all":
		return sourceSel{openrouter: true, modelsdev: true}, nil
	case "openrouter", "or":
		return sourceSel{openrouter: true}, nil
	case "modelsdev", "models.dev", "models-dev", "md":
		return sourceSel{modelsdev: true}, nil
	default:
		return sourceSel{}, fmt.Errorf("%s", locale.T(
			fmt.Sprintf("unknown --source %q (valid: all, openrouter, modelsdev)", v),
			fmt.Sprintf("未知 --source %q（可选：all、openrouter、modelsdev）", v)))
	}
}

// modelHit is a resolved model from either catalog; Source discriminates:
// "openrouter" fills Model, "models.dev" fills ModelsDev.
type modelHit struct {
	Source    string           `json:"source"`
	Model     *api.Model       `json:"model,omitempty"`
	ModelsDev *modelsdev.Match `json:"models_dev,omitempty"`
}

// fetchModels loads the OpenRouter catalog using the resolved client.
func fetchModels(ctx context.Context, c *api.Client) ([]api.Model, error) {
	models, err := c.Models(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New(locale.T("openrouter returned an empty model catalog", "OpenRouter 返回了空模型目录"))
	}
	return models, nil
}

// resolveOpenRouter matches a user id against the OpenRouter catalog: exact,
// then unique prefix, then unique substring. found=false means no hit;
// a found result with m == nil is an ambiguous hit and Ambiguous lists the
// candidate ids.
func resolveOpenRouter(models []api.Model, q string) (m *api.Model, ambiguous []string, found bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil, false
	}
	lower := strings.ToLower(q)
	var prefix, substr []*api.Model
	for i := range models {
		id := strings.ToLower(models[i].ID)
		switch {
		case id == lower:
			return &models[i], nil, true
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
		return nil, nil, false
	case 1:
		return pick[0], nil, true
	default:
		ids := make([]string, 0, len(pick))
		for _, mm := range pick {
			ids = append(ids, mm.ID)
		}
		sort.Strings(ids)
		return nil, ids, true
	}
}

// ambiguousError renders an ambiguous-id error, capping the candidate list.
func ambiguousError(q string, ids []string) error {
	const maxShown = 10
	shown := ids
	if len(ids) > maxShown {
		shown = ids[:maxShown]
	}
	list := strings.Join(shown, "\n  ")
	if len(ids) > maxShown {
		list += locale.T(
			fmt.Sprintf("\n  ... and %d more (mqx search %s)", len(ids)-maxShown, q),
			fmt.Sprintf("\n  ……另有 %d 个（mqx search %s）", len(ids)-maxShown, q))
	}
	return fmt.Errorf("%s", locale.T(
		fmt.Sprintf("%q is ambiguous, candidates:\n  %s", q, list),
		fmt.Sprintf("%q 有歧义，候选：\n  %s", q, list)))
}

// matchModel resolves a user-supplied id with actionable errors (compare).
func matchModel(models []api.Model, q string) (*api.Model, error) {
	if strings.TrimSpace(q) == "" {
		return nil, errors.New(locale.T("empty model id", "模型 id 为空"))
	}
	m, ambiguous, found := resolveOpenRouter(models, q)
	switch {
	case found && m != nil:
		return m, nil
	case found:
		return nil, ambiguousError(q, ambiguous)
	default:
		return nil, fmt.Errorf("%s", locale.T(
			fmt.Sprintf("no model matches %q (try: mqx search %s)", q, q),
			fmt.Sprintf("没有模型匹配 %q（试试：mqx search %s）", q, q)))
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

// foldID lowercases and folds separators so terms match across catalogs that
// differ in "."/"_"/"-" conventions.
func foldID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ReplaceAll(s, ".", "-")
}

// mdHit is one models.dev search result.
type mdHit struct {
	ProviderID string
	Model      modelsdev.Model
}

// mdDisplayID renders a models.dev model as an addressable id; bare ids get
// the provider prefix so "zhipuai/glm-5.3-flash" round-trips through show.
func mdDisplayID(providerID string, m modelsdev.Model) string {
	if strings.Contains(m.ID, "/") {
		return m.ID
	}
	return providerID + "/" + m.ID
}

// searchModelsDev filters the models.dev catalog by AND-ed terms over id,
// name, family, canonical id, and provider id.
func searchModelsDev(cat *modelsdev.Catalog, query string) []mdHit {
	terms := strings.Fields(foldID(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []mdHit
	for pid, p := range cat.Providers {
		for _, m := range p.Models {
			hay := foldID(pid + " " + m.ID + " " + m.Name + " " + m.Family + " " + m.CanonicalModelID)
			ok := true
			for _, t := range terms {
				if !strings.Contains(hay, t) {
					ok = false
					break
				}
			}
			if ok {
				hits = append(hits, mdHit{ProviderID: pid, Model: m})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		return mdDisplayID(hits[i].ProviderID, hits[i].Model) < mdDisplayID(hits[j].ProviderID, hits[j].Model)
	})
	return hits
}

type filterOpts struct {
	FreeOnly   bool
	MinContext int64
	Modality   string
}

// renderList prints the compact table used by list (OpenRouter catalog).
func renderList(models []api.Model, limit int) string {
	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}
	headers := []string{
		locale.T("MODEL", "模型"),
		locale.T("CTX", "上下文"),
		locale.T("MAX OUT", "最大输出"),
		locale.T("INPUT/M", "输入/M"),
		locale.T("OUTPUT/M", "输出/M"),
		locale.T("CACHE/M", "缓存/M"),
	}
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
