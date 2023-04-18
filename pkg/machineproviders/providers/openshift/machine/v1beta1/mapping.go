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

package v1beta1

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// errReplicasRequired is used to inform users that the replicas field is currently unset, and
	// must be set to continue operation.
	errReplicasRequired = errors.New("spec.replicas is unset: replicas is required")

	// errNoFailureDomains is used to indicate that no failure domain mapping is required in the
	// provider because no failure domains are configured on the ControlPlaneMachineSet.
	errNoFailureDomains = errors.New("no failure domains configured")
)

// mapMachineIndexesToFailureDomains creates a mapping of the given failure domains into an index that can be used
// to by external code to create new Machines in the same failure domain. It should start with a basic mapping and
// then use existing Machine information to map failure domains, if possible, so that the Machine names match the
// index of the failure domain in which they currently reside.
func mapMachineIndexesToFailureDomains(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet, failureDomains []failuredomain.FailureDomain) (map[int32]failuredomain.FailureDomain, error) {
	if len(failureDomains) == 0 {
		logger.V(4).Info("No failure domains provided")

		return nil, errNoFailureDomains
	}

	machineMapping, deletingIndexes, err := createMachineMapping(ctx, logger, cl, cpms)
	if err != nil {
		return nil, fmt.Errorf("could not construct machine mapping: %w", err)
	}

	failureDomainsSet := failuredomain.NewSet(failureDomains...)

	baseMapping, err := createBaseFailureDomainMapping(cpms, failureDomainsSet.List(), machineMapping)
	if err != nil {
		return nil, fmt.Errorf("could not construct base failure domain mapping: %w", err)
	}

	out := reconcileMappings(logger, baseMapping, machineMapping, deletingIndexes)

	logger.V(4).Info(
		"Mapped provided failure domains",
		"mapping", fmt.Sprintf("%v", out),
	)

	return out, nil
}

// createBaseFailureDomainMapping is used to create the basic failure domain mapping based on the number of failure
// domains provided and the number of replicas within the ControlPlaneMachineSet or control plane Machine indexes.
// To ensure consistency, we expect the function to create a stable output no matter the order of the input failure
// domains.
// Create the output based on the longer of the number of Machines or replicas so that when we reconcile the machine
// mappings we always have enough candidates which are balanced between the available failure domains.
func createBaseFailureDomainMapping(cpms *machinev1.ControlPlaneMachineSet, failureDomains []failuredomain.FailureDomain, machineMapping map[int32]failuredomain.FailureDomain) (map[int32]failuredomain.FailureDomain, error) {
	out := make(map[int32]failuredomain.FailureDomain)

	if cpms.Spec.Replicas == nil || *cpms.Spec.Replicas < 1 {
		return nil, errReplicasRequired
	}

	machineIndexCount := len(machineMapping)
	machineFailureDomains := failuredomain.NewSet()

	for _, failureDomain := range machineMapping {
		machineFailureDomains.Insert(failureDomain)
	}

	// Create a base mapping which account for the larger of the number of machines or
	// the desired replica count.
	if *cpms.Spec.Replicas > int32(machineIndexCount) {
		machineIndexCount = int(*cpms.Spec.Replicas)
	}

	if len(failureDomains) == 0 {
		return nil, errNoFailureDomains
	}

	// Sort failure domains alphabetically
	sort.Slice(failureDomains, func(i, j int) bool { return failureDomains[i].String() < failureDomains[j].String() })

	// Deprioritise any failure domain that is not present in the machine mapping.
	sort.Slice(failureDomains, func(i, j int) bool {
		return machineFailureDomains.Has(failureDomains[i]) && !machineFailureDomains.Has(failureDomains[j])
	})

	for i := int32(0); i < int32(machineIndexCount); i++ {
		out[i] = failureDomains[i%int32(len(failureDomains))]
	}

	return out, nil
}

// createMachineMapping inspects the state of the Machines on the cluster, selected by the ControlPlaneMachineSet, and
// creates a mapping of their indexes (if available) to their failure domain to allow the mapping to be customised
// to the state of the cluster.
func createMachineMapping(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet) (map[int32]failuredomain.FailureDomain, sets.Set[int32], error) {
	selector, err := metav1.LabelSelectorAsSelector(&cpms.Spec.Selector)
	if err != nil {
		return nil, nil, fmt.Errorf("could not convert label selector to selector: %w", err)
	}

	machineList := &machinev1beta1.MachineList{}
	if err := cl.List(ctx, machineList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, nil, fmt.Errorf("failed to list machines: %w", err)
	}

	mapping, err := mapIndexesToFailureDomainsForMachines(logger, machineList)
	if err != nil {
		return nil, nil, fmt.Errorf("could not map indexes to failure domains for machines: %w", err)
	}

	deletingIndexes := listDeletingIndexes(machineList.Items)

	return mapping, deletingIndexes, nil
}

