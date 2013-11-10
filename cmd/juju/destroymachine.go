// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/juju"
	"github.com/jameinel/juju/names"
)

// DestroyMachineCommand causes an existing machine to be destroyed.
type DestroyMachineCommand struct {
	cmd.EnvCommandBase
	MachineIds []string
}

func (c *DestroyMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-machine",
		Args:    "<machine> ...",
		Purpose: "destroy machines",
		Doc:     "Machines that have assigned units, or are responsible for the environment, cannot be destroyed.",
		Aliases: []string{"terminate-machine"},
	}
}

func (c *DestroyMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsMachine(id) {
			return fmt.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

func (c *DestroyMachineCommand) Run(_ *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	return apiclient.DestroyMachines(c.MachineIds...)
}
