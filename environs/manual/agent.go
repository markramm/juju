// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/jameinel/juju/agent"
	corecloudinit "github.com/jameinel/juju/cloudinit"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/cloudinit"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/provider"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/utils"
)

type provisionMachineAgentArgs struct {
	host          string
	dataDir       string
	bootstrap     bool
	environConfig *config.Config
	machineId     string
	nonce         string
	stateFileURL  string
	stateInfo     *state.Info
	apiInfo       *api.Info
	tools         *tools.Tools

	// agentEnv is an optional map of
	// arbitrary key/value pairs to pass
	// into the machine agent.
	agentEnv map[string]string
}

// provisionMachineAgent connects to a machine over SSH,
// copies across the tools, and installs a machine agent.
func provisionMachineAgent(args provisionMachineAgentArgs) error {
	logger.Infof("Provisioning machine agent on %s", args.host)
	script, err := provisionMachineAgentScript(args)
	if err != nil {
		return err
	}
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	cmd := sshCommand(
		args.host,
		fmt.Sprintf("sudo bash -c '%s'", script),
		allocateTTY,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// provisionMachineAgentScript generates the script necessary
// to install a machine agent on the specified host.
func provisionMachineAgentScript(args provisionMachineAgentArgs) (string, error) {
	// We generate a cloud-init config, which we'll then pull the runcmds
	// and prerequisite packages out of. Rather than generating a cloud-config,
	// we'll just generate a shell script.
	var mcfg *cloudinit.MachineConfig
	if args.bootstrap {
		mcfg = environs.NewBootstrapMachineConfig(args.stateFileURL)
	} else {
		mcfg = environs.NewMachineConfig(args.machineId, args.nonce, args.stateInfo, args.apiInfo)
	}
	if args.dataDir != "" {
		mcfg.DataDir = args.dataDir
	}
	mcfg.Tools = args.tools
	err := environs.FinishMachineConfig(mcfg, args.environConfig, constraints.Value{})
	if err != nil {
		return "", err
	}
	mcfg.AgentEnvironment[agent.ProviderType] = provider.Null
	for k, v := range args.agentEnv {
		mcfg.AgentEnvironment[k] = v
	}
	cloudcfg := corecloudinit.New()
	if cloudcfg, err = cloudinit.Configure(mcfg, cloudcfg); err != nil {
		return "", err
	}

	// TODO(axw): 2013-08-23 bug 1215777
	// Carry out configuration for ssh-keys-per-user,
	// machine-updates-authkeys, using cloud-init config.
	//
	// We should work with smoser to get a supported
	// command in (or next to) cloud-init for manually
	// invoking cloud-config. This would address the
	// above comment by removing the need to generate a
	// script "by hand".

	// Bootcmds must be run before anything else,
	// as they may affect package installation.
	bootcmds, err := cmdlist(cloudcfg.BootCmds())
	if err != nil {
		return "", err
	}

	// Add package sources and packages.
	pkgcmds, err := addPackageCommands(cloudcfg)
	if err != nil {
		return "", err
	}

	// Runcmds come last.
	runcmds, err := cmdlist(cloudcfg.RunCmds())
	if err != nil {
		return "", err
	}

	// We prepend "set -xe". This is already in runcmds,
	// but added here to avoid relying on that to be
	// invariant.
	script := []string{"#!/bin/bash", "set -xe"}
	script = append(script, bootcmds...)
	script = append(script, pkgcmds...)
	script = append(script, runcmds...)
	return strings.Join(script, "\n"), nil
}

// addPackageCommands returns a slice of commands that, when run,
// will add the required apt repositories and packages.
func addPackageCommands(cfg *corecloudinit.Config) ([]string, error) {
	var cmds []string
	if len(cfg.AptSources()) > 0 {
		// Ensure apt-add-repository is available.
		cmds = append(cmds, "apt-get -y install python-software-properties")
	}
	for _, src := range cfg.AptSources() {
		// PPA keys are obtained by apt-add-repository, from launchpad.
		if !strings.HasPrefix(src.Source, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, "apt-add-repository -y "+utils.ShQuote(src.Source))
	}
	if cfg.AptUpdate() {
		cmds = append(cmds, "apt-get -y update")
	}
	// Note: explicitly ignoring apt_upgrade, so as not to trample the target
	// machine's existing configuration.
	for _, pkg := range cfg.Packages() {
		cmd := fmt.Sprintf("apt-get -y install %s", utils.ShQuote(pkg))
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

func cmdlist(cmds []interface{}) ([]string, error) {
	result := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		switch cmd := cmd.(type) {
		case []string:
			// Quote args, so shell meta-characters are not interpreted.
			for i, arg := range cmd[1:] {
				cmd[i] = utils.ShQuote(arg)
			}
			result = append(result, strings.Join(cmd, " "))
		case string:
			result = append(result, cmd)
		default:
			return nil, fmt.Errorf("unexpected command type: %T", cmd)
		}
	}
	return result, nil
}
