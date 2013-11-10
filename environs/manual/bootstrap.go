// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"github.com/jameinel/juju/environs"
	envtools "github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/provider/common"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/worker/localstorage"
)

const BootstrapInstanceId = instance.Id(manualInstancePrefix)

// LocalStorageEnviron is an Environ where the bootstrap node
// manages its own local storage.
type LocalStorageEnviron interface {
	environs.Environ
	localstorage.LocalStorageConfig
}

type BootstrapArgs struct {
	Host          string
	DataDir       string
	Environ       LocalStorageEnviron
	PossibleTools tools.List
}

func errMachineIdInvalid(machineId string) error {
	return fmt.Errorf("%q is not a valid machine ID", machineId)
}

// NewManualBootstrapEnviron wraps a LocalStorageEnviron with another which
// overrides the Bootstrap method; when Bootstrap is invoked, the specified
// host will be manually bootstrapped.
func Bootstrap(args BootstrapArgs) (err error) {
	if args.Host == "" {
		return errors.New("host argument is empty")
	}
	if args.Environ == nil {
		return errors.New("environ argument is nil")
	}
	if args.DataDir == "" {
		return errors.New("data-dir argument is empty")
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		return fmt.Errorf("failed to check provisioned status: %v", err)
	}
	if provisioned {
		return ErrProvisioned
	}

	hc, series, err := detectSeriesAndHardwareCharacteristics(args.Host)
	if err != nil {
		return fmt.Errorf("error detecting hardware characteristics: %v", err)
	}

	// Filter tools based on detected series/arch.
	logger.Infof("Filtering possible tools: %v", args.PossibleTools)
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *hc.Arch,
		Series: series,
	})
	if err != nil {
		return err
	}

	// Store the state file. If provisioning fails, we'll remove the file.
	logger.Infof("Saving bootstrap state file to bootstrap storage")
	bootstrapStorage := args.Environ.Storage()
	err = common.SaveState(
		bootstrapStorage,
		&common.BootstrapState{
			StateInstances:  []instance.Id{BootstrapInstanceId},
			Characteristics: []instance.HardwareCharacteristics{hc},
		},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			logger.Errorf("bootstrapping failed, removing state file: %v", err)
			bootstrapStorage.Remove(common.StateFile)
		}
	}()

	// Get a file:// scheme tools URL for the tools, which will have been
	// copied to the remote machine's storage directory.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)

	// Add the local storage configuration.
	agentEnv, err := localstorage.StoreConfig(args.Environ)
	if err != nil {
		return err
	}

	// Finally, provision the machine agent.
	stateFileURL := fmt.Sprintf("file://%s/%s", storageDir, common.StateFile)
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:          args.Host,
		dataDir:       args.DataDir,
		environConfig: args.Environ.Config(),
		stateFileURL:  stateFileURL,
		bootstrap:     true,
		nonce:         state.BootstrapNonce,
		tools:         &tools,
		agentEnv:      agentEnv,
	})
	return err
}
