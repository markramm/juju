// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"labix.org/v2/mgo/bson"
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api/params"
	statetesting "github.com/jameinel/juju/state/testing"
	"github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/utils"
	"github.com/jameinel/juju/version"
)

type D []bson.DocElem

var goodPassword = "foo-12345678901234567890"
var alternatePassword = "bar-12345678901234567890"

// preventUnitDestroyRemove sets a non-pending status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	err := u.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
}

type StateSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StateSuite{})

func (s *StateSuite) TestDialAgain(c *gc.C) {
	// Ensure idempotent operations on Dial are working fine.
	for i := 0; i < 2; i++ {
		st, err := state.Open(state.TestingStateInfo(), state.TestingDialOpts())
		c.Assert(err, gc.IsNil)
		c.Assert(st.Close(), gc.IsNil)
	}
}

func (s *StateSuite) TestAddresses(c *gc.C) {
	var err error
	machines := make([]*state.Machine, 3)
	machines[0], err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	machines[1], err = s.State.AddMachine("quantal", state.JobManageState, state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	machines[2], err = s.State.AddMachine("quantal", state.JobManageState)
	c.Assert(err, gc.IsNil)

	for i, m := range machines {
		err := m.SetAddresses([]instance.Address{{
			Type:         instance.Ipv4Address,
			NetworkScope: instance.NetworkCloudLocal,
			Value:        fmt.Sprintf("10.0.0.%d", i),
		}, {
			Type:         instance.Ipv6Address,
			NetworkScope: instance.NetworkCloudLocal,
			Value:        "::1",
		}, {
			Type:         instance.Ipv4Address,
			NetworkScope: instance.NetworkMachineLocal,
			Value:        "127.0.0.1",
		}, {
			Type:         instance.Ipv4Address,
			NetworkScope: instance.NetworkPublic,
			Value:        "5.4.3.2",
		}})
		c.Assert(err, gc.IsNil)
	}
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	addrs, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 2)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.1:%d", envConfig.StatePort()),
		fmt.Sprintf("10.0.0.2:%d", envConfig.StatePort()),
	})

	addrs, err = s.State.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 2)
	c.Assert(addrs, jc.SameContents, []string{
		fmt.Sprintf("10.0.0.1:%d", envConfig.APIPort()),
		fmt.Sprintf("10.0.0.2:%d", envConfig.APIPort()),
	})
}

func (s *StateSuite) TestPing(c *gc.C) {
	c.Assert(s.State.Ping(), gc.IsNil)
	testing.MgoRestart()
	c.Assert(s.State.Ping(), gc.NotNil)
}

func (s *StateSuite) TestIsNotFound(c *gc.C) {
	err1 := fmt.Errorf("unrelated error")
	err2 := errors.NotFoundf("foo")
	c.Assert(err1, gc.Not(jc.Satisfies), errors.IsNotFoundError)
	c.Assert(err2, jc.Satisfies, errors.IsNotFoundError)
}

func (s *StateSuite) TestAddCharm(c *gc.C) {
	// Check that adding charms from scratch works correctly.
	ch := testing.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)
	c.Assert(dummy.URL().String(), gc.Equals, curl.String())

	doc := state.CharmDoc{}
	err = s.charms.FindId(curl).One(&doc)
	c.Assert(err, gc.IsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, gc.DeepEquals, curl)
}

func (s *StateSuite) AssertMachineCount(c *gc.C, expect int) {
	ms, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(len(ms), gc.Equals, expect)
}

var jobStringTests = []struct {
	job state.MachineJob
	s   string
}{
	{state.JobHostUnits, "JobHostUnits"},
	{state.JobManageEnviron, "JobManageEnviron"},
	{state.JobManageState, "JobManageState"},
	{0, "<unknown job 0>"},
	{5, "<unknown job 5>"},
}

func (s *StateSuite) TestJobString(c *gc.C) {
	for _, t := range jobStringTests {
		c.Check(t.job.String(), gc.Equals, t.s)
	}
}

func (s *StateSuite) TestAddMachineErrors(c *gc.C) {
	_, err := s.State.AddMachine("")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
	_, err = s.State.AddMachine("quantal")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
	_, err = s.State.AddMachine("quantal", state.JobHostUnits, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: duplicate job: .*")
}

func (s *StateSuite) TestAddMachines(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, gc.IsNil)
	check := func(m *state.Machine, id, series string, jobs []state.MachineJob) {
		c.Assert(m.Id(), gc.Equals, id)
		c.Assert(m.Series(), gc.Equals, series)
		c.Assert(m.Jobs(), gc.DeepEquals, jobs)
		s.assertMachineContainers(c, m, nil)
	}
	check(m0, "0", "quantal", oneJob)
	m0, err = s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	check(m0, "0", "quantal", oneJob)

	allJobs := []state.MachineJob{
		state.JobHostUnits,
		state.JobManageEnviron,
		state.JobManageState,
	}
	m1, err := s.State.AddMachine("blahblah", allJobs...)
	c.Assert(err, gc.IsNil)
	check(m1, "1", "blahblah", allJobs)

	m1, err = s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	check(m1, "1", "blahblah", allJobs)

	m, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.HasLen, 2)
	check(m[0], "0", "quantal", oneJob)
	check(m[1], "1", "blahblah", allJobs)
}

func (s *StateSuite) TestAddMachineExtraConstraints(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, gc.IsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cpu-cores=4")
	params := state.AddMachineParams{
		Series:      "quantal",
		Constraints: extraCons,
		Jobs:        oneJob,
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	expectedCons := constraints.MustParse("cpu-cores=4 mem=4G")
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(mcons, gc.DeepEquals, expectedCons)
}

func (s *StateSuite) assertMachineContainers(c *gc.C, m *state.Machine, containers []string) {
	mc, err := m.Containers()
	c.Assert(err, gc.IsNil)
	c.Assert(mc, gc.DeepEquals, containers)
}

func (s *StateSuite) TestAddContainerToNewMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	params := state.AddMachineParams{
		ContainerType: instance.LXC,
		Series:        "quantal",
		Jobs:          oneJob,
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)

	m, err = s.State.Machine("0")
	c.Assert(err, gc.IsNil)
	s.assertMachineContainers(c, m, []string{"0/lxc/0"})
	m, err = s.State.Machine("0/lxc/0")
	c.Assert(err, gc.IsNil)
	s.assertMachineContainers(c, m, nil)
}

