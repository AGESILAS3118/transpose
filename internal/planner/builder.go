package planner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/adapter/flatpak"
	repoadapter "github.com/transpose-project/transpose/internal/adapter/repos"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func BuildManifest(results []*adapter.CaptureResult) (*manifest.Manifest, error) {
	m, _, err := BuildManifestWithCollectors(results, defaultRepoCollectors())
	return m, err
}

func BuildManifestWithCollectors(results []*adapter.CaptureResult, collectors []repoadapter.RepoCollector) (*manifest.Manifest, []string, error) {
	osRelease, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, nil, fmt.Errorf("lecture /etc/os-release: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		_ = osRelease.Close()
		return nil, nil, fmt.Errorf("lecture du hostname: %w", err)
	}
	m, warnings, buildErr := BuildManifestWithRepoCollectors(
		results, osRelease, hostname, time.Now().UTC(), collectors,
	)
	if err := osRelease.Close(); err != nil {
		return nil, nil, errors.Join(buildErr, fmt.Errorf("fermeture de /etc/os-release: %w", err))
	}
	return m, warnings, buildErr
}

func BuildManifestWithSource(results []*adapter.CaptureResult, osRelease io.Reader, hostname string, capturedAt time.Time) (*manifest.Manifest, error) {
	m, _, err := BuildManifestWithRepoCollectors(results, osRelease, hostname, capturedAt, nil)
	return m, err
}

func BuildManifestWithRepoCollectors(
	results []*adapter.CaptureResult,
	osRelease io.Reader,
	hostname string,
	capturedAt time.Time,
	collectors []repoadapter.RepoCollector,
) (*manifest.Manifest, []string, error) {
	sourceData, err := parseOSRelease(osRelease)
	if err != nil {
		return nil, nil, err
	}
	m := &manifest.Manifest{
		Schema: manifest.SchemaVersion,
		Source: manifest.Source{
			Distro:       sourceData["ID"],
			Version:      sourceData["VERSION_ID"],
			Arch:         runtime.GOARCH,
			Hostname:     hostname,
			CapturedAt:   capturedAt.UTC(),
			CaptureLevel: make(map[string]int, len(results)),
		},
		Repos:    []manifest.Repo{},
		Packages: []manifest.Package{},
		Flatpak: manifest.FlatpakBlock{
			Remotes:   []manifest.FlatpakRemote{},
			Apps:      []manifest.FlatpakApp{},
			Overrides: []manifest.FlatpakOverride{},
		},
	}
	for index, result := range results {
		if result == nil {
			return nil, nil, fmt.Errorf("résultat de capture %d nil", index)
		}
		if result.Manager == "" {
			return nil, nil, fmt.Errorf("nom de gestionnaire vide pour le résultat %d", index)
		}
		if result.Level < 1 || result.Level > 3 {
			return nil, nil, fmt.Errorf("niveau de capture invalide pour %s : %d", result.Manager, result.Level)
		}
		m.Source.CaptureLevel[result.Manager] = int(result.Level)
		if result.Manager == "flatpak" {
			if err := addFlatpakData(&m.Flatpak, result.Raw); err != nil {
				return nil, nil, fmt.Errorf("conversion des données Flatpak: %w", err)
			}
			continue
		}
		for _, pkg := range result.Packages {
			m.Packages = append(m.Packages, manifest.Package{
				Name:    pkg.Name,
				Manager: result.Manager,
				Version: pkg.Version,
			})
		}
	}
	activeRepoManager := ""
	switch sysutil.FamilyForDistro(sourceData["ID"], sourceData["ID_LIKE"]) {
	case "rpm":
		activeRepoManager = "rpm"
	case "debian":
		activeRepoManager = "apt"
	}
	var repoWarnings []string
	for _, collector := range collectors {
		if collector == nil || collector.Name() != activeRepoManager {
			continue
		}
		present, err := collector.Detect()
		if err != nil {
			return nil, nil, fmt.Errorf("détection des dépôts %s impossible: %w", collector.Name(), err)
		}
		if !present {
			continue
		}
		collected, err := collector.Collect(context.Background())
		if err != nil {
			return nil, nil, fmt.Errorf("collecte des dépôts %s impossible: %w", collector.Name(), err)
		}
		m.Repos = append(m.Repos, collected...)
		m.Source.CaptureLevel["repos_"+collector.Name()] = 1
		repoWarnings = append(repoWarnings, repoadapter.Warnings(collector)...)
	}
	sort.Slice(m.Repos, func(i, j int) bool {
		if m.Repos[i].Kind == m.Repos[j].Kind {
			return m.Repos[i].Name < m.Repos[j].Name
		}
		return m.Repos[i].Kind < m.Repos[j].Kind
	})
	sort.Slice(m.Packages, func(i, j int) bool {
		if m.Packages[i].Manager == m.Packages[j].Manager {
			return m.Packages[i].Name < m.Packages[j].Name
		}
		return m.Packages[i].Manager < m.Packages[j].Manager
	})
	sort.Slice(m.Flatpak.Remotes, func(i, j int) bool {
		if m.Flatpak.Remotes[i].Scope == m.Flatpak.Remotes[j].Scope {
			return m.Flatpak.Remotes[i].Name < m.Flatpak.Remotes[j].Name
		}
		return m.Flatpak.Remotes[i].Scope < m.Flatpak.Remotes[j].Scope
	})
	sort.Slice(m.Flatpak.Apps, func(i, j int) bool {
		if m.Flatpak.Apps[i].Scope == m.Flatpak.Apps[j].Scope {
			return m.Flatpak.Apps[i].Ref < m.Flatpak.Apps[j].Ref
		}
		return m.Flatpak.Apps[i].Scope < m.Flatpak.Apps[j].Scope
	})
	sort.Slice(m.Flatpak.Overrides, func(i, j int) bool {
		if m.Flatpak.Overrides[i].Scope == m.Flatpak.Overrides[j].Scope {
			return m.Flatpak.Overrides[i].Ref < m.Flatpak.Overrides[j].Ref
		}
		return m.Flatpak.Overrides[i].Scope < m.Flatpak.Overrides[j].Scope
	})
	if m.Source.Distro == "" {
		return nil, nil, errors.New("ID absent de /etc/os-release")
	}
	return m, repoWarnings, nil
}

