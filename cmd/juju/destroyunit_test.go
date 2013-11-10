// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/charm"
	jujutesting "github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/testing"
)

type DestroyUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&DestroyUnitSuite{})

func runDestroyUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, &DestroyUnitCommand{}, args)
	return err
}

func (s *DestroyUnitSuite) TestDestroyUnit(c *gc.C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	svc, _ := s.AssertService(c, "dummy", curl, 2, 0)

	err = runDestroyUnit(c, "dummy/0", "dummy/1", "dummy/2", "sillybilly/17")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "dummy/2" does not exist; unit "sillybilly/17" does not exist`)
	units, err := svc.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, u := range units {
		c.Assert(u.Life(), gc.Equals, state.Dying)
	}
}
