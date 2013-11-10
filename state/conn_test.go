// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/testing/testbase"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// ConnSuite provides the infrastructure for all other
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	testing.MgoSuite
	testbase.LoggingSuite
	annotations *mgo.Collection
	charms      *mgo.Collection
	machines    *mgo.Collection
	relations   *mgo.Collection
	services    *mgo.Collection
	units       *mgo.Collection
	State       *state.State
}

func (cs *ConnSuite) SetUpSuite(c *gc.C) {
	cs.LoggingSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
}

func (cs *ConnSuite) TearDownSuite(c *gc.C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.LoggingSuite.TearDownSuite(c)
}

func (cs *ConnSuite) SetUpTest(c *gc.C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.State = state.TestingInitialize(c, nil)
	cs.annotations = cs.MgoSuite.Session.DB("juju").C("annotations")
	cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
	cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
	cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
	cs.services = cs.MgoSuite.Session.DB("juju").C("services")
	cs.units = cs.MgoSuite.Session.DB("juju").C("units")
}

func (cs *ConnSuite) TearDownTest(c *gc.C) {
	cs.State.Close()
	cs.MgoSuite.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (s *ConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnSuite) AddSeriesCharm(c *gc.C, name, series string) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "", "", series, -1)
}

// AddConfigCharm clones a testing charm, replaces its config with
// the given YAML string and adds it to the state, using the given
// revision.
func (s *ConnSuite) AddConfigCharm(c *gc.C, name, configYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "config.yaml", configYaml, "quantal", revision)
}

// AddMetaCharm clones a testing charm, replaces its metadata with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnSuite) AddMetaCharm(c *gc.C, name, metaYaml string, revsion int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metadata.yaml", metaYaml, "quantal", revsion)
}
