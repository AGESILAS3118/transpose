package repology

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type StoredPackage struct {
	ProjectName string
	Repo        string
	Name        string
	Version     string
	Category    string
	Maintainer  string
	UpstreamURL string
}

func OpenDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("chemin de base Repology vide")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("création du dossier de la base Repology: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("ouverture base Repology %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("activation des clés étrangères SQLite: %w", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("base Repology nil")
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("migration schéma Repology: %w", err)
	}
	return nil
}

func StoreProject(ctx context.Context, db *sql.DB, project *Project, upstreamURL string, targetRepo string) (int, error) {
	if project == nil {
		return 0, errors.New("projet Repology nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("transaction Repology: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects(name, upstream_url, last_seen)
		VALUES(?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			upstream_url = CASE WHEN excluded.upstream_url <> '' THEN excluded.upstream_url ELSE projects.upstream_url END,
			last_seen = CURRENT_TIMESTAMP
	`, project.Name, upstreamURL); err != nil {
		return 0, fmt.Errorf("enregistrement projet %q: %w", project.Name, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM packages WHERE project_name = ? AND repo = ?", project.Name, targetRepo); err != nil {
		return 0, fmt.Errorf("actualisation paquets %q: %w", project.Name, err)
	}
	inserted := 0
	for _, pkg := range project.Packages {
		if pkg.Repo != targetRepo || pkg.Name == "" {
			continue
		}
		result, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO packages(project_name, repo, name, version, category, maintainer)
			VALUES(?, ?, ?, ?, ?, ?)
		`, project.Name, pkg.Repo, pkg.Name, pkg.Version, pkg.Category, pkg.Maintainer)
		if err != nil {
			return 0, fmt.Errorf("enregistrement paquet %q/%q: %w", project.Name, pkg.Name, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("comptage insertion paquet %q: %w", pkg.Name, err)
		}
		inserted += int(count)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("validation transaction projet %q: %w", project.Name, err)
	}
	return inserted, nil
}

func QueryByProject(ctx context.Context, db *sql.DB, projectName, targetRepo string) ([]StoredPackage, string, error) {
	var upstreamURL string
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(upstream_url, '') FROM projects WHERE name = ?", projectName).Scan(&upstreamURL); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("lecture URL upstream %q: %w", projectName, err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT project_name, repo, name, version, category, maintainer
		FROM packages WHERE project_name = ? AND repo = ? ORDER BY name, version
	`, projectName, targetRepo)
	if err != nil {
		return nil, "", fmt.Errorf("recherche paquet projet %q: %w", projectName, err)
	}
	defer rows.Close()
	var packages []StoredPackage
	for rows.Next() {
		var pkg StoredPackage
		if err := rows.Scan(&pkg.ProjectName, &pkg.Repo, &pkg.Name, &pkg.Version, &pkg.Category, &pkg.Maintainer); err != nil {
			return nil, "", fmt.Errorf("lecture paquet projet %q: %w", projectName, err)
		}
		pkg.UpstreamURL = upstreamURL
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("itération paquets projet %q: %w", projectName, err)
	}
	return packages, upstreamURL, nil
}

func CountPackages(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages").Scan(&count); err != nil {
		return 0, fmt.Errorf("comptage base Repology: %w", err)
	}
	return count, nil
}
