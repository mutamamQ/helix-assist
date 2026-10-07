package assist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR3DuplicateTestsFalsePositive(t *testing.T) {
	existing := "it('test foo', () => {})\n"
	code := "it('test bar', () => {})\n"
	if d := DuplicateTests(code, existing); d != "" {
		t.Errorf("distinct JS tests flagged as duplicate: %q", d)
	}
	if d := DuplicateTests("it('tests the writer', () => {})\n", "it('tests the parser', () => {})\n"); d != "" {
		t.Errorf("distinct JS tests flagged as duplicate: %q", d)
	}
}

func TestR3CleanReplyRejectsLegitCode(t *testing.T) {
	for _, code := range []string{"sure.expect(x).to.equal(1)", "sorry(user)", "sure, rest = split(x)"} {
		if _, err := CleanReply(code, "    old()"); err != nil {
			t.Errorf("legit code %q rejected: %v", code, err)
		}
	}
}

func TestR3HeaderNoWordBoundary(t *testing.T) {
	lines := strings.Split("def f():\n    default = 1\n    return default\n", "\n")
	if s, e := EnclosingBlock(lines, 1); s != 0 || e != 2 {
		t.Errorf("cursor on `default = 1` inside f: block=(%d,%d), want (0,2)", s, e)
	}
	for _, l := range []string{"    object_id = 3", "module_path = x", "types = {}", "defaults = {"} {
		if IsHeader(l) {
			t.Errorf("IsHeader(%q) = true", l)
		}
	}
}

func TestR3JSImportEscapesRoot(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	os.MkdirAll(filepath.Join(proj, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, "secret.env"), []byte("API_KEY=hunter2"), 0o600)
	src := filepath.Join(proj, "a.js")
	rf := RelatedFiles(src, "javascript", "const s = require('../secret.env')\n")
	if len(rf) != 0 {
		t.Errorf("file outside project root sent as completion context: %v", rf[0].Path)
	}
}
