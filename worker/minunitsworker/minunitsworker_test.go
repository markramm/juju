// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"github.com/jameinel/juju/juju/testing"
	coretesting "github.com/jameinel/juju/testing"
	"github.com/jameinel/juju/worker"
	"github.com/jameinel/juju/worker/minunitsworker"
)

var logger = loggo.GetLogger("juju.worker.minunitsworker_test")

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type minUnitsWorkerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&minUnitsWorkerSuite{})

var _ worker.StringsWatchHandler = (*minunitsworker.MinUnitsWorker)(nil)

func (s *minUnitsWorkerSuite) TestMinUnitsWorker(c *gc.C) {
	mu := minunitsworker.NewMinUnitsWorker(s.State)
	defer func() { c.Assert(worker.Stop(mu), gc.IsNil) }()

	// Set up services and units for later use.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, gc.IsNil)

	// Set up minimum units for services.
	err = wordpress.SetMinUnits(3)
	c.Assert(err, gc.IsNil)
	err = mysql.SetMinUnits(2)
	c.Assert(err, gc.IsNil)

	// Remove a unit for a service.
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)

	timeout := time.After(coretesting.LongWait)
	for {
		s.State.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			wordpressUnits, err := wordpress.AllUnits()
			c.Assert(err, gc.IsNil)
			mysqlUnits, err := mysql.AllUnits()
			c.Assert(err, gc.IsNil)
			wordpressCount := len(wordpressUnits)
			mysqlCount := len(mysqlUnits)
			if wordpressCount == 3 && mysqlCount == 2 {
				return
			}
			logger.Infof("wordpress units: %d; mysql units: %d", wordpressCount, mysqlCount)
		case <-timeout:
			c.Fatalf("timed out waiting for minunits events")
		}
	}
}
