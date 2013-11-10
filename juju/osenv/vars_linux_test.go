package osenv_test

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/juju/osenv"
	"github.com/jameinel/juju/testing/testbase"
)

func (*importSuite) TestHomeLinux(c *gc.C) {
	h := "/home/foo/bar"
	testbase.PatchEnvironment("HOME", h)
	c.Check(osenv.Home(), gc.Equals, h)
}
