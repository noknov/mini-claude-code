package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noknov/mini-claude-code/internal/api"
	"github.com/noknov/mini-claude-code/internal/config"
	"github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/permission"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/tool"
	"github.com/noknov/mini-claude-code/internal/tools"
	"github.com/noknov/mini-claude-code/internal/ui"
)

// Engine orchestrates the query loop: user input -> API call -> tool execution -> repeat
type Engine struct {
	client   *api.Client
	session  *session.Session
	ctx      *context.Info
	cfg      *config.Config
	registry *tool.Registry
	perm     *permission.Manager
}

func NewEngine(client *api.Client, sess *session.Session, ctx *context.Info, cfg *config.Config) *Engine {
	return &Engine{
		client:   client,
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

func (e *Engine) ClearSession() {
	e.session.Clear()
}

func (e *Engine) SetModel(model string) {
	e.client.SetModel(model)
}

func (e *Engine) GetModel() string {
	return e.client.Model()
}

// Run processes a user message through the full query loop
func (e *Engine) Run(userInput string, terminal *ui.Terminal) {
	e.session.AddUserMessage(userInput)

	for {
		response := e.callAPI(terminal)
		if response == nil {
			return
		}

		e.session.AddAssistantMessage(response.ContentBlocks)
		e.session.UpdateUsage(response.InputTokens, response.OutputTokens)

		// Check if there are tool calls to process
		toolCalls := extractToolCalls(response.ContentBlocks)
		if len(toolCalls) == 0 {
			return
		}

		// Execute tools and collect results
		allDone := true
		for _, tc := range toolCalls {
			terminal.PrintToolUse(tc.Name, tc.Input)

			// Check permission
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
			allDone = false
		}

		if allDone {
			return
		}
	}
}

func (e *Engine) callAPI(terminal *ui.Terminal) *api.StreamResponse {
	systemPrompt := e.buildSystemPrompt()

	toolDefs := make([]api.ToolDef, 0)
	for _, t := range e.registry.All() {
		var schema interface{}
		json.Unmarshal(t.InputSchema(), &schema)
		toolDefs = append(toolDefs, api.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}

	req := api.CreateMessageRequest{
		System:   systemPrompt,
		Messages: e.session.Messages,
		Tools:    toolDefs,
	}

	eventCh, errCh := e.client.CreateMessageStream(req)

	response := &api.StreamResponse{}
	var currentBlock *api.ContentBlock
	var jsonAccum strings.Builder

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return response
			}

			switch event.Type {
			case "message_start":
				var data api.MessageStartData
				json.Unmarshal(event.Data, &data)
				response.ID = data.ID
				response.Model = data.Model
				if data.Usage != nil {
					response.InputTokens = data.Usage.InputTokens
				}

			case "content_block_start":
				var data api.ContentBlockStartData
				json.Unmarshal(event.Data, &data)
				block := data.ContentBlock
				currentBlock = &block
				jsonAccum.Reset()

				if block.Type == "text" {
					terminal.StartStreaming()
				}

			case "content_block_delta":
				var data api.ContentBlockDeltaData
				json.Unmarshal(event.Data, &data)

				switch data.Delta.Type {
				case "text_delta":
					if currentBlock != nil {
						currentBlock.Text += data.Delta.Text
						terminal.StreamText(data.Delta.Text)
					}
				case "input_json_delta":
					jsonAccum.WriteString(data.Delta.PartialJSON)
				}

			case "content_block_stop":
				if currentBlock != nil {
					if currentBlock.Type == "tool_use" && jsonAccum.Len() > 0 {
						currentBlock.Input = json.RawMessage(jsonAccum.String())
					}
					if currentBlock.Type == "text" {
						terminal.StopStreaming()
					}
					response.ContentBlocks = append(response.ContentBlocks, *currentBlock)
					currentBlock = nil
				}

			case "message_delta":
				var data api.MessageDeltaData
				json.Unmarshal(event.Data, &data)
				response.StopReason = data.Delta.StopReason
				if data.Usage != nil {
					response.OutputTokens = data.Usage.OutputTokens
				}
			}

		case err, ok := <-errCh:
			if ok && err != nil {
				terminal.PrintError(fmt.Errorf("API error: %w", err))
				return nil
			}
		}
	}
}

func (e *Engine) buildSystemPrompt() string {
	var parts []string

	parts = append(parts, coreSystemPrompt)

	if e.ctx.GitStatus != "" {
		parts = append(parts, fmt.Sprintf("<git_context>\n%s\n</git_context>", e.ctx.GitStatus))
	}

	if e.ctx.ClaudeMD != "" {
		parts = append(parts, fmt.Sprintf("<user_instructions>\n%s\n</user_instructions>", e.ctx.ClaudeMD))
	}

	parts = append(parts, fmt.Sprintf("Current date: %s", e.ctx.Date))
	parts = append(parts, fmt.Sprintf("Working directory: %s", e.ctx.WorkDir))
	parts = append(parts, fmt.Sprintf("OS: %s | Shell: %s", e.ctx.OS, e.ctx.Shell))

	return strings.Join(parts, "\n\n")
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func extractToolCalls(blocks []api.ContentBlock) []toolCall {
	var calls []toolCall
	for _, b := range blocks {
		if b.Type == "tool_use" {
			calls = append(calls, toolCall{
				ID:    b.ID,
				Name:  b.Name,
				Input: b.Input,
			})
		}
	}
	return calls
}

const coreSystemPrompt = `You are an AI coding assistant powered by Claude. You help users with software engineering tasks directly in their terminal.

You have access to tools for reading, writing, and editing files, running shell commands, and searching codebases. Use these tools to accomplish tasks efficiently.

Key guidelines:
- Read files before editing them to understand the existing code
- Use Bash for git operations, running tests, installing packages, etc.
- Use Edit for surgical file modifications (preferred over Write for existing files)
- Use Write only for creating new files or complete rewrites
- Use Grep/Glob to find relevant files before making changes
- Always prefer editing existing files over creating new ones
- Be concise in your responses; focus on actions over explanations
- When making changes, verify they work (run tests, check for errors)
- Follow the project's existing code style and conventions`
