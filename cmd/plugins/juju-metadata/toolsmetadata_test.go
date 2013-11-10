// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/configstore"
	envtesting "github.com/jameinel/juju/environs/testing"
	"github.com/jameinel/juju/environs/tools"
	ttesting "github.com/jameinel/juju/environs/tools/testing"
	"github.com/jameinel/juju/provider/dummy"
	coretesting "github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/testing/testbase"
	"github.com/jameinel/juju/version"
)

type ToolsMetadataSuite struct {
	testbase.LoggingSuite
	home *coretesting.FakeHome
	env  environs.Environ
}

var _ = gc.Suite(&ToolsMetadataSuite{})

func (s *ToolsMetadataSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeSampleHome(c)
	s.AddCleanup(func(*gc.C) {
		s.home.Restore()
		dummy.Reset()
		loggo.ResetLoggers()
	})
	env, err := environs.PrepareFromName("erewhemos", configstore.NewMem())
	c.Assert(err, gc.IsNil)
	s.env = env
	envtesting.RemoveAllTools(c, s.env)
	loggo.GetLogger("").SetLogLevel(loggo.INFO)
}

var currentVersionStrings = []string{
	// only these ones will make it into the JSON files.
	version.Current.Number.String() + "-quantal-amd64",
	version.Current.Number.String() + "-quantal-arm",
	version.Current.Number.String() + "-quantal-i386",
}

var versionStrings = append([]string{
	"1.12.0-precise-amd64",
	"1.12.0-precise-i386",
	"1.12.0-raring-amd64",
	"1.12.0-raring-i386",
	"1.13.0-precise-amd64",
}, currentVersionStrings...)

var expectedOutputCommon = makeExpectedOutputCommon()

func makeExpectedOutputCommon() string {
	expected := `Finding tools\.\.\.
.*Fetching tools to generate hash: 1\.12\.0-precise-amd64
.*Fetching tools to generate hash: 1\.12\.0-precise-i386
.*Fetching tools to generate hash: 1\.12\.0-raring-amd64
.*Fetching tools to generate hash: 1\.12\.0-raring-i386
.*Fetching tools to generate hash: 1\.13\.0-precise-amd64
`
	f := ".*Fetching tools to generate hash: %s\n"
	for _, v := range currentVersionStrings {
		expected += fmt.Sprintf(f, regexp.QuoteMeta(v))
	}
	return strings.TrimSpace(expected)
}

var expectedOutputDirectory = expectedOutputCommon + `
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`
var expectedOutputMirrors = expectedOutputCommon + `
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
.*Writing tools/streams/v1/mirrors\.json
`

func (s *ToolsMetadataSuite) TestGenerateDefaultDirectory(c *gc.C) {
	metadataDir := config.JujuHome() // default metadata dir
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noPublic: true}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectory)
	metadata := ttesting.ParseMetadata(c, metadataDir, false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateDirectory(c *gc.C) {
	metadataDir := c.MkDir()
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noPublic: true}, ctx, []string{"-d", metadataDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectory)
	metadata := ttesting.ParseMetadata(c, metadataDir, false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateWithMirrors(c *gc.C) {
	metadataDir := c.MkDir()
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noPublic: true}, ctx, []string{"--public", "-d", metadataDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputMirrors)
	metadata := ttesting.ParseMetadata(c, metadataDir, true)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestNoTools(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noPublic: true}, ctx, nil)
	c.Assert(code, gc.Equals, 1)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Matches, "Finding tools\\.\\.\\.\n")
	stderr := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(stderr, gc.Matches, "error: no tools available\n")
}

func (s *ToolsMetadataSuite) TestPatchLevels(c *gc.C) {
	currentVersion := version.Current.Number
	currentVersion.Build = 0
	versionStrings := []string{
		currentVersion.String() + "-precise-amd64",
		currentVersion.String() + ".1-precise-amd64",
	}
	metadataDir := config.JujuHome() // default metadata dir
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noPublic: true}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expectedOutput := fmt.Sprintf(`
Finding tools\.\.\.
.*Fetching tools to generate hash: %s
.*Fetching tools to generate hash: %s
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]))
	c.Assert(output, gc.Matches, expectedOutput)
	metadata := ttesting.ParseMetadata(c, metadataDir, false)
	c.Assert(metadata, gc.HasLen, 2)

	filename := fmt.Sprintf("juju-%s-precise-amd64.tgz", currentVersion)
	size, sha256 := ttesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "releases", filename))
	c.Assert(metadata[0], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String(),
		Arch:     "amd64",
		Size:     size,
		Path:     "releases/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})

	filename = fmt.Sprintf("juju-%s.1-precise-amd64.tgz", currentVersion)
	size, sha256 = ttesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "releases", filename))
	c.Assert(metadata[1], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String() + ".1",
		Arch:     "amd64",
		Size:     size,
		Path:     "releases/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})
}
