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

package generate

import (
	"encoding/json"
	"fmt"

	"github.com/ghodss/yaml"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

func providerSpecFromRawExtension(rawExtension *runtime.RawExtension) (*machinev1beta1.AWSMachineProviderConfig, error) {
	if rawExtension == nil {
		return &machinev1beta1.AWSMachineProviderConfig{}, nil
	}

	spec := new(machinev1beta1.AWSMachineProviderConfig)
	if err := yaml.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %w", err)
	}

	return spec, nil
}

func rawExtensionFromProviderSpec(spec *machinev1beta1.AWSMachineProviderConfig) (*runtime.RawExtension, error) {
	if spec == nil {
		return &runtime.RawExtension{}, nil
	}

	var (
		rawBytes []byte
		err      error
	)

	if rawBytes, err = json.Marshal(spec); err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}
