// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jameinel/juju/agent"
	agenttools "github.com/jameinel/juju/agent/tools"
	"github.com/jameinel/juju/constraints"
	"github.com/jameinel/juju/container/lxc"
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/cloudinit"
	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/environs/filestorage"
	"github.com/jameinel/juju/environs/httpstorage"
	"github.com/jameinel/juju/environs/simplestreams"
	"github.com/jameinel/juju/environs/storage"
	envtools "github.com/jameinel/juju/environs/tools"
	"github.com/jameinel/juju/instance"
	"github.com/jameinel/juju/juju/osenv"
	"github.com/jameinel/juju/names"
	"github.com/jameinel/juju/provider/common"
	"github.com/jameinel/juju/state"
	"github.com/jameinel/juju/state/api"
	"github.com/jameinel/juju/tools"
	"github.com/jameinel/juju/upstart"
	"github.com/jameinel/juju/utils"
	"github.com/jameinel/juju/version"
)

// boostrapInstanceId is just the name we give to the bootstrap machine.
// Using "localhost" because it is, and it makes sense.
const bootstrapInstanceId instance.Id = "localhost"

// upstartScriptLocation is parameterised purely for testing purposes as we
// don't really want to be installing and starting scripts as root for
// testing.
var upstartScriptLocation = "/etc/init"

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

// localEnviron implements SupportsCustomSources.
var _ envtools.SupportsCustomSources = (*localEnviron)(nil)

type localEnviron struct {
	localMutex            sync.Mutex
	config                *environConfig
	name                  string
	sharedStorageListener net.Listener
	storageListener       net.Listener
	containerManager      lxc.ContainerManager
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *localEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource(e.Storage(), storage.BaseToolsPath)}, nil
}

// Name is specified in the Environ interface.
func (env *localEnviron) Name() string {
	return env.name
}

func (env *localEnviron) mongoServiceName() string {
	return "juju-db-" + env.config.namespace()
}

func (env *localEnviron) machineAgentServiceName() string {
	return "juju-agent-" + env.config.namespace()
}

// PrecheckInstance is specified in the environs.Prechecker interface.
func (*localEnviron) PrecheckInstance(series string, cons constraints.Value) error {
	return nil
}

// PrecheckContainer is specified in the environs.Prechecker interface.
func (*localEnviron) PrecheckContainer(series string, kind instance.ContainerType) error {
	// This check can either go away or be relaxed when the local
	// provider can do nested containers.
	return environs.NewContainersUnsupported("local provider does not support nested containers")
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value, possibleTools tools.List) error {
	if !env.config.runningAsRoot {
		return fmt.Errorf("bootstrapping a local environment must be done as root")
	}
	if err := env.config.createDirs(); err != nil {
		logger.Errorf("failed to create necessary directories: %v", err)
		return err
	}

	// TODO(thumper): check that the constraints don't include "container=lxc" for now.

	cert, key, err := env.setupLocalMongoService()
	if err != nil {
		return err
	}

	// Before we write the agent config file, we need to make sure the
	// instance is saved in the StateInfo.
	if err := common.SaveState(env.Storage(), &common.BootstrapState{
		StateInstances: []instance.Id{bootstrapInstanceId},
	}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		return err
	}

	// Need to write out the agent file for machine-0 before initializing
	// state, as as part of that process, it will reset the password in the
	// agent file.
	agentConfig, err := env.writeBootstrapAgentConfFile(env.config.AdminSecret(), cert, key)
	if err != nil {
		return err
	}

	if err := env.initializeState(agentConfig, cons); err != nil {
		return err
	}

	return env.setupLocalMachineAgent(cons, possibleTools)
}

// StateInfo is specified in the Environ interface.
func (env *localEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
}

// Config is specified in the Environ interface.
func (env *localEnviron) Config() *config.Config {
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	return env.config.Config
}

func createLocalStorageListener(dir, address string) (net.Listener, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("storage directory %q does not exist, bootstrap first", dir)
	} else if err != nil {
		return nil, err
	} else if !info.Mode().IsDir() {
		return nil, fmt.Errorf("%q exists but is not a directory (and it needs to be)", dir)
	}
	storage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return nil, err
	}
	return httpstorage.Serve(address, storage)
}