func (s *StateSuite) TestAddContainerToExistingMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	m0, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, gc.IsNil)
	m1, err := s.State.AddMachine("quantal", oneJob...)
	c.Assert(err, gc.IsNil)

	// Add first container.
	params := state.AddMachineParams{
		ParentId:      "1",
		ContainerType: instance.LXC,
		Series:        "quantal",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})

	s.assertMachineContainers(c, m0, nil)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0"})
	m, err = s.State.Machine("1/lxc/0")
	c.Assert(err, gc.IsNil)
	s.assertMachineContainers(c, m, nil)

	// Add second container.
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "1/lxc/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m1, []string{"1/lxc/0", "1/lxc/1"})
}

func (s *StateSuite) TestAddContainerWithConstraints(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	cons := constraints.MustParse("mem=4G")

	params := state.AddMachineParams{
		ParentId:      "",
		ContainerType: instance.LXC,
		Series:        "quantal",
		Constraints:   cons,
		Jobs:          oneJob,
	}
	params.InstanceId = "id"
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.ErrorMatches, "cannot specify an instance id when adding a new machine")
	params.InstanceId = ""
	params.Nonce = "nonce"
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.ErrorMatches, "cannot specify a nonce when adding a new machine")
	params.Nonce = ""
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, mcons)
}

func (s *StateSuite) TestAddContainerErrors(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}

	params := state.AddMachineParams{
		ParentId:      "10",
		ContainerType: instance.LXC,
		Series:        "quantal",
		Jobs:          oneJob,
	}
	_, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.ErrorMatches, "cannot add a new container: machine 10 not found")
	params.ContainerType = ""
	_, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.ErrorMatches, "cannot add a new container: no container type specified")
}

func (s *StateSuite) TestInjectMachineErrors(c *gc.C) {
	injectMachine := func(series, instanceId, nonce string, jobs ...state.MachineJob) error {
		params := &state.AddMachineParams{
			InstanceId: instance.Id(instanceId),
			Series:     series,
			Nonce:      nonce,
			Jobs:       jobs,
		}
		_, err := s.State.InjectMachine(params)
		return err
	}
	err := injectMachine("", "i-minvalid", state.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no series specified")
	err = injectMachine("quantal", "", state.BootstrapNonce, state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot inject a machine without an instance id")
	err = injectMachine("quantal", "i-minvalid", "", state.JobHostUnits)
	c.Assert(err, gc.ErrorMatches, "cannot inject a machine without a nonce")
	err = injectMachine("quantal", state.BootstrapNonce, "i-mlazy")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: no jobs specified")
}

func (s *StateSuite) TestInjectMachine(c *gc.C) {
	cons := constraints.MustParse("mem=4G")
	arch := "amd64"
	mem := uint64(1024)
	disk := uint64(1024)
	tags := []string{"foo", "bar"}
	params := &state.AddMachineParams{
		Series:      "quantal",
		Constraints: cons,
		InstanceId:  instance.Id("i-mindustrious"),
		Nonce:       state.BootstrapNonce,
		HardwareCharacteristics: instance.HardwareCharacteristics{
			Arch:     &arch,
			Mem:      &mem,
			RootDisk: &disk,
			Tags:     &tags,
		},
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	}
	m, err := s.State.InjectMachine(params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, params.Jobs)
	instanceId, err := m.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instanceId, gc.Equals, params.InstanceId)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, mcons)
	characteristics, err := m.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(*characteristics, gc.DeepEquals, params.HardwareCharacteristics)

	// Make sure the bootstrap nonce value is set.
	c.Assert(m.CheckProvisioned(params.Nonce), gc.Equals, true)
}

func (s *StateSuite) TestAddContainerToInjectedMachine(c *gc.C) {
	oneJob := []state.MachineJob{state.JobHostUnits}
	params := state.AddMachineParams{
		Series:     "quantal",
		InstanceId: instance.Id("i-mindustrious"),
		Nonce:      state.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	}
	m0, err := s.State.InjectMachine(&params)
	c.Assert(err, gc.IsNil)

	// Add first container.
	params = state.AddMachineParams{
		ParentId:      "0",
		ContainerType: instance.LXC,
		Series:        "quantal",
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/0")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	mcons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0"})

	// Add second container.
	m, err = s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, "0/lxc/1")
	c.Assert(m.Series(), gc.Equals, "quantal")
	c.Assert(m.ContainerType(), gc.Equals, instance.LXC)
	c.Assert(m.Jobs(), gc.DeepEquals, oneJob)
	s.assertMachineContainers(c, m0, []string{"0/lxc/0", "0/lxc/1"})
}

func (s *StateSuite) TestReadMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Id(), gc.Equals, expectedId)
}

