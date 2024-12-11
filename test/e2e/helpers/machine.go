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

package helpers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/features"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

const (
	// machineClusterIDLabel is the label used to identify the cluster a machine belongs to.
	// We use this to check that the Machine name has the expected format.
	machineClusterIDLabel = "machine.openshift.io/cluster-api-cluster"
)

var (
	// errMachineNameFormatInvalid is returned when the machine name does not match the expected format.
	errMachineNameFormatInvalid = errors.New("machine name does not match expected format")

	// errMoreThanOneMachineInIndex is returned when there is more than one machine in the given index.
	errMoreThanOneMachineInIndex = errors.New("more than one control plane machine in index")
)

// CheckControlPlaneMachineRollingReplacement checks that the machines with the given index
// are being replaced in a rolling update style with the expected conditions.
// This function explicitly takes a context which is expected to have a timeout in the parent scope.
func CheckControlPlaneMachineRollingReplacement(testFramework framework.Framework, idx int, ctx context.Context) bool {
	oldMachine, newMachine, ok := getOldAndNewMachineForIndex(ctx, testFramework, idx)
	if !ok {
		return false
	}

	// Check the machines name matches the expected format.
	By("Checking the replacement machine name")

	k8sClient := testFramework.GetClient()
	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	currentFeatureGates, err := newFeatureGateFilter(ctx, testFramework)
	Expect(err).NotTo(HaveOccurred())
	Expect(currentFeatureGates).NotTo(BeNil())

	if ok := checkReplacementMachineName(newMachine, cpms, currentFeatureGates, idx); !ok {
		return false
	}

	By("Replacement machine name is correct")

	// Check the the old machine doesn't have a deletion timestamp until the new machine is running.
	if ok := waitForNewMachineRunning(ctx, testFramework, oldMachine, newMachine); !ok {
		return false
	}

	By("Replacement machine is Running")
	By("Checking that the old machine is marked for deletion")

	if ok := Eventually(komega.Object(oldMachine), ctx).Should(HaveField("ObjectMeta.DeletionTimestamp", Not(BeNil())), "expected old machine to be marked for deletion"); !ok {
		return false
	}

	By("Checking that the old machine is removed")

	if ok := Eventually(komega.Get(oldMachine), ctx).Should(MatchError(ContainSubstring("not found")), "expected old machine to be removed from the cluster"); !ok {
		return false
	}

	By(fmt.Sprintf("Rollout of index %d complete", idx))

	return true
}

// CheckControlPlaneMachineOnDeleteReplacement checks that the machines with the given index
// are being replaced in a on delete style with the expected conditions.
// This function explicitly takes a context which is expected to have a timeout in the parent scope.
func CheckControlPlaneMachineOnDeleteReplacement(testFramework framework.Framework, idx int, ctx context.Context) bool {
	oldMachine, newMachine, ok := getOldAndNewMachineForIndex(ctx, testFramework, idx)
	if !ok {
		return false
	}

	// Check the machines name matches the expected format.
	By("Checking the replacement machine name")

	k8sClient := testFramework.GetClient()
	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	currentFeatureGates, err := newFeatureGateFilter(ctx, testFramework)
	Expect(err).NotTo(HaveOccurred())
	Expect(currentFeatureGates).NotTo(BeNil())

	if ok := checkReplacementMachineName(newMachine, cpms, currentFeatureGates, idx); !ok {
		return false
	}

	By("Replacement machine name is correct")
	By("Waiting for the new machine become Running")

	if ok := Eventually(komega.Object(newMachine), ctx).Should(HaveField("Status.Phase", HaveValue(Equal("Running"))), "expected new machine to be running"); !ok {
		return false
	}

	By("Replacement machine is Running")
	By("Checking that the old machine is removed")

	if ok := Eventually(komega.Get(oldMachine), ctx).Should(MatchError(ContainSubstring("not found")), "expected old machine to be removed from the cluster"); !ok {
		return false
	}

	By(fmt.Sprintf("Rollout of index %d complete", idx))

	return true
}

