// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jameinel/juju/charm"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/tools"
)

// ErrorResults holds the results of calling a bulk operation which
// returns no data, only an error result. The order and
// number of elements matches the operations specified in the request.
type ErrorResults struct {
	// Results contains the error results from each operation.
	Results []ErrorResult
}

// OneError returns the error from the result
// of a bulk operation on a single value.
func (result ErrorResults) OneError() error {
	if n := len(result.Results); n != 1 {
		return fmt.Errorf("expected one result, got %d", n)
	}
	if err := result.Results[0].Error; err != nil {
		return err
	}
	return nil
}

// ErrorResult holds the error status of a single operation.
type ErrorResult struct {
	Error *Error
}

// StatusData contains additional information for a status.
type StatusData map[string]interface{}

// AddRelation holds the parameters for making the AddRelation call.
// The endpoints specified are unordered.
type AddRelation struct {
	Endpoints []string
}

// AddRelationResults holds the results of a AddRelation call. The Endpoints
// field maps service names to the involved endpoints.
type AddRelationResults struct {
	Endpoints map[string]charm.Relation
}

// DestroyRelation holds the parameters for making the DestroyRelation call.
// The endpoints specified are unordered.
type DestroyRelation struct {
	Endpoints []string
}

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	Series                  string
	ContainerType           instance.ContainerType
	Constraints             constraints.Value
	ParentId                string
	Jobs                    []MachineJob
	InstanceId              instance.Id
	Nonce                   string
	HardwareCharacteristics instance.HardwareCharacteristics
	Addrs                   []instance.Address
}

// AddMachines holds the parameters for making the AddMachines call.
type AddMachines struct {
	MachineParams []AddMachineParams
}

// AddMachinesResults holds the results of an AddMachines call.
type AddMachinesResults struct {
	Machines []AddMachinesResult
}

// AddMachinesResults holds the name of a machine added by the
// state.api.client.AddMachine call for a single machine.
type AddMachinesResult struct {
	Machine string
	Error   *Error
}

// DestroyMachines holds parameters for the DestroyMachines call.
type DestroyMachines struct {
	MachineNames []string
}

// ServiceDeploy holds the parameters for making the ServiceDeploy call.
type ServiceDeploy struct {
	ServiceName   string
	CharmUrl      string
	NumUnits      int
	Config        map[string]string
	ConfigYAML    string // Takes precedence over config if both are present.
	Constraints   constraints.Value
	ToMachineSpec string
}

// ServiceUpdate holds the parameters for making the ServiceUpdate call.
type ServiceUpdate struct {
	ServiceName     string
	CharmUrl        string
	ForceCharmUrl   bool
	MinUnits        *int
	SettingsStrings map[string]string
	SettingsYAML    string // Takes precedence over SettingsStrings if both are present.
	Constraints     *constraints.Value
}

// ServiceSetCharm sets the charm for a given service.
type ServiceSetCharm struct {
	ServiceName string
	CharmUrl    string
	Force       bool
}

// ServiceExpose holds the parameters for making the ServiceExpose call.
type ServiceExpose struct {
	ServiceName string
}

// ServiceSet holds the parameters for a ServiceSet
// command. Options contains the configuration data.
type ServiceSet struct {
	ServiceName string
	Options     map[string]string
}

// ServiceSetYAML holds the parameters for
// a ServiceSetYAML command. Config contains the
// configuration data in YAML format.
type ServiceSetYAML struct {
	ServiceName string
	Config      string
}

// ServiceUnset holds the parameters for a ServiceUnset
// command. Options contains the option attribute names
// to unset.
type ServiceUnset struct {
	ServiceName string
	Options     []string
}

// ServiceGet holds parameters for making the ServiceGet call.
type ServiceGet struct {
	ServiceName string
}

// ServiceGetResults holds results of the ServiceGet call.
type ServiceGetResults struct {
	Service     string
	Charm       string
	Config      map[string]interface{}
	Constraints constraints.Value
}

// ServiceCharmRelations holds parameters for making the ServiceCharmRelations call.
type ServiceCharmRelations struct {
	ServiceName string
}

