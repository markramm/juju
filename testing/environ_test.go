// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/juju/osenv"
	"github.com/jameinel/juju/testing"
)

type TestingEnvironSuite struct {
	home     string
	jujuHome string
}

var _ = gc.Suite(&TestingEnvironSuite{})

func (s *TestingEnvironSuite) SetUpTest(c *gc.C) {
	s.home = osenv.Home()
	s.jujuHome = os.Getenv("JUJU_HOME")

	osenv.SetHome("/home/eric")
	os.Setenv("JUJU_HOME", "/home/eric/juju")
	config.SetJujuHome("/home/eric/juju")
}

func (s *TestingEnvironSuite) TearDownTest(c *gc.C) {
	osenv.SetHome(s.home)
	os.Setenv("JUJU_HOME", s.jujuHome)
}

func (s *TestingEnvironSuite) TestFakeHomeReplacesEnvironment(c *gc.C) {
	_ = testing.MakeEmptyFakeHome(c)
	c.Assert(osenv.Home(), gc.Not(gc.Equals), "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, "")
	c.Assert(config.JujuHome(), gc.Not(gc.Equals), "/home/eric/juju")
}

func (s *TestingEnvironSuite) TestFakeHomeRestoresEnvironment(c *gc.C) {
	fake := testing.MakeEmptyFakeHome(c)
	fake.Restore()
	c.Assert(osenv.Home(), gc.Equals, "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, "/home/eric/juju")
	c.Assert(config.JujuHome(), gc.Equals, "/home/eric/juju")
}

func (s *TestingEnvironSuite) TestFakeHomeSetsConfigJujuHome(c *gc.C) {
	_ = testing.MakeEmptyFakeHome(c)
	expected := filepath.Join(osenv.Home(), ".juju")
	c.Assert(config.JujuHome(), gc.Equals, expected)
}
