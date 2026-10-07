package handlers

import (
	"strings"
	"testing"

	"github.com/leona/helix-assist/internal/lsp"
)

var blk = strings.Split("def f():\n    a = 1\n    b = 2\n    return a + b\n", "\n")

func TestR3CursorOnEmoji(t *testing.T) {
	lines := strings.Split("def f():\n    a = '😀'\n    b = 2\n    return a\n", "\n")
	// Helix block cursor over the emoji = 1 grapheme = 2 UTF-16 units
	r := lsp.Range{Start: lsp.Position{Line: 1, Character: 9}, End: lsp.Position{Line: 1, Character: 11}}
	if s, e := targetLines(lines, r); s != 0 || e != 3 {
		t.Errorf("cursor on emoji: target=(%d,%d), want enclosing block (0,3)", s, e)
	}
}

func TestR3CursorOnLineEnding(t *testing.T) {
	// Helix block cursor sitting on the newline at the end of line 1 / on an empty line
	r := lsp.Range{Start: lsp.Position{Line: 1, Character: 9}, End: lsp.Position{Line: 2, Character: 0}}
	if s, e := targetLines(blk, r); s != 0 || e != 3 {
		t.Errorf("cursor on EOL: target=(%d,%d), want enclosing block (0,3)", s, e)
	}
}

func TestR3RelocateAmbiguous(t *testing.T) {
	lines := strings.Split("a\n    pass\nb\n    pass\nc", "\n")
	if _, ok := relocate(lines, []string{"    pass"}, 0); ok {
		t.Errorf("relocate silently picked one of 2 identical candidates")
	}
}
