package core

// Buildpack contains the interface to be implemented by Buildpack providers.
type Buildpack interface {
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
type BuildpacksProvider func(name string) (Buildpack, error)
