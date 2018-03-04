package core

import (
	"testing"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/releases"
)

func TestDynoPortAllocation(t *testing.T) {
	server := &Server{
		ListenAddr:          ":",
		LogServerListenAddr: ":59595",
		BuildpacksProvider:  bindata_buildpacks.NewProvider(),
		ReleasesProvider:    releases.NewFSReleasesProvider("/tmp/sb-dyno-test"),
		ConfigFile:          "/tmp/sb-test.json",
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
}
