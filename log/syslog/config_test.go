// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/log/syslog"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type SyslogConfigSuite struct {
	configDir string
}

var _ = gc.Suite(&SyslogConfigSuite{})

func (s *SyslogConfigSuite) SetUpTest(c *gc.C) {
	s.configDir = c.MkDir()
}

func (s *SyslogConfigSuite) assertRsyslogConfigPath(c *gc.C, slConfig *syslog.SyslogConfig) {
	slConfig.ConfigDir = s.configDir
	slConfig.ConfigFileName = "rsyslog.conf"
	c.Assert(slConfig.ConfigFilePath(), gc.Equals, filepath.Join(s.configDir, "rsyslog.conf"))
}

func (s *SyslogConfigSuite) assertRsyslogConfigContents(c *gc.C, slConfig *syslog.SyslogConfig,
	expectedConf string) {
	data, err := slConfig.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, expectedConf)
}

var expectedAccumulateSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-some-machine-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag local-juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun 514

# Messages received from remote rsyslog machines contain a leading space so we
# need to account for that.
$template JujuLogFormatLocal,"%HOSTNAME%:%msg:::drop-last-lf%\n"
$template JujuLogFormat,"%HOSTNAME%:%msg:2:2048:drop-last-lf%\n"

:syslogtag, startswith, "juju-" /var/log/juju/all-machines.log;JujuLogFormat
& ~
:syslogtag, startswith, "local-juju-" /var/log/juju/all-machines.log;JujuLogFormatLocal
& ~
`

func (s *SyslogConfigSuite) TestAccumulateConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine")
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedAccumulateSyslogConf)
}

func (s *SyslogConfigSuite) TestAccumulateConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine")
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(syslogConfData), gc.Equals, expectedAccumulateSyslogConf)
}

var expectedForwardSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-some-machine-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

:syslogtag, startswith, "juju-" @server:514
& ~
`

func (s *SyslogConfigSuite) TestForwardConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", []string{"server"})
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedForwardSyslogConf)
}

func (s *SyslogConfigSuite) TestForwardConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", []string{"server"})
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(syslogConfData), gc.Equals, expectedForwardSyslogConf)
}
