package assist

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Budgets for cross-file context (bytes). Keeps completion latency in check.
const (
	MaxRelatedFiles = 4
	MaxRelatedBytes = 6000  // per file, head of file
	MaxContextBytes = 20000 // all related files together
)

// RelatedFile is a local source file the current file depends on.
type RelatedFile struct {
	Path    string // relative to the importing file's directory when possible
	Content string
}

var (
	pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+(\.*[\w.]*)\s+import|import\s+([\w.]+))`)
	jsImportRe = regexp.MustCompile(`(?m)(?:from\s+|require\(\s*|import\s*\(?\s*)['"](\.{1,2}/[^'"]+)['"]`)
)

// ProjectRoot is the nearest ancestor of dir containing .git (or dir itself).
func ProjectRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			return dir
		}
		d = p
	}
}

// RelatedFiles returns local files imported by text (the contents of path),
// resolved on disk. Supports Python, JS/TS (relative imports) and Go (sibling
// files of the same package). Third-party and stdlib imports are skipped
// because they don't resolve to files in the project.
func RelatedFiles(path, languageID, text string) []RelatedFile {
	dir := filepath.Dir(path)
	var cands []string
	switch languageID {
	case "python":
		root := ProjectRoot(dir)
		for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
			mod := m[1] + m[2]
			base := dir
			for strings.HasPrefix(mod, ".") { // relative import: one dot = this package
				mod = mod[1:]
				if strings.HasPrefix(mod, ".") {
					base = filepath.Dir(base)
				}
			}
			rel := strings.ReplaceAll(mod, ".", string(filepath.Separator))
			if rel == "" {
				continue
			}
			for _, b := range []string{base, root} {
				cands = append(cands, filepath.Join(b, rel+".py"), filepath.Join(b, rel, "__init__.py"))
			}
		}
	case "javascript", "typescript", "javascriptreact", "typescriptreact", "tsx", "jsx":
		for _, m := range jsImportRe.FindAllStringSubmatch(text, -1) {
			p := filepath.Join(dir, m[1])
			cands = append(cands, p)
			for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", "/index.ts", "/index.js"} {
				cands = append(cands, p+ext)
			}
		}
	case "go":
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		sort.Strings(files)
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") {
				cands = append(cands, f)
			}
		}
	}

	self, _ := filepath.Abs(path)
	seen := map[string]bool{self: true}
	var out []RelatedFile
	total := 0
	for _, c := range cands {
		abs, err := filepath.Abs(c)
		if err != nil || seen[abs] {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() {
			continue
		}
		seen[abs] = true
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		s := string(data)
		if len(s) > MaxRelatedBytes {
			s = strings.ToValidUTF8(s[:MaxRelatedBytes], "") + "\n... (truncated)"
		}
		if total+len(s) > MaxContextBytes {
			break
		}
		total += len(s)
		name, err := filepath.Rel(dir, abs)
		if err != nil {
			name = abs
		}
		out = append(out, RelatedFile{Path: name, Content: s})
		if len(out) == MaxRelatedFiles {
			break
		}
	}
	return out
}

// CompletionContext renders project notes + related files as a prompt
// section, or "" when there is nothing to add.
func CompletionContext(path, languageID, text string) string {
	var b strings.Builder
	if pc := ProjectContext(filepath.Dir(path)); pc != "" {
		b.WriteString("Project notes (.helix-assist.md):\n" + pc + "\n\n")
	}
	for _, f := range RelatedFiles(path, languageID, text) {
		b.WriteString("Related file " + f.Path + " (imported by the current file, for reference only):\n" + f.Content + "\n\n")
	}
	return b.String()
}
