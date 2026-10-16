package cmd

import (
	"strings"
	"testing"
)

func TestRootCommandContainsExpectedSubcommands(t *testing.T) {
	root := newRootCmd()
	want := []string{"capture", "pack", "plan", "apply", "resolve", "doctor", "diff", "update-db", "repos"}
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

func TestPlaceholderCommandsReturnNotImplemented(t *testing.T) {
	tests := [][]string{
		{"pack", "manifest.yaml"},
		{"apply", "manifest.yaml"},
		{"diff", "source.yaml", "target.yaml"},
	}
	for _, args := range tests {
		root := newRootCmd()
		root.SetArgs(args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), args[0]+": not implemented") {
			t.Errorf("xpose %s error = %v, want not-implemented error", strings.Join(args, " "), err)
		}
	}
}
