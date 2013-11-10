// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package local

import (
	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/utils"
)

var (
	Provider = providerInstance
)

// SetRootCheckFunction allows tests to override the check for a root user.
// The return value is the function to restore the old value.
func SetRootCheckFunction(f func() bool) func() {
	old := checkIfRoot
	checkIfRoot = f
	return func() { checkIfRoot = old }
}

// SetUpstartScriptLocation allows tests to override the directory where the
// provider writes the upstart scripts.
func SetUpstartScriptLocation(location string) (old string) {
	old, upstartScriptLocation = upstartScriptLocation, location
	return
}

// ConfigNamespace returns the result of the namespace call on the
// localConfig.
func ConfigNamespace(cfg *config.Config) string {
	localConfig, _ := providerInstance.newConfig(cfg)
	return localConfig.namespace()
}

// CreateDirs calls createDirs on the localEnviron.
func CreateDirs(c *gc.C, cfg *config.Config) error {
	localConfig, err := providerInstance.newConfig(cfg)
	c.Assert(err, gc.IsNil)
	return localConfig.createDirs()
}

// CheckDirs returns the list of directories to check for permissions in the test.
func CheckDirs(c *gc.C, cfg *config.Config) []string {
	localConfig, err := providerInstance.newConfig(cfg)
	c.Assert(err, gc.IsNil)
	return []string{
		localConfig.rootDir(),
		localConfig.sharedStorageDir(),
		localConfig.storageDir(),
		localConfig.mongoDir(),
	}
}

// MockAddressForInterface replaces the getAddressForInterface with a function
// that returns a constant localhost ip address.
func MockAddressForInterface() func() {
	getAddressForInterface = func(name string) (string, error) {
		logger.Debugf("getAddressForInterface called for %s", name)
		return "127.0.0.1", nil
	}
	return func() {
		getAddressForInterface = utils.GetAddressForInterface
	}
}

var CheckLocalPort = checkLocalPort
