// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/gnuflag"

	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/configstore"
	"github.com/jameinel/juju/environs/filestorage"
	"github.com/jameinel/juju/environs/imagemetadata"
	"github.com/jameinel/juju/environs/simplestreams"
	"github.com/jameinel/juju/utils"
)

// ImageMetadataCommand is used to write out simplestreams image metadata information.
type ImageMetadataCommand struct {
	cmd.EnvCommandBase
	Dir            string
	Series         string
	Arch           string
	ImageId        string
	Region         string
	Endpoint       string
	privateStorage string
}

var imageMetadataDoc = `
generate-image creates simplestreams image metadata for the specified cloud.

The cloud specification comes from the current Juju environment, as specified in
the usual way from either ~/.juju/environments.yaml, the -e option, or JUJU_ENV.

Using command arguments, it is possible to override cloud attributes region, endpoint, and series.
By default, "amd64" is used for the architecture but this may also be changed.
`

func (c *ImageMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-image",
		Purpose: "generate simplestreams image metadata",
		Doc:     imageMetadataDoc,
	}
}

func (c *ImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "s", "", "the charm series")
	f.StringVar(&c.Arch, "a", "amd64", "the image achitecture")
	f.StringVar(&c.Dir, "d", "", "the destination directory in which to place the metadata files")
	f.StringVar(&c.ImageId, "i", "", "the image id")
	f.StringVar(&c.Region, "r", "", "the region")
	f.StringVar(&c.Endpoint, "u", "", "the cloud endpoint (for Openstack, this is the Identity Service endpoint)")
}

func (c *ImageMetadataCommand) Init(args []string) error {
	c.privateStorage = "<private storage name>"
	var environ environs.Environ
	if store, err := configstore.Default(); err == nil {
		if environ, err = environs.PrepareFromName(c.EnvName, store); err == nil {
			logger.Infof("creating image metadata for environment %q", environ.Name())
			// If the user has not specified region and endpoint, try and get it from the environment.
			if c.Region == "" || c.Endpoint == "" {
				var cloudSpec simplestreams.CloudSpec
				if inst, ok := environ.(simplestreams.HasRegion); ok {
					if cloudSpec, err = inst.Region(); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("environment %q cannot provide region and endpoint", environ.Name())
				}
				// If only one of region or endpoint is provided, that is a problem.
				if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
					return fmt.Errorf("cannot generate metadata without a complete cloud configuration")
				}
				if c.Region == "" {
					c.Region = cloudSpec.Region
				}
				if c.Endpoint == "" {
					c.Endpoint = cloudSpec.Endpoint
				}
			}
			cfg := environ.Config()
			if c.Series == "" {
				c.Series = cfg.DefaultSeries()
			}
			if v, ok := cfg.AllAttrs()["control-bucket"]; ok {
				c.privateStorage = v.(string)
			}
		} else {
			logger.Warningf("environment %q could not be opened: %v", c.EnvName, err)
		}
	}
	if environ == nil {
		logger.Infof("no environment found, creating image metadata using user supplied data")
	}
	if c.Series == "" {
		c.Series = config.DefaultSeries
	}
	if c.ImageId == "" {
		return fmt.Errorf("image id must be specified")
	}
	if c.Region == "" {
		return fmt.Errorf("image region must be specified")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("cloud endpoint URL must be specified")
	}
	if c.Dir == "" {
		logger.Infof("no destination directory specified, using current directory")
		var err error
		if c.Dir, err = os.Getwd(); err != nil {
			return err
		}
	}

	return cmd.CheckEmpty(args)
}

var helpDoc = `
image metadata files have been written to:
%s.
For Juju to use this metadata, the files need to be put into the
image metadata search path. There are 2 options:

1. Use image-metadata-url in $JUJU_HOME/environments.yaml
Configure a http server to serve the contents of
%s
and set the value of image-metadata-url accordingly.

2. Upload the contents of
%s
to your cloud's private storage (for ec2 and openstack).
eg for openstack
"cd %s; swift upload %s images/streams/v1/*"

`

func (c *ImageMetadataCommand) Run(context *cmd.Context) error {
	out := context.Stdout

	im := &imagemetadata.ImageMetadata{
		Id:   c.ImageId,
		Arch: c.Arch,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   c.Region,
		Endpoint: c.Endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(c.Dir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return err
	}
	err = imagemetadata.MergeAndWriteMetadata(c.Series, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return fmt.Errorf("image metadata files could not be created: %v", err)
	}
	dest := filepath.Join(c.Dir, "images", "streams", "v1")
	dir := utils.NormalizePath(c.Dir)
	fmt.Fprintf(out, fmt.Sprintf(helpDoc, dest, dir, dir, dir, c.privateStorage))
	return nil
}
