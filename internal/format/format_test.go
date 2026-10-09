package format

import "testing"

func TestTokens(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "-"},
		{-5, "-"},
		{999, "999"},
		{1000, "1K"},
		{999_499, "999K"},
		{131_072, "131K"},
		{524_288, "524K"},
		{1_048_576, "1M"}, // binary base still renders whole M
		{1_999_999, "2M"},
		{1_048_576_000, "1049M"},
		{943_717, "944K"},
	}
	for _, tc := range cases {
		if got := Tokens(tc.n); got != tc.want {
			t.Errorf("Tokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

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

func TestWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"来源", 4},
		{"上下文", 6},
		{"输入/M", 6}, // 2 CJK runes (4) + "/M" (2)
		{"¥20.1", 5},
	}
	for _, tc := range cases {
		if got := Width(tc.s); got != tc.want {
			t.Errorf("Width(%q) = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestPadCJK(t *testing.T) {
	// "来源" displays as 4 columns; padding to 10 adds 6 spaces, and the
	// padded result still measures 10 display columns.
	got := Pad("来源", 10)
	if w := Width(got); w != 10 {
		t.Errorf("Pad(%q, 10) width = %d, want 10", got, w)
	}
	if got := Pad("abcdef", 3); got != "abcdef" {
		t.Errorf("Pad overflow = %q, want %q", got, "abcdef")
	}
}

func TestTruncateCJK(t *testing.T) {
	if got := Truncate("上下文很长", 6); got != "上下…" {
		t.Errorf("Truncate CJK = %q, want %q", got, "上下…")
	}
	if got := Truncate("上下文", 6); got != "上下文" {
		t.Errorf("Truncate within width = %q, want %q", got, "上下文")
	}
}
