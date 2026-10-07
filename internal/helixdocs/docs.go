// Package helixdocs embeds the Helix 25.07.1 user documentation (book/src) so
// hxai can answer keybinding questions offline and from the real docs.
package helixdocs

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.md
var files embed.FS

// Version is the Helix release these docs were taken from.
const Version = "25.07.1"

// All returns every embedded doc concatenated with ===== name ===== headers.
func All() string {
	entries, _ := fs.ReadDir(files, ".")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		data, err := files.ReadFile(n)
		if err != nil {
			continue
		}
		b.WriteString("\n\n===== " + n + " =====\n")
		b.Write(data)
	}
	return b.String()
}
