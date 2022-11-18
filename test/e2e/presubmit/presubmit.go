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

package presubmit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/common"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

// IncreaseControlPlaneMachineInstanceSize increases the instance size of the control plane machine
// in the given index. This should trigger the control plane machine set to update the machine in
// this index based on the update strategy.
func IncreaseControlPlaneMachineInstanceSize(testFramework framework.Framework, index int, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	machineList := &machinev1beta1.MachineList{}

	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	Expect(k8sClient.List(ctx, machineList, machineSelector)).Should(Succeed(), "control plane machines should be able to be listed")

	var indexMachine *machinev1beta1.Machine

	for _, machine := range machineList.Items {
		if !strings.HasSuffix(machine.Name, fmt.Sprintf("-%d", index)) {
			continue
		}

		if indexMachine != nil {
			Fail(fmt.Sprintf("more than one control plane machine with index %d: %s and %s", index, indexMachine.GetName(), machine.GetName()))
		}

		indexMachine = machine.DeepCopy()
	}

	originalProviderSpec := indexMachine.Spec.ProviderSpec

	updatedProviderSpec := originalProviderSpec.DeepCopy()
	Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")

	By("Updating the control plane machine provider spec")

	updateMachineArgs := append([]interface{}{komega.Update(indexMachine, func() {
		indexMachine.Spec.ProviderSpec = *updatedProviderSpec
	})}, gomegaArgs...)
	Eventually(updateMachineArgs...).Should(Succeed(), "control plane machine should be able to be updated")

	return originalProviderSpec
}

// ItShouldRollingUpdateReplaceTheOutdatedMachine checks that the control plane machine set replaces, via a rolling update,
// the outdated machine in the given index.
func ItShouldRollingUpdateReplaceTheOutdatedMachine(testFramework framework.Framework, index int) {
	It("should rolling update replace the outdated machine", func() {
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, framework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We give the rollout 30 minutes to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), 30*time.Minute)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return common.CheckReplicasDoesNotExceedSurgeCapacity(rolloutCtx)
		})

		framework.Async(wg, cancel, func() bool {
			return common.WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return common.CheckRolloutForIndex(testFramework, rolloutCtx, 1)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine rollout completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		common.EventuallyClusterOperatorsShouldStabilise(20*time.Minute, 20*time.Second)
		By("Cluster stabilised after the rollout")
	})
}
