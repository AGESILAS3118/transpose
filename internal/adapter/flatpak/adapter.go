package flatpak

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/transpose-project/transpose/internal/adapter"
)

const (
	defaultSystemAppDir = "/var/lib/flatpak/app"
	defaultUserAppDir   = ".local/share/flatpak/app"
)

type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Adapter struct {
	run          adapter.CommandRunner
	systemAppDir string
	userAppDir   string
}

func New(run adapter.CommandRunner) *Adapter {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return NewWithAppDirs(run, defaultSystemAppDir, filepath.Join(home, defaultUserAppDir))
}

func NewWithAppDirs(run adapter.CommandRunner, systemAppDir, userAppDir string) *Adapter {
	if run == nil {
		run = adapter.ExecCommandRunner
	}
	return &Adapter{run: run, systemAppDir: systemAppDir, userAppDir: userAppDir}
}

func (a *Adapter) Name() string { return "flatpak" }

func (a *Adapter) Detect() (bool, error) {
	_, err := a.run("flatpak", "--version")
	return err == nil, nil
}

func (a *Adapter) Health() (adapter.HealthStatus, error) {
	status := adapter.HealthStatus{Manager: a.Name()}
	_, versionErr := a.run("flatpak", "--version")
	status.Present = versionErr == nil
	if !status.Present {
		status.Message = "absent"
		return status, nil
	}
	output, err := a.run("flatpak", "list", "--app", "--columns=ref,version,origin,installation")
	if err != nil {
		status.Message = "flatpak présent mais inutilisable: " + err.Error()
		return status, nil
	}
	status.ItemCount = len(parseList(output))
	status.Functional = true
	status.Message = "ok"
	return status, nil
}

func (a *Adapter) Capture() (*adapter.CaptureResult, error) {
	result := &adapter.CaptureResult{Manager: a.Name(), Raw: make(map[string]any)}
	listOutput, listErr := a.run("flatpak", "list", "--app", "--columns=ref,version,origin,installation")
	if listErr == nil {
		systemRemotes, systemRemoteErr := a.readRemotes("--system")
		userRemotes, userRemoteErr := a.readRemotes("--user")
		systemOverrides, systemOverrideErr := a.run("flatpak", "override", "--show", "--system")
		userOverrides, userOverrideErr := a.run("flatpak", "override", "--show", "--user")
		for _, err := range []error{systemRemoteErr, userRemoteErr, systemOverrideErr, userOverrideErr} {
			if err != nil {
				listErr = errors.Join(listErr, err)
			}
		}
		if listErr == nil {
			packages := parseList(listOutput)
			result.Level = adapter.Level1
			result.Reliability = adapter.ReliabilityHigh
			result.Packages = packages
			result.Raw["remotes"] = map[string][]Remote{
				"system": systemRemotes,
				"user":   userRemotes,
			}
			result.Raw["overrides"] = map[string]string{
				"system": string(systemOverrides),
				"user":   string(userOverrides),
			}
			result.Warnings = remoteDivergenceWarnings(systemRemotes, userRemotes)
			return result, nil
		}
	}

	result.Warnings = append(result.Warnings, "niveau 1 flatpak indisponible : "+listErr.Error()+" — bascule au niveau 2")
	packages, scanErr := a.scanApps()
	if scanErr != nil {
		return nil, fmt.Errorf("capture flatpak impossible: niveau 1: %v; niveau 2: %w", listErr, scanErr)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("capture flatpak impossible: aucun ref trouvé après échec du niveau 1 (%v)", listErr)
	}
	result.Level = adapter.Level2
	result.Reliability = adapter.ReliabilityLow
	result.Packages = packages
	result.Raw["refs"] = packages
	return result, nil
}

func (a *Adapter) readRemotes(scope string) ([]Remote, error) {
	output, err := a.run("flatpak", "remotes", scope, "--columns=name,url")
	if err != nil {
		return nil, fmt.Errorf("lecture des remotes %s: %w", scope, err)
	}
	var remotes []Remote
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		remote := Remote{Name: fields[0]}
		if len(fields) == 2 {
			remote.URL = fields[1]
		}
		remotes = append(remotes, remote)
	}
	return remotes, nil
}

func (a *Adapter) scanApps() ([]adapter.Package, error) {
	dirs := []string{a.systemAppDir, a.userAppDir}
	var packages []adapter.Package
	seen := make(map[string]struct{})
	var scanErrors []error
	readableDirs := 0
	for _, root := range dirs {
		entries, err := os.ReadDir(root)
		if err != nil {
			if !os.IsNotExist(err) {
				scanErrors = append(scanErrors, fmt.Errorf("%s: %w", root, err))
			}
			continue
		}
		readableDirs++
		for _, app := range entries {
			if !app.IsDir() {
				continue
			}
			arches, err := os.ReadDir(filepath.Join(root, app.Name()))
			if err != nil {
				scanErrors = append(scanErrors, fmt.Errorf("%s: %w", filepath.Join(root, app.Name()), err))
				continue
			}
			for _, arch := range arches {
				if !arch.IsDir() {
					continue
				}
				branches, err := os.ReadDir(filepath.Join(root, app.Name(), arch.Name()))
				if err != nil {
					scanErrors = append(scanErrors, fmt.Errorf("%s: %w", filepath.Join(root, app.Name(), arch.Name()), err))
					continue
				}
				for _, branch := range branches {
					if !branch.IsDir() {
						continue
					}
					ref := strings.Join([]string{"app", app.Name(), arch.Name(), branch.Name()}, "/")
					if _, ok := seen[ref]; ok {
						continue
					}
					seen[ref] = struct{}{}
					packages = append(packages, adapter.Package{Name: ref, Source: adapter.Level2})
				}
			}
		}
	}
	if readableDirs == 0 {
		return nil, errors.Join(scanErrors...)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	return packages, errors.Join(scanErrors...)
}

func parseList(output []byte) []adapter.Package {
	var packages []adapter.Package
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		ref := strings.TrimSpace(fields[0])
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		version := ""
		if len(fields) > 1 {
			version = strings.TrimSpace(fields[1])
		}
		packages = append(packages, adapter.Package{Name: ref, Version: version, Source: adapter.Level1})
	}
	return packages
}

func remoteDivergenceWarnings(systemRemotes, userRemotes []Remote) []string {
	systemNames := make(map[string]struct{}, len(systemRemotes))
	userNames := make(map[string]struct{}, len(userRemotes))
	for _, remote := range systemRemotes {
		systemNames[remote.Name] = struct{}{}
	}
	for _, remote := range userRemotes {
		userNames[remote.Name] = struct{}{}
	}
	var warnings []string
	for name := range systemNames {
		if _, ok := userNames[name]; !ok {
			warnings = append(warnings, fmt.Sprintf("divergence des portées Flatpak : remote %q présent en système mais absent en utilisateur", name))
		}
	}
	for name := range userNames {
		if _, ok := systemNames[name]; !ok {
			warnings = append(warnings, fmt.Sprintf("divergence des portées Flatpak : remote %q présent en utilisateur mais absent en système", name))
		}
	}
	sort.Strings(warnings)
	return warnings
}
