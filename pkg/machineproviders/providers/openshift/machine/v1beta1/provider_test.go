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
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("MachineProvider", func() {
	const ownerUID = "uid-1234abcd"
	const ownerName = "machineOwner"

	var namespaceName string
	var logger test.TestLogger

	BeforeEach(OncePerOrdered, func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		logger = test.NewTestLogger()
	})

	AfterEach(OncePerOrdered, func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
		)
	})

	Context("CreateMachine", func() {
		var provider machineproviders.MachineProvider
		var template machinev1.ControlPlaneMachineSetTemplate

		assertCreatesMachine := func(index int32, expectedProviderConfig resourcebuilder.RawExtensionBuilder, clusterID, failureDomain string) {
			Context(fmt.Sprintf("creating a machine in index %d", index), Ordered, func() {
				// NOTE: this is an ordered container, each assertion in this
				// function will run ordered, rather than in parallel.
				// This means state can be shared between these.
				// We use this so that we can break up individual assertions
				// on the Machine state into separate containers.

				var err error
				var machine machinev1beta1.Machine

				BeforeAll(func() {
					err = provider.CreateMachine(ctx, logger.Logger(), index)
				})

				PIt("should not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				Context("should create a machine", func() {
					PIt("with a name in the correct format", func() {
						nameMatcher := MatchRegexp(fmt.Sprintf("%s-master-[a-z]{5}-%d", clusterID, index))

						machineList := &machinev1beta1.MachineList{}
						Eventually(komega.ObjectList(machineList, client.InNamespace(namespaceName))).Should(HaveField("Items", ContainElement(
							HaveField("ObjectMeta.Name", nameMatcher),
						)))

						// Set the machine variable to the new Machine so that we can
						// inspect it on subsequent tests.
						for _, m := range machineList.Items {
							if ok, err := nameMatcher.Match(m.Name); err == nil && ok {
								machine = m
								break
							}
						}
					})

					PIt("with the labels from the Machine template", func() {
						Expect(machine.Labels).To(Equal(
							template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels,
						))
					})

					PIt("with annotations from the Machine template", func() {
						Expect(machine.Annotations).To(Equal(
							template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Annotations,
						))
					})

					PIt("with the correct owner reference", func() {
						Expect(machine.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
							APIVersion:         machinev1.GroupVersion.String(),
							Kind:               "ControlPlaneMachineSet",
							Name:               ownerName,
							UID:                ownerUID,
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						}))
					})

					PIt("with the correct provider spec", func() {
						Expect(machine.Spec.ProviderSpec.Value).To(SatisfyAll(
							Not(BeNil()),
							HaveField("Raw", MatchJSON(expectedProviderConfig.BuildRawExtension().Raw)),
						))
					})

					PIt("with no providerID set", func() {
						Expect(machine.Spec.ProviderID).To(BeNil())
					})

					PIt("logs that the machine was created", func() {
						Expect(logger.Entries()).To(ConsistOf(
							test.LogEntry{
								Level: 2,
								KeysAndValues: []interface{}{
									"index", index,
									"machineName", machine.Name,
									"failureDomain", failureDomain,
								},
								Message: "Created machine",
							},
						))
					})
				})
			})
		}

		Context("with an AWS template", func() {
			providerConfigBuilder := resourcebuilder.AWSProviderSpec()
			template = resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(providerConfigBuilder).
				WithLabel(machinev1beta1.MachineClusterIDLabel, "cpms-aws-cluster-id").
				BuildTemplate()

			BeforeEach(OncePerOrdered, func() {
				providerConfig, err := providerconfig.NewProviderConfig(*template.OpenShiftMachineV1Beta1Machine)
				Expect(err).ToNot(HaveOccurred())

				provider = &openshiftMachineProvider{
					client: k8sClient,
					indexToFailureDomain: map[int32]failuredomain.FailureDomain{
						0: failuredomain.NewAWSFailureDomain(resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build()),
						1: failuredomain.NewAWSFailureDomain(resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").Build()),
						2: failuredomain.NewAWSFailureDomain(resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").Build()),
					},
					machineSelector: resourcebuilder.ControlPlaneMachineSet().Build().Spec.Selector,
					machineTemplate: *template.OpenShiftMachineV1Beta1Machine,
					ownerMetadata: metav1.ObjectMeta{
						Name: ownerName,
						UID:  ownerUID,
					},
					providerConfig: providerConfig,
				}
			})

			assertCreatesMachine(0, providerConfigBuilder.WithAvailabilityZone("us-east-1a"), "cpms-aws-cluster-id", "us-east-1a")
			assertCreatesMachine(1, providerConfigBuilder.WithAvailabilityZone("us-east-1b"), "cpms-aws-cluster-id", "us-east-1b")
			assertCreatesMachine(2, providerConfigBuilder.WithAvailabilityZone("us-east-1c"), "cpms-aws-cluster-id", "us-east-1c")

			Context("if the Machine template is missing the cluster ID label", func() {
				var err error

				BeforeEach(func() {
					p, ok := provider.(*openshiftMachineProvider)
					Expect(ok).To(BeTrue())

					delete(p.machineTemplate.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)

					err = provider.CreateMachine(ctx, logger.Logger(), 0)
				})

				PIt("returns an error", func() {
					Expect(err).To(MatchError(errMissingClusterIDLabel))
				})

				PIt("does not create any Machines", func() {
					Consistently(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", BeEmpty()))
				})
			})
		})

	})

	Context("DeleteMachine", func() {
		var machineName string
		var machineRef *machineproviders.ObjectRef
		var machineProvider machineproviders.MachineProvider

		BeforeEach(func() {
			By("Setting up the MachineProvider")
			cpms := resourcebuilder.ControlPlaneMachineSet().Build()

			template := resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(resourcebuilder.AWSProviderSpec()).
				BuildTemplate().OpenShiftMachineV1Beta1Machine
			Expect(template).ToNot(BeNil())

			providerConfig, err := providerconfig.NewProviderConfig(*template)
			Expect(err).ToNot(HaveOccurred())

			machineProvider = &openshiftMachineProvider{
				client:               k8sClient,
				indexToFailureDomain: map[int32]failuredomain.FailureDomain{},
				machineSelector:      cpms.Spec.Selector,
				machineTemplate:      *template,
				ownerMetadata: metav1.ObjectMeta{
					UID: types.UID(ownerUID),
				},
				providerConfig: providerConfig,
			}

			machineBuilder := resourcebuilder.Machine().AsMaster().
				WithGenerateName("control-plane-machine-").
				WithNamespace(namespaceName)

			machine := machineBuilder.Build()
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())
			machineName = machine.Name
		})

		Context("with the correct GVR", func() {
			BeforeEach(func() {
				machineRef = &machineproviders.ObjectRef{
					GroupVersionResource: machinev1beta1.GroupVersion.WithResource("machines"),
				}
			})

			Context("with an existing machine", func() {
				var err error

				BeforeEach(func() {
					machineRef.ObjectMeta.Name = machineName

					err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
				})

				PIt("deletes the Machine", func() {
					machine := resourcebuilder.Machine().
						WithNamespace(namespaceName).
						WithName(machineName).
						Build()

					notFoundErr := apierrors.NewNotFound(machineRef.GroupVersionResource.GroupResource(), machineName)

					Eventually(komega.Get(machine)).Should(MatchError(notFoundErr))
				})

				PIt("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				PIt("logs that the machine was deleted", func() {
					Expect(logger.Entries()).To(ConsistOf(
						test.LogEntry{
							Level: 2,
							KeysAndValues: []interface{}{
								"namespace", namespaceName,
								"machineName", machineName,
								"group", machinev1beta1.GroupVersion.Group,
								"version", machinev1beta1.GroupVersion.Version,
							},
							Message: "Deleted machine",
						},
					))
				})
			})

			Context("with an non-existent machine", func() {
				var err error
				const unknown = "unknown"

				BeforeEach(func() {
					machineRef.ObjectMeta.Name = unknown

					err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
				})

				PIt("does not delete the existing Machine", func() {
					machine := resourcebuilder.Machine().
						WithNamespace(namespaceName).
						WithName(machineName).
						Build()

					Consistently(komega.Get(machine)).Should(Succeed())
				})

				PIt("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				PIt("logs that the machine was already deleted", func() {
					Expect(logger.Entries()).To(ConsistOf(
						test.LogEntry{
							Level: 2,
							KeysAndValues: []interface{}{
								"namespace", namespaceName,
								"machineName", unknown,
								"group", machinev1beta1.GroupVersion.Group,
								"version", machinev1beta1.GroupVersion.Version,
							},
							Message: "Machine not found",
						},
					))
				})
			})
		})

		Context("with an incorrect GVR", func() {
			var err error

			BeforeEach(func() {
				machineRef := &machineproviders.ObjectRef{
					GroupVersionResource: machinev1.GroupVersion.WithResource("machines"),
					ObjectMeta: metav1.ObjectMeta{
						Name: machineName,
					},
				}

				err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
			})

			PIt("returns an error", func() {
				Expect(err).To(MatchError(fmt.Errorf("%w: expected %s, got %s", errUnknownGroupVersionResource, machinev1beta1.GroupVersion.WithResource("machines").String(), machinev1.GroupVersion.WithResource("machines").String())))
			})

			PIt("logs the error", func() {
				Expect(logger.Entries()).To(ConsistOf(
					test.LogEntry{
						Error: errUnknownGroupVersionResource,
						KeysAndValues: []interface{}{
							"expectedGVR", machinev1beta1.GroupVersion.WithResource("machines").String(),
							"gotGVR", machinev1.GroupVersion.WithResource("machines").String(),
						},
						Message: "Could not delete machine",
					},
				))
			})
		})
	})
})
