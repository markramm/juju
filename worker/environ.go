// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	apiwatcher "github.com/jameinel/juju/state/api/watcher"
	"github.com/jameinel/juju/state/watcher"
)

var ErrTerminateAgent = errors.New("agent should be terminated")

var loadedInvalid = func() {}

var logger = loggo.GetLogger("juju.worker")

// EnvironConfigGetter interface defines a way to read the environment
// configuration.
type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on dying.
func WaitForEnviron(w apiwatcher.NotifyWatcher, st EnvironConfigGetter, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return nil, watcher.MustErr(w)
			}
			config, err := st.EnvironConfig()
			if err != nil {
				return nil, err
			}
			environ, err := environs.New(config)
			if err == nil {
				return environ, nil
			}
			logger.Errorf("loaded invalid environment configuration: %v", err)
			loadedInvalid()
		}
	}
}
