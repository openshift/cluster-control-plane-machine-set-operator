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
	"github.com/onsi/gomega/types"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	"github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
)

// stringPtr returns a pointer to the string.
func stringPtr(s string) *string {
	return &s
}

var _ = Describe("Provider Config", func() {
	Context("NewProviderConfigFromMachineTemplate", func() {
		type providerConfigTableInput struct {
			failureDomainsBuilder resourcebuilder.OpenShiftMachineV1Beta1FailureDomainsBuilder
			modifyTemplate        func(tmpl *machinev1.ControlPlaneMachineSetTemplate)
			providerSpecBuilder   resourcebuilder.RawExtensionBuilder
			providerConfigMatcher types.GomegaMatcher
			expectedPlatformType  configv1.PlatformType
			expectedError         error
		}

		var logger testutils.TestLogger

		BeforeEach(func() {
			logger = testutils.NewTestLogger()
		})

		DescribeTable("should extract the config", func(in providerConfigTableInput) {
			tmpl := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithFailureDomainsBuilder(in.failureDomainsBuilder).
				WithProviderSpecBuilder(in.providerSpecBuilder).
				BuildTemplate()

			if in.modifyTemplate != nil {
				// Modify the template to allow injection of errors where the resource builder does not.
				in.modifyTemplate(&tmpl)
			}

			providerConfig, err := NewProviderConfigFromMachineTemplate(logger.Logger(), *tmpl.OpenShiftMachineV1Beta1Machine)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
				return
			}
			Expect(err).ToNot(HaveOccurred())

			Expect(providerConfig.Type()).To(Equal(in.expectedPlatformType))
			Expect(providerConfig).To(in.providerConfigMatcher)
		},
			Entry("with missing provider spec on unknown platform type", providerConfigTableInput{
				modifyTemplate: func(in *machinev1.ControlPlaneMachineSetTemplate) {
					// The platform type should be inferred from here first.
					in.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform = configv1.PlatformType("unknown")
				},
				expectedError: errNilProviderSpec,
			}),
			Entry("with an AWS config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.AWSFailureDomains(),
				providerSpecBuilder:   machinev1beta1resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField("AWS().Config()", *machinev1beta1resourcebuilder.AWSProviderSpec().Build()),
			}),
			Entry("with an AWS config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1beta1resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField("AWS().Config()", *machinev1beta1resourcebuilder.AWSProviderSpec().Build()),
			}),
			Entry("with an Azure config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AzurePlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.AzureFailureDomains(),
				providerSpecBuilder:   machinev1beta1resourcebuilder.AzureProviderSpec(),
				providerConfigMatcher: HaveField("Azure().Config()", *machinev1beta1resourcebuilder.AzureProviderSpec().Build()),
			}),
			Entry("with an Azure config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AzurePlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1beta1resourcebuilder.AzureProviderSpec(),
				providerConfigMatcher: HaveField("Azure().Config()", *machinev1beta1resourcebuilder.AzureProviderSpec().Build()),
			}),
			Entry("with a GCP config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.GCPPlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.GCPFailureDomains(),
				providerSpecBuilder:   machinev1beta1resourcebuilder.GCPProviderSpec(),
				providerConfigMatcher: HaveField("GCP().Config()", *machinev1beta1resourcebuilder.GCPProviderSpec().Build()),
			}),
			Entry("with a GCP config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.GCPPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1beta1resourcebuilder.GCPProviderSpec(),
				providerConfigMatcher: HaveField("GCP().Config()", *machinev1beta1resourcebuilder.GCPProviderSpec().Build()),
			}),
			Entry("with an OpenStack config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.OpenStackPlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.OpenStackFailureDomains(),
				providerSpecBuilder:   machinev1beta1resourcebuilder.OpenStackProviderSpec(),
				providerConfigMatcher: HaveField("OpenStack().Config()", *machinev1beta1resourcebuilder.OpenStackProviderSpec().Build()),
			}),
			Entry("with an OpenStack config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.OpenStackPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1beta1resourcebuilder.OpenStackProviderSpec(),
				providerConfigMatcher: HaveField("OpenStack().Config()", *machinev1beta1resourcebuilder.OpenStackProviderSpec().Build()),
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
				Expect(pc).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(pc).To(HaveField(in.matchPath, Equal(in.matchExpectation)))
			}

		},
			Entry("with nil failure domain", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: nil,
				expectedError: errNilFailureDomain,
			}),
			Entry("with empty failure domain", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: failuredomain.NewAWSFailureDomain(
					machinev1.AWSFailureDomain{},
				),
				expectedError:    nil,
				matchPath:        "AWS().Config().Placement.AvailabilityZone",
				matchExpectation: "",
			}),
			Entry("when keeping an AWS availability zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: failuredomain.NewAWSFailureDomain(
					machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				),
				matchPath:        "AWS().Config().Placement.AvailabilityZone",
				matchExpectation: "us-east-1a",
			}),
			Entry("when changing an AWS availability zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				failureDomain: failuredomain.NewAWSFailureDomain(
					machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").Build(),
				),
				matchPath:        "AWS().Config().Placement.AvailabilityZone",
				matchExpectation: "us-east-1b",
			}),
			Entry("when keeping an Azure availability zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("1").Build(),
					},
				},
				failureDomain: failuredomain.NewAzureFailureDomain(
					machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				),
				matchPath:        "Azure().Config().Zone",
				matchExpectation: stringPtr("1"),
			}),
			Entry("when changing an Azure zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("1").Build(),
					},
				},
				failureDomain: failuredomain.NewAzureFailureDomain(
					machinev1resourcebuilder.AzureFailureDomain().WithZone("2").Build(),
				),
				matchPath:        "Azure().Config().Zone",
				matchExpectation: stringPtr("2"),
			}),
			Entry("when keeping a GCP zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				failureDomain: failuredomain.NewGCPFailureDomain(
					machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				),
				matchPath:        "GCP().Config().Zone",
				matchExpectation: "us-central1-a",
			}),
			Entry("when changing a GCP zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				failureDomain: failuredomain.NewGCPFailureDomain(
					machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-b").Build(),
				),
				matchPath:        "GCP().Config().Zone",
				matchExpectation: "us-central1-b",
			}),
			Entry("when keeping an OpenStack compute availability zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az0").Build(),
					},
				},
				failureDomain: failuredomain.NewOpenStackFailureDomain(
					machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").Build(),
				),
				matchPath:        "OpenStack().Config().AvailabilityZone",
				matchExpectation: "nova-az0",
			}),
			Entry("when keeping an OpenStack volume availability zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithRootVolume(&machinev1alpha1.RootVolume{
							VolumeType: "fast-az1",
							Zone:       "cinder-az1",
						}).Build(),
					},
				},
				failureDomain: failuredomain.NewOpenStackFailureDomain(
					machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&machinev1.RootVolume{
						AvailabilityZone: "cinder-az1",
						VolumeType:       "fast-az1",
					}).Build(),
				),
				matchPath:        "OpenStack().Config().RootVolume.Zone",
				matchExpectation: "cinder-az1",
			}),
			Entry("when changing an OpenStack compute availability zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az0").Build(),
					},
				},
				failureDomain: failuredomain.NewOpenStackFailureDomain(
					machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az1").Build(),
				),
				matchPath:        "OpenStack().Config().AvailabilityZone",
				matchExpectation: "nova-az1",
			}),
			Entry("when changing an OpenStack volume availability zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithRootVolume(&machinev1alpha1.RootVolume{
							VolumeType: "fast-az0",
							Zone:       "cinder-az0",
						}).Build(),
					},
				},
				failureDomain: failuredomain.NewOpenStackFailureDomain(
					machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&machinev1.RootVolume{
						AvailabilityZone: "cinder-az1",
						VolumeType:       "fast-az1",
					}).Build(),
				),
				matchPath:        "OpenStack().Config().RootVolume.Zone",
				matchExpectation: "cinder-az1",
			}),
		)
	})

	Context("NewProviderConfigFromMachineSpec", func() {
		type providerConfigTableInput struct {
			modifyMachine         func(tmpl *machinev1beta1.Machine)
			providerSpecBuilder   resourcebuilder.RawExtensionBuilder
			providerConfigMatcher types.GomegaMatcher
			expectedPlatformType  configv1.PlatformType
			expectedError         error
		}

		var logger testutils.TestLogger

		BeforeEach(func() {
			logger = testutils.NewTestLogger()
		})

		DescribeTable("should extract the config", func(in providerConfigTableInput) {
			machine := machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(in.providerSpecBuilder).Build()

			if in.modifyMachine != nil {
				in.modifyMachine(machine)
			}

			providerConfig, err := NewProviderConfigFromMachineSpec(logger.Logger(), machine.Spec)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
				return
			}
			Expect(err).ToNot(HaveOccurred())

			Expect(providerConfig.Type()).To(Equal(in.expectedPlatformType))
			Expect(providerConfig).To(in.providerConfigMatcher)
		},
			Entry("with nil provider spec", providerConfigTableInput{
				modifyMachine: func(in *machinev1beta1.Machine) {
					in.Spec.ProviderSpec.Value = nil
				},
				providerSpecBuilder: machinev1beta1resourcebuilder.AWSProviderSpec(),
				expectedError:       errNilProviderSpec,
			}),
			Entry("with an AWS config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField("AWS().Config()", *machinev1beta1resourcebuilder.AWSProviderSpec().Build()),
			}),
			Entry("with an Azure config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AzurePlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.AzureProviderSpec(),
				providerConfigMatcher: HaveField("Azure().Config()", *machinev1beta1resourcebuilder.AzureProviderSpec().Build()),
			}),
			Entry("with a GCP config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.GCPPlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.GCPProviderSpec(),
				providerConfigMatcher: HaveField("GCP().Config()", *machinev1beta1resourcebuilder.GCPProviderSpec().Build()),
			}),
			Entry("with an OpenStack config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.OpenStackPlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.OpenStackProviderSpec(),
				providerConfigMatcher: HaveField("OpenStack().Config()", *machinev1beta1resourcebuilder.OpenStackProviderSpec().Build()),
			}),
		)
	})

	Context("ExtractFailureDomainsFromMachines", func() {

		type extractFailureDomainsFromMachinesTableInput struct {
			machines               []machinev1beta1.Machine
			expectedError          error
			expectedFailureDomains []failuredomain.FailureDomain
		}

		var logger testutils.TestLogger

		BeforeEach(func() {
			logger = testutils.NewTestLogger()
		})

		awsSubnet := machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name: "tag:Name",
					Values: []string{
						"aws-subnet-12345678",
					},
				},
			},
		}

		DescribeTable("should correctly extract the failure domains", func(in extractFailureDomainsFromMachinesTableInput) {
			failureDomains, err := ExtractFailureDomainsFromMachines(logger.Logger(), in.machines)

			if in.expectedError != nil {
				Expect(err).To(Equal(MatchError(in.expectedError)))
			}

			Expect(failureDomains).To(Equal(in.expectedFailureDomains))
		},
			Entry("when there are no machines", extractFailureDomainsFromMachinesTableInput{
				machines:               []machinev1beta1.Machine{},
				expectedError:          nil,
				expectedFailureDomains: []failuredomain.FailureDomain{},
			}),
			Entry("with machines", extractFailureDomainsFromMachinesTableInput{
				machines: []machinev1beta1.Machine{
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1c")).Build(),
				},
				expectedError: nil,
				expectedFailureDomains: []failuredomain.FailureDomain{
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(awsSubnet).Build()),
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(awsSubnet).Build()),
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(awsSubnet).Build()),
				},
			}),
			Entry("with machines that duplicate failure domains", extractFailureDomainsFromMachinesTableInput{
				machines: []machinev1beta1.Machine{
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1c")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b")).Build(),
					*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1c")).Build(),
				},
				expectedError: nil,
				expectedFailureDomains: []failuredomain.FailureDomain{
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(awsSubnet).Build()),
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(awsSubnet).Build()),
					failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(awsSubnet).Build()),
				},
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

		rootVolume := &machinev1alpha1.RootVolume{
			VolumeType: "fast-az2",
			Zone:       "cinder-az2",
		}

		DescribeTable("should correctly extract the failure domain", func(in extractFailureDomainTableInput) {
			fd := in.providerConfig.ExtractFailureDomain()

			Expect(fd).To(Equal(in.expectedFailureDomain))
		},
			Entry("with an AWS us-east-1a failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").WithSubnet(convertAWSResourceReferenceV1ToV1Beta1(&filterSubnet)).Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewAWSFailureDomain(
					machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(filterSubnet).Build(),
				),
			}),
			Entry("with an AWS us-east-1b failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b").WithSubnet(convertAWSResourceReferenceV1ToV1Beta1(&filterSubnet)).Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewAWSFailureDomain(
					machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(filterSubnet).Build(),
				),
			}),
			Entry("with an Azure 2 failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("2").Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewAzureFailureDomain(
					machinev1resourcebuilder.AzureFailureDomain().WithZone("2").Build(),
				),
			}),
			Entry("with a GCP us-central1-a failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewGCPFailureDomain(
					machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				),
			}),
			Entry("with an OpenStack az2 failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithRootVolume(rootVolume).WithZone("nova-az2").Build(),
					},
				},
				expectedFailureDomain: failuredomain.NewOpenStackFailureDomain(
					machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az2").WithRootVolume(&machinev1.RootVolume{
						AvailabilityZone: "cinder-az2",
						VolumeType:       "fast-az2",
					}).Build(),
				),
			}),
			Entry("with a VSphere dummy failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				expectedFailureDomain: failuredomain.NewGenericFailureDomain(),
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

		rootVolume := &machinev1alpha1.RootVolume{
			VolumeType: "fast-az0",
			Zone:       "cinder-az0",
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
			Entry("with nil provider config", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
				},
				comparePC:     nil,
				expectedEqual: false,
			}),
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
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched AWS configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1b").Build(),
					},
				},
				expectedEqual: false,
			}),
			Entry("with matching Azure configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("2").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("2").Build(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched Azure configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("1").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("2").Build(),
					},
				},
				expectedEqual: false,
			}),
			Entry("with matching GCP configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched GCP configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-central1-b").Build(),
					},
				},
				expectedEqual: false,
			}),
			Entry("with matching OpenStack configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az0").WithRootVolume(rootVolume).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az0").WithRootVolume(rootVolume).Build(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched OpenStack configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az0").WithRootVolume(rootVolume).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az1").WithRootVolume(rootVolume).Build(),
					},
				},
				expectedEqual: false,
			}),
			Entry("with matching Generic configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched spec using Generic configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().WithTemplate("different-template").BuildRawExtension(),
					},
				},
				expectedEqual: false,
			}),
			Entry("with mis-matched platform type using Generic configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.BareMetalPlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				expectedEqual: false,
				expectedError: errMismatchedPlatformTypes,
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
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().Build(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.AWSProviderSpec().BuildRawExtension().Raw,
			}),
			Entry("with an Azure config", rawConfigTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().Build(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.AzureProviderSpec().BuildRawExtension().Raw,
			}),
			Entry("with a GCP config", rawConfigTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().Build(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.GCPProviderSpec().BuildRawExtension().Raw,
			}),
			Entry("with an OpenStack config", rawConfigTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.OpenStackPlatformType,
					openstack: OpenStackProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.OpenStackProviderSpec().Build(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.OpenStackProviderSpec().BuildRawExtension().Raw,
			}),
			Entry("with a VSphere config", rawConfigTableInput{
				providerConfig: providerConfig{
					platformType: configv1.VSpherePlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension().Raw,
			}),
		)
	})

})
