package repology

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type DumpRecord struct {
	ProjectName string `json:"project_name"`
	UpstreamURL string `json:"upstream_url"`
	Repo        string `json:"repo"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Category    string `json:"category"`
	Maintainer  string `json:"maintainer"`
}

func ImportDump(ctx context.Context, db *sql.DB, input io.Reader, targetRepo string) (int, error) {
	if db == nil || input == nil {
		return 0, fmt.Errorf("base et dump sont requis")
	}
	if strings.TrimSpace(targetRepo) == "" {
		return 0, fmt.Errorf("dépôt cible vide")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("transaction import dump: %w", err)
	}
	defer tx.Rollback()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	line := 0
	imported := 0
	for scanner.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return imported, err
		}
		var record DumpRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return imported, fmt.Errorf("dump JSONL ligne %d: %w", line, err)
		}
		if record.Repo != targetRepo {
			continue
		}
		if record.ProjectName == "" || record.Name == "" {
			return imported, fmt.Errorf("dump JSONL ligne %d: project_name et name sont requis", line)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO projects(name, upstream_url, last_seen)
			VALUES(?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(name) DO UPDATE SET
				upstream_url = CASE WHEN excluded.upstream_url <> '' THEN excluded.upstream_url ELSE projects.upstream_url END,
				last_seen = CURRENT_TIMESTAMP
		`, record.ProjectName, record.UpstreamURL); err != nil {
			return imported, fmt.Errorf("dump JSONL ligne %d, projet %q: %w", line, record.ProjectName, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO packages(project_name, repo, name, version, category, maintainer)
			VALUES(?, ?, ?, ?, ?, ?)
		`, record.ProjectName, record.Repo, record.Name, record.Version, record.Category, record.Maintainer); err != nil {
			return imported, fmt.Errorf("dump JSONL ligne %d, paquet %q: %w", line, record.Name, err)
		}
		imported++
	}
	if err := scanner.Err(); err != nil {
		return imported, fmt.Errorf("lecture dump JSONL: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return imported, fmt.Errorf("validation transaction import dump: %w", err)
	}
	return imported, nil
}