// ServiceCharmRelationsResults holds the results of the ServiceCharmRelations call.
type ServiceCharmRelationsResults struct {
	CharmRelations []string
}

// ServiceUnexpose holds parameters for the ServiceUnexpose call.
type ServiceUnexpose struct {
	ServiceName string
}

// PublicAddress holds parameters for the PublicAddress call.
type PublicAddress struct {
	Target string
}

// PublicAddressResults holds results of the PublicAddress call.
type PublicAddressResults struct {
	PublicAddress string
}

// Resolved holds parameters for the Resolved call.
type Resolved struct {
	UnitName string
	Retry    bool
}

// ResolvedResults holds results of the Resolved call.
type ResolvedResults struct {
	Service  string
	Charm    string
	Settings map[string]interface{}
}

// AddServiceUnitsResults holds the names of the units added by the
// AddServiceUnits call.
type AddServiceUnitsResults struct {
	Units []string
}

// AddServiceUnits holds parameters for the AddUnits call.
type AddServiceUnits struct {
	ServiceName   string
	NumUnits      int
	ToMachineSpec string
}

// DestroyServiceUnits holds parameters for the DestroyUnits call.
type DestroyServiceUnits struct {
	UnitNames []string
}

// ServiceDestroy holds the parameters for making the ServiceDestroy call.
type ServiceDestroy struct {
	ServiceName string
}

// Creds holds credentials for identifying an entity.
type Creds struct {
	AuthTag  string
	Password string
	Nonce    string
}

// GetAnnotationsResults holds annotations associated with an entity.
type GetAnnotationsResults struct {
	Annotations map[string]string
}

// GetAnnotations stores parameters for making the GetAnnotations call.
type GetAnnotations struct {
	Tag string
}

// SetAnnotations stores parameters for making the SetAnnotations call.
type SetAnnotations struct {
	Tag   string
	Pairs map[string]string
}

// GetServiceConstraints stores parameters for making the GetServiceConstraints call.
type GetServiceConstraints struct {
	ServiceName string
}

// GetConstraintsResults holds results of the GetConstraints call.
type GetConstraintsResults struct {
	Constraints constraints.Value
}

// SetConstraints stores parameters for making the SetConstraints call.
type SetConstraints struct {
	ServiceName string //optional, if empty, environment constraints are set.
	Constraints constraints.Value
}

// CharmInfo stores parameters for a CharmInfo call.
type CharmInfo struct {
	CharmURL string
}

// AllWatcherId holds the id of an AllWatcher.
type AllWatcherId struct {
	AllWatcherId string
}

// AllWatcherNextResults holds deltas returned from calling AllWatcher.Next().
type AllWatcherNextResults struct {
	Deltas []Delta
}

// Delta holds details of a change to the environment.
type Delta struct {
	// If Removed is true, the entity has been removed;
	// otherwise it has been created or changed.
	Removed bool
	// Entity holds data about the entity that has changed.
	Entity EntityInfo
}

// MarshalJSON implements json.Marshaler.
func (d *Delta) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(d.Entity)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	c := "change"
	if d.Removed {
		c = "remove"
	}
	fmt.Fprintf(&buf, "%q,%q,", d.Entity.EntityId().Kind, c)
	buf.Write(b)
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Delta) UnmarshalJSON(data []byte) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(data, &elements); err != nil {
		return err
	}
	if len(elements) != 3 {
		return fmt.Errorf(
			"Expected 3 elements in top-level of JSON but got %d",
			len(elements))
	}
	var entityKind, operation string
	if err := json.Unmarshal(elements[0], &entityKind); err != nil {
		return err
	}
	if err := json.Unmarshal(elements[1], &operation); err != nil {
		return err
	}
	if operation == "remove" {
		d.Removed = true
	} else if operation != "change" {
		return fmt.Errorf("Unexpected operation %q", operation)
	}
	switch entityKind {
	case "machine":
		d.Entity = new(MachineInfo)
	case "service":
		d.Entity = new(ServiceInfo)
	case "unit":
		d.Entity = new(UnitInfo)
	case "relation":
		d.Entity = new(RelationInfo)
	case "annotation":
		d.Entity = new(AnnotationInfo)
	default:
		return fmt.Errorf("Unexpected entity name %q", entityKind)
	}
	if err := json.Unmarshal(elements[2], &d.Entity); err != nil {
		return err
	}
	return nil
}

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	// EntityId returns an identifier that will uniquely
	// identify the entity within its kind
	EntityId() EntityId
}

