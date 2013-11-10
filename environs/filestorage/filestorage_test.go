// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage_test

// The filestorage structs are used as stubs in tests.
// The tests defined herein are simple smoke tests for the
// required reader and writer functionality.

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"github.com/jameinel/juju/environs/filestorage"
	"github.com/jameinel/juju/environs/storage"
	coreerrors "github.com/jameinel/juju/errors"
	jc "github.com/jameinel/juju/testing/checkers"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type filestorageSuite struct {
	dir    string
	reader storage.StorageReader
	writer storage.StorageWriter
}

var _ = gc.Suite(&filestorageSuite{})

func (s *filestorageSuite) SetUpTest(c *gc.C) {
	s.dir = c.MkDir()
	var err error
	s.reader, err = filestorage.NewFileStorageReader(s.dir)
	c.Assert(err, gc.IsNil)
	s.writer, err = filestorage.NewFileStorageWriter(s.dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
}

func (s *filestorageSuite) createFile(c *gc.C, name string) (fullpath string, data []byte) {
	fullpath = filepath.Join(s.dir, name)
	dir := filepath.Dir(fullpath)
	c.Assert(os.MkdirAll(dir, 0755), gc.IsNil)
	data = []byte{1, 2, 3, 4, 5}
	err := ioutil.WriteFile(fullpath, data, 0644)
	c.Assert(err, gc.IsNil)
	return fullpath, data
}

func (s *filestorageSuite) TestList(c *gc.C) {
	names := []string{
		"a/b/c",
		"a/bb",
		"a/c",
		"aa",
		"b/c/d",
	}
	for _, name := range names {
		s.createFile(c, name)
	}
	type test struct {
		prefix   string
		expected []string
	}
	for i, test := range []test{
		{"a", []string{"a/b/c", "a/bb", "a/c", "aa"}},
		{"a/b", []string{"a/b/c", "a/bb"}},
		{"a/b/c", []string{"a/b/c"}},
		{"", names},
	} {
		c.Logf("test %d: prefix=%q", i, test.prefix)
		files, err := storage.List(s.reader, test.prefix)
		c.Assert(err, gc.IsNil)
		c.Assert(files, gc.DeepEquals, test.expected)
	}
}

func (s *filestorageSuite) TestURL(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	url, err := s.reader.URL(file)
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "file://"+expectedpath)
}

func (s *filestorageSuite) TestGet(c *gc.C) {
	expectedpath, data := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	rc, err := storage.Get(s.reader, file)
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(err, gc.IsNil)
	b, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.DeepEquals, data)

	// Get on a non-existant path returns NotFoundError
	_, err = s.reader.Get("nowhere")
	c.Assert(err, jc.Satisfies, coreerrors.IsNotFoundError)

	// Get on a directory returns NotFoundError
	s.createFile(c, "dir/file")
	_, err = s.reader.Get("dir")
	c.Assert(err, jc.Satisfies, coreerrors.IsNotFoundError)
}

func (s *filestorageSuite) TestPut(c *gc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put("test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)
	b, err := ioutil.ReadFile(filepath.Join(s.dir, "test-write"))
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.DeepEquals, data)
}

func (s *filestorageSuite) TestRemove(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	err := s.writer.Remove(file)
	c.Assert(err, gc.IsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *filestorageSuite) TestRemoveAll(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	err := s.writer.RemoveAll()
	c.Assert(err, gc.IsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *filestorageSuite) TestPutTmpDir(c *gc.C) {
	// Put should create and clean up the temporary directory if
	// tmpdir==UseDefaultTmpDir.
	err := s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(s.dir + ".tmp")
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// To deal with recovering from hard failure, UseDefaultTmpDir
	// doesn't care if the temporary directory already exists. It
	// still removes it, though.
	err = os.Mkdir(s.dir+".tmp", 0755)
	c.Assert(err, gc.IsNil)
	err = s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(s.dir + ".tmp")
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// If we explicitly set the temporary directory, it must already exist.
	s.writer, err = filestorage.NewFileStorageWriter(s.dir, s.dir+".tmp")
	c.Assert(err, gc.IsNil)
	err = s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	err = os.Mkdir(s.dir+".tmp", 0755)
	c.Assert(err, gc.IsNil)
	err = s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, gc.IsNil)
	// Temporary directory should not have been moved.
	_, err = os.Stat(s.dir + ".tmp")
	c.Assert(err, gc.IsNil)
}
