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

package resourcebuilder

import (
	"encoding/json"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AzureProviderSpec creates a new Azure machine config builder.
func AzureProviderSpec() AzureProviderSpecBuilder {
	return AzureProviderSpecBuilder{
		Zone:   "1",
		VMSize: "Standard_D4s_v3",
	}
}

// AzureProviderSpecBuilder is used to build a Azure machine config object.
type AzureProviderSpecBuilder struct {
	Zone   string
	VMSize string
}

// Build builds a new Azure machine config based on the configuration provided.
func (m AzureProviderSpecBuilder) Build() *machinev1beta1.AzureMachineProviderSpec {
	return &machinev1beta1.AzureMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "machine.openshift.io/v1beta1",
			Kind:       "AzureMachineProviderSpec",
		},
		UserDataSecret: &v1.SecretReference{
			Name: "worker-user-data",
		},
		CredentialsSecret: &v1.SecretReference{
			Name:      "azure-cloud-credentials",
			Namespace: "openshift-machine-api",
		},
		Location: "test-location",
		Vnet:     "vnet-12345678",
		VMSize:   m.VMSize,
		Image: machinev1beta1.Image{
			ResourceID: "/resourceGroups/test-rg/providers/Microsoft.Compute/images/test-image",
		},
		OSDisk: machinev1beta1.OSDisk{
			DiskSettings: machinev1beta1.DiskSettings{},
			DiskSizeGB:   128,
			ManagedDisk: machinev1beta1.OSDiskManagedDiskParameters{
				StorageAccountType: "Premium_LRS",
			},
			OSType: "Linux",
		},
		NetworkResourceGroup:  "network-resource-group-12345678",
		PublicLoadBalancer:    "public-load-balancer-12345678",
		PublicIP:              false,
		ResourceGroup:         "resource-group-12345678",
		Zone:                  &m.Zone,
		AcceleratedNetworking: true,
		Subnet:                "subnet-12345678",
	}
}

// BuildRawExtension builds a new Azure machine config based on the configuration provided.
func (m AzureProviderSpecBuilder) BuildRawExtension() *runtime.RawExtension {
	providerConfig := m.Build()

	raw, err := json.Marshal(providerConfig)
	if err != nil {
		// As we are building the input to json.Marshal, this should never happen.
		panic(err)
	}

	return &runtime.RawExtension{
		Raw: raw,
	}
}

// WithZone sets the availabilityZone for the Azure machine config builder.
func (m AzureProviderSpecBuilder) WithZone(az string) AzureProviderSpecBuilder {
	m.Zone = az
	return m
}

// WithVMSize sets the VMSize (Instance type) for the Azure machine config builder.
func (m AzureProviderSpecBuilder) WithVMSize(vmSize string) AzureProviderSpecBuilder {
	m.VMSize = vmSize
	return m
}
