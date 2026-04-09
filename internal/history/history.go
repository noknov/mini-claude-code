// Package history provides session persistence and resume functionality.
//
// Sessions are saved as JSON files under ~/.claude/projects/<slug>/sessions/.
// The /resume command lists recent sessions and restores one.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/noknov/mini-claude-code/internal/memory"
	"github.com/noknov/mini-claude-code/internal/provider"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SessionRecord is the on-disk format for a saved session.
type SessionRecord struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Model     string             `json:"model"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []provider.Message `json:"messages"`
}

// Summary is a lightweight listing entry (no messages).
type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Turns     int       `json:"turns"`
}

// ---------------------------------------------------------------------------
// Save / Load
// ---------------------------------------------------------------------------

// Save persists a session to disk.
func Save(workDir string, record *SessionRecord) error {
	dir := sessionsDir(workDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	record.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, record.ID+".json"), data, 0644)
}

// Load reads a session from disk by ID.
func Load(workDir, sessionID string) (*SessionRecord, error) {
	path := filepath.Join(sessionsDir(workDir), sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	var record SessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &record, nil
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

// List returns summaries of recent sessions, sorted by update time (newest first).
func List(workDir string, limit int) []Summary {
	dir := sessionsDir(workDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var summaries []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var record SessionRecord
		if json.Unmarshal(data, &record) != nil {
			continue
		}
		summaries = append(summaries, Summary{
			ID:        record.ID,
			Title:     record.Title,
			Model:     record.Model,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
			Turns:     len(record.Messages),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries
}

// Delete removes a session from disk.
func Delete(workDir, sessionID string) error {
	return os.Remove(filepath.Join(sessionsDir(workDir), sessionID+".json"))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sessionsDir(workDir string) string {
	return filepath.Join(memory.AutoMemoryDir(workDir), "sessions")
}

// GenerateID creates a short session ID from the current timestamp.
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
}
