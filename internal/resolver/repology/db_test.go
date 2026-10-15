package repology

import (
	"context"
	"testing"
)

func TestDB_InsertIdempotent(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &Project{
		Name: "htop",
		Packages: []ProjectPackage{
			{Repo: "debian_13", Name: "htop", Version: "3.4", Category: "admin"},
		},
	}
	for range 2 {
		if _, err := StoreProject(context.Background(), db, project, "https://example.test", "debian_13"); err != nil {
			t.Fatalf("StoreProject() error = %v", err)
		}
	}
	count, err := CountPackages(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("package count = %d, want 1", count)
	}
}

func TestDB_QueryByProject(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &Project{
		Name: "firefox",
		Packages: []ProjectPackage{
			{Repo: "debian_13", Name: "firefox-esr", Version: "128"},
			{Repo: "ubuntu_24_04", Name: "firefox", Version: "1"},
		},
	}
	if _, err := StoreProject(context.Background(), db, project, "https://mozilla.org", "debian_13"); err != nil {
		t.Fatal(err)
	}
	packages, upstream, err := QueryByProject(context.Background(), db, "firefox", "debian_13")
	if err != nil {
		t.Fatal(err)
	}
	if upstream != "https://mozilla.org" || len(packages) != 1 || packages[0].Name != "firefox-esr" {
		t.Errorf("QueryByProject() = %#v, upstream %q", packages, upstream)
	}
}
