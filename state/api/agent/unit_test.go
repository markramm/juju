// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/state/api/params"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/utils"
)

var _ = gc.Suite(&unitSuite{})

type unitSuite struct {
	testing.JujuConnSuite
	unit *state.Unit
	st   *api.State
}

func (s *unitSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetPassword(password)

	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
}

func (s *unitSuite) TestUnitEntity(c *gc.C) {
	m, err := s.st.Agent().Entity("wordpress/1")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, gc.IsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(m.Tag(), gc.Equals, s.unit.Tag())
	c.Assert(m.Life(), gc.Equals, params.Alive)
	c.Assert(m.Jobs(), gc.HasLen, 0)

	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("unit %q not found", s.unit.Name()))
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, gc.IsNil)
}