// SetConfig is specified in the Environ interface.
func (env *localEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	env.config = ecfg
	env.name = ecfg.Name()

	env.containerManager = lxc.NewContainerManager(
		lxc.ManagerConfig{
			Name:   env.config.namespace(),
			LogDir: env.config.logDir(),
		})

	// Here is the end of normal config setting.
	if ecfg.bootstrapped() {
		return nil
	}
	return env.bootstrapAddressAndStorage(cfg)
}

// bootstrapAddressAndStorage finishes up the setup of the environment in
// situations where there is no machine agent running yet.
func (env *localEnviron) bootstrapAddressAndStorage(cfg *config.Config) error {
	// If we get to here, it is because we haven't yet bootstrapped an
	// environment, and saved the config in it, or we are running a command
	// from the command line, so it is ok to work on the assumption that we
	// have direct access to the directories.
	if err := env.config.createDirs(); err != nil {
		return err
	}

	// We need the provider config to get the network bridge.
	config, err := providerInstance.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	networkBridge := config.networkBridge()
	bridgeAddress, err := env.findBridgeAddress(networkBridge)
	if err != nil {
		logger.Infof("configure a different bridge using 'network-bridge' in the config file")
		return fmt.Errorf("cannot find address of network-bridge: %q", networkBridge)
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, networkBridge)
	cfg, err = cfg.Apply(map[string]interface{}{
		"bootstrap-ip": bridgeAddress,
	})
	if err != nil {
		logger.Errorf("failed to apply new addresses to config: %v", err)
		return err
	}
	// Now recreate the config based on the settings with the bootstrap id.
	config, err = providerInstance.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.config = config

	return env.setupLocalStorage()
}

// setupLocalStorage looks to see if there is someone listening on the storage
// address port.  If there is we assume that it is ours and all is good.  If
// there is no one listening on that port, create listeners for both storage
// and the shared storage for the duration of the commands execution.
func (env *localEnviron) setupLocalStorage() error {
	// Try to listen to the storageAddress.
	logger.Debugf("checking %s to see if machine agent running storage listener", env.config.storageAddr())
	connection, err := net.Dial("tcp", env.config.storageAddr())
	if err != nil {
		logger.Debugf("nope, start some")
		// These listeners are part of the environment structure so as to remain
		// referenced for the duration of the open environment.  This is only for
		// environs that have been created due to a user command.
		env.storageListener, err = createLocalStorageListener(env.config.storageDir(), env.config.storageAddr())
		if err != nil {
			return err
		}
		env.sharedStorageListener, err = createLocalStorageListener(env.config.sharedStorageDir(), env.config.sharedStorageAddr())
		if err != nil {
			return err
		}
	} else {
		logger.Debugf("yes, don't start local storage listeners")
		connection.Close()
	}
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	series := possibleTools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", machineConfig.MachineId, series)
	machineConfig.Tools = possibleTools[0]
	machineConfig.MachineContainerType = instance.LXC
	logger.Debugf("tools: %#v", machineConfig.Tools)
	network := lxc.BridgeNetworkConfig(env.config.networkBridge())
	if err := environs.FinishMachineConfig(machineConfig, env.config.Config, cons); err != nil {
		return nil, nil, err
	}
	inst, err := env.containerManager.StartContainer(machineConfig, series, network)
	if err != nil {
		return nil, nil, err
	}
	// TODO(thumper): return some hardware characteristics.
	return inst, nil, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StopInstances(instances []instance.Instance) error {
	for _, inst := range instances {
		if inst.Id() == bootstrapInstanceId {
			return fmt.Errorf("cannot stop the bootstrap instance")
		}
		if err := env.containerManager.StopContainer(inst); err != nil {
			return err
		}
	}
	return nil
}

// Instances is specified in the Environ interface.
func (env *localEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// NOTE: do we actually care about checking the existance of the instances?
	// I posit that here we don't really care, and that we are only called with
	// instance ids that we know exist.
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instance.Instance, len(ids))
	for i, id := range ids {
		insts[i] = &localInstance{id, env}
	}
	return insts, nil
}

// AllInstances is specified in the InstanceBroker interface.
func (env *localEnviron) AllInstances() (instances []instance.Instance, err error) {
	instances = append(instances, &localInstance{bootstrapInstanceId, env})
	// Add in all the containers as well.
	lxcInstances, err := env.containerManager.ListContainers()
	if err != nil {
		return nil, err
	}
	for _, inst := range lxcInstances {
		instances = append(instances, &localInstance{inst.Id(), env})
	}
	return instances, nil
}

