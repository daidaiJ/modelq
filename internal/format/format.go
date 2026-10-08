package format

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Tokens renders a token count compactly: 1048576 -> "1.05M", 131072 -> "131K".
func Tokens(n int64) string {
	if n <= 0 {
		return "-"
	}
	switch {
	case n >= 1_000_000:
		return trimZero(fmt.Sprintf("%.2f", float64(n)/1_000_000)) + "M"
	case n >= 1_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1_000)) + "K"
	default:
		return strconv.FormatInt(n, 10)
	}
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

// Truncate shortens s to at most w display columns, adding an ellipsis.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

// Pad right-pads s to width w based on rune count.
func Pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// Table renders a simple aligned table with a header separator.
func Table(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := utf8.RuneCountInString(cell); n > widths[i] {
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
