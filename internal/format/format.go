package format

import (
	"fmt"
	"strconv"
	"strings"
)

// Tokens renders a token count as whole K/M units: 1048576 -> "1M",
// 131072 -> "131K". Values are rounded to the nearest unit regardless of
// whether the underlying catalog uses 1000 or 1024 as its base.
func Tokens(n int64) string {
	if n <= 0 {
		return "-"
	}
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	k := (n + 500) / 1000
	if k >= 1000 {
		return fmt.Sprintf("%dM", (k+500)/1000)
	}
	return fmt.Sprintf("%dK", k)
}

// TokensExact renders an exact count with thousands separators.
func TokensExact(n int64) string {
	if n <= 0 {
		return "-"
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Price renders USD-per-million-tokens. Unknown is "-", free is "free".
func Price(perM float64) string {
	switch {
	case perM < 0:
		return "-"
	case perM == 0:
		return "free"
	case perM < 0.01:
		return "$" + trimZero(fmt.Sprintf("%.4f", perM))
	case perM < 1:
		return "$" + trimZero(fmt.Sprintf("%.3f", perM))
	default:
		return "$" + trimZero(fmt.Sprintf("%.2f", perM))
	}
}

func trimZero(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// WithCNY renders a USD-per-1M price and, when rate > 0, its CNY equivalent
// in parentheses. Unknown stays "-", free stays "free".
func WithCNY(usdPerM, rate float64) string {
	p := Price(usdPerM)
	if rate <= 0 || usdPerM <= 0 {
		return p
	}
	return p + " (¥" + cny(usdPerM*rate) + ")"
}

// cny formats a CNY value with the same precision ladder as Price.
func cny(v float64) string {
	switch {
	case v < 0.1:
		return trimZero(fmt.Sprintf("%.4f", v))
	case v < 1:
		return trimZero(fmt.Sprintf("%.3f", v))
	case v < 100:
		return trimZero(fmt.Sprintf("%.2f", v))
	default:
		return trimZero(fmt.Sprintf("%.1f", v))
	}
}

// eastAsian marks a terminal whose font renders East Asian Ambiguous runes
// (¥, …, ™, ...) as 2 columns, the common case for CJK-locale terminals.
// It is set once at startup from the resolved UI language.
var eastAsian bool

// SetEastAsian selects how ambiguous-width runes are measured: wide (2
// columns, CJK terminals) or narrow (1 column, Latin terminals).
func SetEastAsian(v bool) {
	eastAsian = v
}

// Truncate shortens s to at most w display columns, adding an ellipsis.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if displayWidth(s) <= w {
		return s
	}
	ew := runeWidth('…')
	if w == ew {
		return "…"
	}
	if w < ew {
		return ""
	}
	var b strings.Builder
	cols := 0
	for _, r := range s {
		rw := runeWidth(r)
		if cols+rw > w-ew {
			break
		}
		b.WriteRune(r)
		cols += rw
	}
	return b.String() + "…"
}

// Pad right-pads s to width w based on terminal display columns.
func Pad(s string, w int) string {
	n := displayWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// displayWidth returns the terminal column count of s, counting East Asian
// wide runes (CJK, Hangul, Kana, fullwidth forms, common emoji) as 2 columns
// so tables with Chinese headers stay aligned.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// Width returns the terminal display column count of s.
func Width(s string) int {
	return displayWidth(s)
}

// runeWidth reports the display width of one rune: 2 for East Asian wide
// ranges, 1 otherwise. When eastAsian is set (CJK-locale terminals) the
// ambiguous-width runes below also count as 2; those are exactly the runes
// CJK terminal fonts render fullwidth, so ignoring them drifts every body
// row containing ¥ or a truncated "…" name off its header columns.
func runeWidth(r rune) int {
	if eastAsian {
		switch {
		case r >= 0x00A1 && r <= 0x00A5, // ¡ ¤ £ ¥
			r >= 0x00A7 && r <= 0x00AA,   // § ¨ ª
			r >= 0x00B0 && r <= 0x00B3,   // ° ± ² ³
			r >= 0x00B7 && r <= 0x00B8,   // · ¸
			r == 0x00D7, r == 0x00F7,     // × ÷
			r >= 0x2013 && r <= 0x2014,   // – —
			r >= 0x2018 && r <= 0x2019,   // ‘ ’
			r >= 0x201C && r <= 0x201D,   // “ ”
			r >= 0x2020 && r <= 0x2023,   // † ‡ • ′
			r >= 0x2024 && r <= 0x2026,   // …
			r == 0x2030,                  // ‰
			r >= 0x2032 && r <= 0x2033,   // ′ ″
			r == 0x203B,                  // ※
			r == 0x203E,                  // ‾
			r == 0x20A9,                  // ₩
			r >= 0x2103 && r <= 0x2105,   // ℃ ℅
			r == 0x2109,                  // ℉
			r == 0x2116,                  // №
			r == 0x2121, r == 0x2122,     // ℡ ™
			r == 0x212B,                  // Å
			r >= 0x2190 && r <= 0x2199,   // ← → arrows
			r == 0x21D2, r == 0x21D4,     // ⇒ ⇔
			r == 0x2202, r == 0x2206,     // ∂ ∆
			r == 0x220F, r == 0x2211,     // ∏ ∑
			r == 0x221A, r == 0x221E,     // √ ∞
			r == 0x222B, r == 0x2248,     // ∫ ≈
			r >= 0x2260 && r <= 0x2262,   // ≠ ≡
			r >= 0x2264 && r <= 0x2267,   // ≦ ≧
			r >= 0x2460 && r <= 0x24FF,   // ① circled numbers
			r >= 0x25A0 && r <= 0x25FF,   // ■ ● geometric shapes
			r >= 0x2605 && r <= 0x2606,   // ★ ☆
			r == 0x2640, r == 0x2642,     // ♀ ♂
			r >= 0x2660 && r <= 0x2669: // ♠ ♣ card suits
			return 2
		}
	}
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK Radicals..CJK Symbols
		r >= 0x3041 && r <= 0x33FF, // Hiragana..CJK Compatibility
		r >= 0x3400 && r <= 0x4DBF, // CJK Extension A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xA960 && r <= 0xA97F, // Hangul Jamo Extended-A
		r >= 0xAC00 && r <= 0xD7A3, // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility Ideographs
		r >= 0xFE10 && r <= 0xFE19, // Vertical Forms
		r >= 0xFE30 && r <= 0xFE6F, // CJK Compatibility Forms
		r >= 0xFF00 && r <= 0xFF60, // Fullwidth Forms
		r >= 0xFFE0 && r <= 0xFFE6, // Fullwidth Signage
		r >= 0x1F300 && r <= 0x1F64F, // emoji pictographs
		r >= 0x1F900 && r <= 0x1F9FF, // emoji supplements
		r >= 0x20000 && r <= 0x2FFFD, // CJK Extension B+
		r >= 0x30000 && r <= 0x3FFFD: // CJK Extension G+
		return 2
	default:
		return 1
	}
}

// Table renders a simple aligned table with a header separator.
func Table(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := displayWidth(cell); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	var b strings.Builder
	for i, h := range headers {
		b.WriteString(Pad(h, widths[i]))
		if i < len(headers)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')
	for i, w := range widths {
		b.WriteString(strings.Repeat("-", w))
		if i < len(widths)-1 {
			b.WriteString("  ")
		}
	}
	for _, row := range rows {
		b.WriteByte('\n')
		for i := range headers {
			var cell string
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(Pad(cell, widths[i]))
			if i < len(headers)-1 {
				b.WriteString("  ")
			}
		}
	}
	return strings.TrimRight(b.String(), " ")
}
