package bindata_buildpacks

import (
	"testing"
)

func TestBindataBuildpacks(t *testing.T) {
	available := []string{
		"java8-mvn",
		"java9-mvn",
		"nodejs",
		"playframework2",
		"python",
		"scala-sbt",
	}
	for _, name := range available {
		bp, err := New(name)
		if err != nil {
			t.Errorf("Error constructing BindataBuildpack for name=%v: %s (first thing to check: has `make generate' been run?", name, err)
			continue
		}
		if bp == nil {
			t.Errorf("Unexpectedly nil buildpack received for name=%v", name)
		}
	}
}
