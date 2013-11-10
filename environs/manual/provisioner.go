// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"launchpad.net/loggo"

	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/juju"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/state/api/params"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/utils"
)

const manualInstancePrefix = "manual:"

var logger = loggo.GetLogger("juju.environs.manual")

type ProvisionMachineArgs struct {
	// Host is the SSH host: [user@]host
	Host string

	// DataDir is the root directory for juju data.
	// If left blank, the default location "/var/lib/juju" will be used.
	DataDir string

	// EnvName is the name of the environment for which the machine will be provisioned.
	EnvName string

	// Tools to install on the machine. If nil, tools will be automatically
	// chosen using environs/tools FindInstanceTools.
	Tools *tools.Tools
}

// ErrProvisioned is returned by ProvisionMachine if the target
// machine has an existing machine agent.
var ErrProvisioned = errors.New("machine is already provisioned")

// ProvisionMachine provisions a machine agent to an existing host, via
// an SSH connection to the specified host. The host may optionally be preceded
// with a login username, as in [user@]host.
//
// On successful completion, this function will return the id of the state.Machine
// that was entered into state.
func ProvisionMachine(args ProvisionMachineArgs) (machineId string, err error) {
	client, err := juju.NewAPIClientFromName(args.EnvName)
	if err != nil {
		return "", err
	}
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			client.DestroyMachines(machineId)
			machineId = ""
		}
		client.Close()
	}()

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	instanceId := instance.Id(manualInstancePrefix + hostWithoutUser(args.Host))
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())

	// Inform Juju that the machine exists.
	machineId, series, arch, err := recordMachineInState(client, args.Host, nonce, instanceId)
	if err != nil {
		return "", err
	}

	// Gather the information needed by the machine agent to run the provisioning script.
	provisioningArgs, err := createProvisioningArgs(client, machineId, series, arch)
	if err != nil {
		return machineId, err
	}
	provisioningArgs.host = args.Host
	provisioningArgs.dataDir = args.DataDir
	provisioningArgs.nonce = nonce

	// Finally, provision the machine agent.
	err = provisionMachineAgent(*provisioningArgs)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}

func hostWithoutUser(host string) string {
	hostWithoutUser := host
	if at := strings.Index(hostWithoutUser, "@"); at != -1 {
		hostWithoutUser = hostWithoutUser[at+1:]
	}
	return hostWithoutUser
}

func recordMachineInState(
	client *api.Client, host, nonce string, instanceId instance.Id) (machineId, series, arch string, err error) {

	// First, gather the parameters needed to inject the existing host into state.
	sshHostWithoutUser := hostWithoutUser(host)
	if ip := net.ParseIP(sshHostWithoutUser); ip != nil {
		// Do a reverse-lookup on the IP. The IP may not have
		// a DNS entry, so just log a warning if this fails.
		names, err := net.LookupAddr(ip.String())
		if err != nil {
			logger.Infof("failed to resolve %v: %v", ip, err)
		} else {
			logger.Infof("resolved %v to %v", ip, names)
			sshHostWithoutUser = names[0]
		}
	}
	addrs, err := instance.HostAddresses(sshHostWithoutUser)
	if err != nil {
		return "", "", "", err
	}
	logger.Infof("addresses for %v: %v", sshHostWithoutUser, addrs)

	provisioned, err := checkProvisioned(host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return "", "", "", err
	}
	if provisioned {
		return "", "", "", ErrProvisioned
	}

	hc, series, err := detectSeriesAndHardwareCharacteristics(host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return "", "", "", err
	}

	// Inject a new machine into state.
	//
	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.
	machineParams := params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   addrs,
		Jobs:                    []params.MachineJob{params.JobHostUnits},
	}
	results, err := client.InjectMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return "", "", "", err
	}
	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return "", "", "", machineInfo.Error
	}
	return machineInfo.Machine, series, *hc.Arch, nil
}

func createProvisioningArgs(client *api.Client, machineId, series, arch string) (*provisionMachineAgentArgs, error) {
	configParameters, err := client.MachineConfig(machineId, series, arch)
	if err != nil {
		return nil, err
	}

	stateInfo := &state.Info{
		Addrs:    configParameters.StateAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	apiInfo := &api.Info{
		Addrs:    configParameters.StateAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	environConfig, err := config.New(config.NoDefaults, configParameters.EnvironAttrs)
	if err != nil {
		return nil, err
	}

	return &provisionMachineAgentArgs{
		environConfig: environConfig,
		machineId:     machineId,
		bootstrap:     false,
		stateInfo:     stateInfo,
		apiInfo:       apiInfo,
		tools:         configParameters.Tools,
	}, nil
}
