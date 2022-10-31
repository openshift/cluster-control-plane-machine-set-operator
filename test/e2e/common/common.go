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

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
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

// EventuallyClusterOperatorsShouldStabilise checks that the cluster operators stabilise over time.
// Stabilise means that they are available, are not progressing, and are not degraded.
func EventuallyClusterOperatorsShouldStabilise(args ...interface{}) {
	// The following assertion checks:
	// The list "Items", all (ConsistOf) have a field "Status.Conditions",
	// that contain elements that are both "Type" something and "Status" something.
	clusterOperators := &configv1.ClusterOperatorList{}
	args = append([]interface{}{komega.ObjectList(clusterOperators)}, args...)

	By("Waiting for the cluster operators to stabilise")

	Eventually(args...).Should(HaveField("Items", HaveEach(HaveField("Status.Conditions",
		SatisfyAll(
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorAvailable)), HaveField("Status", Equal(configv1.ConditionTrue)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorProgressing)), HaveField("Status", Equal(configv1.ConditionFalse)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorDegraded)), HaveField("Status", Equal(configv1.ConditionFalse)))),
		),
	))), "cluster operators should all be available, not progressing and not degraded")
}
