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

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/helpers"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.Periodic(), func() {
	BeforeEach(func() {
		helpers.EventuallyClusterOperatorsShouldStabilise(60*time.Minute, 30*time.Second)
	})

	Context("With an active ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureActiveControlPlaneMachineSet(testFramework)
		})

		Context("and the instance type is changed", func() {
			BeforeEach(func() {
				helpers.IncreaseControlPlaneMachineSetInstanceSize(testFramework)
			})

			helpers.ItShouldPerformARollingUpdate(&helpers.RollingUpdatePeriodicTestOptions{
				TestFramework: testFramework,
			})
		})

		Context("and an instance is terminated on the cloud provider", framework.Informing(), func() {
			BeforeEach(func() {
				client := testFramework.GetClient()
				machineList := &machinev1beta1.MachineList{}
				machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

				By("Getting a list of all control plane machines")
				Expect(client.List(testFramework.GetContext(), machineList, machineSelector)).To(Succeed(), "should be able to retrieve list of control plane machines")

				machine, err := helpers.GetMachineAtIndex(machineList, 0)
				Expect(err).ToNot(HaveOccurred())
				Expect(machine).ToNot(BeNil())

				By("Checking that the machine is actually in index 0")
				machineIdx, err := helpers.MachineIndex(*machine)
				Expect(err).ToNot(HaveOccurred())
				Expect(machineIdx).To(Equal(0), "Expected first machine to have index 0")

				By("Deleting an instance from the cloud provider")
				Expect(testFramework.DeleteAnInstanceFromCloudProvider(machine)).To(Succeed())

				By("Waiting for the machine to get into Failed phase")
				Eventually(komega.Object(machine), 15*time.Minute).Should(HaveField("Status.Phase", HaveValue(Equal("Failed"))))

				By("Deleting a control plane machine in phase Failed at index 0")
				Expect(client.Delete(testFramework.GetContext(), machine)).To(Succeed())
			})

			helpers.ItShouldReplaceTheOutDatedMachineInDeleting(testFramework, 0)
		})

		Context("and a node with terminated kubelet", framework.Informing(), func() {
			var client runtimeclient.Client
			var ctx context.Context
			var delObjects map[string]runtimeclient.Object

			BeforeEach(func() {
				client = testFramework.GetClient()
				ctx = testFramework.GetContext()
				delObjects = make(map[string]runtimeclient.Object)
				machineList := &machinev1beta1.MachineList{}
				machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
				node := &corev1.Node{}

				By("Getting a list of all control plane machines")
				Expect(client.List(ctx, machineList, machineSelector)).To(Succeed(), "should be able to retrieve list of control plane machines")

				machine, err := helpers.GetMachineAtIndex(machineList, 2)
				Expect(err).ToNot(HaveOccurred())
				Expect(machine).ToNot(BeNil())

				By("Checking that the machine is actually in index 2")
				machineIdx, err := helpers.MachineIndex(*machine)
				Expect(err).ToNot(HaveOccurred())
				Expect(machineIdx).To(Equal(2), "Expected first machine to have index 2")

				By("Getting the node from the machine")
				Expect(client.Get(ctx, types.NamespacedName{
					Name: machine.Status.NodeRef.Name,
				}, node)).To(Succeed())

				By("Shutting down the kubelet on a node")
				Expect(testFramework.TerminateKubelet(node, delObjects)).To(Succeed())

				By("Waiting for the node to get into NotReady phase")
				Eventually(komega.Object(node), 15*time.Minute).Should(HaveField("Status.Conditions",
					HaveEach(HaveField("Status",
						SatisfyAny(Equal(corev1.ConditionUnknown), Equal(corev1.ConditionFalse)),
					)),
				))

				By("Deleting the node's control plane machine")
				Expect(client.Delete(testFramework.GetContext(), machine)).To(Succeed())
			})

			helpers.ItShouldReplaceTheOutDatedMachineInDeleting(testFramework, 2)

			AfterEach(func() {
				for _, obj := range delObjects {
					Expect(client.Delete(ctx, obj)).To(Succeed())
				}
				By("All created objects have been deleted successfully")
			})
		})
	})
})
