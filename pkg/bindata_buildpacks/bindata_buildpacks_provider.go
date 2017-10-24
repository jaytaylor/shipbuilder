package bindata_buildpacks

import (
	"sort"
	"strings"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks/data"
	"github.com/jaytaylor/shipbuilder/pkg/domain"
)

type BindataBuildpacksProvider struct{}

func NewProvider() *BindataBuildpacksProvider {
	provider := &BindataBuildpacksProvider{}
	return provider
}

// New locates and constructs the corresponding buildpack.
func (provider BindataBuildpacksProvider) New(name string) (domain.Buildpack, error) {
	return New(name)
}

// Available returns the listing of available buildpacks.
func (provider BindataBuildpacksProvider) Available() []string {
	m := map[string]struct{}{}
	for _, name := range data.AssetNames() {
		name = strings.Split(name, "/")[0]
		if _, ok := m[name]; !ok {
			m[name] = struct{}{}
		}
	}
	names := []string{}
	for name, _ := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// All returns all buildpacks.
func (provider BindataBuildpacksProvider) All() []domain.Buildpack {
	bps := []domain.Buildpack{}

	for _, name := range provider.Available() {
		bp, _ := provider.New(name)
		bps = append(bps, bp)
	}

	return bps
}