// EventuallyIndexIsBeingReplaced checks that the index given, eventually receives
// a replacement Machine.
// It simultaneously checks that no other index is being replaced, this short circuits
// what otherwise could be a long wait.
func EventuallyIndexIsBeingReplaced(ctx context.Context, testFramework framework.Framework, idx int) bool {
	controlPlaneMachineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	k8sClient := testFramework.GetClient()

	// Wait for the replacement machine to be created.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Use RunCheckUntil to check that the other indexes do not get an additional
	// replacement while we wait for the desired index to get a replacement.
	passed := framework.RunCheckUntil(ctx,
		func(_ context.Context, g framework.GomegaAssertions) bool { // Check function
			By(fmt.Sprintf("Checking that other indexes (not %d) do not have 2 replicas", idx))

			list := &machinev1beta1.MachineList{}
			if err := k8sClient.List(ctx, list, controlPlaneMachineSelector); err != nil {
				// For temporary errors in listing objects we don't want to break this check,
				// so we return happy and retry at the next check.
				return g.Expect(err).Should(WithTransform(isRetryableAPIError, BeTrue()), "expected temporary error while listing machines: %v", err)
			}

			return g.Expect(list).Should(
				HaveField("Items", WithTransform(extractMachineIndexCounts, Not(HaveKeyWithValue(Not(Equal(idx)), 2)))), fmt.Sprintf("expected not to have 2 replicas for index other than index %d", idx),
			)
		},
		func(_ context.Context, g framework.GomegaAssertions) bool { // Until function
			By(fmt.Sprintf("Checking that index %d has 2 replicas", idx))

			list := &machinev1beta1.MachineList{}
			if err := k8sClient.List(ctx, list, controlPlaneMachineSelector); err != nil {
				// For temporary errors in listing the objects we don't want to break the until condition,
				// so we return false, which is the standard behaviour for this condition when things haven't settled yet.
				return !g.Expect(err).Should(WithTransform(isRetryableAPIError, BeTrue()), "expected temporary error while listing machines: %v", err)
			}

			return g.Expect(list).Should(
				HaveField("Items", WithTransform(extractMachineIndexCounts, HaveKeyWithValue(idx, 2))), fmt.Sprintf("expected 2 replicas for index %d", idx),
			)
		},
	)

	if passed {
		By("Correct index is being replaced")
	}

	return passed
}

// extractMachineIndexCounts returns a map of index to count of machines with that index.
func extractMachineIndexCounts(machines []machinev1beta1.Machine) (map[int]int, error) {
	indexCounts := map[int]int{}

	for _, machine := range machines {
		idx, err := machineIndex(machine)
		if err != nil {
			return nil, fmt.Errorf("machine name %q does not match expected format: %w", machine.Name, err)
		}

		indexCounts[idx]++
	}

	return indexCounts, nil
}

// machineIndex returns the index of the machine, the numeric suffix of the machine name.
func machineIndex(machine machinev1beta1.Machine) (int, error) {
	re := regexp.MustCompile(`^.*-([0-9]+)$`)
	matches := re.FindStringSubmatch(machine.Name)

	if len(matches) != 2 {
		return -1, fmt.Errorf("%w: %s", errMachineNameFormatInvalid, machine.Name)
	}
	// The second element in the matches slice is the first capture group.
	// This should be the final number in the machine name.
	idx, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1, fmt.Errorf("failed to parse machine name suffix: %w", err)
	}

	return idx, nil
}

// getOldAndNewMachineForIndex lists and extracts the old and replacement machines for a given index.
func getOldAndNewMachineForIndex(ctx context.Context, testFramework framework.Framework, idx int) (*machinev1beta1.Machine, *machinev1beta1.Machine, bool) {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	k8sClient := testFramework.GetClient()

	machineList := &machinev1beta1.MachineList{}
	if ok := Expect(k8sClient.List(ctx, machineList, machineSelector)).To(Succeed(), "should be able to list machines"); !ok {
		return nil, nil, false
	}

	// Sort the machines by oldest first so we catch the old machine first and then the new machine.
	sort.Slice(machineList.Items, func(i, j int) bool {
		return machineList.Items[i].ObjectMeta.CreationTimestamp.Before(&machineList.Items[j].ObjectMeta.CreationTimestamp)
	})

	// Extract the old and new machine for the index.
	var oldMachine, newMachine *machinev1beta1.Machine

	for _, machine := range machineList.Items {
		machineIdx, err := machineIndex(machine)
		if err != nil {
			return nil, nil, false
		}

		if machineIdx != idx {
			continue
		}

		if oldMachine == nil {
			oldMachine = machine.DeepCopy()
			continue
		}

		newMachine = machine.DeepCopy()

		break
	}

	if ok := Expect(oldMachine).ToNot(BeNil(), "should have found an old machine for index %d", idx); !ok {
		return nil, nil, false
	}

	if ok := Expect(newMachine).ToNot(BeNil(), "should have found a new machine for index %d", idx); !ok {
		return nil, nil, false
	}

	return oldMachine, newMachine, true
}

