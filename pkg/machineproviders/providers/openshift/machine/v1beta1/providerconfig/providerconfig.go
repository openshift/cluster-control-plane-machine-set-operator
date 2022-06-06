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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// errMismatchedPlatformTypes is an error used when two provider configs
	// are being compared but are from different platform types.
	errMismatchedPlatformTypes = errors.New("mistmatched platform types")

	// errUnsupportedPlatformType is an error used when an unknown platform
	// type is configured within the failure domain config.
	errUnsupportedPlatformType = errors.New("unsupported platform type")

	// errUnknownProviderConfigType is an error used when provider type
	// cannot be deduced from providerSpec object kind.
	errUnknownProviderConfigType = errors.New("unknown provider config type")
)

// ProviderConfig is an interface that allows external code to interact
// with provider configuration across different platform types.
type ProviderConfig interface {
	// InjectFailureDomain is used to inject a failure domain into the ProviderConfig.
	// The returned ProviderConfig will be a copy of the current ProviderConfig with
	// the new failure domain injected.
	InjectFailureDomain(failuredomain.FailureDomain) (ProviderConfig, error)

	// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
	ExtractFailureDomain() failuredomain.FailureDomain

	// Equal compares two ProviderConfigs to determine whether or not they are equal.
	Equal(ProviderConfig) (bool, error)

	// RawConfig marshalls the configuration into a JSON byte slice.
	RawConfig() ([]byte, error)

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

// InjectFailureDomain is used to inject a failure domain into the ProviderConfig.
// The returned ProviderConfig will be a copy of the current ProviderConfig with
// the new failure domain injected.
func (p providerConfig) InjectFailureDomain(fd failuredomain.FailureDomain) (ProviderConfig, error) {
	newConfig := p

	switch p.platformType {
	case configv1.AWSPlatformType:
		newConfig.aws = p.AWS().InjectFailureDomain(fd.AWS())
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, p.platformType)
	}

	return newConfig, nil
}

// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
func (p providerConfig) ExtractFailureDomain() failuredomain.FailureDomain {
	switch p.platformType {
	case configv1.AWSPlatformType:
		return failuredomain.NewAWSFailureDomain(p.AWS().ExtractFailureDomain())
	default:
		return nil
	}
}

// Equal compares two ProviderConfigs to determine whether or not they are equal.
func (p providerConfig) Equal(other ProviderConfig) (bool, error) {
	if p.platformType != other.Type() {
		return false, errMismatchedPlatformTypes
	}

	switch p.platformType {
	case configv1.AWSPlatformType:
		return reflect.DeepEqual(p.aws.providerConfig, other.AWS().providerConfig), nil
	default:
		return false, errUnsupportedPlatformType
	}
}

// RawConfig marshalls the configuration into a JSON byte slice.
func (p providerConfig) RawConfig() ([]byte, error) {
	var (
		rawConfig []byte
		err       error
	)

	switch p.platformType {
	case configv1.AWSPlatformType:
		rawConfig, err = json.Marshal(p.aws.providerConfig)
	default:
		return nil, errUnsupportedPlatformType
	}

	if err != nil {
		return nil, fmt.Errorf("could not marshal provider config: %w", err)
	}

	return rawConfig, nil
}

// Type returns the platform type of the provider config.
func (p providerConfig) Type() configv1.PlatformType {
	return p.platformType
}

// AWS returns the AWSProviderConfig if the platform type is AWS.
func (p providerConfig) AWS() AWSProviderConfig {
	return p.aws
}

// getPlatformTypeFromProviderSpecKind determines machine platform from providerSpec kind.
func getPlatformTypeFromProviderSpecKind(kind string) (configv1.PlatformType, bool) {
	var providerSpecKindToPlatformType = map[string]configv1.PlatformType{
		"AWSMachineProviderConfig":     configv1.AWSPlatformType,
		"AzureMachineProviderSpec":     configv1.AzurePlatformType,
		"GCPMachineProviderSpec":       configv1.GCPPlatformType,
		"OpenStackMachineProviderSpec": configv1.OpenStackPlatformType,
	}

	platformType, ok := providerSpecKindToPlatformType[kind]

	return platformType, ok
}

// getPlatformType extracts the platform type from the Machine template.
// This can either be gathered from the platform type within the template failure domains,
// or if that isn't present, by inspecting the providerSpec kind and inferring from there
// what the configured platform type is.
func getPlatformType(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (configv1.PlatformType, error) {
	platformType := tmpl.FailureDomains.Platform
	if platformType != "" {
		return platformType, nil
	}

	// Simple type for unmarshalling providerSpec kind.
	type providerSpecKind struct {
		metav1.TypeMeta `json:",inline"`
	}

	providerSpec := providerSpecKind{}
	if err := json.Unmarshal(tmpl.Spec.ProviderSpec.Value.Raw, &providerSpec); err != nil {
		return "", fmt.Errorf("could not unmarshal provider spec: %w", err)
	}

	var ok bool
	if platformType, ok = getPlatformTypeFromProviderSpecKind(providerSpec.Kind); !ok {
		return "", fmt.Errorf("%w: %s", errUnknownProviderConfigType, providerSpec.Kind)
	}

	return platformType, nil
}
