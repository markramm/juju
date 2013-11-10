// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/jameinel/juju/environs/config"
)

func (o *configObserver) SetObserver(observer chan<- *config.Config) {
	o.Lock()
	o.observer = observer
	o.Unlock()
}
