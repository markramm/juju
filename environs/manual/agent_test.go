// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/config"
	envtools "github.com/jameinel/juju/environs/tools"
	_ "github.com/jameinel/juju/provider/dummy"
	coretesting "github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/testing/testbase"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
)

type agentSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&agentSuite{})

func dummyConfig(c *gc.C, stateServer bool, vers version.Binary) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, gc.IsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":           "dummy",
		"state-server":   stateServer,
		"default-series": vers.Series,
		"agent-version":  vers.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *agentSuite) getArgs(c *gc.C, stateServer bool, vers version.Binary) provisionMachineAgentArgs {
	tools := &tools.Tools{Version: vers}
	tools.URL = "file:///var/lib/juju/storage/" + envtools.StorageName(vers)
	return provisionMachineAgentArgs{
		bootstrap:     stateServer,
		environConfig: dummyConfig(c, stateServer, vers),
		machineId:     "0",
		nonce:         "ya",
		stateFileURL:  "http://whatever/dotcom",
		tools:         tools,
		// stateInfo *state.Info
		// apiInfo *api.Info
		agentEnv: make(map[string]string),
	}
}

var allSeries = [...]string{"precise", "quantal", "raring", "saucy"}

func checkIff(checker gc.Checker, condition bool) gc.Checker {
	if condition {
		return checker
	}
	return gc.Not(checker)
}

func (s *agentSuite) TestAptSources(c *gc.C) {
	for _, series := range allSeries {
		vers := version.MustParseBinary("1.16.0-" + series + "-amd64")
		script, err := provisionMachineAgentScript(s.getArgs(c, true, vers))
		c.Assert(err, gc.IsNil)

		// Only Precise requires the cloud-tools pocket.
		//
		// The only source we add that requires an explicitly
		// specified key is cloud-tools.
		needsCloudTools := series == "precise"
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*apt-key add.*(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*apt-add-repository.*cloud-tools(.|\n)*",
		)

		// Only Quantal requires the PPA (for mongo).
		needsJujuPPA := series == "quantal"
		c.Assert(
			script,
			checkIff(gc.Matches, needsJujuPPA),
			"(.|\n)*apt-add-repository.*ppa:juju/stable(.|\n)*",
		)

		// Only install python-software-properties (apt-add-repository)
		// if we need to.
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools || needsJujuPPA),
			"(.|\n)*apt-get -y install.*python-software-properties(.|\n)*",
		)
	}
}
