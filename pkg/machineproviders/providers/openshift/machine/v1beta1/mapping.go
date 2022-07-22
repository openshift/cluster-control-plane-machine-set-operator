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

	baseMapping, err := createBaseFailureDomainMapping(cpms, failureDomains)
	if err != nil {
		return nil, fmt.Errorf("could not construct base failure domain mapping: %w", err)
	}

	machineMapping, err := createMachineMapping(ctx, logger, cl, cpms)
	if err != nil {
		return nil, fmt.Errorf("could not construct machine mapping: %w", err)
	}

	reconciledMapping := reconcileMappings(logger, baseMapping, machineMapping)

	rebalancedMachineMapping := rebalanceMachineMapping(logger, reconciledMapping, failureDomains)

	logger.V(4).Info(
		"Mapped provided failure domains",
		"mapping", rebalancedMachineMapping,
	)

	return rebalancedMachineMapping, nil
}

// createBaseFailureDomainMapping is used to create the basic failure domain mapping based on the number of failure
// domains provided and the number of replicas within the ControlPlaneMachineSet.
// To ensure consistency, we expect the function to create a stable output no matter the order of the input failure
// domains.
func createBaseFailureDomainMapping(cpms *machinev1.ControlPlaneMachineSet, failureDomains []failuredomain.FailureDomain) (map[int32]failuredomain.FailureDomain, error) {
	out := make(map[int32]failuredomain.FailureDomain)

	if cpms.Spec.Replicas == nil || *cpms.Spec.Replicas < 1 {
		return nil, errReplicasRequired
	}

	if len(failureDomains) == 0 {
		return nil, errNoFailureDomains
	}

	// Sort failure domains alphabetically
	sort.Slice(failureDomains, func(i, j int) bool { return failureDomains[i].String() < failureDomains[j].String() })

	for i := int32(0); i < *cpms.Spec.Replicas; i++ {
		out[i] = failureDomains[i%int32(len(failureDomains))]
	}

	return out, nil
}

// createMachineMapping inspects the state of the Machines on the cluster, selected by the ControlPlaneMachineSet, and
// creates a mapping of their indexes (if available) to their failure domain to allow the mapping to be customised
// to the state of the cluster.
func createMachineMapping(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet) (map[int32]failuredomain.FailureDomain, error) {
	out := make(map[int32]failuredomain.FailureDomain)

	selector, err := metav1.LabelSelectorAsSelector(&cpms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("could not convert label selector to selector: %w", err)
	}

	machineList := &machinev1beta1.MachineList{}
	if err := cl.List(ctx, machineList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}

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

		if fd, ok := out[int32(machineNameIndex)]; ok && !fd.Equal(failureDomain) {
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

// reconcileMappings takes a base mapping and a machines mapping and reconciles the differences. If any machine failure
// domain has an identical failure domain in the base mapping, the mapping from the Machine should take precedence.
// When overwriting a mapping, the mapping in place must be swapped to avoid losing information.
func reconcileMappings(logger logr.Logger, base, machines map[int32]failuredomain.FailureDomain) map[int32]failuredomain.FailureDomain {
	out := make(map[int32]failuredomain.FailureDomain)
	used := mapToSlice(machines) // Consider all failure domains from machines as used.
	replicas := int32(len(base))

	for i := int32(0); i < replicas; i++ {
		machineFailureDomain, ok := machines[i]
		if !ok {
			// If there is no failure domain specified in "machines", we pick the
			// first unused item from "base". If all "base" domain are used then
			// pick ith element from "base".
			firstUnusedFailureDomain := getFirstUnusedFailureDomain(used, base)
			if firstUnusedFailureDomain == nil {
				out[i] = base[i]
			} else {
				out[i] = *firstUnusedFailureDomain
				used = append(used, *firstUnusedFailureDomain)
			}

			continue
		}

		// If current machine failure domain has been removed from "base", we replace it
		// with ith element from "base".
		if !contains(mapToSlice(base), machineFailureDomain) {
			out[i] = base[i]

			logger.V(4).Info(
				"Ignoring unknown failure domain",
				"index", int(i),
				"failureDomain", machineFailureDomain.String(),
			)

			continue
		}

		// If there are several machines in the same failure domain, we try to replace one
		// with the first unused element from "base". If it's not possible - keep the duplicate.
		if contains(mapToSlice(out), machineFailureDomain) {
			firstUnusedFailureDomain := getFirstUnusedFailureDomain(used, base)
			if firstUnusedFailureDomain == nil {
				out[i] = machineFailureDomain
			} else {
				out[i] = *firstUnusedFailureDomain
				used = append(used, *firstUnusedFailureDomain)

				logger.V(4).Info(
					"Failure domain changed for index",
					"index", int(i),
					"oldFailureDomain", machineFailureDomain.String(),
					"newFailureDomain", out[i].String(),
				)
			}

			continue
		}

		out[i] = machineFailureDomain
	}

	return out
}

// contains checks if there is a failure domain in the slice.
func contains(s []failuredomain.FailureDomain, e failuredomain.FailureDomain) bool {
	for _, a := range s {
		if a.Equal(e) {
			return true
		}
	}

	return false
}

// mapToSlice converts a map of failure domains into a slice.
func mapToSlice(s map[int32]failuredomain.FailureDomain) []failuredomain.FailureDomain {
	d := []failuredomain.FailureDomain{}

	for _, fd := range s {
		d = append(d, fd)
	}

	return d
}

// getFirstUnusedFailureDomain returns the first failure domain from candidates that doesn't exist in the used list.
func getFirstUnusedFailureDomain(used []failuredomain.FailureDomain, candidatesMap map[int32]failuredomain.FailureDomain) *failuredomain.FailureDomain {
	candidates := []failuredomain.FailureDomain{}

	for _, candidate := range candidatesMap {
		candidates = append(candidates, candidate)
	}

	// Sort failure domains alphabetically
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].String() < candidates[j].String() })

	for _, candidate := range candidates {
		if !contains(used, candidate) {
			return &candidate
		}
	}

	return nil
}

