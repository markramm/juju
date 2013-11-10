// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage_test

import (
	"bytes"
	"io"
	"testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/storage"
	envtesting "github.com/jameinel/juju/environs/testing"
	"github.com/jameinel/juju/provider/ec2/httpstorage"
	"github.com/jameinel/juju/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type storageSuite struct {
	storage *envtesting.EC2HTTPTestStorage
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	var err error

	s.storage, err = envtesting.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, gc.IsNil)

	for _, v := range versions {
		s.storage.PutBinary(v)
	}
}

func (s *storageSuite) TearDownTest(c *gc.C) {
	c.Assert(s.storage.Stop(), gc.IsNil)
}

func (s *storageSuite) TestHTTPStorage(c *gc.C) {
	sr := httpstorage.NewHTTPStorageReader(s.storage.Location())
	list, err := storage.List(sr, "tools/releases/juju-")
	c.Assert(err, gc.IsNil)
	c.Assert(len(list), gc.Equals, 6)

	url, err := sr.URL(list[0])
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Matches, "http://127.0.0.1:.*/tools/releases/juju-1.0.0-precise-amd64.tgz")

	rc, err := storage.Get(sr, list[0])
	c.Assert(err, gc.IsNil)
	defer rc.Close()

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, rc)
	c.Assert(err, gc.IsNil)
	c.Assert(buf.String(), gc.Equals, "1.0.0-precise-amd64")
}

var versions = []version.Binary{
	version.MustParseBinary("1.0.0-precise-amd64"),
	version.MustParseBinary("1.0.0-quantal-amd64"),
	version.MustParseBinary("1.0.0-quantal-i386"),
	version.MustParseBinary("1.9.0-quantal-amd64"),
	version.MustParseBinary("1.9.0-precise-i386"),
	version.MustParseBinary("2.0.0-precise-amd64"),
}
