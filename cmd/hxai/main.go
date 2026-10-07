// hxai: AI helpers for Helix shell commands (:pipe, :insert-output, :sh).
//
// Helix 25.07.1 inserts stderr into the buffer for :pipe/:insert-output, and
// aborts the edit on a non-zero exit with empty stderr. So: result on stdout,
// errors logged to ~/.cache/hxai.log + exit 1 (stderr only on a terminal).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/leona/helix-assist/internal/assist"
	"github.com/leona/helix-assist/internal/config"
	"github.com/leona/helix-assist/internal/helixdocs"
	"github.com/leona/helix-assist/internal/lsp"
	"github.com/leona/helix-assist/internal/providers"
)

const (
	modelDefault = "15d8cc8844/CLAUDE_V5_5_SONNET"
	modelDeep    = "15d8cc8844/CLAUDE_V5_5_OPUS"
	modelFast    = "15d8cc8844/CLAUDE_V4_5_HAIKU"
	maxDiff      = 60000
)

const usage = `hxai - AI helpers for Helix (use from :pipe, :insert-output, :sh)

  hxai edit <instruction> [--file P] [--lang L]   stdin selection -> rewritten code
  hxai gen  <instruction> [--file P] [--line N]   new code to insert at cursor line
  hxai ask  <question>    [--file P] [--line N]   short plain-text answer (stdin = selection)
  hxai commit                                     commit message from git diff
  hxai keys [question]                            Helix keybinding tutor (from the Helix docs)
  hxai doc                                        config.toml keybinding snippets

Instruction words need no quotes. Flags: --deep (Opus), --fast (Haiku).
Env: BRYANT_API_KEY, BRYANT_ENDPOINT, HXAI_MODEL.
`

type opts struct {
	file, lang string
	line       int
	model      string
	text       string // positional words joined
}

// parseArgs accepts flags anywhere; all other words are joined into the instruction.
func parseArgs(args []string) (o opts, err error) {
	o.model = os.Getenv("HXAI_MODEL")
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(a, "=")
		next := func() string {
			if hasVal {
				return val
			}
			if i+1 >= len(args) {
				err = fmt.Errorf("%s needs a value", name)
				return ""
			}
			i++
			return args[i]
		}
		switch name {
		case "--file":
			o.file = next()
			// Helix doesn't quote %{buffer_name}: glue following words while that names a real file.
			for j := i + 1; j < len(args) && err == nil; j++ {
				if _, e := os.Stat(o.file); e == nil {
					break
				}
				if _, e := os.Stat(o.file + " " + args[j]); e == nil {
					o.file += " " + args[j]
					i = j
				}
			}
		case "--lang":
			o.lang = next()
		case "--line":
			n, e := strconv.Atoi(next())
			if e != nil && err == nil {
				err = fmt.Errorf("--line needs a number")
			}
			o.line = n
		case "--deep":
			o.model = modelDeep
		case "--fast":
			o.model = modelFast
		default:
			if strings.HasPrefix(a, "--") {
				return o, fmt.Errorf("unknown flag %s", a)
			}
			words = append(words, a)
		}
		if err != nil {
			return
		}
	}
	o.text = strings.TrimSpace(strings.Join(words, " "))
	return
}

// finish validates model output and restores the input's indent/newline style.
func finish(out, in string) (string, error) {
	code, err := assist.CleanReply(out, in)
	if err != nil {
		return "", err
	}
	code = assist.FixIndent(code, in)
	if !strings.HasSuffix(in, "\n") {
		code = strings.TrimSuffix(code, "\n")
	}
	return code, nil
}

func readFile(path string) (string, []string) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	s := string(b)
	return s, strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func stdinText() string {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	b, _ := io.ReadAll(os.Stdin)
	return string(b)
}

func ask(model, system, user string, max int) (string, error) {
	key := os.Getenv("BRYANT_API_KEY")
	if key == "" {
		key = config.ReadHermesEnvKey("BRYANT_API_KEY")
	}
	if key == "" {
		return "", fmt.Errorf("BRYANT_API_KEY not set")
	}
	ep := os.Getenv("BRYANT_ENDPOINT")
	if ep == "" {
		ep = "http://127.0.0.1:8765/v1"
	}
	if model == "" {
		model = modelDefault
	}
	p := providers.NewBryantProvider(key, model, model, ep, 120000, lsp.NewLogger(""))
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	return p.Raw(ctx, model, system, user, max)
}

// context builds the shared "file + project conventions" prompt section.
func fileContext(o opts, lines []string, start, end int) string {
	var b strings.Builder
	if pc := assist.ProjectContext(filepath.Dir(absOr(o.file))); pc != "" {
		b.WriteString("Project conventions:\n" + pc + "\n\n")
	}
	if lines != nil {
		note := ""
		if start >= 0 {
			note = " (the cursor line is wrapped in <<<<<<< TARGET START / >>>>>>> TARGET END markers that are NOT part of the file)"
		}
		fmt.Fprintf(&b, "File %s%s:\n%s\n", o.file, note, assist.FileWithMarkers(lines, start, end))
	}
	return b.String()
}

