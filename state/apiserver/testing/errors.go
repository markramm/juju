// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/jameinel/juju/state/api/params"
)

var ErrUnauthorized = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}

func NotFoundError(prefixMessage string) *params.Error {
	return &params.Error{
		Message: prefixMessage + " not found",
		Code:    params.CodeNotFound,
	}
}

func NotProvisionedError(machineId string) *params.Error {
	return &params.Error{
		Message: "machine " + machineId + " is not provisioned",
		Code:    params.CodeNotProvisioned,
	}
}
