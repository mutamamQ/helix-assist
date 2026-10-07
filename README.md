> Graduated from autocomplete to full AI agents? Check out [kb](https://github.com/leona/kb) - a shared knowledge base for Claude Code, Codex, and OpenCode.

# helix-assist

![Build Status](https://github.com/leona/helix-assist/actions/workflows/release.yml/badge.svg)
![GitHub Release](https://img.shields.io/github/v/release/leona/helix-assist)

A Go port of the [helix-gpt](https://github.com/leona/helix-gpt) language server, providing LLM code completions and actions tailored specifically for the Helix editor's LSP spec. This port serves as a more efficient, lightweight alternative using significantly less memory and resolving timeout issues and inconsistencies. Support is limited to OpenAI and Anthropic, with zero external dependencies.

### Completions 

![helix-assist example](https://github.com/leona/helix-assist/raw/master/assets/completions.gif)

### Code actions (space + a)

![helix-assist example](https://github.com/leona/helix-assist/raw/master/assets/actions.gif)

## Features

- **Code Completions**: AI-powered code suggestions as you type
- **Code Actions**: Built-in commands for code improvement
  - Resolve diagnostics
  - Improve code
  - Refactor from comment

## Supported Providers

- **OpenAI** (default)
- **Anthropic**

## Installation

### Pre-built Binaries

Download the latest release from the [releases page](https://github.com/leona/helix-assist/releases), or install directly:

```bash
# Linux AMD64 example
wget https://github.com/leona/helix-assist/releases/latest/download/helix-assist-linux-amd64
chmod +x helix-assist-linux-amd64
sudo mv helix-assist-linux-amd64 /usr/local/bin/helix-assist
```

Binaries are available for Linux and macOS (both AMD64 and ARM64).

### Building from Source

```bash
# Install directly from GitHub
go install github.com/leona/helix-assist/cmd/helix-assist@latest

# Or clone and build manually
git clone https://github.com/leona/helix-assist.git
cd helix-assist

# Build for current platform
make build

# Install to $GOPATH/bin
make install

# Build for specific platform
make linux-amd64
make darwin-arm64

# Build for all platforms
make build-all
```

The binary will be created in the `build/` directory (or `$GOPATH/bin` with `make install` or `go install`).

## Helix Configuration

Add to `~/.config/helix/languages.toml`:

```toml
[language-server.helix-assist]
command = "helix-assist"
# Optional
args = ["--handler", "anthropic", "--num-suggestions", "2"]

[[language]]
name = "go"
language-servers = ["gopls", "helix-assist"]

[[language]]
name = "typescript"
language-servers = ["typescript-language-server", "helix-assist"]

[[language]]
name = "python"
language-servers = ["pylsp", "helix-assist"]
```
## BryantGPT (fork addition)

This fork adds a `bryant` handler that talks to the local BryantGPT bridge
(`bryant-provider`, OpenAI-compatible `/v1/chat/completions`).

| Variable / flag | Default | Description |
|---|---|---|
| `HANDLER` / `--handler` | - | set to `bryant` |
| `BRYANT_API_KEY` / `--bryant-key` | read from `~/.hermes/.env` | local bridge key |
| `BRYANT_MODEL` / `--bryant-model` | `15d8cc8844/CLAUDE_V5_5_SONNET` | completion model |
| `BRYANT_MODEL_FOR_CHAT` / `--bryant-model-for-chat` | `15d8cc8844/CLAUDE_V5_5_SONNET` | code-action model |
| `BRYANT_ENDPOINT` / `--bryant-endpoint` | `http://127.0.0.1:8765/v1` | bridge URL |

Faster alternatives for completions: `15d8cc8844/CLAUDE_V4_5_HAIKU`, `15d8cc8844/OPENAI_GPT6_LUNA`.

```toml
[language-server.helix-assist]
command = "helix-assist"
args = ["--handler", "bryant", "--num-suggestions", "2"]
```

## AI features (fork additions)

### space-a code actions (context-aware)
The model always sees the whole file with the target marked. With nothing selected the
target is the enclosing function/class; otherwise the selected lines.
- `AI fix: <diagnostic>` entries for diagnostics under the cursor (top of the menu), plus `AI: fix all diagnostics`.
- `# ai: <instruction>` (any comment syntax) above or inside code: space-a offers `AI: <instruction>`,
  applies it and deletes the comment. A lone comment generates code in its place.
- improve, refactor, refactor (deep = Opus 5.5), add docs, add type hints, add error handling,
  optimize, simplify, explain (opens a markdown split), write tests (creates/appends `test_<file>`).
- Safety: prose/empty replies are rejected, CRLF preserved, stale edits re-located or discarded,
  edits are versioned, the same action can't run twice at once. Undo with `u`.
- Optional project notes: put `.helix-assist.md` in the repo (conventions, libraries); it's sent with every action.

### Autocomplete project context (opt-in)
`--completion-context` (or `COMPLETION_CONTEXT=true`) also sends `.helix-assist.md` and up to 4 local
files the current file imports (Python, JS/TS relative imports, Go same-package files; ~20KB cap).
Off by default: an A/B test on 45 real completions showed no accuracy gain and ~7% more latency.

### hxai CLI (space-i keybinds)
`hxai doc` prints keybinds for `~/.config/helix/config.toml`; type the instruction inside the
prefilled quotes (avoid `"`, `$` and backticks there: the shell runs them; use `'` instead; a
shell syntax error is written into the buffer before hxai starts, `u` undoes it). Save the file first for best
`g`/`a` context. `Space i l` shows the last errors. `hxai keys` answers Helix keybinding questions from the embedded 25.07.1 docs.
Errors abort the edit (Helix shows "Shell command failed"); details in `~/.cache/hxai.log`.
`make install-hxai` installs it.

## Usage

1. Start Helix and open a file
2. Type a trigger character (`{`, `(`, or space) to get completions
3. Manually trigger the completion list with `Ctrl + X` to see suggestions
4. Select code and press `Space + a` to see code actions

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HANDLER` | `openai` | Provider: `openai` or `anthropic` |
| `OPENAI_API_KEY` | - | OpenAI API key |
| `OPENAI_MODEL` | `gpt-4.1-mini` | OpenAI model for completions |
| `OPENAI_ENDPOINT` | `https://api.openai.com/v1` | OpenAI API endpoint |
| `ANTHROPIC_API_KEY` | - | Anthropic API key |
| `ANTHROPIC_MODEL` | `claude-sonnet-4-5` | Anthropic model |
| `ANTHROPIC_ENDPOINT` | `https://api.anthropic.com` | Anthropic API endpoint |
| `DEBOUNCE` | `200` | Debounce delay in milliseconds |
| `TRIGGER_CHARACTERS` | `{`\|\|`(`\|\|` ` | Completion triggers (separated by `\|\|`) |
| `NUM_SUGGESTIONS` | `1` | Number of completion suggestions |
| `LOG_FILE` | `~/.cache/helix-assist.log` | Log file path |
| `FETCH_TIMEOUT` | `15000` | API request timeout (ms) |
| `ACTION_TIMEOUT` | `15000` | Code action timeout (ms) |
| `COMPLETION_TIMEOUT` | `15000` | Completion timeout (ms) |

## Debugging

Monitor helix-assist activity by tailing the log files:

```bash
tail -f ~/.cache/helix-assist.log
tail -f ~/.cache/helix/helix.log
```

