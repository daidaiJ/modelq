package locale

import "testing"

func TestFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		want Lang
		err  bool
	}{
		{"default", nil, "", EN, false},
		{"env zh", nil, "zh", ZH, false},
		{"env zh-CN normalizes", nil, "zh-CN", ZH, false},
		{"env zh_CN normalizes", nil, "zh_CN", ZH, false},
		{"env en", nil, "en", EN, false},
		{"flag beats env", []string{"--lang", "en"}, "zh", EN, false},
		{"flag equals form", []string{"--lang=zh"}, "en", ZH, false},
		{"after subcommand", []string{"dev", "--lang", "zh"}, "", ZH, false},
		{"last wins", []string{"--lang", "zh", "--lang", "en"}, "", EN, false},
		{"invalid", []string{"--lang", "fr"}, "", EN, true},
		{"flag without value left to cobra", []string{"--lang"}, "", EN, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang = EN
			err := FromArgs(tc.args, tc.env)
			if tc.err {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lang != tc.want {
				t.Errorf("lang = %v, want %v", lang, tc.want)
			}
		})
	}
	lang = EN
}

func TestT(t *testing.T) {
	lang = EN
	if got := T("hello", "你好"); got != "hello" {
		t.Errorf("en: got %q, want %q", got, "hello")
	}
	lang = ZH
	if got := T("hello", "你好"); got != "你好" {
		t.Errorf("zh: got %q, want %q", got, "你好")
	}
	lang = EN
}
