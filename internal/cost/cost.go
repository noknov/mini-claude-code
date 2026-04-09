// Package cost tracks token usage and estimates costs across multiple models.
package cost

import "strings"

// ---------------------------------------------------------------------------
// Model pricing (per million tokens)
// ---------------------------------------------------------------------------

type pricing struct {
	input  float64
	output float64
}

var modelPricing = map[string]pricing{
	// Anthropic
	"claude-sonnet-4": {3.0, 15.0},
	"claude-opus-4":   {15.0, 75.0},
	"claude-haiku":    {0.25, 1.25},
	// OpenAI
	"gpt-4o":      {2.5, 10.0},
	"gpt-4o-mini": {0.15, 0.60},
	"gpt-4-turbo": {10.0, 30.0},
	"o1":          {15.0, 60.0},
	"o3-mini":     {1.10, 4.40},
}

// ---------------------------------------------------------------------------
// Tracker
// ---------------------------------------------------------------------------

// Tracker accumulates token usage across model switches.
type Tracker struct {
	entries []entry
}

type entry struct {
	model        string
	inputTokens  int
	outputTokens int
}

func NewTracker() *Tracker {
	return &Tracker{}
}

// Add records token usage for a model.
func (t *Tracker) Add(model string, inputTokens, outputTokens int) {
	t.entries = append(t.entries, entry{model, inputTokens, outputTokens})
}

// TotalTokens returns aggregate input and output tokens.
func (t *Tracker) TotalTokens() (input, output int) {
	for _, e := range t.entries {
		input += e.inputTokens
		output += e.outputTokens
	}
	return
}

// EstimateCost returns the total estimated cost in USD.
func (t *Tracker) EstimateCost() float64 {
	var total float64
	for _, e := range t.entries {
		p := lookupPricing(e.model)
		total += float64(e.inputTokens)/1e6*p.input + float64(e.outputTokens)/1e6*p.output
	}
	return total
}

// Clear resets all tracked usage.
func (t *Tracker) Clear() {
	t.entries = nil
}

// ---------------------------------------------------------------------------
// Pricing lookup
// ---------------------------------------------------------------------------

func lookupPricing(model string) pricing {
	// Exact match first
	if p, ok := modelPricing[model]; ok {
		return p
	}
	// Prefix match (e.g. "claude-sonnet-4-20250514" → "claude-sonnet-4")
	for prefix, p := range modelPricing {
		if strings.HasPrefix(model, prefix) {
			return p
		}
	}
	// Default to Sonnet pricing
	return pricing{3.0, 15.0}
}
