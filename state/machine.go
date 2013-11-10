// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/state/api/params"
	"github.com/jameinel/juju/state/presence"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/utils"
	"github.com/jameinel/juju/version"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
	annotator
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob int

const (
	_ MachineJob = iota
	JobHostUnits
	JobManageEnviron
	JobManageState
)

var jobNames = map[MachineJob]params.MachineJob{
	JobHostUnits:     params.JobHostUnits,
	JobManageEnviron: params.JobManageEnviron,
	JobManageState:   params.JobManageState,
}

// AllJobs returns all supported machine jobs.
func AllJobs() []MachineJob {
	return []MachineJob{JobHostUnits, JobManageState, JobManageEnviron}
}

// ToParams returns the job as params.MachineJob.
func (job MachineJob) ToParams() params.MachineJob {
	if paramsJob, ok := jobNames[job]; ok {
		return paramsJob
	}
	return params.MachineJob(fmt.Sprintf("<unknown job %d>", int(job)))
}

// MachineJobFromParams returns the job corresponding to params.MachineJob.
func MachineJobFromParams(job params.MachineJob) (MachineJob, error) {
	for machineJob, paramJob := range jobNames {
		if paramJob == job {
			return machineJob, nil
		}
	}
	return -1, fmt.Errorf("invalid machine job %q", job)
}

func (job MachineJob) String() string {
	return string(job.ToParams())
}

// machineDoc represents the internal state of a machine in MongoDB.
// Note the correspondence with MachineInfo in state/api/params.
type machineDoc struct {
	Id            string `bson:"_id"`
	Nonce         string
	Series        string
	ContainerType string
	Principals    []string
	Life          Life
	Tools         *tools.Tools `bson:",omitempty"`
	TxnRevno      int64        `bson:"txn-revno"`
	Jobs          []MachineJob
	PasswordHash  string
	Clean         bool
	Addresses     []address
	// Deprecated. InstanceId, now lives on instanceData.
	// This attribute is retained so that data from existing machines can be read.
	// SCHEMACHANGE
	// TODO(wallyworld): remove this attribute when schema upgrades are possible.
	InstanceId instance.Id
}

func newMachine(st *State, doc *machineDoc) *Machine {
	machine := &Machine{
		st:  st,
		doc: *doc,
	}
	machine.annotator = annotator{
		globalKey: machine.globalKey(),
		tag:       machine.Tag(),
		st:        st,
	}
	return machine
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// Series returns the operating system series running on the machine.
func (m *Machine) Series() string {
	return m.doc.Series
}

// ContainerType returns the type of container hosting this machine.
func (m *Machine) ContainerType() instance.ContainerType {
	return instance.ContainerType(m.doc.ContainerType)
}

// machineGlobalKey returns the global database key for the identified machine.
func machineGlobalKey(id string) string {
	return "m#" + id
}

// globalKey returns the global database key for the machine.
func (m *Machine) globalKey() string {
	return machineGlobalKey(m.doc.Id)
}

// instanceData holds attributes relevant to a provisioned machine.
type instanceData struct {
	Id         string      `bson:"_id"`
	InstanceId instance.Id `bson:"instanceid"`
	Arch       *string     `bson:"arch,omitempty"`
	Mem        *uint64     `bson:"mem,omitempty"`
	RootDisk   *uint64     `bson:"rootdisk,omitempty"`
	CpuCores   *uint64     `bson:"cpucores,omitempty"`
	CpuPower   *uint64     `bson:"cpupower,omitempty"`
	Tags       *[]string   `bson:"tags,omitempty"`
	TxnRevno   int64       `bson:"txn-revno"`
}

// TODO(wallyworld): move this method to a service.
func (m *Machine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	hc := &instance.HardwareCharacteristics{}
	instData, err := getInstanceData(m.st, m.Id())
	if err != nil {
		return nil, err
	}
	hc.Arch = instData.Arch
	hc.Mem = instData.Mem
	hc.RootDisk = instData.RootDisk
	hc.CpuCores = instData.CpuCores
	hc.CpuPower = instData.CpuPower
	hc.Tags = instData.Tags
	return hc, nil
}

func getInstanceData(st *State, id string) (instanceData, error) {
	var instData instanceData
	err := st.instanceData.FindId(id).One(&instData)
	if err == mgo.ErrNotFound {
		return instanceData{}, errors.NotFoundf("instance data for machine %v", id)
	}
	if err != nil {
		return instanceData{}, fmt.Errorf("cannot get instance data for machine %v: %v", id, err)
	}
	return instData, nil
}

// Tag returns a name identifying the machine that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (m *Machine) Tag() string {
	return names.MachineTag(m.Id())
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// Jobs returns the responsibilities that must be fulfilled by m's agent.
func (m *Machine) Jobs() []MachineJob {
	return m.doc.Jobs
}

// IsStateServer returns true if the machine has a JobManageState job.
func (m *Machine) IsStateServer() bool {
	for _, job := range m.doc.Jobs {
		if job == JobManageState {
			return true
		}
	}
	return false
}

// AgentTools returns the tools that the agent is currently running.
// It returns an error that satisfies IsNotFound if the tools have not yet been set.
func (m *Machine) AgentTools() (*tools.Tools, error) {
	if m.doc.Tools == nil {
		return nil, errors.NotFoundf("agent tools for machine %v", m)
	}
	tools := *m.doc.Tools
	return &tools, nil
}

// checkVersionValidity checks whether the given version is suitable
// for passing to SetAgentVersion.
func checkVersionValidity(v version.Binary) error {
	if v.Series == "" || v.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	return nil
}

// SetAgentVersion sets the version of juju that the agent is
// currently running.
func (m *Machine) SetAgentVersion(v version.Binary) (err error) {
	defer utils.ErrorContextf(&err, "cannot set agent version for machine %v", m)
	if err = checkVersionValidity(v); err != nil {
		return err
	}
	tools := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"tools", tools}}}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return onAbort(err, errDead)
	}
	m.doc.Tools = tools
	return nil
}

