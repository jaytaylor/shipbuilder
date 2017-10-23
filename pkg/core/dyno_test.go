package core

import (
	"testing"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
)

func TestDynoPortAllocation(t *testing.T) {
	server := &Server{
		BuildpacksProvider: bindata_buildpacks.New,
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
}
