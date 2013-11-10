// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/version"
)

// SetAgentVersion sets the current agent version in the state's
// environment configuration.
func SetAgentVersion(st *state.State, vers version.Number) error {
	return UpdateConfig(st, map[string]interface{}{"agent-version": vers.String()})
}
