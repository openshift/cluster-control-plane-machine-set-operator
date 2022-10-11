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

package v1beta1

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("Failure Domain Mapping", func() {
	var namespaceName string

	cpmsBuilder := resourcebuilder.ControlPlaneMachineSet().WithReplicas(3)
	machineBuilder := resourcebuilder.Machine().AsMaster().WithLabel(machinev1beta1.MachineClusterIDLabel, "cpms-cluster-test-id")

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

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()
	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&machinev1beta1.Machine{},
		)
	})

	Context("mappingMachineIndexesToFailureDomains", func() {
		type mappingMachineIndexesTableInput struct {
			cpmsBuilder     resourcebuilder.ControlPlaneMachineSetInterface
			failureDomains  machinev1.FailureDomains
			machines        []*machinev1beta1.Machine
			expectedError   error
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedLogs    []test.LogEntry
		}

		DescribeTable("should map failure domains to indexes", func(in mappingMachineIndexesTableInput) {
			failureDomains, err := failuredomain.NewFailureDomains(in.failureDomains)
			Expect(err).ToNot(HaveOccurred())

			logger := test.NewTestLogger()

			cpms := in.cpmsBuilder.Build()
			// Make sure all resources use the right namespace.
			cpms.SetNamespace(namespaceName)

			for _, machine := range in.machines {
				machine.SetNamespace(namespaceName)
				machine.Finalizers = append(machine.Finalizers, "machine.machine.openshift.io")
				status := machine.Status.DeepCopy()
				Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				machine.Status = *status
				Expect(k8sClient.Status().Update(ctx, machine)).To(Succeed())
				// It is not possible to create machine with deletion timestamp directly. Create one without it first, then delete the machine.
				if pointer.StringDeref(status.Phase, "") == "Deleting" {
					Expect(k8sClient.Delete(ctx, machine)).To(Succeed())
				}
			}

			originalCPMS := cpms.DeepCopy()

			mapping, err := mapMachineIndexesToFailureDomains(ctx, logger.Logger(), k8sClient, cpms, failureDomains)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(mapping).To(Equal(in.expectedMapping))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			Entry("with no failure domains defined, returns an empty mapping", mappingMachineIndexesTableInput{
				cpmsBuilder:    cpmsBuilder,
				failureDomains: machinev1.FailureDomains{},
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedError:   errNoFailureDomains,
				expectedMapping: nil,
				expectedLogs: []test.LogEntry{
					{
						Level:   4,
						Message: "No failure domains provided",
					},
				},
			}),
			Entry("with three failure domains matching three machines in order (a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
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
				},
			}),
			Entry("with three failure domains matching three machines in a order (b,c,a)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching three machines in a order (b,a,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching five machines in order (a,b,c,a,b)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching five machines in order (b,c,a,c,b)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching five machines in order (b,a,c,c,a)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with a machine in an unrecognised failure domain (failure domains ordered a,b)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()), // The extra failure domain must be the first alphabetically in this case.
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"failureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Ignoring unknown failure domain",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with a machine in an unrecognised failure domain (failure domains ordered b,a)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()), // The extra failure domain must be the first alphabetically in this case.
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"failureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Ignoring unknown failure domain",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with multiple machines in unrecognised failure domains ", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 1,
							"failureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1b]}]}}",
						},
						Message: "Ignoring unknown failure domain",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"failureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Ignoring unknown failure domain",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with multiple machines in the same failure domain", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()), // The missing failure domain fills in for the last Machine alphabetically.
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 1,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1b]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
						},
						Message: "Failure domain changed for index",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with deleting machine and maximum replicas", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithPhase("Deleting").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Failure domain changed for index",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with a machine name does not indicate its index (a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-a").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machine", "machine-a",
						},
						Message: "Ignoring machine in failure domain mapping with unexpected name",
					},
					{
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
				},
			}),
			Entry("with a machine name does not indicate its index (b,c,a)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-a").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machine", "machine-a",
						},
						Message: "Ignoring machine in failure domain mapping with unexpected name",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("when the machine mappings are unbalanced, should rebalance the failure domains (c,b,a,c,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 3,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
						},
						Message: "Failure domain changed for index",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
		)
	})

	Context("createBaseFailureDomainMapping", func() {
		type createBaseMappingTableInput struct {
			cpmsBuilder     resourcebuilder.ControlPlaneMachineSetInterface
			failureDomains  machinev1.FailureDomains
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedError   error
		}

		DescribeTable("should map the failure domains based on the replicas", func(in createBaseMappingTableInput) {
			failureDomains, err := failuredomain.NewFailureDomains(in.failureDomains)
			Expect(err).ToNot(HaveOccurred())

			cpms := in.cpmsBuilder.Build()
			mapping, err := createBaseFailureDomainMapping(cpms, failureDomains)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(mapping).To(Equal(in.expectedMapping))
		},
			Entry("with no replicas set", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(0),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedError: errReplicasRequired,
			}),
			Entry("with three replicas and three failure domains (order a,b,c)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("with three replicas and three failure domains (order b,c,a)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("with three replicas and three failure domains (order b,a,c)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("with three replicas and one failure domains", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
			}),
			Entry("with three replicas and two failure domains (order a,b)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
			}),
			Entry("with three replicas and two failure domains (order b,a)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
			}),
			Entry("with five replicas and three failure domains (order a,b,c)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
			}),
			Entry("with five replicas and three failure domains (order b,c,a)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
			}),
			Entry("with five replicas and two failure domains (order a,b)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
			}),
			Entry("with five replicas and two failure domains (order b,a)", createBaseMappingTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(5),
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1bFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
				).BuildFailureDomains(),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
			}),
		)
	})

	Context("createMachineMapping", func() {
		type machineMappingTableInput struct {
			cpmsBuilder     resourcebuilder.ControlPlaneMachineSetInterface
			machines        []*machinev1beta1.Machine
			expectedError   error
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedLogs    []test.LogEntry
		}

		DescribeTable("maps Machines based on their failure domain", func(in machineMappingTableInput) {
			logger := test.NewTestLogger()

			cpms := cpmsBuilder.Build()
			// Make sure all resources use the right namespace.
			cpms.SetNamespace(namespaceName)

			for _, machine := range in.machines {
				machine.SetNamespace(namespaceName)
				Expect(k8sClient.Create(ctx, machine)).To(Succeed())
			}

			originalCPMS := cpms.DeepCopy()

			mapping, err := createMachineMapping(ctx, logger.Logger(), k8sClient, cpms)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(mapping).To(Equal(in.expectedMapping))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			Entry("with machines in three failure domains (order a,b,c)", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with machines in three failure domains (order b,c,a)", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with machines with non-index names", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-a").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-c").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machine", "machine-a",
						},
						Message: "Ignoring machine in failure domain mapping with unexpected name",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machine", "machine-c",
						},
						Message: "Ignoring machine in failure domain mapping with unexpected name",
					},
				},
			}),
			Entry("with multiple machines in a failure domains", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with multiple machines in the same index in the same failure domain", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with multiple machines in the same index in different failure domains", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-0").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"oldMachine", "machine-0",
							"oldFaliureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
							"newerMachine", "machine-replacement-0",
							"newerFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1b]}]}}",
						},
						Message: "Conflicting failure domains found for the same index, relying on the newer machine",
					},
				},
			}),
			Entry("with machines not matched by the selector", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					resourcebuilder.Machine().AsWorker().WithName("machine-1").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					resourcebuilder.Machine().AsWorker().WithName("machine-2").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
		)
	})

	Context("reconcileMappings", func() {
		type reconcileMappingsTableInput struct {
			baseMapping     map[int32]failuredomain.FailureDomain
			machineMapping  map[int32]failuredomain.FailureDomain
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedLogs    []test.LogEntry
		}

		DescribeTable("should keep the machine indexes stable where possible", func(in reconcileMappingsTableInput) {
			// Run each test 10 times in an attempt to make sure the output is stable.
			for i := 0; i < 10; i++ {
				logger := test.NewTestLogger()

				mapping := reconcileMappings(logger.Logger(), in.baseMapping, in.machineMapping)

				Expect(mapping).To(Equal(in.expectedMapping))
				Expect(logger.Entries()).To(Equal(in.expectedLogs))
			}
		},
			Entry("when the mappings match", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when the mappings match but some failure domains are duplicated", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when the mappings differ, machines take precedence (order b,c,a)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when the mappings differ, machines take precedence (order b,a,c)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when the mappings differ, and failure domains are duplicated, machines take precedence (order b,a,a,c,b)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machine has a failure domain not in the base mapping", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"failureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Ignoring unknown failure domain",
					},
				},
			}),
			Entry("when the base mapping has a failure domain not in the machine mapping", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 2,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Failure domain changed for index",
					},
				},
			}),
			Entry("when a failure domain isn't represented in the machine mapping", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machine mapping is balanced in a different way to the base mapping, should not rebalance", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when the machine mappings are unbalanced, should rebalance the failure domains (c,b,a,c,c)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 3,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
						},
						Message: "Failure domain changed for index",
					},
				},
			}),
			Entry("when a machine has a an index not present in the base mapping, ignores the additional index", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
		)
	})
})
