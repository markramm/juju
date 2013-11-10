// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/configstore"
	"github.com/jameinel/juju/environs/imagemetadata"
	sstesting "github.com/jameinel/juju/environs/simplestreams/testing"
	"github.com/jameinel/juju/provider/dummy"
	"github.com/jameinel/juju/testing"
)

type URLsSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *URLsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.home.Restore()
}

func (s *URLsSuite) env(c *gc.C, imageMetadataURL string) environs.Environ {
	attrs := dummy.SampleConfig()
	if imageMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"image-metadata-url": imageMetadataURL,
		})
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestImageMetadataURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "")
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("images")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "http://cloud-images.ubuntu.com/releases/"})
}

func (s *URLsSuite) TestImageMetadataURLs(c *gc.C) {
	env := s.env(c, "config-image-metadata-url")
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("images")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-image-metadata-url/", privateStorageURL, "http://cloud-images.ubuntu.com/releases/"})
}
