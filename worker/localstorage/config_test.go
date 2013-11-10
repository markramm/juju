// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"github.com/jameinel/juju/worker/localstorage"
)

type configSuite struct{}

var _ = gc.Suite(&configSuite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type localStorageConfig struct {
	storageDir        string
	storageAddr       string
	sharedStorageDir  string
	sharedStorageAddr string
}

func (c *localStorageConfig) StorageDir() string {
	return c.storageDir
}

func (c *localStorageConfig) StorageAddr() string {
	return c.storageAddr
}

func (c *localStorageConfig) SharedStorageDir() string {
	return c.sharedStorageDir
}

func (c *localStorageConfig) SharedStorageAddr() string {
	return c.sharedStorageAddr
}

type localTLSStorageConfig struct {
	localStorageConfig
	caCertPEM []byte
	caKeyPEM  []byte
	hostnames []string
	authkey   string
}

func (c *localTLSStorageConfig) StorageCACert() []byte {
	return c.caCertPEM
}

func (c *localTLSStorageConfig) StorageCAKey() []byte {
	return c.caKeyPEM
}

func (c *localTLSStorageConfig) StorageHostnames() []string {
	return c.hostnames
}

func (c *localTLSStorageConfig) StorageAuthKey() string {
	return c.authkey
}

func (*configSuite) TestStoreConfig(c *gc.C) {
	var config localStorageConfig
	m, err := localstorage.StoreConfig(&config)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]string{
		localstorage.StorageDir:        "",
		localstorage.StorageAddr:       "",
		localstorage.SharedStorageDir:  "",
		localstorage.SharedStorageAddr: "",
	})

	config.storageDir = "a"
	config.storageAddr = "b"
	config.sharedStorageDir = "c"
	config.sharedStorageAddr = "d"
	m, err = localstorage.StoreConfig(&config)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]string{
		localstorage.StorageDir:        config.storageDir,
		localstorage.StorageAddr:       config.storageAddr,
		localstorage.SharedStorageDir:  config.sharedStorageDir,
		localstorage.SharedStorageAddr: config.sharedStorageAddr,
	})
}

func (*configSuite) TestStoreConfigTLS(c *gc.C) {
	var config localTLSStorageConfig
	m, err := localstorage.StoreConfig(&config)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]string{
		localstorage.StorageDir:        "",
		localstorage.StorageAddr:       "",
		localstorage.SharedStorageDir:  "",
		localstorage.SharedStorageAddr: "",
	})

	config.storageDir = "a"
	config.storageAddr = "b"
	config.sharedStorageDir = "c"
	config.sharedStorageAddr = "d"
	config.caCertPEM = []byte("heyhey")
	config.caKeyPEM = []byte("hoho")
	config.hostnames = []string{"easy", "as", "1.2.3"}
	config.authkey = "password"
	m, err = localstorage.StoreConfig(&config)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]string{
		localstorage.StorageDir:        config.storageDir,
		localstorage.StorageAddr:       config.storageAddr,
		localstorage.SharedStorageDir:  config.sharedStorageDir,
		localstorage.SharedStorageAddr: config.sharedStorageAddr,
		localstorage.StorageCACert:     string(config.caCertPEM),
		localstorage.StorageCAKey:      string(config.caKeyPEM),
		localstorage.StorageHostnames:  mustMarshalYAML(c, config.hostnames),
		localstorage.StorageAuthKey:    config.authkey,
	})
}

func mustMarshalYAML(c *gc.C, v interface{}) string {
	data, err := goyaml.Marshal(v)
	c.Assert(err, gc.IsNil)
	return string(data)
}
