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
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
)

var (
	vsphereInfrastructureWithFailureDomains = configv1resourcebuilder.Infrastructure().AsVSphereWithFailureDomains("vsphere-test", nil).Build()
	nutanixInfrastructureWithFailureDomains = configv1resourcebuilder.Infrastructure().AsNutanixWithFailureDomains("nutanix-test", nil).Build()
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
			infrastructure        *configv1.Infrastructure
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

			providerConfig, err := NewProviderConfigFromMachineTemplate(logger.Logger(), *tmpl.OpenShiftMachineV1Beta1Machine, in.infrastructure)
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
					in.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1.FailureDomains{
						Platform: configv1.PlatformType("unknown"),
					}
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
			Entry("with an vSphere config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.VSpherePlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.VSphereFailureDomains(),
				providerSpecBuilder:   machinev1beta1resourcebuilder.VSphereProviderSpec(),
				infrastructure:        configv1resourcebuilder.Infrastructure().AsVSphere("test-vsphere").Build(),
				providerConfigMatcher: HaveField("VSphere().Config()", *machinev1beta1resourcebuilder.VSphereProviderSpec().Build()),
			}),
			Entry("with an vSphere config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.VSpherePlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1beta1resourcebuilder.VSphereProviderSpec(),
				infrastructure:        configv1resourcebuilder.Infrastructure().AsVSphere("test-vsphere").Build(),
				providerConfigMatcher: HaveField("VSphere().Config()", *machinev1beta1resourcebuilder.VSphereProviderSpec().Build()),
			}),
			Entry("with a Nutanix config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.NutanixPlatformType,
				failureDomainsBuilder: machinev1resourcebuilder.NutanixFailureDomains(),
				providerSpecBuilder:   machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0"),
				infrastructure:        nutanixInfrastructureWithFailureDomains,
				providerConfigMatcher: HaveField("Nutanix().Config()", *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build()),
			}),
			Entry("with a Nutanix config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.NutanixPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder(),
				infrastructure:        configv1resourcebuilder.Infrastructure().AsNutanix("nutanix-test").Build(),
				providerConfigMatcher: HaveField("Nutanix().Config()", *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().Build()),
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
				matchExpectation: "us-east-1a", // from the provider config
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
				matchExpectation: "1",
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
				matchExpectation: "2",
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
			Entry("when keeping a VSphere zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				failureDomain: failuredomain.NewVSphereFailureDomain(
					machinev1.VSphereFailureDomain(machinev1resourcebuilder.VSphereFailureDomain().WithZone("us-central1-a")),
				),
				matchPath:        "VSphere().ExtractFailureDomain().Name",
				matchExpectation: "us-central1-a",
			}),
			Entry("when changing a VSphere zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				failureDomain: failuredomain.NewVSphereFailureDomain(
					machinev1.VSphereFailureDomain(machinev1resourcebuilder.VSphereFailureDomain().WithZone("us-central1-b")),
				),
				matchPath:        "VSphere().ExtractFailureDomain().Name",
				matchExpectation: "us-central1-b",
			}),
			Entry("when keeping a Nutanix zone the same", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				failureDomain:    failuredomain.NewNutanixFailureDomain(machinev1resourcebuilder.NewNutanixFailureDomainBuilder().WithName("fd-pe0").Build()),
				matchPath:        "Nutanix().ExtractFailureDomain().Name",
				matchExpectation: "fd-pe0",
			}),
			Entry("when changing a Nutanix zone", injectFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				failureDomain:    failuredomain.NewNutanixFailureDomain(machinev1resourcebuilder.NewNutanixFailureDomainBuilder().WithName("fd-pe1").Build()),
				matchPath:        "Nutanix().ExtractFailureDomain().Name",
				matchExpectation: "fd-pe1",
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
			infrastructure        configv1.Infrastructure
			expectedError         error
		}

		var (
			logger                                     testutils.TestLogger
			vsphereInfrastructureWithoutFailureDomains = *configv1resourcebuilder.Infrastructure().AsVSphere("vsphere-test").Build()
			vsphereInfrastructureWithFailureDomains    = *configv1resourcebuilder.Infrastructure().AsVSphereWithFailureDomains("vsphere-test", nil).Build()
		)
		BeforeEach(func() {
			logger = testutils.NewTestLogger()
		})

		DescribeTable("should extract the config", func(in providerConfigTableInput) {
			machine := machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(in.providerSpecBuilder).Build()

			if in.modifyMachine != nil {
				in.modifyMachine(machine)
			}

			providerConfig, err := NewProviderConfigFromMachineSpec(logger.Logger(), machine.Spec, &in.infrastructure)
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
			Entry("with a VSphere config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.VSpherePlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.VSphereProviderSpec().AsControlPlaneMachineSetProviderSpec().WithInfrastructure(vsphereInfrastructureWithFailureDomains).WithZone("us-central1-a"),
				infrastructure:        vsphereInfrastructureWithFailureDomains,
				providerConfigMatcher: HaveField("VSphere().Config()", *machinev1beta1resourcebuilder.VSphereProviderSpec().AsControlPlaneMachineSetProviderSpec().WithInfrastructure(vsphereInfrastructureWithFailureDomains).WithZone("us-central1-a").Build()),
			}),
			Entry("with a VSphere config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.VSpherePlatformType,
				providerSpecBuilder:   machinev1beta1resourcebuilder.VSphereProviderSpec(),
				infrastructure:        vsphereInfrastructureWithoutFailureDomains,
				providerConfigMatcher: HaveField("VSphere().Config()", *machinev1beta1resourcebuilder.VSphereProviderSpec().Build()),
			}),
			Entry("with a Nutanix config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.NutanixPlatformType,
				providerSpecBuilder:   machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0"),
				infrastructure:        *nutanixInfrastructureWithFailureDomains,
				providerConfigMatcher: HaveField("Nutanix().Config()", *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build()),
			}),
			Entry("with a Nutanix config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.NutanixPlatformType,
				providerSpecBuilder:   machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder(),
				infrastructure:        *configv1resourcebuilder.Infrastructure().AsNutanix("nutanix-test").Build(),
				providerConfigMatcher: HaveField("Nutanix().Config()", *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().Build()),
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
			failureDomains, err := ExtractFailureDomainsFromMachines(logger.Logger(), in.machines, nil)

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
					machinev1resourcebuilder.AzureFailureDomain().WithZone("2").WithSubnet("cluster-subnet-12345678").Build(),
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
			Entry("with a VSphere us-central1-a failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				expectedFailureDomain: failuredomain.NewVSphereFailureDomain(
					machinev1.VSphereFailureDomain(machinev1resourcebuilder.VSphereFailureDomain().WithZone("us-central1-a")),
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
			Entry("with a VSphere failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				expectedFailureDomain: failuredomain.NewVSphereFailureDomain(machinev1.VSphereFailureDomain{
					Name: "us-central1-a",
				}),
			}),
			Entry("with a Nutanix fd-pe0 failure domain", extractFailureDomainTableInput{
				providerConfig: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				expectedFailureDomain: failuredomain.NewNutanixFailureDomain(machinev1resourcebuilder.NewNutanixFailureDomainBuilder().WithName("fd-pe0").Build()),
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
			Entry("with matching VSphere configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched VSphere configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-a").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.VSpherePlatformType,
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().WithZone("us-central1-b").Build(),
						infrastructure: vsphereInfrastructureWithFailureDomains,
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
			Entry("with matching Nutanix configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched Nutanix configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe1").Build(),
						infrastructure: nutanixInfrastructureWithFailureDomains,
					},
				},
				expectedEqual: false,
			}),
			Entry("with matching Generic configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.ExternalPlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.ExternalPlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				expectedEqual: true,
			}),
			Entry("with mis-matched spec using Generic configs", equalTableInput{
				basePC: &providerConfig{
					platformType: configv1.ExternalPlatformType,
					generic: GenericProviderConfig{
						providerSpec: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.ExternalPlatformType,
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

	Context("Diff", func() {
		type diffTableInput struct {
			basePC        ProviderConfig
			comparePC     ProviderConfig
			expectedDiff  []string
			expectedError error
		}

		DescribeTable("should return correct diff for provider configs", func(in diffTableInput) {
			diff, err := in.basePC.Diff(in.comparePC)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(diff).To(Equal(in.expectedDiff))
			}
		},
			Entry("with nil provider config", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.large").Build(),
					},
				},
				comparePC: nil,
			}),
			Entry("with mismatched platform types", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.large").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v4").Build(),
					},
				},
				expectedError: errMismatchedPlatformTypes,
			}),
			Entry("with identical AWS configs", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.large").WithAvailabilityZone("us-east-1a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.large").WithAvailabilityZone("us-east-1a").Build(),
					},
				},
			}),
			Entry("with different AWS instance types", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.large").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithInstanceType("m5.xlarge").Build(),
					},
				},
				expectedDiff: []string{"InstanceType: m5.large != m5.xlarge"},
			}),
			Entry("with different AWS AMIs", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAMI(machinev1beta1.AWSResourceReference{ID: stringPtr("ami-12345678")}).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AWSPlatformType,
					aws: AWSProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AWSProviderSpec().WithAMI(machinev1beta1.AWSResourceReference{ID: stringPtr("ami-87654321")}).Build(),
					},
				},
				// expectedDiff is nil because AMI fields are ignored in diff comparison
				expectedDiff: nil,
			}),
			Entry("with identical Azure configs", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v4").WithZone("1").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v4").WithZone("1").Build(),
					},
				},
			}),
			Entry("with different Azure VM sizes", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v4").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithVMSize("Standard_D8s_v3").Build(),
					},
				},
				expectedDiff: []string{"VMSize: Standard_D8s_v4 != Standard_D8s_v3"},
			}),
			Entry("with different Azure images", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithImage(machinev1beta1.Image{Publisher: "RedHat", Offer: "RHEL", SKU: "8-LVM", Version: "8.4.2021040911"}).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.AzurePlatformType,
					azure: AzureProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.AzureProviderSpec().WithImage(machinev1beta1.Image{Publisher: "RedHat", Offer: "RHEL", SKU: "8-LVM", Version: "8.5.2021111016"}).Build(),
					},
				},
				// expectedDiff is nil because Image fields are ignored in diff comparison
				expectedDiff: nil,
			}),
			Entry("with identical GCP configs", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithMachineType("n1-standard-4").WithZone("us-central1-a").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithMachineType("n1-standard-4").WithZone("us-central1-a").Build(),
					},
				},
			}),
			Entry("with different GCP machine types", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithMachineType("n1-standard-4").Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithMachineType("n1-standard-8").Build(),
					},
				},
				expectedDiff: []string{"MachineType: n1-standard-4 != n1-standard-8"},
			}),
			Entry("with different GCP disk images", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
							{
								AutoDelete: true,
								Boot:       true,
								SizeGB:     100,
								Type:       "pd-standard",
								Image:      "projects/rhcos-cloud/global/images/rhcos-416-92-202301311551-0-gcp-x86-64",
							},
						}).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
							{
								AutoDelete: true,
								Boot:       true,
								SizeGB:     100,
								Type:       "pd-standard",
								Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
							},
						}).Build(),
					},
				},
				// expectedDiff is nil because disk Image fields are ignored in diff comparison
				expectedDiff: nil,
			}),
			Entry("with different GCP disk sizes", diffTableInput{
				basePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
							{
								AutoDelete: true,
								Boot:       true,
								SizeGB:     100,
								Type:       "pd-standard",
								Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
							},
						}).Build(),
					},
				},
				comparePC: &providerConfig{
					platformType: configv1.GCPPlatformType,
					gcp: GCPProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
							{
								AutoDelete: true,
								Boot:       true,
								SizeGB:     200,
								Type:       "pd-standard",
								Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
							},
						}).Build(),
					},
				},
				expectedDiff: []string{"Disks.slice[0].SizeGB: 100 != 200"},
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
					vsphere: VSphereProviderConfig{
						providerConfig: *machinev1beta1resourcebuilder.VSphereProviderSpec().Build(),
					},
				},
				expectedOut: machinev1beta1resourcebuilder.VSphereProviderSpec().BuildRawExtension().Raw,
			}),
			Entry("with a Nutanix config", rawConfigTableInput{
				providerConfig: providerConfig{
					platformType: configv1.NutanixPlatformType,
					nutanix: NutanixProviderConfig{
						providerConfig: *machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").Build(),
					},
				},
				expectedOut: machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder().WithFailureDomains(nutanixInfrastructureWithFailureDomains.Spec.PlatformSpec.Nutanix.FailureDomains).WithFailureDomainName("fd-pe0").BuildRawExtension().Raw,
			}),
		)
	})
})
