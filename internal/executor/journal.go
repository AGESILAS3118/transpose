package executor

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry represents a single journal line recording planned or executed actions.
type Entry struct {
	Timestamp  time.Time `json:"ts"`
	RunID      string    `json:"run_id"`
	StepOrder  int       `json:"step_order"`
	Manager    string    `json:"manager"` // repos | dnf | flatpak | apt
	Action     string    `json:"action"`  // install | restore-repos | enable-service
	Command    string    `json:"command"`
	Items      []string  `json:"items,omitempty"`
	Status     string    `json:"status"` // pending | running | success | failed | skipped | dry-run
	DurationMs int64     `json:"duration_ms,omitempty"`
	ExitCode   int       `json:"exit_code,omitempty"`
	Stdout     string    `json:"stdout,omitempty"` // truncated to 4KB
	Stderr     string    `json:"stderr,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type JournalSummary struct {
	TotalEntries int
	Pending      int
	Running      int
	Success      int
	Failed       int
	Skipped      int
	DryRun       int
}

type Journal struct {
	Path  string
	RunID string
}

// GenerateRunID returns an 8-byte hex run ID.
func GenerateRunID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()%0xFFFFFFFF)
	}
	return hex.EncodeToString(b)
}

// DefaultJournalDir returns the standard run directory ~/.local/state/xpose/runs/.
func DefaultJournalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "xpose", "runs"), nil
}

// NewJournal creates a new Journal instance and initializes the file with 0600 permissions in dir with 0700.
func NewJournal(dir string) (*Journal, error) {
	if dir == "" {
		var err error
		dir, err = DefaultJournalDir()
		if err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create journal directory: %w", err)
	}

	runID := GenerateRunID()
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	filename := fmt.Sprintf("%s-%s.jsonl", timestamp, runID)
	filePath := filepath.Join(dir, filename)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create journal file: %w", err)
	}
	_ = f.Close()

	return &Journal{
		Path:  filePath,
		RunID: runID,
	}, nil
}

// OpenJournal creates a Journal handle pointing to an existing file.
func OpenJournal(path string) (*Journal, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return &Journal{Path: path}, nil
}

// Append writes an Entry to the journal file as a single JSON line.
func (j *Journal) Append(e Entry) error {
	if j == nil || j.Path == "" {
		return errors.New("invalid journal handle")
	}

	if e.RunID == "" && j.RunID != "" {
		e.RunID = j.RunID
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	// Truncate stdout/stderr to 4096 bytes if needed
	if len(e.Stdout) > 4096 {
		e.Stdout = e.Stdout[:4096] + "... [truncated]"
	}
	if len(e.Stderr) > 4096 {
		e.Stderr = e.Stderr[:4096] + "... [truncated]"
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal journal entry: %w", err)
	}

	f, err := os.OpenFile(j.Path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open journal for append: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, byte('\n'))); err != nil {
		return fmt.Errorf("failed to write journal entry: %w", err)
	}
	return nil
}

// ReadAll parses and returns all entries in the journal.
func (j *Journal) ReadAll() ([]Entry, error) {
	if j == nil || j.Path == "" {
		return nil, errors.New("invalid journal handle")
	}

	f, err := os.Open(j.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open journal for reading: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("invalid journal line: %w", err)
		}
		entries = append(entries, e)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading journal: %w", err)
	}

	return entries, nil
}

// FindIncomplete returns all entries with status "pending" or "running".
func (j *Journal) FindIncomplete() ([]Entry, error) {
	entries, err := j.ReadAll()
	if err != nil {
		return nil, err
	}

	var incomplete []Entry
	for _, e := range entries {
		if e.Status == "pending" || e.Status == "running" {
			incomplete = append(incomplete, e)
		}
	}
	return incomplete, nil
}

// Summarize aggregates statistics over all entries in the journal.
func (j *Journal) Summarize() (JournalSummary, error) {
	entries, err := j.ReadAll()
	if err != nil {
		return JournalSummary{}, err
	}

	var summary JournalSummary
	summary.TotalEntries = len(entries)
	for _, e := range entries {
		switch e.Status {
		case "pending":
			summary.Pending++
		case "running":
			summary.Running++
		case "success":
			summary.Success++
		case "failed":
			summary.Failed++
		case "skipped":
			summary.Skipped++
		case "dry-run":
			summary.DryRun++
		}
	}
	return summary, nil
}
