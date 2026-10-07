package assist

import (
	"strings"
	"testing"
)

func lines(s string) []string { return strings.Split(s, "\n") }

func TestEnclosingBlockPython(t *testing.T) {
	src := lines("import os\n\n@deco\ndef f(x):\n    y = x\n    if y:\n        return 1\n    return 2\n\ndef g():\n    pass")
	s, e := EnclosingBlock(src, 6)
	if s != 2 || e != 7 {
		t.Fatalf("got %d-%d want 2-7", s, e)
	}
}

func TestEnclosingBlockBraces(t *testing.T) {
	src := lines("package x\n\nfunc a() {\n\tb := 1\n\t_ = b\n}\n\nfunc c() {}")
	s, e := EnclosingBlock(src, 4)
	if s != 2 || e != 5 {
		t.Fatalf("got %d-%d want 2-5", s, e)
	}
}

func TestEnclosingBlockTopLevelParagraph(t *testing.T) {
	src := lines("a = 1\nb = 2\n\nc = 3")
	s, e := EnclosingBlock(src, 1)
	if s != 0 || e != 1 {
		t.Fatalf("got %d-%d want 0-1", s, e)
	}
}

func TestFindInstructions(t *testing.T) {
	src := lines("x = 1\n    # ai: add retry\n// AI: make async\n-- ai> fix sql\ny = 'ai: not a comment'")
	got := FindInstructions(src, 0, len(src)-1)
	want := []string{"add retry", "make async", "fix sql"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if got[i].Instruction != w {
			t.Fatalf("%d: got %q want %q", i, got[i].Instruction, w)
		}
	}
}

func TestInstructionTarget(t *testing.T) {
	src := lines("# ai: speed up\nfor i in r:\n    work(i)\n\nother()")
	s, e := InstructionTarget(src, 0)
	if s != 0 || e != 2 {
		t.Fatalf("got %d-%d want 0-2", s, e)
	}
	lone := lines("x()\n# ai: write a parser\n\ny()")
	if s, e := InstructionTarget(lone, 1); s != 1 || e != 1 {
		t.Fatalf("lone got %d-%d", s, e)
	}
}

func TestFixIndent(t *testing.T) {
	got := FixIndent("a = 1\nb = 2\n", "    a=1\n    b=2\n")
	if got != "    a = 1\n    b = 2\n" {
		t.Fatalf("got %q", got)
	}
	if got := FixIndent("    a\n", "    x\n"); got != "    a\n" {
		t.Fatalf("kept indent: %q", got)
	}
}

func TestFileWithMarkersWindow(t *testing.T) {
	src := make([]string, 5000)
	for i := range src {
		src[i] = "l"
	}
	out := FileWithMarkers(src, 2500, 2501)
	if !strings.Contains(out, "TARGET START") || !strings.Contains(out, "lines above omitted") || strings.Count(out, "\n") > MaxContextLines+10 {
		t.Fatal("bad window")
	}
}

func TestStripPreamble(t *testing.T) {
	if got := StripPreamble("Here you go:\n```go\nx := 1\n```\n"); got != "x := 1" {
		t.Fatalf("got %q", got)
	}
}

func TestTestPath(t *testing.T) {
	cases := map[string][2]string{
		"python": {"/a/b.py", "/a/test_b.py"},
		"go":     {"/a/b.go", "/a/b_test.go"},
	}
	for lang, c := range cases {
		if got := TestPath(c[0], lang); got != c[1] {
			t.Fatalf("%s: %s", lang, got)
		}
	}
}
