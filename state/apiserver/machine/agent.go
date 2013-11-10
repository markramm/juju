// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api/params"
	"github.com/jameinel/juju/state/apiserver/common"
)

// DEPRECATED(v1.14)
type AgentAPI struct {
	*common.PasswordChanger

	st   *state.State
	auth common.Authorizer
}

// NewAgentAPI returns an object implementing the machine agent API
// with the given authorizer representing the currently logged in client.
// DEPRECATED(v1.14)
func NewAgentAPI(st *state.State, auth common.Authorizer) (*AgentAPI, error) {
	if !auth.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanChange := func() (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}
	return &AgentAPI{
		PasswordChanger: common.NewPasswordChanger(st, getCanChange),
		st:              st,
		auth:            auth,
	}, nil
}

func (api *AgentAPI) GetMachines(args params.Entities) params.MachineAgentGetMachinesResults {
	results := params.MachineAgentGetMachinesResults{
		Machines: make([]params.MachineAgentGetMachinesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result, err := api.getMachine(entity.Tag)
		result.Error = common.ServerError(err)
		results.Machines[i] = result
	}
	return results
}

func (api *AgentAPI) getMachine(tag string) (result params.MachineAgentGetMachinesResult, err error) {
	// Allow only for the owner agent.
	// Note: having a bulk API call for this is utter madness, given that
	// this check means we can only ever return a single object.
	if !api.auth.AuthOwner(tag) {
		err = common.ErrPerm
		return
	}
	_, id, err := names.ParseTag(tag, names.MachineTagKind)
	if err != nil {
		return
	}
	machine, err := api.st.Machine(id)
	if err != nil {
		return
	}
	result.Life = params.Life(machine.Life().String())
	result.Jobs = stateJobsToAPIParamsJobs(machine.Jobs())
	return
}

func stateJobsToAPIParamsJobs(jobs []state.MachineJob) []params.MachineJob {
	pjobs := make([]params.MachineJob, len(jobs))
	for i, job := range jobs {
		pjobs[i] = params.MachineJob(job.String())
	}
	return pjobs
}
