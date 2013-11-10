// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/testing/testbase"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type DependencySuite struct{}

var _ = gc.Suite(&DependencySuite{})

func (*DependencySuite) TestPackageDependencies(c *gc.C) {
	// This test is to ensure we don't bring in any juju-core dependencies.
	c.Assert(testbase.FindJujuCoreImports(c, "github.com/jameinel/juju/testing/testbase"),
		gc.HasLen, 0)
}
