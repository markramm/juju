// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"github.com/jameinel/juju/container/lxc"
	"github.com/jameinel/juju/provider/local"
	"github.com/jameinel/juju/testing"
)

type baseProviderSuite struct {
	lxc.TestSuite
	home    *testing.FakeHome
	restore func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "test")
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.restore = local.MockAddressForInterface()
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restore()
	s.home.Restore()
	s.TestSuite.TearDownTest(c)
}
