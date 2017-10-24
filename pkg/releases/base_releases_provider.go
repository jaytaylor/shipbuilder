package releases

import (
	"encoding/json"
	"errors"

	"github.com/jaytaylor/shipbuilder/pkg/domain"
)

// baseReleasesProvider contains the common implementation bits of a domain.ReleasesProvider.
type baseReleasesProvider struct {
}

func (_ *baseReleasesProvider) parseManifest(data []byte) ([]domain.Release, error) {
	releases := []domain.Release{}
	if err := json.Unmarshal(data, releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func (_ *baseReleasesProvider) createManifest(releases []domain.Release) ([]byte, error) {
	data, err := json.Marshal(releases)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (_ *baseReleasesProvider) find(version string, releases []domain.Release) (*domain.Release, error) {
	for _, release := range releases {
		if release.Version == version {
			return &release, nil
		}
	}
	return nil, errors.New("release not found")
}
