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
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("Azure Provider Config", func() {
	var logger testutils.TestLogger

	var providerConfig AzureProviderConfig

	zone1 := "1"
	zone2 := "2"

	subnetZone1 := "subnet-zone-1"
	subnetZone2 := "subnet-zone-2"

	BeforeEach(func() {
		machineProviderConfig := machinev1beta1resourcebuilder.AzureProviderSpec().
			WithZone(zone1).
			WithSubnet(subnetZone1).
			Build()

		providerConfig = AzureProviderConfig{
			providerConfig: *machineProviderConfig,
		}

		logger = testutils.NewTestLogger()
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.AzureFailureDomain().
				WithZone(zone1).
				WithSubnet(subnetZone1).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig AzureProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.AzureFailureDomain().
				WithZone(zone2).
				WithSubnet(subnetZone2).
				Build()

			changedProviderConfig = providerConfig.InjectFailureDomain(changedFailureDomain)
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := machinev1resourcebuilder.AzureFailureDomain().
					WithZone(zone2).
					WithSubnet(subnetZone2).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.AzureFailureDomain().
					WithZone(zone1).
					WithSubnet(subnetZone1).
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
			providerConfig, err = newAzureProviderConfig(logger.Logger(), rawConfig)
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

	Context("Diff", func() {
		type diffTableInput struct {
			baseConfig    AzureProviderConfig
			compareConfig machinev1beta1.AzureMachineProviderSpec
			expectedDiff  []string
		}

		DescribeTable("should return correct diff", func(in diffTableInput) {
			diff := in.baseConfig.Diff(in.compareConfig)

			Expect(diff).To(Equal(in.expectedDiff))
		},
			Entry("with different images", diffTableInput{
				baseConfig: AzureProviderConfig{
					providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithImage(machinev1beta1.Image{Publisher: "RedHat", Offer: "RHEL", SKU: "8-LVM", Version: "8.4.2021040911"}).Build(),
				},
				compareConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithImage(machinev1beta1.Image{Publisher: "RedHat", Offer: "RHEL", SKU: "8-LVM", Version: "8.5.2021111016"}).Build(),
				// expectedDiff is nil because Image fields are ignored in diff comparison
				expectedDiff: nil,
			}),
			Entry("with different VM sizes", diffTableInput{
				baseConfig: AzureProviderConfig{
					providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v4").Build(),
				},
				compareConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v3").Build(),
				expectedDiff:  []string{"VMSize: Standard_D8s_v4 != Standard_D8s_v3"},
			}),
		)
	})
})