// mapIndexesToFailureDomainsForMachines creates an index to failure domain mapping for machine in the list.
func mapIndexesToFailureDomainsForMachines(logger logr.Logger, machineList *machinev1beta1.MachineList) (map[int32]failuredomain.FailureDomain, error) {
	out := make(map[int32]failuredomain.FailureDomain)

	// indexToMachine contains a mapping between the machine domain index in the newest machine
	// for this particular index.
	indexToMachine := make(map[int32]machinev1beta1.Machine)

	for _, machine := range machineList.Items {
		failureDomain, err := providerconfig.ExtractFailureDomainFromMachine(machine)
		if err != nil {
			return nil, fmt.Errorf("could not extract failure domain from machine %s: %w", machine.Name, err)
		}

		machineNameIndex, ok := parseMachineNameIndex(machine.Name)
		if !ok {
			// Ignore the machine as it doesn't contain an index in its name.
			logger.V(4).Info(
				"Ignoring machine in failure domain mapping with unexpected name",
				"machine", machine.Name,
			)

			continue
		}

		if fd, ok := out[int32(machineNameIndex)]; ok && fd.String() != failureDomain.String() {
			oldMachine := indexToMachine[int32(machineNameIndex)]

			if oldMachine.CreationTimestamp.After(machine.CreationTimestamp.Time) {
				continue
			}

			oldMachineFailureDomain, err := providerconfig.ExtractFailureDomainFromMachine(oldMachine)
			if err != nil {
				return nil, fmt.Errorf("could not extract failure domain from machine %s: %w", oldMachine.Name, err)
			}

			logger.V(4).Info(
				"Conflicting failure domains found for the same index, relying on the newer machine",
				"oldMachine", oldMachine.Name,
				"oldFaliureDomain", oldMachineFailureDomain.String(),
				"newerMachine", machine.Name,
				"newerFailureDomain", failureDomain.String(),
			)
		}

		out[int32(machineNameIndex)] = failureDomain

		indexToMachine[int32(machineNameIndex)] = machine
	}

	return out, nil
}

// listDeletingIndexes creates a list of indexes for machines which are being deleted.
// Indexes in the list only have machines in them that are being deleted.
func listDeletingIndexes(machines []machinev1beta1.Machine) sets.Set[int32] {
	indexes := sets.New[int32]()

	// Add all machines that are being deleted to the set.
	for _, machine := range machines {
		if !machine.DeletionTimestamp.IsZero() {
			index, ok := parseMachineNameIndex(machine.Name)
			if !ok {
				continue
			}

			indexes.Insert(int32(index))
		}
	}

	// Remove any index that has a non-deleting machine.
	for _, machine := range machines {
		if machine.DeletionTimestamp.IsZero() {
			index, ok := parseMachineNameIndex(machine.Name)
			if !ok {
				continue
			}

			indexes.Delete(int32(index))
		}
	}

	return indexes
}

// reconcileMappings takes a base mapping and a machines mapping and reconciles the differences. If any machine failure
// domain has an identical failure domain in the base mapping, the mapping from the Machine should take precedence.
// This works by starting with the machine mapping and identifying where in the base mapping (candidates) the failure
// domains can be matched. If any index isn't matched this can then be handled later.
// When matching the indexes, it's important to swap the index to match the machine index to ensure any missing index
// from the Machine mapping is handled later in the unmatched index processing.
// When processing the indexes, everything must be sorted to ensure the output is stable (note iterating over a map
// is randomised by golang).
// The base mapping should always be at least as long as the machine mapping for this to work.
func reconcileMappings(logger logr.Logger, base, machines map[int32]failuredomain.FailureDomain, deletingIndexes sets.Set[int32]) map[int32]failuredomain.FailureDomain {
	if len(base) < len(machines) {
		// This is a programming error since user input doesn't affect this.
		panic("base must have at least as many indexes as machines")
	}

	// Create the initial output mapping based on the machines.
	out := copyMapping(machines)

	// Create the list of candidates based on the base mapping.
	// These will be matched against the machine mapping.
	candidates := copyMapping(base)

	// Ensure that, if the machines aren't indexed from 0, the candidate mapping reflects
	// the indexes present in the machine as opposed to the base mapping which will always
	// index from 0.
	reconcileIndexes(candidates, out)

	// We need to keep track of the indexes we haven't yet matched.
	// Create a list of unmatched indexes.
	unmatchedIndexes := createUnmatchedIndexes(candidates)

	// Run through the mappings and match these to candidates where possible.
	matchMachinesToCandidates(out, candidates, unmatchedIndexes, deletingIndexes)

	// Get the maximum number of replicas per failure domain. This is needed
	// to ensure we balance appropriately across the available failure domains.
	maxPerFailureDomain := maxIndexesPerFailureDomain(base)

	// Handle any remaining unmatched indexes.
	for _, idx := range sortedIndexes(unmatchedIndexes) {
		handleUnmatchedIndex(logger, idx, out, base, candidates, unmatchedIndexes, maxPerFailureDomain)
	}

	return out
}

