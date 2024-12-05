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

package providers

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/record"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
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

	usEast1aProviderSpecBuilder := machinev1beta1resourcebuilder.AWSProviderSpec().
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

	usEast1bProviderSpecBuilder := machinev1beta1resourcebuilder.AWSProviderSpec().
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

	usEast1cProviderSpecBuilder := machinev1beta1resourcebuilder.AWSProviderSpec().
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

	usEast1aFailureDomainBuilder := machinev1resourcebuilder.AWSFailureDomain().
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

	usEast1bFailureDomainBuilder := machinev1resourcebuilder.AWSFailureDomain().
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

	usEast1cFailureDomainBuilder := machinev1resourcebuilder.AWSFailureDomain().
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
		var cpmsBuilder machinev1resourcebuilder.ControlPlaneMachineSetBuilder
		var logger testutils.TestLogger
		var namespaceName string
		var recorder *record.FakeRecorder
		var opts MachineProviderOptions

		const invalidCPMSType = machinev1.ControlPlaneMachineSetMachineType("invalid")

		BeforeEach(func() {
			By("Setting up a namespace for the test")
			ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			namespaceName = ns.GetName()

			cpmsBuilder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName)

			logger = testutils.NewTestLogger()

			recorder = record.NewFakeRecorder(100)

			opts = MachineProviderOptions{
				AllowMachineNamePrefix: false,
			}
		})

		AfterEach(func() {
			testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
				&machinev1beta1.Machine{},
			)
		})

		Context("With an invalid machine type", func() {
			var provider machineproviders.MachineProvider
			var err error

			BeforeEach(func() {
				cpms := cpmsBuilder.Build()
				cpms.Spec.Template.MachineType = invalidCPMSType

				provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpms, opts)
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

		Context("With allowed machine name prefix", func() {
			var provider machineproviders.MachineProvider
			var err error

			BeforeEach(func() {
				cpms := cpmsBuilder.Build()
				opts.AllowMachineNamePrefix = true

				provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpms, opts)
			})

			It("does not return an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a machine provider", func() {
				Expect(provider).NotTo(BeNil())
			})
		})

		Context("With duplicate failure domains", func() {
			BeforeEach(func() {
				awsProviderSpec := machinev1beta1resourcebuilder.AWSProviderSpec()

				cpmsBuilder = cpmsBuilder.WithMachineTemplateBuilder(
					machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(
						awsProviderSpec,
					).WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders( // here is the duplication of the failure domains
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
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				for i, ps := range []machinev1beta1resourcebuilder.AWSProviderSpecBuilder{usEast1aProviderSpecBuilder, usEast1bProviderSpecBuilder, usEast1cProviderSpecBuilder} {
					machine := machineBuilder.WithName(fmt.Sprintf("master-%d", i)).WithProviderSpecBuilder(ps).Build()
					Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				}
			})

			Context("with a valid template, but with duplicate failure domains", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpmsBuilder.Build(), opts)
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
						testutils.LogEntry{
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
				awsProviderSpec := machinev1beta1resourcebuilder.AWSProviderSpec()

				cpmsBuilder = cpmsBuilder.WithMachineTemplateBuilder(
					machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(
						awsProviderSpec,
					).WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aFailureDomainBuilder,
							usEast1bFailureDomainBuilder,
							usEast1cFailureDomainBuilder,
						),
					),
				)

				// We create a happy path so that the construction is successful.
				// More detailed error cases will happen in the machine provider tests themselves.
				By("Creating some master machines")
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				for i, ps := range []machinev1beta1resourcebuilder.AWSProviderSpecBuilder{usEast1aProviderSpecBuilder, usEast1bProviderSpecBuilder, usEast1cProviderSpecBuilder} {
					machine := machineBuilder.WithName(fmt.Sprintf("master-%d", i)).WithProviderSpecBuilder(ps).Build()
					Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				}
			})

			Context("with a valid template", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {

					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpmsBuilder.Build(), opts)
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
						testutils.LogEntry{
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

			Context("with a valid template and allowed machine name prefix", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					opts.AllowMachineNamePrefix = true
					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpmsBuilder.Build(), opts)
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
						testutils.LogEntry{
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

					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, recorder, cpms, opts)
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
