// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strconv"
	"strings"
)

// Unit represents the state of a service unit.
type Unit struct {
	zk          *zookeeper.Conn
	node        string
	serviceNode   string
	serviceName string
	sequenceNo  int
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.serviceName
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return fmt.Sprintf("%s/%d", u.serviceName, u.sequenceNo)
}

// UnassignFromMachine removes the assignment between this unit and
// the machine it's assigned to.
func (u *Unit) UnassignFromMachine() error {
	unassignUnit := func(t *topology) error {
		if !t.hasService(u.serviceNode) || !t.hasUnit(u.serviceNode, u.node) {
			return stateChanged
		}
		// If for whatever reason it's already not assigned to a
		// machine, ignore it and move forward so that we don't
		// have to deal with conflicts.
		machineNode, err := t.unitMachineNode(u.serviceNode, u.node)
		if err == nil && machineNode != "" {
			t.unassignUnitFromMachine(u.serviceNode, u.node)
		}
		return nil
	}
	return retryTopologyChange(u.zk, unassignUnit)
}

// zkNodeName returns the ZooKeeper node name of the unit.
func (u *Unit) zkNodeName() string {
	return u.node
}

// Name returns the name of the unit based on the service
// zkPortsPath returns the ZooKeeper path for the open ports.
func (u *Unit) zkPortsPath() string {
	return fmt.Sprintf("/units/%s/ports", u.node)
}

// zkAgentPath returns the ZooKeeper path for the unit agent.
func (u *Unit) zkAgentPath() string {
	return fmt.Sprintf("/units/%s/agent", u.node)
}

// parseUnitName parses a unit name like "wordpress/0" into
// its service name and sequence number parts.
func parseUnitName(name string) (serviceName string, seqNo int, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%q is no valid unit name", name)
	}
	sequenceNo, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		return "", 0, err
	}
	return parts[0], int(sequenceNo), nil
}
