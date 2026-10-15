package repology

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

type rankedPackage struct {
	pkg      StoredPackage
	distance int
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

func CountPackagesInRepo(ctx context.Context, db *sql.DB, repo string) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages WHERE repo = ?", repo).Scan(&count); err != nil {
		return 0, fmt.Errorf("comptage paquets Repology du dépôt %q: %w", repo, err)
	}
	return count, nil
}

func SearchPackages(ctx context.Context, db *sql.DB, targetRepo string, terms []string, limit int) ([]StoredPackage, error) {
	if limit <= 0 {
		return nil, errors.New("limite de recherche fuzzy doit être positif")
	}
	ranked := make(map[string]rankedPackage)
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if len(term) < 3 {
			continue
		}
		for _, gram := range searchGrams(term) {
			rows, err := db.QueryContext(ctx, `
				SELECT project_name, repo, name, version, category, maintainer
				FROM packages
				WHERE repo = ? AND name LIKE ? ESCAPE '\'
				LIMIT 500
			`, targetRepo, "%"+escapeLike(gram)+"%")
			if err != nil {
				return nil, fmt.Errorf("recherche fuzzy %q: %w", gram, err)
			}
			for rows.Next() {
				var pkg StoredPackage
				if err := rows.Scan(&pkg.ProjectName, &pkg.Repo, &pkg.Name, &pkg.Version, &pkg.Category, &pkg.Maintainer); err != nil {
					rows.Close()
					return nil, fmt.Errorf("lecture résultat fuzzy: %w", err)
				}
				pkgDistance := levenshtein(strings.ToLower(term), strings.ToLower(pkg.Name))
				maxDistance := len(term) / 4
				if maxDistance < 1 {
					maxDistance = 1
				}
				if pkgDistance > maxDistance {
					continue
				}
				key := pkg.ProjectName + "\x00" + pkg.Name + "\x00" + pkg.Version
				if previous, ok := ranked[key]; !ok || pkgDistance < previous.distance {
					ranked[key] = rankedPackage{pkg: pkg, distance: pkgDistance}
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("itération résultats fuzzy: %w", err)
			}
			if err := rows.Close(); err != nil {
				return nil, fmt.Errorf("fermeture résultats fuzzy: %w", err)
			}
		}
	}
	results := make([]rankedPackage, 0, len(ranked))
	for _, candidate := range ranked {
		results = append(results, candidate)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].distance == results[j].distance {
			if results[i].pkg.Name == results[j].pkg.Name {
				return results[i].pkg.ProjectName < results[j].pkg.ProjectName
			}
			return results[i].pkg.Name < results[j].pkg.Name
		}
		return results[i].distance < results[j].distance
	})
	if len(results) > limit {
		results = results[:limit]
	}
	packages := make([]StoredPackage, len(results))
	for index, result := range results {
		packages[index] = result.pkg
	}
	return packages, nil
}

func searchGrams(value string) []string {
	runes := []rune(value)
	gramSize := 4
	if len(runes) < gramSize {
		gramSize = len(runes)
	}
	grams := make([]string, 0, len(runes)-gramSize+1)
	seen := make(map[string]struct{})
	for index := 0; index+gramSize <= len(runes); index++ {
		gram := string(runes[index : index+gramSize])
		if _, exists := seen[gram]; exists {
			continue
		}
		seen[gram] = struct{}{}
		grams = append(grams, gram)
	}
	return grams
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func levenshtein(left, right string) int {
	leftRunes, rightRunes := []rune(left), []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for i, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = i + 1
		for j, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			deletion := previous[j+1] + 1
			insertion := current[j] + 1
			substitution := previous[j] + cost
			current[j+1] = min(deletion, insertion, substitution)
		}
		previous = current
	}
	return previous[len(right)]
}

func min(values ...int) int {
	value := values[0]
	for _, candidate := range values[1:] {
		if candidate < value {
			value = candidate
		}
	}
	return value
}
