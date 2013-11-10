// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/configstore"
	sstesting "github.com/jameinel/juju/environs/simplestreams/testing"
	"github.com/jameinel/juju/environs/storage"
	"github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/provider/dummy"
	"github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
)

type URLsSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *URLsSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	dummy.Reset()
}

func (s *URLsSuite) env(c *gc.C, toolsMetadataURL string) environs.Environ {
	attrs := dummy.SampleConfig()
	if toolsMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"tools-metadata-url": toolsMetadataURL,
		})
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "https://streams.canonical.com/juju/tools/"})
}

func (s *URLsSuite) TestToolsSources(c *gc.C) {
	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-tools-metadata-url/", privateStorageURL, "https://streams.canonical.com/juju/tools/"})
	haveExpectedSources := false
	for _, source := range sources {
		if allowRetry, ok := storage.TestingGetAllowRetry(source); ok {
			haveExpectedSources = true
			c.Assert(allowRetry, jc.IsFalse)
		}
	}
	c.Assert(haveExpectedSources, jc.IsTrue)
}

func (s *URLsSuite) TestToolsSourcesWithRetry(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSourcesWithRetries(env, true)
	c.Assert(err, gc.IsNil)
	haveExpectedSources := false
	for _, source := range sources {
		if allowRetry, ok := storage.TestingGetAllowRetry(source); ok {
			haveExpectedSources = true
			c.Assert(allowRetry, jc.IsTrue)
		}
	}
	c.Assert(haveExpectedSources, jc.IsTrue)
	c.Assert(haveExpectedSources, jc.IsTrue)
}
