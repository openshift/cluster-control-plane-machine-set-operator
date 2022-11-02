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

package periodic

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/common"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// IncreaseControlPlaneMachineSetInstanceSize increases the instance size of the control plane machine set.
// This should trigger the control plane machine set to update the machines based on the
// update strategy.
func IncreaseControlPlaneMachineSetInstanceSize(testFramework framework.Framework, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	cpms := framework.NewEmptyControlPlaneMachineSet()

	getCPMSArgs := append([]interface{}{komega.Get(cpms)}, gomegaArgs...)
	Eventually(getCPMSArgs...).Should(Succeed(), "control plane machine set should exist")

	originalProviderSpec := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec

	updatedProviderSpec := originalProviderSpec.DeepCopy()
	Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")

	By("Increasing the control plane machine set instance size")

	updateCPMSArgs := append([]interface{}{komega.Update(cpms, func() {
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec = *updatedProviderSpec
	})}, gomegaArgs...)
	Eventually(updateCPMSArgs...).Should(Succeed(), "control plane machine set should be able to be updated")

	return originalProviderSpec
}

// ItShouldPerformARollingUpdate checks that the control plane machine set performs a rolling update
// in the manner desired.
func ItShouldPerformARollingUpdate(testFramework framework.Framework) {
	It("should perform a rolling update", Offset(1), func() {
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, framework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We give the rollout an hour to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), 1*time.Hour)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			By("Checking the number of control plane machines never goes above 4 replicas")
			controlPlaneMachineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

			// For now, we are checking that the surge is limited to just 1 instance. So 3 + 1 = 4 maximum replicas.
			return Consistently(komega.ObjectList(&machinev1beta1.MachineList{}, controlPlaneMachineSelector), rolloutCtx).Should(HaveField("Items", SatisfyAny(
				HaveLen(3),
				HaveLen(4),
			)), "control plane machines should never go above 4 replicas, or below 3 replicas")
		})

		framework.Async(wg, cancel, func() bool {
			return waitForDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return checkRolloutProgress(testFramework, rolloutCtx)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine replacement completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		common.EventuallyClusterOperatorsShouldStabilise(20*time.Minute, 20*time.Second)
		By("Cluster stabilised after the rollout")
	})
}

// waitForDesiredReplicas waits for the control plane machine set to have the desired number of replicas.
// It first waits for the updated replicas to equal the desired number, and then waits for the final replica
// count to equal the desired number.
func waitForDesiredReplicas(ctx context.Context, cpms *machinev1.ControlPlaneMachineSet) bool {
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

// checkRolloutProgress monitors the progress of each index in the rollout in turn.
func checkRolloutProgress(testFramework framework.Framework, ctx context.Context) bool {
	if ok := checkRolloutForIndex(testFramework, ctx, 0); !ok {
		return false
	}

	if ok := checkRolloutForIndex(testFramework, ctx, 1); !ok {
		return false
	}

	if ok := checkRolloutForIndex(testFramework, ctx, 2); !ok {
		return false
	}

	return true
}

// checkRolloutForIndex first checks that a new machine is created in the correct index,
// and then checks that the new machine in the index is replaced correctly.
func checkRolloutForIndex(testFramework framework.Framework, ctx context.Context, idx int) bool {
	By(fmt.Sprintf("Waiting for the index %d to be replaced", idx))
	// Don't provide additional timeouts here, the default should be enough.
	if ok := common.EventuallyIndexIsBeingReplaced(ctx, idx); !ok {
		return false
	}

	By(fmt.Sprintf("Index %d replacement created", idx))
	By(fmt.Sprintf("Checking the replacement machine for index %d", idx))

	if ok := common.CheckControlPlaneMachineRollingReplacement(testFramework, idx, ctx); !ok {
		return false
	}

	By(fmt.Sprintf("Replacement for index %d is complete", idx))

	return true
}
