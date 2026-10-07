package main

import "strings"

// keysModel: Sonnet 5.5. Measured on 5 questions: Haiku (~2s) got 2 wrong (said
// "multiple cursors" was not built-in; wrong surround sequence "v w m s"), Sonnet
// (~2.5s, same ballpark) got all 5 right, so Sonnet is the default. Use --fast to override.
const keysModel = modelDefault

// keyRemaps returns the [keys.*] sections of a Helix config.toml.
func keyRemaps(toml string) string {
	var out []string
	in := false
	for _, l := range strings.Split(toml, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "[") {
			in = strings.HasPrefix(t, "[keys")
		}
		if in && strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// keysSystem builds the tutor prompt: answers come only from the embedded docs.
func keysSystem(docs, remaps string) string {
	s := `You are a Helix editor (version 25.07.1) keybinding tutor. Answer ONLY from the documentation below; never invent bindings.
Output plain text in exactly this format (no markdown, no code fences, lines under 80 columns):
line 1: the key sequence(s), e.g. "mi(  then  c"
then 2-5 short lines explaining it
optionally a last line "Docs: <file>.md"
If the thing is NOT a built-in binding, say so clearly on line 1 and give a config.toml remap snippet (see remapping docs) instead.
Helix is selection-first: select, then act.

=== HELIX DOCS ===
` + docs
	if remaps != "" {
		s += "\n=== USER'S CUSTOM KEY REMAPS (config.toml); mention them when relevant ===\n" + remaps + "\n"
	}
	return s
}
