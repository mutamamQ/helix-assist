package handlers

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leona/helix-assist/internal/lsp"
)

func titles(as []lsp.CodeAction) string {
	var t []string
	for _, a := range as {
		t = append(t, a.Title)
	}
	return strings.Join(t, "|")
}

func params(line int, ds ...lsp.Diagnostic) lsp.CodeActionParams {
	return lsp.CodeActionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: "file:///x.py"},
		Range:        lsp.Range{Start: lsp.Position{Line: line}, End: lsp.Position{Line: line, Character: 1}},
		Context:      lsp.CodeActionContext{Diagnostics: ds},
	}
}

func TestBuildActions(t *testing.T) {
	lines := strings.Split("def f():\n    # ai: add logging\n    return x\n", "\n")
	d1 := lsp.Diagnostic{Message: "undefined name 'x'"}
	d2 := lsp.Diagnostic{Message: "other"}

	none := buildActions(lines, "go", params(0))
	if strings.Contains(titles(none), "fix") || strings.Contains(titles(none), "type hints") {
		t.Errorf("no diagnostics / go: %s", titles(none))
	}
	one := buildActions(lines, "python", params(2, d1))
	if !strings.HasPrefix(titles(one), "AI fix: undefined name 'x'|AI: add logging|") || strings.Contains(titles(one), "fix all") {
		t.Errorf("one diag: %s", titles(one))
	}
	if !one[0].IsPreferred || one[0].Kind != "quickfix" || len(one[0].Diagnostics) != 1 || !strings.Contains(titles(one), "type hints") {
		t.Errorf("bad first action %+v", one[0])
	}
	two := buildActions(lines, "python", params(2, d1, d2))
	if !strings.Contains(titles(two), "AI: fix all diagnostics") || len(two) > 14 {
		t.Errorf("two diags: %s", titles(two))
	}
	// comment target covers the comment through the following block
	for _, a := range one {
		if a.Title == "AI: add logging" {
			arg := a.Command.Arguments[0].(actionArgs)
			if arg.TargetStart != 1 || arg.TargetEnd != 2 {
				t.Errorf("instruction target %d-%d", arg.TargetStart, arg.TargetEnd)
			}
		}
	}
}

func TestLineEdit(t *testing.T) {
	lines := []string{"a", "b", "cé"}
	e := lineEdit(lines, 0, 1, "X\n")
	if e.Range.End != (lsp.Position{Line: 2}) || e.NewText != "X\n" {
		t.Errorf("mid: %+v", e)
	}
	e = lineEdit(lines, 2, 2, "Y\n")
	if e.Range.End != (lsp.Position{Line: 2, Character: 2}) || e.NewText != "Y" {
		t.Errorf("last without newline: %+v", e)
	}
}

func TestRelocate(t *testing.T) {
	orig := []string{"x = 1", "y = 2"}
	if i, ok := relocate([]string{"#", "#", "x = 1", "y = 2"}, orig, 0); !ok || i != 2 {
		t.Errorf("shifted: %d %v", i, ok)
	}
	if _, ok := relocate([]string{"x = 1", "y = 3"}, orig, 0); ok {
		t.Error("changed target must not relocate")
	}
}

func rng(sl, sc, el, ec int) lsp.Range {
	return lsp.Range{Start: lsp.Position{Line: sl, Character: sc}, End: lsp.Position{Line: el, Character: ec}}
}

func TestTargetLinesSelectionVsCursor(t *testing.T) {
	lines := strings.Split("def f():\n    a = 1\n    return a\n", "\n")
	if s, e := targetLines(lines, rng(1, 0, 2, 0)); s != 1 || e != 1 { // Helix `x`
		t.Errorf("linewise: %d-%d", s, e)
	}
	if s, e := targetLines(lines, rng(1, 2, 1, 3)); s != 0 || e != 2 { // cursor
		t.Errorf("cursor: %d-%d", s, e)
	}
	if s, e := targetLines(lines, rng(1, 4, 1, 4)); s != 0 || e != 2 {
		t.Errorf("empty: %d-%d", s, e)
	}
	if s, e := targetLines(lines, rng(1, 4, 1, 9)); s != 1 || e != 1 { // in-line selection
		t.Errorf("inline: %d-%d", s, e)
	}
}

func TestTargetLinesHugeBlockContainsCursor(t *testing.T) {
	lines := []string{"def f():"}
	for i := 0; i < 1600; i++ {
		lines = append(lines, "    x = 1")
	}
	s, e := targetLines(lines, rng(1550, 0, 1550, 1))
	if s > 1550 || e < 1550 || e-s+1 > windowLines {
		t.Fatalf("got %d-%d", s, e)
	}
	lines[1549], lines[1551] = "", ""
	if s, e := targetLines(lines, rng(1550, 0, 1550, 1)); s != 1550 || e != 1550 {
		t.Fatalf("paragraph: %d-%d", s, e)
	}
}

func TestBuildActionsInstructionAboveDef(t *testing.T) {
	lines := strings.Split("# ai: add docstring\ndef f():\n    return 1\n", "\n")
	for _, l := range []int{1, 2} {
		if !strings.Contains(titles(buildActions(lines, "python", params(l))), "AI: add docstring") {
			t.Errorf("line %d: missing", l)
		}
	}
}

func captureSvc(t *testing.T) (*lsp.Service, func() string) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	svc := lsp.NewService(lsp.ServerCapabilities{}, &lsp.Logger{}, "t")
	os.Stdout = old
	return svc, func() string {
		w.Close()
		b, _ := io.ReadAll(r)
		return string(b)
	}
}

func TestOpenNewUsesCurrentBufferAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_x.py")
	os.WriteFile(path, []byte("old\n"), 0o600)
	svc, out := captureSvc(t)
	svc.Buffers.Set(&lsp.Buffer{URI: pathURI(path), Text: "# edited\nold\n", Version: 7})
	h := &ActionHandler{}
	if err := h.openNew(svc, path, "new\n", false, "l"); err != nil {
		t.Fatal(err)
	}
	got := out()
	if !strings.Contains(got, `"version":7`) || !strings.Contains(got, `"line":2`) || strings.Contains(got, `"kind":"create"`) {
		t.Fatalf("append edit: %s", got)
	}
	svc, out = captureSvc(t)
	svc.Buffers.Set(&lsp.Buffer{URI: pathURI(path), Text: "# edited\nold\n", Version: 9})
	h.openNew(svc, path, "X\n", true, "l")
	got = out()
	if !strings.Contains(got, `"start":{"line":0,"character":0},"end":{"line":2,"character":0}`) || !strings.Contains(got, `"version":9`) || strings.Contains(got, "overwrite") {
		t.Fatalf("overwrite edit: %s", got)
	}
	svc, out = captureSvc(t)
	h.openNew(svc, filepath.Join(t.TempDir(), "new.md"), "X\n", true, "l")
	if got = out(); !strings.Contains(got, `"kind":"create"`) || !strings.Contains(got, "ignoreIfExists") {
		t.Fatalf("create: %s", got)
	}
}
