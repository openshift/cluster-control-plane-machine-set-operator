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
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	configv1 "github.com/openshift/api/config/v1"
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
	TestFramework                    framework.Framework
	RolloutTimeout                   time.Duration
	StabilisationTimeout             time.Duration
	StabilisationMinimumAvailability time.Duration
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

		stabilisationTimeout := 32 * time.Minute

		if opts.StabilisationTimeout.Seconds() != 0 {
			stabilisationTimeout = opts.StabilisationTimeout
		}

		stabilisationInterval := stabilisationTimeout / 50

		stabilisationMinimumAvailability := 2 * time.Minute
		if opts.StabilisationMinimumAvailability != 0 {
			stabilisationMinimumAvailability = opts.StabilisationMinimumAvailability
		}

		EventuallyClusterOperatorsShouldStabilise(stabilisationMinimumAvailability, stabilisationTimeout, stabilisationInterval)
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

		timeout := 30 * time.Minute

		platform := configv1.NonePlatformType
		if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains != nil {
			platform = cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform
		}

		if platform == configv1.VSpherePlatformType {
			timeout = 60 * time.Minute

			By("Test timeout set to 60 minutes for vSphere")
		}

		// We give the rollout 30 minutes to complete.
		// We pass this to Eventually and Consistently assertions to ensure that they check
		// until they pass or until the timeout is reached.
		rolloutCtx, cancel := context.WithTimeout(testFramework.GetContext(), timeout)
		defer cancel()

		wg := &sync.WaitGroup{}

		framework.Async(wg, cancel, func() bool {
			return CheckReplicasDoesNotExceedSurgeCapacity(rolloutCtx)
		})

		framework.Async(wg, cancel, func() bool {
			return WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms.DeepCopy())
		})

		framework.Async(wg, cancel, func() bool {
			return CheckRolloutForIndex(testFramework, rolloutCtx, index, machinev1.RollingUpdate)
		})

		wg.Wait()

		// If there's an error in the context, either it timed out or one of the async checks failed.
		Expect(rolloutCtx.Err()).ToNot(HaveOccurred(), "rollout should have completed successfully")
		By("Control plane machine rollout completed successfully")

		By("Waiting for the cluster to stabilise after the rollout")
		// 30 minutes for the rollout to complete, 2 minutes for the cluster to stabilise.
		// Check every 30 seconds.
		// The timeout includes the 2 minutes to stabilise, hence 32 minutes.
		EventuallyClusterOperatorsShouldStabilise(2*time.Minute, 32*time.Minute, 30*time.Second)
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
		// 20 minutes for the rollout to complete, 2 minutes for the cluster to stabilise.
		// Check every 20 seconds.
		// The timeout includes the 2 minutes to stabilise, hence 22 minutes.
		EventuallyClusterOperatorsShouldStabilise(2*time.Minute, 22*time.Minute, 20*time.Second)
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
		EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 2*time.Minute, 10*time.Second)
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
		EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 2*time.Minute, 10*time.Second)
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
		EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 2*time.Minute, 10*time.Second)
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

		// Failure domain fields need to be removed from template providerSpec for comparison.
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value, err = opts.TestFramework.ConvertToControlPlaneMachineSetProviderSpec(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec)
		Expect(err).NotTo(HaveOccurred(), "template providerSpec should support removal of failure domain fields")

		By("Checking the control plane machine set has the correct providerSpec")
		Expect(cpms).To(
			HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw",
				MatchJSON(rawExtension.Raw)),
		)
	})
}

