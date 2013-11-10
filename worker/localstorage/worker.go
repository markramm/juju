// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage

import (
	"net"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"github.com/jameinel/juju/agent"
	"github.com/jameinel/juju/environs/filestorage"
	"github.com/jameinel/juju/environs/httpstorage"
	"github.com/jameinel/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.localstorage")

type storageWorker struct {
	config agent.Config
	tomb   tomb.Tomb
}

func NewWorker(config agent.Config) worker.Worker {
	w := &storageWorker{config: config}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.waitForDeath())
	}()
	return w
}

// Kill implements worker.Worker.Kill.
func (s *storageWorker) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (s *storageWorker) Wait() error {
	return s.tomb.Wait()
}

func (s *storageWorker) serveStorage(storageAddr, storageDir string, config *config) (net.Listener, error) {
	authenticated := len(config.caCertPEM) > 0 && len(config.caKeyPEM) > 0
	scheme := "http://"
	if authenticated {
		scheme = "https://"
	}
	logger.Infof("serving storage from %s to %s%s", storageDir, scheme, storageAddr)
	storage, err := filestorage.NewFileStorageWriter(storageDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return nil, err
	}
	if authenticated {
		return httpstorage.ServeTLS(
			storageAddr,
			storage,
			config.caCertPEM,
			config.caKeyPEM,
			config.hostnames,
			config.authkey,
		)
	}
	return httpstorage.Serve(storageAddr, storage)
}

func (s *storageWorker) waitForDeath() error {
	config, err := loadConfig(s.config)
	if err != nil {
		logger.Errorf("error loading config: %v", err)
		return err
	}

	storageListener, err := s.serveStorage(config.storageAddr, config.storageDir, config)
	if err != nil {
		logger.Errorf("error with local storage: %v", err)
		return err
	}
	defer storageListener.Close()

	if config.sharedStorageAddr != "" && config.sharedStorageDir != "" {
		sharedStorageListener, err := s.serveStorage(config.sharedStorageAddr, config.sharedStorageDir, config)
		if err != nil {
			logger.Errorf("error with local storage: %v", err)
			return err
		}
		defer sharedStorageListener.Close()
	} else {
		logger.Infof("no shared storage: dir=%q addr=%q", config.sharedStorageDir, config.sharedStorageAddr)
	}

	logger.Infof("storage routines started, awaiting death")

	<-s.tomb.Dying()

	logger.Infof("dying, closing storage listeners")
	return tomb.ErrDying
}
