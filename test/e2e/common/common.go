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

package common

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

// ItShouldHaveAnActiveControlPlaneMachineSet returns an It that checks
// there is an active control plane machine set installed within the cluster.
func ItShouldHaveAnActiveControlPlaneMachineSet(testFramework framework.Framework) {
	It("should have an active control plane machine set", Offset(1), func() {
		ExpectControlPlaneMachineSetToBeActive(testFramework)
	})
}

// ExpectControlPlaneMachineSetToBeActive gets the control plane machine set and
// checks that it is active.
func ExpectControlPlaneMachineSetToBeActive(testFramework framework.Framework) {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	k8sClient := testFramework.GetClient()

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(testFramework.GetContext(), framework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateActive), "control plane machine set should be active")
}
