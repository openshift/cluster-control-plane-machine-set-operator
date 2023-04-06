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

package helpers

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

// ItShouldHaveAnActiveControlPlaneMachineSet returns an It that checks
// there is an active control plane machine set installed within the cluster.
func ItShouldHaveAnActiveControlPlaneMachineSet(testFramework framework.Framework) {
	It("should have an active control plane machine set", Offset(1), func() {
		ExpectControlPlaneMachineSetToBeActive(testFramework)
	})
}

// RollingUpdatePeriodicTestOptions allow the test cases to be configured.
type RollingUpdatePeriodicTestOptions struct {
	TestFramework        framework.Framework
	RolloutTimeout       time.Duration
	StabilisationTimeout time.Duration
}

// ControlPlaneMachineSetRegenerationTestOptions allow test cases to be configured.
type ControlPlaneMachineSetRegenerationTestOptions struct {
	TestFramework        framework.Framework
	OriginalProviderSpec machinev1beta1.ProviderSpec
	UpdatedProviderSpec  machinev1beta1.ProviderSpec
	UID                  types.UID
	Index                int
}

// ItShouldPerformARollingUpdate checks that the control plane machine set performs a rolling update
// in the manner desired.
func ItShouldPerformARollingUpdate(opts *RollingUpdatePeriodicTestOptions) {
	It("should perform a rolling update", Offset(1), func() {
		Expect(opts).ToNot(BeNil(), "test options are required")
		Expect(opts.TestFramework).ToNot(BeNil(), "testFramework is required")

		testFramework := opts.TestFramework
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We give the rollout two hours to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutTimeout := 2 * time.Hour
		if opts.RolloutTimeout.Seconds() != 0 {
			rolloutTimeout = opts.RolloutTimeout
		}

		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), rolloutTimeout)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return CheckReplicasDoesNotExceedSurgeCapacity(rolloutCtx)
		})

		framework.Async(wg, cancel, func() bool {
			return WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return checkRolloutProgress(testFramework, rolloutCtx)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine replacement completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		stabilisationTimeout := 30 * time.Minute
		if opts.StabilisationTimeout.Seconds() != 0 {
			stabilisationTimeout = opts.StabilisationTimeout
		}

		stabilisationInterval := stabilisationTimeout / 50

		EventuallyClusterOperatorsShouldStabilise(stabilisationTimeout, stabilisationInterval)
		By("Cluster stabilised after the rollout")
	})
}

