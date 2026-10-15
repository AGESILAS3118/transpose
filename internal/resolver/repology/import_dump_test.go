package repology

import (
	"context"
	"strings"
	"testing"
)

func TestImportDump_JSONLFiltersRepositoryAndUsesProjectName(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dump := strings.NewReader(`{"project_name":"networkmanager","upstream_url":"https://networkmanager.dev","repo":"debian_13","name":"network-manager","version":"1.44","category":"admin"}
{"project_name":"networkmanager","repo":"ubuntu_24_04","name":"network-manager","version":"1.46"}
`)
	imported, err := ImportDump(context.Background(), db, dump, "debian_13")
	if err != nil {
		t.Fatalf("ImportDump() error = %v", err)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1", imported)
	}
	packages, upstream, err := QueryByProject(context.Background(), db, "networkmanager", "debian_13")
	if err != nil {
		t.Fatal(err)
	}
	if upstream != "https://networkmanager.dev" || len(packages) != 1 ||
		packages[0].Name != "network-manager" || packages[0].ProjectName != "networkmanager" {
		t.Errorf("query = %#v upstream=%q", packages, upstream)
	}
}