// createUnmatchedIndexes creates a set of indexes that haven't been matched to a machine
// from the list of candidates.
func createUnmatchedIndexes(candidates map[int32]failuredomain.FailureDomain) sets.Set[int32] {
	out := sets.New[int32]()

	for idx := range candidates {
		out.Insert(idx)
	}

	return out
}

// matchMachinesToCandidates runs through the output mapping and looks for a candidate that has an equal failure domain.
// If the candidate index differs from the output index, swap these and then use the candidate.
// This matches and cements this index to a particular failure domain.
// Any unmatched indexes will be handled separately later.
func matchMachinesToCandidates(out, candidates map[int32]failuredomain.FailureDomain, unmatchedIndexes, deletingIndexes sets.Set[int32]) {
	for _, idy := range sortedIndexes(out) {
		// Ignore any indexes that only contain deleting machines.
		// This allows other indexes to take precedence for keeping their failure domain stable.
		// This is important for the case where a machine is being deleted and a the failure domains
		// definition has changed to include more failure domains.
		if deletingIndexes.Has(idy) {
			continue
		}

		for _, idx := range sortedIndexes(candidates) {
			if out[idy].Equal(candidates[idx]) {
				if idx != idy {
					swapIndexes(candidates, idx, idy)
				}

				useCandidate(candidates, unmatchedIndexes, idy)

				break
			}
		}
	}
}

// handleUnmatchedIndex is used to assess what should be done with an index that doesn't match with the Machine mapping.
// They may not have matched originally for one of the following reasons:
// - There's no machine mapping for that index.
// - The failure domain from the machine mapping was removed from the base.
// - A new failure domain was added to the base mapping.
// - The machine mapping is balanced in a different weighting to the machine mapping.
func handleUnmatchedIndex(logger logr.Logger, idx int32, out, base, candidates map[int32]failuredomain.FailureDomain, unmatchedIndexes sets.Set[int32], maxPerFailureDomain int) {
	switch {
	case !indexExists(out, idx):
		// There is no machine in this index presently,
		// so just use the candidate for this index.
		out[idx] = candidates[idx]
		useCandidate(candidates, unmatchedIndexes, idx)
	case !contains(base, out[idx]):
		// The mapped failure domain no longer exists in the base,
		// this likely means the failure domains on the CPMS have been changed and this failure domain has been removed.
		// Use the candidate instead going forward.
		logger.V(4).Info(
			"Ignoring unknown failure domain",
			"index", int(idx),
			"failureDomain", out[idx].String(),
		)

		out[idx] = candidates[idx]
		useCandidate(candidates, unmatchedIndexes, idx)
	case countForFailureDomain(out, out[idx]) > maxPerFailureDomain:
		// This failure domain is over represented in the mapping.
		// In this case, we must switch it to the candidate failure domain to rebalance
		// the mapping.
		logger.V(4).Info(
			"Failure domain changed for index",
			"index", int(idx),
			"oldFailureDomain", out[idx].String(),
			"newFailureDomain", candidates[idx].String(),
		)

		out[idx] = candidates[idx]
		useCandidate(candidates, unmatchedIndexes, idx)
	default:
		// The index exists, the failure domain is contained in the base,
		// and is represented in the mapping fewer than maxPerFailureDomain times.
		// In this case, it's ok to accept the mapping even though it doesn't match
		// a candidate.
		// This is likely to happen if the machine mapping is balanced using a
		// different weighting to the base, eg aabbc vs abbcc.
		useCandidate(candidates, unmatchedIndexes, idx)
	}
}

// reconcileIndexes tries to adjust the output mapping based on a list of preferred
// indexes. This is used so that when Machines aren't indexed exactly as in the base
// mapping, we still respect their original mappings.
// For example, if the output mapping starts as [0:a, 1:b, 2:c] and the preferred
// mapping contains keys 2, 3, 4, then the output after this function would be
// [3:a, 4:b, 2:c], note 2 in this case was fixed as there was overlap.
func reconcileIndexes(reconciled, preferred map[int32]failuredomain.FailureDomain) {
	nonPreferredIndexes := extraIndexes(reconciled, preferred)
	nonReconciledIndexes := extraIndexes(preferred, reconciled)

	// For each index that isn't in the preferred list, swap it for one
	// that isn't in the reconciled list if there are any.
	for _, idx := range nonPreferredIndexes {
		if len(nonReconciledIndexes) == 0 {
			break
		}

		var newIdx int32
		newIdx, nonReconciledIndexes = pop(nonReconciledIndexes)

		reconciled[newIdx] = reconciled[idx]
		delete(reconciled, idx)
	}
}