func (s *StateSuite) TestMachineNotFound(c *gc.C) {
	_, err := s.State.Machine("0")
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *StateSuite) TestMachineIdLessThan(c *gc.C) {
	c.Assert(state.MachineIdLessThan("0", "0"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("0", "1"), gc.Equals, true)
	c.Assert(state.MachineIdLessThan("1", "0"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("10", "2"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("0", "0/lxc/0"), gc.Equals, true)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "0"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("1", "0/lxc/0"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("0/lxc/0", "1"), gc.Equals, true)
	c.Assert(state.MachineIdLessThan("0/lxc/0/lxc/1", "0/lxc/0"), gc.Equals, false)
	c.Assert(state.MachineIdLessThan("0/kvm/0", "0/lxc/0"), gc.Equals, true)
}

func (s *StateSuite) TestAllMachines(c *gc.C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, gc.IsNil)
		err = m.SetProvisioned(instance.Id(fmt.Sprintf("foo-%d", i)), "fake_nonce", nil)
		c.Assert(err, gc.IsNil)
		err = m.SetAgentVersion(version.MustParseBinary("7.8.9-foo-bar"))
		c.Assert(err, gc.IsNil)
		err = m.Destroy()
		c.Assert(err, gc.IsNil)
	}
	s.AssertMachineCount(c, numInserts)
	ms, _ := s.State.AllMachines()
	for i, m := range ms {
		c.Assert(m.Id(), gc.Equals, strconv.Itoa(i))
		instId, err := m.InstanceId()
		c.Assert(err, gc.IsNil)
		c.Assert(string(instId), gc.Equals, fmt.Sprintf("foo-%d", i))
		tools, err := m.AgentTools()
		c.Check(err, gc.IsNil)
		c.Check(tools.Version, gc.DeepEquals, version.MustParseBinary("7.8.9-foo-bar"))
		c.Assert(m.Life(), gc.Equals, state.Dying)
	}
}

func (s *StateSuite) TestAddService(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("haha/borken", charm)
	c.Assert(err, gc.ErrorMatches, `cannot add service "haha/borken": invalid name`)
	_, err = s.State.Service("haha/borken")
	c.Assert(err, gc.ErrorMatches, `"haha/borken" is not a valid service name`)

	// set that a nil charm is handled correctly
	_, err = s.State.AddService("umadbro", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add service "umadbro": charm is nil`)

	wordpress, err := s.State.AddService("wordpress", charm)
	c.Assert(err, gc.IsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	mysql, err := s.State.AddService("mysql", charm)
	c.Assert(err, gc.IsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(wordpress.Name(), gc.Equals, "wordpress")
	ch, _, err := wordpress.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
	mysql, err = s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	c.Assert(mysql.Name(), gc.Equals, "mysql")
	ch, _, err = mysql.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
}

func (s *StateSuite) TestServiceNotFound(c *gc.C) {
	_, err := s.State.Service("bummer")
	c.Assert(err, gc.ErrorMatches, `service "bummer" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *StateSuite) TestAllServices(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.State.AllServices()
	c.Assert(err, gc.IsNil)
	c.Assert(len(services), gc.Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.State.AddService("wordpress", charm)
	c.Assert(err, gc.IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, gc.IsNil)
	c.Assert(len(services), gc.Equals, 1)

	_, err = s.State.AddService("mysql", charm)
	c.Assert(err, gc.IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, gc.IsNil)
	c.Assert(len(services), gc.Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), gc.Equals, "wordpress")
	c.Assert(services[1].Name(), gc.Equals, "mysql")
}

var inferEndpointsTests = []struct {
	summary string
	inputs  [][]string
	eps     []state.Endpoint
	err     string
}{
	{
		summary: "insane args",
		inputs:  [][]string{nil},
		err:     `cannot relate 0 endpoints`,
	}, {
		summary: "insane args",
		inputs:  [][]string{{"blah", "blur", "bleurgh"}},
		err:     `cannot relate 3 endpoints`,
	}, {
		summary: "invalid args",
		inputs: [][]string{
			{"ping:"},
			{":pong"},
			{":"},
		},
		err: `invalid endpoint ".*"`,
	}, {
		summary: "unknown service",
		inputs:  [][]string{{"wooble"}},
		err:     `service "wooble" not found`,
	}, {
		summary: "invalid relations",
		inputs: [][]string{
			{"lg", "lg"},
			{"ms", "ms"},
			{"wp", "wp"},
			{"rk1", "rk1"},
			{"rk1", "rk2"},
		},
		err: `no relations found`,
	}, {
		summary: "valid peer relation",
		inputs: [][]string{
			{"rk1"},
			{"rk1:ring"},
		},
		eps: []state.Endpoint{{
			ServiceName: "rk1",
			Relation: charm.Relation{
				Name:      "ring",
				Interface: "riak",
				Limit:     1,
				Role:      charm.RolePeer,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "ambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms", "wp"},
			{"ms", "wp:db"},
		},
		err: `ambiguous relation: ".*" could refer to "wp:db ms:dev"; "wp:db ms:prod"`,
	}, {
		summary: "unambiguous provider/requirer relation",
		inputs: [][]string{
			{"ms:dev", "wp"},
			{"ms:dev", "wp:db"},
		},
		eps: []state.Endpoint{{
			ServiceName: "ms",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "dev",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
				Limit:     2,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		}},
	}, {
		summary: "explicit logging relation is preferred over implicit juju-info",
		inputs:  [][]string{{"lg", "wp"}},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-directory",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "logging",
				Name:      "logging-dir",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeContainer,
			},
		}},
	}, {
		summary: "implict relations can be chosen explicitly",
		inputs: [][]string{
			{"lg:info", "wp"},
			{"lg", "wp:juju-info"},
			{"lg:info", "wp:juju-info"},
		},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "wp",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	}, {
		summary: "implicit relations will be chosen if there are no other options",
		inputs:  [][]string{{"lg", "ms"}},
		eps: []state.Endpoint{{
			ServiceName: "lg",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "info",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeContainer,
				Limit:     1,
			},
		}, {
			ServiceName: "ms",
			Relation: charm.Relation{
				Interface: "juju-info",
				Name:      "juju-info",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	},
}

func (s *StateSuite) TestInferEndpoints(c *gc.C) {
	_, err := s.State.AddService("ms", s.AddTestingCharm(c, "mysql-alternative"))
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("wp", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("lg", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	riak := s.AddTestingCharm(c, "riak")
	_, err = s.State.AddService("rk1", riak)
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("rk2", riak)
	c.Assert(err, gc.IsNil)

	for i, t := range inferEndpointsTests {
		c.Logf("test %d", i)
		for j, input := range t.inputs {
			c.Logf("  input %d", j)
			eps, err := s.State.InferEndpoints(input)
			if t.err == "" {
				c.Assert(err, gc.IsNil)
				c.Assert(eps, gc.DeepEquals, t.eps)
			} else {
				c.Assert(err, gc.ErrorMatches, t.err)
			}
		}
	}
}

func (s *StateSuite) TestEnvironConfig(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	change, err := cfg.Apply(map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(change)
	c.Assert(err, gc.IsNil)
	cfg, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, change.AllAttrs())
}

func (s *StateSuite) TestEnvironConstraints(c *gc.C) {
	// Environ constraints start out empty (for now).
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	// Environ constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(1024)}
	err = s.State.SetEnvironConstraints(cons2)
	c.Assert(err, gc.IsNil)
	cons3, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Environ constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(250)}
	err = s.State.SetEnvironConstraints(cons4)
	c.Assert(err, gc.IsNil)
	cons5, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons5, gc.DeepEquals, cons4)
}

func (s *StateSuite) TestWatchServicesBulkEvents(c *gc.C) {
	// Alive service...
	dummyCharm := s.AddTestingCharm(c, "dummy")
	alive, err := s.State.AddService("service0", dummyCharm)
	c.Assert(err, gc.IsNil)

	// Dying service...
	dying, err := s.State.AddService("service1", dummyCharm)
	c.Assert(err, gc.IsNil)
	keepDying, err := dying.AddUnit()
	c.Assert(err, gc.IsNil)
	err = dying.Destroy()
	c.Assert(err, gc.IsNil)

	// Dead service (actually, gone, Dead == removed in this case).
	gone, err := s.State.AddService("service2", dummyCharm)
	c.Assert(err, gc.IsNil)
	err = gone.Destroy()
	c.Assert(err, gc.IsNil)

	// All except gone are reported in initial event.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported.
	err = alive.Destroy()
	c.Assert(err, gc.IsNil)
	err = keepDying.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesLifecycle(c *gc.C) {
	// Initial event is empty when no services.
	w := s.State.WatchServices()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a service: reported.
	service, err := s.State.AddService("service", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	wc.AssertChange("service")
	wc.AssertNoChange()

	// Change the service: not reported.
	keepDying, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = service.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("service")
	wc.AssertNoChange()

	// Make it Dead(/removed): reported.
	err = keepDying.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("service")
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchServicesDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.lifecycleWatcher,
	// which is also used by:
	//     Service.WatchUnits
	//     Service.WatchRelations
	//     State.WatchEnviron
	//     Machine.WatchContainers
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		w := st.WatchServices()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchMachinesBulkEvents(c *gc.C) {
	// Alive machine...
	alive, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// Dying machine...
	dying, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = dying.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, gc.IsNil)
	err = dying.Destroy()
	c.Assert(err, gc.IsNil)

	// Dead machine...
	dead, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = dead.EnsureDead()
	c.Assert(err, gc.IsNil)

	// Gone machine.
	gone, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = gone.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = gone.Remove()
	c.Assert(err, gc.IsNil)

	// All except gone machine are reported in initial event.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Id(), dying.Id(), dead.Id())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, gc.IsNil)
	err = dying.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = dying.Remove()
	c.Assert(err, gc.IsNil)
	err = dead.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(alive.Id(), dying.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesLifecycle(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Change the machine: not reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make it Dying: reported.
	err = machine.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Make it Dead: reported.
	err = machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Remove it: not reported.
	err = machine.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIncludesOldMachines(c *gc.C) {
	// Older versions of juju do not write the "containertype" field.
	// This has caused machines to not be detected in the initial event.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machines.Update(
		D{{"_id", machine.Id()}},
		D{{"$unset", D{{"containertype", 1}}}},
	)
	c.Assert(err, gc.IsNil)

	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(machine.Id())
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachinesIgnoresContainers(c *gc.C) {
	// Initial event is empty when no machines.
	w := s.State.WatchEnvironMachines()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a machine: reported.
	params := state.AddMachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0")
	wc.AssertNoChange()

	// Add a container: not reported.
	params.ParentId = machine.Id()
	params.ContainerType = instance.LXC
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the container Dying: not reported.
	err = m.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the container Dead: not reported.
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchContainerLifecycle(c *gc.C) {
	// Add a host machine.
	params := state.AddMachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	machine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)

	otherMachine, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)

	// Initial event is empty when no containers.
	w := machine.WatchContainers(instance.LXC)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a container of the required type: reported.
	params.ParentId = machine.Id()
	params.ContainerType = instance.LXC
	m, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()

	// Add a container of a different type: not reported.
	params.ContainerType = instance.KVM
	m1, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add a nested container of the right type: not reported.
	params.ParentId = m.Id()
	params.ContainerType = instance.LXC
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Add a container of a different machine: not reported.
	params.ParentId = otherMachine.Id()
	params.ContainerType = instance.LXC
	m2, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
	statetesting.AssertStop(c, w)

	w = machine.WatchContainers(instance.LXC)
	defer statetesting.AssertStop(c, w)
	wc = statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()

	// Make the container Dying: reported.
	err = m.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()

	// Make the other containers Dying: not reported.
	err = m1.Destroy()
	c.Assert(err, gc.IsNil)
	err = m2.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the container Dead: reported.
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("0/lxc/0")
	wc.AssertNoChange()

	// Make the other containers Dead: not reported.
	err = m1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = m2.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Remove the container: not reported.
	err = m.Remove()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchMachineHardwareCharacteristics(c *gc.C) {
	// Add a machine: reported.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	w := machine.WatchHardwareCharacteristics()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Provision a machine: reported.
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Alter the machine: not reported.
	vers := version.MustParseBinary("1.2.3-gutsy-ppc")
	err = machine.SetAgentVersion(vers)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

var sortPortsTests = []struct {
	have, want []instance.Port
}{
	{nil, []instance.Port{}},
	{[]instance.Port{{"b", 1}, {"a", 99}, {"a", 1}}, []instance.Port{{"a", 1}, {"a", 99}, {"b", 1}}},
}

func (*StateSuite) TestSortPorts(c *gc.C) {
	for _, t := range sortPortsTests {
		p := make([]instance.Port, len(t.have))
		copy(p, t.have)
		state.SortPorts(p)
		c.Check(p, gc.DeepEquals, t.want)
		state.SortPorts(p)
		c.Check(p, gc.DeepEquals, t.want)
	}
}

type attrs map[string]interface{}

func (s *StateSuite) TestWatchEnvironConfig(c *gc.C) {
	w := s.State.WatchEnvironConfig()
	defer statetesting.AssertStop(c, w)

	// TODO(fwereade) just use a NotifyWatcher and NotifyWatcherC to test it.
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case got := <-w.Changes():
			c.Fatalf("got unexpected change: %#v", got)
		case <-time.After(testing.ShortWait):
		}
	}
	assertChange := func(change attrs) {
		cfg, err := s.State.EnvironConfig()
		c.Assert(err, gc.IsNil)
		if change != nil {
			cfg, err = cfg.Apply(change)
			c.Assert(err, gc.IsNil)
			err = s.State.SetEnvironConfig(cfg)
			c.Assert(err, gc.IsNil)
		}
		s.State.StartSync()
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, gc.Equals, true)
			c.Assert(got.AllAttrs(), gc.DeepEquals, cfg.AllAttrs())
		case <-time.After(testing.LongWait):
			c.Fatalf("did not get change: %#v", change)
		}
		assertNoChange()
	}
	assertChange(nil)
	assertChange(attrs{"default-series": "another-series"})
	assertChange(attrs{"fancy-new-key": "arbitrary-value"})
}

func (s *StateSuite) TestWatchEnvironConfigDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		w := st.WatchEnvironConfig()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchForEnvironConfigChanges(c *gc.C) {
	cur := version.Current.Number
	err := statetesting.SetAgentVersion(s.State, cur)
	c.Assert(err, gc.IsNil)
	w := s.State.WatchForEnvironConfigChanges()
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	// Initially we get one change notification
	wc.AssertOneChange()

	// Multiple changes will only result in a single change notification
	newVersion := cur
	newVersion.Minor += 1
	err = statetesting.SetAgentVersion(s.State, newVersion)
	c.Assert(err, gc.IsNil)

	newerVersion := newVersion
	newerVersion.Minor += 1
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Setting it to the same value does not trigger a change notification
	err = statetesting.SetAgentVersion(s.State, newerVersion)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()
}

func (s *StateSuite) TestWatchEnvironConfigCorruptConfig(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	// Corrupt the environment configuration.
	settings := s.Session.DB("juju").C("settings")
	err = settings.UpdateId("e", bson.D{{"$unset", bson.D{{"name", 1}}}})
	c.Assert(err, gc.IsNil)

	s.State.StartSync()

	// Start watching the configuration.
	watcher := s.State.WatchEnvironConfig()
	defer watcher.Stop()
	done := make(chan *config.Config)
	go func() {
		select {
		case cfg, ok := <-watcher.Changes():
			if !ok {
				c.Errorf("watcher channel closed")
			} else {
				done <- cfg
			}
		case <-time.After(5 * time.Second):
			c.Fatalf("no environment configuration observed")
		}
	}()

	s.State.StartSync()

	// The invalid configuration must not have been generated.
	select {
	case <-done:
		c.Fatalf("configuration returned too soon")
	case <-time.After(testing.ShortWait):
	}

	// Fix the configuration.
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, gc.IsNil)
	fixed := cfg.AllAttrs()

	s.State.StartSync()
	select {
	case got := <-done:
		c.Assert(got.AllAttrs(), gc.DeepEquals, fixed)
	case <-time.After(5 * time.Second):
		c.Fatalf("no environment configuration observed")
	}
}

func (s *StateSuite) TestAddAndGetEquivalence(c *gc.C) {
	// The equivalence tested here isn't necessarily correct, and
	// comparing private details is discouraged in the project.
	// The implementation might choose to cache information, or
	// to have different logic when adding or removing, and the
	// comparison might fail despite it being correct.
	// That said, we've had bugs with txn-revno being incorrect
	// before, so this testing at least ensures we're conscious
	// about such changes.

	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	m2, err := s.State.Machine(m1.Id())
	c.Assert(m1, gc.DeepEquals, m2)

	charm1 := s.AddTestingCharm(c, "wordpress")
	charm2, err := s.State.Charm(charm1.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(charm1, gc.DeepEquals, charm2)

	wordpress1, err := s.State.AddService("wordpress", charm1)
	c.Assert(err, gc.IsNil)
	wordpress2, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(wordpress1, gc.DeepEquals, wordpress2)

	unit1, err := wordpress1.AddUnit()
	c.Assert(err, gc.IsNil)
	unit2, err := s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit1, gc.DeepEquals, unit2)

	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	relation1, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	relation2, err := s.State.EndpointsRelation(eps...)
	c.Assert(relation1, gc.DeepEquals, relation2)
	relation3, err := s.State.Relation(relation1.Id())
	c.Assert(relation1, gc.DeepEquals, relation3)
}

func tryOpenState(info *state.Info) error {
	st, err := state.Open(info, state.TestingDialOpts())
	if err == nil {
		st.Close()
	}
	return err
}

func (s *StateSuite) TestOpenWithoutSetMongoPassword(c *gc.C) {
	info := state.TestingStateInfo()
	info.Tag, info.Password = "arble", "bar"
	err := tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	info.Tag, info.Password = "arble", ""
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	info.Tag, info.Password = "", ""
	err = tryOpenState(info)
	c.Assert(err, gc.IsNil)
}

func (s *StateSuite) TestOpenBadAddress(c *gc.C) {
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}
	st, err := state.Open(info, state.DialOpts{
		Timeout: 1 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "no reachable servers")
}

func (s *StateSuite) TestOpenDelaysRetryBadAddress(c *gc.C) {
	// Default mgo retry delay
	retryDelay := 500 * time.Millisecond
	info := state.TestingStateInfo()
	info.Addrs = []string{"0.1.2.3:1234"}

	t0 := time.Now()
	st, err := state.Open(info, state.DialOpts{
		Timeout: 1 * time.Millisecond,
	})
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "no reachable servers")
	// tryOpenState should have delayed for at least retryDelay
	// internally mgo will try three times in a row before returning
	// to the caller.
	if t1 := time.Since(t0); t1 < 3*retryDelay {
		c.Errorf("mgo.Dial only paused for %v, expected at least %v", t1, 3*retryDelay)
	}
}

func testSetPassword(c *gc.C, getEntity func() (state.Authenticator, error)) {
	e, err := getEntity()
	c.Assert(err, gc.IsNil)

	c.Assert(e.PasswordValid(goodPassword), gc.Equals, false)
	err = e.SetPassword(goodPassword)
	c.Assert(err, gc.IsNil)
	c.Assert(e.PasswordValid(goodPassword), gc.Equals, true)

	// Check a newly-fetched entity has the same password.
	e2, err := getEntity()
	c.Assert(err, gc.IsNil)
	c.Assert(e2.PasswordValid(goodPassword), gc.Equals, true)

	err = e.SetPassword(alternatePassword)
	c.Assert(err, gc.IsNil)
	c.Assert(e.PasswordValid(goodPassword), gc.Equals, false)
	c.Assert(e.PasswordValid(alternatePassword), gc.Equals, true)

	// Check that refreshing fetches the new password
	err = e2.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(e2.PasswordValid(alternatePassword), gc.Equals, true)

	if le, ok := e.(lifer); ok {
		testWhenDying(c, le, noErr, deadErr, func() error {
			return e.SetPassword("arble-farble-dying-yarble")
		})
	}
}

func testSetAgentCompatPassword(c *gc.C, entity state.Authenticator) {
	// In Juju versions 1.16 and older we used UserPasswordHash(password,CompatSalt)
	// for Machine and Unit agents. This was determined to be overkill
	// (since we know that Unit agents will actually use
	// utils.RandomPassword() and get 18 bytes of entropy, and thus won't
	// be brute-forced.)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsFalse)
	agentHash := utils.AgentPasswordHash(goodPassword)
	err := state.SetPasswordHash(entity, agentHash)
	c.Assert(err, gc.IsNil)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)
	c.Assert(entity.PasswordValid(alternatePassword), jc.IsFalse)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)

	backwardsCompatibleHash := utils.UserPasswordHash(goodPassword, utils.CompatSalt)
	c.Assert(backwardsCompatibleHash, gc.Not(gc.Equals), agentHash)
	err = state.SetPasswordHash(entity, backwardsCompatibleHash)
	c.Assert(err, gc.IsNil)
	c.Assert(entity.PasswordValid(alternatePassword), jc.IsFalse)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, backwardsCompatibleHash)
	// After succeeding to log in with the old compatible hash, the db
	// should be updated with the new hash
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)
	c.Assert(entity.PasswordValid(goodPassword), jc.IsTrue)

	// Agents are unable to set short passwords
	err = entity.SetPassword("short")
	c.Check(err, gc.ErrorMatches, "password is only 5 bytes long, and is not a valid Agent password")
	// Grandfather clause. Agents that have short passwords are allowed if
	// it was done in the compatHash form
	agentHash = utils.AgentPasswordHash("short")
	backwardsCompatibleHash = utils.UserPasswordHash("short", utils.CompatSalt)
	err = state.SetPasswordHash(entity, backwardsCompatibleHash)
	c.Assert(err, gc.IsNil)
	c.Assert(entity.PasswordValid("short"), jc.IsTrue)
	// We'll still update the hash, but now it points to the hash of the
	// shorter password. Agents still can't set the password to it
	c.Assert(state.GetPasswordHash(entity), gc.Equals, agentHash)
	// Still valid with the shorter password
	c.Assert(entity.PasswordValid("short"), jc.IsTrue)
}

