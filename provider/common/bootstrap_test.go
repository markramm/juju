// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/cloudinit"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/provider/common"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/testing/testbase"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
)

type BootstrapSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) TestCannotWriteStateFile(c *gc.C) {
	brokenStorage := &mockStorage{putErr: fmt.Errorf("noes!")}
	env := &mockEnviron{storage: brokenStorage}
	err := common.Bootstrap(env, constraints.Value{}, nil)
	c.Assert(err, gc.ErrorMatches, "cannot create initial state file: noes!")
}

func (s *BootstrapSuite) TestCannotStartInstance(c *gc.C) {
	stor := newStorage(s, c)
	checkURL, err := stor.URL(common.StateFile)
	c.Assert(err, gc.IsNil)
	checkCons := constraints.MustParse("mem=8G")
	checkTools := tools.List{&tools.Tools{Version: version.Current}}

	startInstance := func(
		cons constraints.Value, possibleTools tools.List, mcfg *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		c.Assert(cons, gc.DeepEquals, checkCons)
		c.Assert(possibleTools, gc.DeepEquals, checkTools)
		c.Assert(mcfg, gc.DeepEquals, environs.NewBootstrapMachineConfig(checkURL))
		return nil, nil, fmt.Errorf("meh, not started")
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
	}

	err = common.Bootstrap(env, checkCons, checkTools)
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestCannotRecordStartedInstance(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ constraints.Value, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil
	}

	var stopped []instance.Instance
	stopInstances := func(instances []instance.Instance) error {
		stopped = append(stopped, instances...)
		return nil
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
	}

	err := common.Bootstrap(env, constraints.Value{}, nil)
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0].Id(), gc.Equals, instance.Id("i-blah"))
}

func (s *BootstrapSuite) TestCannotRecordThenCannotStop(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ constraints.Value, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil
	}

	var stopped []instance.Instance
	stopInstances := func(instances []instance.Instance) error {
		stopped = append(stopped, instances...)
		return fmt.Errorf("bork bork borken")
	}

	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("bootstrap-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("bootstrap-tester")

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
	}

	err := common.Bootstrap(env, constraints.Value{}, nil)
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0].Id(), gc.Equals, instance.Id("i-blah"))
	c.Assert(tw.Log, jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR, `cannot stop failed bootstrap instance "i-blah": bork bork borken`,
	}})
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("mem=2T")

	checkURL := ""
	startInstance := func(
		_ constraints.Value, _ tools.List, mcfg *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		checkURL = mcfg.StateInfoURL
		return &mockInstance{id: checkInstanceId}, &checkHardware, nil
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
	}
	err := common.Bootstrap(env, constraints.Value{}, nil)
	c.Assert(err, gc.IsNil)

	savedState, err := common.LoadStateFromURL(checkURL)
	c.Assert(err, gc.IsNil)
	c.Assert(savedState, gc.DeepEquals, &common.BootstrapState{
		StateInstances:  []instance.Id{instance.Id(checkInstanceId)},
		Characteristics: []instance.HardwareCharacteristics{checkHardware},
	})
}
