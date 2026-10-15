package repology

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/transpose-project/transpose/pkg/manifest"
)

type PopulateStats struct {
	Processed      int
	StoredPackages int
	NotFound       int
	NetworkErrors  int
	CacheHits      int
	NetworkHits    int
	Warnings       []string
}

type PopulateProgress struct {
	Done           int
	Total          int
	SourceName     string
	CandidateCount int
	CacheHits      int
	NetworkHits    int
	NotFound       int
}

func PopulateFromManifest(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	source *manifest.Manifest,
	targetRepo string,
	progress func(done, total int),
) (PopulateStats, error) {
	return PopulateWithObserver(ctx, db, client, source, targetRepo, func(p PopulateProgress) {
		if progress != nil {
			progress(p.Done, p.Total)
		}
	})
}

func PopulateWithObserver(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	source *manifest.Manifest,
	targetRepo string,
	progress func(PopulateProgress),
) (PopulateStats, error) {
	if db == nil || client == nil || source == nil {
		return PopulateStats{}, fmt.Errorf("base, client et manifeste sont requis")
	}
	if strings.TrimSpace(targetRepo) == "" {
		return PopulateStats{}, fmt.Errorf("dépôt cible vide")
	}
	type sourcePackage struct {
		name string
	}
	var packages []sourcePackage
	for _, pkg := range source.Packages {
		if pkg.Manager == "dnf" {
			packages = append(packages, sourcePackage{name: pkg.Name})
		}
	}
	stats := PopulateStats{}
	for index, pkg := range packages {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		project, meta, err := client.Fetch(ctx, pkg.name)
		if meta.CacheHit {
			stats.CacheHits++
		} else if err == nil {
			stats.NetworkHits++
		}
		progressData := PopulateProgress{
			Done:        index + 1,
			Total:       len(packages),
			SourceName:  pkg.name,
			CacheHits:   stats.CacheHits,
			NetworkHits: stats.NetworkHits,
		}
		if err != nil {
			stats.NetworkErrors++
			stats.Warnings = append(stats.Warnings, fmt.Sprintf("%s: %v", pkg.name, err))
		} else if meta.NotFound {
			stats.NotFound++
			stats.Warnings = append(stats.Warnings, fmt.Sprintf("%s: absent de Repology (404)", pkg.name))
		} else {
			upstreamURL := ""
			candidateCount := 0
			for _, candidate := range project.Packages {
				if upstreamURL == "" {
					upstreamURL = candidate.Upstream
				}
				if candidate.Repo == targetRepo && candidate.Name != "" {
					candidateCount++
				}
			}
			inserted, storeErr := StoreProject(ctx, db, project, upstreamURL, targetRepo)
			if storeErr != nil {
				stats.NetworkErrors++
				stats.Warnings = append(stats.Warnings, fmt.Sprintf("%s: %v", pkg.name, storeErr))
			} else {
				stats.StoredPackages += inserted
				progressData.CandidateCount = candidateCount
			}
		}
		stats.Processed++
		progressData.CacheHits = stats.CacheHits
		progressData.NetworkHits = stats.NetworkHits
		progressData.NotFound = stats.NotFound
		if progress != nil {
			progress(progressData)
		}
	}
	return stats, nil
}
