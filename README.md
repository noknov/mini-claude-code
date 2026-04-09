# mini-claude-code

A minimal reimplementation of [Claude Code](https://docs.anthropic.com/en/docs/claude-code) in Go. ~5400 lines, single binary, zero dependencies.

## Architecture

```
User input
  │
  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  main.go — Entry point                                              │
│  Parses flags, loads config, gathers context, creates provider,     │
│  starts REPL (interactive) or runs single prompt (pipe mode -p).    │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
  ┌────────────────────────┼────────────────────────────────┐
  ▼                        ▼                                ▼
┌──────────────┐    ┌──────────────────────────────┐  ┌──────────────┐
│  ui/terminal │    │  query/engine — core loop     │  │  context/    │
│              │    │                                │  │              │
│  • REPL      │◄──►│  1. Build system prompt        │  │  Gathers:    │
│  • Streaming │    │  2. Auto-compact if needed     │  │  • OS/Shell  │
│  • /commands │    │  3. Send to provider (retry)   │  │  • Git status│
│  • Permission│    │  4. Parse streaming response   │  │  • Memory    │
│    prompts   │    │  5. Execute tools (with hooks) │  │  • Rules     │
│  • Skill     │    │  6. Loop until no more tools   │  │  • Skills    │
│    invocation│    │                                │  │  • Agents    │
└──────────────┘    └──────────────────────────────┘  │  • MCP       │
                           │                           │  • Settings  │
         ┌─────────────────┼─────────────────┐         └──────────────┘
         ▼                 ▼                 ▼
  ┌────────────┐  ┌──────────────┐  ┌──────────────┐
  │ provider/  │  │ session/     │  │ permission/  │
  │            │  │              │  │              │
  │ Anthropic  │  │ Messages     │  │ ask/auto/    │
  │ OpenAI     │  │ Cost tracker │  │ deny/plan    │
  │ (any       │  │ Session ID   │  │ Rules file   │
  │  compat.)  │  │              │  │ Hook-based   │
  └────────────┘  └──────────────┘  │ Classifier   │
                                    └──────────────┘
```

## Module Map

| Module | Lines | What it does |
|--------|------:|--------------|
| **Provider Layer** | | |
| `provider/provider.go` | 81 | Generic `Provider` interface + unified message/event types |
| `provider/anthropic.go` | 234 | Anthropic Messages API with SSE streaming |
| `provider/openai.go` | 313 | OpenAI Chat Completions API (compatible with any OpenAI-like endpoint) |
| **Core Loop** | | |
| `query/engine.go` | 350 | Tool-use loop: API call → parse stream → execute tools → repeat. Integrates hooks, compaction, retry, permissions |
| `prompt/prompt.go` | 163 | System prompt builder with static/dynamic sections, cache boundary |
| `retry/retry.go` | 136 | Exponential backoff retry + model fallback |
| **Instruction System** | | |
| `memory/memory.go` | 170 | 5-layer memory: managed → user → project → local → auto-memory |
| `rules/rules.go` | 187 | `.claude/rules/*.md` loader with YAML frontmatter conditional globs |
| `skills/skills.go` | 140 | `.claude/commands/` skill loader, `/skill-name` invocation |
| **Extensibility** | | |
| `hooks/hooks.go` | 154 | Lifecycle hooks (PreToolUse, PostToolUse, Pre/PostCompact, Session*) |
| `agent/agent.go` | 177 | Custom agent definitions from `.claude/agents/*.md` |
| `mcp/mcp.go` | 309 | MCP client: JSON-RPC over stdio, tool/resource discovery |
| **Tools** | | |
| `tools/bash.go` | 108 | Shell execution with timeout |
| `tools/file_read.go` | 134 | File/directory reading with line numbers |
| `tools/file_write.go` | 65 | File creation with auto-mkdir |
| `tools/file_edit.go` | 101 | Exact string replacement |
| `tools/glob.go` | 181 | File pattern search with `**/` support |
| `tools/grep.go` | 112 | Content search (ripgrep/grep fallback) |
| `tools/web_fetch.go` | 76 | HTTP URL fetching |
| `tools/web_search.go` | 135 | Web search via DuckDuckGo |
| `tools/agent_tool.go` | 68 | Subagent spawning |
| `tools/skill_tool.go` | 57 | Programmatic skill invocation |
| `tools/mcp_tool.go` | 117 | MCP tool/resource access |
| `tools/notebook_edit.go` | 141 | Jupyter notebook cell editing |
| `tools/todo_write.go` | 115 | Structured task list management |
| **Runtime** | | |
| `compact/compact.go` | 192 | Conversation compaction: auto-detect, full summarize, micro-trim |
| `permission/permission.go` | 155 | Permission modes + settings rules + hook integration + classifier stub |
| `session/session.go` | 87 | Message history + cost tracking |
| `cost/cost.go` | 93 | Multi-model pricing lookup and cost estimation |
| `sandbox/sandbox.go` | 67 | Bash sandbox stub (macOS/Linux) |
| `history/history.go` | 137 | Session persistence + resume listing |
| **Configuration** | | |
| `config/config.go` | 119 | CLI flags + env vars + pipe mode |
| `settings/settings.go` | 136 | Multi-layer settings merge (managed/user/project/local) |
| `context/context.go` | 120 | System/project context gathering |
| **UI** | | |
| `ui/terminal.go` | 278 | REPL, streaming, slash commands, skill invocation, permission prompts |

## Quick Start

```bash
export ANTHROPIC_API_KEY="your-key-here"
go build -o mini-claude-code ./cmd/mini-claude-code
./mini-claude-code
```

### OpenAI / Compatible Endpoints

```bash
export MINI_CLAUDE_PROVIDER=openai
export OPENAI_API_KEY="your-key"
export OPENAI_BASE_URL="https://api.openai.com"  # or any compatible endpoint
./mini-claude-code
```

### Non-Interactive Mode

```bash
./mini-claude-code -p "explain this error"
```

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MINI_CLAUDE_PROVIDER` | Provider: `anthropic` (default), `openai` |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `ANTHROPIC_BASE_URL` | Custom Anthropic endpoint |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENAI_BASE_URL` | Custom OpenAI-compatible endpoint |

### Settings Files

Settings are loaded and merged from (lowest to highest priority):
1. `/etc/claude-code/settings.json` (managed)
2. `~/.claude/settings.json` (user)
3. `.claude/settings.json` (project, in each ancestor dir)
4. `.claude/settings.local.json` (local, not committed)

### Memory / Instructions

Instruction files are loaded in priority order:
1. `/etc/claude-code/CLAUDE.md` (managed)
2. `~/.claude/CLAUDE.md` (user)
3. `CLAUDE.md` + `.claude/CLAUDE.md` (project, in ancestor dirs)
4. `CLAUDE.local.md` (local, not committed)
5. `~/.claude/projects/<slug>/MEMORY.md` (auto-memory)

### Rules

Place `.md` files in `.claude/rules/` for project-specific instructions.
Add YAML frontmatter with `paths:` to make rules conditional:

```markdown
---
paths:
  - "*.go"
  - "internal/**/*.go"
---
Always use Go error wrapping with %w.
```

### Skills

Place `.md` files in `.claude/commands/` to create reusable prompt templates.
Invoke with `/skill-name` in the REPL.

### Agents

Place `.md` files in `.claude/agents/` with optional frontmatter:

```markdown
---
description: Code reviewer
model: claude-sonnet-4-20250514
permission_mode: auto
tools:
  - Read
  - Grep
  - Glob
---
You are a code reviewer. Analyze the code for bugs and improvements.
```

### Hooks

Configure in `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      { "command": "echo $TOOL_NAME $TOOL_INPUT", "if": "Bash" }
    ],
    "PostToolUse": [
      { "command": "notify-send 'Tool done'" }
    ]
  }
}
```

### MCP Servers

Configure in `.mcp.json`:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "node",
      "args": ["server.js"],
      "env": { "PORT": "3000" }
    }
  }
}
```

## Feature Coverage vs Claude Code

| Feature | Claude Code | mini-claude-code |
|---------|:-----------:|:----------------:|
| LLM tool-use loop | ✅ | ✅ |
| Multi-provider (Anthropic/OpenAI) | ✅ | ✅ |
| SSE streaming | ✅ | ✅ |
| Core tools (Bash/Read/Write/Edit/Glob/Grep) | ✅ | ✅ |
| WebFetch / WebSearch | ✅ | ✅ |
| Notebook editing | ✅ | ✅ |
| Todo/task management | ✅ | ✅ |
| Multi-layer memory (5 levels) | ✅ | ✅ |
| Rules system (conditional globs) | ✅ | ✅ |
| Skills system | ✅ | ✅ |
| Hooks (pre/post tool, compact, session) | ✅ | ✅ |
| Conversation compaction (auto/full/micro) | ✅ | ✅ |
| Agent/Subagent definitions | ✅ | ✅ (basic) |
| MCP integration | ✅ | ✅ (stdio) |
| Permission rules (allow/deny globs) | ✅ | ✅ |
| Multi-layer settings merge | ✅ | ✅ |
| API retry + model fallback | ✅ | ✅ |
| Non-interactive pipe mode | ✅ | ✅ |
| Session persistence / resume | ✅ | ✅ (basic) |
| Multi-model cost tracking | ✅ | ✅ |
| System prompt caching boundary | ✅ | ✅ (designed) |
| Sandbox stub | ✅ | ✅ (stub) |
| Ctrl+C interruption | ✅ | ✅ |
| Bash classifier (auto-approve safe cmds) | ✅ | stub |
| Fork subagent (cache sharing) | ✅ | planned |
| React+Ink terminal UI | ✅ | ANSI (simpler) |
| LSP integration | ✅ | — |
| Voice input | ✅ | — |
| Plugin system | ✅ | — |
| OAuth / auth | ✅ | — |
| Remote sessions | ✅ | — |
| Vim keybindings | ✅ | — |

## License

MIT