// ItShouldRollingUpdateReplaceTheOutdatedMachine checks that the control plane machine set replaces, via a rolling update,
// the outdated machine in the given index.
func ItShouldRollingUpdateReplaceTheOutdatedMachine(testFramework framework.Framework, index int) {
	It("should rolling update replace the outdated machine", func() {
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		// We give the rollout 30 minutes to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), 30*time.Minute)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return CheckReplicasDoesNotExceedSurgeCapacity(rolloutCtx)
		})

		framework.Async(wg, cancel, func() bool {
			return WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return CheckRolloutForIndex(testFramework, rolloutCtx, 1, machinev1.RollingUpdate)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine rollout completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		EventuallyClusterOperatorsShouldStabilise(30*time.Minute, 30*time.Second)
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
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

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

// ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted checks that the control plane machine set replaces the outdated
// machine in the given index when the update strategy is OnDelete and the outdated machine is deleted.
func ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(testFramework framework.Framework, index int) {
	It("should replace the outdated machine when deleted", func() {
		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		// Make sure the CPMS exists before we delete the Machine, just in case.
		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		machine, err := machineForIndex(testFramework, index)
		Expect(err).ToNot(HaveOccurred(), "control plane machine should exist")

		// Delete the Machine.
		By("Deleting the machine")
		Expect(k8sClient.Delete(ctx, machine)).To(Succeed(), "control plane machine should be able to be deleted")

		// Deleting the Machine triggers a rollout, give the rollout 30 minutes to complete.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), 30*time.Minute)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return CheckRolloutForIndex(testFramework, rolloutCtx, index, machinev1.OnDelete)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine rollout completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		EventuallyClusterOperatorsShouldStabilise(20*time.Minute, 20*time.Second)
		By("Cluster stabilised after the rollout")
	})
}

// ItShouldUninstallTheControlPlaneMachineSet checks that the control plane machine set is correctly uninstalled
// when a deletion is triggered, without triggering control plane machines changes.
func ItShouldUninstallTheControlPlaneMachineSet(testFramework framework.Framework) {
	It("should uninstall the control plane machine set without control plane machine changes", func() {
		ExpectControlPlaneMachineSetToBeInactiveOrNotFound(testFramework)
		ExpectControlPlaneMachinesAllRunning(testFramework)
		ExpectControlPlaneMachinesNotOwned(testFramework)
		ExpectControlPlaneMachinesWithoutDeletionTimestamp(testFramework)
		EventuallyClusterOperatorsShouldStabilise()
	})
}

// ItShouldHaveTheControlPlaneMachineSetReplicasUpdated checks that the control plane machine set replicas are updated.
func ItShouldHaveTheControlPlaneMachineSetReplicasUpdated(testFramework framework.Framework) {
	It("should have the control plane machine set replicas up to date", func() {
		By("Checking the control plane machine set replicas are up to date")

		Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
		k8sClient := testFramework.GetClient()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		Expect(cpms.Spec.Replicas).ToNot(BeNil(), "replicas should always be set")

		desiredReplicas := *cpms.Spec.Replicas

		Expect(cpms).To(SatisfyAll(
			HaveField("Status.Replicas", Equal(desiredReplicas)),
			HaveField("Status.UpdatedReplicas", Equal(desiredReplicas)),
			HaveField("Status.ReadyReplicas", Equal(desiredReplicas)),
			HaveField("Status.UnavailableReplicas", Equal(int32(0))),
		), "control plane machine set replicas should be up to date")
	})
}

// ItShouldNotCauseARollout checks that the control plane machine set doesn't cause a rollout.
func ItShouldNotCauseARollout(testFramework framework.Framework) {
	It("should have the control plane machine set not cause a rollout", func() {
		By("Checking the control plane machine set replicas are consistently up to date")

		Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

		k8sClient := testFramework.GetClient()
		ctx := testFramework.GetContext()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

		Expect(cpms.Spec.Replicas).ToNot(BeNil(), "replicas should always be set")
		desiredReplicas := *cpms.Spec.Replicas

		// We expect the control plane machine set replicas to consistently
		// be up to date, which should mean no rollout has been triggered.
		// Here assume that if no changes happen to the replica counts
		// within the default timeout interval, then they won't happen at all.
		Consistently(komega.Object(cpms)).Should(SatisfyAll(
			HaveField("Status.Replicas", Equal(desiredReplicas)),
			HaveField("Status.UpdatedReplicas", Equal(desiredReplicas)),
			HaveField("Status.ReadyReplicas", Equal(desiredReplicas)),
			HaveField("Status.UnavailableReplicas", Equal(int32(0))),
		), "control plane machine set replicas should consisently be up to date")

		// Check that the operators are stable.
		EventuallyClusterOperatorsShouldStabilise()
	})
}

// ItShouldCheckAllControlPlaneMachinesHaveCorrectOwnerReferences checks that all the control plane machines
// have the correct owner references set.
func ItShouldCheckAllControlPlaneMachinesHaveCorrectOwnerReferences(testFramework framework.Framework) {
	It("should find all control plane machines to have owner references set", func() {
		// Check that all the control plane machines are owned.
		ExpectControlPlaneMachinesOwned(testFramework)

		// Check that no control plane machine is garbage collected (being deleted),
		// as this may happen if incorrect owner references are added.
		ConsistentlyControlPlaneMachinesWithoutDeletionTimestamp(testFramework)

		// Check that the operators are stable.
		EventuallyClusterOperatorsShouldStabilise()
	})
}

// ItShouldPerformControlPlaneMachineSetRegeneration checks that an inactive control plane machine set
// is regenerated if the reference machine spec changes.
func ItShouldPerformControlPlaneMachineSetRegeneration(opts *ControlPlaneMachineSetRegenerationTestOptions, gomegaArgs ...interface{}) {
	It("should perform control plane machine set regeneration", func() {
		Expect(opts.TestFramework).ToNot(BeNil(), "test framework should not be nil")
		ctx := opts.TestFramework.GetContext()
		cpms := opts.TestFramework.NewEmptyControlPlaneMachineSet()

		// Check that the control plane machine set is regenerated.
		WaitForControlPlaneMachineSetRemovedOrRecreated(ctx, opts.TestFramework, opts.UID)
		EnsureInactiveControlPlaneMachineSet(opts.TestFramework)

		rawExtension, err := opts.TestFramework.ConvertToControlPlaneMachineSetProviderSpec(opts.UpdatedProviderSpec)
		Expect(err).NotTo(HaveOccurred())

		By("Checking the control plane machine set reports 1 updated machine, 2 needing update")
		Eventually(komega.Object(cpms)).Should(
			SatisfyAll(
				HaveField("Status.UpdatedReplicas", Equal(int32(1))),
				HaveField("Status.UnavailableReplicas", Equal(int32(0))),
			),
		)

		By("Checking the control plane machine set has the correct providerSpec")
		Expect(cpms).To(
			HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw",
				MatchJSON(rawExtension.Raw)),
		)
	})
}
