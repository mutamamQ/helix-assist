package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leona/helix-assist/internal/assist"
	"github.com/leona/helix-assist/internal/config"
	"github.com/leona/helix-assist/internal/lsp"
	"github.com/leona/helix-assist/internal/providers"
	"github.com/leona/helix-assist/internal/util"
)

// spec describes one AI action. Menu order follows the slice order.
type spec struct {
	key, title, kind, task string
	langs                  []string // nil = any language
	deep                   bool
}

var specs = []spec{
	{key: "improve", title: "AI: improve code", kind: "refactor.rewrite", task: "Improve this code: readability, correctness, idiomatic style. Keep behaviour and public interfaces."},
	{key: "refactor", title: "AI: refactor", kind: "refactor.rewrite", task: "Refactor this code for clarity and maintainability without changing behaviour."},
	{key: "refactor", title: "AI: refactor (deep)", kind: "refactor.rewrite", task: "Refactor this code for clarity and maintainability without changing behaviour. Think carefully about structure.", deep: true},
	{key: "docs", title: "AI: add docs/comments", kind: "refactor.rewrite", task: "Add docstrings and helpful comments where missing. Do not change the code logic."},
	{key: "types", title: "AI: add type hints", kind: "refactor.rewrite", task: "Add type hints / type annotations (JSDoc for plain JavaScript). Do not change behaviour.", langs: []string{"python", "typescript", "javascript", "tsx", "typescriptreact", "javascriptreact"}},
	{key: "errors", title: "AI: add error handling", kind: "refactor.rewrite", task: "Add proper error handling and input validation. Keep the existing style."},
	{key: "optimize", title: "AI: optimize", kind: "refactor.rewrite", task: "Optimize this code for performance without changing behaviour."},
	{key: "simplify", title: "AI: simplify", kind: "refactor.rewrite", task: "Simplify this code: remove redundancy and needless complexity without changing behaviour."},
	{key: "explain", title: "AI: explain", kind: "source"},
	{key: "tests", title: "AI: write tests", kind: "source"},
}

// Internal keys for the dynamic entries.
const (
	keyFix         = "fix"
	keyFixAll      = "fixAll"
	keyInstruction = "instruction"
	cmdPrefix      = "ai."
	maxDiagEntries = 5
)

