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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/common"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var (
	errMoreThanOneMachineInIndex = errors.New("more than one control plane machine in index")
)

// IncreaseControlPlaneMachineInstanceSize increases the instance size of the control plane machine
// in the given index. This should trigger the control plane machine set to update the machine in
// this index based on the update strategy.
func IncreaseControlPlaneMachineInstanceSize(testFramework framework.Framework, index int, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	machine, err := machineForIndex(testFramework, index)
	Expect(err).ToNot(HaveOccurred(), "control plane machine should exist")

	originalProviderSpec := machine.Spec.ProviderSpec

	updatedProviderSpec := originalProviderSpec.DeepCopy()
	Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")

	By("Updating the control plane machine provider spec")

	updateMachineArgs := append([]interface{}{komega.Update(machine, func() {
		machine.Spec.ProviderSpec = *updatedProviderSpec
	})}, gomegaArgs...)
	Eventually(updateMachineArgs...).Should(Succeed(), "control plane machine should be able to be updated")

	return originalProviderSpec
}

// UpdateControlPlaneMachineProviderSpec updates the provider spec of the control plane machine in the given index
// to match the provider spec given.
func UpdateControlPlaneMachineProviderSpec(testFramework framework.Framework, index int, updatedProviderSpec machinev1beta1.ProviderSpec, gomegaArgs ...interface{}) {
	machine, err := machineForIndex(testFramework, index)
	Expect(err).ToNot(HaveOccurred(), "control plane machine should exist")

	updateMachineArgs := append([]interface{}{komega.Update(machine, func() {
		machine.Spec.ProviderSpec = updatedProviderSpec
	})}, gomegaArgs...)
	Eventually(updateMachineArgs...).Should(Succeed(), "control plane machine should be able to be updated")
}

// machineForIndex returns the control plane machine in the given index.
// If multiple machines exist within the index, an error is returned.
// Only use this when you expect exactly one machine in the index.
func machineForIndex(testFramework framework.Framework, index int) (*machinev1beta1.Machine, error) {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	machineList := &machinev1beta1.MachineList{}

	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	if err := k8sClient.List(ctx, machineList, machineSelector); err != nil {
		return nil, fmt.Errorf("could not list control plane machines: %w", err)
	}

	var indexMachine *machinev1beta1.Machine

	for _, machine := range machineList.Items {
		if !strings.HasSuffix(machine.Name, fmt.Sprintf("-%d", index)) {
			continue
		}

		if indexMachine != nil {
			return nil, fmt.Errorf("%w %d: %s and %s", errMoreThanOneMachineInIndex, index, indexMachine.GetName(), machine.GetName())
		}

		indexMachine = machine.DeepCopy()
	}

	return indexMachine, nil
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

// ItShouldNotOnDeleteReplaceTheOutdatedMachine checks that the control plane machine set does not replace the outdated
// machine in the given index when the update strategy is OnDelete.
func ItShouldNotOnDeleteReplaceTheOutdatedMachine(testFramework framework.Framework, index int) {
	It("should not replace the outdated machine", func() {
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, framework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We expected the updated replicas count to fall to 2, other values should remain
		// as expected.
		Eventually(komega.Object(cpms)).Should(HaveField("Status", SatisfyAll(
			HaveField("Replicas", Equal(int32(3))),
			HaveField("UpdatedReplicas", Equal(int32(2))),
			HaveField("ReadyReplicas", Equal(int32(3))),
		)))

		// Check the Machine doesn't get deleted by the CPMS. We assume that if the CPMS hasn't removed
		// the Machine within 1 minute that it won't remove it at all.
		Consistently(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", ContainElement(SatisfyAll(
			HaveField("ObjectMeta.Name", HaveSuffix(fmt.Sprintf("-%d", index))),
			HaveField("ObjectMeta.DeletionTimestamp", BeNil()),
			HaveField("Status.Phase", HaveValue(Equal("Running"))),
		))))
	})
}

// ItShouldUninstallTheControlPlaneMachineSet checks that the control plane machine set is correctly uninstalled
// when a deletion is triggered, without triggering control plane machines changes.
func ItShouldUninstallTheControlPlaneMachineSet(testFramework framework.Framework) {
	It("should uninstall the control plane machine set without control plane machine changes", func() {
		common.ExpectControlPlaneMachineSetToBeInactiveOrNotFound(testFramework)
		common.ExpectControlPlaneMachinesAllRunning(testFramework)
		common.ExpectControlPlaneMachinesNotOwned(testFramework)
		common.ExpectControlPlaneMachinesWithoutDeletionTimestamp(testFramework)
		common.EventuallyClusterOperatorsShouldStabilise()
	})
}
