// Package assist holds the editor-agnostic logic behind helix-assist's AI
// code actions and the hxai CLI: building file context, finding the block
// under the cursor, parsing "ai:" comment instructions and post-processing
// model output.
package assist

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

// MaxContextLines caps how much of a large file is sent around the target.
const MaxContextLines = 1200

// ProjectContext returns the contents of the nearest .helix-assist.md found
// walking up from dir (optional, used to describe project conventions).
func ProjectContext(dir string) string {
	for i := 0; i < 12 && dir != "" && dir != "/"; i++ {
		data, err := os.ReadFile(filepath.Join(dir, ".helix-assist.md"))
		if err == nil {
			return strings.TrimSpace(string(data))
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// FileWithMarkers returns the file text with the target lines (0-based,
// inclusive) wrapped in markers. Large files are windowed around the target.
func FileWithMarkers(lines []string, start, end int) string {
	from, to := 0, len(lines)
	if len(lines) > MaxContextLines {
		half := (MaxContextLines - (end - start)) / 2
		if half < 100 {
			half = 100
		}
		from = max(0, start-half)
		to = min(len(lines), end+1+half)
	}
	var b strings.Builder
	if from > 0 {
		fmt.Fprintf(&b, "... (%d lines above omitted)\n", from)
	}
	for i := from; i < to; i++ {
		if i == start {
			b.WriteString("<<<<<<< TARGET START\n")
		}
		b.WriteString(lines[i])
		b.WriteString("\n")
		if i == end {
			b.WriteString(">>>>>>> TARGET END\n")
		}
	}
	if to < len(lines) {
		fmt.Fprintf(&b, "... (%d lines below omitted)\n", len(lines)-to)
	}
	return b.String()
}

func indentOf(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

var headerRe = regexp.MustCompile(`^\s*(export\s+)?(pub(\([^)]*\))?\s+)?(async\s+)?((def|class|func|function|fn|impl|struct|enum|interface|trait|type|module|object)\b|const\s+\w+\s*=\s*(async\s*)?\(|let\s+\w+\s*=\s*(async\s*)?\(|(public|private|protected|static)\b)`)

// IsHeader reports whether a line looks like the start of a definition.
func IsHeader(s string) bool { return headerRe.MatchString(s) }

// EnclosingBlock finds the function/class-like block containing line (0-based)
// using indentation. Returns the line itself if nothing better is found.
func EnclosingBlock(lines []string, line int) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	line = min(max(line, 0), len(lines)-1)
	cur := line
	for cur < len(lines)-1 && isBlank(lines[cur]) {
		cur++
	}
	header := -1
	if IsHeader(lines[cur]) {
		header = cur
	} else {
		ind := indentOf(lines[cur])
		for i := cur - 1; i >= 0; i-- {
			if isBlank(lines[i]) {
				continue
			}
			if indentOf(lines[i]) < ind {
				if IsHeader(lines[i]) {
					header = i
					break
				}
				ind = indentOf(lines[i])
				if ind == 0 {
					break
				}
			}
		}
	}
	if header == -1 {
		// top-level statement: use the surrounding paragraph
		s, e := cur, cur
		for s > 0 && !isBlank(lines[s-1]) {
			s--
		}
		for e < len(lines)-1 && !isBlank(lines[e+1]) {
			e++
		}
		return s, e
	}
	// include decorators / doc comments directly above the header
	start := header
	for start > 0 {
		p := strings.TrimSpace(lines[start-1])
		if strings.HasPrefix(p, "@") || strings.HasPrefix(p, "#[") || strings.HasPrefix(p, "///") || strings.HasPrefix(p, "//!") {
			start--
			continue
		}
		break
	}
	hind := indentOf(lines[header])
	end := header
	for i := header + 1; i < len(lines); i++ {
		if isBlank(lines[i]) {
			continue
		}
		if indentOf(lines[i]) > hind {
			end = i
			continue
		}
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "}") || strings.HasPrefix(t, ")") || t == "end" || strings.HasPrefix(t, "]") {
			end = i
		}
		break
	}
	return start, end
}

// CommentInstruction is an "ai: ..." instruction written in a code comment.
type CommentInstruction struct {
	Line        int    // 0-based line of the comment
	Instruction string // text after "ai:"
}

var aiCommentRe = regexp.MustCompile(`^\s*(?:#+|//+|--+|;+|/\*+|\*+|<!--|%+)\s*(?i:ai)\s*[:>]\s*(.+?)\s*(?:\*/|-->)?\s*$`)

var commentLineRe = regexp.MustCompile(`^\s*(?:#|//|--|;|/\*|\*|<!--|%)`)

// LeadingInstructions returns "ai:" comments in the contiguous comment lines
// directly above line start (stopping at the first non-comment line).
func LeadingInstructions(lines []string, start int) []CommentInstruction {
	var out []CommentInstruction
	for i := min(start, len(lines)) - 1; i >= 0 && commentLineRe.MatchString(lines[i]); i-- {
		out = append(FindInstructions(lines, i, i), out...)
	}
	return out
}

// FindInstructions returns "ai:" comments within [start,end].
func FindInstructions(lines []string, start, end int) []CommentInstruction {
	var out []CommentInstruction
	for i := max(0, start); i <= end && i < len(lines); i++ {
		if m := aiCommentRe.FindStringSubmatch(lines[i]); m != nil {
			out = append(out, CommentInstruction{Line: i, Instruction: m[1]})
		}
	}
	return out
}

// InstructionTarget returns the lines an "ai:" comment applies to: the comment
// plus the following block (up to the next blank line that is followed by
// code at the comment's indentation or less). A comment followed by nothing
// targets just itself (code is generated in its place).
func InstructionTarget(lines []string, commentLine int) (int, int) {
	ind := indentOf(lines[commentLine])
	end := commentLine
	if commentLine+1 < len(lines) && !isBlank(lines[commentLine+1]) && IsHeader(lines[commentLine+1]) {
		_, e := EnclosingBlock(lines, commentLine+1)
		return commentLine, e
	}
	for i := commentLine + 1; i < len(lines); i++ {
		if isBlank(lines[i]) {
			// stop at a blank line unless the code continues more indented
			j := i + 1
			for j < len(lines) && isBlank(lines[j]) {
				j++
			}
			if j < len(lines) && indentOf(lines[j]) > ind {
				continue
			}
			break
		}
		if indentOf(lines[i]) < ind {
			break
		}
		end = i
	}
	return commentLine, end
}

// FixIndent re-indents model output when the model dropped the target's
// base indentation, and normalises the trailing newline.
func FixIndent(out, original string) string {
	out = strings.TrimRight(out, " \t\n")
	origMin := minIndent(original)
	outMin := minIndent(out)
	if origMin != "" && outMin == "" {
		ls := strings.Split(out, "\n")
		for i, l := range ls {
			if !isBlank(l) {
				ls[i] = origMin + l
			}
		}
		out = strings.Join(ls, "\n")
	}
	return out + "\n"
}

func minIndent(text string) string {
	best := ""
	first := true
	for _, l := range strings.Split(text, "\n") {
		if isBlank(l) {
			continue
		}
		ind := l[:indentOf(l)]
		if first || len(ind) < len(best) {
			best, first = ind, false
		}
	}
	return best
}

// UTF16Len is the LSP default (UTF-16) length of s.
func UTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// TestPath picks where tests for a source file should live.
func TestPath(path, languageID string) string {
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	switch languageID {
	case "python":
		return filepath.Join(dir, "test_"+name+ext)
	case "go":
		return filepath.Join(dir, name+"_test.go")
	case "javascript", "typescript", "javascriptreact", "typescriptreact", "tsx", "jsx":
		return filepath.Join(dir, name+".test"+ext)
	case "rust":
		return filepath.Join(dir, "tests_"+name+ext)
	default:
		return filepath.Join(dir, name+"_test"+ext)
	}
}

// CleanReply turns a model reply into replacement code for original, or
// returns an error when the reply isn't usable (empty, prose, refusal).
// A ``` fence is only stripped when it wraps the entire reply, so Markdown
// and doc comments that legitimately contain fences survive.
func CleanReply(reply, original string) (string, error) {
	t := strings.TrimRight(strings.TrimLeft(reply, "\r\n"), " \t\r\n") // keep first-line indent
	if strings.HasPrefix(strings.TrimSpace(t), "```") && strings.HasSuffix(t, "```") && strings.HasSuffix(t, "```") && len(t) > 6 && !strings.HasPrefix(strings.TrimSpace(original), "```") {
		if nl := strings.Index(t, "\n"); nl != -1 && strings.HasPrefix(strings.TrimSpace(t), "```") {
			t = strings.TrimSuffix(t[nl+1:], "```")
		} else {
			t = ""
		}
	} else if i := strings.Index(t, "\n```"); i > 0 && i < 300 && proseRe.MatchString(t) && !strings.Contains(original, "```") {
		// "Here is the code:\n```lang\n...\n```": keep the block
		rest := t[i+1:]
		if nl := strings.Index(rest, "\n"); nl != -1 {
			rest = rest[nl+1:]
			if j := strings.LastIndex(rest, "```"); j != -1 {
				rest = rest[:j]
			}
			t = rest
		}
	}
	t = strings.TrimRight(t, " \t\r\n")
	if strings.TrimSpace(t) == "" {
		return "", fmt.Errorf("model returned no code")
	}
	if proseRe.MatchString(t) && !proseRe.MatchString(original) {
		return "", fmt.Errorf("model replied with prose instead of code: %.60q", t)
	}
	return t, nil
}

// Openers only count as prose when followed by prose-ish text, so code like
// sure.expect(x), sorry(user) or "sure, rest = split(x)" is not rejected.
var proseRe = regexp.MustCompile(`^\s*(?i:sorry(?:[,.!]|\s+(?:i|but|for|about|that|,))|i can(?:no|')?t\s|i'm (?:sorry|unable)\b|i am (?:sorry|unable)\b|as an ai\b|unfortunately[,\s]|here(?: is|'s| you go| are)[\s:,!.]|(?:sure|certainly|of course)(?:[!.](?:\s|$)|,\s+(?:here|i|this|that|the|let)\b))`)

var testNameRe = regexp.MustCompile(`(?m)^\s*(?:async\s+)?(?:def|func|fn|function)\s+(Test\w*|test\w*)`)

// testDescRe matches JS/TS it/test/describe calls with a quoted description.
var testDescRe = regexp.MustCompile("(?m)^\\s*(?:it|test|describe)(?:\\.\\w+)?\\s*\\(\\s*(?:'([^']+)'|\"([^\"]+)\"|`([^`]+)`)")

// DuplicateTests returns the first test name or description in code that
// already exists in existing (so appending would duplicate it), or "".
func DuplicateTests(code, existing string) string {
	have := map[string]bool{}
	for _, re := range []*regexp.Regexp{testNameRe, testDescRe} {
		for _, m := range re.FindAllStringSubmatch(existing, -1) {
			have[firstNonEmpty(m[1:])] = true
		}
	}
	for _, re := range []*regexp.Regexp{testNameRe, testDescRe} {
		for _, m := range re.FindAllStringSubmatch(code, -1) {
			if n := firstNonEmpty(m[1:]); have[n] {
				return n
			}
		}
	}
	return ""
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
