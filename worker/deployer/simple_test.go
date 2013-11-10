// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/agent"
	"github.com/jameinel/juju/agent/tools"
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
	"github.com/jameinel/juju/testing/testbase"
	coretools "github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
	"github.com/jameinel/juju/worker/deployer"
)

type SimpleContextSuite struct {
	SimpleToolsFixture
}

var _ = gc.Suite(&SimpleContextSuite{})

func (s *SimpleContextSuite) SetUpTest(c *gc.C) {
	s.SimpleToolsFixture.SetUp(c, c.MkDir())
}

func (s *SimpleContextSuite) TearDownTest(c *gc.C) {
	s.SimpleToolsFixture.TearDown(c)
}

func (s *SimpleContextSuite) TestDeployRecall(c *gc.C) {
	mgr0 := s.getContext(c)
	units, err := mgr0.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
	s.assertUpstartCount(c, 0)

	err = mgr0.DeployUnit("foo/123", "some-password")
	c.Assert(err, gc.IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.DeepEquals, []string{"foo/123"})
	s.assertUpstartCount(c, 1)
	s.checkUnitInstalled(c, "foo/123", "some-password")

	err = mgr0.RecallUnit("foo/123")
	c.Assert(err, gc.IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
	s.assertUpstartCount(c, 0)
	s.checkUnitRemoved(c, "foo/123")
}

func (s *SimpleContextSuite) TestOldDeployedUnitsCanBeRecalled(c *gc.C) {
	// After r1347 deployer tag is no longer part of the upstart conf filenames,
	// now only the units' tags are used. This change is with the assumption only
	// one deployer will be running on a machine (in the machine agent as a task,
	// unlike before where there was one in the unit agent as well).
	// This test ensures units deployed previously (or their upstart confs more
	// specifically) can be detected and recalled by the deployer.

	manager := s.getContext(c)

	// No deployed units at first.
	units, err := manager.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
	s.assertUpstartCount(c, 0)

	// Trying to recall any units will fail.
	err = manager.RecallUnit("principal/1")
	c.Assert(err, gc.ErrorMatches, `unit "principal/1" is not deployed`)

	// Simulate some previously deployed units with the old
	// upstart conf filename format (+deployer tags).
	s.injectUnit(c, "jujud-machine-0:unit-mysql-0.conf", "unit-mysql-0")
	s.assertUpstartCount(c, 1)
	s.injectUnit(c, "jujud-unit-wordpress-0:unit-nrpe-0.conf", "unit-nrpe-0")
	s.assertUpstartCount(c, 2)

	// Make sure we can discover them.
	units, err = manager.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 2)
	sort.Strings(units)
	c.Assert(units, gc.DeepEquals, []string{"mysql/0", "nrpe/0"})

	// Deploy some units.
	err = manager.DeployUnit("principal/1", "some-password")
	c.Assert(err, gc.IsNil)
	s.checkUnitInstalled(c, "principal/1", "some-password")
	s.assertUpstartCount(c, 3)
	err = manager.DeployUnit("subordinate/2", "fake-password")
	c.Assert(err, gc.IsNil)
	s.checkUnitInstalled(c, "subordinate/2", "fake-password")
	s.assertUpstartCount(c, 4)

	// Verify the newly deployed units are also discoverable.
	units, err = manager.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 4)
	sort.Strings(units)
	c.Assert(units, gc.DeepEquals, []string{"mysql/0", "nrpe/0", "principal/1", "subordinate/2"})

	// Recall all of them - should work ok.
	unitCount := 4
	for _, unitName := range units {
		err = manager.RecallUnit(unitName)
		c.Assert(err, gc.IsNil)
		unitCount--
		s.checkUnitRemoved(c, unitName)
		s.assertUpstartCount(c, unitCount)
	}

	// Verify they're no longer discoverable.
	units, err = manager.DeployedUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
}

type SimpleToolsFixture struct {
	testbase.LoggingSuite
	dataDir         string
	initDir         string
	logDir          string
	origPath        string
	binDir          string
	syslogConfigDir string
}

var fakeJujud = "#!/bin/bash --norc\n# fake-jujud\nexit 0\n"

func (fix *SimpleToolsFixture) SetUp(c *gc.C, dataDir string) {
	fix.LoggingSuite.SetUpTest(c)
	fix.dataDir = dataDir
	fix.initDir = c.MkDir()
	fix.logDir = c.MkDir()
	fix.syslogConfigDir = c.MkDir()
	toolsDir := tools.SharedToolsDir(fix.dataDir, version.Current)
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, gc.IsNil)
	jujudPath := filepath.Join(toolsDir, "jujud")
	err = ioutil.WriteFile(jujudPath, []byte(fakeJujud), 0755)
	c.Assert(err, gc.IsNil)
	toolsPath := filepath.Join(toolsDir, "downloaded-tools.txt")
	testTools := coretools.Tools{Version: version.Current, URL: "http://testing.invalid/tools"}
	data, err := json.Marshal(testTools)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(toolsPath, data, 0644)
	c.Assert(err, gc.IsNil)
	fix.binDir = c.MkDir()
	fix.origPath = os.Getenv("PATH")
	os.Setenv("PATH", fix.binDir+":"+fix.origPath)
	fix.makeBin(c, "status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "stopped-status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "started-status", `echo "blah start/running, process 666"`)
	fix.makeBin(c, "start", "cp $(which started-status) $(which status)")
	fix.makeBin(c, "stop", "cp $(which stopped-status) $(which status)")
}

