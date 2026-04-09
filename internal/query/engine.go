// Package query implements the core query loop: user input → LLM → tool
// execution → repeat until the model stops calling tools.
package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/noknov/mini-claude-code/internal/compact"
	"github.com/noknov/mini-claude-code/internal/config"
	ctxinfo "github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/hooks"
	"github.com/noknov/mini-claude-code/internal/permission"
	"github.com/noknov/mini-claude-code/internal/prompt"
	"github.com/noknov/mini-claude-code/internal/provider"
	"github.com/noknov/mini-claude-code/internal/retry"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/tool"
	"github.com/noknov/mini-claude-code/internal/tools"
	"github.com/noknov/mini-claude-code/internal/ui"
)

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

// Engine orchestrates the query loop.
type Engine struct {
	provider   provider.Provider
	session    *session.Session
	ctx        *ctxinfo.Info
	cfg        *config.Config
	registry   *tool.Registry
	perm       *permission.Manager
	hooks      *hooks.Runner
	compactor  *compact.Compactor
	retryCfg   retry.Config
	cancelFunc context.CancelFunc // set per-turn for Ctrl+C interruption
}

func NewEngine(
	prov provider.Provider,
	sess *session.Session,
	ctx *ctxinfo.Info,
	cfg *config.Config,
) *Engine {
	e := &Engine{
		provider:  prov,
		session:   sess,
		ctx:       ctx,
		cfg:       cfg,
		registry:  tools.NewDefaultRegistry(ctx.Skills, ctx.MCPClient),
		perm:      permission.NewManager(cfg.PermissionMode, ctx.Settings.Permissions),
		hooks:     hooks.NewRunner(ctx.Settings, cfg.WorkDir),
		compactor: compact.New(prov),
		retryCfg:  retry.DefaultConfig(),
	}

	// Wire the agent tool callback
	e.wireAgentTool()

	return e
}

// wireAgentTool connects the AgentTool to the engine's execution capability.
func (e *Engine) wireAgentTool() {
	if t, ok := e.registry.Get("Agent"); ok {
		if at, ok := t.(*tools.AgentTool); ok {
			at.OnSpawn = func(prompt, agentName string) (string, error) {
				// TODO: implement full subagent execution with isolated context
				return fmt.Sprintf("[Agent spawned with prompt: %s]", truncate(prompt, 200)), nil
			}
		}
	}
}

// ---------------------------------------------------------------------------
// REPLEngine interface
// ---------------------------------------------------------------------------

func (e *Engine) SessionInfo() (inputTokens, outputTokens int, cost float64) {
	in, out := e.session.TotalTokens()
	return in, out, e.session.EstimateCost()
}

func (e *Engine) ClearSession()     { e.session.Clear() }
func (e *Engine) SetModel(m string) { e.provider.SetModel(m) }
func (e *Engine) GetModel() string  { return e.provider.Model() }

// Cancel aborts the current in-flight API call.
func (e *Engine) Cancel() {
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
}

// ---------------------------------------------------------------------------
// Main query loop
// ---------------------------------------------------------------------------

// Run processes a user message through the full tool-use loop.
func (e *Engine) Run(userInput string, terminal *ui.Terminal) {
	e.session.AddUserMessage(userInput)
	e.autoCompactIfNeeded(terminal)

	for {
		resp := e.callAPI(terminal)
		if resp == nil {
			return
		}

		e.session.AddAssistantMessage(resp.ContentBlocks)
		e.session.UpdateUsage(resp.Model, resp.InputTokens, resp.OutputTokens)

		calls := extractToolCalls(resp.ContentBlocks)
		if len(calls) == 0 {
			return
		}

		e.executeTools(calls, terminal)
	}
}

// ---------------------------------------------------------------------------
// Tool execution
// ---------------------------------------------------------------------------

func (e *Engine) executeTools(calls []toolCall, terminal *ui.Terminal) {
	for _, tc := range calls {
		terminal.PrintToolUse(tc.Name, tc.Input)

		t, ok := e.registry.Get(tc.Name)
		if !ok {
			e.recordToolError(tc.ID, tc.Name, "unknown tool", terminal)
			continue
		}

		if !e.checkPermission(t, tc, terminal) {
			e.session.AddToolResult(tc.ID, "Permission denied by user", true)
			terminal.PrintToolDenied(tc.Name)
			continue
		}

		e.runPostHooksAndRecord(t, tc, terminal)
	}
}

