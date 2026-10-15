package resolver

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type Resolution struct {
	Index         int
	SourceName    string
	SourceVersion string
	TargetName    string
	TargetVersion string
	Status        string
	Confidence    string
	Notes         string
}

var CSVHeader = []string{
	"index", "source_name", "source_version", "target_name", "target_version", "status", "confidence", "notes",
}

func ResolveManifest(ctx context.Context, db *sql.DB, source *manifest.Manifest, targetRepo string, limit int) ([]Resolution, error) {
	if db == nil || source == nil {
		return nil, fmt.Errorf("base et manifeste sont requis")
	}
	if strings.TrimSpace(targetRepo) == "" {
		return nil, fmt.Errorf("dépôt cible vide")
	}
	packages := source.Packages
	if limit > 0 && limit < len(packages) {
		packages = packages[:limit]
	}
	resolutions := make([]Resolution, 0, len(packages))
	for index, sourcePackage := range packages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidates, upstreamURL, err := repology.QueryByProject(ctx, db, sourcePackage.Name, targetRepo)
		if err != nil {
			return nil, err
		}
		resolution := Resolution{
			Index:         index + 1,
			SourceName:    sourcePackage.Name,
			SourceVersion: sourcePackage.Version,
		}
		switch len(candidates) {
		case 0:
			resolution.Status = "not_found"
			resolution.Confidence = "low"
			resolution.Notes = "0 candidats"
			if upstreamURL != "" {
				resolution.Notes += "; upstream: " + upstreamURL
			}
			resolution.Notes += "; aucun paquet cible pour le project_name"
		case 1:
			resolution.TargetName = candidates[0].Name
			resolution.TargetVersion = candidates[0].Version
			if strings.EqualFold(sourcePackage.Name, candidates[0].Name) {
				resolution.Status = "exact"
				resolution.Confidence = "high"
			} else {
				resolution.Status = "equivalent"
				resolution.Confidence = "medium"
			}
			resolution.Notes = "1 candidat"
			if upstreamURL == "" {
				upstreamURL = candidates[0].UpstreamURL
			}
			if upstreamURL != "" {
				resolution.Notes += "; upstream: " + upstreamURL
			}
		default:
			resolution.Status = "ambiguous"
			resolution.Confidence = "low"
			resolution.Notes = fmt.Sprintf("%d candidats", len(candidates))
			if upstreamURL != "" {
				resolution.Notes += "; upstream: " + upstreamURL
			}
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

func WriteCSV(writer io.Writer, resolutions []Resolution) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(CSVHeader); err != nil {
		return fmt.Errorf("écriture en-tête CSV: %w", err)
	}
	for _, resolution := range resolutions {
		record := []string{
			fmt.Sprintf("%d", resolution.Index),
			resolution.SourceName,
			resolution.SourceVersion,
			resolution.TargetName,
			resolution.TargetVersion,
			resolution.Status,
			resolution.Confidence,
			resolution.Notes,
		}
		if err := csvWriter.Write(record); err != nil {
			return fmt.Errorf("écriture ligne CSV %d: %w", resolution.Index, err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("finalisation CSV: %w", err)
	}
	return nil
}
