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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/helpers"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.Periodic(), func() {
	BeforeEach(func() {
		helpers.EventuallyClusterOperatorsShouldStabilise(10*time.Minute, 10*time.Second)
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

		Context("and an instance is terminated on the cloud provider", func() {
			var client runtimeclient.Client
			var machineList machinev1beta1.MachineList
			var machineSelector runtimeclient.MatchingLabels

			BeforeEach(func() {
				client = testFramework.GetClient()

				By("Getting a list of all control plane machines")
				machineSelector = runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
				Expect(client.List(testFramework.GetContext(), &machineList, machineSelector)).To(Succeed(), "should be able to retrieve list of control plane machines")

				By("Deleting an instance from the cloud provider")
				Expect(testFramework.DeleteAnInstanceFromCloudProvider()).To(Succeed())

				By("Waiting for a machine to get into failed phase")
				Eventually(komega.Object(&machineList.Items[0]), 10*time.Minute).Should(HaveField("Status.Phase", HaveValue(Equal("Failed"))))

				By("Deleting a control plane machine in state Failed at index 0")
				Expect(client.Delete(testFramework.GetContext(), &machineList.Items[0])).To(Succeed())
			})

			helpers.ItShouldReplaceTheOutDatedMachineInDeleting(testFramework, 0)
		})
	})
})
