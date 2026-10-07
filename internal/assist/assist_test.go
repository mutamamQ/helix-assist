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

func TestEnclosingBlockRustDocs(t *testing.T) {
	src := lines("/// Adds.\n/// ```\n/// x\n/// ```\nfn add() -> i32 {\n    1\n}")
	if s, e := EnclosingBlock(src, 5); s != 0 || e != 6 {
		t.Fatalf("got %d-%d want 0-6", s, e)
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
	src := lines("x = 1\n    # ai: add retry\n// AI: make async\n-- ai> fix sql\ny = 'ai: not a comment'\n    \"ai: docstring line\"")
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

func TestCleanReply(t *testing.T) {
	ok := map[[2]string]string{
		{"Here you go:\n```go\nx := 1\n```\n", "y"}:                      "x := 1",
		{"```py\nx = 1\n```", "y"}:                                       "x = 1",
		{"# Title\n```sh\nls -la\n```\nmore", "# Title\n```sh\nls\n```"}: "# Title\n```sh\nls -la\n```\nmore",
		{"/// ```\n/// assert!(f())\n/// ```\nfn f() {}", "fn f() {}"}:   "/// ```\n/// assert!(f())\n/// ```\nfn f() {}",
		{"    x = 1\n", "    y"}:                                         "    x = 1",
	}
	for in, want := range ok {
		if got, err := CleanReply(in[0], in[1]); err != nil || got != want {
			t.Errorf("CleanReply(%q)=%q,%v want %q", in[0], got, err, want)
		}
	}
	for _, bad := range []string{"", "```\n```", "Sorry, I can't help with that request.", "I cannot do that"} {
		if got, err := CleanReply(bad, "def f():\n    pass"); err == nil {
			t.Errorf("CleanReply(%q) accepted: %q", bad, got)
		}
	}
}

func TestDuplicateTests(t *testing.T) {
	if d := DuplicateTests("def test_f():\n    pass", "import x\n\ndef test_f():\n    assert 1"); d != "test_f" {
		t.Fatalf("got %q", d)
	}
	if d := DuplicateTests("func TestB(t *testing.T) {}", "func TestA(t *testing.T) {}"); d != "" {
		t.Fatalf("got %q", d)
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

func TestEnclosingBlockUncapped(t *testing.T) {
	src := []string{"def f():"}
	for i := 0; i < 1600; i++ {
		src = append(src, "    x = 1")
	}
	if s, e := EnclosingBlock(src, 1550); s != 0 || e != 1600 {
		t.Fatalf("got %d-%d", s, e)
	}
}

func TestDuplicateTestsJS(t *testing.T) {
	ex := "describe('math', () => {\n  it.only(\"adds numbers\", () => {});\n})\n"
	for _, code := range []string{"test('adds numbers', () => {});", "  it.skip(`adds numbers`, () => {})", "describe(\"math\", () => {})"} {
		if DuplicateTests(code, ex) == "" {
			t.Errorf("missed duplicate %q", code)
		}
	}
	if d := DuplicateTests("it('subtracts', () => {})", ex); d != "" {
		t.Fatalf("false positive %q", d)
	}
}

func TestLeadingInstructions(t *testing.T) {
	src := lines("x = 1\n# note\n# ai: add docstring\ndef f():\n    pass")
	got := LeadingInstructions(src, 3)
	if len(got) != 1 || got[0].Line != 2 {
		t.Fatalf("got %+v", got)
	}
	if got := LeadingInstructions(lines("# ai: old\ny = 1\ndef f():\n    pass"), 2); len(got) != 0 {
		t.Fatalf("crossed code: %+v", got)
	}
}
