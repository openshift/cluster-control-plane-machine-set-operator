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

package providers

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("MachineProviders", func() {
	usEast1aSubnet := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1a",
				},
			},
		},
	}

	usEast1bSubnet := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1b",
				},
			},
		},
	}

	usEast1cSubnet := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1c",
				},
			},
		},
	}

	usEast1aProviderSpecBuilder := resourcebuilder.AWSProviderSpec().
		WithAvailabilityZone("us-east-1a").
		WithSecurityGroups([]machinev1beta1.AWSResourceReference{
			{
				Filters: []machinev1beta1.Filter{
					{
						Name:   "tag:Name",
						Values: []string{"subnet-us-east-1a"},
					},
				},
			},
		}).WithSubnet(usEast1aSubnet)

	usEast1bProviderSpecBuilder := resourcebuilder.AWSProviderSpec().
		WithAvailabilityZone("us-east-1b").
		WithSecurityGroups([]machinev1beta1.AWSResourceReference{
			{
				Filters: []machinev1beta1.Filter{
					{
						Name:   "tag:Name",
						Values: []string{"subnet-us-east-1b"},
					},
				},
			},
		}).WithSubnet(usEast1bSubnet)

	usEast1cProviderSpecBuilder := resourcebuilder.AWSProviderSpec().
		WithAvailabilityZone("us-east-1c").
		WithSecurityGroups([]machinev1beta1.AWSResourceReference{
			{
				Filters: []machinev1beta1.Filter{
					{
						Name:   "tag:Name",
						Values: []string{"subnet-us-east-1c"},
					},
				},
			},
		}).WithSubnet(usEast1cSubnet)

	usEast1aFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1a").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1a"},
				},
			},
		})

	usEast1bFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1b").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1b"},
				},
			},
		})

	usEast1cFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1c").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1c"},
				},
			},
		})

	Context("NewMachineProvider", func() {
		var cpmsBuilder resourcebuilder.ControlPlaneMachineSetBuilder
		var logger test.TestLogger
		var namespaceName string

		const invalidCPMSType = machinev1.ControlPlaneMachineSetMachineType("invalid")

		BeforeEach(func() {
			By("Setting up a namespace for the test")
			ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			namespaceName = ns.GetName()

			cpmsBuilder = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName)

			logger = test.NewTestLogger()
		})

		AfterEach(func() {
			test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
				&machinev1beta1.Machine{},
			)
		})

		Context("With an invalid machine type", func() {
			var provider machineproviders.MachineProvider
			var err error

			BeforeEach(func() {
				cpms := cpmsBuilder.Build()
				cpms.Spec.Template.MachineType = invalidCPMSType

				provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpms)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(fmt.Errorf("%w: %s", errUnexpectedMachineType, invalidCPMSType)))
			})

			It("returns a nil machine provider", func() {
				Expect(provider).To(BeNil())
			})

			It("does not log", func() {
				Expect(logger.Entries()).To(BeEmpty())
			})
		})

		Context("With duplicate failure domains", func() {
			BeforeEach(func() {
				awsProviderSpec := resourcebuilder.AWSProviderSpec()

				cpmsBuilder = cpmsBuilder.WithMachineTemplateBuilder(
					resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(
						awsProviderSpec,
					).WithFailureDomainsBuilder(
						resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders( // here is the duplication of the failure domains
							usEast1aFailureDomainBuilder,
							usEast1aFailureDomainBuilder,
							usEast1bFailureDomainBuilder,
							usEast1cFailureDomainBuilder,
						),
					),
				)

				// We create a happy path so that the construction is successful.
				// More detailed error cases will happen in the machine provider tests themselves.
				By("Creating some master machines")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				for i, ps := range []resourcebuilder.AWSProviderSpecBuilder{usEast1aProviderSpecBuilder, usEast1bProviderSpecBuilder, usEast1cProviderSpecBuilder} {
					machine := machineBuilder.WithName(fmt.Sprintf("master-%d", i)).WithProviderSpecBuilder(ps).Build()
					Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				}
			})

			Context("with a valid template, but with duplicate failure domains", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpmsBuilder.Build())
				})

				It("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns an OpenShift Machine v1beta1 implementation of the machine provider", func() {
					typ := reflect.TypeOf(provider)
					Expect(typ.String()).To(Equal("*v1beta1.openshiftMachineProvider"))
				})

				It("logs based on the machine info mappings", func() {
					Expect(logger.Entries()).To(ConsistOf(
						test.LogEntry{
							Level: 4,
							KeysAndValues: []interface{}{
								"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
									0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
									1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
									2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								}),
							},
							Message: "Mapped provided failure domains",
						},
					))
				})
			})
		})

		Context("With an OpenShift Machine v1beta1 machine type", func() {
			BeforeEach(func() {
				awsProviderSpec := resourcebuilder.AWSProviderSpec()

				cpmsBuilder = cpmsBuilder.WithMachineTemplateBuilder(
					resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(
						awsProviderSpec,
					).WithFailureDomainsBuilder(
						resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aFailureDomainBuilder,
							usEast1bFailureDomainBuilder,
							usEast1cFailureDomainBuilder,
						),
					),
				)

				// We create a happy path so that the construction is successful.
				// More detailed error cases will happen in the machine provider tests themselves.
				By("Creating some master machines")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				for i, ps := range []resourcebuilder.AWSProviderSpecBuilder{usEast1aProviderSpecBuilder, usEast1bProviderSpecBuilder, usEast1cProviderSpecBuilder} {
					machine := machineBuilder.WithName(fmt.Sprintf("master-%d", i)).WithProviderSpecBuilder(ps).Build()
					Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				}
			})

			Context("with a valid template", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {

					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpmsBuilder.Build())
				})

				It("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns an OpenShift Machine v1beta1 implementation of the machine provider", func() {
					typ := reflect.TypeOf(provider)
					Expect(typ.String()).To(Equal("*v1beta1.openshiftMachineProvider"))
				})

				It("logs based on the machine info mappings", func() {
					Expect(logger.Entries()).To(ConsistOf(
						test.LogEntry{
							Level: 4,
							KeysAndValues: []interface{}{
								"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
									0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
									1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
									2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								}),
							},
							Message: "Mapped provided failure domains",
						},
					))
				})
			})

			Context("with an invalid template", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					cpms := cpmsBuilder.Build()
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine = nil

					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpms)
				})

				It("returns an error", func() {
					Expect(err).To(MatchError("error constructing machines_v1beta1_machine_openshift_io machine provider: cannot initialise machines_v1beta1_machine_openshift_io provider with empty config"))
				})

				It("returns a nil machine provider", func() {
					Expect(provider).To(BeNil())
				})

				It("does not log", func() {
					Expect(logger.Entries()).To(BeEmpty())
				})
			})
		})
	})
})
