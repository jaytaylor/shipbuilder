package core

// Buildpack contains the interface to be implemented by Buildpack providers.
type Buildpack interface {
	// Name identifier of the buildpack.
	Name() string

	// ContainerCustomCommands returns a string of the bash script commands unique
	// to the buildpack.
	ContainerCustomCommands() string

	// ContainerPackages returns an array of packages to install to the container
	// for the buildpack.
	ContainerPackages() []string

	// PreHook returns a string of the bash script to be run upon receipt of new
	// app git contents.
	PreHook() string
}

// BuildpacksProvider defines a generalized function type for constructing and
// retrieving buildpacks.
type BuildpacksProvider interface {

	// New locates and constructs the corresponding buildpack.
	New(name string) (Buildpack, error)

	// Available returns the listing of available buildpacks.
	Available() []string

	// All returns all buildpacks.
	All() []Buildpack
}
