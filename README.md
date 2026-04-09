# mini-claude-code

A minimal reimplementation of [Claude Code](https://docs.anthropic.com/en/docs/claude-code) in Go. ~1850 lines, single binary, zero dependencies.

## How It Works

When you type a message, the following happens:

```
User input
  │
  ▼
┌─────────────────────────────────────────────────────────────────┐
│  main.go                                                        │
│  Loads config, gathers context, wires everything together,      │
│  starts the REPL loop.                                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
  ▼                        ▼
┌──────────────┐    ┌──────────────────────────────────────────────┐
│  ui/terminal │    │  query/engine — the core loop                │
│              │    │                                              │
│  • Reads     │◄──►│  1. Add user message to session              │
│    user      │    │  2. Build system prompt (core + git + CLAUDE  │
│    input     │    │     .md + env info)                          │
│  • Streams   │    │  3. Send messages + tool defs to API         │
│    LLM text  │    │  4. Parse SSE stream, accumulate response    │
│  • Shows     │    │  5. Extract tool_use blocks                  │
│    tool use  │    │  6. For each tool call:                      │
│  • Asks      │    │     a. Check permission (ask/auto/deny)      │
│    permission│    │     b. Execute tool                          │
│  • Handles   │    │     c. Record tool_result in session         │
│    /commands │    │  7. If any tool was called → go to step 2    │
│              │    │     If no tools → done, wait for next input  │
└──────────────┘    └──────────────────────────────────────────────┘
                           │
              ┌────────────┼────────────────┐
              ▼            ▼                ▼
        ┌──────────┐ ┌──────────┐   ┌─────────────┐
        │ api/     │ │ session/ │   │ permission/ │
        │ client   │ │          │   │             │
        │          │ │ Messages │   │ ask/auto/   │
        │ HTTP POST│ │ Token    │   │ deny + per- │
        │ SSE parse│ │ counts   │   │ tool always │
        └──────────┘ └──────────┘   └─────────────┘
              │
              ▼
        ┌──────────────────────────────────────┐
        │  tool/ + tools/                       │
        │                                       │
        │  Registry holds 6 tools:              │
        │  • Bash  — shell execution + timeout  │
        │  • Read  — file/dir reading           │
        │  • Write — file creation              │
        │  • Edit  — string replacement         │
        │  • Glob  — file pattern search        │
        │  • Grep  — content search (rg/grep)   │
        └──────────────────────────────────────┘
```

### Module Breakdown

| Module | Lines | What it does |
|--------|------:|--------------|
| `cmd/mini-claude-code/main.go` | 79 | Entry point. Parses `--version`/`--help`/`--model`, loads config, creates all components, starts REPL. Sets up SIGINT/SIGTERM handler for graceful exit. |
| `internal/api/client.go` | 224 | Anthropic Messages API client. Builds HTTP request with auth headers, sends it, and parses the SSE stream (`event:` / `data:` lines) into typed `StreamEvent`s delivered via a Go channel. Accepts `context.Context` for cancellation. |
| `internal/query/engine.go` | 285 | **The heart of the program.** Runs the tool-use loop: calls the API, parses the streamed response into `ContentBlock`s, extracts `tool_use` blocks, executes tools, records `tool_result`s, and loops until the model stops calling tools. Also builds the system prompt and tool definitions. |
| `internal/tool/tool.go` | 61 | Defines the `Tool` interface (Name, Description, InputSchema, Execute, NeedsPermission, FormatPermissionRequest) and a `Registry` that stores tools in registration order. |
| `internal/tools/*.go` | 618 | Six built-in tools. Each implements the `Tool` interface. `registry.go` wires them all into a default registry. `resolvePath()` is shared across file tools. |
| `internal/ui/terminal.go` | 215 | Terminal I/O: prints welcome banner, reads input, streams LLM text character-by-character, displays tool use/results/errors, handles slash commands (`/help`, `/clear`, `/cost`, `/model`, `/compact`, `/exit`), and prompts for permission. Defines the `REPLEngine` interface to decouple from the query engine. |
| `internal/config/config.go` | 73 | Loads settings from env vars and CLI flags (priority: flags > env > defaults). Also provides `FindClaudeMD()` which walks up the directory tree and checks `~/.claude/CLAUDE.md`. |
| `internal/context/context.go` | 85 | Gathers system context at startup: OS, shell, git branch/status/recent commits, CLAUDE.md content, current date. All injected into the system prompt. |
| `internal/session/session.go` | 65 | Manages the `[]api.Message` conversation history, accumulates token counts, and estimates cost based on Claude Sonnet pricing. |
| `internal/permission/permission.go` | 49 | Three modes: `ask` (prompt user with Y/n/always), `auto` (allow all), `deny` (block all). Remembers per-tool "always" approvals for the session. |

### Key Design Decisions

- **`ContentBlock.Content` is `string`, not `interface{}`** — the Anthropic API expects `tool_result.content` as a string. Using `interface{}` caused serialization ambiguity.
- **Tool registry preserves insertion order** — so the tools array sent to the API is deterministic (matters for prompt caching).
- **SSE parsing is hand-rolled** — no SDK dependency. A `bufio.Scanner` reads `event:` / `data:` lines and sends typed events through a channel.
- **Tool results always feed back to the model** — even when all tools are denied/unknown, the loop continues so the model sees the rejection and can adjust.
- **`context.Context` plumbed through API calls** — ready for future Ctrl+C cancellation support.

## Quick Start

```bash
export ANTHROPIC_API_KEY="your-key-here"
go build -o mini-claude-code ./cmd/mini-claude-code
./mini-claude-code
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | API key (required) |
| `ANTHROPIC_BASE_URL` | Custom API endpoint |
| `ANTHROPIC_MODEL` | Default model override |

## Roadmap

Features present in the official Claude Code that we haven't implemented yet, roughly ordered by impact:

### High Priority

- [ ] **Conversation compaction** (`/compact`) — When the conversation approaches the context window limit, summarize older messages to free space. The official version has a 5-level compaction hierarchy (snip → microcompact → context collapse → auto-compact → reactive compact). We need at least a basic single-pass summarization.
- [ ] **Request cancellation (Ctrl+C interrupt)** — `context.Context` is already plumbed through the API layer; wire it to a signal handler so Escape/Ctrl+C aborts the in-flight API call and any running tool. Must generate `tool_result` with `is_error: true` for every orphaned `tool_use` block.
- [ ] **Concurrent tool execution** — The official version partitions tool calls into read-only batches (run concurrently) and write batches (run serially). We currently run everything serially.
- [ ] **Streaming tool execution** — Start executing tools as soon as each `tool_use` block is fully received, without waiting for the entire API response to finish.
- [ ] **Richer system prompt** — The official prompt is thousands of tokens covering git safety protocols, commit/PR formatting, code style rules, and per-tool behavioral instructions. Our prompt is minimal.

### Medium Priority

- [ ] **Auto-memory system** — Persistent file-based memory (`~/.claude/projects/<slug>/memory/MEMORY.md`) that the model reads and writes across sessions. Four memory types: user preferences, feedback, project context, reference.
- [ ] **Prompt caching** — Split the system prompt at a static/dynamic boundary marker. Static content (identity, rules, tool descriptions) gets `cache_control: { scope: "global" }` for cross-session reuse. Dynamic content (git status, CLAUDE.md, date) is uncached.
- [ ] **API retry and model fallback** — Retry on transient errors (429, 5xx) with exponential backoff. Fall back to a secondary model when the primary is unavailable.
- [ ] **Multi-turn max-output recovery** — When the model hits `max_tokens` mid-response, automatically continue generation (up to 3 retries) instead of truncating.
- [ ] **`.claude/rules/*.md` support** — Load all `.md` files from `.claude/rules/` directories (not just `CLAUDE.md`), with `@include` directive support.
- [ ] **Non-interactive / pipe mode** — Accept a prompt via CLI argument or stdin, print the response, and exit. Useful for scripting (`mini-claude-code -p "explain this error"`).

### Lower Priority

- [ ] **Agent / subagent system** — Spawn child conversations with isolated context for research, code review, or parallel tasks. The official version supports both "fork" (inherits parent context) and "fresh" (clean slate) agents.
- [ ] **Hooks system** — User-configurable shell commands that run before/after tool execution (pre-tool-use, post-tool-use, post-sampling hooks).
- [ ] **MCP (Model Context Protocol) integration** — Connect to external MCP servers to extend tool capabilities.
- [ ] **Permission rules engine** — Glob-based allow/deny rules in settings (e.g., `Bash(git *)` to auto-approve git commands) instead of per-invocation prompts.
- [ ] **Bash sandbox** — Restrict file system and network access for shell commands using OS-level sandboxing.
- [ ] **Session persistence** — Save/restore conversation history to disk so sessions survive restarts. Support `/resume` to continue a previous session.
- [ ] **Token budget tracking** — Let users specify a token budget (e.g., `+500k`) and auto-continue until the budget is spent.
- [ ] **Cost tracking per model** — Dynamic pricing lookup instead of hardcoded Sonnet rates. Track costs across model switches.
- [ ] **Multi-line input** — Support pasting or typing multi-line prompts (the official version uses a full React+Ink terminal UI).
- [ ] **Markdown rendering** — Render code blocks with syntax highlighting and format tables/lists in the terminal output.

## License

MIT
