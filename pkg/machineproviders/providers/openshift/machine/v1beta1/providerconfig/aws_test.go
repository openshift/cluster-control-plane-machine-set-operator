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
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("AWS Provider Config", func() {
	var logger testutils.TestLogger

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
		machineProviderConfig := machinev1beta1resourcebuilder.AWSProviderSpec().
			WithAvailabilityZone(azUSEast1a).
			WithSubnet(machinev1beta1SubnetUSEast1a).
			Build()

		providerConfig = AWSProviderConfig{
			providerConfig: *machineProviderConfig,
		}

		logger = testutils.NewTestLogger()
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.AWSFailureDomain().
				WithAvailabilityZone(azUSEast1a).
				WithSubnet(machinev1SubnetUSEast1a).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig AWSProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.AWSFailureDomain().
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
				expected := machinev1resourcebuilder.AWSFailureDomain().
					WithAvailabilityZone(azUSEast1b).
					WithSubnet(machinev1SubnetUSEast1b).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.AWSFailureDomain().
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
			configBuilder := machinev1beta1resourcebuilder.AWSProviderSpec()
			expectedAWSConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			var err error
			providerConfig, err = newAWSProviderConfig(logger.Logger(), rawConfig)
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

	Context("ConvertAWSResourceReference", func() {
		type convertAWSResourceReferenceInput struct {
			awsResourceV1    *machinev1.AWSResourceReference
			awsResourceBeta1 machinev1beta1.AWSResourceReference
		}

		idInput := convertAWSResourceReferenceInput{
			awsResourceBeta1: machinev1beta1.AWSResourceReference{
				ID: stringPtr("test-id"),
			},
			awsResourceV1: &machinev1.AWSResourceReference{
				Type: machinev1.AWSIDReferenceType,
				ID:   stringPtr("test-id"),
			},
		}

		arnInput := convertAWSResourceReferenceInput{
			awsResourceBeta1: machinev1beta1.AWSResourceReference{
				ARN: stringPtr("test-arn"),
			},
			awsResourceV1: &machinev1.AWSResourceReference{
				Type: machinev1.AWSARNReferenceType,
				ARN:  stringPtr("test-arn"),
			},
		}

		filterInput := convertAWSResourceReferenceInput{
			awsResourceBeta1: machinev1beta1.AWSResourceReference{
				Filters: []machinev1beta1.Filter{{
					Name:   "tag:Name",
					Values: []string{"aws-subnet-12345678"},
				}},
			},
			awsResourceV1: &machinev1.AWSResourceReference{
				Type: machinev1.AWSFiltersReferenceType,
				Filters: &[]machinev1.AWSResourceFilter{{
					Name:   "tag:Name",
					Values: []string{"aws-subnet-12345678"},
				}},
			},
		}

		nilInput := convertAWSResourceReferenceInput{
			awsResourceBeta1: machinev1beta1.AWSResourceReference{},
			awsResourceV1:    nil,
		}

		DescribeTable("converts correctly to V1", func(in convertAWSResourceReferenceInput) {
			Expect(in.awsResourceV1).To(Equal(convertAWSResourceReferenceV1Beta1ToV1(in.awsResourceBeta1)))
		},
			Entry("with ID", idInput),
			Entry("with ARN", arnInput),
			Entry("with Filter", filterInput),
			Entry("with Nil", nilInput),
		)

		DescribeTable("converts correctly to Beta1", func(in convertAWSResourceReferenceInput) {
			Expect(in.awsResourceBeta1).To(Equal(convertAWSResourceReferenceV1ToV1Beta1(in.awsResourceV1)))
		},
			Entry("with ID", idInput),
			Entry("with ARN", arnInput),
			Entry("with Filter", filterInput),
			Entry("with Nil", nilInput),
		)

		DescribeTable("is the same after back and forth conversion - V1", func(in convertAWSResourceReferenceInput) {
			converted := convertAWSResourceReferenceV1Beta1ToV1(convertAWSResourceReferenceV1ToV1Beta1(in.awsResourceV1))
			Expect(in.awsResourceV1).To(Equal(converted))
		},
			Entry("with ID", idInput),
			Entry("with ARN", arnInput),
			Entry("with Filter", filterInput),
			Entry("with Nil", nilInput),
		)

		DescribeTable("is the same after back and forth conversion - Beta1", func(in convertAWSResourceReferenceInput) {
			converted := convertAWSResourceReferenceV1ToV1Beta1(convertAWSResourceReferenceV1Beta1ToV1(in.awsResourceBeta1))
			Expect(in.awsResourceBeta1).To(Equal(converted))
		},
			Entry("with ID", idInput),
			Entry("with ARN", arnInput),
			Entry("with Filter", filterInput),
			Entry("with Nil", nilInput),
		)

	})
})
