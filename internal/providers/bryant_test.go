package providers

import "testing"

func TestStripSentinel(t *testing.T) {
	for in, want := range map[string]string{
		"@@@\n        x = 1": "        x = 1",
		"@@@\r\n  y":         "  y",
		"no sentinel":        "no sentinel",
		"\n@@@\nz":           "z",
	} {
		if got := stripSentinel(in); got != want {
			t.Errorf("stripSentinel(%q)=%q want %q", in, got, want)
		}
	}
}
