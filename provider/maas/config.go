// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jameinel/juju/environs/config"
	"github.com/jameinel/juju/schema"
)

var configFields = schema.Fields{
	"maas-server": schema.String(),
	// maas-oauth is a colon-separated triplet of:
	// consumer-key:resource-token:resource-secret
	"maas-oauth": schema.String(),
	// maas-agent-name is an optional UUID to group the instances
	// acquired from MAAS, to support multiple environments per MAAS user.
	"maas-agent-name": schema.String(),
}
var configDefaults = schema.Defaults{
	// For backward-compatibility, maas-agent-name is the empty string
	// by default. However, new environments should all use a UUID.
	"maas-agent-name": "",
}

type maasEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *maasEnvironConfig) maasServer() string {
	return cfg.attrs["maas-server"].(string)
}

func (cfg *maasEnvironConfig) maasOAuth() string {
	return cfg.attrs["maas-oauth"].(string)
}

func (cfg *maasEnvironConfig) maasAgentName() string {
	if uuid, ok := cfg.attrs["maas-agent-name"].(string); ok {
		return uuid
	}
	return ""
}

func (prov maasEnvironProvider) newConfig(cfg *config.Config) (*maasEnvironConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(maasEnvironConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

var errMalformedMaasOAuth = errors.New("malformed maas-oauth (3 items separated by colons)")

func (prov maasEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating MAAS specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	if oldCfg != nil {
		oldAttrs := oldCfg.UnknownAttrs()
		if validated["maas-agent-name"] != oldAttrs["maas-agent-name"] {
			return nil, fmt.Errorf("cannot change maas-agent-name")
		}
	}
	envCfg := new(maasEnvironConfig)
	envCfg.Config = cfg
	envCfg.attrs = validated
	server := envCfg.maasServer()
	serverURL, err := url.Parse(server)
	if err != nil || serverURL.Scheme == "" || serverURL.Host == "" {
		return nil, fmt.Errorf("malformed maas-server URL '%v': %s", server, err)
	}
	oauth := envCfg.maasOAuth()
	if strings.Count(oauth, ":") != 2 {
		return nil, errMalformedMaasOAuth
	}
	return cfg.Apply(envCfg.attrs)
}
