/*
Copyright 2022 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providerconfig

import (
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
)

var (
	// errUnsupportedPlatformType is an error used when an unknown platform
	// type is configured within the failure domain config.
	errUnsupportedPlatformType = errors.New("unsupported platform type")
)

// ProviderConfig is an interface that allows external code to interact
// with provider configuration across different platform types.
type ProviderConfig interface {
	// Type returns the platform type of the provider config.
	Type() configv1.PlatformType

	// AWS returns the AWSProviderConfig if the platform type is AWS.
	AWS() AWSProviderConfig
}

// NewProviderConfig creates a new ProviderConfig from the provided machine template.
func NewProviderConfig(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (ProviderConfig, error) {
	platformType, err := getPlatformType(tmpl)
	if err != nil {
		return nil, fmt.Errorf("could not determine platform type: %w", err)
	}

	switch platformType {
	case configv1.AWSPlatformType:
		return newAWSProviderConfig(tmpl.Spec.ProviderSpec.Value)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, platformType)
	}
}

// providerConfig is an implementation of the ProviderConfig interface.
type providerConfig struct {
	platformType configv1.PlatformType
	aws          AWSProviderConfig
}

// Type returns the platform type of the provider config.
func (p providerConfig) Type() configv1.PlatformType {
	return p.platformType
}

// AWS returns the AWSProviderConfig if the platform type is AWS.
func (p providerConfig) AWS() AWSProviderConfig {
	return p.aws
}

// getPlatformType extracts the platform type from the Machine template.
// This can either be gathered from the platform type within the template failure domains,
// or if that isn't present, by inspecting the providerSpec kind and inferring from there
// what the configured platform type is.
func getPlatformType(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (configv1.PlatformType, error) {
	return "", nil
}