func defaultRepoCollectors() []repoadapter.RepoCollector {
	return []repoadapter.RepoCollector{
		repoadapter.NewRPMRepoCollector(""),
		repoadapter.NewAPTRepoCollector(""),
	}
}

func parseOSRelease(input io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lecture os-release: %w", err)
	}
	return values, nil
}

func addFlatpakData(block *manifest.FlatpakBlock, raw map[string]any) error {
	if raw == nil {
		return nil
	}
	if remotes, ok := raw["remotes"]; ok {
		if err := appendFlatpakRemotes(block, remotes); err != nil {
			return err
		}
	}
	if apps, ok := raw["apps"]; ok {
		if err := appendFlatpakApps(block, apps); err != nil {
			return err
		}
	}
	if overrides, ok := raw["overrides"]; ok {
		if err := appendFlatpakOverrides(block, overrides); err != nil {
			return err
		}
	}
	return nil
}

func appendFlatpakRemotes(block *manifest.FlatpakBlock, raw any) error {
	switch remotes := raw.(type) {
	case map[string][]flatpak.Remote:
		for scope, scoped := range remotes {
			for _, remote := range scoped {
				block.Remotes = append(block.Remotes, manifest.FlatpakRemote{Name: remote.Name, URL: remote.URL, Scope: scope})
			}
		}
	case []flatpak.Remote:
		for _, remote := range remotes {
			block.Remotes = append(block.Remotes, manifest.FlatpakRemote{Name: remote.Name, URL: remote.URL, Scope: "system"})
		}
	default:
		return fmt.Errorf("type Raw[\"remotes\"] non pris en charge: %T", raw)
	}
	return nil
}

func appendFlatpakApps(block *manifest.FlatpakBlock, raw any) error {
	switch apps := raw.(type) {
	case []flatpak.App:
		for _, app := range apps {
			block.Apps = append(block.Apps, manifest.FlatpakApp{Ref: app.Ref, Scope: app.Scope})
		}
	case []adapter.Package:
		for _, app := range apps {
			block.Apps = append(block.Apps, manifest.FlatpakApp{Ref: app.Name, Scope: "system"})
		}
	default:
		return fmt.Errorf("type Raw[\"apps\"] non pris en charge: %T", raw)
	}
	return nil
}

func appendFlatpakOverrides(block *manifest.FlatpakBlock, raw any) error {
	overrides, ok := raw.(map[string]string)
	if !ok {
		return fmt.Errorf("type Raw[\"overrides\"] non pris en charge: %T", raw)
	}
	for scope, content := range overrides {
		if strings.TrimSpace(content) == "" {
			continue
		}
		args := strings.Fields(content)
		block.Overrides = append(block.Overrides, manifest.FlatpakOverride{Scope: scope, Args: args})
	}
	return nil
}
