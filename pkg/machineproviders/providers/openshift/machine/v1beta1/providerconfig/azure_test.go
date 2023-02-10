/*
Copyright 2023 Red Hat, Inc.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("Azure Provider Config", func() {
	var providerConfig AzureProviderConfig

	zone1 := "1"
	zone2 := "2"

	BeforeEach(func() {
		machineProviderConfig := machinev1beta1resourcebuilder.AzureProviderSpec().
			WithZone(zone1).
			Build()

		providerConfig = AzureProviderConfig{
			providerConfig: *machineProviderConfig,
		}
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.AzureFailureDomain().
				WithZone(zone1).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig AzureProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.AzureFailureDomain().
				WithZone(zone2).
				Build()

			changedProviderConfig = providerConfig.InjectFailureDomain(changedFailureDomain)
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := machinev1resourcebuilder.AzureFailureDomain().
					WithZone(zone2).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.AzureFailureDomain().
					WithZone(zone1).
					Build()

				Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
			})
		})
	})

	Context("newAzureProviderConfig", func() {
		var providerConfig ProviderConfig
		var expectedAzureConfig machinev1beta1.AzureMachineProviderSpec

		BeforeEach(func() {
			configBuilder := machinev1beta1resourcebuilder.AzureProviderSpec()
			expectedAzureConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			var err error
			providerConfig, err = newAzureProviderConfig(rawConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the type to Azure", func() {
			Expect(providerConfig.Type()).To(Equal(configv1.AzurePlatformType))
		})

		It("returns the correct Azure config", func() {
			Expect(providerConfig.Azure()).ToNot(BeNil())
			Expect(providerConfig.Azure().Config()).To(Equal(expectedAzureConfig))
		})
	})
})
