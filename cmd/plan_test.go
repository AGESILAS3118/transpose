package cmd

import (
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/sysutil"
)

func TestPlanCmd_MissingFile(t *testing.T) {
	command := newPlanCmdWithQuerierFactory(func(sysutil.Target) (resolver.RepoQuerier, error) {
		t.Fatal("querier factory called for missing manifest")
		return nil, nil
	})
	root := commandRoot(command)
	root.SetArgs([]string{"plan", "/definitely/missing/manifest.yaml"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "ouverture du manifeste") {
		t.Fatalf("plan error = %v, want missing manifest error", err)
	}
}

func TestPlanCmd_CrossFamilyRejected(t *testing.T) {
	command := newPlanCmdWithQuerierFactory(func(sysutil.Target) (resolver.RepoQuerier, error) {
		t.Fatal("querier factory called for cross-family target")
		return nil, nil
	})
	root := commandRoot(command)
	root.SetArgs([]string{"plan", writeCommandManifest(t, nil), "--target", "debian_13"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "cross-family non supporté") {
		t.Fatalf("plan error = %v, want cross-family rejection", err)
	}
}
