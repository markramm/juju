// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api/params"
	"github.com/jameinel/juju/state/apiserver/common"
	apiservertesting "github.com/jameinel/juju/state/apiserver/testing"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/version"
)

type toolsSuite struct {
	testing.JujuConnSuite
	machine0 *state.Machine
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
}

func (s *toolsSuite) TestTools(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return tag == "machine-0" || tag == "machine-42"
		}, nil
	}
	tg := common.NewToolsGetter(s.State, getCanRead)
	c.Assert(tg, gc.NotNil)

	err := s.machine0.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		}}
	result, err := tg.Tools(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Tools, gc.NotNil)
	c.Assert(result.Results[0].Tools.Version, gc.DeepEquals, version.Current)
	c.Assert(result.Results[0].DisableSSLHostnameVerification, jc.IsFalse)
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *toolsSuite) TestToolsError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(s.State, getCanRead)
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}
