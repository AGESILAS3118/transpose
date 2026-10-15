package resolver

import (
	"reflect"
	"testing"
)

func TestCandidateNames(t *testing.T) {
	tests := []struct {
		source string
		want   []string
	}{
		{"foo-devel", []string{"foo-devel", "foo"}},
		{"foo-libs", []string{"foo-libs", "foo"}},
		{"foo-doc", []string{"foo-doc", "foo"}},
		{"foo-data", []string{"foo-data", "foo"}},
		{"foo-config", []string{"foo-config", "foo"}},
		{"foo-plugin-x", []string{"foo-plugin-x", "foo"}},
		{"libfoo", []string{"libfoo", "foo"}},
		{"python3-foo", []string{"python3-foo", "python-foo", "foo"}},
		{"NetworkManager-adsl", []string{"NetworkManager-adsl", "NetworkManager", "networkmanager", "networkmanager-adsl"}},
		{"akregator", []string{"akregator"}},
		{"Foo", []string{"Foo", "foo"}},
		{"FOO-devel", []string{"FOO-devel", "FOO", "foo-devel"}},
		{"libssl-doc", []string{"libssl-doc", "ssl-doc", "ssl", "libssl"}},
		{"python3-requests-doc", []string{"python3-requests-doc", "python-requests-doc", "requests-doc", "python-requests", "requests", "python3-requests"}},
		{"NetworkManager-wifi", []string{"NetworkManager-wifi", "NetworkManager", "networkmanager", "networkmanager-wifi"}},
		{"foo-plugin-bar-doc", []string{"foo-plugin-bar-doc", "foo-plugin-bar", "foo"}},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			if got := CandidateNames(test.source); !reflect.DeepEqual(got, test.want) {
				t.Errorf("CandidateNames(%q) = %#v, want %#v", test.source, got, test.want)
			}
		})
	}
}
