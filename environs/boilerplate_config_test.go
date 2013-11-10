// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	_ "github.com/jameinel/juju/provider/ec2"
	_ "github.com/jameinel/juju/provider/openstack"
)

type BoilerplateConfigSuite struct {
}

var _ = gc.Suite(&BoilerplateConfigSuite{})

func (*BoilerplateConfigSuite) TestBoilerPlateGeneration(c *gc.C) {
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	boilerplate_text := environs.BoilerplateConfig()
	_, err := environs.ReadEnvironsBytes([]byte(boilerplate_text))
	c.Assert(err, gc.IsNil)
}
