// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/testing/testbase"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies at all.
	c.Assert(testbase.FindJujuCoreImports(c, "github.com/jameinel/juju/juju/osenv"),
		gc.HasLen, 0)
}
