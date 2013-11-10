// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"os"

	"launchpad.net/gnuflag"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/juju"
	"github.com/jameinel/juju/juju/osenv"
	"github.com/jameinel/juju/names"
)

type DeployCommand struct {
	cmd.EnvCommandBase
	UnitCommandBase
	CharmName    string
	ServiceName  string
	Config       cmd.FileVar
	Constraints  constraints.Value
	BumpRevision bool
	RepoPath     string // defaults to JUJU_REPOSITORY
}

const deployDoc = `
<charm name> can be a charm URL, or an unambiguously condensed form of it;
assuming a current default series of "precise", the following forms will be
accepted.

For cs:precise/mysql
  mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/mysql

For local:precise/mysql
  local:mysql

In all cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

<service name>, if omitted, will be derived from <charm name>.

Constraints can be specified when using deploy by specifying the --constraints
flag.  When used with deploy, service-specific constraints are set so that later
machines provisioned with add-unit will use the same constraints (unless changed
by set-constraints).

Charms can be deployed to a specific machine using the --to argument.

Examples:
   juju deploy mysql --to 23       (Deploy to machine 23)
   juju deploy mysql --to 24/lxc/3 (Deploy to lxc container 3 on host machine 24)
   juju deploy mysql --to lxc:25   (Deploy to a new lxc container on host machine 25)
   
   juju deploy mysql -n 5 --constraints mem=8G (deploy 5 instances of mysql with at least 8 GB of RAM each)

See Also:
   juju help constraints
   juju help set-constraints
   juju help get-constraints
`

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "deploy",
		Args:    "<charm name> [<service name>]",
		Purpose: "deploy a new service",
		Doc:     deployDoc,
	}
}

func (c *DeployCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy for principal charms")
	f.BoolVar(&c.BumpRevision, "u", false, "increment local charm directory revision")
	f.BoolVar(&c.BumpRevision, "upgrade", false, "")
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set service constraints")
	f.StringVar(&c.RepoPath, "repository", os.Getenv(osenv.JujuRepository), "local charm repository")
}

func (c *DeployCommand) Init(args []string) error {
	switch len(args) {
	case 2:
		if !names.IsService(args[1]) {
			return fmt.Errorf("invalid service name %q", args[1])
		}
		c.ServiceName = args[1]
		fallthrough
	case 1:
		if _, err := charm.InferURL(args[0], "fake"); err != nil {
			return fmt.Errorf("invalid charm name %q", args[0])
		}
		c.CharmName = args[0]
	case 0:
		return errors.New("no charm specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	return c.UnitCommandBase.Init(args)
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	conf, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	curl, err := charm.InferURL(c.CharmName, conf.DefaultSeries())
	if err != nil {
		return err
	}
	repo, err := charm.InferRepository(curl, ctx.AbsPath(c.RepoPath))
	if err != nil {
		return err
	}
	// TODO(fwereade) it's annoying to roundtrip the bytes through the client
	// here, but it's the original behaviour and not convenient to change.
	// PutCharm will always be required in some form for local charms; and we
	// will need an EnsureStoreCharm method somewhere that gets the state.Charm
	// for use in the following checks.
	ch, err := conn.PutCharm(curl, repo, c.BumpRevision)
	if err != nil {
		return err
	}
	numUnits := c.NumUnits
	if ch.Meta().Subordinate {
		if !constraints.IsEmpty(&c.Constraints) {
			return errors.New("cannot use --constraints with subordinate service")
		}
		if numUnits == 1 && c.ToMachineSpec == "" {
			numUnits = 0
		} else {
			return errors.New("cannot use --num-units or --to with subordinate service")
		}
	}
	serviceName := c.ServiceName
	if serviceName == "" {
		serviceName = ch.Meta().Name
	}
	var settings charm.Settings
	if c.Config.Path != "" {
		configYAML, err := c.Config.Read(ctx)
		if err != nil {
			return err
		}
		settings, err = ch.Config().ParseSettingsYAML(configYAML, serviceName)
		if err != nil {
			return err
		}
	}
	_, err = conn.DeployService(juju.DeployServiceParams{
		ServiceName:    serviceName,
		Charm:          ch,
		NumUnits:       numUnits,
		ConfigSettings: settings,
		Constraints:    c.Constraints,
		ToMachineSpec:  c.ToMachineSpec,
	})
	return err
}
