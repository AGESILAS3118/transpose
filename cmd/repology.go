package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/transpose-project/transpose/pkg/manifest"
)

func repologyPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("détermination du dossier utilisateur: %w", err)
	}
	return filepath.Join(home, ".local", "share", "xpose", "repology.db"),
		filepath.Join(home, ".cache", "xpose", "repology"), nil
}

func readManifest(path string) (*manifest.Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ouverture manifeste %s: %w", path, err)
	}
	m, readErr := manifest.ReadYAML(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("lecture manifeste %s: %w", path, errors.Join(readErr, closeErr))
	}
	return m, nil
}
