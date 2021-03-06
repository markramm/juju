// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/state/api/agent"
	"launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/logger"
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/api/uniter"
	"launchpad.net/juju-core/state/api/upgrader"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.  This
// method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *State) Login(tag, password, nonce string) error {
	err := st.Call("Admin", "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
		Nonce:    nonce,
	}, nil)
	if err == nil {
		st.authTag = tag
	}
	return err
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *State) Client() *Client {
	return &Client{st}
}

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func (st *State) Machiner() *machiner.State {
	return machiner.NewState(st)
}

// Provisioner returns a version of the state that provides functionality
// required by the provisioner worker.
func (st *State) Provisioner() *provisioner.State {
	return provisioner.NewState(st)
}

// Uniter returns a version of the state that provides functionality
// required by the uniter worker.
func (st *State) Uniter() *uniter.State {
	return uniter.NewState(st, st.authTag)
}

// Agent returns a version of the state that provides
// functionality required by the agent code.
func (st *State) Agent() *agent.State {
	return agent.NewState(st)
}

// Upgrader returns access to the Upgrader API
func (st *State) Upgrader() *upgrader.State {
	return upgrader.NewState(st)
}

// Deployer returns access to the Deployer API
func (st *State) Deployer() *deployer.State {
	return deployer.NewState(st)
}

// Logger returns access to the Logger API
func (st *State) Logger() *logger.State {
	return logger.NewState(st)
}
