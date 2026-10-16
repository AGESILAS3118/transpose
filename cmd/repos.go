package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	repoadapter "github.com/transpose-project/transpose/internal/adapter/repos"
	"github.com/transpose-project/transpose/pkg/manifest"
	"gopkg.in/yaml.v3"
)

func newReposCmd() *cobra.Command {
	return newReposCmdWithCollectors([]repoadapter.RepoCollector{
		repoadapter.NewRPMRepoCollector(""),
		repoadapter.NewAPTRepoCollector(""),
	})
}

func newReposCmdWithCollectors(collectors []repoadapter.RepoCollector) *cobra.Command {
	command := &cobra.Command{
		Use:   "repos",
		Short: "Inspect and export native package repositories",
		Args:  cobra.NoArgs,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List detected package repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			found, warnings, err := collectRepos(cmd, collectors)
			if err != nil {
				return err
			}
			for _, warning := range warnings {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning); err != nil {
					return err
				}
			}
			format, err := repoFormat(cmd)
			if err != nil {
				return err
			}
			return writeRepos(cmd.OutOrStdout(), found, format)
		},
	}
	list.Flags().String("format", "yaml", "Output format (yaml|json)")

	export := &cobra.Command{
		Use:   "export",
		Short: "Export detected repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("repos export requiert -o <fichier>")
			}
			if err := rejectEtcOutputPath(path); err != nil {
				return err
			}
			found, warnings, err := collectRepos(cmd, collectors)
			if err != nil {
				return err
			}
			for _, warning := range warnings {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning); err != nil {
					return err
				}
			}
			return writeResolveFile(path, func(writer io.Writer) error {
				encoder := yaml.NewEncoder(writer)
				encoder.SetIndent(2)
				if err := encoder.Encode(struct {
					Repos []manifest.Repo `yaml:"repos"`
				}{Repos: found}); err != nil {
					return fmt.Errorf("encodage YAML des dépôts: %w", err)
				}
				return encoder.Close()
			})
		},
	}
	export.Flags().StringP("output", "o", "", "Write repository export to this file")

	check := &cobra.Command{
		Use:   "check <repos.yaml>",
		Short: "Validate an exported repository list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("lecture de %s: %w", args[0], err)
			}
			found, err := decodeRepoExport(data)
			if err != nil {
				return err
			}
			if err := validateRepoExport(found); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "OK: %d dépôts cohérents\n", len(found))
			return err
		},
	}
	command.AddCommand(list, export, check)
	return command
}

func rejectEtcOutputPath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("résolution du chemin de sortie: %w", err)
	}
	current := filepath.Clean(absolute)
	var suffix []string
	for {
		_, statErr := os.Lstat(current)
		if statErr == nil {
			current, err = filepath.EvalSymlinks(current)
			if err != nil {
				return fmt.Errorf("résolution des liens symboliques du chemin de sortie: %w", err)
			}
			break
		}
		if !os.IsNotExist(statErr) {
			return fmt.Errorf("inspection du chemin de sortie: %w", statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("impossible de résoudre le chemin de sortie %q", path)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		current = filepath.Join(current, suffix[i])
	}
	etcRoot, err := filepath.EvalSymlinks("/etc")
	if err != nil {
		return fmt.Errorf("résolution de /etc: %w", err)
	}
	if current == etcRoot || strings.HasPrefix(current, etcRoot+string(filepath.Separator)) {
		return fmt.Errorf("écriture interdite sous /etc : %s", current)
	}
	return nil
}

func collectRepos(cmd *cobra.Command, collectors []repoadapter.RepoCollector) ([]manifest.Repo, []string, error) {
	var found []manifest.Repo
	var warnings []string
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		present, err := collector.Detect()
		if err != nil {
			return nil, warnings, fmt.Errorf("détection du collecteur %s: %w", collector.Name(), err)
		}
		if !present {
			continue
		}
		repos, err := collector.Collect(cmd.Context())
		if err != nil {
			return nil, warnings, fmt.Errorf("collecte des dépôts %s: %w", collector.Name(), err)
		}
		found = append(found, repos...)
		warnings = append(warnings, repoadapter.Warnings(collector)...)
	}
	return found, warnings, nil
}

func repoFormat(cmd *cobra.Command) (string, error) {
	format, err := cmd.Flags().GetString("format")
	return format, err
}

func writeRepos(writer io.Writer, repos []manifest.Repo, format string) error {
	switch format {
	case "json":
		if repos == nil {
			repos = []manifest.Repo{}
		}
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(repos)
	case "yaml":
		if repos == nil {
			repos = []manifest.Repo{}
		}
		encoder := yaml.NewEncoder(writer)
		encoder.SetIndent(2)
		if err := encoder.Encode(repos); err != nil {
			return err
		}
		return encoder.Close()
	default:
		return fmt.Errorf("format inconnu %q (attendu yaml ou json)", format)
	}
}

func decodeRepoExport(data []byte) ([]manifest.Repo, error) {
	var wrapped struct {
		Repos []manifest.Repo `yaml:"repos"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("lecture de l'export des dépôts: %w", err)
	}
	if wrapped.Repos != nil {
		return wrapped.Repos, nil
	}
	var repos []manifest.Repo
	if err := yaml.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("lecture de l'export des dépôts: %w", err)
	}
	return repos, nil
}

func validateRepoExport(repos []manifest.Repo) error {
	names := make(map[string]struct{}, len(repos))
	for index, repo := range repos {
		name := strings.ToLower(strings.TrimSpace(repo.Name))
		if name == "" {
			return fmt.Errorf("repos[%d].name ne peut pas être vide", index)
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("nom de dépôt dupliqué : %q", repo.Name)
		}
		names[name] = struct{}{}
		if strings.TrimSpace(repo.URL) == "" {
			return fmt.Errorf("URL vide pour le dépôt %q", repo.Name)
		}
		parsedURL, err := url.Parse(repo.URL)
		if err != nil || parsedURL.Scheme == "" ||
			((parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host == "") {
			return fmt.Errorf("URL invalide pour le dépôt %q : %q", repo.Name, repo.URL)
		}
	}
	var output bytes.Buffer
	m := &manifest.Manifest{Schema: manifest.SchemaVersion, Source: manifest.Source{Distro: "validation"}, Repos: repos}
	if err := manifest.WriteYAML(m, &output); err != nil {
		return err
	}
	if _, err := manifest.ReadYAML(bytes.NewReader(output.Bytes())); err != nil {
		return fmt.Errorf("export de dépôts invalide: %w", err)
	}
	return nil
}