// SetMongoPassword sets the password the agent responsible for the machine
// should use to communicate with the state servers.  Previous passwords
// are invalidated.
func (m *Machine) SetMongoPassword(password string) error {
	return m.st.setMongoPassword(m.Tag(), password)
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	if len(password) < utils.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	return m.setPasswordHash(utils.AgentPasswordHash(password))
}

// setPasswordHash sets the underlying password hash in the database directly
// to the value supplied. This is split out from SetPassword to allow direct
// manipulation in tests (to check for backwards compatibility).
func (m *Machine) setPasswordHash(passwordHash string) error {
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
		Update: D{{"$set", D{{"passwordhash", passwordHash}}}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set password of machine %v: %v", m, onAbort(err, errDead))
	}
	m.doc.PasswordHash = passwordHash
	return nil
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func (m *Machine) getPasswordHash() string {
	return m.doc.PasswordHash
}

// PasswordValid returns whether the given password is valid
// for the given machine.
func (m *Machine) PasswordValid(password string) bool {
	agentHash := utils.AgentPasswordHash(password)
	if agentHash == m.doc.PasswordHash {
		return true
	}
	// In Juju 1.16 and older we used the slower password hash for unit
	// agents. So check to see if the supplied password matches the old
	// path, and if so, update it to the new mechanism.
	// We ignore any error in setting the password, as we'll just try again
	// next time
	if utils.UserPasswordHash(password, utils.CompatSalt) == m.doc.PasswordHash {
		logger.Debugf("%s logged in with old password hash, changing to AgentPasswordHash",
			m.Tag())
		m.setPasswordHash(agentHash)
		return true
	}
	return false
}

// Destroy sets the machine lifecycle to Dying if it is Alive. It does
// nothing otherwise. Destroy will fail if the machine has principal
// units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, Destroy will return
// a HasAssignedUnitsError.
func (m *Machine) Destroy() error {
	return m.advanceLifecycle(Dying)
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, EnsureDead will return
// a HasAssignedUnitsError.
func (m *Machine) EnsureDead() error {
	return m.advanceLifecycle(Dead)
}

type HasAssignedUnitsError struct {
	MachineId string
	UnitNames []string
}

func (e *HasAssignedUnitsError) Error() string {
	return fmt.Sprintf("machine %s has unit %q assigned", e.MachineId, e.UnitNames[0])
}

func IsHasAssignedUnitsError(err error) bool {
	_, ok := err.(*HasAssignedUnitsError)
	return ok
}

// Containers returns the container ids belonging to a parent machine.
// TODO(wallyworld): move this method to a service
func (m *Machine) Containers() ([]string, error) {
	var mc machineContainers
	err := m.st.containerRefs.FindId(m.Id()).One(&mc)
	if err == nil {
		return mc.Children, nil
	}
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("container info for machine %v", m.Id())
	}
	return nil, err
}

// ParentId returns the Id of the host machine if this machine is a container.
func (m *Machine) ParentId() (string, bool) {
	parentId := ParentId(m.Id())
	return parentId, parentId != ""
}

type HasContainersError struct {
	MachineId    string
	ContainerIds []string
}

func (e *HasContainersError) Error() string {
	return fmt.Sprintf("machine %s is hosting containers %q", e.MachineId, strings.Join(e.ContainerIds, ","))
}

func IsHasContainersError(err error) bool {
	_, ok := err.(*HasContainersError)
	return ok
}

// advanceLifecycle ensures that the machine's lifecycle is no earlier
// than the supplied value. If the machine already has that lifecycle
// value, or a later one, no changes will be made to remote state. If
// the machine has any responsibilities that preclude a valid change in
// lifecycle, it will return an error.
func (original *Machine) advanceLifecycle(life Life) (err error) {
	containers, err := original.Containers()
	if err != nil {
		return err
	}
	if len(containers) > 0 {
		return &HasContainersError{
			MachineId:    original.doc.Id,
			ContainerIds: containers,
		}
	}
	m := original
	defer func() {
		if err == nil {
			// The machine's lifecycle is known to have advanced; it may be
			// known to have already advanced further than requested, in
			// which case we set the latest known valid value.
			if m == nil {
				life = Dead
			} else if m.doc.Life > life {
				life = m.doc.Life
			}
			original.doc.Life = life
		}
	}()
	// op and
	op := txn.Op{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Update: D{{"$set", D{{"life", life}}}},
	}
	advanceAsserts := D{
		{"jobs", D{{"$nin", []MachineJob{JobManageEnviron}}}},
		{"$or", []D{
			{{"principals", D{{"$size", 0}}}},
			{{"principals", D{{"$exists", false}}}},
		}},
	}
	// 3 attempts: one with original data, one with refreshed data, and a final
	// one intended to determine the cause of failure of the preceding attempt.
	for i := 0; i < 3; i++ {
		// If the transaction was aborted, grab a fresh copy of the machine data.
		// We don't write to original, because the expectation is that state-
		// changing methods only set the requested change on the receiver; a case
		// could perhaps be made that this is not a helpful convention in the
		// context of the new state API, but we maintain consistency in the
		// face of uncertainty.
		if i != 0 {
			if m, err = m.st.Machine(m.doc.Id); errors.IsNotFoundError(err) {
				return nil
			} else if err != nil {
				return err
			}
		}
		// Check that the life change is sane, and collect the assertions
		// necessary to determine that it remains so.
		switch life {
		case Dying:
			if m.doc.Life != Alive {
				return nil
			}
			op.Assert = append(advanceAsserts, isAliveDoc...)
		case Dead:
			if m.doc.Life == Dead {
				return nil
			}
			op.Assert = append(advanceAsserts, notDeadDoc...)
		default:
			panic(fmt.Errorf("cannot advance lifecycle to %v", life))
		}
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		for _, j := range m.doc.Jobs {
			if j == JobManageEnviron {
				// (NOTE: When we enable multiple JobManageEnviron machines,
				// the restriction will become "there must be at least one
				// machine with this job".)
				return fmt.Errorf("machine %s is required by the environment", m.doc.Id)
			}
		}
		if len(m.doc.Principals) != 0 {
			return &HasAssignedUnitsError{
				MachineId: m.doc.Id,
				UnitNames: m.doc.Principals,
			}
		}
		// Run the transaction...
		if err := m.st.runTransaction([]txn.Op{op}); err != txn.ErrAborted {
			return err
		}
		// ...and retry on abort.
	}
	// In very rare circumstances, the final iteration above will have determined
	// no cause of failure, and attempted a final transaction: if this also failed,
	// we can be sure that the machine document is changing very fast, in a somewhat
	// surprising fashion, and that it is sensible to back off for now.
	return fmt.Errorf("machine %s cannot advance lifecycle: %v", m, ErrExcessiveContention)
}

// Remove removes the machine from state. It will fail if the machine is not
// Dead.
func (m *Machine) Remove() (err error) {
	defer utils.ErrorContextf(&err, "cannot remove machine %s", m.doc.Id)
	if m.doc.Life != Dead {
		return fmt.Errorf("machine is not dead")
	}
	ops := []txn.Op{
		{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: txn.DocExists,
			Remove: true,
		},
		{
			C:      m.st.instanceData.Name,
			Id:     m.doc.Id,
			Remove: true,
		},
		removeStatusOp(m.st, m.globalKey()),
		removeConstraintsOp(m.st, m.globalKey()),
		annotationRemoveOp(m.st, m.globalKey()),
	}
	ops = append(ops, removeContainerRefOps(m.st, m.Id())...)
	// The only abort conditions in play indicate that the machine has already
	// been removed.
	return onAbort(m.st.runTransaction(ops), nil)
}

// Refresh refreshes the contents of the machine from the underlying
// state. It returns an error that satisfies IsNotFound if the machine has
// been removed.
func (m *Machine) Refresh() error {
	doc := machineDoc{}
	err := m.st.machines.FindId(m.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("machine %v", m)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh machine %v: %v", m, err)
	}
	m.doc = doc
	return nil
}

// AgentAlive returns whether the respective remote agent is alive.
func (m *Machine) AgentAlive() (bool, error) {
	return m.st.pwatcher.Alive(m.globalKey())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (m *Machine) WaitAgentAlive(timeout time.Duration) (err error) {
	defer utils.ErrorContextf(&err, "waiting for agent of machine %v", m)
	ch := make(chan presence.Change)
	m.st.pwatcher.Watch(m.globalKey(), ch)
	defer m.st.pwatcher.Unwatch(m.globalKey(), ch)
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("still not alive after timeout")
		case <-m.st.pwatcher.Dead():
			return m.st.pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for machine %v", m))
}

// SetAgentAlive signals that the agent for machine m is alive.
// It returns the started pinger.
func (m *Machine) SetAgentAlive() (*presence.Pinger, error) {
	p := presence.NewPinger(m.st.presence, m.globalKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// InstanceId returns the provider specific instance id for this
// machine, or a NotProvisionedError, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	// SCHEMACHANGE
	// TODO(wallyworld) - remove this backward compatibility code when schema upgrades are possible
	// (we first check for InstanceId stored on the machineDoc)
	if m.doc.InstanceId != "" {
		return m.doc.InstanceId, nil
	}
	instData, err := getInstanceData(m.st, m.Id())
	if (err == nil && instData.InstanceId == "") || (err != nil && errors.IsNotFoundError(err)) {
		err = NotProvisionedError(m.Id())
	}
	if err != nil {
		return "", err
	}
	return instData.InstanceId, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer utils.ErrorContextf(&err, "cannot get units assigned to machine %v", m)
	pudocs := []unitDoc{}
	err = m.st.units.Find(D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, &pudoc))
		docs := []unitDoc{}
		err = m.st.units.Find(D{{"principal", pudoc.Name}}).All(&docs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			units = append(units, newUnit(m.st, &doc))
		}
	}
	return units, nil
}

// SetProvisioned sets the provider specific machine id, nonce and also metadata for
// this machine. Once set, the instance id cannot be changed.
func (m *Machine) SetProvisioned(id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics) (err error) {
	defer utils.ErrorContextf(&err, "cannot set instance data for machine %q", m)

	if id == "" || nonce == "" {
		return fmt.Errorf("instance id and nonce cannot be empty")
	}

	if characteristics == nil {
		characteristics = &instance.HardwareCharacteristics{}
	}
	hc := &instanceData{
		Id:         m.doc.Id,
		InstanceId: id,
		Arch:       characteristics.Arch,
		Mem:        characteristics.Mem,
		RootDisk:   characteristics.RootDisk,
		CpuCores:   characteristics.CpuCores,
		CpuPower:   characteristics.CpuPower,
		Tags:       characteristics.Tags,
	}
	// SCHEMACHANGE
	// TODO(wallyworld) - do not check instanceId on machineDoc after schema is upgraded
	notSetYet := D{{"instanceid", ""}, {"nonce", ""}}
	ops := []txn.Op{
		{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: append(isAliveDoc, notSetYet...),
			Update: D{{"$set", D{{"instanceid", id}, {"nonce", nonce}}}},
		}, {
			C:      m.st.instanceData.Name,
			Id:     m.doc.Id,
			Assert: txn.DocMissing,
			Insert: hc,
		},
	}

	if err = m.st.runTransaction(ops); err == nil {
		m.doc.Nonce = nonce
		// SCHEMACHANGE
		// TODO(wallyworld) - remove this backward compatibility code when schema upgrades are possible
		// (InstanceId is stored on the instanceData document but we duplicate the value on the machineDoc.
		m.doc.InstanceId = id
		return nil
	} else if err != txn.ErrAborted {
		return err
	} else if alive, err := isAlive(m.st.machines, m.doc.Id); err != nil {
		return err
	} else if !alive {
		return errNotAlive
	}
	return fmt.Errorf("already set")
}

// notProvisionedError records an error when a machine is not provisioned.
type notProvisionedError struct {
	machineId string
}

func NotProvisionedError(machineId string) error {
	return &notProvisionedError{machineId}
}

func (e *notProvisionedError) Error() string {
	return fmt.Sprintf("machine %v is not provisioned", e.machineId)
}

// IsNotProvisionedError returns true if err is a notProvisionedError.
func IsNotProvisionedError(err error) bool {
	_, ok := err.(*notProvisionedError)
	return ok
}

// Addresses returns any hostnames and ips associated with a machine
func (m *Machine) Addresses() (addresses []instance.Address) {
	for _, address := range m.doc.Addresses {
		addresses = append(addresses, address.InstanceAddress())
	}
	return
}

// SetAddresses records any addresses related to the machine
func (m *Machine) SetAddresses(addresses []instance.Address) (err error) {
	var stateAddresses []address
	for _, address := range addresses {
		stateAddresses = append(stateAddresses, NewAddress(address))
	}

	ops := []txn.Op{
		{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: notDeadDoc,
			Update: D{{"$set", D{{"addresses", stateAddresses}}}},
		},
	}

	if err = m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set addresses of machine %v: %v", m, onAbort(err, errDead))
	}
	m.doc.Addresses = stateAddresses
	return nil
}

