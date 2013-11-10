// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/tomb"

	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/cloudinit"
	"github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/state/api/params"
	apiprovisioner "github.com/jameinel/juju/state/api/provisioner"
	"github.com/jameinel/juju/state/watcher"
	coretools "github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/utils"
	"github.com/jameinel/juju/worker"
)

type ProvisionerTask interface {
	worker.Worker
	Stop() error
	Dying() <-chan struct{}
	Err() error
}

type Watcher interface {
	watcher.Errer
	watcher.Stopper
	Changes() <-chan []string
}

type MachineGetter interface {
	Machine(tag string) (*apiprovisioner.Machine, error)
}

func NewProvisionerTask(
	machineTag string,
	machineGetter MachineGetter,
	watcher Watcher,
	broker environs.InstanceBroker,
	auth AuthenticationProvider,
) ProvisionerTask {
	task := &provisionerTask{
		machineTag:     machineTag,
		machineGetter:  machineGetter,
		machineWatcher: watcher,
		broker:         broker,
		auth:           auth,
		machines:       make(map[string]*apiprovisioner.Machine),
	}
	go func() {
		defer task.tomb.Done()
		task.tomb.Kill(task.loop())
	}()
	return task
}

type provisionerTask struct {
	machineTag     string
	machineGetter  MachineGetter
	machineWatcher Watcher
	broker         environs.InstanceBroker
	tomb           tomb.Tomb
	auth           AuthenticationProvider

	// instance id -> instance
	instances map[instance.Id]instance.Instance
	// machine id -> machine
	machines map[string]*apiprovisioner.Machine
}

