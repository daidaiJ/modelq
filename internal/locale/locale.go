// Package locale resolves and applies the CLI's human-facing language.
// Machine-facing identifiers (field labels, JSON keys) stay in English;
// help text, errors, hints, and table headers are localized via T.
package locale

import (
	"fmt"
	"strings"
)

// Lang is a supported UI language.
type Lang int

const (
	// EN is the default language.
	EN Lang = iota
	// ZH is Simplified Chinese.
	ZH
)

var lang = EN

// FromArgs resolves the language before the command tree is built (help
// strings are baked in at construction time): the env value applies first,
// then the --lang flag (last occurrence wins, anywhere in args). Supported
// values: en, zh; zh-CN/zh_CN normalize to zh.
func FromArgs(args []string, env string) error {
	v := env
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--lang" && i+1 < len(args) {
			v = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--lang=") {
			v = strings.TrimPrefix(a, "--lang=")
		}
	}
	switch normalize(v) {
	case "":
		return nil
	case "zh":
		lang = ZH
	case "en":
	default:
		return fmt.Errorf("unknown language %q (supported: en, zh) / 未知语言 %q（支持：en、zh）", v, v)
	}
	return nil
}

// IsZH reports whether the UI language is Chinese.
func IsZH() bool { return lang == ZH }

// normalize folds regional suffixes so "zh-CN" and "zh_CN" select zh.
func normalize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "-_"); i > 0 {
		v = v[:i]
	}
	return v
}

// T returns the Chinese variant when the UI language is zh, the English one
// otherwise. Both variants are written inline at the call site.
func T(en, zh string) string {
	if lang == ZH {
		return zh
	}
	return en
}