// useCandidate is used when we have matched a candidate with a machine mapping. It removes it from the list of
// candidates and "matches" the index.
func useCandidate(candidates map[int32]failuredomain.FailureDomain, unmatchedIndexes sets.Set[int32], idx int32) {
	unmatchedIndexes.Delete(idx)
	delete(candidates, idx)
}

// contains checks if there is a failure domain in the map.
func contains(s map[int32]failuredomain.FailureDomain, e failuredomain.FailureDomain) bool {
	for _, a := range s {
		if a.Equal(e) {
			return true
		}
	}

	return false
}

// sortedIndexes looks at a map of int32 to anything and returns a sorted list of the keys.
func sortedIndexes[V any](mapping map[int32]V) []int32 {
	out := []int32{}

	for idx := range mapping {
		out = append(out, idx)
	}

	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})

	return out
}

// indexExists checks whether an index exists within a map.
func indexExists(mapping map[int32]failuredomain.FailureDomain, idx int32) bool {
	_, ok := mapping[idx]
	return ok
}

// swapIndexes swaps the items in the given indexes within the mapping.
func swapIndexes(mapping map[int32]failuredomain.FailureDomain, x, y int32) {
	mapping[x], mapping[y] = mapping[y], mapping[x]
}

// copyMapping creates a new map with a copy of the keys and values from the source mapping.
func copyMapping(mapping map[int32]failuredomain.FailureDomain) map[int32]failuredomain.FailureDomain {
	out := make(map[int32]failuredomain.FailureDomain)

	for idx, val := range mapping {
		out[idx] = val
	}

	return out
}

// maxIndexesPerFailureDomain is used to calculate the maximum number of allowed indexes per failure domain.
// That is, based on how many failure domains are in the base mapping and the total number of indexes, to
// create a balanced mapping, what is the maximum number of Machines we want to create per failure domain.
func maxIndexesPerFailureDomain(base map[int32]failuredomain.FailureDomain) int {
	uniqueFailureDomains := countUniqueFailureDomain(base)

	// To get an accurate division we must work in floats.
	d := float64(len(base)) / float64(uniqueFailureDomains)

	return int(math.Ceil(d))
}

// countUniqueFailureDomain calculates the number of unique failure domains present within the passed mapping.
func countUniqueFailureDomain(mapping map[int32]failuredomain.FailureDomain) int {
	out := make(map[failuredomain.FailureDomain]struct{})

	for _, failureDomain := range mapping {
		matched := false

		for k := range out {
			if k.Equal(failureDomain) {
				matched = true
				break
			}
		}

		if !matched {
			// This is a new failure domain, make sure it's accounted for.
			out[failureDomain] = struct{}{}
		}
	}

	return len(out)
}

// countForFailureDomain counts how many times the target failure domain is present in the mapping.
func countForFailureDomain(mapping map[int32]failuredomain.FailureDomain, target failuredomain.FailureDomain) int {
	count := 0

	for _, failureDomain := range mapping {
		if failureDomain.Equal(target) {
			count++
		}
	}

	return count
}

// extraIndexes returns a list of indexes in the set that are not present in the superset.
// This is used to identify entries which are not mapped in the superset.
// The output of this function is sorted in ascending order to provide stable output.
func extraIndexes(set, superset map[int32]failuredomain.FailureDomain) []int32 {
	out := []int32{}

	for idx := range set {
		if _, ok := superset[idx]; !ok {
			out = append(out, idx)
		}
	}

	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})

	return out
}

// pop removes the first entry from a slice and returns the remaining slice.
func pop[V any](in []V) (V, []V) {
	return in[0], in[1:]
}

// parseMachineNameIndex returns an integer suffix from the machine name. If there is no sufficient suffix, it
// returns "false" as a second value.
// Example:
//
//	machine-master-3 -> 3, true
//	machine-master-a -> 0, false
//	machine-master3  -> 0 , false
func parseMachineNameIndex(machineName string) (int, bool) {
	machineNameIndex, err := strconv.ParseInt(machineName[strings.LastIndex(machineName, "-")+1:], 10, 32)
	if err != nil {
		return 0, false
	}

	return int(machineNameIndex), true
}
