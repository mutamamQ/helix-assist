package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"make", "it", "--file", "a.go", "--line=7", "fast", "--deep"})
	if err != nil || o.text != "make it fast" || o.file != "a.go" || o.line != 7 || o.model != modelDeep {
		t.Fatalf("%+v %v", o, err)
	}
	if _, err := parseArgs([]string{"--file"}); err == nil {
		t.Fatal("want error")
	}
	if _, err := parseArgs([]string{"--line", "x"}); err == nil {
		t.Fatal("want error")
	}
}

func TestFinishNewline(t *testing.T) {
	for _, c := range []struct{ out, in, want string }{
		{"```py\nx = 1\n```", "y = 2", "x = 1"},
		{"x = 1\n", "y = 2\n", "x = 1\n"},
		{"a\nb", "    a\n    b", "    a\n    b"},
	} {
		if got := finish(c.out, c.in); got != c.want {
			t.Errorf("finish(%q,%q)=%q want %q", c.out, c.in, got, c.want)
		}
	}
}

func TestKeyRemaps(t *testing.T) {
	toml := "theme = \"x\"\n[editor]\nmouse = false\n[keys.normal]\nC-s = \":w\"\n\n[keys.normal.space.i]\ne = \"x\"\n[editor.lsp]\na = 1\n"
	got := keyRemaps(toml)
	if !strings.Contains(got, "C-s") || !strings.Contains(got, "[keys.normal.space.i]") || strings.Contains(got, "mouse") || strings.Contains(got, "a = 1") {
		t.Fatalf("got %q", got)
	}
	p := keysSystem("DOCS", got)
	if !strings.Contains(p, "DOCS") || !strings.Contains(p, "C-s") || strings.Contains(keysSystem("D", ""), "REMAPS") {
		t.Fatal("prompt")
	}
}
