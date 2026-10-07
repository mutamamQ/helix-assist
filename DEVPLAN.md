# helix-assist dev plan (fork: mutamamQ/helix-assist)

Branches: `dev` (integration) <- `feat/code-actions`, `feat/hxai`. Worktrees:
- ~/Documents/coding/helix-assist          -> dev
- ~/Documents/coding/helix-assist-actions  -> feat/code-actions
- ~/Documents/coding/helix-assist-hxai     -> feat/hxai

Style: follow the ponytail skill (~/.hermes/skills/software-development/ponytail/SKILL.md):
shortest working diff, reuse existing helpers, stdlib only (go.mod has zero deps, keep it that way),
one runnable check per non-trivial logic (Go `_test.go`, table tests ok, no frameworks).
DO NOT modify autocomplete behaviour (internal/handlers/completions.go, completion prompts, bryant Completion()).

Target: Helix 25.07.1 (installed /usr/bin/helix). Verified client facts (from helix 25.07.1 source):
- code actions: Helix sorts by kind category (quickfix first), then actions with non-empty `diagnostics`, then `isPreferred`.
  Menu is NOT filterable by typing; keep titles short. Disabled actions hidden.
- executing a Command: Helix sends workspace/executeCommand and does NOT wait for the edit; server must send
  `workspace/applyEdit` request (server->client, needs an id). Supports `documentChanges` with
  `{"kind":"create","uri":..,"options":{"ignoreIfExists":true}}` + TextDocumentEdit (version may be null).
  Edits to a file not open are applied by opening it in background.
- `window/showDocument` supported: takeFocus=true -> replaces current view; otherwise opens in a vertical split.
- `window/showMessage` -> statusline. Position encoding default UTF-16 (we never negotiate; columns are UTF-16).
- `:lsp-workspace-command <name> <json args>` runs any command listed in executeCommandProvider.commands
  (no args -> picker). Arguments parsed as JSON values.
- Shell integration: `:pipe cmd` / `|` replaces each selection with stdout; `:insert-output` / `!` inserts;
  `:append-output` / `A-!`; `:sh cmd` / `:run-shell-command` shows stdout in a popup rendered as ```sh block.
  Non-zero exit or any stderr -> stderr shown as error, NO edit. Expansions in these commands:
  %{buffer_name} (relative path), %{cursor_line}, %{cursor_column}, %{selection}, %{selection_line_start},
  %{selection_line_end}, %{language}, %{line_ending}, %sh{...}. NOT available in 25.07.1: file_path_absolute,
  workspace_directory, current_working_directory. Keymap macros: `"@..."` strings; keybind values can be
  ":cmd args" strings or lists of commands.
- Helix trims one trailing newline from shell output if the input selection didn't end in newline.

Existing building blocks (already on dev):
- internal/assist: ProjectContext(dir) (.helix-assist.md walk-up), FileWithMarkers(lines,start,end),
  EnclosingBlock(lines,line), IsHeader, FindInstructions(lines,start,end) ("# ai: ..." comments),
  InstructionTarget(lines, commentLine), FixIndent(out, original), TestPath(path, lang), StripPreamble, UTF16Len.
- internal/helixdocs: All() -> embedded Helix 25.07.1 book (keymap, textobjects, commands, remapping, ...), ~100KB.
- providers.Registry.Raw(ctx, model, system, user, maxTokens) -> runs any prompt on bryant ("" model = chat model).
  providers.CleanCodeOutput(s) strips ``` fences.
- config: BryantModel (completions), BryantModelForChat (sonnet 5.5), BryantModelDeep (opus 5.5),
  ActionTimeout now 120000ms, config.ReadHermesEnvKey("BRYANT_API_KEY").
- lsp: Service.SendRequest(method, params) (server->client request with id), Service.Reply(id, result),
  types: CodeAction.IsPreferred, WorkspaceEdit.DocumentChanges, TextDocumentEdit, CreateFile, ShowDocumentParams,
  EventShowDocument.
- Bridge: http://127.0.0.1:8765/v1 (OpenAI chat completions), key in ~/.hermes/.env BRYANT_API_KEY.
  Model ids: 15d8cc8844/CLAUDE_V5_5_SONNET (default), 15d8cc8844/CLAUDE_V5_5_OPUS (deep), 15d8cc8844/CLAUDE_V4_5_HAIKU (fast).
