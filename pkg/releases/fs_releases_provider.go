package releases

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/jaytaylor/shipbuilder/pkg/domain"

	"github.com/gigawattio/oslib"
	log "github.com/sirupsen/logrus"
)

// FSReleasesProvider is a filesystem-based releases provider.
type FSReleasesProvider struct {
	baseReleasesProvider

	path string
}

// NewFSReleasesProvider attempts to create the path if it doesn't exist, or
// checks that it is a valid directory.  Returns a new instance of
// *NewFSReleasesProvider.
func NewFSReleasesProvider(path string) *FSReleasesProvider {
	provider := &FSReleasesProvider{
		path: path,
	}
	return provider
}

// List returns the list of releases for an application.
func (provider *FSReleasesProvider) List(applicationName string) ([]domain.Release, error) {
	data, err := ioutil.ReadFile(provider.manifestPath(applicationName))
	if err != nil {
		return nil, err
	}
	releases, err := provider.parseManifest(data)
	if err != nil {
		return nil, err
	}
	return releases, nil
}

// Set sets the list of releases for an application.
func (provider *FSReleasesProvider) Set(applicationName string, releases []domain.Release) error {
	data, err := provider.createManifest(releases)
	if err != nil {
		return err
	}

	var (
		manifest = provider.manifestPath(applicationName)
		dir      = oslib.PathDirName(manifest)
	)

	if err := os.MkdirAll(dir, os.FileMode(int(0700))); err != nil {
		return fmt.Errorf("creating path %q: %s", dir, err)
	}
	if err := ioutil.WriteFile(manifest, data, os.FileMode(int(0600))); err != nil {
		return fmt.Errorf("writing manifest file %q: %s", manifest, err)
	}
	return nil
}

// Delete removes all releases for an application.
func (provider *FSReleasesProvider) Delete(applicationName string, logger io.Writer) error {
	if err := os.RemoveAll(fmt.Sprintf("%v%v%v", provider.path, string(os.PathSeparator), applicationName)); err != nil {
		return err
	}
	return nil
}

// Store adds a new release to the set of releases.
func (provider *FSReleasesProvider) Store(applicationName string, version string, r io.Reader, length int64) error {
	archive := provider.releasePath(applicationName, version)
	fd, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(int(0600)))
	if err != nil {
		return fmt.Errorf("creating archive file %v: %s", archive, err)
	}
	go func() {
		w := bufio.NewWriter(fd)
		if _, err := w.ReadFrom(r); err != nil {
			log.Errorf("Problem writing archive file %q: %s", archive, err)
		}
	}()

	return nil
}

// Get retrieves a specific release.
func (provider *FSReleasesProvider) Get(applicationName string, version string) (*domain.Release, error) {
	releases, err := provider.List(applicationName)
	if err != nil {
		return nil, err
	}
	return provider.find(version, releases)
}

func (provider *FSReleasesProvider) manifestPath(applicationName string) string {
	manifest := fmt.Sprintf("%[1]v%[2]v%[3]v%[2]vmanifest.json", provider.path, string(os.PathSeparator), applicationName)
	return manifest
}

func (provider *FSReleasesProvider) releasePath(applicationName string, version string) string {
	archive := fmt.Sprintf("%[1]v%[2]v%[3]v%[2]v%[4]v.tar.gz", provider.path, string(os.PathSeparator), applicationName, version)
	return archive
}