func (fix *SimpleToolsFixture) TearDown(c *gc.C) {
	os.Setenv("PATH", fix.origPath)
	fix.LoggingSuite.TearDownTest(c)
}

func (fix *SimpleToolsFixture) makeBin(c *gc.C, name, script string) {
	path := filepath.Join(fix.binDir, name)
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\n"+script), 0755)
	c.Assert(err, gc.IsNil)
}

func (fix *SimpleToolsFixture) assertUpstartCount(c *gc.C, count int) {
	fis, err := ioutil.ReadDir(fix.initDir)
	c.Assert(err, gc.IsNil)
	c.Assert(fis, gc.HasLen, count)
}

func (fix *SimpleToolsFixture) getContext(c *gc.C) *deployer.SimpleContext {
	config := agentConfig("machine-tag", fix.dataDir)
	return deployer.NewTestSimpleContext(config, fix.initDir, fix.logDir, fix.syslogConfigDir)
}

func (fix *SimpleToolsFixture) getContextForMachine(c *gc.C, machineTag string) *deployer.SimpleContext {
	config := agentConfig(machineTag, fix.dataDir)
	return deployer.NewTestSimpleContext(config, fix.initDir, fix.logDir, fix.syslogConfigDir)
}

func (fix *SimpleToolsFixture) paths(tag string) (confPath, agentDir, toolsDir, syslogConfPath string) {
	confName := fmt.Sprintf("jujud-%s.conf", tag)
	confPath = filepath.Join(fix.initDir, confName)
	agentDir = agent.Dir(fix.dataDir, tag)
	toolsDir = tools.ToolsDir(fix.dataDir, tag)
	syslogConfPath = filepath.Join(fix.syslogConfigDir, fmt.Sprintf("26-juju-%s.conf", tag))
	return
}

var expectedSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-%s-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/%s.log
$InputFileTag juju-%s:
$InputFileStateFile %s
$InputRunFileMonitor

:syslogtag, startswith, "juju-" @s1:514
& ~
`

func (fix *SimpleToolsFixture) checkUnitInstalled(c *gc.C, name, password string) {
	tag := names.UnitTag(name)
	uconfPath, _, toolsDir, syslogConfPath := fix.paths(tag)
	uconfData, err := ioutil.ReadFile(uconfPath)
	c.Assert(err, gc.IsNil)
	uconf := string(uconfData)
	var execLine string
	for _, line := range strings.Split(uconf, "\n") {
		if strings.HasPrefix(line, "exec ") {
			execLine = line
			break
		}
	}
	if execLine == "" {
		c.Fatalf("no command found in %s:\n%s", uconfPath, uconf)
	}
	logPath := filepath.Join(fix.logDir, tag+".log")
	jujudPath := filepath.Join(toolsDir, "jujud")
	for _, pat := range []string{
		"^exec " + jujudPath + " unit ",
		" --unit-name " + name + " ",
		" >> " + logPath + " 2>&1$",
	} {
		match, err := regexp.MatchString(pat, execLine)
		c.Assert(err, gc.IsNil)
		if !match {
			c.Fatalf("failed to match:\n%s\nin:\n%s", pat, execLine)
		}
	}

	conf, err := agent.ReadConf(fix.dataDir, tag)
	c.Assert(err, gc.IsNil)
	c.Assert(conf.Tag(), gc.Equals, tag)
	c.Assert(conf.DataDir(), gc.Equals, fix.dataDir)

	jujudData, err := ioutil.ReadFile(jujudPath)
	c.Assert(err, gc.IsNil)
	c.Assert(string(jujudData), gc.Equals, fakeJujud)

	syslogConfData, err := ioutil.ReadFile(syslogConfPath)
	c.Assert(err, gc.IsNil)
	parts := strings.SplitN(name, "/", 2)
	unitTag := fmt.Sprintf("unit-%s-%s", parts[0], parts[1])
	expectedSyslogConfReplaced := fmt.Sprintf(expectedSyslogConf, unitTag, unitTag, unitTag, unitTag)
	c.Assert(string(syslogConfData), gc.Equals, expectedSyslogConfReplaced)

}

func (fix *SimpleToolsFixture) checkUnitRemoved(c *gc.C, name string) {
	tag := names.UnitTag(name)
	confPath, agentDir, toolsDir, syslogConfPath := fix.paths(tag)
	for _, path := range []string{confPath, agentDir, toolsDir, syslogConfPath} {
		_, err := ioutil.ReadFile(path)
		if err == nil {
			c.Log("Warning: %q not removed as expected", path)
		} else {
			c.Assert(err, jc.Satisfies, os.IsNotExist)
		}
	}
}

func (fix *SimpleToolsFixture) injectUnit(c *gc.C, upstartConf, unitTag string) {
	confPath := filepath.Join(fix.initDir, upstartConf)
	err := ioutil.WriteFile(confPath, []byte("#!/bin/bash --norc\necho $0"), 0644)
	c.Assert(err, gc.IsNil)
	toolsDir := filepath.Join(fix.dataDir, "tools", unitTag)
	err = os.MkdirAll(toolsDir, 0755)
	c.Assert(err, gc.IsNil)
}

type mockConfig struct {
	agent.Config
	tag     string
	datadir string
}

func (mock *mockConfig) Tag() string {
	return mock.tag
}

func (mock *mockConfig) DataDir() string {
	return mock.datadir
}

func (mock *mockConfig) CACert() []byte {
	return []byte(testing.CACert)
}

func (mock *mockConfig) Value(_ string) string {
	return ""
}

func agentConfig(tag, datadir string) agent.Config {
	return &mockConfig{tag: tag, datadir: datadir}
}