// CommandKeys lists every executeCommand name the server accepts.
func CommandKeys() []string {
	keys := []string{cmdPrefix + keyFix, cmdPrefix + keyFixAll, cmdPrefix + keyInstruction}
	for _, s := range specs {
		if k := cmdPrefix + s.key; !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

type diagArg struct {
	Message string    `json:"message"`
	Range   lsp.Range `json:"range"`
}

// actionArgs is the JSON argument carried by every command.
type actionArgs struct {
	URI         string    `json:"uri"`
	Range       lsp.Range `json:"range"`
	Kind        string    `json:"kind"`
	Deep        bool      `json:"deep,omitempty"`
	Instruction string    `json:"instruction,omitempty"`
	Diagnostics []diagArg `json:"diagnostics,omitempty"`
	TargetStart int       `json:"targetStart,omitempty"` // instruction actions only
	TargetEnd   int       `json:"targetEnd,omitempty"`
}

type ActionHandler struct {
	cfg      *config.Config
	registry *providers.Registry
	inflight sync.Map // uri|kind|line -> struct{}: same action twice would double-apply
}

func NewActionHandler(cfg *config.Config, registry *providers.Registry) *ActionHandler {
	return &ActionHandler{cfg: cfg, registry: registry}
}

func (h *ActionHandler) Register(svc *lsp.Service) {
	svc.On(lsp.EventCodeAction, func(svc *lsp.Service, msg *lsp.JSONRPCMessage) {
		var params lsp.CodeActionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			svc.Logger.Log("codeAction parse error:", err.Error())
			return
		}
		svc.Buffers.SetCurrentURI(params.TextDocument.URI)
		actions := []lsp.CodeAction{}
		if buf, ok := svc.Buffers.Get(params.TextDocument.URI); ok {
			actions = buildActions(strings.Split(buf.Text, "\n"), buf.LanguageID, params)
		}
		svc.Send(&lsp.JSONRPCMessage{ID: msg.ID, Result: actions})
	})

	svc.On(lsp.EventExecuteCommand, func(svc *lsp.Service, msg *lsp.JSONRPCMessage) {
		h.executeCommand(svc, msg)
	})
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func command(title, key string, a actionArgs) *lsp.Command {
	return &lsp.Command{Title: title, Command: cmdPrefix + key, Arguments: []any{a}}
}

const maxBlockLines, windowLines = 400, 200

// targetLines returns the 0-based inclusive lines an action applies to: the
// selected lines, or the enclosing block for a bare cursor (range of at most
// one character on one line). The result always contains the cursor line.
func targetLines(lines []string, r lsp.Range) (int, int) {
	last := max(len(lines)-1, 0)
	s, e := min(r.Start.Line, last), min(r.End.Line, last)
	if r.Start.Line != r.End.Line || r.End.Character-r.Start.Character > 1 {
		if r.End.Character == 0 && e > s {
			e-- // linewise selection ends at column 0 of the next line
		}
		return s, e
	}
	bs, be := assist.EnclosingBlock(lines, s)
	if be-bs+1 <= maxBlockLines || bs > s || be < s {
		return bs, be
	}
	// huge block: window of the cursor's paragraph, at most windowLines long
	ps, pe := s, s
	for ps > bs && strings.TrimSpace(lines[ps-1]) != "" {
		ps--
	}
	for pe < be && strings.TrimSpace(lines[pe+1]) != "" {
		pe++
	}
	ws := max(ps, s-windowLines/2)
	we := min(pe, ws+windowLines-1)
	return max(ps, we-windowLines+1), we
}

// buildActions makes the context-aware menu for the cursor/selection.
func buildActions(lines []string, lang string, p lsp.CodeActionParams) []lsp.CodeAction {
	uri := p.TextDocument.URI
	base := actionArgs{URI: uri, Range: p.Range}
	diags := make([]diagArg, len(p.Context.Diagnostics))
	for i, d := range p.Context.Diagnostics {
		diags[i] = diagArg{d.Message, d.Range}
	}
	var out []lsp.CodeAction

	for i, d := range p.Context.Diagnostics {
		if i == maxDiagEntries {
			break // ponytail: the rest are covered by "fix all"
		}
		a := base
		a.Kind, a.Diagnostics = keyFix, diags[i:i+1]
		t := "AI fix: " + truncate(d.Message, 60)
		out = append(out, lsp.CodeAction{Title: t, Kind: "quickfix", IsPreferred: true, Diagnostics: []any{d}, Command: command(t, keyFix, a)})
	}
	if len(diags) >= 2 {
		a := base
		a.Kind, a.Diagnostics = keyFixAll, diags
		all := make([]any, len(p.Context.Diagnostics))
		for i, d := range p.Context.Diagnostics {
			all[i] = d
		}
		t := "AI: fix all diagnostics"
		out = append(out, lsp.CodeAction{Title: t, Kind: "quickfix", Diagnostics: all, Command: command(t, keyFixAll, a)})
	}

	if len(lines) > 0 {
		s, e := targetLines(lines, p.Range)
		found := assist.FindInstructions(lines, s, e)
		cur := min(p.Range.Start.Line, len(lines)-1)
		found = append(found, assist.LeadingInstructions(lines, s)...)
		for _, c := range assist.FindInstructions(lines, cur, cur) {
			if !slices.ContainsFunc(found, func(f assist.CommentInstruction) bool { return f.Line == c.Line }) {
				found = append(found, c)
			}
		}
		for _, c := range found {
			a := base
			a.Kind, a.Instruction = keyInstruction, c.Instruction
			a.TargetStart, a.TargetEnd = assist.InstructionTarget(lines, c.Line)
			t := "AI: " + truncate(c.Instruction, 60)
			out = append(out, lsp.CodeAction{Title: t, Kind: "quickfix", IsPreferred: true, Command: command(t, keyInstruction, a)})
		}
	}

	for _, sp := range specs {
		if sp.langs != nil && !slices.Contains(sp.langs, lang) {
			continue
		}
		a := base
		a.Kind, a.Deep = sp.key, sp.deep
		out = append(out, lsp.CodeAction{Title: sp.title, Kind: sp.kind, Command: command(sp.title, sp.key, a)})
	}
	return out
}

// lineEdit replaces whole lines start..end (inclusive) with text. text should
// end in "\n"; the last line of a file without trailing newline is handled.
func lineEdit(lines []string, start, end int, text string) lsp.TextEdit {
	if end+1 < len(lines) {
		return lsp.TextEdit{Range: lsp.Range{Start: lsp.Position{Line: start}, End: lsp.Position{Line: end + 1}}, NewText: text}
	}
	return lsp.TextEdit{
		Range:   lsp.Range{Start: lsp.Position{Line: start}, End: lsp.Position{Line: end, Character: assist.UTF16Len(lines[end])}},
		NewText: strings.TrimSuffix(text, "\n"),
	}
}

func indentLen(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

// relocate finds orig in lines (exact match), preferring the match closest to hint.
func relocate(lines, orig []string, hint int) (int, bool) {
	best := -1
	for i := 0; i+len(orig) <= len(lines); i++ {
		if slices.Equal(lines[i:i+len(orig)], orig) && (best < 0 || abs(i-hint) < abs(best-hint)) {
			best = i
		}
	}
	return best, best >= 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func stripCR(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSuffix(l, "\r")
	}
	return out
}

func uriPath(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Path != "" {
		return u.Path
	}
	return strings.TrimPrefix(uri, "file://")
}

func pathURI(p string) string { return (&url.URL{Scheme: "file", Path: p}).String() }

const editSystem = `You are a precise code-editing engine inside a text editor. You receive a whole file in which the TARGET region is delimited by lines "<<<<<<< TARGET START" and ">>>>>>> TARGET END". Reply with ONLY the new text that replaces the lines between the markers: no markers, no code fences, no explanations. Keep the original indentation and style, and make the replacement fit the surrounding code. Never output code outside the target.`

const explainSystem = `You are a senior engineer explaining code inside a text editor. You receive a whole file; the TARGET region is delimited by "<<<<<<< TARGET START" / ">>>>>>> TARGET END". Explain the target clearly and concisely in Markdown (what it does, how, and notable pitfalls), using the rest of the file only as context.`

const testsSystem = `You are a senior engineer writing tests. You receive a source file (the TARGET region marks the code to focus on) and possibly the existing test file. Reply with ONLY test code, no code fences, no explanations. If a test file already exists, output ONLY new tests to append to it (no duplicate imports or existing tests); otherwise output a complete test file. Use the project's idiomatic test framework.`

func (h *ActionHandler) ask(ctx context.Context, deep bool, system, user, uri, lang string) (string, error) {
	model := ""
	if deep {
		model = h.cfg.BryantModelDeep
	}
	if !h.registry.SupportsRaw() {
		r, err := h.registry.Chat(ctx, system+"\n\n"+user, "", uri, lang)
		if err != nil {
			return "", err
		}
		return r.Result, nil
	}
	return h.registry.Raw(ctx, model, system, user, 8192)
}

func (h *ActionHandler) executeCommand(svc *lsp.Service, msg *lsp.JSONRPCMessage) {
	defer func() {
		if r := recover(); r != nil {
			svc.Logger.Log("executeCommand panic:", r)
		}
	}()

	var params lsp.ExecuteCommandParams
	var a actionArgs
	if err := json.Unmarshal(msg.Params, &params); err != nil || len(params.Arguments) == 0 {
		svc.Logger.Log("executeCommand: bad params")
		svc.Reply(msg.ID, nil)
		return
	}
	raw, _ := json.Marshal(params.Arguments[0])
	if err := json.Unmarshal(raw, &a); err != nil {
		svc.Logger.Log("executeCommand: bad argument:", err.Error())
		svc.Reply(msg.ID, nil)
		return
	}
	svc.Reply(msg.ID, nil) // Helix doesn't wait for the edit; it comes as workspace/applyEdit

	if err := h.run(svc, a, params.Command); err != nil {
		svc.Logger.Log("action failed:", err.Error())
		svc.SendShowMessage(lsp.MessageTypeError, "helix-assist: "+err.Error())
	}
}

func (h *ActionHandler) run(svc *lsp.Service, a actionArgs, cmd string) error {
	buf, ok := svc.Buffers.Get(a.URI)
	if !ok {
		return fmt.Errorf("buffer not found")
	}
	text, version, lang := buf.Text, buf.Version, buf.LanguageID
	lines := strings.Split(text, "\n")
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	s, e := targetLines(lines, a.Range)
	switch a.Kind {
	case keyInstruction:
		s, e = min(a.TargetStart, len(lines)-1), min(a.TargetEnd, len(lines)-1)
	case keyFix, keyFixAll:
		for _, d := range a.Diagnostics {
			if l := min(d.Range.Start.Line, len(lines)-1); l < s || l > e {
				bs, be := assist.EnclosingBlock(lines, l)
				s, e = min(s, bs), max(e, be)
			}
		}
	}
	key := fmt.Sprintf("%s|%s|%s|%d", a.URI, a.Kind, a.Instruction, s)
	if _, busy := h.inflight.LoadOrStore(key, struct{}{}); busy {
		return fmt.Errorf("that action is already running")
	}
	defer h.inflight.Delete(key)
	orig := lines[s : e+1]
	clean := stripCR(lines) // prompt/model see LF-only text
	origText := strings.Join(clean[s:e+1], "\n")

	var task, system string
	var deep = a.Deep
	switch a.Kind {
	case keyFix, keyFixAll:
		var b strings.Builder
		b.WriteString("Fix the following diagnostics in the target code:\n")
		for _, d := range a.Diagnostics {
			fmt.Fprintf(&b, "- line %d: %s\n", d.Range.Start.Line+1, d.Message)
		}
		task, system = b.String(), editSystem
	case keyInstruction:
		task = "Apply this instruction found in a comment inside the target: \"" + a.Instruction + "\"\nREMOVE the instruction comment line itself from your output. If the target holds only the comment, write the code it asks for in its place."
		system = editSystem
	case "explain":
		system = explainSystem
	case "tests":
		system = testsSystem
	default:
		i := slices.IndexFunc(specs, func(sp spec) bool { return sp.key == a.Kind && sp.deep == a.Deep })
		if i < 0 {
			return fmt.Errorf("unknown action %q", cmd)
		}
		task, system = specs[i].task, editSystem
	}

	path := uriPath(a.URI)
	var u strings.Builder
	fmt.Fprintf(&u, "File: %s\nLanguage: %s\n", path, lang)
	if pc := assist.ProjectContext(filepath.Dir(path)); pc != "" {
		fmt.Fprintf(&u, "\nProject context:\n%s\n", pc)
	}
	if task != "" {
		fmt.Fprintf(&u, "\nTask: %s\n", task)
	}
	var testPath, testText string
	if a.Kind == "tests" {
		testPath = assist.TestPath(path, lang)
		testText, _, _ = currentText(svc, testPath)
		if testText != "" {
			fmt.Fprintf(&u, "\nTask: write tests for the target code. Existing test file %s (append new tests only):\n%s\n", testPath, testText)
		} else {
			fmt.Fprintf(&u, "\nTask: write tests for the target code. Test file %s does not exist yet; output the complete file.\n", testPath)
		}
	}
	fmt.Fprintf(&u, "\nFull file with target:\n%s", assist.FileWithMarkers(clean, s, e))

	var progress *util.ProgressIndicator
	if h.cfg.EnableProgressSpinner {
		progress = util.NewProgressIndicator(svc, h.cfg)
		progress.Start()
		defer progress.Stop()
	} else {
		svc.SendShowMessage(lsp.MessageTypeInfo, "Executing "+cmd+"...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.ActionTimeout)*time.Millisecond)
	defer cancel()
	resp, err := h.ask(ctx, deep, system, u.String(), a.URI, lang)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resp) == "" {
		return fmt.Errorf("model returned nothing")
	}

	switch a.Kind {
	case "explain":
		dir, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(path))
		name := fmt.Sprintf("explain-%x-%s.md", sum[:4], filepath.Base(path)) // unique per source path
		return h.openNew(svc, filepath.Join(dir, "helix-assist", name), resp+"\n", true, cmd)
	case "tests":
		code, err := assist.CleanReply(resp, "")
		if err != nil {
			return err
		}
		// the destination may have been edited while the model ran: use its current text
		cur, _, _ := currentText(svc, testPath)
		if dup := assist.DuplicateTests(code, cur); dup != "" {
			return fmt.Errorf("model re-wrote existing test %s; nothing appended", dup)
		}
		if cur != "" {
			code = "\n" + strings.TrimRight(code, "\n") + "\n"
			if !strings.HasSuffix(cur, "\n") {
				code = "\n" + code
			}
		} else {
			code = strings.TrimRight(code, "\n") + "\n"
		}
		return h.openNew(svc, testPath, code, false, cmd)
	}

	// stale-edit guard
	if cur, ok := svc.Buffers.Get(a.URI); ok && cur.Version != version {
		version = cur.Version
		lines = strings.Split(cur.Text, "\n")
		ns, found := relocate(lines, orig, s)
		if !found {
			return fmt.Errorf("buffer changed while the model was running; edit discarded")
		}
		s, e = ns, ns+len(orig)-1
	}
	code, err := assist.CleanReply(resp, origText)
	if err != nil {
		return err
	}
	out := strings.ReplaceAll(assist.FixIndent(code, origText), "\n", eol)
	edit := lineEdit(lines, s, e, out)
	if eol == "\r\n" && e+1 >= len(lines) {
		edit.NewText = strings.TrimSuffix(out, eol)
	}
	svc.SendRequest(lsp.EventApplyEdit, lsp.ApplyWorkspaceEditParams{
		Label: cmd,
		// version lets Helix itself reject the edit if the buffer moved on since we read it
		Edit: lsp.WorkspaceEdit{DocumentChanges: []any{lsp.TextDocumentEdit{TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: a.URI, Version: &version}, Edits: []lsp.TextEdit{edit}}}},
	})
	return nil
}

// endOf returns the position just after the last character of text.
func endOf(text string) lsp.Position {
	ls := strings.Split(text, "\n")
	return lsp.Position{Line: len(ls) - 1, Character: assist.UTF16Len(ls[len(ls)-1])}
}

// currentText returns the text of path: the open buffer (with its version)
// if there is one, else the file on disk (ok reports whether it exists).
func currentText(svc *lsp.Service, path string) (text string, version *int, ok bool) {
	if b, found := svc.Buffers.Get(pathURI(path)); found {
		v := b.Version
		return b.Text, &v, true
	}
	data, err := os.ReadFile(path)
	return string(data), nil, err == nil
}

// openNew writes content to path through workspace/applyEdit and shows it in
// a split. overwrite replaces the whole current document, otherwise content is
// appended. Edits are positioned against the document's current text and
// carry the open buffer's version so Helix rejects them if it moved on.
func (h *ActionHandler) openNew(svc *lsp.Service, path, content string, overwrite bool, label string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	uri := pathURI(path)
	cur, ver, exists := currentText(svc, path)
	rng := lsp.Range{Start: endOf(cur), End: endOf(cur)}
	if overwrite {
		rng.Start = lsp.Position{}
	}
	changes := []any{}
	if !exists {
		changes = append(changes, lsp.CreateFile{Kind: "create", URI: uri, Options: &lsp.CreateFileOptions{IgnoreIfExists: true}})
	}
	changes = append(changes, lsp.TextDocumentEdit{TextDocument: lsp.OptionalVersionedTextDocumentIdentifier{URI: uri, Version: ver}, Edits: []lsp.TextEdit{{Range: rng, NewText: content}}})
	svc.SendRequest(lsp.EventApplyEdit, lsp.ApplyWorkspaceEditParams{Label: label, Edit: lsp.WorkspaceEdit{DocumentChanges: changes}})
	svc.SendRequest(lsp.EventShowDocument, lsp.ShowDocumentParams{URI: uri})
	return nil
}
