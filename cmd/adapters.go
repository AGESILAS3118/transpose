package cmd

import (
	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/adapter/apt"
	"github.com/transpose-project/transpose/internal/adapter/dnf"
	"github.com/transpose-project/transpose/internal/adapter/flatpak"
)

func allAdapters() []adapter.Adapter {
	return []adapter.Adapter{
		dnf.New(nil),
		flatpak.New(nil),
		apt.New(nil),
	}
}
