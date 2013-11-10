// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"

	"github.com/jameinel/juju/environs/config"
	envtesting "github.com/jameinel/juju/environs/testing"
	coretesting "github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/testing/testbase"
)

func TestMAAS(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	testMAASObject  *gomaasapi.TestMAASObject
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(&shortAttempt)
	s.LoggingSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

const exampleAgentName = "dfb69555-0bc4-4d1f-85f2-4ee390974984"

// makeEnviron creates a functional maasEnviron for a test.
func (suite *providerSuite) makeEnviron() *maasEnviron {
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":            "test env",
		"type":            "maas",
		"maas-oauth":      "a:b:c",
		"maas-server":     suite.testMAASObject.TestServer.URL,
		"maas-agent-name": exampleAgentName,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}
