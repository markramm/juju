// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/bootstrap"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/configstore"
	envtesting "github.com/jameinel/juju/environs/testing"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/juju"
	"github.com/jameinel/juju/juju/osenv"
	"github.com/jameinel/juju/juju/testing"
	"github.com/jameinel/juju/provider/dummy"
	"github.com/jameinel/juju/state"
	coretesting "github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/testing/testbase"
	"github.com/jameinel/juju/utils"
	"github.com/jameinel/juju/utils/set"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type NewConnSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewConnSuite{})

func (cs *NewConnSuite) SetUpTest(c *gc.C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.ToolsFixture.SetUpTest(c)
}

func (cs *NewConnSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewConnSuite) TestNewConnWithoutAdminSecret(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	attrs := env.Config().AllAttrs()
	delete(attrs, "admin-secret")
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)
	conn, err := juju.NewConn(env1)
	c.Check(conn, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot connect without admin-secret")
}

func bootstrapEnv(c *gc.C, envName string, store configstore.Storage) {
	if store == nil {
		store = configstore.NewMem()
	}
	env, err := environs.PrepareFromName(envName, store)
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (*NewConnSuite) TestConnMultipleCloseOk(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "", defaultConfigStore(c))
	// Error return from here is tested in TestNewConnFromNameNotSetGetsDefault.
	conn, err := juju.NewConnFromName("")
	c.Assert(err, gc.IsNil)
	conn.Close()
	conn.Close()
	conn.Close()
}

func (*NewConnSuite) TestNewConnFromNameNotSetGetsDefault(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "", defaultConfigStore(c))
	conn, err := juju.NewConnFromName("")
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	c.Assert(conn.Environ.Name(), gc.Equals, coretesting.SampleEnvName)
}

func (*NewConnSuite) TestNewConnFromNameNotDefault(c *gc.C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The default environment is "erewhemos", so make sure we get what we ask for.
	const envName = "erewhemos-2"
	bootstrapEnv(c, envName, defaultConfigStore(c))
	conn, err := juju.NewConnFromName(envName)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	c.Assert(conn.Environ.Name(), gc.Equals, envName)
}

func (cs *NewConnSuite) TestConnStateSecretsSideEffect(c *gc.C) {
	attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
		"admin-secret": "side-effect secret",
		"secret":       "pork",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	info, _, err := env.StateInfo()
	c.Assert(err, gc.IsNil)
	info.Password = utils.UserPasswordHash("side-effect secret", utils.CompatSalt)
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)

	// Verify we have no secret in the environ config
	cfg, err = st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], gc.IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	cfg, err = conn.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], gc.Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
}

func (cs *NewConnSuite) TestConnStateDoesNotUpdateExistingSecrets(c *gc.C) {
	attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
		"secret": "pork",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	// Make another env with a different secret.
	attrs = env.Config().AllAttrs()
	attrs["secret"] = "squirrel"
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)

	// Connect with the new env and check that the secret has not changed
	conn, err = juju.NewConn(env1)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	cfg, err = conn.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], gc.Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
}

func (cs *NewConnSuite) TestConnWithPassword(c *gc.C) {
	attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
		"admin-secret": "nutkin",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	// Check that Bootstrap has correctly used a hash
	// of the admin password.
	info, _, err := env.StateInfo()
	c.Assert(err, gc.IsNil)
	info.Password = utils.UserPasswordHash("nutkin", utils.CompatSalt)
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	st.Close()

	// Check that we can connect with the original environment.
	conn, err := juju.NewConn(env)
	c.Assert(err, gc.IsNil)
	conn.Close()

	// Check that the password has now been changed to the original
	// admin password.
	info.Password = "nutkin"
	st1, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	st1.Close()

	// Check that we can still connect with the original
	// environment.
	conn, err = juju.NewConn(env)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
}

type ConnSuite struct {
	testbase.LoggingSuite
	coretesting.MgoSuite
	envtesting.ToolsFixture
	conn *juju.Conn
	repo *charm.LocalRepository
}

var _ = gc.Suite(&ConnSuite{})

func (s *ConnSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	environ, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, environ.Storage())
	err = bootstrap.Bootstrap(environ, constraints.Value{})
	c.Assert(err, gc.IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, gc.IsNil)
	s.repo = &charm.LocalRepository{Path: c.MkDir()}
}

func (s *ConnSuite) TearDownTest(c *gc.C) {
	if s.conn == nil {
		return
	}
	err := s.conn.State.SetAdminMongoPassword("")
	c.Assert(err, gc.IsNil)
	err = s.conn.Environ.Destroy()
	c.Check(err, gc.IsNil)
	s.conn.Close()
	s.conn = nil
	dummy.Reset()
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *ConnSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *ConnSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *ConnSuite) TestNewConnFromState(c *gc.C) {
	conn, err := juju.NewConnFromState(s.conn.State)
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Environ.Name(), gc.Equals, dummy.SampleConfig()["name"])
}

