// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/jameinel/juju/state"
)

// UpdateConfig sets the current agent version in the state's
// environment configuration.
func UpdateConfig(st *state.State, newValues map[string]interface{}) error {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	cfg, err = cfg.Apply(newValues)
	if err != nil {
		return err
	}
	return st.SetEnvironConfig(cfg)
}
