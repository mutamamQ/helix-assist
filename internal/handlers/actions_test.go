package handlers

import (
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

func TestShiftIndent(t *testing.T) {
	if got := shiftIndent("   a\n     b", "    x"); got != "    a\n      b" {
		t.Errorf("%q", got)
	}
	if got := shiftIndent("a\n  b", "x"); got != "a\n  b" {
		t.Errorf("%q", got)
	}
	if got := shiftIndent("  a\nb", "    x"); got != "  a\nb" { // first line not the shallowest
		t.Errorf("%q", got)
	}
}
