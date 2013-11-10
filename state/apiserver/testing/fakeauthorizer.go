// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/jameinel/juju/state"
)

// FakeAuthorizer implements the common.Authorizer interface.
type FakeAuthorizer struct {
	Tag          string
	LoggedIn     bool
	Manager      bool
	MachineAgent bool
	UnitAgent    bool
	Client       bool
	Entity       state.Entity
}

func (fa FakeAuthorizer) AuthOwner(tag string) bool {
	return fa.Tag == tag
}

func (fa FakeAuthorizer) AuthEnvironManager() bool {
	return fa.Manager
}

func (fa FakeAuthorizer) AuthMachineAgent() bool {
	return fa.MachineAgent
}

func (fa FakeAuthorizer) AuthUnitAgent() bool {
	return fa.UnitAgent
}

func (fa FakeAuthorizer) AuthClient() bool {
	return fa.Client
}

func (fa FakeAuthorizer) GetAuthTag() string {
	return fa.Tag
}

func (fa FakeAuthorizer) GetAuthEntity() state.Entity {
	return fa.Entity
}
