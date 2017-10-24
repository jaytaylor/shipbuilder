package core

import (
	"testing"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/releases"
)

func TestDynoPortAllocation(t *testing.T) {
	server := &Server{
		BuildpacksProvider: bindata_buildpacks.NewProvider(),
		ReleasesProvider:   releases.NewFSReleasesProvider("/tmp/sb-dyno-test"),
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
}
