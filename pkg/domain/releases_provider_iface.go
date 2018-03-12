package domain

import (
	"io"
)

// ReleasesProvider defines the interface to be implemented by release managers.
type ReleasesProvider interface {
	// List returns the list of releases for an application.
	List(applicationName string) ([]Release, error)

	// Set sets the list of releases for an application.
	Set(applicationName string, releases []Release) error

	// Delete removes all releases for an application.
	Delete(applicationName string, logger io.Writer) error

	// Store adds a new release to the set of releases.
	Store(applicationName string, version string, rs io.ReadSeeker, length int64) error

	// Get retrieves a specific release.
	Get(applicationName string, version string) (*Release, error)
}