// Kill implements worker.Worker.Kill.
func (task *provisionerTask) Kill() {
	task.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (task *provisionerTask) Wait() error {
	return task.tomb.Wait()
}

func (task *provisionerTask) Stop() error {
	task.Kill()
	return task.Wait()
}

func (task *provisionerTask) Dying() <-chan struct{} {
	return task.tomb.Dying()
}

func (task *provisionerTask) Err() error {
	return task.tomb.Err()
}

func (task *provisionerTask) loop() error {
	logger.Infof("Starting up provisioner task %s", task.machineTag)
	defer watcher.Stop(task.machineWatcher, &task.tomb)

	// When the watcher is started, it will have the initial changes be all
	// the machines that are relevant. Also, since this is available straight
	// away, we know there will be some changes right off the bat.
	for {
		select {
		case <-task.tomb.Dying():
			logger.Infof("Shutting down provisioner task %s", task.machineTag)
			return tomb.ErrDying
		case ids, ok := <-task.machineWatcher.Changes():
			if !ok {
				return watcher.MustErr(task.machineWatcher)
			}
			// TODO(dfc; lp:1042717) fire process machines periodically to shut down unknown
			// instances.
			if err := task.processMachines(ids); err != nil {
				logger.Errorf("Process machines failed: %v", err)
				return err
			}
		}
	}
}

func (task *provisionerTask) processMachines(ids []string) error {
	logger.Tracef("processMachines(%v)", ids)
	// Populate the tasks maps of current instances and machines.
	err := task.populateMachineMaps(ids)
	if err != nil {
		return err
	}

	// Find machines without an instance id or that are dead
	pending, dead, err := task.pendingOrDead(ids)
	if err != nil {
		return err
	}

	// Stop all machines that are dead
	stopping := task.instancesForMachines(dead)

	// Find running instances that have no machines associated
	unknown, err := task.findUnknownInstances(stopping)
	if err != nil {
		return err
	}

	// It's important that we stop unknown instances before starting
	// pending ones, because if we start an instance and then fail to
	// set its InstanceId on the machine we don't want to start a new
	// instance for the same machine ID.
	if err := task.stopInstances(append(stopping, unknown...)); err != nil {
		return err
	}

	// Start an instance for the pending ones
	return task.startMachines(pending)
}

func (task *provisionerTask) populateMachineMaps(ids []string) error {
	task.instances = make(map[instance.Id]instance.Instance)

	instances, err := task.broker.AllInstances()
	if err != nil {
		logger.Errorf("failed to get all instances from broker: %v", err)
		return err
	}
	for _, i := range instances {
		task.instances[i.Id()] = i
	}

	// Update the machines map with new data for each of the machines in the
	// change list.
	// TODO(thumper): update for API server later to get all machines in one go.
	for _, id := range ids {
		machineTag := names.MachineTag(id)
		machine, err := task.machineGetter.Machine(machineTag)
		switch {
		case params.IsCodeNotFound(err):
			logger.Debugf("machine %q not found in state", id)
			delete(task.machines, id)
		case err == nil:
			task.machines[id] = machine
		default:
			logger.Errorf("failed to get machine: %v", err)
		}
	}
	return nil
}

// pendingOrDead looks up machines with ids and retuns those that do not
// have an instance id assigned yet, and also those that are dead.
func (task *provisionerTask) pendingOrDead(ids []string) (pending, dead []*apiprovisioner.Machine, err error) {
	for _, id := range ids {
		machine, found := task.machines[id]
		if !found {
			logger.Infof("machine %q not found", id)
			continue
		}
		switch machine.Life() {
		case params.Dying:
			if _, err := machine.InstanceId(); err == nil {
				continue
			} else if !params.IsCodeNotProvisioned(err) {
				logger.Errorf("failed to load machine %q instance id: %v", machine, err)
				return nil, nil, err
			}
			logger.Infof("killing dying, unprovisioned machine %q", machine)
			if err := machine.EnsureDead(); err != nil {
				logger.Errorf("failed to ensure machine dead %q: %v", machine, err)
				return nil, nil, err
			}
			fallthrough
		case params.Dead:
			dead = append(dead, machine)
			logger.Infof("removing dead machine %q", machine)
			if err := machine.Remove(); err != nil {
				logger.Errorf("failed to remove dead machine %q", machine)
				return nil, nil, err
			}
			// now remove it from the machines map
			delete(task.machines, machine.Id())
			continue
		}
		if instId, err := machine.InstanceId(); err != nil {
			if !params.IsCodeNotProvisioned(err) {
				logger.Errorf("failed to load machine %q instance id: %v", machine, err)
				continue
			}
			status, _, err := machine.Status()
			if err != nil {
				logger.Infof("cannot get machine %q status: %v", machine, err)
				continue
			}
			if status == params.StatusPending {
				pending = append(pending, machine)
				logger.Infof("found machine %q pending provisioning", machine)
				continue
			}
		} else {
			logger.Infof("machine %v already started as instance %q", machine, instId)
		}
	}
	logger.Tracef("pending machines: %v", pending)
	logger.Tracef("dead machines: %v", dead)
	return
}

// findUnknownInstances finds instances which are not associated with a machine.
func (task *provisionerTask) findUnknownInstances(stopping []instance.Instance) ([]instance.Instance, error) {
	// Make a copy of the instances we know about.
	instances := make(map[instance.Id]instance.Instance)
	for k, v := range task.instances {
		instances[k] = v
	}

	for _, m := range task.machines {
		if instId, err := m.InstanceId(); err == nil {
			delete(instances, instId)
		} else if !params.IsCodeNotProvisioned(err) {
			return nil, err
		}
	}
	// Now remove all those instances that we are stopping already, as they
	// have been removed from the task.machines map.
	for _, i := range stopping {
		delete(instances, i.Id())
	}
	var unknown []instance.Instance
	for _, i := range instances {
		unknown = append(unknown, i)
	}
	logger.Tracef("unknown: %v", unknown)
	return unknown, nil
}

// instancesForMachines returns a list of instance.Instance that represent
// the list of machines running in the provider. Missing machines are
// omitted from the list.
func (task *provisionerTask) instancesForMachines(machines []*apiprovisioner.Machine) []instance.Instance {
	var instances []instance.Instance
	for _, machine := range machines {
		instId, err := machine.InstanceId()
		if err == nil {
			instance, found := task.instances[instId]
			// If the instance is not found we can't stop it.
			if found {
				instances = append(instances, instance)
			}
		}
	}
	return instances
}

func (task *provisionerTask) stopInstances(instances []instance.Instance) error {
	// Although calling StopInstance with an empty slice should produce no change in the
	// provider, environs like dummy do not consider this a noop.
	if len(instances) == 0 {
		return nil
	}
	logger.Debugf("Stopping instances: %v", instances)
	if err := task.broker.StopInstances(instances); err != nil {
		logger.Errorf("broker failed to stop instances: %v", err)
		return err
	}
	return nil
}

func (task *provisionerTask) startMachines(machines []*apiprovisioner.Machine) error {
	for _, m := range machines {
		if err := task.startMachine(m); err != nil {
			return fmt.Errorf("cannot start machine %v: %v", m, err)
		}
	}
	return nil
}

func (task *provisionerTask) startMachine(machine *apiprovisioner.Machine) error {
	cons, err := machine.Constraints()
	if err != nil {
		return err
	}
	series, err := machine.Series()
	if err != nil {
		return err
	}
	possibleTools, err := task.possibleTools(series, cons)
	if err != nil {
		return err
	}
	machineConfig, err := task.machineConfig(machine)
	if err != nil {
		return err
	}
	inst, metadata, err := task.broker.StartInstance(cons, possibleTools, machineConfig)
	if err != nil {
		// Set the state to error, so the machine will be skipped next
		// time until the error is resolved, but don't return an
		// error; just keep going with the other machines.
		logger.Errorf("cannot start instance for machine %q: %v", machine, err)
		if err1 := machine.SetStatus(params.StatusError, err.Error()); err1 != nil {
			// Something is wrong with this machine, better report it back.
			logger.Errorf("cannot set error status for machine %q: %v", machine, err1)
			return err1
		}
		return nil
	}
	nonce := machineConfig.MachineNonce
	if err := machine.SetProvisioned(inst.Id(), nonce, metadata); err != nil {
		logger.Errorf("cannot register instance for machine %v: %v", machine, err)
		// The machine is started, but we can't record the mapping in
		// state. It'll keep running while we fail out and restart,
		// but will then be detected by findUnknownInstances and
		// killed again.
		//
		// TODO(dimitern) Stop the instance right away here.
		//
		// Multiple instantiations of a given machine (with the same
		// machine ID) cannot coexist, because findUnknownInstances is
		// called before startMachines. However, if the first machine
		// had started to do work before being replaced, we may
		// encounter surprising problems.
		return err
	}
	logger.Infof("started machine %s as instance %s with hardware %q", machine, inst.Id(), metadata)
	return nil
}

func (task *provisionerTask) possibleTools(series string, cons constraints.Value) (coretools.List, error) {
	if env, ok := task.broker.(environs.Environ); ok {
		agentVersion, ok := env.Config().AgentVersion()
		if !ok {
			return nil, fmt.Errorf("no agent version set in environment configuration")
		}
		return tools.FindInstanceTools(env, agentVersion, series, cons.Arch)
	}
	if hasTools, ok := task.broker.(coretools.HasTools); ok {
		return hasTools.Tools(), nil
	}
	panic(fmt.Errorf("broker of type %T does not provide any tools", task.broker))
}

func (task *provisionerTask) machineConfig(machine *apiprovisioner.Machine) (*cloudinit.MachineConfig, error) {
	stateInfo, apiInfo, err := task.auth.SetupAuthentication(machine)
	if err != nil {
		logger.Errorf("failed to setup authentication: %v", err)
		return nil, err
	}
	// Generated a nonce for the new instance, with the format: "machine-#:UUID".
	// The first part is a badge, specifying the tag of the machine the provisioner
	// is running on, while the second part is a random UUID.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	nonce := fmt.Sprintf("%s:%s", task.machineTag, uuid.String())
	machineConfig := environs.NewMachineConfig(machine.Id(), nonce, stateInfo, apiInfo)
	return machineConfig, nil
}
