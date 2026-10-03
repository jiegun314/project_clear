package service

import "testing"

// The log window is Chinese, so the 来源 column must not show the machine names
// the import pipeline uses internally.
func TestLogSourceIsChinese(t *testing.T) {
	cases := map[string]string{
		"import": "导入",
		"add":    "添加",
		// Anything else is passed through so a future action name still shows up.
		"merge": "merge",
		"":      "",
	}
	for in, want := range cases {
		if got := logSource(in); got != want {
			t.Errorf("logSource(%q) = %q, want %q", in, got, want)
		}
	}
}
