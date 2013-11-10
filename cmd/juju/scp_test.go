// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"net/url"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/cmd"
	coretesting "github.com/jameinel/juju/testing"
)

var _ = gc.Suite(&SCPSuite{})

type SCPSuite struct {
	SSHCommonSuite
}

var scpTests = []struct {
	args   []string
	result string
}{
	{
		[]string{"0:foo", "."},
		commonArgs + "ubuntu@dummyenv-0.dns:foo .\n",
	},
	{
		[]string{"foo", "0:"},
		commonArgs + "foo ubuntu@dummyenv-0.dns:\n",
	},
	{
		[]string{"0:foo", "mysql/0:/foo"},
		commonArgs + "ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n",
	},
	{
		[]string{"a", "b", "mysql/0"},
		commonArgs + "a b mysql/0\n",
	},
	{
		[]string{"mongodb/1:foo", "mongodb/0:"},
		commonArgs + "ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns:\n",
	},
}

func (s *SCPSuite) TestSCPCommand(c *gc.C) {
	m := s.makeMachines(3, c, true)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)
	srv, err := s.State.AddService("mysql", dummy)
	c.Assert(err, gc.IsNil)
	s.addUnit(srv, m[0], c)

	srv, err = s.State.AddService("mongodb", dummy)
	c.Assert(err, gc.IsNil)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for _, t := range scpTests {
		c.Logf("testing juju scp %s", t.args)
		ctx := coretesting.Context(c)
		code := cmd.Main(&SCPCommand{}, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, t.result)
	}
}
