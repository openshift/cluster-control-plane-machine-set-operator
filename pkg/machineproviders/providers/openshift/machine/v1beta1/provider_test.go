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

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("MachineProvider", func() {
	const ownerUID = "uid-1234abcd"

	var namespaceName string
	var logger test.TestLogger

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		logger = test.NewTestLogger()
	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
		)
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
