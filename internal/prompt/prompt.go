// Package prompt constructs the system prompt from multiple sections.
//
// The prompt is split into a static prefix (cacheable) and a dynamic suffix
// (changes per turn). This enables prompt caching on the Anthropic API.
package prompt

import (
	"fmt"
	"strings"

	"github.com/noknov/mini-claude-code/internal/agent"
	"github.com/noknov/mini-claude-code/internal/mcp"
	"github.com/noknov/mini-claude-code/internal/memory"
	"github.com/noknov/mini-claude-code/internal/rules"
	"github.com/noknov/mini-claude-code/internal/skills"
)

// ---------------------------------------------------------------------------
// Context passed to the builder
// ---------------------------------------------------------------------------

// Context holds all the data needed to build a system prompt.
type Context struct {
	OS        string
	Shell     string
	WorkDir   string
	Date      string
	GitStatus string

	MemoryFiles []memory.File
	Rules       []rules.Rule
	Skills      []skills.Skill
	Agents      []agent.Definition
	MCPClient   *mcp.Client

	OutputLanguage string
}

// ---------------------------------------------------------------------------
// Builder
// ---------------------------------------------------------------------------

// Build constructs the full system prompt from all sections.
func Build(ctx *Context) string {
	sections := []section{
		{name: "identity", content: coreIdentity},
		{name: "tool_guidelines", content: toolGuidelines},
		{name: "git_safety", content: gitSafety},
		{name: "code_style", content: codeStyle},
	}

	// Dynamic sections (vary per project/session)
	if ctx.GitStatus != "" {
		sections = append(sections, section{"git_context", wrapXML("git_context", ctx.GitStatus)})
	}

	if memText := memory.FormatForPrompt(ctx.MemoryFiles); memText != "" {
		sections = append(sections, section{"user_instructions", wrapXML("user_instructions", memText)})
	}

	if rulesText := rules.FormatUnconditional(ctx.Rules); rulesText != "" {
		sections = append(sections, section{"rules", wrapXML("rules", rulesText)})
	}

	if listing := skills.FormatListing(ctx.Skills); listing != "" {
		sections = append(sections, section{"skills", wrapXML("skills", listing)})
	}

	if len(ctx.Agents) > 0 {
		sections = append(sections, section{"agents", wrapXML("agents", formatAgentListing(ctx.Agents))})
	}

	if ctx.MCPClient != nil && ctx.MCPClient.HasServers() {
		sections = append(sections, section{"mcp", wrapXML("mcp", ctx.MCPClient.FormatInstructions())})
	}

	if ctx.OutputLanguage != "" {
		sections = append(sections, section{"output_language",
			fmt.Sprintf("Respond in %s unless the user explicitly requests another language.", ctx.OutputLanguage)})
	}

	sections = append(sections, section{"environment", formatEnvironment(ctx)})

	return joinSections(sections)
}

// StaticPrefix returns the portion of the prompt that is stable across turns
// (suitable for prompt caching). Dynamic content starts after this boundary.
func StaticPrefix() string {
	return strings.Join([]string{coreIdentity, toolGuidelines, gitSafety, codeStyle}, "\n\n")
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

type section struct {
	name    string
	content string
}

func joinSections(sections []section) string {
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.content != "" {
			parts = append(parts, s.content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func wrapXML(tag, content string) string {
	return fmt.Sprintf("<%s>\n%s\n</%s>", tag, content, tag)
}

func formatEnvironment(ctx *Context) string {
	return fmt.Sprintf("Working directory: %s\nOS: %s | Shell: %s\nDate: %s",
		ctx.WorkDir, ctx.OS, ctx.Shell, ctx.Date)
}

func formatAgentListing(agents []agent.Definition) string {
	var sb strings.Builder
	sb.WriteString("Available agents (spawn with AgentTool):\n")
	for _, a := range agents {
		sb.WriteString("  - " + a.Name)
		if a.Description != "" {
			sb.WriteString(": " + a.Description)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Static prompt sections
// ---------------------------------------------------------------------------

const coreIdentity = `You are an AI coding assistant. You help users with software engineering tasks directly in their terminal.

You have tools for reading, writing, and editing files, running shell commands, searching codebases, fetching web content, and managing subagents. Use them to accomplish tasks efficiently.`

const toolGuidelines = `Tool usage guidelines:
- Read files before editing to understand existing code
- Use Bash for git operations, running tests, installing packages
- Use Edit for surgical file modifications (preferred over Write for existing files)
- Use Write only for new files or complete rewrites
- Use Grep/Glob to find relevant files before making changes
- Be concise; focus on actions over explanations
- Verify changes work (run tests, check for errors)
- Follow the project's existing code style and conventions`

const gitSafety = `Git safety:
- Never force push to main/master
- Never use --no-verify unless explicitly asked
- Always use conventional commit messages
- Check git status before committing
- Never commit secrets or credentials`

const codeStyle = `Code quality:
- Do not add obvious comments that just narrate what code does
- Comments should explain non-obvious intent, trade-offs, or constraints
- Match the existing project's style and conventions
- Prefer editing existing files over creating new ones`
