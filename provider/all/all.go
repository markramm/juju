// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

// Register all the available providers.
import (
	_ "github.com/jameinel/juju/provider/azure"
	_ "github.com/jameinel/juju/provider/ec2"
	_ "github.com/jameinel/juju/provider/local"
	_ "github.com/jameinel/juju/provider/maas"
	_ "github.com/jameinel/juju/provider/null"
	_ "github.com/jameinel/juju/provider/openstack"
)
