// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/configstore"
	"github.com/jameinel/juju/environs/storage"
	"github.com/jameinel/juju/errors"
	"github.com/jameinel/juju/provider/dummy"
	"github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/testing/testbase"
)

type EmptyStorageSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&EmptyStorageSuite{})

func (s *EmptyStorageSuite) TestGet(c *gc.C) {
	f, err := storage.Get(environs.EmptyStorage, "anything")
	c.Assert(f, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestURL(c *gc.C) {
	url, err := environs.EmptyStorage.URL("anything")
	c.Assert(url, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestList(c *gc.C) {
	names, err := storage.List(environs.EmptyStorage, "anything")
	c.Assert(names, gc.IsNil)
	c.Assert(err, gc.IsNil)
}

type verifyStorageSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&verifyStorageSuite{})

const existingEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

func (s *verifyStorageSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *verifyStorageSuite) TestVerifyStorage(c *gc.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test", configstore.NewMem())
	c.Assert(err, gc.IsNil)
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)
	reader, err := storage.Get(stor, environs.VerificationFilename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	contents, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(contents), gc.Equals,
		"juju-core storage writing verified: ok\n")
}

func (s *verifyStorageSuite) TestVerifyStorageFails(c *gc.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test", configstore.NewMem())
	c.Assert(err, gc.IsNil)
	stor := environ.Storage()
	someError := errors.Unauthorizedf("you shall not pass")
	dummy.Poison(stor, environs.VerificationFilename, someError)
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.Equals, environs.VerifyStorageError)
}
