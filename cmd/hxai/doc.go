package main

// Macros ("@...") replay keys into a command prompt left open for the user to finish (no trailing "ret").
// Use ":pipe"/":insert-output", not the "|"/"!" prompts: only typable commands expand %{...} (verified in Helix 25.07.1).
// Helix hands the line to sh -c unquoted, so the prefill opens double quotes and
// <left> parks the cursor inside them: parens, & and apostrophes in the
// instruction are then safe (avoid " $ and backticks).
const docSnippets = `# Paste into ~/.config/helix/config.toml  (space+i = "AI"; i is free in 25.07.1)
# Type the instruction inside the prefilled quotes, then Enter. Esc cancels.
# Inside the quotes avoid " $ and backticks (the shell runs them; use ' or \" instead).
# Save the file first for best context (gen/ask read it from disk).
# Errors: the edit is aborted ("Shell command failed"); l shows the last ones (~/.cache/hxai.log).
[keys.normal.space.i]
e = "@:pipe hxai edit --file %{buffer_name} \"\"<left>"   # rewrite selection (select first)
g = "@x:append-output hxai gen --file %{buffer_name} --line %{cursor_line} \"\"<left>"  # insert code below cursor line
a = "@:sh hxai ask --file %{buffer_name} --line %{cursor_line} \"\"<left>"  # answer in a popup
k = "@:sh hxai keys \"\"<left>"                       # keybinding tutor
c = ":insert-output hxai commit --file %{buffer_name}"  # commit message from git diff (run in a commit buffer)
l = ":sh tail -n 5 ~/.cache/hxai.log"       # show last errors

[keys.select.space.i]
e = "@:pipe hxai edit --file %{buffer_name} \"\"<left>"
g = "@x:append-output hxai gen --file %{buffer_name} --line %{cursor_line} \"\"<left>"
a = "@:sh hxai ask --file %{buffer_name} --line %{cursor_line} \"\"<left>"
k = "@:sh hxai keys \"\"<left>"
c = ":insert-output hxai commit --file %{buffer_name}"
l = ":sh tail -n 5 ~/.cache/hxai.log"
`
