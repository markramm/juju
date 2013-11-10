// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/state/api/machiner"
	"github.com/jameinel/juju/state/api/params"
	statetesting "github.com/jameinel/juju/state/testing"
	coretesting "github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type machinerSuite struct {
	testing.JujuConnSuite
	st      *api.State
	machine *state.Machine

	machiner *machiner.State
}

var _ = gc.Suite(&machinerSuite{})

func (s *machinerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.machiner = s.st.Machiner()
	c.Assert(s.machiner, gc.NotNil)
}

func (s *machinerSuite) TestMachineAndMachineTag(c *gc.C) {
	machine, err := s.machiner.Machine("machine-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(machine, gc.IsNil)

	machine, err = s.machiner.Machine("machine-0")
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Tag(), gc.Equals, "machine-0")
}

func (s *machinerSuite) TestSetStatus(c *gc.C) {
	machine, err := s.machiner.Machine("machine-0")
	c.Assert(err, gc.IsNil)

	status, info, data, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = machine.SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)

	status, info, data, err = s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
	c.Assert(data, gc.HasLen, 0)
}

func (s *machinerSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)

	machine, err := s.machiner.Machine("machine-0")
	c.Assert(err, gc.IsNil)

	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)

	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)

	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)

	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)
	err = s.machine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	err = machine.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *machinerSuite) TestRefresh(c *gc.C) {
	machine, err := s.machiner.Machine("machine-0")
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Life(), gc.Equals, params.Alive)

	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Life(), gc.Equals, params.Alive)

	err = machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Life(), gc.Equals, params.Dead)
}

func (s *machinerSuite) TestWatch(c *gc.C) {
	machine, err := s.machiner.Machine("machine-0")
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Life(), gc.Equals, params.Alive)

	w, err := machine.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = machine.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the machine dead and check it's detected.
	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
