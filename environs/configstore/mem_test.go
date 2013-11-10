// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/configstore"
)

var _ = gc.Suite(&memInterfaceSuite{})

type memInterfaceSuite struct {
	interfaceSuite
}

func (s *memInterfaceSuite) SetUpSuite(c *gc.C) {
	s.NewStore = func(c *gc.C) configstore.Storage {
		return configstore.NewMem()
	}
}
