package main

import (
	"os"
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
	if _, err := parseArgs([]string{"q", "--verbose"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
	dir := t.TempDir()
	spaced := dir + "/my file.py"
	if err := os.WriteFile(spaced, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	o, err = parseArgs([]string{"--file", dir + "/my", "file.py", "add", "types"})
	if err != nil || o.file != spaced || o.text != "add types" {
		t.Fatalf("spaced path: %+v %v", o, err)
	}
	sp := dir + "/my dir"
	if err := os.Mkdir(sp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sp+"/my file.txt", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	o, err = parseArgs([]string{"--file", dir + "/my", "dir/my", "file.txt", "fix", "it"})
	if err != nil || o.file != sp+"/my file.txt" || o.text != "fix it" {
		t.Fatalf("spaced dir: %+v %v", o, err)
	}
	if _, err := finish(" \n", "x\n"); err == nil {
		t.Fatal("blank reply accepted")
	}
}

func TestFinishNewline(t *testing.T) {
	for _, c := range []struct{ out, in, want string }{
		{"```py\nx = 1\n```", "y = 2", "x = 1"},
		{"x = 1\n", "y = 2\n", "x = 1\n"},
		{"x\ny\n", "a\r\nb\r\n", "x\r\ny\r\n"},
		{"x\r\ny", "a\r\nb", "x\r\ny"},
		{"a\nb", "    a\n    b", "    a\n    b"},
	} {
		if got, _ := finish(c.out, c.in); got != c.want {
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