type entity interface {
	state.Entity
	state.Lifer
	state.Authenticator
	state.MongoPassworder
}

func testSetMongoPassword(c *gc.C, getEntity func(st *state.State) (entity, error)) {
	info := state.TestingStateInfo()
	st, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, gc.IsNil)

	// Set the password for the entity
	ent, err := getEntity(st)
	c.Assert(err, gc.IsNil)
	err = ent.SetMongoPassword("foo")
	c.Assert(err, gc.IsNil)

	// Check that we cannot log in with the wrong password.
	info.Tag = ent.Tag()
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	st1, err := state.Open(info, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	defer st1.Close()

	// Change the password with an entity derived from the newly
	// opened and authenticated state.
	ent, err = getEntity(st)
	c.Assert(err, gc.IsNil)
	err = ent.SetMongoPassword("bar")
	c.Assert(err, gc.IsNil)

	// Check that we cannot log in with the old password.
	info.Password = "foo"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	// Check that we can log in with the correct password.
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, gc.IsNil)

	// Check that the administrator can still log in.
	info.Tag, info.Password = "", "admin-secret"
	err = tryOpenState(info)
	c.Assert(err, gc.IsNil)

	// Remove the admin password so that the test harness can reset the state.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
}

func (s *StateSuite) TestSetAdminMongoPassword(c *gc.C) {
	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err := s.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)

	err = s.State.SetAdminMongoPassword("foo")
	c.Assert(err, gc.IsNil)
	defer s.State.SetAdminMongoPassword("")
	info := state.TestingStateInfo()
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorizedError)

	info.Password = "foo"
	err = tryOpenState(info)
	c.Assert(err, gc.IsNil)

	err = s.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)

	// Check that removing the password is idempotent.
	err = s.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)

	info.Password = ""
	err = tryOpenState(info)
	c.Assert(err, gc.IsNil)
}

