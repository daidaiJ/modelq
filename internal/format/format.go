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

// Truncate shortens s to at most w display columns, adding an ellipsis.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if displayWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	var b strings.Builder
	cols := 0
	for _, r := range s {
		rw := runeWidth(r)
		if cols+rw > w-1 {
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
// ranges, 1 otherwise. Zero-width runes are not expected in table content.
func runeWidth(r rune) int {
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
