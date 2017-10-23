package bindata_buildpacks

import (
	"bytes"
	"errors"
	"strings"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks/data"

	"github.com/gigawattio/errorlib"
)

type BindataBuildpack struct {
	containerCustomCommands []byte
	containerPackages       []byte
	preHook                 []byte
}

// New retrieves raw buildpack data from gobindata compiled assets and
// constructs a new BindataBuildpack from it.
func New(name string) (*BindataBuildpack, error) {
	errs := []error{}

	containerCustomCommands, err := data.Asset(name + "/container-custom-commands")
	if err != nil {
		errs = append(errs, errors.New("missing required asset: container-custom-commands"))
	}

	containerPackages, err := data.Asset(name + "/container-packages")
	if err != nil {
		errs = append(errs, errors.New("missing required asset: container-packages"))
	}

	preHook, err := data.Asset(name + "/pre-hook")
	if err != nil {
		errs = append(errs, errors.New("missing required asset: pre-hook"))
	}

	if err := errorlib.Merge(errs); err != nil {
		return nil, err
	}

	bp := &BindataBuildpack{
		containerCustomCommands: containerCustomCommands,
		containerPackages:       bytes.Trim(containerPackages, "\r\n"),
		preHook:                 preHook,
	}

	return bp, nil
}

// ContainerCustomCommands returns string containing the corresponding bash
// script.
func (bp BindataBuildpack) ContainerCustomCommands() string {
	return string(bp.containerCustomCommands)
}

// ContainerPackages returns the list of container packages to install into the
// buildpacks base container image.
func (bp BindataBuildpack) ContainerPackages() []string {
	return strings.Split(string(bp.containerPackages), "\n")
}

// PreHook returns string containing the corresponding bash script.
func (bp BindataBuildpack) PreHook() string {
	return string(bp.preHook)
}