func (e *Engine) checkPermission(t tool.Tool, tc toolCall, terminal *ui.Terminal) bool {
	if !t.NeedsPermission(tc.Input) {
		return true
	}

	// Run pre-tool hooks
	hookResults := e.hooks.Run(hooks.PreToolUse, tc.Name, tc.Input)
	hookDecision := hooks.ResolvePermission(hookResults)

	desc := t.FormatPermissionRequest(tc.Input)
	return e.perm.CheckWithHookDecision(hookDecision, tc.Name, desc, terminal)
}

func (e *Engine) runPostHooksAndRecord(t tool.Tool, tc toolCall, terminal *ui.Terminal) {
	result, err := e.registry.Execute(tc.Name, tc.Input, e.cfg.WorkDir)

	// Run post-tool hooks
	e.hooks.Run(hooks.PostToolUse, tc.Name, tc.Input)

	if err != nil {
		e.session.AddToolResult(tc.ID, err.Error(), true)
		terminal.PrintToolError(tc.Name, err)
	} else {
		e.session.AddToolResult(tc.ID, result, false)
		terminal.PrintToolResult(tc.Name, result)
	}
}

func (e *Engine) recordToolError(id, name, msg string, terminal *ui.Terminal) {
	e.session.AddToolResult(id, fmt.Sprintf("%s: %s", name, msg), true)
	terminal.PrintToolError(name, fmt.Errorf("%s", msg))
}

// ---------------------------------------------------------------------------
// API call with retry + streaming
// ---------------------------------------------------------------------------

func (e *Engine) callAPI(terminal *ui.Terminal) *provider.Response {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFunc = cancel
	defer func() { e.cancelFunc = nil }()

	req := provider.Request{
		SystemPrompt: e.buildSystemPrompt(),
		Messages:     e.session.Messages,
		Tools:        e.buildToolDefs(),
	}

	eventCh, errCh := retry.SendStreamWithRetry(ctx, e.provider, req, e.retryCfg)
	return e.consumeStream(eventCh, errCh, terminal)
}

func (e *Engine) consumeStream(
	eventCh <-chan provider.StreamEvent,
	errCh <-chan error,
	terminal *ui.Terminal,
) *provider.Response {
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
			streaming = e.handleStreamEvent(evt, &resp, streaming, terminal)

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

func (e *Engine) handleStreamEvent(
	evt provider.StreamEvent,
	resp *provider.Response,
	streaming bool,
	terminal *ui.Terminal,
) bool {
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
			*resp = *evt.Response
		}
	}
	return streaming
}

// ---------------------------------------------------------------------------
// Auto-compaction
// ---------------------------------------------------------------------------

func (e *Engine) autoCompactIfNeeded(terminal *ui.Terminal) {
	if !e.ctx.Settings.IsAutoCompactEnabled() {
		return
	}
	if !e.compactor.ShouldCompact(e.session.Messages) {
		return
	}

	terminal.PrintInfo("Auto-compacting conversation...")
	e.hooks.Run(hooks.PreCompact, "", nil)

	msgs, err := e.compactor.Compact(context.Background(), e.session.Messages)
	if err != nil {
		terminal.PrintError(fmt.Errorf("compact failed: %w", err))
		return
	}

	e.session.SetMessages(msgs)
	e.hooks.Run(hooks.PostCompact, "", nil)
	terminal.PrintSuccess("Conversation compacted")
}

// ---------------------------------------------------------------------------
// Prompt + tool definitions
// ---------------------------------------------------------------------------

func (e *Engine) buildSystemPrompt() string {
	return prompt.Build(&prompt.Context{
		OS:             e.ctx.OS,
		Shell:          e.ctx.Shell,
		WorkDir:        e.ctx.WorkDir,
		Date:           e.ctx.Date,
		GitStatus:      e.ctx.GitStatus,
		MemoryFiles:    e.ctx.MemoryFiles,
		Rules:          e.ctx.Rules,
		Skills:         e.ctx.Skills,
		Agents:         e.ctx.Agents,
		MCPClient:      e.ctx.MCPClient,
		OutputLanguage: e.ctx.Settings.OutputLanguage,
	})
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
