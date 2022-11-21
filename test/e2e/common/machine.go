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
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

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

	if ok := checkReplacementMachineName(newMachine, idx); !ok {
		return false
	}

	By("Replacement machine name is correct")

	// Check the the old machine doesn't have a deletion timestamp until the new machine is running.
	if ok := waitForNewMachineRunning(ctx, oldMachine, newMachine); !ok {
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

// EventuallyIndexIsBeingReplaced checks that the index given, eventually receives
// a replacement Machine.
// It simultaneously checks that no other index is being replaced, this short circuits
// what otherwise could be a long wait.
func EventuallyIndexIsBeingReplaced(ctx context.Context, idx int) bool {
	controlPlaneMachineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	// Wait up to a minute for the replacement machine to be created.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// Use RunCheckUntil to check that the other indexes do not get an additional
	// replacement while we wait for the desired index to get a replacement.
	passed := framework.RunCheckUntil(ctx,
		func(_ context.Context, g framework.GomegaAssertions) bool { // Check function
			By(fmt.Sprintf("Checking that other indexes (not %d) do not have 2 replicas", idx))
			list := komega.ObjectList(&machinev1beta1.MachineList{}, controlPlaneMachineSelector)

			return g.Expect(list()).Should(
				HaveField("Items", WithTransform(extractMachineIndexCounts, Not(HaveKeyWithValue(Not(Equal(idx)), 2)))), fmt.Sprintf("expected not to have 2 replicas for index other than index %d", idx),
			)
		},
		func(_ context.Context, g framework.GomegaAssertions) bool { // Until function
			By(fmt.Sprintf("Checking that index %d has 2 replicas", idx))
			list := komega.ObjectList(&machinev1beta1.MachineList{}, controlPlaneMachineSelector)

			return g.Expect(list()).Should(
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
func checkReplacementMachineName(machine *machinev1beta1.Machine, idx int) bool {
	if ok := Expect(machine.ObjectMeta.Labels).To(HaveKey(machineClusterIDLabel), "machine should have cluster label"); !ok {
		return false
	}

	clusterID := machine.ObjectMeta.Labels[machineClusterIDLabel]
	// Check that the replacement machine has the same name as the old machine.
	return Expect(machine.ObjectMeta.Name).To(MatchRegexp("%s-master-[a-z0-9]{5}-%d", clusterID, idx), "replacement machine name should match the expected format")
}

// waitForNewMachineRunning checks that the new Machine eventually becomes running.
// While it checks this, it also checks that the old Machine does not get a deletion timestamp.
// During a rolling update we expect the old machine to only be deleted after the new Machine
// becomes running.
func waitForNewMachineRunning(ctx context.Context, oldMachine, newMachine *machinev1beta1.Machine) bool {
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	By("Waiting for the new machine become Running")
	By("Checking that the old machine is not deleted until the new machine is Running")

	// Check the the old machine doesn't have a deletion timestamp until the new machine is running.
	return framework.RunCheckUntil(ctx,
		func(_ context.Context, g framework.GomegaAssertions) bool { // Check function
			list := komega.ObjectList(&machinev1beta1.MachineList{}, machineSelector)

			return g.Expect(list()).Should(HaveField("Items", SatisfyAll(
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
			machine := komega.Object(newMachine)

			return g.Expect(machine()).Should(HaveField("Status.Phase", HaveValue(Equal("Running"))), "expected new machine to be running")
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
