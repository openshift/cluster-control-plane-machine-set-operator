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
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
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

	// errUnsupportedProviderConfigType is an error used when provider spec is nil.
	errNilProviderSpec = errors.New("provider spec is nil")

	// errNilFailureDomain is an error used when when nil value is present and failure domain is expected.
	errNilFailureDomain = errors.New("failure domain is nil")
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

	// Azure returns the AzureProviderConfig if the platform type is Azure.
	Azure() AzureProviderConfig

	// GCP returns the GCPProviderConfig if the platform type is GCP.
	GCP() GCPProviderConfig
}

// NewProviderConfigFromMachineTemplate creates a new ProviderConfig from the provided machine template.
func NewProviderConfigFromMachineTemplate(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (ProviderConfig, error) {
	platformType, err := getPlatformTypeFromMachineTemplate(tmpl)
	if err != nil {
		return nil, fmt.Errorf("could not determine platform type: %w", err)
	}

	return newProviderConfigFromProviderSpec(tmpl.Spec.ProviderSpec, platformType)
}

// NewProviderConfigFromMachine creates a new ProviderConfig from the provided machine object.
func NewProviderConfigFromMachine(machine machinev1beta1.Machine) (ProviderConfig, error) {
	platformType, err := getPlatformTypeFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("could not determine platform type: %w", err)
	}

	return newProviderConfigFromProviderSpec(machine.Spec.ProviderSpec, platformType)
}

func newProviderConfigFromProviderSpec(providerSpec machinev1beta1.ProviderSpec, platformType configv1.PlatformType) (ProviderConfig, error) {
	switch platformType {
	case configv1.AWSPlatformType:
		return newAWSProviderConfig(providerSpec.Value)
	case configv1.AzurePlatformType:
		return newAzureProviderConfig(providerSpec.Value)
	case configv1.GCPPlatformType:
		return newGCPProviderConfig(providerSpec.Value)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, platformType)
	}
}

// providerConfig is an implementation of the ProviderConfig interface.
type providerConfig struct {
	platformType configv1.PlatformType
	aws          AWSProviderConfig
	azure        AzureProviderConfig
	gcp          GCPProviderConfig
}

// InjectFailureDomain is used to inject a failure domain into the ProviderConfig.
// The returned ProviderConfig will be a copy of the current ProviderConfig with
// the new failure domain injected.
func (p providerConfig) InjectFailureDomain(fd failuredomain.FailureDomain) (ProviderConfig, error) {
	if fd == nil {
		return nil, errNilFailureDomain
	}

	newConfig := p

	switch p.platformType {
	case configv1.AWSPlatformType:
		newConfig.aws = p.AWS().InjectFailureDomain(fd.AWS())
	case configv1.AzurePlatformType:
		newConfig.azure = p.Azure().InjectFailureDomain(fd.Azure())
	case configv1.GCPPlatformType:
		newConfig.gcp = p.GCP().InjectFailureDomain(fd.GCP())
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
	case configv1.AzurePlatformType:
		return failuredomain.NewAzureFailureDomain(p.Azure().ExtractFailureDomain())
	case configv1.GCPPlatformType:
		return failuredomain.NewGCPFailureDomain(p.GCP().ExtractFailureDomain())
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
	case configv1.AzurePlatformType:
		return reflect.DeepEqual(p.azure.providerConfig, other.Azure().providerConfig), nil
	case configv1.GCPPlatformType:
		return reflect.DeepEqual(p.gcp.providerConfig, other.GCP().providerConfig), nil
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
	case configv1.AzurePlatformType:
		rawConfig, err = json.Marshal(p.azure.providerConfig)
	case configv1.GCPPlatformType:
		rawConfig, err = json.Marshal(p.gcp.providerConfig)
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

// Azure returns the AzureProviderConfig if the platform type is Azure.
func (p providerConfig) Azure() AzureProviderConfig {
	return p.azure
}

// GCP returns the GCPProviderConfig if the platform type is GCP.
func (p providerConfig) GCP() GCPProviderConfig {
	return p.gcp
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

// getPlatformTypeFromMachineTemplate extracts the platform type from the Machine template.
// This can either be gathered from the platform type within the template failure domains,
// or if that isn't present, by inspecting the providerSpec kind and inferring from there
// what the configured platform type is.
func getPlatformTypeFromMachineTemplate(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (configv1.PlatformType, error) {
	platformType := tmpl.FailureDomains.Platform
	if platformType != "" {
		return platformType, nil
	}

	return getPlatformTypeFromProviderSpec(tmpl.Spec.ProviderSpec)
}

// getPlatformTypeFromProviderSpec determines machine platform from the providerSpec.
// The providerSpec object's kind field is unmarshalled and the platform type is inferred from it.
func getPlatformTypeFromProviderSpec(providerSpec machinev1beta1.ProviderSpec) (configv1.PlatformType, error) {
	var platformType configv1.PlatformType
	// Simple type for unmarshalling providerSpec kind.
	type providerSpecKind struct {
		metav1.TypeMeta `json:",inline"`
	}

	providerKind := providerSpecKind{}

	if providerSpec.Value == nil {
		return "", errNilProviderSpec
	}

	if err := json.Unmarshal(providerSpec.Value.Raw, &providerKind); err != nil {
		return "", fmt.Errorf("could not unmarshal provider spec: %w", err)
	}

	var ok bool
	if platformType, ok = getPlatformTypeFromProviderSpecKind(providerKind.Kind); !ok {
		return "", fmt.Errorf("%w: %s", errUnknownProviderConfigType, providerKind.Kind)
	}

	return platformType, nil
}

// ExtractFailureDomainsFromMachines creates list of FailureDomains extracted from the provided list of machines.
func ExtractFailureDomainsFromMachines(machines []machinev1beta1.Machine) ([]failuredomain.FailureDomain, error) {
	machineFailureDomains := []failuredomain.FailureDomain{}

	for _, machine := range machines {
		providerconfig, err := NewProviderConfigFromMachine(machine)
		if err != nil {
			return nil, fmt.Errorf("error getting failure domain from machine %s: %w", machine.Name, err)
		}

		machineFailureDomains = append(machineFailureDomains, providerconfig.ExtractFailureDomain())
	}

	return machineFailureDomains, nil
}

// ExtractFailureDomainFromMachine FailureDomain extracted from the provided machine.
func ExtractFailureDomainFromMachine(machine machinev1beta1.Machine) (failuredomain.FailureDomain, error) {
	providerConfig, err := NewProviderConfigFromMachine(machine)
	if err != nil {
		return nil, fmt.Errorf("error getting failure domain from machine %s: %w", machine.Name, err)
	}

	return providerConfig.ExtractFailureDomain(), nil
}
