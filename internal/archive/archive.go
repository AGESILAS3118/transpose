package archive

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/transpose-project/transpose/pkg/manifest"
)

const (
	CurrentArchiveVersion = 1
	MetaFileName          = "meta.json"
	ManifestFileName      = "manifest.yaml"
	ReposDirName          = "repos"
)

// Meta represents metadata stored inside the archive.
type Meta struct {
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	SourceDistro string    `json:"source_distro"`
	SourceFamily string    `json:"source_family,omitempty"`
	TargetFamily string    `json:"target_family,omitempty"`
	PackageCount int       `json:"package_count"`
	RepoCount    int       `json:"repo_count"`
}

// PackOptions defines inputs for creating a .tar.zst archive.
type PackOptions struct {
	ManifestPath string
	OutputPath   string
	IncludeRepos bool
}

// PackResult summarizes the created archive.
type PackResult struct {
	ArchivePath  string
	Size         int64
	PackageCount int
	RepoCount    int
	SourceDistro string
	Warnings     []string
}

// UnpackOptions defines inputs for unpacking a .tar.zst archive.
type UnpackOptions struct {
	ArchivePath string
	TargetDir   string
}

// UnpackResult summarizes the extracted archive contents.
type UnpackResult struct {
	TargetDir    string
	ManifestPath string
	Meta         Meta
	Extracted    []string
}

