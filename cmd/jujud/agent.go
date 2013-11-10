// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"time"

	"launchpad.net/gnuflag"

	"github.com/jameinel/juju/agent"
	"github.com/jameinel/juju/cmd"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/log"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	apiagent "github.com/jameinel/juju/state/api/agent"
	apideployer "github.com/jameinel/juju/state/api/deployer"
	"github.com/jameinel/juju/state/api/params"
	"github.com/jameinel/juju/version"
	"github.com/jameinel/juju/worker"
	"github.com/jameinel/juju/worker/deployer"
	"github.com/jameinel/juju/worker/upgrader"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	dataDir string
	config  agent.Config
}

// addFlags injects common agent flags into f.
func (c *AgentConf) addFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.dataDir, "data-dir", "/var/lib/juju", "directory for juju data")
}

func (c *AgentConf) checkArgs(args []string) error {
	if c.dataDir == "" {
		return requiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

func (c *AgentConf) read(tag string) (err error) {
	c.config, err = agent.ReadConf(c.dataDir, tag)
	return
}

func importance(err error) int {
	switch {
	case err == nil:
		return 0
	default:
		return 1
	case isUpgraded(err):
		return 2
	case err == worker.ErrTerminateAgent:
		return 3
	}
}

// moreImportant returns whether err0 is
// more important than err1 - that is, whether
// we should act on err0 in preference to err1.
func moreImportant(err0, err1 error) bool {
	return importance(err0) > importance(err1)
}

func isUpgraded(err error) bool {
	_, ok := err.(*upgrader.UpgradeReadyError)
	return ok
}

type Agent interface {
	Entity(st *state.State) (AgentState, error)
	Tag() string
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentVersion sets the tools version that the agent is
	// currently running.
	SetAgentVersion(v version.Binary) error
	Tag() string
	SetMongoPassword(password string) error
	Life() state.Life
}

type fatalError struct {
	Err string
}

func (e *fatalError) Error() string {
	return e.Err
}

func isFatal(err error) bool {
	isTerminate := err == worker.ErrTerminateAgent
	notProvisioned := params.IsCodeNotProvisioned(err)
	if isTerminate || notProvisioned || isUpgraded(err) {
		return true
	}
	_, ok := err.(*fatalError)
	return ok
}

type pinger interface {
	Ping() error
}

// connectionIsFatal returns a function suitable for passing
// as the isFatal argument to worker.NewRunner,
// that diagnoses an error as fatal if the connection
// has failed or if the error is otherwise fatal.
func connectionIsFatal(conn pinger) func(err error) bool {
	return func(err error) bool {
		if isFatal(err) {
			return true
		}
		if err := conn.Ping(); err != nil {
			log.Infof("error pinging %T: %v", conn, err)
			return true
		}
		return false
	}
}

// isleep waits for the given duration or until it receives a value on
// stop.  It returns whether the full duration was slept without being
// stopped.
func isleep(d time.Duration, stop <-chan struct{}) bool {
	select {
	case <-stop:
		return false
	case <-time.After(d):
	}
	return true
}

func openState(agentConfig agent.Config, a Agent) (*state.State, AgentState, error) {
	st, err := agentConfig.OpenState()
	if err != nil {
		return nil, nil, err
	}
	entity, err := a.Entity(st)
	if errors.IsNotFoundError(err) || err == nil && entity.Life() == state.Dead {
		err = worker.ErrTerminateAgent
	}
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, entity, nil
}

func openAPIState(agentConfig agent.Config, a Agent) (*api.State, *apiagent.Entity, error) {
	// We let the API dial fail immediately because the
	// runner's loop outside the caller of openAPIState will
	// keep on retrying. If we block for ages here,
	// then the worker that's calling this cannot
	// be interrupted.
	st, newPassword, err := agentConfig.OpenAPI(api.DialOpts{})
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			err = worker.ErrTerminateAgent
		}
		return nil, nil, err
	}
	entity, err := st.Agent().Entity(a.Tag())
	unauthorized := params.IsCodeUnauthorized(err)
	dead := err == nil && entity.Life() == params.Dead
	if unauthorized || dead {
		err = worker.ErrTerminateAgent
	}
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	if newPassword != "" {
		if err := entity.SetPassword(newPassword); err != nil {
			return nil, nil, err
		}
	}
	return st, entity, nil
}

// agentDone processes the error returned by
// an exiting agent.
func agentDone(err error) error {
	if err == worker.ErrTerminateAgent {
		err = nil
	}
	if ug, ok := err.(*upgrader.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let upstart deal with the restart.
			return err
		}
	}
	return err
}

type closeWorker struct {
	worker worker.Worker
	closer io.Closer
}

// newCloseWorker returns a task that wraps the given task,
// closing the given closer when it finishes.
func newCloseWorker(worker worker.Worker, closer io.Closer) worker.Worker {
	return &closeWorker{
		worker: worker,
		closer: closer,
	}
}

func (c *closeWorker) Kill() {
	c.worker.Kill()
}

func (c *closeWorker) Wait() error {
	err := c.worker.Wait()
	if err := c.closer.Close(); err != nil {
		log.Errorf("closeWorker: close error: %v", err)
	}
	return err
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, agentConfig agent.Config) deployer.Context {
	return deployer.NewSimpleContext(agentConfig, st)
}
