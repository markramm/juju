// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	gc "launchpad.net/gocheck"
)

type CleanupFunc func(*gc.C)
type cleanupStack []CleanupFunc

// CleanupSuite adds the ability to add cleanup functions that are called
// during either test tear down or suite tear down depending on the method
// called.
type CleanupSuite struct {
	testStack  cleanupStack
	suiteStack cleanupStack
}

func (s *CleanupSuite) SetUpSuite(c *gc.C) {
	s.suiteStack = nil
}

func (s *CleanupSuite) TearDownSuite(c *gc.C) {
	s.callStack(c, s.suiteStack)
}

func (s *CleanupSuite) SetUpTest(c *gc.C) {
	s.testStack = nil
}

func (s *CleanupSuite) TearDownTest(c *gc.C) {
	s.callStack(c, s.testStack)
}

func (s *CleanupSuite) callStack(c *gc.C, stack cleanupStack) {
	for i := len(stack) - 1; i >= 0; i-- {
		stack[i](c)
	}
}

// AddCleanup pushes the cleanup function onto the stack of functions to be
// called during TearDownTest.
func (s *CleanupSuite) AddCleanup(cleanup CleanupFunc) {
	s.testStack = append(s.testStack, cleanup)
}

// AddSuiteCleanup pushes the cleanup function onto the stack of functions to
// be called during TearDownSuite.
func (s *CleanupSuite) AddSuiteCleanup(cleanup CleanupFunc) {
	s.suiteStack = append(s.suiteStack, cleanup)
}

// PatchEnvironment sets the environment variable 'name' the the value passed
// in. The old value is saved and returned to the original value at test tear
// down time using a cleanup function.
func (s *CleanupSuite) PatchEnvironment(name, value string) {
	restore := PatchEnvironment(name, value)
	s.AddCleanup(func(*gc.C) { restore() })
}

// PatchValue sets the 'dest' variable the the value passed in. The old value
// is saved and returned to the original value at test tear down time using a
// cleanup function. The value must be assignable to the element type of the
// destination.
func (s *CleanupSuite) PatchValue(dest, value interface{}) {
	restore := PatchValue(dest, value)
	s.AddCleanup(func(*gc.C) { restore() })
}