var findEntityTests = []struct {
	tag string
	err string
}{{
	tag: "",
	err: `"" is not a valid tag`,
}, {
	tag: "machine",
	err: `"machine" is not a valid tag`,
}, {
	tag: "-foo",
	err: `"-foo" is not a valid tag`,
}, {
	tag: "foo-",
	err: `"foo-" is not a valid tag`,
}, {
	tag: "---",
	err: `"---" is not a valid tag`,
}, {
	tag: "machine-bad",
	err: `"machine-bad" is not a valid machine tag`,
}, {
	tag: "unit-123",
	err: `"unit-123" is not a valid unit tag`,
}, {
	tag: "relation-blah",
	err: `"relation-blah" is not a valid relation tag`,
}, {
	tag: "relation-svc1.rel1#svc2.rel2",
	err: `relation "svc1:rel1 svc2:rel2" not found`,
}, {
	tag: "unit-foo",
	err: `"unit-foo" is not a valid unit tag`,
}, {
	tag: "service-",
	err: `"service-" is not a valid service tag`,
}, {
	tag: "service-foo/bar",
	err: `"service-foo/bar" is not a valid service tag`,
}, {
	tag: "environment-foo",
	err: `environment "foo" not found`,
}, {
	tag: "machine-1234",
	err: `machine 1234 not found`,
}, {
	tag: "unit-foo-654",
	err: `unit "foo/654" not found`,
}, {
	tag: "unit-foo-bar-654",
	err: `unit "foo-bar/654" not found`,
}, {
	tag: "machine-0",
}, {
	tag: "service-ser-vice2",
}, {
	tag: "relation-wordpress.db#ser-vice2.server",
}, {
	tag: "unit-ser-vice2-0",
}, {
	tag: "user-arble",
}, {
	tag: "environment-testenv",
}}

