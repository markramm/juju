// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/storage"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/tools"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// Prepare prepares an environment for use. Any additional
	// configuration attributes in the returned environment should
	// be saved to be used later. If the environment is already
	// prepared, this call is equivalent to Open.
	Prepare(cfg *config.Config) (Environ, error)

	// Open opens the environment and returns it.
	// The configuration must have come from a previously
	// prepared environment.
	Open(cfg *config.Config) (Environ, error)

	// Validate ensures that config is a valid configuration for this
	// provider, applying changes to it if necessary, and returns the
	// validated configuration.
	// If old is not nil, it holds the previous environment configuration
	// for consideration when validating changes.
	Validate(cfg, old *config.Config) (valid *config.Config, err error)

	// Boilerplate returns a default configuration for the environment in yaml format.
	// The text should be a key followed by some number of attributes:
	//    `environName:
	//        type: environTypeName
	//        attr1: val1
	//    `
	// The text is used as a template (see the template package) with one extra template
	// function available, rand, which expands to a random hexadecimal string when invoked.
	BoilerplateConfig() string

	// SecretAttrs filters the supplied configuration returning only values
	// which are considered sensitive. All of the values of these secret
	// attributes need to be strings.
	SecretAttrs(cfg *config.Config) (map[string]string, error)

	// PublicAddress returns this machine's public host name.
	PublicAddress() (string, error)

	// PrivateAddress returns this machine's private host name.
	PrivateAddress() (string, error)
}

// EnvironStorage implements storage access for an environment
type EnvironStorage interface {
	// Storage returns storage specific to the environment.
	Storage() storage.Storage
}

// BootstrapStorager is an interface through which an Environ may be
// instructed to use a special "bootstrap storage". Bootstrap storage
// is one that may be used before the bootstrap machine agent has been
// provisioned.
//
// This is useful for environments where the storage is managed by the
// machine agent once bootstrapped.
type BootstrapStorager interface {
	// EnableBootstrapStorage enables bootstrap storage, returning an
	// error if enablement failed. If nil is returned, then calling
	// this again will have no effect and will return nil.
	EnableBootstrapStorage() error
}

// ConfigGetter implements access to an environments configuration.
type ConfigGetter interface {
	// Config returns the configuration data with which the Environ was created.
	// Note that this is not necessarily current; the canonical location
	// for the configuration data is stored in the state.
	Config() *config.Config
}

// Prechecker is an optional interface that an Environ may implement,
// in order to support pre-flight checking of instance/container creation.
//
// Prechecker's methods are best effort, and not guaranteed to eliminate
// all invalid parameters. If a precheck method returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
type Prechecker interface {
	// PrecheckInstance performs a preflight check on the specified
	// series and constraints, ensuring that they are possibly valid for
	// creating an instance in this environment.
	PrecheckInstance(series string, cons constraints.Value) error

	// PrecheckContainer performs a preflight check on the container type,
	// ensuring that the environment is possibly capable of creating a
	// container of the specified type and series.
	//
	// The container type must be a valid ContainerType as specified
	// in the instance package, and != instance.NONE.
	PrecheckContainer(series string, kind instance.ContainerType) error
}

// An Environ represents a juju environment as specified
// in the environments.yaml file.
//
// Due to the limitations of some providers (for example ec2), the
// results of the Environ methods may not be fully sequentially
// consistent. In particular, while a provider may retry when it
// gets an error for an operation, it will not retry when
// an operation succeeds, even if that success is not
// consistent with a previous operation.
//
// Even though Juju takes care not to share an Environ between concurrent
// workers, it does allow concurrent method calls into the provider
// implementation.  The typical provider implementation needs locking to
// avoid undefined behaviour when the configuration changes.
type Environ interface {
	// Name returns the Environ's name.
	Name() string

	// Bootstrap initializes the state for the environment, possibly
	// starting one or more instances.  If the configuration's
	// AdminSecret is non-empty, the administrator password on the
	// newly bootstrapped state will be set to a hash of it (see
	// utils.PasswordHash), When first connecting to the
	// environment via the juju package, the password hash will be
	// automatically replaced by the real password.
	//
	// The supplied constraints are used to choose the initial instance
	// specification, and will be stored in the new environment's state.
	Bootstrap(cons constraints.Value, possibleTools tools.List) error

	// StateInfo returns information on the state initialized
	// by Bootstrap.
	StateInfo() (*state.Info, *api.Info, error)

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

	// ConfigGetter allows the retrieval of the configuration data.
	ConfigGetter

	// SetConfig updates the Environ's configuration.
	//
	// Calls to SetConfig do not affect the configuration of
	// values previously obtained from Storage.
	SetConfig(cfg *config.Config) error

	// Instances returns a slice of instances corresponding to the
	// given instance ids.  If no instances were found, but there
	// was no other error, it will return ErrNoInstances.  If
	// some but not all the instances were found, the returned slice
	// will have some nil slots, and an ErrPartialInstances error
	// will be returned.
	Instances(ids []instance.Id) ([]instance.Instance, error)

	EnvironStorage

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. Note that on some providers,
	// very recently started instances may not be destroyed
	// because they are not yet visible.
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid
	Destroy() error

	// OpenPorts opens the given ports for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	OpenPorts(ports []instance.Port) error

	// ClosePorts closes the given ports for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	ClosePorts(ports []instance.Port) error

	// Ports returns the ports opened for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	Ports() ([]instance.Port, error)

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider
}