// parseMachineNameIndex returns an integer suffix from the machine name. If there is no sufficient suffix, it
// returns "false" as a second value.
// Example:
//   machine-master-3 -> 3, true
//   machine-master-a -> 0, false
//   machine-master3  -> 0, false
func parseMachineNameIndex(machineName string) (int, bool) {
	machineNameIndex, err := strconv.ParseInt(machineName[strings.LastIndex(machineName, "-")+1:], 10, 32)
	if err != nil {
		return 0, false
	}

	return int(machineNameIndex), true
}

// rebalanceMachineMapping replaces domains that are used too much with least used ones.
// Example:
//   c,b,a,c,c -> c,b,a,c,a
//   c,c,c,c,c -> c,c,a,b,a
//   a,b,c,a,b -> a,b,c,a,b (stays the same as it's already balanced)
// Algorithm:
//   1. Calculate the maximum possible number for each failure domain in the output mapping:
//        maxCount := int(math.Ceil(float64(len(machines))/float64(len(failureDomains))))
//   2. Iterate over the mapping and calculate how many occurrences of failure domain we have.
//     2.1 If the current number is still less or equal than the maximum allowed number, we
//         add the domain to the result without change and increase its count.
//     2.2 Otherwise we pick the least used domain and increase its count. If there several
//         domains with the same number of occurrences, we sort them alphabetically and pick
//         the highest one.
func rebalanceMachineMapping(logger logr.Logger, machines map[int32]failuredomain.FailureDomain, failureDomains []failuredomain.FailureDomain) map[int32]failuredomain.FailureDomain {
	out := make(map[int32]failuredomain.FailureDomain)

	// stringsToFailureDomains is used to quickly convert failure domain string representation into the failure domain.
	stringsToFailureDomains := make(map[string]failuredomain.FailureDomain)

	// failureDomainCount contains the amount for each failure domain in machines.
	// As a key we use failure domain string representation.
	failureDomainCount := make(map[string]int)

	for _, fd := range failureDomains {
		stringsToFailureDomains[fd.String()] = fd
		failureDomainCount[fd.String()] = 0
	}

	// maxCount shows the allowed amount for each failure domain in the mapping.
	maxCount := int(math.Ceil(float64(len(machines)) / float64(len(failureDomains))))

	for i := int32(0); i < int32(len(machines)); i++ {
		fd := machines[i]

		if failureDomainCount[fd.String()] < maxCount {
			// Keep the domain as-is.
			out[i] = fd
			failureDomainCount[fd.String()]++
		} else {
			// Too many occurrences of this particular domain - we have to replace it with
			// the least used one.
			out[i] = stringsToFailureDomains[leastUsedFailureDomain(failureDomainCount)]
			failureDomainCount[out[i].String()]++

			logger.V(4).Info(
				"Failure domain changed for index",
				"index", int(i),
				"oldFailureDomain", fd.String(),
				"newFailureDomain", out[i].String(),
			)
		}
	}

	return out
}

// leastUsedFailureDomain returns the least used failure domain.
// If several domains have the same count, we pick the first alphabetically ordered one.
func leastUsedFailureDomain(failureDomainCount map[string]int) string {
	minCount := math.MaxInt

	var res string

	for item, count := range failureDomainCount {
		if count < minCount || (count == minCount && item < res) {
			minCount = count
			res = item
		}
	}

	return res
}
