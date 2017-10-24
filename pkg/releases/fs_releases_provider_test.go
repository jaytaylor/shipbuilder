package releases

import (
	"os"
	"testing"
	"time"

	"github.com/jaytaylor/shipbuilder/pkg/domain"
)

func TestFSReleasesProvider(t *testing.T) {
	const path = "/tmp/sb-fs-releases-provider"
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("Removing path %q: %s", path, err)
	}

	// defer func() {
	// 	if err := os.RemoveAll(path); err != nil {
	// 		t.Fatalf("Removing path %q: %s", path, err)
	// 	}
	// }()

	provider := NewFSReleasesProvider(path)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	t.Logf("provider=%+v", provider)

	releases := []domain.Release{
		{
			Config: map[string]string{
				"bar": "baz",
			},
			Date:     time.Now(),
			Revision: "a1b2c3",
			Version:  "v1",
		},
	}

	if err := provider.Set("foo", releases); err != nil {
		t.Fatal(err)
	}
}