// Storage is specified in the Environ interface.
func (env *localEnviron) Storage() storage.Storage {
	return httpstorage.Client(env.config.storageAddr())
}

// Implements environs.BootstrapStorager.
func (env *localEnviron) EnableBootstrapStorage() error {
	return env.setupLocalStorage()
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy() error {
	if !env.config.runningAsRoot {
		return fmt.Errorf("destroying a local environment must be done as root")
	}
	// Kill all running instances.
	containers, err := env.containerManager.ListContainers()
	if err != nil {
		return err
	}
	for _, inst := range containers {
		if err := env.containerManager.StopContainer(inst); err != nil {
			return err
		}
	}

	logger.Infof("removing service %s", env.machineAgentServiceName())
	machineAgent := upstart.NewService(env.machineAgentServiceName())
	machineAgent.InitDir = upstartScriptLocation
	if err := machineAgent.StopAndRemove(); err != nil {
		logger.Errorf("could not remove machine agent service: %v", err)
		return err
	}

	logger.Infof("removing service %s", env.mongoServiceName())
	mongo := upstart.NewService(env.mongoServiceName())
	mongo.InitDir = upstartScriptLocation
	if err := mongo.StopAndRemove(); err != nil {
		logger.Errorf("could not remove mongo service: %v", err)
		return err
	}

	// Remove the rootdir.
	logger.Infof("removing state dir %s", env.config.rootDir())
	if err := os.RemoveAll(env.config.rootDir()); err != nil {
		logger.Errorf("could not remove local state dir: %v", err)
		return err
	}

	return nil
}

// OpenPorts is specified in the Environ interface.
func (env *localEnviron) OpenPorts(ports []instance.Port) error {
	return fmt.Errorf("open ports not implemented")
}

// ClosePorts is specified in the Environ interface.
func (env *localEnviron) ClosePorts(ports []instance.Port) error {
	return fmt.Errorf("close ports not implemented")
}

// Ports is specified in the Environ interface.
func (env *localEnviron) Ports() ([]instance.Port, error) {
	return nil, nil
}

// Provider is specified in the Environ interface.
func (env *localEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}

// setupLocalMongoService returns the cert and key if there was no error.
func (env *localEnviron) setupLocalMongoService() ([]byte, []byte, error) {
	journalDir := filepath.Join(env.config.mongoDir(), "journal")
	logger.Debugf("create mongo journal dir: %v", journalDir)
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return nil, nil, err
	}

	logger.Debugf("generate server cert")
	cert, key, err := env.config.GenerateStateServerCertAndKey()
	if err != nil {
		logger.Errorf("failed to generate server cert: %v", err)
		return nil, nil, err
	}
	if err := ioutil.WriteFile(
		env.config.configFile("server.pem"),
		append(cert, key...),
		0600); err != nil {
		logger.Errorf("failed to write server.pem: %v", err)
		return nil, nil, err
	}

	mongo := upstart.MongoUpstartService(
		env.mongoServiceName(),
		env.config.rootDir(),
		env.config.mongoDir(),
		env.config.StatePort())
	mongo.InitDir = upstartScriptLocation
	logger.Infof("installing service %s to %s", env.mongoServiceName(), mongo.InitDir)
	if err := mongo.Install(); err != nil {
		logger.Errorf("could not install mongo service: %v", err)
		return nil, nil, err
	}
	return cert, key, nil
}

