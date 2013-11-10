// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/config"
	envtools "github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api/params"
	coretools "github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/version"
)

type EntityFinderEnvironConfigGetter interface {
	state.EntityFinder
	EnvironConfig() (*config.Config, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	st         EntityFinderEnvironConfigGetter
	getCanRead GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(st EntityFinderEnvironConfigGetter, getCanRead GetAuthFunc) *ToolsGetter {
	return &ToolsGetter{
		st:         st,
		getCanRead: getCanRead,
	}
}

// Tools finds the tools necessary for the given agents.
func (t *ToolsGetter) Tools(args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	canRead, err := t.getCanRead()
	if err != nil {
		return result, err
	}
	agentVersion, cfg, err := t.getGlobalAgentVersion()
	if err != nil {
		return result, err
	}
	// SSLHostnameVerification defaults to true, so we need to
	// invert that, for backwards-compatibility (older versions
	// will have DisableSSLHostnameVerification: false by default).
	disableSSLHostnameVerification := !cfg.SSLHostnameVerification()
	env, err := environs.New(cfg)
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		agentTools, err := t.oneAgentTools(canRead, entity.Tag, agentVersion, env)
		if err == nil {
			result.Results[i].Tools = agentTools
			result.Results[i].DisableSSLHostnameVerification = disableSSLHostnameVerification
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) getGlobalAgentVersion() (version.Number, *config.Config, error) {
	// Get the Agent Version requested in the Environment Config
	nothing := version.Number{}
	cfg, err := t.st.EnvironConfig()
	if err != nil {
		return nothing, nil, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return nothing, nil, fmt.Errorf("agent version not set in environment config")
	}
	return agentVersion, cfg, nil
}

func (t *ToolsGetter) oneAgentTools(canRead AuthFunc, tag string, agentVersion version.Number, env environs.Environ) (*coretools.Tools, error) {
	if !canRead(tag) {
		return nil, ErrPerm
	}
	entity, err := t.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	tooler, ok := entity.(state.AgentTooler)
	if !ok {
		return nil, NotSupportedError(tag, "agent tools")
	}
	existingTools, err := tooler.AgentTools()
	if err != nil {
		return nil, err
	}
	// TODO(jam): Avoid searching the provider for every machine
	// that wants to upgrade. The information could just be cached
	// in state, or even in the API servers
	return envtools.FindExactTools(env, agentVersion, existingTools.Version.Series, existingTools.Version.Arch)
}
