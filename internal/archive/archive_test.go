package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func sampleManifestYAML() string {
	return `schema: 1
source:
  distro: fedora
  version: "44"
  arch: amd64
packages:
  - name: htop
    manager: dnf
  - name: tree
    manager: dnf
repos:
  - kind: rpm
    name: testrepo
    file_name: testrepo.repo
    content: "[testrepo]\nname=testrepo\nenabled=1\n"
`
}

func sampleManifestWithoutReposYAML() string {
	return `schema: 1
source:
  distro: fedora
  version: "44"
  arch: amd64
packages:
  - name: htop
    manager: dnf
`
}

func createArchiveWithEntry(t *testing.T, entryName string, content []byte) string {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "malicious.tar.zst")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw, err := zstd.NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	// Write manifest.yaml so archive isn't rejected just for missing manifest
	manifestContent := []byte(sampleManifestYAML())
	hdrM := &tar.Header{
		Name:     ManifestFileName,
		Mode:     0644,
		Size:     int64(len(manifestContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdrM); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(manifestContent); err != nil {
		t.Fatal(err)
	}

	// Write the target entry
	hdr := &tar.Header{
		Name:     entryName,
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return archivePath
}

// TestPackUnpack_RoundTrip : pack un manifeste -> unpack -> manifeste identique
func TestPackUnpack_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(sampleManifestYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(tmpDir, "bundle.tar.zst")
	packRes, err := Pack(PackOptions{
		ManifestPath: manifestPath,
		OutputPath:   archivePath,
		IncludeRepos: true,
	})
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if packRes.PackageCount != 2 {
		t.Errorf("PackageCount = %d, want 2", packRes.PackageCount)
	}
	if packRes.RepoCount != 1 {
		t.Errorf("RepoCount = %d, want 1", packRes.RepoCount)
	}
	if packRes.Size <= 0 {
		t.Errorf("Size = %d, want > 0", packRes.Size)
	}

	unpackDir := filepath.Join(tmpDir, "unpacked")
	unpackRes, err := Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   unpackDir,
	})
	if err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	if unpackRes.Meta.PackageCount != 2 {
		t.Errorf("Meta.PackageCount = %d, want 2", unpackRes.Meta.PackageCount)
	}
	if unpackRes.Meta.Version != 1 {
		t.Errorf("Meta.Version = %d, want 1", unpackRes.Meta.Version)
	}

	unpackedManifestData, err := os.ReadFile(unpackRes.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unpackedManifestData) != sampleManifestYAML() {
		t.Errorf("Manifest content altered after round-trip")
	}

	repoPath := filepath.Join(unpackDir, "repos", "testrepo.repo")
	if _, err := os.Stat(repoPath); err != nil {
		t.Errorf("testrepo.repo not found: %v", err)
	}
}

