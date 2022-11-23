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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/common"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// IncreaseControlPlaneMachineSetInstanceSize increases the instance size of the control plane machine set.
// This should trigger the control plane machine set to update the machines based on the
// update strategy.
func IncreaseControlPlaneMachineSetInstanceSize(testFramework framework.Framework, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	cpms := testFramework.NewEmptyControlPlaneMachineSet()

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
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We give the rollout an hour to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), 1*time.Hour)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return common.CheckReplicasDoesNotExceedSurgeCapacity(rolloutCtx)
		})

		framework.Async(wg, cancel, func() bool {
			return common.WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
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

// checkRolloutProgress monitors the progress of each index in the rollout in turn.
func checkRolloutProgress(testFramework framework.Framework, ctx context.Context) bool {
	if ok := common.CheckRolloutForIndex(testFramework, ctx, 0, machinev1.RollingUpdate); !ok {
		return false
	}

	if ok := common.CheckRolloutForIndex(testFramework, ctx, 1, machinev1.RollingUpdate); !ok {
		return false
	}

	if ok := common.CheckRolloutForIndex(testFramework, ctx, 2, machinev1.RollingUpdate); !ok {
		return false
	}

	return true
}
