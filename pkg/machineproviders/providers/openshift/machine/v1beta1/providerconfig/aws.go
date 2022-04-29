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
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AWSProviderConfig holds the provider spec of an AWS Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type AWSProviderConfig struct {
	providerConfig machinev1beta1.AWSMachineProviderConfig
}

// InjectFailureDomain returns a new AWSProviderConfig configured with the failure domain
// information provided.
func (a AWSProviderConfig) InjectFailureDomain(fd machinev1.AWSFailureDomain) AWSProviderConfig {
	return a
}

// ExtractFailureDomain returns an AWSFailureDomain based on the failure domain
// information stored within the AWSProviderConfig.
func (a AWSProviderConfig) ExtractFailureDomain() machinev1.AWSFailureDomain {
	return machinev1.AWSFailureDomain{}
}

// Config returns the stored AWSMachineProviderConfig.
func (a AWSProviderConfig) Config() machinev1beta1.AWSMachineProviderConfig {
	return a.providerConfig
}

// newAWSProviderConfig creates an AWSProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent
// an AWSMachineProviderConfig.
func newAWSProviderConfig(raw *runtime.RawExtension) (ProviderConfig, error) {
	// TODO: replace this with actual logic to create the provider config from the raw extension.
	// This is here as a dummy to keep the linter happy.
	return providerConfig{}, nil
}
