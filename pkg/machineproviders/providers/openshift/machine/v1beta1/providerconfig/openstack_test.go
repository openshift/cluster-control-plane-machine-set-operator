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
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("OpenStack Provider Config", func() {
	var logger testutils.TestLogger

	var providerConfig OpenStackProviderConfig
	var providerConfigWithVolumeType OpenStackProviderConfig

	novaZone1 := "nova-az1"
	novaZone2 := "nova-az2"
	cinderZone1 := "cinder-az1"
	volumeType1 := "volumetype-1"

	machinev1alpha1RootVolume1 := &machinev1alpha1.RootVolume{
		Zone: cinderZone1,
	}
	machinev1RootVolume1 := machinev1.RootVolume{
		AvailabilityZone: cinderZone1,
	}

	machinev1alpha1RootVolume2 := &machinev1alpha1.RootVolume{
		VolumeType: volumeType1,
	}
	machinev1RootVolume2 := machinev1.RootVolume{
		VolumeType: volumeType1,
	}

	BeforeEach(func() {
		machineProviderConfig := machinev1beta1resourcebuilder.OpenStackProviderSpec().
			WithZone(novaZone1).
			WithRootVolume(machinev1alpha1RootVolume1).
			Build()

		providerConfig = OpenStackProviderConfig{
			providerConfig: *machineProviderConfig,
		}

		providerConfig = OpenStackProviderConfig{
			providerConfig: *machineProviderConfig,
		}

		machineProviderConfigWithVolumeType := machinev1beta1resourcebuilder.OpenStackProviderSpec().
			WithZone(novaZone1).
			WithRootVolume(machinev1alpha1RootVolume2).
			Build()

		providerConfigWithVolumeType = OpenStackProviderConfig{
			providerConfig: *machineProviderConfigWithVolumeType,
		}

		logger = testutils.NewTestLogger()
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.OpenStackFailureDomain().
				WithComputeAvailabilityZone(novaZone1).
				WithRootVolume(&machinev1RootVolume1).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("ExtractFailureDomainWithVolumeType", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.OpenStackFailureDomain().
				WithComputeAvailabilityZone(novaZone1).
				WithRootVolume(&machinev1RootVolume2).
				Build()

			Expect(providerConfigWithVolumeType.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig OpenStackProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.OpenStackFailureDomain().
				WithComputeAvailabilityZone(novaZone2).
				WithRootVolume(&machinev1RootVolume1).
				Build()

			changedProviderConfig = providerConfig.InjectFailureDomain(changedFailureDomain)
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := machinev1resourcebuilder.OpenStackFailureDomain().
					WithComputeAvailabilityZone(novaZone2).
					WithRootVolume(&machinev1RootVolume1).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.OpenStackFailureDomain().
					WithComputeAvailabilityZone(novaZone1).
					WithRootVolume(&machinev1RootVolume1).
					Build()

				Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
			})
		})
	})

	Context("newOpenStackProviderConfig", func() {
		var providerConfig ProviderConfig
		var expectedOpenStackConfig machinev1alpha1.OpenstackProviderSpec

		BeforeEach(func() {
			configBuilder := machinev1beta1resourcebuilder.OpenStackProviderSpec()
			expectedOpenStackConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			var err error
			providerConfig, err = newOpenStackProviderConfig(logger.Logger(), rawConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the type to OpenStack", func() {
			Expect(providerConfig.Type()).To(Equal(configv1.OpenStackPlatformType))
		})

		It("returns the correct OpenStack config", func() {
			Expect(providerConfig.OpenStack()).ToNot(BeNil())
			Expect(providerConfig.OpenStack().Config()).To(Equal(expectedOpenStackConfig))
		})
	})
})