func (env *localEnviron) setupLocalMachineAgent(cons constraints.Value, possibleTools tools.List) error {
	dataDir := env.config.rootDir()
	// unpack the first tools into the agent dir.
	agentTools := possibleTools[0]
	logger.Debugf("tools: %#v", agentTools)
	// brutally abuse our knowledge of storage to directly open the file
	toolsUrl, err := url.Parse(agentTools.URL)
	if err != nil {
		return err
	}
	toolsLocation := filepath.Join(env.config.storageDir(), toolsUrl.Path)
	logger.Infof("tools location: %v", toolsLocation)
	toolsFile, err := os.Open(toolsLocation)
	defer toolsFile.Close()
	// Again, brutally abuse our knowledge here.

	// The tools that possible bootstrap tools are based on the
	// default series in the config.  However we are running potentially on a
	// different series.  When the machine agent is started, it will be
	// looking based on the current series, so we need to override the series
	// returned in the tools to be the current series.
	agentTools.Version.Series = version.Current.Series
	err = agenttools.UnpackTools(dataDir, agentTools, toolsFile)

	machineId := "0" // Always machine 0
	tag := names.MachineTag(machineId)

	// make sure we create the symlink so we have it for the upstart config to use
	if _, err := agenttools.ChangeAgentTools(dataDir, tag, agentTools.Version); err != nil {
		logger.Errorf("could not create tools directory symlink: %v", err)
		return err
	}

	toolsDir := agenttools.ToolsDir(dataDir, tag)

	logDir := env.config.logDir()
	machineEnvironment := map[string]string{
		"USER": env.config.user,
		"HOME": osenv.Home(),
	}
	agentService := upstart.MachineAgentUpstartService(
		env.machineAgentServiceName(),
		toolsDir, dataDir, logDir, tag, machineId, machineEnvironment)

	agentService.InitDir = upstartScriptLocation
	logger.Infof("installing service %s to %s", env.machineAgentServiceName(), agentService.InitDir)
	if err := agentService.Install(); err != nil {
		logger.Errorf("could not install machine agent service: %v", err)
		return err
	}
	return nil
}

func (env *localEnviron) findBridgeAddress(networkBridge string) (string, error) {
	return getAddressForInterface(networkBridge)
}

func (env *localEnviron) writeBootstrapAgentConfFile(secret string, cert, key []byte) (agent.Config, error) {
	tag := names.MachineTag("0")
	passwordHash := utils.UserPasswordHash(secret, utils.CompatSalt)
	// We don't check the existance of the CACert here as if it wasn't set, we
	// wouldn't get this far.
	cfg := env.config.Config
	caCert, _ := cfg.CACert()
	agentValues := map[string]string{
		agent.ProviderType:      env.config.Type(),
		agent.StorageDir:        env.config.storageDir(),
		agent.StorageAddr:       env.config.storageAddr(),
		agent.SharedStorageDir:  env.config.sharedStorageDir(),
		agent.SharedStorageAddr: env.config.sharedStorageAddr(),
	}
	// NOTE: the state address HAS to be localhost, otherwise the mongo
	// initialization fails.  There is some magic code somewhere in the mongo
	// connection code that treats connections from localhost as special, and
	// will raise unauthorized errors during the initialization if the caller
	// is not connected from localhost.
	stateAddress := fmt.Sprintf("localhost:%d", cfg.StatePort())
	apiAddress := fmt.Sprintf("localhost:%d", cfg.APIPort())
	config, err := agent.NewStateMachineConfig(
		agent.StateMachineConfigParams{
			AgentConfigParams: agent.AgentConfigParams{
				DataDir:        env.config.rootDir(),
				Tag:            tag,
				Password:       passwordHash,
				Nonce:          state.BootstrapNonce,
				StateAddresses: []string{stateAddress},
				APIAddresses:   []string{apiAddress},
				CACert:         caCert,
				Values:         agentValues,
			},
			StateServerCert: cert,
			StateServerKey:  key,
			StatePort:       cfg.StatePort(),
			APIPort:         cfg.APIPort(),
		})
	if err != nil {
		return nil, err
	}
	if err := config.Write(); err != nil {
		logger.Errorf("failed to write bootstrap agent file: %v", err)
		return nil, err
	}
	return config, nil
}

func (env *localEnviron) initializeState(agentConfig agent.Config, cons constraints.Value) error {
	bootstrapCfg, err := environs.BootstrapConfig(env.config.Config)
	if err != nil {
		return err
	}
	st, m, err := agentConfig.InitializeState(bootstrapCfg, agent.BootstrapMachineConfig{
		Constraints: cons,
		Jobs: []state.MachineJob{
			state.JobManageEnviron,
			state.JobManageState,
		},
		InstanceId: bootstrapInstanceId,
	}, state.DialOpts{
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return err
	}
	defer st.Close()
	addr, err := env.findBridgeAddress(env.config.networkBridge())
	if err != nil {
		return fmt.Errorf("failed to get bridge address: %v", err)
	}
	err = m.SetAddresses([]instance.Address{{
		NetworkScope: instance.NetworkPublic,
		Type:         instance.HostName,
		Value:        "localhost",
	}, {
		NetworkScope: instance.NetworkCloudLocal,
		Type:         instance.Ipv4Address,
		Value:        addr,
	}})
	if err != nil {
		return fmt.Errorf("cannot set addresses on bootstrap instance: %v", err)
	}
	return nil
}