func (s *ConnSuite) TestPutCharmBasic(c *gc.C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "quantal", "riak")
	curl.Revision = -1 // make sure we trigger the repo.Latest logic.
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.Meta().Summary, gc.Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(sch.Meta().Summary, gc.Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutBundledCharm(c *gc.C) {
	// Bundle the riak charm into a charm repo directory.
	dir := filepath.Join(s.repo.Path, "quantal")
	err := os.Mkdir(dir, 0777)
	c.Assert(err, gc.IsNil)
	w, err := os.Create(filepath.Join(dir, "riak.charm"))
	c.Assert(err, gc.IsNil)
	defer w.Close()
	charmDir := coretesting.Charms.Dir("riak")
	err = charmDir.BundleTo(w)
	c.Assert(err, gc.IsNil)

	// Invent a URL that points to the bundled charm, and
	// test putting that.
	curl := &charm.URL{
		Schema:   "local",
		Series:   "quantal",
		Name:     "riak",
		Revision: -1,
	}
	_, err = s.conn.PutCharm(curl, s.repo, true)
	c.Assert(err, gc.ErrorMatches, `cannot increment revision of charm "local:quantal/riak-7": not a directory`)

	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.Meta().Summary, gc.Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(sch.Meta().Summary, gc.Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutCharmUpload(c *gc.C) {
	repo := &charm.LocalRepository{c.MkDir()}
	curl := coretesting.Charms.ClonedURL(repo.Path, "quantal", "riak")

	// Put charm for the first time.
	sch, err := s.conn.PutCharm(curl, repo, false)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.Meta().Summary, gc.Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, gc.IsNil)
	sha256 := sch.BundleSha256()
	rev := sch.Revision()

	// Change the charm on disk.
	ch, err := repo.Get(curl)
	c.Assert(err, gc.IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, gc.IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = s.conn.PutCharm(curl, repo, false)
	c.Assert(err, gc.IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(sch.BundleSha256(), gc.Equals, sha256)
	c.Assert(sch.Revision(), gc.Equals, rev)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.
	sch, err = s.conn.PutCharm(curl, repo, true)
	c.Assert(err, gc.IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(sch.BundleSha256(), gc.Not(gc.Equals), sha256)
	c.Assert(sch.Revision(), gc.Equals, rev+1)
}

func (s *ConnSuite) TestAddUnits(c *gc.C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "quantal", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, gc.IsNil)
	svc, err := s.conn.State.AddService("testriak", sch)
	c.Assert(err, gc.IsNil)
	units, err := s.conn.AddUnits(svc, 2, "")
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 2)

	id0, err := units[0].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	id1, err := units[1].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(id0, gc.Not(gc.Equals), id1)

	units, err = s.conn.AddUnits(svc, 2, "0")
	c.Assert(err, gc.ErrorMatches, `cannot add multiple units of service "testriak" to a single machine`)

	units, err = s.conn.AddUnits(svc, 1, "0")
	c.Assert(err, gc.IsNil)
	id2, err := units[0].AssignedMachineId()
	c.Assert(id2, gc.Equals, id0)

	units, err = s.conn.AddUnits(svc, 1, fmt.Sprintf("%s:0", instance.LXC))
	c.Assert(err, gc.IsNil)
	id3, err := units[0].AssignedMachineId()
	c.Assert(id3, gc.Equals, id0+"/lxc/0")
}

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployService demands that a charm already exists in state,
// and that's is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.JujuConnSuite
	repo        charm.Repository
	charm       *state.Charm
	oldCacheDir string
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.repo = &charm.LocalRepository{Path: coretesting.Charms.Path}
	s.oldCacheDir, charm.CacheDir = charm.CacheDir, c.MkDir()
}

func (s *DeployLocalSuite) TearDownSuite(c *gc.C) {
	charm.CacheDir = s.oldCacheDir
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	curl := charm.MustParseURL("local:quantal/dummy")
	charm, err := s.Conn.PutCharm(curl, s.repo, false)
	c.Assert(err, gc.IsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployMinimal(c *gc.C) {
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName: "bob",
		Charm:       s.charm,
	})
	c.Assert(err, gc.IsNil)
	s.assertCharm(c, service, s.charm.URL())
	s.assertSettings(c, service, charm.Settings{})
	s.assertConstraints(c, service, constraints.Value{})
	s.assertMachines(c, service, constraints.Value{})
}

func (s *DeployLocalSuite) TestDeploySettings(c *gc.C) {
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName: "bob",
		Charm:       s.charm,
		ConfigSettings: charm.Settings{
			"title":       "banana cupcakes",
			"skill-level": 9901,
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSettings(c, service, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *gc.C) {
	_, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName: "bob",
		Charm:       s.charm,
		ConfigSettings: charm.Settings{
			"skill-level": 99.01,
		},
	})
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got 99.01`)
	_, err = s.State.Service("bob")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *DeployLocalSuite) TestDeployConstraints(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, gc.IsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName: "bob",
		Charm:       s.charm,
		Constraints: serviceCons,
	})
	c.Assert(err, gc.IsNil)
	s.assertConstraints(c, service, serviceCons)
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *gc.C) {
	err := s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, gc.IsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName: "bob",
		Charm:       s.charm,
		Constraints: serviceCons,
		NumUnits:    2,
	})
	c.Assert(err, gc.IsNil)
	s.assertConstraints(c, service, serviceCons)
	s.assertMachines(c, service, constraints.MustParse("mem=2G cpu-cores=2"), "0", "1")
}

func (s *DeployLocalSuite) TestDeployWithForceMachineRejectsTooManyUnits(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	_, err = s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName:   "bob",
		Charm:         s.charm,
		NumUnits:      2,
		ToMachineSpec: "0",
	})
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units with --to")
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	err = s.State.SetEnvironConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, gc.IsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName:   "bob",
		Charm:         s.charm,
		Constraints:   serviceCons,
		NumUnits:      1,
		ToMachineSpec: "0",
	})
	c.Assert(err, gc.IsNil)
	s.assertConstraints(c, service, serviceCons)
	s.assertMachines(c, service, constraints.Value{}, "0")
}

func (s *DeployLocalSuite) TestDeployForceMachineIdWithContainer(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Id(), gc.Equals, "0")
	cons := constraints.MustParse("mem=2G")
	err = s.State.SetEnvironConstraints(cons)
	c.Assert(err, gc.IsNil)
	serviceCons := constraints.MustParse("cpu-cores=2")
	service, err := s.Conn.DeployService(juju.DeployServiceParams{
		ServiceName:   "bob",
		Charm:         s.charm,
		Constraints:   serviceCons,
		NumUnits:      1,
		ToMachineSpec: fmt.Sprintf("%s:0", instance.LXC),
	})
	c.Assert(err, gc.IsNil)
	s.assertConstraints(c, service, serviceCons)
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 1)

	// The newly created container will use the constraints.
	id, err := units[0].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err = s.State.Machine(id)
	c.Assert(err, gc.IsNil)
	expectedCons, err := machine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, expectedCons)
}

func (s *DeployLocalSuite) assertCharm(c *gc.C, service *state.Service, expect *charm.URL) {
	curl, force := service.CharmURL()
	c.Assert(curl, gc.DeepEquals, expect)
	c.Assert(force, gc.Equals, false)
}

func (s *DeployLocalSuite) assertSettings(c *gc.C, service *state.Service, expect charm.Settings) {
	settings, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertConstraints(c *gc.C, service *state.Service, expect constraints.Value) {
	cons, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertMachines(c *gc.C, service *state.Service, expectCons constraints.Value, expectIds ...string) {
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	unseenIds := set.NewStrings(expectIds...)
	for _, unit := range units {
		id, err := unit.AssignedMachineId()
		c.Assert(err, gc.IsNil)
		unseenIds.Remove(id)
		machine, err := s.State.Machine(id)
		c.Assert(err, gc.IsNil)
		cons, err := machine.Constraints()
		c.Assert(err, gc.IsNil)
		c.Assert(cons, gc.DeepEquals, expectCons)
	}
	c.Assert(unseenIds, gc.DeepEquals, set.NewStrings())
}

type InitJujuHomeSuite struct {
	originalHome     string
	originalJujuHome string
}

var _ = gc.Suite(&InitJujuHomeSuite{})

func (s *InitJujuHomeSuite) SetUpTest(c *gc.C) {
	s.originalHome = osenv.Home()
	s.originalJujuHome = os.Getenv("JUJU_HOME")
}

func (s *InitJujuHomeSuite) TearDownTest(c *gc.C) {
	osenv.SetHome(s.originalHome)
	os.Setenv("JUJU_HOME", s.originalJujuHome)
}

func (s *InitJujuHomeSuite) TestJujuHome(c *gc.C) {
	os.Setenv("JUJU_HOME", "/my/juju/home")
	err := juju.InitJujuHome()
	c.Assert(err, gc.IsNil)
	c.Assert(config.JujuHome(), gc.Equals, "/my/juju/home")
}

func (s *InitJujuHomeSuite) TestHome(c *gc.C) {
	os.Setenv("JUJU_HOME", "")
	osenv.SetHome("/my/home/")
	err := juju.InitJujuHome()
	c.Assert(err, gc.IsNil)
	c.Assert(config.JujuHome(), gc.Equals, "/my/home/.juju")
}

func (s *InitJujuHomeSuite) TestError(c *gc.C) {
	os.Setenv("JUJU_HOME", "")
	osenv.SetHome("")
	err := juju.InitJujuHome()
	c.Assert(err, gc.ErrorMatches, "cannot determine juju home.*")
}

func (s *InitJujuHomeSuite) TestCacheDir(c *gc.C) {
	os.Setenv("JUJU_HOME", "/foo/bar")
	c.Assert(charm.CacheDir, gc.Equals, "")
	err := juju.InitJujuHome()
	c.Assert(err, gc.IsNil)
	c.Assert(charm.CacheDir, gc.Equals, "/foo/bar/charmcache")
}