// TestUnpack_RejectsPathTraversal : tar avec ../evil.txt -> erreur
func TestUnpack_RejectsPathTraversal(t *testing.T) {
	archivePath := createArchiveWithEntry(t, "../evil.txt", []byte("evil content"))
	unpackDir := filepath.Join(t.TempDir(), "target")

	_, err := Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   unpackDir,
	})
	if err == nil {
		t.Fatalf("expected Unpack to reject ../evil.txt path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "escapes target") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestUnpack_RejectsAbsolutePath : tar avec /etc/passwd -> erreur
func TestUnpack_RejectsAbsolutePath(t *testing.T) {
	archivePath := createArchiveWithEntry(t, "/etc/passwd", []byte("evil content"))
	unpackDir := filepath.Join(t.TempDir(), "target")

	_, err := Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   unpackDir,
	})
	if err == nil {
		t.Fatalf("expected Unpack to reject absolute path entry")
	}
	if !strings.Contains(err.Error(), "absolute path forbidden") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestPack_EmptyRepos : manifeste sans repos -> archive valide, warning
func TestPack_EmptyRepos(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest-no-repos.yaml")
	if err := os.WriteFile(manifestPath, []byte(sampleManifestWithoutReposYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(tmpDir, "no-repos.tar.zst")
	packRes, err := Pack(PackOptions{
		ManifestPath: manifestPath,
		OutputPath:   archivePath,
		IncludeRepos: true,
	})
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if len(packRes.Warnings) == 0 {
		t.Errorf("expected warning for empty repos, got 0")
	}

	unpackDir := filepath.Join(tmpDir, "unpacked")
	unpackRes, err := Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   unpackDir,
	})
	if err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	if unpackRes.Meta.RepoCount != 0 {
		t.Errorf("Meta.RepoCount = %d, want 0", unpackRes.Meta.RepoCount)
	}
}

// TestUnpack_MissingManifest : archive sans manifest.yaml -> erreur claire
func TestUnpack_MissingManifest(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.zst")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw, _ := zstd.NewWriter(f)
	tw := tar.NewWriter(zw)
	hdr := &tar.Header{
		Name:     "dummy.txt",
		Mode:     0644,
		Size:     5,
		Typeflag: tar.TypeReg,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte("hello"))
	tw.Close()
	zw.Close()
	f.Close()

	_, err = Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(tmpDir, "unpacked"),
	})
	if err == nil {
		t.Fatalf("expected error for missing manifest.yaml")
	}
	if !strings.Contains(err.Error(), "manifest.yaml manquant") {
		t.Errorf("error message = %v, want mention of manifest.yaml manquant", err)
	}
}

// TestPack_ArchiveIsZstdCompressed : magic bytes zstd présents (0x28, 0xB5, 0x2F, 0xFD)
func TestPack_ArchiveIsZstdCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(sampleManifestYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(tmpDir, "compressed.tar.zst")
	_, err := Pack(PackOptions{
		ManifestPath: manifestPath,
		OutputPath:   archivePath,
		IncludeRepos: true,
	})
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 {
		t.Fatalf("archive file too small")
	}

	zstdMagic := []byte{0x28, 0xB5, 0x2F, 0xFD}
	if !bytes.Equal(data[:4], zstdMagic) {
		t.Errorf("archive magic bytes = %v, want zstd magic %v", data[:4], zstdMagic)
	}
}

// TestUnpack_RejectsUnsupportedMetaVersion : version 99 -> erreur explicite (Précision 2)
func TestUnpack_RejectsUnsupportedMetaVersion(t *testing.T) {
	badMeta := `{"version": 99, "source_distro": "fedora"}`
	archivePath := createArchiveWithEntry(t, MetaFileName, []byte(badMeta))

	_, err := Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(t.TempDir(), "target"),
	})
	if err == nil {
		t.Fatalf("expected Unpack to reject version 99")
	}
	if !strings.Contains(err.Error(), "archive version 99 non supportée (attendu 1)") {
		t.Errorf("error message = %q, want mention of version 99 non supportée", err.Error())
	}
}

// TestUnpack_RejectsSymlink : symlink -> rejet immédiat
func TestUnpack_RejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "symlink.tar.zst")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw, _ := zstd.NewWriter(f)
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	manifestContent := []byte(sampleManifestYAML())
	hdrM := &tar.Header{
		Name:     ManifestFileName,
		Mode:     0644,
		Size:     int64(len(manifestContent)),
		Typeflag: tar.TypeReg,
	}
	tw.WriteHeader(hdrM)
	tw.Write(manifestContent)

	hdrSym := &tar.Header{
		Name:     "link_to_passwd",
		Linkname: "/etc/passwd",
		Typeflag: tar.TypeSymlink,
	}
	tw.WriteHeader(hdrSym)

	tw.Close()
	zw.Close()
	f.Close()

	_, err = Unpack(UnpackOptions{
		ArchivePath: archivePath,
		TargetDir:   filepath.Join(tmpDir, "target"),
	})
	if err == nil {
		t.Fatalf("expected Unpack to reject symlink")
	}
	if !strings.Contains(err.Error(), "symlinks/hardlinks forbidden") {
		t.Errorf("unexpected error: %v", err)
	}
}
