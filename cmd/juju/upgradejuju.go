// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	stderrors "errors"
	"fmt"

	"launchpad.net/gnuflag"

	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/storage"
	"github.com/jameinel/juju/environs/sync"
	envtools "github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/juju"
	"github.com/jameinel/juju/log"
	coretools "github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	cmd.EnvCommandBase
	vers        string
	Version     version.Number
	Development bool
	UploadTools bool
	Series      []string
}

var uploadTools = sync.Upload

var upgradeJujuDoc = `
The upgrade-juju command upgrades a running environment by setting a version
number for all juju agents to run. By default, it chooses the most recent non-
development version compatible with the command-line envtools.

A development version is defined to be any version with an odd minor version
or a nonzero build component (for example version 2.1.1, 3.3.0 and 2.0.0.1 are
development versions; 2.0.3 and 3.4.1 are not). A development version may be
chosen if any of the following conditions hold:

 - the current juju tool has a development version.
 - the juju environment has a development version
 - the environment "development" setting is true
 - the --dev flag is specified

For development use, the --upload-tools flag specifies that the juju tools will
be compiled locally and uploaded before the version is set. Currently the tools
will be uploaded as if they had the version of the current juju tool, unless
specified otherwise by the --version flag.
`

func (c *UpgradeJujuCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-juju",
		Purpose: "upgrade the tools in a juju environment",
		Doc:     upgradeJujuDoc,
	}
}

func (c *UpgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.vers, "version", "", "upgrade to specific version")
	f.BoolVar(&c.Development, "dev", false, "allow development versions to be chosen")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools")
	f.Var(seriesVar{&c.Series}, "series", "upload tools for supplied comma-separated series list")
}

func (c *UpgradeJujuCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := version.Parse(c.vers)
		if err != nil {
			return err
		}
		if vers.Major != version.Current.Major {
			return fmt.Errorf("cannot upgrade to version incompatible with CLI")
		}
		if c.UploadTools && vers.Build != 0 {
			// TODO(fwereade): when we start taking versions from actual built
			// code, we should disable --version when used with --upload-tools.
			// For now, it's the only way to experiment with version upgrade
			// behaviour live, so the only restriction is that Build cannot
			// be used (because its value needs to be chosen internally so as
			// not to collide with existing tools).
			return fmt.Errorf("cannot specify build number when uploading tools")
		}
		c.Version = vers
	}
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	return cmd.CheckEmpty(args)
}

var errUpToDate = stderrors.New("no upgrades available")

// Run changes the version proposed for the juju envtools.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer func() {
		if err == errUpToDate {
			log.Noticef(err.Error())
			err = nil
		}
	}()

	// Determine the version to upgrade to, uploading tools if necessary.
	env := conn.Environ
	cfg, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	v, err := c.initVersions(cfg, env)
	if err != nil {
		return err
	}
	if c.UploadTools {
		series := getUploadSeries(cfg, c.Series)
		if err := v.uploadTools(env.Storage(), series); err != nil {
			return err
		}
	}
	if err := v.validate(); err != nil {
		return err
	}
	log.Infof("upgrade version chosen: %s", v.chosen)
	// TODO(fwereade): this list may be incomplete, pending envtools.Upload change.
	log.Infof("available tools: %s", v.tools)

	// Write updated config back to state if necessary. Note that this is
	// crackful and racy, because we have no idea what incompatible agent-
	// version might be set by another administrator in the meantime. If
	// this happens, tough: I'm not going to pretend to do it right when
	// I'm not.
	// TODO(fwereade): Do this right. Warning: scope unclear.
	cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": v.chosen.String(),
	})
	if err != nil {
		return err
	}
	if err := conn.State.SetEnvironConfig(cfg); err != nil {
		return err
	}
	log.Noticef("started upgrade to %s", v.chosen)
	return nil
}

// initVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available tools, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *UpgradeJujuCommand) initVersions(cfg *config.Config, env environs.Environ) (*upgradeVersions, error) {
	agent, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return nil, fmt.Errorf("incomplete environment configuration")
	}
	if c.Version == agent {
		return nil, errUpToDate
	}
	client := version.Current.Number
	available, err := envtools.FindTools(env, client.Major, -1, coretools.Filter{}, envtools.DoNotAllowRetry)
	if err != nil {
		if !errors.IsNotFoundError(err) {
			return nil, err
		}
		if !c.UploadTools {
			if c.Version == version.Zero {
				return nil, errUpToDate
			}
			return nil, err
		}
	}
	dev := c.Development || cfg.Development() || agent.IsDev() || client.IsDev()
	return &upgradeVersions{
		dev:    dev,
		agent:  agent,
		client: client,
		chosen: c.Version,
		tools:  available,
	}, nil
}

// upgradeVersions holds the version information for making upgrade decisions.
type upgradeVersions struct {
	dev    bool
	agent  version.Number
	client version.Number
	chosen version.Number
	tools  coretools.List
}

// uploadTools compiles jujud from $GOPATH and uploads it into the supplied
// storage. If no version has been explicitly chosen, the version number
// reported by the built tools will be based on the client version number.
// In any case, the version number reported will have a build component higher
// than that of any otherwise-matching available envtools.
// uploadTools resets the chosen version and replaces the available tools
// with the ones just uploaded.
func (v *upgradeVersions) uploadTools(storage storage.Storage, series []string) error {
	// TODO(fwereade): this is kinda crack: we should not assume that
	// version.Current matches whatever source happens to be built. The
	// ideal would be:
	//  1) compile jujud from $GOPATH into some build dir
	//  2) get actual version with `jujud version`
	//  3) check actual version for compatibility with CLI tools
	//  4) generate unique build version with reference to available tools
	//  5) force-version that unique version into the dir directly
	//  6) archive and upload the build dir
	// ...but there's no way we have time for that now. In the meantime,
	// considering the use cases, this should work well enough; but it
	// won't detect an incompatible major-version change, which is a shame.
	if v.chosen == version.Zero {
		v.chosen = v.client
	}
	v.chosen = uploadVersion(v.chosen, v.tools)

	// TODO(fwereade): envtools.Upload should return envtools.List, and should
	// include all the extra series we build, so we can set *that* onto
	// v.available and maybe one day be able to check that a given upgrade
	// won't leave out-of-date machines lying around, starved of envtools.
	uploaded, err := uploadTools(storage, &v.chosen, series...)
	if err != nil {
		return err
	}
	v.tools = coretools.List{uploaded}
	return nil
}

// validate chooses an upgrade version, if one has not already been chosen,
// and ensures the tools list contains no entries that do not have that version.
// If validate returns no error, the environment agent-version can be set to
// the value of the chosen field.
func (v *upgradeVersions) validate() (err error) {
	// If not completely specified already, pick a single tools version.
	v.dev = v.dev || v.chosen.IsDev()
	filter := coretools.Filter{Number: v.chosen, Released: !v.dev}
	if v.tools, err = v.tools.Match(filter); err != nil {
		return err
	}
	v.chosen, v.tools = v.tools.Newest()
	if v.chosen == v.agent {
		return errUpToDate
	}

	// Major version upgrade
	if v.chosen.Major < v.agent.Major {
		// TODO(fwereade): I'm a bit concerned about old agent/CLI tools even
		// *connecting* to environments with higher agent-versions; but ofc they
		// have to connect in order to discover they shouldn't. However, once
		// any of our tools detect an incompatible version, they should act to
		// minimize damage: the CLI should abort politely, and the agents should
		// run an Upgrader but no other tasks.
		return fmt.Errorf("cannot change major version from %d to %d", v.agent.Major, v.chosen.Major)
	} else if v.chosen.Major > v.agent.Major {
		return fmt.Errorf("major version upgrades are not supported yet")
	}

	return nil
}

// uploadVersion returns a copy of the supplied version with a build number
// higher than any of the supplied tools that share its major, minor and patch.
func uploadVersion(vers version.Number, existing coretools.List) version.Number {
	vers.Build++
	for _, t := range existing {
		if t.Version.Major != vers.Major || t.Version.Minor != vers.Minor || t.Version.Patch != vers.Patch {
			continue
		}
		if t.Version.Build >= vers.Build {
			vers.Build = t.Version.Build + 1
		}
	}
	return vers
}
