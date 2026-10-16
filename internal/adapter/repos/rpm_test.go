package repos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPMCollector_ParsesValidRepoFile(t *testing.T) {
	directory := t.TempDir()
	content := `[fedora]
name=Fedora 44 - x86_64
baseurl=https://download.fedoraproject.org/pub/fedora/linux/releases/44/Everything/x86_64/os/
enabled=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-fedora-44-x86_64
`
	if err := os.WriteFile(filepath.Join(directory, "fedora.repo"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := NewRPMRepoCollector(directory)
	found, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Kind != "rpm" || found[0].Name != "fedora" ||
		!found[0].Enabled || found[0].ThirdParty || found[0].URL == "" {
		t.Fatalf("collected repos = %#v", found)
	}
	if found[0].FileName != "fedora.repo" || !strings.Contains(found[0].Content, "baseurl=") {
		t.Fatalf("repo source metadata = %#v", found[0])
	}
}

func TestRPMCollector_DetectsThirdParty(t *testing.T) {
	found, _ := ParseRPMRepoFile("external.repo", `[external]
baseurl=https://example.com/repo
enabled=1
`)
	if len(found) != 1 || !found[0].ThirdParty {
		t.Fatalf("repos = %#v, want third-party repo", found)
	}
}

func TestRPMCollector_HandlesMalformedFile(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "broken.repo"), []byte("[broken\nbad line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := NewRPMRepoCollector(directory)
	found, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(found) != 0 || len(collector.Warnings()) == 0 {
		t.Fatalf("repos=%#v warnings=%#v, want warning and no repo", found, collector.Warnings())
	}
}

func TestRPMCollector_NeverIncludesPrivateKeys(t *testing.T) {
	directory := t.TempDir()
	content := `[external]
baseurl=https://example.com/repo
password=hunter2
gpgkey=-----BEGIN PGP PRIVATE KEY BLOCK-----
not-a-key
-----END PGP PRIVATE KEY BLOCK-----
`
	if err := os.WriteFile(filepath.Join(directory, "external.repo"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := NewRPMRepoCollector(directory).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("collected repos = %#v", found)
	}
	if strings.Contains(strings.ToLower(found[0].Content), "password") ||
		strings.Contains(found[0].Content, "BEGIN PGP PRIVATE") ||
		strings.Contains(found[0].GPGKeyURL, "BEGIN PGP PRIVATE") {
		t.Fatalf("sensitive data leaked: %#v", found[0])
	}
}

func TestRPMCollector_WarnsOnInsecureBaseURL(t *testing.T) {
	_, warnings := ParseRPMRepoFile("local.repo", `[local]
baseurl=http://example.com/repo
enabled=1
`)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "non sécurisée") {
		t.Fatalf("warnings = %#v", warnings)
	}
}