// ItShouldHaveValidClusterOperatorStatus checks that the control-plane-machine-set ClusterOperator
// reports the correct status and version information.
// Migrated from openshift-tests-private OCP-53610.
func ItShouldHaveValidClusterOperatorStatus(testFramework framework.Framework) {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	By("Getting the control-plane-machine-set ClusterOperator")

	co := &configv1.ClusterOperator{}
	key := runtimeclient.ObjectKey{Name: "control-plane-machine-set"}

	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, key, co)
		g.Expect(err).NotTo(HaveOccurred())
	}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("Verifying ClusterOperator conditions")

	var available, progressing, degraded *configv1.ClusterOperatorStatusCondition

	for i := range co.Status.Conditions {
		cond := &co.Status.Conditions[i]
		//nolint:exhaustive // We only need to check these three core conditions
		switch cond.Type {
		case configv1.OperatorAvailable:
			available = cond
		case configv1.OperatorProgressing:
			progressing = cond
		case configv1.OperatorDegraded:
			degraded = cond
		}
	}

	Expect(available).NotTo(BeNil(), "Available condition should be present")
	Expect(available.Status).To(Equal(configv1.ConditionTrue), "ClusterOperator should be Available")

	Expect(progressing).NotTo(BeNil(), "Progressing condition should be present")
	Expect(progressing.Status).To(Equal(configv1.ConditionFalse), "ClusterOperator should not be Progressing")

	Expect(degraded).NotTo(BeNil(), "Degraded condition should be present")
	Expect(degraded.Status).To(Equal(configv1.ConditionFalse), "ClusterOperator should not be Degraded")

	By("Verifying version information is reported")
	Expect(co.Status.Versions).NotTo(BeEmpty(), "ClusterOperator should report version information")
	Expect(co.Status.Versions[0].Version).To(MatchRegexp(`^4\.`), "Version should be a valid OpenShift version")
}

// ItShouldRejectInvalidMachineNamePrefix checks that invalid machine name prefix formats are rejected.
// Migrated from openshift-tests-private OCP-78773.
func ItShouldRejectInvalidMachineNamePrefix(testFramework framework.Framework) {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	By("Getting the current ControlPlaneMachineSet")

	cpms := &machinev1.ControlPlaneMachineSet{}
	key := runtimeclient.ObjectKey{
		Name:      framework.ControlPlaneMachineSetName,
		Namespace: framework.MachineAPINamespace,
	}

	err := k8sClient.Get(ctx, key, cpms)
	Expect(err).NotTo(HaveOccurred())

	By("Attempting to set an invalid machine name prefix with underscore")

	cpmsUpdate := cpms.DeepCopy()
	cpmsUpdate.Spec.MachineNamePrefix = "abcd_0"

	err = k8sClient.Update(ctx, cpmsUpdate)
	Expect(err).To(HaveOccurred(), "Should reject invalid machine name prefix")
	Expect(err.Error()).To(ContainSubstring("lowercase RFC 1123"),
		"Error should mention RFC 1123 validation")

	By("Attempting to set an invalid prefix with uppercase letters")

	err = k8sClient.Get(ctx, key, cpms)
	Expect(err).NotTo(HaveOccurred())

	cpmsUpdate = cpms.DeepCopy()
	cpmsUpdate.Spec.MachineNamePrefix = "Master-Node"

	err = k8sClient.Update(ctx, cpmsUpdate)
	Expect(err).To(HaveOccurred(), "Should reject uppercase letters in prefix")
}

// ItShouldNotRolloutWhenFailureDomainOrderChanges checks that changing the order of
// failureDomains without changing the zones themselves does not trigger a rollout.
// Migrated from openshift-tests-private OCP-53328.
func ItShouldNotRolloutWhenFailureDomainOrderChanges(testFramework framework.Framework) {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	// Skip for platforms without failureDomain support
	platform := testFramework.GetPlatformType()
	if platform != configv1.AWSPlatformType &&
		platform != configv1.AzurePlatformType &&
		platform != configv1.GCPPlatformType {
		Skip("Test only applicable to AWS, Azure, and GCP platforms")
	}

	By("Getting the current ControlPlaneMachineSet")

	cpms := &machinev1.ControlPlaneMachineSet{}
	key := runtimeclient.ObjectKey{
		Name:      framework.ControlPlaneMachineSetName,
		Namespace: framework.MachineAPINamespace,
	}

	err := k8sClient.Get(ctx, key, cpms)
	Expect(err).NotTo(HaveOccurred())

	failureDomains := getFailureDomainsFromCPMS(cpms, platform)
	if len(failureDomains) <= 1 {
		Skip("Test requires multiple failure domains")
	}

	By("Recording current machine names")

	initialMachineNames := recordInitialMachineNames(ctx, k8sClient)

	By("Temporarily switching to OnDelete to prevent automatic rollout")

	originalStrategy := EnsureControlPlaneMachineSetUpdateStrategy(testFramework, machinev1.OnDelete)
	defer EnsureControlPlaneMachineSetUpdateStrategy(testFramework, originalStrategy)

	By("Changing the order of failure domains")
	changeFailureDomainsOrderInCPMS(cpms, platform)

	By("Switching back to RollingUpdate after a brief pause")
	time.Sleep(10 * time.Second)
	EnsureControlPlaneMachineSetUpdateStrategy(testFramework, machinev1.RollingUpdate)

	By("Verifying that no machines are replaced (names remain the same)")
	verifyMachinesNotReplaced(ctx, k8sClient, initialMachineNames)

	By("Verifying ControlPlaneMachineSet remains up to date")
	EnsureControlPlaneMachineSetUpdated(testFramework)
}

