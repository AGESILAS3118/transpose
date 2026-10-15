package resolver

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type Resolution struct {
	Index            int
	SourceName       string
	SourceVersion    string
	TargetName       string
	TargetVersion    string
	Status           string
	Confidence       string
	Notes            string
	ProjectNames     []string
	TargetCandidates []string
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
		resolution := Resolution{
			Index:         index + 1,
			SourceName:    sourcePackage.Name,
			SourceVersion: sourcePackage.Version,
		}
		candidates, upstreamURL, matchedProject, err := queryCandidateNames(ctx, db, sourcePackage.Name, targetRepo)
		if err != nil {
			return nil, err
		}
		resolution.ProjectNames = CandidateNames(sourcePackage.Name)
		switch len(candidates) {
		case 0:
			fuzzy, fuzzyErr := repology.SearchPackages(ctx, db, targetRepo, fuzzySearchTerms(sourcePackage.Name), 3)
			if fuzzyErr != nil {
				return nil, fuzzyErr
			}
			if len(fuzzy) > 0 {
				candidates = fuzzy
				resolution.TargetName = fuzzy[0].Name
				resolution.TargetVersion = fuzzy[0].Version
				for _, candidate := range fuzzy {
					resolution.TargetCandidates = append(resolution.TargetCandidates, candidate.Name)
				}
				if len(fuzzy) == 1 {
					resolution.Status = "equivalent"
					resolution.Confidence = "medium"
					resolution.Notes = "fuzzy match; 1 candidat (recherche approximative)"
				} else {
					resolution.Status = "ambiguous"
					resolution.Confidence = "low"
					resolution.Notes = fmt.Sprintf("%d candidats fuzzy (recherche approximative)", len(fuzzy))
				}
			} else {
				resolution.Status = "not_found"
				resolution.Confidence = "low"
				resolution.Notes = "0 candidats"
				if upstreamURL != "" {
					resolution.Notes += "; upstream: " + upstreamURL
				}
				resolution.Notes += "; aucun paquet cible pour les project_name candidats"
			}
		case 1:
			resolution.TargetName = candidates[0].Name
			resolution.TargetVersion = candidates[0].Version
			resolution.TargetCandidates = []string{candidates[0].Name}
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
			if !strings.EqualFold(matchedProject, sourcePackage.Name) {
				resolution.Notes += "; project_name: " + matchedProject
			}
		default:
			resolution.Status = "ambiguous"
			resolution.Confidence = "low"
			resolution.Notes = fmt.Sprintf("%d candidats", len(candidates))
			for _, candidate := range candidates {
				resolution.TargetCandidates = append(resolution.TargetCandidates, candidate.Name)
			}
			if upstreamURL != "" {
				resolution.Notes += "; upstream: " + upstreamURL
			}
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

func queryCandidateNames(ctx context.Context, db *sql.DB, sourceName, targetRepo string) ([]repology.StoredPackage, string, string, error) {
	names := CandidateNames(sourceName)
	for _, name := range names {
		candidates, upstreamURL, err := repology.QueryByProject(ctx, db, name, targetRepo)
		if err != nil {
			return nil, "", "", err
		}
		if len(candidates) > 0 {
			return candidates, upstreamURL, name, nil
		}
	}
	return nil, "", "", nil
}

func fuzzySearchTerms(source string) []string {
	names := CandidateNames(source)
	terms := make([]string, 0, len(names))
	for _, name := range names {
		if len(name) >= 3 {
			terms = append(terms, name)
		}
	}
	sort.SliceStable(terms, func(i, j int) bool {
		return len(terms[i]) > len(terms[j])
	})
	return terms
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
