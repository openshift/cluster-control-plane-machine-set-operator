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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("AWS Provider Config", func() {
	var providerConfig AWSProviderConfig

	var azUSEast1a = "us-east-1a"
	var machinev1SubnetUSEast1a = machinev1.AWSResourceReference{
		Type: machinev1.AWSFiltersReferenceType,
		Filters: &[]machinev1.AWSResourceFilter{
			{
				Name:   "tag:Name",
				Values: []string{"subnet-us-east-1a"},
			},
		},
	}
	var machinev1beta1SubnetUSEast1a = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name:   "tag:Name",
				Values: []string{"subnet-us-east-1a"},
			},
		},
	}

	var azUSEast1b = "us-east-1b"
	var machinev1SubnetUSEast1b = machinev1.AWSResourceReference{
		Type: machinev1.AWSFiltersReferenceType,
		Filters: &[]machinev1.AWSResourceFilter{
			{
				Name:   "tag:Name",
				Values: []string{"subnet-us-east-1b"},
			},
		},
	}
	var machinev1beta1SubnetUSEast1b = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name:   "tag:Name",
				Values: []string{"subnet-us-east-1b"},
			},
		},
	}

	BeforeEach(func() {
		machineProviderConfig := resourcebuilder.AWSProviderSpec().
			WithAvailabilityZone(azUSEast1a).
			WithSubnet(machinev1beta1SubnetUSEast1a).
			Build()

		providerConfig = AWSProviderConfig{
			providerConfig: *machineProviderConfig,
		}
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := resourcebuilder.AWSFailureDomain().
				WithAvailabilityZone(azUSEast1a).
				WithSubnet(machinev1SubnetUSEast1a).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig AWSProviderConfig

		BeforeEach(func() {
			changedFailureDomain := resourcebuilder.AWSFailureDomain().
				WithAvailabilityZone(azUSEast1b).
				WithSubnet(machinev1SubnetUSEast1b).
				Build()

			changedProviderConfig = providerConfig.InjectFailureDomain(changedFailureDomain)
		})

		It("stores the new subnet in the provider config", func() {
			Expect(changedProviderConfig.Config().Subnet).To(Equal(machinev1beta1SubnetUSEast1b))
		})

		It("does not modify the original provider config", func() {
			Expect(providerConfig.Config().Subnet).To(Equal(machinev1beta1SubnetUSEast1a))
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := resourcebuilder.AWSFailureDomain().
					WithAvailabilityZone(azUSEast1b).
					WithSubnet(machinev1SubnetUSEast1b).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := resourcebuilder.AWSFailureDomain().
					WithAvailabilityZone(azUSEast1a).
					WithSubnet(machinev1SubnetUSEast1a).
					Build()

				Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
			})
		})
	})

	Context("newAWSProviderConfig", func() {
		var providerConfig ProviderConfig
		var expectedAWSConfig machinev1beta1.AWSMachineProviderConfig

		BeforeEach(func() {
			configBuilder := resourcebuilder.AWSProviderSpec()
			expectedAWSConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			var err error
			providerConfig, err = newAWSProviderConfig(rawConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the type to AWS", func() {
			Expect(providerConfig.Type()).To(Equal(configv1.AWSPlatformType))
		})

		It("returns the correct AWS config", func() {
			Expect(providerConfig.AWS()).ToNot(BeNil())
			Expect(providerConfig.AWS().Config()).To(Equal(expectedAWSConfig))
		})
	})
})
