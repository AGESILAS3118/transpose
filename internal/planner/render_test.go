package planner

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestRenderTerminal_NotVerboseTruncates(t *testing.T) {
	plan := &Plan{Steps: []PlanStep{{Order: 1, Manager: "dnf", Command: "dnf install -y"}}}
	for i := 1; i <= 10; i++ {
		name := "pkg" + string(rune('0'+i))
		plan.Steps[0].Items = append(plan.Steps[0].Items, PlanItem{Source: name, Target: name, Status: resolver.StatusExact})
	}
	var output bytes.Buffer
	if err := RenderTerminal(plan, &output, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "pkg") != 5 || !strings.Contains(output.String(), "… (10 au total)") {
		t.Fatalf("terminal output = %q", output.String())
	}
}

func TestRenderMarkdown_ContainsCommands(t *testing.T) {
	plan := &Plan{Steps: []PlanStep{
		{Order: 1, Manager: "dnf", Command: "dnf install -y", Items: []PlanItem{{Source: "htop", Target: "htop"}}},
		{Order: 2, Manager: "flatpak", Command: "flatpak install flathub", Items: []PlanItem{{Source: "Firefox", Target: "org.mozilla.firefox"}}},
	}}
	var output bytes.Buffer
	if err := RenderMarkdown(plan, &output); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"dnf install -y", "flatpak install flathub"} {
		if !strings.Contains(output.String(), command) {
			t.Errorf("Markdown output missing command %q", command)
		}
	}
	if !strings.Contains(output.String(), "dnf install -y \\\n  htop") {
		t.Errorf("Markdown command missing target package: %s", output.String())
	}
}

func TestRenderJSON_RoundTrip(t *testing.T) {
	want := &Plan{
		Steps: []PlanStep{{Order: 1, Manager: "dnf", Action: "install", Command: "dnf install -y",
			Items: []PlanItem{{Source: "htop", Target: "htop", Status: resolver.StatusExact, Confidence: "high"}}}},
		Skipped: []SkippedItem{}, Warnings: []string{},
	}
	var output bytes.Buffer
	if err := RenderJSON(want, &output); err != nil {
		t.Fatal(err)
	}
	var got Plan
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, *want) {
		t.Fatalf("round-trip plan = %#v, want %#v", got, *want)
	}
}

func TestRenderTerminal_ShowsReposFirst(t *testing.T) {
	plan := &Plan{Steps: []PlanStep{
		{
			Order: 1, Manager: "repos", Action: "restore-repos",
			RepoItems: []RepoPlanItem{
				{Repo: manifest.Repo{Name: "fedora"}, TargetPath: "/etc/yum.repos.d/fedora.repo"},
				{Repo: manifest.Repo{Name: "rpmfusion"}, TargetPath: "/etc/yum.repos.d/rpmfusion.repo", RequiresConfirmation: true},
			},
		},
		{Order: 2, Manager: "dnf", Command: "dnf install -y", Items: []PlanItem{{Target: "htop"}}},
	}}
	var output bytes.Buffer
	if err := RenderTerminal(plan, &output, false); err != nil {
		t.Fatal(err)
	}
	if strings.Index(output.String(), "dépôts") > strings.Index(output.String(), "Étape 2 — dnf") ||
		!strings.Contains(output.String(), "[tiers, confirmation requise]") {
		t.Fatalf("terminal output = %s", output.String())
	}
}

func TestRenderMarkdown_MarksThirdPartyRepos(t *testing.T) {
	plan := &Plan{Steps: []PlanStep{{
		Order: 1, Manager: "repos",
		RepoItems: []RepoPlanItem{{
			Repo:       manifest.Repo{Name: "external", ThirdParty: true},
			TargetPath: "/etc/yum.repos.d/external.repo", RequiresConfirmation: true,
		}},
	}}}
	var output bytes.Buffer
	if err := RenderMarkdown(plan, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "**external** ⚠") || !strings.Contains(output.String(), "confirmation requise") {
		t.Fatalf("Markdown output = %s", output.String())
	}
}