// checkReplacementMachineName checks that the name of the replacement machine fits the expected format.
func checkReplacementMachineName(machine *machinev1beta1.Machine, cpms *machinev1.ControlPlaneMachineSet, fg *featureGateFilter, idx int) bool {
	if ok := Expect(machine.ObjectMeta.Labels).To(HaveKey(machineClusterIDLabel), "machine should have cluster label"); !ok {
		return false
	}

	clusterID := machine.ObjectMeta.Labels[machineClusterIDLabel]

	// If CPMSMachineNamePrefix featuregate is enabled and MachineNamePrefix is set,
	// the replacement machine should follow prefixed naming convention.
	allowMachineNamePrefix := fg.isEnabled(string(features.FeatureGateCPMSMachineNamePrefix))
	machineNamePrefix := cpms.Spec.MachineNamePrefix

	if allowMachineNamePrefix && len(machineNamePrefix) > 0 {
		return Expect(machine.ObjectMeta.Name).To(MatchRegexp("%s-[a-z0-9]{5}-%d", machineNamePrefix, idx), "replacement machine name should match the expected prefixed format")
	}

	// Check that the replacement machine has the same name as the old machine.
	return Expect(machine.ObjectMeta.Name).To(MatchRegexp("%s-master-[a-z0-9]{5}-%d", clusterID, idx), "replacement machine name should match the expected format")
}

// waitForNewMachineRunning checks that the new Machine eventually becomes running.
// While it checks this, it also checks that the old Machine does not get a deletion timestamp.
// During a rolling update we expect the old machine to only be deleted after the new Machine
// becomes running.
func waitForNewMachineRunning(ctx context.Context, testFramework framework.Framework, oldMachine, newMachine *machinev1beta1.Machine) bool {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	k8sClient := testFramework.GetClient()

	By("Waiting for the new machine become Running")
	By("Checking that the old machine is not deleted until the new machine is Running")

	// Check the the old machine doesn't have a deletion timestamp until the new machine is running.
	return framework.RunCheckUntil(ctx,
		func(_ context.Context, g framework.GomegaAssertions) bool { // Check function
			machineList := &machinev1beta1.MachineList{}
			if err := k8sClient.List(ctx, machineList, machineSelector); err != nil {
				// For temporary errors in listing objects we don't want to break this check,
				// so we return happy and retry at the next check.
				return g.Expect(err).Should(WithTransform(isRetryableAPIError, BeTrue()), "expected temporary error while listing machines: %v", err)
			}

			return g.Expect(machineList).Should(HaveField("Items", SatisfyAll(
				ContainElement(SatisfyAll(
					HaveField("ObjectMeta.Name", Equal(oldMachine.ObjectMeta.Name)),
					HaveField("ObjectMeta.DeletionTimestamp", BeNil()),
				)),
				ContainElement(SatisfyAll(
					HaveField("ObjectMeta.Name", Equal(newMachine.ObjectMeta.Name)),
					HaveField("Status.Phase", SatisfyAny(
						BeNil(), // HaveValue errors when the value is nil, use this as a workaround.
						Not(HaveValue(Equal("Running"))),
					)),
				)),
			)), "expected old machine to not be deleted until new machine is Running")
		},
		func(_ context.Context, g framework.GomegaAssertions) bool { // Condition function
			machineKey := runtimeclient.ObjectKey{Namespace: newMachine.Namespace, Name: newMachine.Name}

			machine := &machinev1beta1.Machine{}
			if err := k8sClient.Get(ctx, machineKey, machine); err != nil {
				// For temporary errors in getting the object we don't want to break the until condition,
				// so we return false, which is the standard behaviour for this condition when things haven't settled yet.
				return !g.Expect(err).Should(WithTransform(isRetryableAPIError, BeTrue()), "expected temporary error while listing machines: %v", err)
			}

			return g.Expect(machine).Should(HaveField("Status.Phase", HaveValue(Equal("Running"))), "expected new machine to be running")
		},
	)
}