var entityTypes = map[string]interface{}{
	names.UserTagKind:     (*state.User)(nil),
	names.EnvironTagKind:  (*state.Environment)(nil),
	names.ServiceTagKind:  (*state.Service)(nil),
	names.UnitTagKind:     (*state.Unit)(nil),
	names.MachineTagKind:  (*state.Machine)(nil),
	names.RelationTagKind: (*state.Relation)(nil),
}

func (s *StateSuite) TestFindEntity(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	svc, err := s.State.AddService("ser-vice2", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	_, err = svc.AddUnit()
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddUser("arble", "pass")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "ser-vice2"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db ser-vice2:server")

	for i, test := range findEntityTests {
		c.Logf("test %d: %q", i, test.tag)
		e, err := s.State.FindEntity(test.tag)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, gc.IsNil)
			kind, err := names.TagKind(test.tag)
			c.Assert(err, gc.IsNil)
			c.Assert(e, gc.FitsTypeOf, entityTypes[kind])
			c.Assert(e.Tag(), gc.Equals, test.tag)
		}
	}
}

func (s *StateSuite) TestParseTag(c *gc.C) {
	bad := []string{
		"",
		"machine",
		"-foo",
		"foo-",
		"---",
		"foo-bar",
		"environment-foo",
		"unit-foo",
	}
	for _, name := range bad {
		c.Logf(name)
		coll, id, err := state.ParseTag(s.State, name)
		c.Check(coll, gc.Equals, "")
		c.Check(id, gc.Equals, "")
		c.Assert(err, gc.ErrorMatches, `".*" is not a valid( [a-z]+)? tag`)
	}

	// Parse a machine entity name.
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	coll, id, err := state.ParseTag(s.State, m.Tag())
	c.Assert(coll, gc.Equals, "machines")
	c.Assert(id, gc.Equals, m.Id())
	c.Assert(err, gc.IsNil)

	// Parse a service entity name.
	svc, err := s.State.AddService("ser-vice2", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	coll, id, err = state.ParseTag(s.State, svc.Tag())
	c.Assert(coll, gc.Equals, "services")
	c.Assert(id, gc.Equals, svc.Name())
	c.Assert(err, gc.IsNil)

	// Parse a unit entity name.
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	coll, id, err = state.ParseTag(s.State, u.Tag())
	c.Assert(coll, gc.Equals, "units")
	c.Assert(id, gc.Equals, u.Name())
	c.Assert(err, gc.IsNil)

	// Parse a user entity name.
	user, err := s.State.AddUser("arble", "pass")
	c.Assert(err, gc.IsNil)
	coll, id, err = state.ParseTag(s.State, user.Tag())
	c.Assert(coll, gc.Equals, "users")
	c.Assert(id, gc.Equals, user.Name())
	c.Assert(err, gc.IsNil)
}

func (s *StateSuite) TestCleanup(c *gc.C) {
	needed, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, false)

	_, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, false)

	err = relM.Destroy()
	c.Assert(err, gc.IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, true)

	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, false)
}

