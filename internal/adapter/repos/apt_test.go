package repos

import (
	"testing"
)

func TestAPTCollector_ParsesLegacyFormat(t *testing.T) {
	found, warnings := ParseAPTList("sources.list", "deb https://deb.debian.org/debian bookworm main contrib\n")
	if len(warnings) != 0 || len(found) != 1 {
		t.Fatalf("repos=%#v warnings=%#v", found, warnings)
	}
	if found[0].Kind != "apt" || found[0].URL != "https://deb.debian.org/debian" ||
		found[0].ThirdParty || !found[0].Enabled || found[0].FileName != "sources.list" {
		t.Fatalf("repo = %#v", found[0])
	}
}

func TestAPTCollector_ParsesDeb822Format(t *testing.T) {
	found, warnings := ParseAPTDeb822("debian.sources", `Types: deb
URIs: https://deb.debian.org/debian
Suites: bookworm bookworm-updates
Components: main contrib
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
`)
	if len(warnings) != 0 || len(found) != 2 {
		t.Fatalf("repos=%#v warnings=%#v", found, warnings)
	}
	if found[0].Kind != "apt" || found[0].GPGKeyURL != "/usr/share/keyrings/debian-archive-keyring.gpg" ||
		found[0].FileName != "debian.sources" || found[0].ThirdParty {
		t.Fatalf("repo = %#v", found[0])
	}
}

func TestAPTCollector_MarksThirdParty(t *testing.T) {
	found, warnings := ParseAPTList("ppa.list", "deb https://ppa.launchpadcontent.net/example/stable/ubuntu noble main\n")
	if len(warnings) != 0 || len(found) != 1 || !found[0].ThirdParty {
		t.Fatalf("repos=%#v warnings=%#v, want third-party PPA", found, warnings)
	}
}