// ExpectControlPlaneMachinesAllRunning checks that all the control plane machines
// are in running phase.
func ExpectControlPlaneMachinesAllRunning(testFramework framework.Framework) {
	By("Checking the control plane machines are all in running phase")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	machineList := &machinev1beta1.MachineList{}

	Eventually(komega.ObjectList(machineList, machineSelector)).Should(HaveField("Items",
		HaveEach(HaveField("Status.Phase", HaveValue(Equal("Running")))),
	), "expected all of the control plane machines to be in running phase")
}

// ExpectControlPlaneMachinesNotOwned checks that none of the control plane machines
// have owner references.
func ExpectControlPlaneMachinesNotOwned(testFramework framework.Framework) {
	By("Checking that none of the control plane machines have owner references")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	machineList := &machinev1beta1.MachineList{}

	Eventually(komega.ObjectList(machineList, machineSelector)).Should(HaveField("Items",
		HaveEach(HaveField("ObjectMeta.OwnerReferences", HaveLen(0))),
	), "expected none of the control plane machines to have owner references")
}

// ExpectControlPlaneMachinesWithoutDeletionTimestamp checks that none of the control plane machines
// has a deletion timestamp.
func ExpectControlPlaneMachinesWithoutDeletionTimestamp(testFramework framework.Framework) {
	By("Checking that none of the control plane machines have a deletion timestamp")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	machineList := &machinev1beta1.MachineList{}

	Eventually(komega.ObjectList(machineList, machineSelector)).Should(HaveField("Items",
		HaveEach(HaveField("ObjectMeta.DeletionTimestamp", BeNil())),
	), "expected none of the control plane machines to have a deletionTimestap")
}

// ConsistentlyControlPlaneMachinesWithoutDeletionTimestamp checks that none of the control plane machines
// have a deletion timestamp, consistently.
func ConsistentlyControlPlaneMachinesWithoutDeletionTimestamp(testFramework framework.Framework) {
	By("Checking that none of the control plane machines have a deletion timestamp")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	machineList := &machinev1beta1.MachineList{}

	Consistently(komega.ObjectList(machineList, machineSelector)).Should(HaveField("Items",
		HaveEach(HaveField("ObjectMeta.DeletionTimestamp", BeNil())),
	), "expected none of the control plane machines to have a deletionTimestap")
}

// ExpectControlPlaneMachinesOwned checks that all of the control plane machines
// have owner references.
func ExpectControlPlaneMachinesOwned(testFramework framework.Framework) {
	By("Checking that all of the control plane machines have owner references")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	machineList := &machinev1beta1.MachineList{}

	Eventually(komega.ObjectList(machineList, machineSelector)).Should(HaveField("Items",
		Not(ContainElement(HaveField("ObjectMeta.OwnerReferences", BeEmpty()))),
	), "expected none of the control plane machines to not have an owner reference")
}

