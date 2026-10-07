package assist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRelatedFilesPython(t *testing.T) {
	d := t.TempDir()
	write(t, d+"/.git/HEAD", "")
	write(t, d+"/app/client.py", "def fetch(): pass\n")
	write(t, d+"/app/pkg/__init__.py", "X = 1\n")
	write(t, d+"/util.py", "def helper(): pass\n")
	src := "import os\nfrom client import fetch\nfrom .pkg import X\nimport util\nimport requests\n"
	write(t, d+"/app/main.py", src)
	got := RelatedFiles(d+"/app/main.py", "python", src)
	var names []string
	for _, f := range got {
		names = append(names, f.Path)
	}
	want := "client.py pkg/__init__.py ../util.py"
	if strings.Join(names, " ") != want {
		t.Fatalf("got %v want %s", names, want)
	}
}

func TestRelatedFilesJSAndBudget(t *testing.T) {
	d := t.TempDir()
	write(t, d+"/lib/a.ts", strings.Repeat("x", MaxRelatedBytes+100))
	src := "import { a } from './lib/a'\nimport React from 'react'\n"
	got := RelatedFiles(d+"/index.ts", "typescript", src)
	if len(got) != 1 || got[0].Path != "lib/a.ts" || !strings.HasSuffix(got[0].Content, "(truncated)") {
		t.Fatalf("got %+v", got)
	}
}

func TestCompletionContextNotes(t *testing.T) {
	d := t.TempDir()
	write(t, d+"/.helix-assist.md", "Use Robot373.")
	if c := CompletionContext(d+"/x.py", "python", ""); !strings.Contains(c, "Use Robot373.") {
		t.Fatalf("got %q", c)
	}
	if c := CompletionContext(t.TempDir()+"/y.py", "python", "import os"); c != "" {
		t.Fatalf("want empty, got %q", c)
	}
}
