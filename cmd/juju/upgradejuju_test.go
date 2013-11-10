// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"strings"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/filestorage"
	"github.com/jameinel/juju/environs/storage"
	"github.com/jameinel/juju/environs/sync"
	envtesting "github.com/jameinel/juju/environs/testing"
	envtools "github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/juju/testing"
	coretesting "github.com/jameinel/juju/testing"
	jc "github.com/jameinel/juju/testing/checkers"
	coretools "github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
)

type UpgradeJujuSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&UpgradeJujuSuite{})

var upgradeJujuTests = []struct {
	about          string
	private        []string
	public         []string
	currentVersion string
	agentVersion   string
	development    bool

	args           []string
	expectInitErr  string
	expectErr      string
	expectVersion  string
	expectUploaded []string
}{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --version value",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"--version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "major version upgrade to incompatible version",
	currentVersion: "2.0.0-quantal-amd64",
	args:           []string{"--version", "5.2.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "major version downgrade to incompatible version",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--version", "3.2.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "invalid --series",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--series", "precise&quantal"},
	expectInitErr:  `invalid value "precise&quantal" for flag --series: .*`,
}, {
	about:          "--series without --upload-tools",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--series", "precise,quantal"},
	expectInitErr:  "--series requires --upload-tools",
}, {
	about:          "--upload-tools with inappropriate version 1",
	currentVersion: "4.2.0-quantal-amd64",
	args:           []string{"--upload-tools", "--version", "3.1.0"},
	expectInitErr:  "cannot upgrade to version incompatible with CLI",
}, {
	about:          "--upload-tools with inappropriate version 2",
	currentVersion: "3.2.7-quantal-amd64",
	args:           []string{"--upload-tools", "--version", "3.1.0.4"},
	expectInitErr:  "cannot specify build number when uploading tools",
}, {
	about:          "latest release from private storage",
	private:        []string{"2.0.0-quantal-amd64", "2.0.2-quantal-i386", "2.0.3-quantal-amd64"},
	public:         []string{"2.0.0-quantal-amd64", "2.0.4-quantal-amd64", "2.0.5-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.3",
}, {
	about:          "latest dev from private storage (because client is dev)",
	private:        []string{"2.0.0-quantal-amd64", "2.2.0-quantal-amd64", "2.3.0-quantal-amd64", "3.0.1-quantal-amd64"},
	public:         []string{"2.0.0-quantal-amd64", "2.4.0-quantal-amd64", "2.5.0-quantal-amd64"},
	currentVersion: "2.1.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.3.0",
}, {
	about:          "latest dev from private storage (because agent is dev)",
	private:        []string{"2.0.0-quantal-amd64", "2.2.0-quantal-amd64", "2.3.0-quantal-amd64", "3.0.1-quantal-amd64"},
	public:         []string{"2.0.0-quantal-amd64", "2.4.0-quantal-amd64", "2.5.0-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.1.0",
	expectVersion:  "2.3.0",
}, {
	about:          "latest dev from private storage (because --dev flag)",
	private:        []string{"2.0.0-quantal-amd64", "2.2.0-quantal-amd64", "2.3.0-quantal-amd64"},
	public:         []string{"2.0.0-quantal-amd64", "2.4.0-quantal-amd64", "2.5.0-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	args:           []string{"--dev"},
	agentVersion:   "2.0.0",
	expectVersion:  "2.3.0",
}, {
	about:          "latest dev from private storage (because dev env setting)",
	private:        []string{"2.0.0-quantal-amd64", "2.2.0-quantal-amd64", "2.3.0-quantal-amd64"},
	public:         []string{"2.0.0-quantal-amd64", "2.4.0-quantal-amd64", "2.5.0-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	development:    true,
	agentVersion:   "2.0.0",
	expectVersion:  "2.3.0",
}, {
	about:          "specified version",
	private:        []string{"2.3.0-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--version", "2.3.0"},
	expectVersion:  "2.3.0",
}, {
	about:          "specified version missing, but already set",
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.0.0"},
	expectVersion:  "3.0.0",
}, {
	about:          "specified version, no tools",
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching major version",
	private:        []string{"4.2.0-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching minor version",
	private:        []string{"3.4.0-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching patch version",
	private:        []string{"3.2.5-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "specified version, no matching build version",
	private:        []string{"3.2.0.2-quantal-amd64"},
	currentVersion: "3.0.0-quantal-amd64",
	agentVersion:   "3.0.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "no matching tools available",
}, {
	about:          "major version downgrade to incompatible version",
	private:        []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "4.2.0",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "cannot change major version from 4 to 3",
}, {
	about:          "major version upgrade to compatible version",
	private:        []string{"3.2.0-quantal-amd64"},
	currentVersion: "3.2.0-quantal-amd64",
	agentVersion:   "2.8.2",
	args:           []string{"--version", "3.2.0"},
	expectErr:      "major version upgrades are not supported yet",
}, {
	about:          "nothing available 1",
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "nothing available 2",
	currentVersion: "2.0.0-quantal-amd64",
	public:         []string{"3.2.0-quantal-amd64"},
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "nothing available 3",
	currentVersion: "2.0.0-quantal-amd64",
	private:        []string{"3.2.0-quantal-amd64"},
	public:         []string{"3.4.0-quantal-amd64"},
	agentVersion:   "2.0.0",
	expectVersion:  "2.0.0",
}, {
	about:          "upload with default series",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.2.0.1",
	expectUploaded: []string{"2.2.0.1-quantal-amd64", "2.2.0.1-precise-amd64", "2.2.0.1-raring-amd64"},
}, {
	about:          "upload with explicit version",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--version", "2.7.3"},
	expectVersion:  "2.7.3.1",
	expectUploaded: []string{"2.7.3.1-quantal-amd64", "2.7.3.1-precise-amd64", "2.7.3.1-raring-amd64"},
}, {
	about:          "upload with explicit series",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--series", "raring"},
	expectVersion:  "2.2.0.1",
	expectUploaded: []string{"2.2.0.1-quantal-amd64", "2.2.0.1-raring-amd64"},
}, {
	about:          "upload dev version, currently on release version",
	currentVersion: "2.1.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.1.0.1",
	expectUploaded: []string{"2.1.0.1-quantal-amd64", "2.1.0.1-precise-amd64", "2.1.0.1-raring-amd64"},
}, {
	about:          "upload bumps version when necessary",
	private:        []string{"2.4.6-quantal-amd64", "2.4.8-quantal-amd64"},
	public:         []string{"2.4.6.4-quantal-amd64"}, //ignored
	currentVersion: "2.4.6-quantal-amd64",
	agentVersion:   "2.4.0",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.4.6.1",
	expectUploaded: []string{"2.4.6.1-quantal-amd64", "2.4.6.1-precise-amd64", "2.4.6.1-raring-amd64"},
}, {
	about:          "upload re-bumps version when necessary",
	private:        []string{"2.4.6-quantal-amd64", "2.4.6.2-saucy-i386", "2.4.8-quantal-amd64"},
	public:         []string{"2.4.6.10-quantal-amd64"}, //ignored
	currentVersion: "2.4.6-quantal-amd64",
	agentVersion:   "2.4.6.2",
	args:           []string{"--upload-tools"},
	expectVersion:  "2.4.6.3",
	expectUploaded: []string{"2.4.6.3-quantal-amd64", "2.4.6.3-precise-amd64", "2.4.6.3-raring-amd64"},
}, {
	about:          "upload with explicit version bumps when necessary",
	currentVersion: "2.2.0-quantal-amd64",
	private:        []string{"2.7.3.1-quantal-amd64"},
	agentVersion:   "2.0.0",
	args:           []string{"--upload-tools", "--version", "2.7.3"},
	expectVersion:  "2.7.3.2",
	expectUploaded: []string{"2.7.3.2-quantal-amd64", "2.7.3.2-precise-amd64", "2.7.3.2-raring-amd64"},
}}

// mockUploadTools simulates the effect of tools.Upload, but skips the time-
// consuming build from source.
// TODO(fwereade) better factor agent/tools such that build logic is
// exposed and can itself be neatly mocked?
func mockUploadTools(stor storage.Storage, forceVersion *version.Number, series ...string) (*coretools.Tools, error) {
	vers := version.Current
	if forceVersion != nil {
		vers.Number = *forceVersion
	}
	versions := []version.Binary{vers}
	for _, series := range series {
		if series != version.Current.Series {
			newVers := vers
			newVers.Series = series
			versions = append(versions, newVers)
		}
	}
	agentTools, err := envtesting.UploadFakeToolsVersions(stor, versions...)
	if err != nil {
		return nil, err
	}
	return agentTools[0], nil
}

func (s *UpgradeJujuSuite) TestUpgradeJuju(c *gc.C) {
	oldVersion := version.Current
	uploadTools = mockUploadTools
	defer func() {
		version.Current = oldVersion
		uploadTools = sync.Upload
	}()

	for i, test := range upgradeJujuTests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)

		// Set up apparent CLI version and initialize the command.
		version.Current = version.MustParseBinary(test.currentVersion)
		com := &UpgradeJujuCommand{}
		if err := coretesting.InitCommand(com, test.args); err != nil {
			if test.expectInitErr != "" {
				c.Check(err, gc.ErrorMatches, test.expectInitErr)
			} else {
				c.Check(err, gc.IsNil)
			}
			continue
		}

		// Set up state and environ, and run the command.
		cfg, err := s.State.EnvironConfig()
		c.Assert(err, gc.IsNil)
		toolsDir := c.MkDir()
		cfg, err = cfg.Apply(map[string]interface{}{
			"agent-version":      test.agentVersion,
			"development":        test.development,
			"tools-metadata-url": "file://" + toolsDir,
		})
		c.Assert(err, gc.IsNil)
		err = s.State.SetEnvironConfig(cfg)
		c.Assert(err, gc.IsNil)
		versions := make([]version.Binary, len(test.private))
		for i, v := range test.private {
			versions[i] = version.MustParseBinary(v)

		}
		envtesting.MustUploadFakeToolsVersions(s.Conn.Environ.Storage(), versions...)
		versions = make([]version.Binary, len(test.public))
		for i, v := range test.public {
			versions[i] = version.MustParseBinary(v)
		}
		stor, err := filestorage.NewFileStorageWriter(toolsDir, "")
		c.Assert(err, gc.IsNil)
		envtesting.MustUploadFakeToolsVersions(stor, versions...)
		err = com.Run(coretesting.Context(c))
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			continue
		} else if !c.Check(err, gc.IsNil) {
			continue
		}

		// Check expected changes to environ/state.
		cfg, err = s.State.EnvironConfig()
		c.Check(err, gc.IsNil)
		agentVersion, ok := cfg.AgentVersion()
		c.Check(ok, gc.Equals, true)
		c.Check(agentVersion, gc.Equals, version.MustParse(test.expectVersion))
		c.Check(cfg.Development(), gc.Equals, test.development)

		for _, uploaded := range test.expectUploaded {
			vers := version.MustParseBinary(uploaded)
			r, err := storage.Get(s.Conn.Environ.Storage(), envtools.StorageName(vers))
			if !c.Check(err, gc.IsNil) {
				continue
			}
			data, err := ioutil.ReadAll(r)
			r.Close()
			c.Check(err, gc.IsNil)
			checkToolsContent(c, data, "jujud contents "+uploaded)
		}
	}
}

func checkToolsContent(c *gc.C, data []byte, uploaded string) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	c.Check(err, gc.IsNil)
	defer zr.Close()
	tr := tar.NewReader(zr)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Check(err, gc.IsNil)
		if strings.ContainsAny(hdr.Name, "/\\") {
			c.Fail()
		}
		if hdr.Typeflag != tar.TypeReg {
			c.Fail()
		}
		content, err := ioutil.ReadAll(tr)
		c.Check(err, gc.IsNil)
		c.Check(string(content), gc.Equals, uploaded)
		found = true
	}
	c.Check(found, jc.IsTrue)
}

// JujuConnSuite very helpfully uploads some default
// tools to the environment's storage. We don't want
// 'em there; but we do want a consistent default-series
// in the environment state.
func (s *UpgradeJujuSuite) Reset(c *gc.C) {
	s.JujuConnSuite.Reset(c)
	envtesting.RemoveTools(c, s.Conn.Environ.Storage())
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	cfg, err = cfg.Apply(map[string]interface{}{
		"default-series": "raring",
		"agent-version":  "1.2.3",
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, gc.IsNil)
}

func (s *UpgradeJujuSuite) TestUpgradeJujuWithRealUpload(c *gc.C) {
	s.Reset(c)
	_, err := coretesting.RunCommand(c, &UpgradeJujuCommand{}, []string{"--upload-tools"})
	c.Assert(err, gc.IsNil)
	vers := version.Current
	vers.Build = 1
	tools, err := envtools.FindInstanceTools(s.Conn.Environ, vers.Number, vers.Series, &vers.Arch)
	c.Assert(err, gc.IsNil)
	c.Assert(len(tools), gc.Equals, 1)
}