// ItShouldReplaceMachineInRemovedZone verifies that a machine with the same suffix as the given machine
// has been moved to a different zone after the zone was removed from the ControlPlaneMachineSet.
// It ensures the old machine is deleted and only the new machine in a different zone remains.
func ItShouldReplaceMachineInRemovedZone(testFramework framework.Framework, zoneToRemove *string, machineInZone *string) {
	It("should replace the machine in removed zone with one in another zone", func() {
		ctx := testFramework.GetContext()
		k8sClient := testFramework.GetClient()
		timeout := 30 * time.Minute

		machineSuffix := getMachineSuffixFromName(*machineInZone)

		By("Verifying the machine in removed zone is replaced in another zone and old machine is deleted")
		WaitForMachineReplacedInDifferentZone(ctx, k8sClient, machineSuffix, *zoneToRemove, timeout)

		By("Waiting for cluster to stabilize")
		EventuallyClusterOperatorsShouldStabilise(2*time.Minute, 32*time.Minute, 30*time.Second)
	})
}

// ItShouldStabilizeAfterFailureDomainRestoration verifies that the cluster stabilizes
// after the failureDomain is restored to the ControlPlaneMachineSet.
// It first ensures the controller has observed the spec change, then waits for any rollout
// to complete (if needed), and finally waits for cluster operators to stabilize.
func ItShouldStabilizeAfterFailureDomainRestoration(testFramework framework.Framework) {
	It("should stabilize after failureDomain restoration", func() {
		ctx := testFramework.GetContext()
		k8sClient := testFramework.GetClient()
		timeout := 30 * time.Minute

		rolloutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cpms := &machinev1.ControlPlaneMachineSet{}
		Expect(k8sClient.Get(rolloutCtx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed())

		// Capture the current generation to ensure we wait for the controller to observe the change
		currentGeneration := cpms.Generation

		By("Waiting for controller to observe the failureDomain restoration")
		// Wait for ObservedGeneration to match the current Generation, ensuring the controller
		// has processed the spec change and updated the status accordingly.
		// Use a 3 minute timeout for this check - the controller should reconcile within seconds,
		// but we allow extra time for slow environments.
		Eventually(komega.Object(cpms), 3*time.Minute).WithContext(rolloutCtx).Should(
			HaveField("Status.ObservedGeneration", Equal(currentGeneration)),
			"controller should observe the spec change",
		)

		By("Waiting for rollout to complete after failureDomain restoration")
		WaitForControlPlaneMachineSetDesiredReplicas(rolloutCtx, cpms)

		By("Waiting for cluster to stabilize")
		EventuallyClusterOperatorsShouldStabilise(2*time.Minute, 32*time.Minute, 30*time.Second)
	})
}

// ItShouldDeleteMachineAndVerifyReplacementInOnDeleteMode deletes a machine in the removed zone
// and verifies that a replacement machine is created in another zone in OnDelete mode.
// It ensures the old machine is deleted and only the new machine in a different zone remains.
func ItShouldDeleteMachineAndVerifyReplacementInOnDeleteMode(testFramework framework.Framework, zoneToRemove *string, machineInZone *string) {
	It("should create replacement in another zone after machine deletion", func() {
		ctx := testFramework.GetContext()
		k8sClient := testFramework.GetClient()
		timeout := 30 * time.Minute

		machineSuffix := getMachineSuffixFromName(*machineInZone)

		By("Deleting the machine in the removed zone: " + *machineInZone)

		machine := &machinev1beta1.Machine{}
		machineKey := runtimeclient.ObjectKey{Name: *machineInZone, Namespace: framework.MachineAPINamespace}
		err := k8sClient.Get(ctx, machineKey, machine)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Delete(ctx, machine)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying replacement machine is created in another zone and old machine is deleted")
		WaitForMachineReplacedInDifferentZone(ctx, k8sClient, machineSuffix, *zoneToRemove, timeout)

		By("Waiting for cluster to stabilize")
		EventuallyClusterOperatorsShouldStabilise(2*time.Minute, 32*time.Minute, 30*time.Second)
	})
}