// IMPORTANT NOTE: the types below are direct subsets of the entity docs
// held in mongo, as defined in the state package (serviceDoc,
// machineDoc etc).
// In particular, the document marshalled into mongo
// must unmarshal correctly into these documents.
// If the format of a field in a document is changed in mongo, or
// a field is removed and it coincides with one of the
// fields below, a similar change must be made here.
//
// MachineInfo corresponds with state.machineDoc.
// ServiceInfo corresponds with state.serviceDoc.
// UnitInfo corresponds with state.unitDoc.
// RelationInfo corresponds with state.relationDoc.
// AnnotationInfo corresponds with state.annotatorDoc.

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ServiceInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
	_ EntityInfo = (*AnnotationInfo)(nil)
)

type EntityId struct {
	Kind string
	Id   interface{}
}

// MachineInfo holds the information about a Machine
// that is watched by StateWatcher.
type MachineInfo struct {
	Id         string `bson:"_id"`
	InstanceId string
	Status     Status
	StatusInfo string
	StatusData StatusData
}

func (i *MachineInfo) EntityId() EntityId {
	return EntityId{
		Kind: "machine",
		Id:   i.Id,
	}
}

type ServiceInfo struct {
	Name        string `bson:"_id"`
	Exposed     bool
	CharmURL    string
	Life        Life
	MinUnits    int
	Constraints constraints.Value
	Config      map[string]interface{}
}

func (i *ServiceInfo) EntityId() EntityId {
	return EntityId{
		Kind: "service",
		Id:   i.Name,
	}
}

type UnitInfo struct {
	Name           string `bson:"_id"`
	Service        string
	Series         string
	CharmURL       string
	PublicAddress  string
	PrivateAddress string
	MachineId      string
	Ports          []instance.Port
	Status         Status
	StatusInfo     string
	StatusData     StatusData
}

func (i *UnitInfo) EntityId() EntityId {
	return EntityId{
		Kind: "unit",
		Id:   i.Name,
	}
}

type Endpoint struct {
	ServiceName string
	Relation    charm.Relation
}

type RelationInfo struct {
	Key       string `bson:"_id"`
	Id        int
	Endpoints []Endpoint
}

func (i *RelationInfo) EntityId() EntityId {
	return EntityId{
		Kind: "relation",
		Id:   i.Key,
	}
}

type AnnotationInfo struct {
	Tag         string
	Annotations map[string]string
}

func (i *AnnotationInfo) EntityId() EntityId {
	return EntityId{
		Kind: "annotation",
		Id:   i.Tag,
	}
}

// ContainerConfig contains information from the environment config that is
// needed for container cloud-init.
type ContainerConfig struct {
	ProviderType            string
	AuthorizedKeys          string
	SSLHostnameVerification bool
}

type MachineConfigParams struct {
	MachineId string
	Series    string
	Arch      string
}

// MachineConfig contains information from the environment config that is
// needed for a machine cloud-init.
type MachineConfig struct {
	EnvironAttrs map[string]interface{}
	Tools        *tools.Tools
	// state.Info and api.Info attributes (cannot use state.Info, api.Info directly due to import loops)
	StateAddrs []string
	APIAddrs   []string
	CACert     []byte
	Tag        string
	Password   string
}

// EnvironmentGetResults contains the result of EnvironmentGet client
// API call.
type EnvironmentGetResults struct {
	Config map[string]interface{}
}

// EnvironmentSet contains the arguments for EnvironmentSet client API
// call.
type EnvironmentSet struct {
	Config map[string]interface{}
}
