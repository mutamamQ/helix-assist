package main

// Macros ("@...") replay keys into a command prompt left open for the user to finish (no trailing "ret").
// Use ":pipe"/":insert-output", not the "|"/"!" prompts: only typable commands expand %{...} (verified in Helix 25.07.1).
// Shell commands in Helix don't interpret quotes, and hxai joins its words, so the
// instruction needs no quoting.
const docSnippets = `# Paste into ~/.config/helix/config.toml  (space+i = "AI"; i is free in 25.07.1)
# Type the instruction after the prefilled prompt, then Enter. Esc cancels.
[keys.normal.space.i]
e = "@:pipe hxai edit --file %{buffer_name} "   # rewrite selection (select first)
g = "@:insert-output hxai gen --file %{buffer_name} --line %{cursor_line} "  # insert generated code
a = "@:sh hxai ask --file %{buffer_name} --line %{cursor_line} "  # answer in a popup
k = "@:sh hxai keys "                       # keybinding tutor
c = ":insert-output hxai commit"            # commit message from git diff (run in a commit buffer)

[keys.select.space.i]
e = "@:pipe hxai edit --file %{buffer_name} "
`
