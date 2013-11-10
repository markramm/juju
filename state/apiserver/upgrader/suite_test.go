// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	coretesting "github.com/jameinel/juju/testing"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
