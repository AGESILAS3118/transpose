package repos

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpose-project/transpose/pkg/manifest"
)

const defaultAPTDirectory = "/etc/apt"

type APTRepoCollector struct {
	Directory string
	warnings  []string
}

func NewAPTRepoCollector(directory string) *APTRepoCollector {
	if directory == "" {
		directory = defaultAPTDirectory
	}
	return &APTRepoCollector{Directory: directory}
}

func (c *APTRepoCollector) Name() string { return "apt" }

func (c *APTRepoCollector) Warnings() []string {
	return append([]string(nil), c.warnings...)
}

func (c *APTRepoCollector) Detect() (bool, error) {
	_, err := os.Stat(c.Directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lecture du dossier APT %s: %w", c.Directory, err)
	}
	return true, nil
}

func (c *APTRepoCollector) Collect(ctx context.Context) ([]manifest.Repo, error) {
	c.warnings = nil
	sourcesDirectory := filepath.Join(c.Directory, "sources.list.d")
	paths := []string{filepath.Join(c.Directory, "sources.list")}
	for _, pattern := range []string{"*.list", "*.sources"} {
		matches, err := filepath.Glob(filepath.Join(sourcesDirectory, pattern))
		if err != nil {
			return nil, fmt.Errorf("recherche des sources APT %s: %w", pattern, err)
		}
		paths = append(paths, matches...)
	}

	var result []manifest.Repo
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("lecture des sources APT %s: %w", path, err)
		}
		filename := filepath.Base(path)
		var parsed []manifest.Repo
		var warnings []string
		if strings.HasSuffix(filename, ".sources") {
			parsed, warnings = ParseAPTDeb822(filename, string(data))
		} else {
			parsed, warnings = ParseAPTList(filename, string(data))
		}
		result = append(result, parsed...)
		for _, warning := range warnings {
			c.warnings = append(c.warnings, fmt.Sprintf("%s: %s", filename, warning))
		}
	}
	return result, nil
}

func ParseAPTList(fileName, content string) ([]manifest.Repo, []string) {
	var repos []manifest.Repo
	var warnings []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		repo, ok := parseAPTLine(fileName, line, len(repos))
		if !ok {
			warnings = append(warnings, fmt.Sprintf("ligne sources.list invalide %d", lineNumber))
			continue
		}
		repos = append(repos, repo)
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, "erreur pendant la lecture du contenu: "+err.Error())
	}
	return repos, warnings
}

func parseAPTLine(fileName, line string, index int) (manifest.Repo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || (fields[0] != "deb" && fields[0] != "deb-src") {
		return manifest.Repo{}, false
	}
	position := 1
	signedBy := ""
	if strings.HasPrefix(fields[position], "[") {
		options := strings.Trim(fields[position], "[]")
		for _, option := range strings.Fields(options) {
			key, value, ok := strings.Cut(option, "=")
			if ok && strings.EqualFold(key, "signed-by") {
				signedBy = value
			}
		}
		position++
	}
	if len(fields) <= position+1 {
		return manifest.Repo{}, false
	}
	uri, suite := fields[position], fields[position+1]
	name := aptRepoName(fileName, suite, index)
	return manifest.Repo{
		Kind:       "apt",
		Name:       name,
		URL:        sanitizeReference(uri),
		FileName:   fileName,
		Content:    sanitizeContent(line),
		Enabled:    true,
		ThirdParty: !officialAPTURL(uri),
		GPGKeyURL:  sanitizeReference(signedBy),
	}, true
}

func ParseAPTDeb822(fileName, content string) ([]manifest.Repo, []string) {
	paragraphs, warnings := parseDeb822(content)
	var repos []manifest.Repo
	for _, paragraph := range paragraphs {
		if paragraph["types"] == "" || paragraph["uris"] == "" || paragraph["suites"] == "" {
			warnings = append(warnings, "source deb822 sans Types, URIs ou Suites")
			continue
		}
		enabled := !strings.EqualFold(strings.TrimSpace(paragraph["enabled"]), "no") &&
			!strings.EqualFold(strings.TrimSpace(paragraph["enabled"]), "false")
		uriValues := strings.Fields(paragraph["uris"])
		suites := strings.Fields(paragraph["suites"])
		for _, uri := range uriValues {
			for _, suite := range suites {
				index := len(repos)
				repos = append(repos, manifest.Repo{
					Kind:       "apt",
					Name:       aptRepoName(fileName, suite, index),
					URL:        sanitizeReference(uri),
					FileName:   fileName,
					Content:    sanitizeContent(paragraph["_content"]),
					Enabled:    enabled,
					ThirdParty: !officialAPTURL(uri),
					GPGKeyURL:  sanitizeReference(strings.TrimSpace(paragraph["signed-by"])),
				})
			}
		}
	}
	return repos, warnings
}

func parseDeb822(content string) ([]map[string]string, []string) {
	var paragraphs []map[string]string
	var warnings []string
	var current map[string]string
	var original []string
	lastKey := ""
	flush := func() {
		if current == nil {
			return
		}
		current["_content"] = strings.Join(original, "\n")
		paragraphs = append(paragraphs, current)
		current = nil
		original = nil
		lastKey = ""
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		original = append(original, line)
		if current == nil {
			current = make(map[string]string)
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(current) == 0 {
				warnings = append(warnings, "continuation deb822 sans champ précédent")
				continue
			}
			if lastKey != "" {
				current[lastKey] += "\n" + strings.TrimSpace(line)
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			warnings = append(warnings, fmt.Sprintf("syntaxe deb822 invalide : %q", line))
			continue
		}
		lastKey = strings.ToLower(strings.TrimSpace(key))
		current[lastKey] = strings.TrimSpace(value)
	}
	flush()
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, "erreur pendant la lecture du contenu: "+err.Error())
	}
	return paragraphs, warnings
}

func aptRepoName(fileName, suite string, index int) string {
	name := strings.TrimSuffix(strings.TrimSuffix(fileName, ".list"), ".sources")
	suite = strings.NewReplacer("/", "-", " ", "-", ":", "-").Replace(suite)
	return fmt.Sprintf("%s-%s-%d", name, suite, index+1)
}

func officialAPTURL(uri string) bool {
	value := strings.ToLower(uri)
	return strings.Contains(value, "debian.org") || strings.Contains(value, "ubuntu.com")
}
