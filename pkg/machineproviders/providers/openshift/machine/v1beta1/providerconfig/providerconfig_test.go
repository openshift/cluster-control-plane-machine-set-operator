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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("Provider Config", func() {
	Context("NewProviderConfig", func() {
		type providerConfigTableInput struct {
			failureDomainsBuilder resourcebuilder.OpenShiftMachineV1Beta1FailureDomainsBuilder
			modifyTemplate        func(tmpl *machinev1.ControlPlaneMachineSetTemplate)
			providerSpecBuilder   resourcebuilder.RawExtensionBuilder
			providerConfigMatcher types.GomegaMatcher
			expectedPlatformType  configv1.PlatformType
			expectedError         error
		}

		DescribeTable("should extract the config", func(in providerConfigTableInput) {
			tmpl := resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithFailureDomainsBuilder(in.failureDomainsBuilder).
				WithProviderSpecBuilder(in.providerSpecBuilder).
				BuildTemplate()

			if in.modifyTemplate != nil {
				// Modify the template to allow injection of errors where the resource builder does not.
				in.modifyTemplate(&tmpl)
			}

			providerConfig, err := NewProviderConfig(*tmpl.OpenShiftMachineV1Beta1Machine)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
				return
			}
			Expect(err).ToNot(HaveOccurred())

			Expect(providerConfig.Type()).To(Equal(in.expectedPlatformType))
			Expect(providerConfig).To(in.providerConfigMatcher)
		},
			Entry("with an invalid platform type", providerConfigTableInput{
				modifyTemplate: func(in *machinev1.ControlPlaneMachineSetTemplate) {
					// The platform type should be inferred from here first.
					in.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform = configv1.PlatformType("invalid")
				},
				expectedError: fmt.Errorf("%w: %s", errUnsupportedPlatformType, "invalid"),
			}),
			Entry("with an AWS config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: resourcebuilder.AWSFailureDomains(),
				providerSpecBuilder:   resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField("AWS().Config()", *resourcebuilder.AWSProviderSpec().Build()),
			}),
			Entry("with an AWS config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField("AWS().Config()", *resourcebuilder.AWSProviderSpec().Build()),
			}),
		)
	})

	Context("InjectFailureDomain", func() {
		type injectFailureDomainTableInput struct {
			providerConfig   ProviderConfig
			failureDomain    failuredomain.FailureDomain
			matchPath        string
			matchExpectation interface{}
			expectedError    error
		}

		DescribeTable("should inject the failure domain into the provider config", func(in injectFailureDomainTableInput) {
			pc, err := in.providerConfig.InjectFailureDomain(in.failureDomain)

			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(pc).To(HaveField(in.matchPath, Equal(in.matchExpectation)))
		},
			Entry("when keeping an AWS availability zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: failuredomain.NewAWSFailureDomain(
					resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				),
				matchPath:        "AWS().Config().Placement.AvailabilityZone",
				matchExpectation: "us-east-1a",
			}),
			Entry("when changing an AWS availability zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: failuredomain.NewAWSFailureDomain(
					resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").Build(),
				),
				matchPath:        "AWS().Config().Placement.AvailabilityZone",
				matchExpectation: "us-east-1b",
			}),
		)
	})

	Context("ExtractFailureDomain", func() {
		type extractFailureDomainTableInput struct {
			providerConfig        ProviderConfig
			expectedFailureDomain failuredomain.FailureDomain
		}
		filterSubnet := machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{{
				Name:   "tag:Name",
				Values: []string{"aws-subnet-12345678"},
			}},
		}

		DescribeTable("should correctly extract the failure domain", func(in extractFailureDomainTableInput) {
			fd := in.providerConfig.ExtractFailureDomain()

			Expect(fd).To(Equal(in.expectedFailureDomain))
		},
			Entry("with an AWS us-east-1a failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").WithSubnet(failuredomain.ConvertAWSResourceReferenceToBeta1(&filterSubnet)).Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewAWSFailureDomain(
					resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(filterSubnet).Build(),
				),
			}),
			Entry("with an AWS us-east-1b failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b").WithSubnet(failuredomain.ConvertAWSResourceReferenceToBeta1(&filterSubnet)).Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewAWSFailureDomain(
					resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(filterSubnet).Build(),
				),
			}),
		)
	})

	Context("Equal", func() {
		type equalTableInput struct {
			basePC        ProviderConfig
			comparePC     ProviderConfig
			expectedEqual bool
			expectedError error
		}

		DescribeTable("should compare provider configs", func(in equalTableInput) {
			equal, err := in.basePC.Equal(in.comparePC)

			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(equal).To(Equal(in.expectedEqual), "Equality of provider configs was not as expected")
		},
			Entry("with different platform types", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
				},
				expectedEqual: false,
				expectedError: errMismatchedPlatformTypes,
			}),
			Entry("with matching AWS configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched AWS configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b").Build(),
					},
				},
				expectedEqual: false,
			}),
		)
	})

	Context("RawConfig", func() {
		type rawConfigTableInput struct {
			providerConfig ProviderConfig
			expectedError  error
			expectedOut    []byte
		}

		DescribeTable("should marshal the correct config", func(in rawConfigTableInput) {
			out, err := in.providerConfig.RawConfig()

			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(out).To(Equal(in.expectedOut))
		},
			Entry("with an AWS config", rawConfigTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *resourcebuilder.AWSProviderSpec().Build(),
					},
				},
				expectedOut: resourcebuilder.AWSProviderSpec().BuildRawExtension().Raw,
			}),
		)
	})
})