// CheckProvisioned returns true if the machine was provisioned with the given nonce.
func (m *Machine) CheckProvisioned(nonce string) bool {
	return nonce == m.doc.Nonce && nonce != ""
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return m.doc.Id
}

// Constraints returns the exact constraints that should apply when provisioning
// an instance for the machine.
func (m *Machine) Constraints() (constraints.Value, error) {
	return readConstraints(m.st, m.globalKey())
}

// SetConstraints sets the exact constraints to apply when provisioning an
// instance for the machine. It will fail if the machine is Dead, or if it
// is already provisioned.
func (m *Machine) SetConstraints(cons constraints.Value) (err error) {
	defer utils.ErrorContextf(&err, "cannot set constraints")
	notSetYet := D{{"nonce", ""}}
	ops := []txn.Op{
		{
			C:      m.st.machines.Name,
			Id:     m.doc.Id,
			Assert: append(isAliveDoc, notSetYet...),
		},
		setConstraintsOp(m.st, m.globalKey(), cons),
	}
	// 3 attempts is enough to push the ErrExcessiveContention case out of the
	// realm of plausibility: it implies local state indicating unprovisioned,
	// and remote state indicating provisioned (reasonable); but which changes
	// back to unprovisioned and then to provisioned again with *very* specific
	// timing in the course of this loop.
	for i := 0; i < 3; i++ {
		if m.doc.Life != Alive {
			return errNotAlive
		}
		if _, err := m.InstanceId(); err == nil {
			return fmt.Errorf("machine is already provisioned")
		} else if !IsNotProvisionedError(err) {
			return err
		}
		if err := m.st.runTransaction(ops); err != txn.ErrAborted {
			return err
		}
		if m, err = m.st.Machine(m.doc.Id); err != nil {
			return err
		}
	}
	return ErrExcessiveContention
}

// Status returns the status of the machine.
func (m *Machine) Status() (status params.Status, info string, data params.StatusData, err error) {
	doc, err := getStatus(m.st, m.globalKey())
	if err != nil {
		return "", "", nil, err
	}
	status = doc.Status
	info = doc.StatusInfo
	data = doc.StatusData
	return
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string, data params.StatusData) error {
	doc := statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}
	if err := doc.validateSet(); err != nil {
		return err
	}
	ops := []txn.Op{{
		C:      m.st.machines.Name,
		Id:     m.doc.Id,
		Assert: notDeadDoc,
	},
		updateStatusOp(m.st, m.globalKey(), doc),
	}
	if err := m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set status of machine %q: %v", m, onAbort(err, errNotAlive))
	}
	return nil
}

// Clean returns true if the machine does not have any deployed units or containers.
func (m *Machine) Clean() bool {
	return m.doc.Clean
}
