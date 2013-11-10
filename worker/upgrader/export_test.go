// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/jameinel/juju/tools"
)

var RetryAfter = &retryAfter

func EnsureTools(u *Upgrader, agentTools *tools.Tools, disableSSLHostnameVerification bool) error {
	return u.ensureTools(agentTools, disableSSLHostnameVerification)
}