func absOr(p string) string {
	if p == "" {
		p = "."
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func gitDiff() (string, error) {
	for _, args := range [][]string{{"diff", "--staged"}, {"diff"}} {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			return "", fmt.Errorf("git %s failed", args[0])
		}
		if strings.TrimSpace(string(out)) != "" {
			return string(out), nil
		}
	}
	return "", fmt.Errorf("no changes to describe")
}

func run(args []string) (string, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		return usage, nil
	}
	cmd := args[0]
	if cmd == "doc" {
		return docSnippets, nil
	}
	o, err := parseArgs(args[1:])
	if err != nil {
		return "", err
	}
	_, lines := readFile(o.file)
	switch cmd {
	case "edit":
		if o.text == "" {
			return "", fmt.Errorf("edit needs an instruction")
		}
		in := stdinText()
		if strings.TrimSpace(in) == "" {
			return "", fmt.Errorf("nothing selected")
		}
		user := fmt.Sprintf("Instruction: %s\n\n%s", o.text, fileContext(o, lines, -1, -1))
		if o.lang != "" {
			user += "Language: " + o.lang + "\n\n"
		}
		user += "Selected code to rewrite:\n" + in
		out, err := ask(o.model, "You rewrite code per the instruction. Reply with ONLY the rewritten replacement for the selected code: no markdown fences, no commentary. Keep the original indentation and style.", user, 8000)
		if err != nil {
			return "", err
		}
		return finish(out, in)
	case "gen":
		if o.text == "" {
			return "", fmt.Errorf("gen needs an instruction")
		}
		cur := max(o.line-1, 0)
		base := ""
		if cur < len(lines) {
			base = lines[cur]
		}
		user := fmt.Sprintf("Instruction: %s\n\n%s", o.text, fileContext(o, lines, cur, cur))
		out, err := ask(o.model, "You write NEW code that will be inserted on new lines directly AFTER the marked line. Reply with ONLY the new code, indented correctly for that position: no markdown fences, no commentary, never repeat the marked line or other existing code.", user, 8000)
		if err != nil {
			return "", err
		}
		return finish(out, base+"\n")
	case "ask":
		if o.text == "" {
			return "", fmt.Errorf("ask needs a question")
		}
		user := "Question: " + o.text + "\n\n" + fileContext(o, lines, -1, -1)
		if o.line > 0 {
			user += fmt.Sprintf("Cursor is on line %d.\n\n", o.line)
		}
		if sel := stdinText(); strings.TrimSpace(sel) != "" {
			user += "Selected code:\n" + sel
		}
		out, err := ask(o.model, "Answer briefly (under ~10 lines) in plain text. Wrap lines at 80 columns. No markdown: no headings, bold, or code fences; indent code with 2 spaces.", user, 2000)
		if err != nil {
			return "", err
		}
		return tidyAnswer(out), nil
	case "commit":
		d, err := gitDiff()
		if err != nil {
			return "", err
		}
		if len(d) > maxDiff {
			d = strings.ToValidUTF8(d[:maxDiff], "") + "\n... (diff truncated)"
		}
		out, err := ask(o.model, "Write a git commit message for the diff: imperative subject under 72 chars, blank line, short body only if needed. Reply with ONLY the message, no fences.", d, 600)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(providers.CleanCodeOutput(out)) + "\n", nil
	case "keys":
		q := o.text
		if q == "" {
			q = strings.TrimSpace(stdinText())
		}
		if q == "" {
			return "", fmt.Errorf("keys needs a question")
		}
		model := o.model
		if model == "" {
			model = keysModel
		}
		cfg, err := os.UserConfigDir() // honours XDG_CONFIG_HOME
		if err != nil {
			return "", err
		}
		b, _ := os.ReadFile(filepath.Join(cfg, "helix", "config.toml"))
		out, err := ask(model, keysSystem(helixdocs.All(), keyRemaps(string(b))), q, 800)
		if err != nil {
			return "", err
		}
		return tidyAnswer(out), nil
	}
	return "", fmt.Errorf("unknown command %q (try --help)", cmd)
}

// tidyAnswer drops fence lines (the :sh popup is already a ```sh block).
func tidyAnswer(s string) string {
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "```") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n") + "\n"
}

// popupCmds print to the :sh popup, so errors can go to stdout there.
var popupCmds = map[string]bool{"ask": true, "keys": true, "doc": true}

func main() {
	out, err := run(os.Args[1:])
	if err == nil {
		fmt.Print(out)
		return
	}
	msg := "hxai: " + strings.Join(strings.Fields(err.Error()), " ")
	if len(os.Args) > 1 && popupCmds[os.Args[1]] {
		fmt.Println(msg)
		return
	}
	// Helix 25.07.1 :pipe/:insert-output insert stderr INTO the buffer, but a
	// non-zero exit with empty stderr aborts the edit. So: log, keep stderr empty.
	if dir, e := os.UserCacheDir(); e == nil {
		if f, e := os.OpenFile(filepath.Join(dir, "hxai.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); e == nil {
			fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), msg)
			f.Close()
		}
	}
	if !isTerminal(os.Stderr) {
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, msg) // interactive use: show it
	os.Exit(1)
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
