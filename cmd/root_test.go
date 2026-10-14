package cmd

import (
	"testing"
)

func TestRootCommandContainsExpectedSubcommands(t *testing.T) {
	root := newRootCmd()
	want := []string{"capture", "pack", "plan", "apply", "resolve", "doctor", "diff", "update-db"}
	found := make(map[string]bool, len(want))

	for _, command := range root.Commands() {
		found[command.Name()] = true
	}

	for _, name := range want {
		if !found[name] {
			t.Errorf("root command is missing subcommand %q", name)
		}
	}

	if len(root.Commands()) != len(want) {
		t.Errorf("got %d subcommands, want %d", len(root.Commands()), len(want))
	}
}
