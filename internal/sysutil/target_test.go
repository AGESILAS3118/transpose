package sysutil

import "testing"

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input  string
		distro string
		family string
	}{
		{input: "fedora-44", distro: "fedora", family: "rpm"},
		{input: "debian_13", distro: "debian", family: "debian"},
		{input: "arch", distro: "arch", family: "arch"},
		{input: "ubuntu-24.04", distro: "ubuntu", family: "debian"},
		{input: "opensuse-leap-15.6", distro: "opensuse-leap", family: "suse"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseTarget(test.input)
			if err != nil {
				t.Fatalf("ParseTarget() error = %v", err)
			}
			if got.Distro != test.distro || got.Family != test.family {
				t.Errorf("ParseTarget() = %#v, want distro %q family %q", got, test.distro, test.family)
			}
		})
	}
	for _, input := range []string{"", "unknown-1", "fedora-", "-44"} {
		if _, err := ParseTarget(input); err == nil {
			t.Errorf("ParseTarget(%q) succeeded, want error", input)
		}
	}
}

func TestSameFamily(t *testing.T) {
	if !SameFamily(Target{Family: "rpm"}, Target{Family: "rpm"}) {
		t.Error("rpm/rpm should be same family")
	}
	if SameFamily(Target{Family: "rpm"}, Target{Family: "debian"}) {
		t.Error("rpm/debian should not be same family")
	}
}