func (s *StateSuite) TestWatchCleanups(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Set up two relations for later use, check no events.
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddService("varnish", s.AddTestingCharm(c, "varnish"))
	c.Assert(err, gc.IsNil)
	eps, err = s.State.InferEndpoints([]string{"wordpress", "varnish"})
	c.Assert(err, gc.IsNil)
	relV, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy one relation, check one change.
	err = relM.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Handle that cleanup doc and create another, check one change.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	err = relV.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Clean up final doc, check change.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchCleanupsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		w := st.WatchCleanups()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestWatchCleanupsBulk(c *gc.C) {
	// Check initial event.
	w := s.State.WatchCleanups()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Create two peer relations by creating their services.
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, gc.IsNil)
	_, err = riak.Endpoint("ring")
	c.Assert(err, gc.IsNil)
	allHooks, err := s.State.AddService("all-hooks", s.AddTestingCharm(c, "all-hooks"))
	c.Assert(err, gc.IsNil)
	_, err = allHooks.Endpoint("self")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy them both, check one change.
	err = riak.Destroy()
	c.Assert(err, gc.IsNil)
	err = allHooks.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Clean them both up, check one change.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()
}

func (s *StateSuite) TestWatchMinUnits(c *gc.C) {
	// Check initial event.
	w := s.State.WatchMinUnits()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Set up services for later use.
	wordpress, err := s.State.AddService(
		"wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	wordpressName := wordpress.Name()

	// Add service units for later use.
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	wordpress1, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	mysql0, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	// No events should occur.
	wc.AssertNoChange()

	// Add minimum units to a service; a single change should occur.
	err = wordpress.SetMinUnits(2)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Decrease minimum units for a service; expect no changes.
	err = wordpress.SetMinUnits(1)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Increase minimum units for two services; a single change should occur.
	err = mysql.SetMinUnits(1)
	c.Assert(err, gc.IsNil)
	err = wordpress.SetMinUnits(3)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(mysql.Name(), wordpressName)
	wc.AssertNoChange()

	// Remove minimum units for a service; expect no changes.
	err = mysql.SetMinUnits(0)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy a unit of a service with required minimum units.
	// Also avoid the unit removal. A single change should occur.
	preventUnitDestroyRemove(c, wordpress0)
	err = wordpress0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Two actions: destroy a unit and increase minimum units for a service.
	// A single change should occur, and the service name should appear only
	// one time in the change.
	err = wordpress.SetMinUnits(5)
	c.Assert(err, gc.IsNil)
	err = wordpress1.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(wordpressName)
	wc.AssertNoChange()

	// Destroy a unit of a service not requiring minimum units; expect no changes.
	err = mysql0.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy a service with required minimum units; expect no changes.
	err = wordpress.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Destroy a service not requiring minimum units; expect no changes.
	err = mysql.Destroy()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Stop watcher, check closed.
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *StateSuite) TestWatchMinUnitsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		w := st.WatchMinUnits()
		<-w.Changes()
		return w
	})
}

func (s *StateSuite) TestNestingLevel(c *gc.C) {
	c.Assert(state.NestingLevel("0"), gc.Equals, 0)
	c.Assert(state.NestingLevel("0/lxc/1"), gc.Equals, 1)
	c.Assert(state.NestingLevel("0/lxc/1/kvm/0"), gc.Equals, 2)
}