// IncreaseControlPlaneMachineInstanceSize increases the instance size of the control plane machine
// in the given index. This should trigger the control plane machine set to update the machine in
// this index based on the update strategy.
func IncreaseControlPlaneMachineInstanceSize(testFramework framework.Framework, index int, gomegaArgs ...interface{}) (machinev1beta1.ProviderSpec, machinev1beta1.ProviderSpec) {
	machine, err := machineForIndex(testFramework, index)
	Expect(err).ToNot(HaveOccurred(), "control plane machine should exist")

	originalProviderSpec := machine.Spec.ProviderSpec

	updatedProviderSpec := originalProviderSpec.DeepCopy()
	platformType := testFramework.GetPlatformType()

	switch platformType {
	case configv1.OpenStackPlatformType:
		// OpenStack flavors are not predictable. So if OPENSTACK_CONTROLPLANE_FLAVOR_ALTERNATE is set in the environment, we'll use it
		// to change the instance flavor, otherwise we just tag the instance with a new tag, which will trigger the redeployment.
		if os.Getenv("OPENSTACK_CONTROLPLANE_FLAVOR_ALTERNATE") == "" {
			Expect(testFramework.TagInstanceInProviderSpec(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with a new tag")
		} else {
			Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")
		}
	default:
		Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")
	}

	By(fmt.Sprintf("Updating the provider spec of the control plane machine at index %d", index))

	Eventually(komega.Update(machine, func() {
		machine.Spec.ProviderSpec = *updatedProviderSpec
	}), gomegaArgs...).Should(Succeed(), "control plane machine should be able to be updated")

	return originalProviderSpec, machine.Spec.ProviderSpec
}

// UpdateControlPlaneMachineProviderSpec updates the provider spec of the control plane machine in the given index
// to match the provider spec given.
func UpdateControlPlaneMachineProviderSpec(testFramework framework.Framework, index int, updatedProviderSpec machinev1beta1.ProviderSpec, gomegaArgs ...interface{}) {
	By(fmt.Sprintf("Updating the provider spec of the control plane machine at index %d", index))

	machine, err := machineForIndex(testFramework, index)
	Expect(err).ToNot(HaveOccurred(), "control plane machine should exist")

	Eventually(komega.Update(machine, func() {
		machine.Spec.ProviderSpec = updatedProviderSpec
	}), gomegaArgs...).Should(Succeed(), "control plane machine should be able to be updated")
}

// IncreaseNewestControlPlaneMachineInstanceSize increases the instance size of the the newest control plane machine
// to match the provider spec given.
func IncreaseNewestControlPlaneMachineInstanceSize(testFramework framework.Framework, gomegaArgs ...interface{}) (int, machinev1beta1.ProviderSpec, machinev1beta1.ProviderSpec) {
	index, err := newestMachineIndex(testFramework)
	Expect(err).ToNot(HaveOccurred(), "control plane newest machine index should be found")

	originalProviderSpec, updatedProviderSpec := IncreaseControlPlaneMachineInstanceSize(testFramework, index)

	return index, originalProviderSpec, updatedProviderSpec
}

// newestMachineIndex returns the index of the newest (latest .metadata.creationTimestamp) control plane machine.
// If multiple machines have the same creationTimestamp, the index of the one with the alphabetically greater name is picked.
func newestMachineIndex(testFramework framework.Framework) (int, error) {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
	machineList := &machinev1beta1.MachineList{}

	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	if err := k8sClient.List(ctx, machineList, machineSelector); err != nil {
		return 0, fmt.Errorf("could not list control plane machines: %w", err)
	}

	sortedMachines := sortMachinesByCreationTimeDescending(machineList.Items)
	newestMachine := sortedMachines[0]

	index, err := machineIndex(newestMachine)
	if err != nil {
		return 0, fmt.Errorf("could not get control plane machine index: %w", err)
	}

	return index, nil
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

// sortMachinesByCreationTimeDescending sorts a slice of Machines by CreationTime, Name (descending).
func sortMachinesByCreationTimeDescending(machines []machinev1beta1.Machine) []machinev1beta1.Machine {
	// Sort in inverse order so that the newest one is first.
	sort.Slice(machines, func(i, j int) bool {
		first, second := machines[i].CreationTimestamp, machines[j].CreationTimestamp
		if first != second {
			return second.Before(&first)
		}

		return machines[i].Name > machines[j].Name
	})

	return machines
}

// isRetryableAPIError returns whether an API error is retryable or not.
// inspired by: k8s.io/kubernetes/test/utils.
func isRetryableAPIError(err error) bool {
	// These errors may indicate a transient error that we can retry in tests.
	if apierrs.IsInternalError(err) || apierrs.IsTimeout(err) || apierrs.IsServerTimeout(err) ||
		apierrs.IsTooManyRequests(err) || utilnet.IsProbableEOF(err) || utilnet.IsConnectionReset(err) ||
		utilnet.IsHTTP2ConnectionLost(err) {
		return true
	}

	// If the error sends the Retry-After header, we respect it as an explicit confirmation we should retry.
	if _, shouldRetry := apierrs.SuggestsClientDelay(err); shouldRetry {
		return true
	}

	return false
}
