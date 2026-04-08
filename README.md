# mini-claude-code

A minimal reimplementation of Claude Code in Go.

## Features

- **Streaming API**: Real-time streaming responses from Claude
- **Tool Loop**: Full tool_use → tool_result → continue cycle
- **Built-in Tools**:
  - `Bash` - Shell command execution with timeout and permission control
  - `Read` - File reading with line range support
  - `Write` - File creation/overwrite
  - `Edit` - Surgical string replacement editing
  - `Glob` - File pattern matching search
  - `Grep` - Content search via ripgrep
- **Permission System**: Ask/auto/deny modes for tool execution
- **Context Awareness**: Git status, CLAUDE.md support
- **Slash Commands**: `/help`, `/clear`, `/cost`, `/model`, `/exit`
- **Single Binary**: Zero runtime dependencies, cross-platform

## Quick Start

```bash
export ANTHROPIC_API_KEY="your-key-here"
go build -o mini-claude-code ./cmd/mini-claude-code
./mini-claude-code
```

## Architecture

```
cmd/mini-claude-code/    # Entry point
internal/
  api/                   # Anthropic API client (streaming SSE)
  query/                 # Query loop engine (tool_use cycle)
  tool/                  # Tool interface and registry
  tools/                 # Built-in tool implementations
  ui/                    # Terminal UI (ANSI colors, streaming)
  config/                # Configuration and CLAUDE.md
  context/               # System/project context gathering
  session/               # Conversation state management
  permission/            # Tool permission management
  command/               # Slash command system
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | API key (required) |
| `ANTHROPIC_BASE_URL` | Custom API endpoint |
| `ANTHROPIC_MODEL` | Default model override |

## License

MIT
