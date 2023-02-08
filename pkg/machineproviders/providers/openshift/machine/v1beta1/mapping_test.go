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

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
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
			Entry("with duplicate failure domains at the beginning of the Control Plane Machine Set", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
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
			Entry("with duplicate failure domains at the end of the Control Plane Machine Set", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
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
			Entry("with duplicate failure domains in the middle of the Control Plane Machine Set", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
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
			Entry("with multiple duplicate failure domains in the Control Plane Machine Set", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
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
			Entry("when rebalancing indexes and an index only contains deleted machines", mappingMachineIndexesTableInput{
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
							"index", 0,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1b]}]}}",
						},
						Message: "Failure domain changed for index",
					},
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
			Entry("with three failure domains matching three machines indexed from 3 (a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching three machines that are not sequentially indexed (a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching three machines indexed from 3 (a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching four machines that are not sequentially indexed (b,a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-6").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-9").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					6: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					9: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								6: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								9: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching five machines that are not sequentially indexed (c,a,a,b,c)", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-8").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-13").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					2:  failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3:  failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					5:  failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					8:  failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					13: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								2:  failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
								3:  failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								5:  failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								8:  failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								13: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching five machines which duplicate indexes", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
							}),
						},
						Message: "Mapped provided failure domains",
					},
				},
			}),
			Entry("with three failure domains matching 7 machines which duplicate indexes", mappingMachineIndexesTableInput{
				cpmsBuilder: cpmsBuilder,
				failureDomains: resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				).BuildFailureDomains(),
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"mapping", fmt.Sprintf("%v", map[int32]failuredomain.FailureDomain{
								0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
								4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
								5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
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
			machineCount    int
			failureDomains  machinev1.FailureDomains
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedError   error
		}

		DescribeTable("should map the failure domains based on the replicas", func(in createBaseMappingTableInput) {
			failureDomains, err := failuredomain.NewFailureDomains(in.failureDomains)
			Expect(err).ToNot(HaveOccurred())

			cpms := in.cpmsBuilder.Build()
			mapping, err := createBaseFailureDomainMapping(cpms, failureDomains, in.machineCount)
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 3,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(5),
				machineCount: 5,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(5),
				machineCount: 5,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(5),
				machineCount: 5,
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
				cpmsBuilder:  cpmsBuilder.WithReplicas(5),
				machineCount: 5,
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
			Entry("with three replicas but a machine count of five", createBaseMappingTableInput{
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 5,
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
			Entry("with four replicas but a machine count of 2", createBaseMappingTableInput{
				cpmsBuilder:  cpmsBuilder.WithReplicas(4),
				machineCount: 2,
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
				},
			}),
			Entry("with 3 replicas but a machine count of 1", createBaseMappingTableInput{
				cpmsBuilder:  cpmsBuilder.WithReplicas(3),
				machineCount: 1,
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
		)
	})

	Context("createMachineMapping", func() {
		type machineMappingTableInput struct {
			cpmsBuilder             resourcebuilder.ControlPlaneMachineSetInterface
			machines                []*machinev1beta1.Machine
			deleteMachines          []int
			expectedDeletingIndexes sets.Set[int32]
			expectedError           error
			expectedMapping         map[int32]failuredomain.FailureDomain
			expectedLogs            []test.LogEntry
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

			for _, index := range in.deleteMachines {
				Eventually(komega.Update(in.machines[index], func() {
					in.machines[index].SetFinalizers([]string{"machine.openshift.io/machine"})
				})).Should(Succeed())

				Expect(k8sClient.Delete(ctx, in.machines[index])).To(Succeed())
			}

			originalCPMS := cpms.DeepCopy()

			mapping, deletingIndexes, err := createMachineMapping(ctx, logger.Logger(), k8sClient, cpms)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(mapping).To(Equal(in.expectedMapping))
			Expect(deletingIndexes).To(Equal(in.expectedDeletingIndexes))
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
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
				expectedDeletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with machines in three failure domains indexed from 3 (order a,b,c)", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-3").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-5").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedDeletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with machines in three failure domains not sequentially indexed (order a,b,c)", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-4").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				expectedDeletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with an index that only contains deleted machines", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				deleteMachines:          []int{0},
				expectedDeletingIndexes: sets.New[int32](0),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with multiple indexes that only contain deleted machines", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				deleteMachines:          []int{0, 2},
				expectedDeletingIndexes: sets.New[int32](0, 2),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("with an index that contains a deleted machine and a non-deleted machine", machineMappingTableInput{
				cpmsBuilder: cpmsBuilder,
				machines: []*machinev1beta1.Machine{
					machineBuilder.WithName("machine-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-replacement-0").WithProviderSpecBuilder(usEast1aProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-1").WithProviderSpecBuilder(usEast1bProviderSpecBuilder).Build(),
					machineBuilder.WithName("machine-2").WithProviderSpecBuilder(usEast1cProviderSpecBuilder).Build(),
				},
				deleteMachines:          []int{0},
				expectedDeletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
		)
	})

	Context("reconcileMappings", func() {
		type reconcileMappingsTableInput struct {
			baseMapping     map[int32]failuredomain.FailureDomain
			machineMapping  map[int32]failuredomain.FailureDomain
			deletingIndexes sets.Set[int32]
			expectedMapping map[int32]failuredomain.FailureDomain
			expectedLogs    []test.LogEntry
		}

		DescribeTable("should keep the machine indexes stable where possible", func(in reconcileMappingsTableInput) {
			// Run each test 10 times in an attempt to make sure the output is stable.
			for i := 0; i < 10; i++ {
				logger := test.NewTestLogger()

				mapping := reconcileMappings(logger.Logger(), in.baseMapping, in.machineMapping, in.deletingIndexes)

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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
			Entry("when the base mapping has a failure domain not in the machine mapping, and a machine index only contains deleting machines (1)", reconcileMappingsTableInput{
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
				deletingIndexes: sets.New[int32](1),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"index", 1,
							"oldFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1a]}]}}",
							"newFailureDomain", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1c]}]}}",
						},
						Message: "Failure domain changed for index",
					},
				},
			}),
			Entry("when the base mapping has a failure domain not in the machine mapping, and a machine index only contains deleting machines (2)", reconcileMappingsTableInput{
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
				deletingIndexes: sets.New[int32](2),
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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
				deletingIndexes: sets.New[int32](),
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
			Entry("when a machine has a an index not present in the base mapping, keeps the additional index", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				deletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machines are indexed from 3, it shifts the mapping to match the machines", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				deletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machines are not sequentially indexed, it shifts the mapping to match the machines", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				deletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machines are not sequentially indexed, it keeps the ordering of the failure domains (a,b,a,c)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				deletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
			Entry("when a machines are not sequentially indexed, it keeps the ordering of the failure domains (b,b,a,c)", reconcileMappingsTableInput{
				baseMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
				},
				machineMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					6: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				deletingIndexes: sets.New[int32](),
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					6: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				expectedLogs: []test.LogEntry{},
			}),
		)
	})

	Context("reconcileIndexes", func() {
		type reconcileIndexesTableInput struct {
			reconciledMapping map[int32]failuredomain.FailureDomain
			preferredMapping  map[int32]failuredomain.FailureDomain
			expectedMapping   map[int32]failuredomain.FailureDomain
		}

		DescribeTable("should keep the machine indexes stable where possible", func(in reconcileIndexesTableInput) {
			// Note the preferred mapping values don't matter, so can be nil for each test case.

			// Run each test 10 times in an attempt to make sure the output is stable.
			for i := 0; i < 10; i++ {
				// Copy the mapping to avoid mutating the input state.
				mapping := make(map[int32]failuredomain.FailureDomain)
				for k, v := range in.reconciledMapping {
					// Copy the pointer to avoid loop capture.
					val := v
					mapping[k] = val
				}

				reconcileIndexes(mapping, in.preferredMapping)

				Expect(mapping).To(Equal(in.expectedMapping))
			}
		},
			Entry("when the mappings match", reconcileIndexesTableInput{
				reconciledMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				preferredMapping: map[int32]failuredomain.FailureDomain{
					0: nil,
					1: nil,
					2: nil,
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("when the preferred mapping has additional values, does nothing", reconcileIndexesTableInput{
				reconciledMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				preferredMapping: map[int32]failuredomain.FailureDomain{
					0: nil,
					1: nil,
					2: nil,
					3: nil,
					4: nil,
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("when the preferred mapping is missing values", reconcileIndexesTableInput{
				reconciledMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				preferredMapping: map[int32]failuredomain.FailureDomain{
					0: nil,
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("when the preferred mapping is not aligned (3,4,5)", reconcileIndexesTableInput{
				reconciledMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				preferredMapping: map[int32]failuredomain.FailureDomain{
					3: nil,
					4: nil,
					5: nil,
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					4: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					5: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
			Entry("when the preferred mapping is not aligned (0,2,3)", reconcileIndexesTableInput{
				reconciledMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					1: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()),
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
				preferredMapping: map[int32]failuredomain.FailureDomain{
					0: nil,
					2: nil,
					3: nil,
				},
				expectedMapping: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(usEast1aFailureDomainBuilder.Build()),
					3: failuredomain.NewAWSFailureDomain(usEast1bFailureDomainBuilder.Build()), // 2 wasn't swapped so 3 ends up here
					2: failuredomain.NewAWSFailureDomain(usEast1cFailureDomainBuilder.Build()),
				},
			}),
		)
	})
})
