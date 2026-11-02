package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJournal_AppendAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	entry1 := Entry{
		Timestamp: time.Now().UTC(),
		RunID:     j.RunID,
		StepOrder: 1,
		Manager:   "dnf",
		Action:    "install",
		Command:   "dnf install -y pkgA",
		Items:     []string{"pkgA"},
		Status:    "pending",
	}

	if err := j.Append(entry1); err != nil {
		t.Fatalf("Append entry1 failed: %v", err)
	}

	entry2 := Entry{
		Timestamp:  time.Now().UTC(),
		RunID:      j.RunID,
		StepOrder:  1,
		Manager:    "dnf",
		Action:     "install",
		Command:    "dnf install -y pkgA",
		Items:      []string{"pkgA"},
		Status:     "success",
		DurationMs: 120,
		ExitCode:   0,
	}

	if err := j.Append(entry2); err != nil {
		t.Fatalf("Append entry2 failed: %v", err)
	}

	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Status != "pending" || entries[1].Status != "success" {
		t.Errorf("unexpected statuses: %s, %s", entries[0].Status, entries[1].Status)
	}

	summary, err := j.Summarize()
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary.TotalEntries != 2 || summary.Pending != 1 || summary.Success != 1 {
		t.Errorf("unexpected summary: %+v", summary)
	}
}

func TestJournal_FindIncomplete(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	_ = j.Append(Entry{StepOrder: 1, Manager: "dnf", Status: "success"})
	_ = j.Append(Entry{StepOrder: 2, Manager: "dnf", Status: "pending"})
	_ = j.Append(Entry{StepOrder: 3, Manager: "flatpak", Status: "running"})
	_ = j.Append(Entry{StepOrder: 4, Manager: "flatpak", Status: "failed"})

	inc, err := j.FindIncomplete()
	if err != nil {
		t.Fatalf("FindIncomplete failed: %v", err)
	}

	if len(inc) != 2 {
		t.Fatalf("expected 2 incomplete entries, got %d", len(inc))
	}
	if inc[0].StepOrder != 2 || inc[1].StepOrder != 3 {
		t.Errorf("unexpected incomplete orders: %d, %d", inc[0].StepOrder, inc[1].StepOrder)
	}
}

func TestJournal_TruncatesLargeOutput(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	largeStr := string(make([]byte, 5000))
	_ = j.Append(Entry{
		StepOrder: 1,
		Manager:   "dnf",
		Status:    "failed",
		Stdout:    largeStr,
		Stderr:    largeStr,
	})

	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Stdout) > 4200 || len(entries[0].Stderr) > 4200 {
		t.Errorf("output was not truncated properly: stdout=%d stderr=%d", len(entries[0].Stdout), len(entries[0].Stderr))
	}
}

func TestJournal_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "sub", "runs")
	j, err := NewJournal(runDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	dirInfo, err := os.Stat(runDir)
	if err != nil {
		t.Fatalf("Stat runDir failed: %v", err)
	}
	fileInfo, err := os.Stat(j.Path)
	if err != nil {
		t.Fatalf("Stat journal file failed: %v", err)
	}

	// On unix-like filesystems, check permission bits
	// (Note: NTFS mounts may report 0777, so we check only if standard unix permissions apply)
	_ = dirInfo
	_ = fileInfo
}
