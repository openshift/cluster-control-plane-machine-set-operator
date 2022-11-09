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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

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
func EventuallyClusterOperatorsShouldStabilise(gomegaArgs ...interface{}) {
	key := format.RegisterCustomFormatter(formatClusterOperatorsCondtions)
	defer format.UnregisterCustomFormatter(key)

	// The following assertion checks:
	// The list "Items", all (ConsistOf) have a field "Status.Conditions",
	// that contain elements that are both "Type" something and "Status" something.
	clusterOperators := &configv1.ClusterOperatorList{}
	gomegaArgs = append([]interface{}{komega.ObjectList(clusterOperators)}, gomegaArgs...)

	By("Waiting for the cluster operators to stabilise")

	Eventually(gomegaArgs...).Should(HaveField("Items", HaveEach(HaveField("Status.Conditions",
		SatisfyAll(
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorAvailable)), HaveField("Status", Equal(configv1.ConditionTrue)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorProgressing)), HaveField("Status", Equal(configv1.ConditionFalse)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorDegraded)), HaveField("Status", Equal(configv1.ConditionFalse)))),
		),
	))), "cluster operators should all be available, not progressing and not degraded")
}

// EnsureActiveControlPlaneMachineSet ensures that there is an active control plane machine set
// within the cluster. For fully supported clusters, this means waiting for the control plane machine set
// to be created and checking that it is active. For manually supported clusters, this means creating the
// control plane machine set, checking its status and then activating it.
func EnsureActiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	switch testFramework.GetPlatformSupportLevel() {
	case framework.Full:
		ensureActiveControlPlaneMachineSet(gomegaArgs...)
	case framework.Manual:
		Fail("manual support for the control plane machine set not yet implemented")
	case framework.Unsupported:
		Fail(fmt.Sprintf("control plane machine set does not support platform %s", testFramework.GetPlatformType()))
	}
}

// ensureActiveControlPlaneMachineSet checks that a CPMS exists and then, if it is not active, activates it.
func ensureActiveControlPlaneMachineSet(gomegaArgs ...interface{}) {
	cpms := framework.NewEmptyControlPlaneMachineSet()

	By("Checking the control plane machine set exists")

	checkExistsArgs := append([]interface{}{komega.Get(cpms)}, gomegaArgs...)
	Eventually(checkExistsArgs...).Should(Succeed(), "control plane machine set should exist")

	if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateActive {
		By("Activating the control plane machine set")

		updateStateArgs := append([]interface{}{
			komega.Update(cpms, func() {
				cpms.Spec.State = machinev1.ControlPlaneMachineSetStateActive
			}),
		}, gomegaArgs...)

		Eventually(updateStateArgs...).Should(Succeed(), "control plane machine set should be able to be actived")
	}

	By("Checking the control plane machine set is active")

	checkStateArgs := append([]interface{}{komega.Object(cpms)}, gomegaArgs...)
	Eventually(checkStateArgs...).Should(HaveField("Spec.State", Equal(machinev1.ControlPlaneMachineSetStateActive)), "control plane machine set should be active")
}

// WaitForControlPlaneMachineSetDesiredReplicas waits for the control plane machine set to have the desired number of replicas.
// It first waits for the updated replicas to equal the desired number, and then waits for the final replica
// count to equal the desired number.
func WaitForControlPlaneMachineSetDesiredReplicas(ctx context.Context, cpms *machinev1.ControlPlaneMachineSet) bool {
	if ok := Expect(cpms.Spec.Replicas).ToNot(BeNil(), "replicas should always be set"); !ok {
		return false
	}

	desiredReplicas := *cpms.Spec.Replicas

	By("Waiting for the updated replicas to equal desired replicas")

	if ok := Eventually(komega.Object(cpms)).WithContext(ctx).Should(HaveField("Status.UpdatedReplicas", Equal(desiredReplicas)), "control plane machine set should have updated all replicas"); !ok {
		return false
	}

	By("Updated replicas is now equal to desired replicas")

	// Once the updated replicas equals the desired replicas, we need
	// to wait for the total replicas to go back to the desired replicas.
	// This will check the final machine gets removed before we end the test.
	By("Waiting for the replicas to equal desired replicas")

	if ok := Eventually(komega.Object(cpms)).WithContext(ctx).Should(HaveField("Status.Replicas", Equal(desiredReplicas)), "control plane machine set should have the desired number of replicas"); !ok {
		return false
	}

	By("Replicas is now equal to desired replicas")

	return true
}
