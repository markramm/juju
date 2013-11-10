// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/state"
	coretesting "github.com/jameinel/juju/testing"
)

var _ = gc.Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

type SSHCommonSuite struct {
	testing.JujuConnSuite
	oldpath string
}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

echo $@
`

func (s *SSHCommonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	path := c.MkDir()
	s.oldpath = os.Getenv("PATH")
	os.Setenv("PATH", path+":"+s.oldpath)
	for _, name := range []string{"ssh", "scp"} {
		f, err := os.OpenFile(filepath.Join(path, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		c.Assert(err, gc.IsNil)
		_, err = f.Write([]byte(fakecommand))
		c.Assert(err, gc.IsNil)
		err = f.Close()
		c.Assert(err, gc.IsNil)
	}
}

func (s *SSHCommonSuite) TearDownTest(c *gc.C) {
	os.Setenv("PATH", s.oldpath)
	s.JujuConnSuite.TearDownTest(c)
}

const (
	commonArgs = `-o StrictHostKeyChecking no -o PasswordAuthentication no `
	sshArgs    = `-l ubuntu -t ` + commonArgs
)

var sshTests = []struct {
	args   []string
	result string
}{
	{
		[]string{"ssh", "0"},
		sshArgs + "dummyenv-0.dns\n",
	},
	// juju ssh 0 'uname -a'
	{
		[]string{"ssh", "0", "uname -a"},
		sshArgs + "dummyenv-0.dns uname -a\n",
	},
	// juju ssh 0 -- uname -a
	{
		[]string{"ssh", "0", "--", "uname", "-a"},
		sshArgs + "dummyenv-0.dns -- uname -a\n",
	},
	// juju ssh 0 uname -a
	{
		[]string{"ssh", "0", "uname", "-a"},
		sshArgs + "dummyenv-0.dns uname -a\n",
	},
	{
		[]string{"ssh", "mysql/0"},
		sshArgs + "dummyenv-0.dns\n",
	},
	{
		[]string{"ssh", "mongodb/1"},
		sshArgs + "dummyenv-2.dns\n",
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
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

	for _, t := range sshTests {
		c.Logf("testing juju ssh %s", t.args)
		ctx := coretesting.Context(c)
		jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
		jujucmd.Register(&SSHCommand{})

		code := cmd.Main(jujucmd, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, t.result)
	}
}

type callbackAttemptStarter struct {
	next func() bool
}

func (s *callbackAttemptStarter) Start() attempt {
	return callbackAttempt{next: s.next}
}

type callbackAttempt struct {
	next func() bool
}

func (a callbackAttempt) Next() bool {
	return a.next()
}

func (s *SSHSuite) TestSSHCommandHostAddressRetry(c *gc.C) {
	m := s.makeMachines(1, c, false)
	ctx := coretesting.Context(c)
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	jujucmd.Register(&SSHCommand{})

	var called int
	next := func() bool {
		called++
		return called < 2
	}
	attemptStarter := &callbackAttemptStarter{next: next}
	s.PatchValue(&sshHostFromTargetAttemptStrategy, attemptStarter)

	// Ensure that the ssh command waits for a public address, or the attempt
	// strategy's Done method returns false.
	code := cmd.Main(jujucmd, ctx, []string{"ssh", "0"})
	c.Check(code, gc.Equals, 1)
	c.Assert(called, gc.Equals, 2)
	called = 0
	attemptStarter.next = func() bool {
		called++
		s.setAddress(m[0], c)
		return false
	}
	code = cmd.Main(jujucmd, ctx, []string{"ssh", "0"})
	c.Check(code, gc.Equals, 0)
	c.Assert(called, gc.Equals, 1)
}

func (s *SSHCommonSuite) setAddress(m *state.Machine, c *gc.C) {
	addr := instance.NewAddress(fmt.Sprintf("dummyenv-%s.dns", m.Id()))
	addr.NetworkScope = instance.NetworkPublic
	err := m.SetAddresses([]instance.Address{addr})
	c.Assert(err, gc.IsNil)
}

func (s *SSHCommonSuite) makeMachines(n int, c *gc.C, setAddress bool) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, gc.IsNil)
		if setAddress {
			s.setAddress(m, c)
		}
		// must set an instance id as the ssh command uses that as a signal the
		// machine has been provisioned
		inst, md := testing.AssertStartInstance(c, s.Conn.Environ, m.Id())
		c.Assert(m.SetProvisioned(inst.Id(), "fake_nonce", md), gc.IsNil)
		machines[i] = m
	}
	return machines
}

func (s *SSHCommonSuite) addUnit(srv *state.Service, m *state.Machine, c *gc.C) {
	u, err := srv.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	// fudge unit.SetPublicAddress
	id, err := m.InstanceId()
	c.Assert(err, gc.IsNil)
	insts, err := s.Conn.Environ.Instances([]instance.Id{id})
	c.Assert(err, gc.IsNil)
	addr, err := insts[0].WaitDNSName()
	c.Assert(err, gc.IsNil)
	err = u.SetPublicAddress(addr)
	c.Assert(err, gc.IsNil)
}