func (s *StateSuite) TestTopParentId(c *gc.C) {
	c.Assert(state.TopParentId("0"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1"), gc.Equals, "0")
	c.Assert(state.TopParentId("0/lxc/1/kvm/2"), gc.Equals, "0")
}

func (s *StateSuite) TestParentId(c *gc.C) {
	c.Assert(state.ParentId("0"), gc.Equals, "")
	c.Assert(state.ParentId("0/lxc/1"), gc.Equals, "0")
	c.Assert(state.ParentId("0/lxc/1/kvm/0"), gc.Equals, "0/lxc/1")
}

func (s *StateSuite) TestContainerTypeFromId(c *gc.C) {
	c.Assert(state.ContainerTypeFromId("0"), gc.Equals, instance.ContainerType(""))
	c.Assert(state.ContainerTypeFromId("0/lxc/1"), gc.Equals, instance.LXC)
	c.Assert(state.ContainerTypeFromId("0/lxc/1/kvm/0"), gc.Equals, instance.KVM)
}

func (s *StateSuite) TestSetEnvironAgentVersionErrors(c *gc.C) {
	// Get the agent-version set in the environment.
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	stringVersion := agentVersion.String()

	// Add 4 machines: one with a different version, one with an
	// empty version, one with the current version, and one with
	// the new version.
	machine0, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine0.SetAgentVersion(version.MustParseBinary("9.9.9-series-arch"))
	c.Assert(err, gc.IsNil)
	machine1, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	machine2, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine2.SetAgentVersion(version.MustParseBinary(stringVersion + "-series-arch"))
	c.Assert(err, gc.IsNil)
	machine3, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine3.SetAgentVersion(version.MustParseBinary("4.5.6-series-arch"))
	c.Assert(err, gc.IsNil)

	// Verify machine0 and machine1 are reported as error.
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr := fmt.Sprintf("some agents have not upgraded to the current environment version %s: machine-0, machine-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Add a service and 4 units: one with a different version, one
	// with an empty version, one with the current version, and one
	// with the new version.
	service, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit0, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit0.SetAgentVersion(version.MustParseBinary("6.6.6-series-arch"))
	c.Assert(err, gc.IsNil)
	_, err = service.AddUnit()
	c.Assert(err, gc.IsNil)
	unit2, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit2.SetAgentVersion(version.MustParseBinary(stringVersion + "-series-arch"))
	c.Assert(err, gc.IsNil)
	unit3, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit3.SetAgentVersion(version.MustParseBinary("4.5.6-series-arch"))
	c.Assert(err, gc.IsNil)

	// Verify unit0 and unit1 are reported as error, along with the
	// machines from before.
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current environment version %s: machine-0, machine-1, unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)

	// Now remove the machines.
	for _, machine := range []*state.Machine{machine0, machine1, machine2} {
		err = machine.EnsureDead()
		c.Assert(err, gc.IsNil)
		err = machine.Remove()
		c.Assert(err, gc.IsNil)
	}

	// Verify only the units are reported as error.
	err = s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	expectErr = fmt.Sprintf("some agents have not upgraded to the current environment version %s: unit-wordpress-0, unit-wordpress-1", stringVersion)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(err, jc.Satisfies, state.IsVersionInconsistentError)
}

func (s *StateSuite) prepareAgentVersionTests(c *gc.C) (*config.Config, string) {
	// Get the agent-version set in the environment.
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	currentVersion := agentVersion.String()

	// Add a machine and a unit with the current version.
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	service, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)

	err = machine.SetAgentVersion(version.MustParseBinary(currentVersion + "-series-arch"))
	c.Assert(err, gc.IsNil)
	err = unit.SetAgentVersion(version.MustParseBinary(currentVersion + "-series-arch"))
	c.Assert(err, gc.IsNil)

	return envConfig, currentVersion
}

func (s *StateSuite) changeEnviron(c *gc.C, envConfig *config.Config, name string, value interface{}) {
	attrs := envConfig.AllAttrs()
	attrs[name] = value
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	c.Assert(s.State.SetEnvironConfig(newConfig), gc.IsNil)
}

func (s *StateSuite) assertAgentVersion(c *gc.C, envConfig *config.Config, vers string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion.String(), gc.Equals, vers)
}

func (s *StateSuite) TestSetEnvironAgentVersionRetriesOnConfigChange(c *gc.C) {
	envConfig, _ := s.prepareAgentVersionTests(c)

	// Set up a transaction hook to change something
	// other than the version, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, envConfig, "default-series", "foo")
	}).Check()

	// Change the agent-version and ensure it has changed.
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, gc.IsNil)
	s.assertAgentVersion(c, envConfig, "4.5.6")
}

func (s *StateSuite) TestSetEnvironAgentVersionSucceedsWithSameVersion(c *gc.C) {
	envConfig, _ := s.prepareAgentVersionTests(c)

	// Set up a transaction hook to change the version
	// to the new one, and make sure it retries
	// and passes.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.changeEnviron(c, envConfig, "agent-version", "4.5.6")
	}).Check()

	// Change the agent-version and verify.
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, gc.IsNil)
	s.assertAgentVersion(c, envConfig, "4.5.6")
}

func (s *StateSuite) TestSetEnvironAgentVersionExcessiveContention(c *gc.C) {
	envConfig, currentVersion := s.prepareAgentVersionTests(c)

	// Set a hook to change the config 5 times
	// to test we return ErrExcessiveContention.
	changeFuncs := []func(){
		func() { s.changeEnviron(c, envConfig, "default-series", "1") },
		func() { s.changeEnviron(c, envConfig, "default-series", "2") },
		func() { s.changeEnviron(c, envConfig, "default-series", "3") },
		func() { s.changeEnviron(c, envConfig, "default-series", "4") },
		func() { s.changeEnviron(c, envConfig, "default-series", "5") },
	}
	defer state.SetBeforeHooks(c, s.State, changeFuncs...).Check()
	err := s.State.SetEnvironAgentVersion(version.MustParse("4.5.6"))
	c.Assert(err, gc.Equals, state.ErrExcessiveContention)
	// Make sure the version remained the same.
	s.assertAgentVersion(c, envConfig, currentVersion)
}

type waiter interface {
	Wait() error
}

// testWatcherDiesWhenStateCloses calls the given function to start a watcher,
// closes the state and checks that the watcher dies with the expected error.
// The watcher should already have consumed the first
// event, otherwise the watcher's initialisation logic may
// interact with the closed state, causing it to return an
// unexpected error (often "Closed explictly").
func testWatcherDiesWhenStateCloses(c *gc.C, startWatcher func(c *gc.C, st *state.State) waiter) {
	st, err := state.Open(state.TestingStateInfo(), state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	watcher := startWatcher(c, st)
	err = st.Close()
	c.Assert(err, gc.IsNil)
	done := make(chan error)
	go func() {
		done <- watcher.Wait()
	}()
	select {
	case err := <-done:
		c.Assert(err, gc.Equals, state.ErrStateClosed)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher %T did not exit when state closed", watcher)
	}
}
