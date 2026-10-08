package format

import "testing"

func TestWithCNY(t *testing.T) {
	cases := []struct {
		name string
		usd  float64
		rate float64
		want string
	}{
		{"annotates", 3, 6.7, "$3 (¥20.1)"},
		{"small value", 0.002, 6.7, "$0.002 (¥0.0134)"},
		{"sub-one yuan", 0.5, 6.7, "$0.5 (¥3.35)"},
		{"hundreds", 217.4, 6.7, "$217.4 (¥1456.6)"},
		{"no rate", 3, 0, "$3"},
		{"unknown price", -1, 6.7, "-"},
		{"free price", 0, 6.7, "free"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithCNY(tc.usd, tc.rate); got != tc.want {
				t.Errorf("WithCNY(%v, %v) = %q, want %q", tc.usd, tc.rate, got, tc.want)
			}
		})
	}
}
