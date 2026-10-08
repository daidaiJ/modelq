package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/daidaiJ/modelq/internal/api"
	"github.com/daidaiJ/modelq/internal/fx"
	"github.com/daidaiJ/modelq/internal/locale"
)

// fxRate resolves the USD→CNY rate once per run: Chinese text output uses it
// to annotate CNY prices and --json output attaches it as an fx field, so
// English text output is the only mode that skips the fetch entirely. nil
// means annotate/attach nothing.
func fxRate(ctx context.Context, refresh bool) *fx.Rate {
	if !locale.IsZH() && !flagJSON {
		return nil
	}
	r, err := fx.Fetch(ctx, refresh, nil)
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, locale.T(
			"exchange rate unavailable ("+err.Error()+"); showing USD prices only",
			"汇率获取失败（"+err.Error()+"）；仅显示美元价格"))
		return nil
	case r.Stale:
		stale := fmt.Sprintf("exchange rate: refresh failed, using cached rate from %s", r.FetchedAt.Format("2006-01-02 15:04"))
		fmt.Fprintln(os.Stderr, locale.T(stale, "汇率：刷新失败，使用 "+r.FetchedAt.Format("2006-01-02 15:04")+" 的缓存副本"))
	}
	return r
}

// cnyRate unwraps the rate; 0 makes format.WithCNY skip the CNY suffix.
func cnyRate(r *fx.Rate) float64 {
	if r == nil {
		return 0
	}
	return r.USDCNY
}

// fxNote renders the one-line provenance note shown under zh price output;
// empty when no rate is available.
func fxNote(r *fx.Rate) string {
	if r == nil {
		return ""
	}
	return locale.T(
		fmt.Sprintf("exchange rate: 1 USD = %.4f CNY (%s, %s)", r.USDCNY, r.Source, r.FetchedAt.Format("2006-01-02 15:04 MST")),
		fmt.Sprintf("汇率：1 USD = %.4f CNY（%s，%s）", r.USDCNY, r.Source, r.FetchedAt.Format("2006-01-02 15:04 MST")))
}

// fxModel inlines a model's JSON fields plus the resolved USD→CNY rate, so
// array-shaped --json output (list, compare) carries the rate without
// changing how the model fields themselves are addressed.
type fxModel struct {
	api.Model
	FX *fx.Rate `json:"fx,omitempty"`
}

// withFX pairs every model with the rate (possibly nil, which omits fx).
func withFX(models []api.Model, fxr *fx.Rate) []fxModel {
	out := make([]fxModel, len(models))
	for i, m := range models {
		out[i] = fxModel{Model: m, FX: fxr}
	}
	return out
}