// Pack creates a portable .tar.zst archive from a manifest.
func Pack(opts PackOptions) (*PackResult, error) {
	if opts.ManifestPath == "" {
		return nil, errors.New("manifest path is required")
	}

	manifestFile, err := os.Open(opts.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("impossible d'ouvrir le manifeste: %w", err)
	}
	defer manifestFile.Close()

	m, err := manifest.ReadYAML(manifestFile)
	if err != nil {
		return nil, fmt.Errorf("manifeste invalide: %w", err)
	}

	manifestBytes, err := os.ReadFile(opts.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("lecture du manifeste: %w", err)
	}

	outPath := opts.OutputPath
	if outPath == "" {
		dir := filepath.Dir(opts.ManifestPath)
		base := strings.TrimSuffix(filepath.Base(opts.ManifestPath), filepath.Ext(opts.ManifestPath))
		outPath = filepath.Join(dir, base+".tar.zst")
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("création de l'archive: %w", err)
	}
	defer outFile.Close()

	zstdWriter, err := zstd.NewWriter(outFile)
	if err != nil {
		return nil, fmt.Errorf("initialisation zstd: %w", err)
	}
	defer zstdWriter.Close()

	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	var warnings []string
	repoCount := 0

	// 1. Write manifest.yaml
	manifestHdr := &tar.Header{
		Name:     ManifestFileName,
		Mode:     0644,
		Size:     int64(len(manifestBytes)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(manifestHdr); err != nil {
		return nil, fmt.Errorf("écriture header manifest.yaml: %w", err)
	}
	if _, err := tarWriter.Write(manifestBytes); err != nil {
		return nil, fmt.Errorf("écriture contenu manifest.yaml: %w", err)
	}

	// 2. Write repos if enabled
	if opts.IncludeRepos {
		if len(m.Repos) == 0 {
			warnings = append(warnings, "aucun dépôt natif déclaré dans le manifeste")
		} else {
			// Write directory repos/
			dirHdr := &tar.Header{
				Name:     ReposDirName + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
				ModTime:  time.Now().UTC(),
			}
			if err := tarWriter.WriteHeader(dirHdr); err != nil {
				return nil, fmt.Errorf("écriture header dossier repos/: %w", err)
			}

			for _, repo := range m.Repos {
				fileName := repo.FileName
				if fileName == "" {
					fileName = repo.Name + ".repo"
				}
				repoEntryName := filepath.ToSlash(filepath.Join(ReposDirName, filepath.Base(fileName)))
				repoBytes := []byte(repo.Content)

				repoHdr := &tar.Header{
					Name:     repoEntryName,
					Mode:     0644,
					Size:     int64(len(repoBytes)),
					ModTime:  time.Now().UTC(),
					Typeflag: tar.TypeReg,
				}
				if err := tarWriter.WriteHeader(repoHdr); err != nil {
					return nil, fmt.Errorf("écriture header repo %s: %w", repoEntryName, err)
				}
				if _, err := tarWriter.Write(repoBytes); err != nil {
					return nil, fmt.Errorf("écriture contenu repo %s: %w", repoEntryName, err)
				}
				repoCount++
			}
		}
	}

	// 3. Write meta.json
	meta := Meta{
		Version:      CurrentArchiveVersion,
		CreatedAt:    time.Now().UTC(),
		SourceDistro: m.Source.Distro,
		PackageCount: len(m.Packages),
		RepoCount:    repoCount,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sérialisation meta.json: %w", err)
	}

	metaHdr := &tar.Header{
		Name:     MetaFileName,
		Mode:     0644,
		Size:     int64(len(metaBytes)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(metaHdr); err != nil {
		return nil, fmt.Errorf("écriture header meta.json: %w", err)
	}
	if _, err := tarWriter.Write(metaBytes); err != nil {
		return nil, fmt.Errorf("écriture contenu meta.json: %w", err)
	}

	// Flush and close tar & zstd streams to ensure size is accurate
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := zstdWriter.Close(); err != nil {
		return nil, err
	}
	if err := outFile.Sync(); err != nil {
		// Ignore sync error on non-regular files
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		return nil, fmt.Errorf("stat de l'archive créée: %w", err)
	}

	return &PackResult{
		ArchivePath:  outPath,
		Size:         fi.Size(),
		PackageCount: len(m.Packages),
		RepoCount:    repoCount,
		SourceDistro: m.Source.Distro,
		Warnings:     warnings,
	}, nil
}

// Unpack extracts a .tar.zst archive safely into opts.TargetDir.
// It strictly protects against path traversal vulnerabilities.
func Unpack(opts UnpackOptions) (*UnpackResult, error) {
	if opts.ArchivePath == "" {
		return nil, errors.New("archive path is required")
	}
	if opts.TargetDir == "" {
		return nil, errors.New("target directory is required")
	}

	cleanTargetDir := filepath.Clean(opts.TargetDir)
	if err := os.MkdirAll(cleanTargetDir, 0755); err != nil {
		return nil, fmt.Errorf("création du dossier cible: %w", err)
	}

	archiveFile, err := os.Open(opts.ArchivePath)
	if err != nil {
		return nil, fmt.Errorf("ouverture de l'archive: %w", err)
	}
	defer archiveFile.Close()

	zstdReader, err := zstd.NewReader(archiveFile)
	if err != nil {
		return nil, fmt.Errorf("décompression zstd: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)
	var extracted []string
	hasManifest := false
	hasMeta := false
	var archiveMeta Meta

	targetPrefix := cleanTargetDir + string(filepath.Separator)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("lecture tar: %w", err)
		}

		name := header.Name

		// Rejeter SYMLINK et HARDLINK pour MVP
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return nil, fmt.Errorf("tar entry rejected (symlinks/hardlinks forbidden): %q", name)
		}

		// Anti-path-traversal protection (PRD §14)
		if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
			return nil, fmt.Errorf("tar entry rejected (absolute path forbidden): %q", name)
		}

		cleanRel := filepath.Clean(name)
		if strings.HasPrefix(cleanRel, "..") || strings.Contains(cleanRel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("tar entry rejected (path traversal detected): %q", name)
		}

		destPath := filepath.Join(cleanTargetDir, cleanRel)
		absDest, err := filepath.Abs(destPath)
		if err != nil {
			return nil, fmt.Errorf("résolution chemin absolu %s: %w", destPath, err)
		}
		cleanAbsDest := filepath.Clean(absDest)
		cleanDest := cleanAbsDest

		// Verification that cleanAbsDest stays inside cleanTargetDir
		if cleanAbsDest != cleanTargetDir && !strings.HasPrefix(cleanAbsDest, targetPrefix) {
			return nil, fmt.Errorf("tar entry escapes target dir: %q -> %q", name, cleanAbsDest)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanDest, 0755); err != nil {
				return nil, fmt.Errorf("création sous-dossier %s: %w", cleanDest, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(cleanDest), 0755); err != nil {
				return nil, fmt.Errorf("création parent %s: %w", filepath.Dir(cleanDest), err)
			}
			outFile, err := os.OpenFile(cleanDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return nil, fmt.Errorf("création fichier extrait %s: %w", cleanDest, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return nil, fmt.Errorf("écriture fichier extrait %s: %w", cleanDest, err)
			}
			outFile.Close()

			if cleanRel == ManifestFileName {
				hasManifest = true
			}
			if cleanRel == MetaFileName {
				hasMeta = true
				metaData, err := os.ReadFile(cleanDest)
				if err == nil {
					_ = json.Unmarshal(metaData, &archiveMeta)
				}
			}
			extracted = append(extracted, cleanRel)
		default:
			// Ignore unsupported tar types for security (symlinks, devices, etc.)
		}
	}

	if !hasManifest {
		return nil, fmt.Errorf("archive invalide : %s manquant", ManifestFileName)
	}

	if hasMeta && archiveMeta.Version != CurrentArchiveVersion {
		return nil, fmt.Errorf("archive version %d non supportée (attendu %d)", archiveMeta.Version, CurrentArchiveVersion)
	}

	manifestPath := filepath.Join(cleanTargetDir, ManifestFileName)
	return &UnpackResult{
		TargetDir:    cleanTargetDir,
		ManifestPath: manifestPath,
		Meta:         archiveMeta,
		Extracted:    extracted,
	}, nil
}
