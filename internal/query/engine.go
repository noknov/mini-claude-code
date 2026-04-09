package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noknov/mini-claude-code/internal/config"
	ctxinfo "github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/permission"
	"github.com/noknov/mini-claude-code/internal/provider"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/tool"
	"github.com/noknov/mini-claude-code/internal/tools"
	"github.com/noknov/mini-claude-code/internal/ui"
)

// Engine orchestrates the query loop: user input → API → tool execution → repeat.
type Engine struct {
	provider provider.Provider
	session  *session.Session
	ctx      *ctxinfo.Info
	cfg      *config.Config
	registry *tool.Registry
	perm     *permission.Manager
}

func NewEngine(prov provider.Provider, sess *session.Session, ctx *ctxinfo.Info, cfg *config.Config) *Engine {
	return &Engine{
		provider: prov,
		session:  sess,
		ctx:      ctx,
		cfg:      cfg,
		registry: tools.NewDefaultRegistry(),
		perm:     permission.NewManager(cfg.PermissionMode),
	}
}

func (e *Engine) SessionInfo() (inputTokens, outputTokens int, cost float64) {
	return e.session.InputTokens, e.session.OutputTokens, e.session.EstimateCost()
}

func (e *Engine) ClearSession()     { e.session.Clear() }
func (e *Engine) SetModel(m string) { e.provider.SetModel(m) }
func (e *Engine) GetModel() string  { return e.provider.Model() }

// Run processes a user message through the full tool-use loop.
func (e *Engine) Run(userInput string, terminal *ui.Terminal) {
	e.session.AddUserMessage(userInput)

	for {
		resp := e.callAPI(context.Background(), terminal)
		if resp == nil {
			return
		}

		e.session.AddAssistantMessage(resp.ContentBlocks)
		e.session.UpdateUsage(resp.InputTokens, resp.OutputTokens)

		toolCalls := extractToolCalls(resp.ContentBlocks)
		if len(toolCalls) == 0 {
			return
		}

		e.executeTools(toolCalls, terminal)
	}
}

func (e *Engine) executeTools(calls []toolCall, terminal *ui.Terminal) {
	for _, tc := range calls {
		terminal.PrintToolUse(tc.Name, tc.Input)

		t, ok := e.registry.Get(tc.Name)
		if !ok {
			e.session.AddToolResult(tc.ID, fmt.Sprintf("Unknown tool: %s", tc.Name), true)
			terminal.PrintToolError(tc.Name, fmt.Errorf("unknown tool: %s", tc.Name))
			continue
		}

		if t.NeedsPermission(tc.Input) {
			desc := t.FormatPermissionRequest(tc.Input)
			if !e.perm.Check(tc.Name, desc, terminal) {
				e.session.AddToolResult(tc.ID, "Permission denied by user", true)
				terminal.PrintToolDenied(tc.Name)
				continue
			}
		}

		result, err := e.registry.Execute(tc.Name, tc.Input, e.cfg.WorkDir)
		if err != nil {
			e.session.AddToolResult(tc.ID, err.Error(), true)
			terminal.PrintToolError(tc.Name, err)
		} else {
			e.session.AddToolResult(tc.ID, result, false)
			terminal.PrintToolResult(tc.Name, result)
		}
	}
}

func (e *Engine) callAPI(ctx context.Context, terminal *ui.Terminal) *provider.Response {
	req := provider.Request{
		SystemPrompt: e.buildSystemPrompt(),
		Messages:     e.session.Messages,
		Tools:        e.buildToolDefs(),
	}

	eventCh, errCh := e.provider.SendStream(ctx, req)

	var resp provider.Response
	streaming := false

	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				if streaming {
					terminal.StopStreaming()
				}
				return &resp
			}
			switch evt.Type {
			case "text":
				if evt.Text != "" {
					if !streaming {
						terminal.StartStreaming()
						streaming = true
					}
					terminal.StreamText(evt.Text)
				}
			case "tool_use_start":
				if streaming {
					terminal.StopStreaming()
					streaming = false
				}
			case "done":
				if streaming {
					terminal.StopStreaming()
					streaming = false
				}
				if evt.Response != nil {
					resp = *evt.Response
				}
			}

		case err, ok := <-errCh:
			if ok && err != nil {
				if streaming {
					terminal.StopStreaming()
				}
				terminal.PrintError(fmt.Errorf("API error: %w", err))
				return nil
			}
		}
	}
}

func (e *Engine) buildToolDefs() []provider.ToolDef {
	allTools := e.registry.All()
	defs := make([]provider.ToolDef, 0, len(allTools))
	for _, t := range allTools {
		var schema interface{}
		_ = json.Unmarshal(t.InputSchema(), &schema)
		defs = append(defs, provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}
	return defs
}

func (e *Engine) buildSystemPrompt() string {
	parts := []string{coreSystemPrompt}

	if e.ctx.GitStatus != "" {
		parts = append(parts, "<git_context>\n"+e.ctx.GitStatus+"\n</git_context>")
	}
	if e.ctx.ClaudeMD != "" {
		parts = append(parts, "<user_instructions>\n"+e.ctx.ClaudeMD+"\n</user_instructions>")
	}

	env := fmt.Sprintf("Working directory: %s\nOS: %s | Shell: %s\nDate: %s",
		e.ctx.WorkDir, e.ctx.OS, e.ctx.Shell, e.ctx.Date)
	parts = append(parts, env)

	return strings.Join(parts, "\n\n")
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func extractToolCalls(blocks []provider.ContentBlock) []toolCall {
	var calls []toolCall
	for _, b := range blocks {
		if b.Type == "tool_use" {
			calls = append(calls, toolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return calls
}

const coreSystemPrompt = `You are an AI coding assistant powered by Claude. You help users with software engineering tasks directly in their terminal.

You have tools for reading, writing, and editing files, running shell commands, and searching codebases. Use them to accomplish tasks efficiently.

Key guidelines:
- Read files before editing to understand existing code
- Use Bash for git operations, running tests, installing packages, etc.
- Use Edit for surgical file modifications (preferred over Write for existing files)
- Use Write only for new files or complete rewrites
- Use Grep/Glob to find relevant files before making changes
- Be concise; focus on actions over explanations
- Verify changes work (run tests, check for errors)
- Follow the project's existing code style and conventions`
