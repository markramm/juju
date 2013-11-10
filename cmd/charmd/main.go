// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"launchpad.net/goyaml"

	"github.com/jameinel/juju/store"
)

func main() {
	err := serve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type config struct {
	MongoURL string `yaml:"mongo-url"`
	APIAddr  string `yaml:"api-addr"`
}

func readConfig(path string, conf interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening config file: %v", err)
	}
	data, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading config file: %v", err)
	}
	err = goyaml.Unmarshal(data, conf)
	if err != nil {
		return fmt.Errorf("processing config file: %v", err)
	}
	return nil
}

func serve() error {
	var confPath string
	if len(os.Args) == 2 {
		if _, err := os.Stat(os.Args[1]); err == nil {
			confPath = os.Args[1]
		}
	}
	if confPath == "" {
		return fmt.Errorf("usage: %s <config path>", filepath.Base(os.Args[0]))
	}
	var conf config
	err := readConfig(confPath, &conf)
	if err != nil {
		return err
	}
	if conf.MongoURL == "" || conf.APIAddr == "" {
		return fmt.Errorf("missing mongo-url or api-addr in config file")
	}
	s, err := store.Open(conf.MongoURL)
	if err != nil {
		return err
	}
	defer s.Close()
	server, err := store.NewServer(s)
	if err != nil {
		return err
	}
	return http.ListenAndServe(conf.APIAddr, server)
}
