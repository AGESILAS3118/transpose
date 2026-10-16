package repos

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpose-project/transpose/pkg/manifest"
)

const defaultRPMRepoDirectory = "/etc/yum.repos.d"

type RPMRepoCollector struct {
	Directory string
	warnings  []string
}

func NewRPMRepoCollector(directory string) *RPMRepoCollector {
	if directory == "" {
		directory = defaultRPMRepoDirectory
	}
	return &RPMRepoCollector{Directory: directory}
}

func (c *RPMRepoCollector) Name() string { return "rpm" }

func (c *RPMRepoCollector) Warnings() []string {
	return append([]string(nil), c.warnings...)
}

func (c *RPMRepoCollector) Detect() (bool, error) {
	info, err := os.Stat(c.Directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lecture du dossier de dépôts RPM %s: %w", c.Directory, err)
	}
	return info.IsDir(), nil
}

func (c *RPMRepoCollector) Collect(ctx context.Context) ([]manifest.Repo, error) {
	c.warnings = nil
	files, err := filepath.Glob(filepath.Join(c.Directory, "*.repo"))
	if err != nil {
		return nil, fmt.Errorf("recherche des fichiers .repo: %w", err)
	}
	disabledFiles, err := filepath.Glob(filepath.Join(c.Directory, "*.repo.disabled"))
	if err != nil {
		return nil, fmt.Errorf("recherche des fichiers .repo.disabled: %w", err)
	}
	files = append(files, disabledFiles...)
	repos := make([]manifest.Repo, 0)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("lecture du dépôt RPM %s: %w", path, err)
		}
		parsed, warnings := ParseRPMRepoFile(filepath.Base(path), string(content))
		for _, warning := range warnings {
			c.warnings = append(c.warnings, fmt.Sprintf("%s: %s", filepath.Base(path), warning))
		}
		for i := range parsed {
			parsed[i].Content = sanitizeContent(parsed[i].Content)
			if parsed[i].GPGKeyURL != "" {
				if checksum, ok := localGPGChecksum(parsed[i].GPGKeyURL); ok {
					parsed[i].GPGKeySHA256 = checksum
				}
			}
			repos = append(repos, parsed[i])
		}
	}
	return repos, nil
}

func ParseRPMRepoFile(fileName, content string) ([]manifest.Repo, []string) {
	sections, warnings := parseINI(content)
	result := make([]manifest.Repo, 0, len(sections))
	for _, section := range sections {
		values := section.values
		urlValue := firstNonEmpty(values["baseurl"], values["metalink"], values["mirrorlist"])
		enabled := true
		if raw, ok := values["enabled"]; ok {
			enabled = strings.TrimSpace(raw) != "0" && !strings.EqualFold(strings.TrimSpace(raw), "false")
		}
		if strings.HasSuffix(strings.ToLower(fileName), ".repo.disabled") {
			enabled = false
		}
		repo := manifest.Repo{
			Kind:       "rpm",
			Name:       section.name,
			URL:        sanitizeReference(firstURL(urlValue)),
			FileName:   fileName,
			Content:    sanitizeContent(content),
			Enabled:    enabled,
			ThirdParty: !officialRPMURL(urlValue),
			GPGKeyURL:  sanitizeReference(gpgKeyReference(values["gpgkey"])),
		}
		if strings.TrimSpace(values["name"]) != "" {
			repo.Extra = map[string]any{"display_name": strings.TrimSpace(values["name"])}
		}
		for _, rawURL := range strings.Fields(urlValue) {
			if strings.HasPrefix(strings.ToLower(rawURL), "file://") ||
				(strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://")) {
				warnings = append(warnings, fmt.Sprintf("dépôt %s utilise une URL non sécurisée : %s", section.name, rawURL))
			}
		}
		result = append(result, repo)
	}
	return result, warnings
}

type iniSection struct {
	name   string
	values map[string]string
}

func parseINI(content string) ([]iniSection, []string) {
	sections := make([]iniSection, 0)
	var current *iniSection
	var warnings []string
	scanner := strings.Split(content, "\n")
	for lineNumber, line := range scanner {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && len(line) > 2 {
			sections = append(sections, iniSection{name: strings.TrimSpace(line[1 : len(line)-1]), values: make(map[string]string)})
			current = &sections[len(sections)-1]
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || current == nil || strings.TrimSpace(key) == "" {
			warnings = append(warnings, fmt.Sprintf("syntaxe INI invalide ligne %d", lineNumber+1))
			continue
		}
		current.values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return sections, warnings
}

func localGPGChecksum(value string) (string, bool) {
	for _, candidate := range strings.Fields(value) {
		path := strings.TrimPrefix(candidate, "file://")
		if !filepath.IsAbs(path) || !strings.HasPrefix(path, "/etc/pki/rpm-gpg/") {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr == nil && closeErr == nil {
			return fmt.Sprintf("%x", hash.Sum(nil)), true
		}
	}
	return "", false
}

func officialRPMURL(value string) bool {
	hosts := []string{"fedoraproject.org", "rpmfusion.org", "rockylinux.org", "almalinux.org", "centos.org", "opensuse.org", "redhat.com"}
	for _, host := range hosts {
		if strings.Contains(strings.ToLower(value), host) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstURL(value string) string {
	for _, candidate := range strings.Fields(value) {
		if strings.Contains(candidate, "://") {
			return candidate
		}
	}
	return value
}

func gpgKeyReference(value string) string {
	for _, candidate := range strings.Fields(value) {
		if strings.HasPrefix(strings.ToLower(candidate), "http://") ||
			strings.HasPrefix(strings.ToLower(candidate), "https://") ||
			strings.HasPrefix(candidate, "file:///etc/pki/rpm-gpg/") {
			return candidate
		}
	}
	return ""
}
