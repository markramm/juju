// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"github.com/jameinel/juju/environs"
	"github.com/jameinel/juju/environs/simplestreams"
)

// SupportsCustomSources represents an environment that
// can host tools metadata at provider specific sources.
type SupportsCustomSources interface {
	GetToolsSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for
// simplestreams tools metadata. If env implements SupportsCustomSurces,
// the sources returned from that method will also be considered.
// The sources are configured to not use retries.
func GetMetadataSources(env environs.ConfigGetter) ([]simplestreams.DataSource, error) {
	return GetMetadataSourcesWithRetries(env, false)
}

// GetMetadataSourcesWithRetries returns the sources to use when looking for
// simplestreams tools metadata. If env implements SupportsCustomSurces,
// the sources returned from that method will also be considered.
// The sources are configured to use retries according to the value of allowRetry.
func GetMetadataSourcesWithRetries(env environs.ConfigGetter, allowRetry bool) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	config := env.Config()
	if userURL, ok := config.ToolsURL(); ok {
		verify := simplestreams.VerifySSLHostnames
		if !config.SSLHostnameVerification() {
			verify = simplestreams.NoVerifySSLHostnames
		}
		sources = append(sources, simplestreams.NewURLDataSource(userURL, verify))
	}
	if custom, ok := env.(SupportsCustomSources); ok {
		customSources, err := custom.GetToolsSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	if DefaultBaseURL != "" {
		sources = append(sources, simplestreams.NewURLDataSource(DefaultBaseURL, simplestreams.VerifySSLHostnames))
	}
	for _, source := range sources {
		source.SetAllowRetry(allowRetry)
	}
	return sources, nil
}
