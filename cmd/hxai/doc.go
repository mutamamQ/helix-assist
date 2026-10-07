package main

// Macros ("@...") replay keys: "|" opens the pipe prompt, "!" insert-output, ":" a command prompt; the text typed stays there for the user to finish, so no trailing "ret".
// Shell commands in Helix don't interpret quotes, and hxai joins its words, so the
// instruction needs no quoting.
const docSnippets = `# Paste into ~/.config/helix/config.toml  (space+i = "AI"; i is free in 25.07.1)
# Type the instruction after the prefilled prompt, then Enter. Esc cancels.
[keys.normal.space.i]
e = "@|hxai edit --file %{buffer_name} "   # rewrite selection (select first)
g = "@!hxai gen --file %{buffer_name} --line %{cursor_line} "  # insert generated code
a = "@:sh hxai ask --file %{buffer_name} --line %{cursor_line} "  # answer in a popup
k = "@:sh hxai keys "                       # keybinding tutor
c = ":insert-output hxai commit"            # commit message from git diff (run in a commit buffer)

[keys.select.space.i]
e = "@|hxai edit --file %{buffer_name} "
`
