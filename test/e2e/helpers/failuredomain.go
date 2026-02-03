/*
Copyright 2024 Red Hat, Inc.

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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

var (
	// ErrInsufficientFailureDomains indicates that the test requires more failure domains than are available.
	ErrInsufficientFailureDomains = errors.New("test requires multiple failure domains")
	// ErrNoZoneWithSingleMachine indicates that no zone was found with exactly one machine.
	ErrNoZoneWithSingleMachine = errors.New("could not find a zone with exactly one machine")
	// ErrPlatformNotSupported indicates that the platform doesn't support failure domains.
	ErrPlatformNotSupported = errors.New("test only applicable to AWS, Azure, and GCP platforms")
)

// extractAWSZones extracts zone names from AWS failure domains.
func extractAWSZones(fds *[]machinev1.AWSFailureDomain) []string {
	var zones []string

	if fds != nil {
		for _, fd := range *fds {
			if fd.Placement.AvailabilityZone != "" {
				zones = append(zones, fd.Placement.AvailabilityZone)
			}
		}
	}

	return zones
}

// extractAzureZones extracts zone names from Azure failure domains.
func extractAzureZones(fds *[]machinev1.AzureFailureDomain) []string {
	var zones []string

	if fds != nil {
		for _, fd := range *fds {
			zones = append(zones, fd.Zone)
		}
	}

	return zones
}

// extractGCPZones extracts zone names from GCP failure domains.
func extractGCPZones(fds *[]machinev1.GCPFailureDomain) []string {
	var zones []string

	if fds != nil {
		for _, fd := range *fds {
			zones = append(zones, fd.Zone)
		}
	}

	return zones
}

// getFailureDomainsFromCPMS extracts the list of zone names from a ControlPlaneMachineSet
// for the specified platform (AWS, Azure, or GCP).
func getFailureDomainsFromCPMS(cpms *machinev1.ControlPlaneMachineSet, platform configv1.PlatformType) []string {
	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil ||
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains == nil {
		return []string{}
	}

	fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains

	switch platform {
	case configv1.AWSPlatformType:
		return extractAWSZones(fds.AWS)
	case configv1.AzurePlatformType:
		return extractAzureZones(fds.Azure)
	case configv1.GCPPlatformType:
		return extractGCPZones(fds.GCP)
	default:
		return []string{}
	}
}

// findZoneWithSingleMachine finds a zone that contains exactly one master machine.
// Returns the zone name and the machine name, or empty strings if no such zone is found.
func findZoneWithSingleMachine(ctx context.Context, k8sClient runtimeclient.Client, zones []string) (string, string) {
	for _, zone := range zones {
		machineList := &machinev1beta1.MachineList{}
		labelSelector := runtimeclient.MatchingLabels{
			"machine.openshift.io/cluster-api-machine-type": "master",
			"machine.openshift.io/zone":                     zone,
		}

		err := k8sClient.List(ctx, machineList, labelSelector, runtimeclient.InNamespace(framework.MachineAPINamespace))
		if err != nil {
			continue
		}

		if len(machineList.Items) == 1 {
			return zone, machineList.Items[0].Name
		}
	}

	return "", ""
}

// getMachineSuffixFromName extracts the suffix (index) from a machine name.
// Machine names are in format: <prefix>-<random-id>-<index> or <prefix>-<index>.
// We want to extract the index (including the dash) after the last dash.
// For example: "cluster-master-abc12-1" returns "-1", "cluster-master-0" returns "-0".
func getMachineSuffixFromName(machineName string) string {
	parts := []rune(machineName)
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '-' {
			return string(parts[i:])
		}
	}

	return ""
}

// swapFirstTwoFailureDomains is a generic helper that swaps the first two elements of a slice.
// This is used to change the order of failure domains without changing the zones themselves.
// If the slice has fewer than 2 elements, it returns the slice unchanged.
func swapFirstTwoFailureDomains[T any](slice []T) []T {
	if len(slice) < 2 {
		return slice
	}
	// Swap first and second elements
	slice[0], slice[1] = slice[1], slice[0]

	return slice
}

// removeFailureDomainByZone is a generic helper that removes a failure domain from a slice by zone name.
// The getZone function is used to extract the zone name from each failure domain element.
// Returns the modified slice and the removed failure domain as a JSON string.
// If no matching zone is found, returns the original slice and an empty string.
func removeFailureDomainByZone[T any](fds []T, zone string, getZone func(T) string) ([]T, string) {
	for i, fd := range fds {
		if getZone(fd) == zone {
			// Store the FD for restoration
			rawFD, _ := json.Marshal(fd)
			// Remove this FD and return immediately
			return append(fds[:i], fds[i+1:]...), string(rawFD)
		}
	}

	return fds, ""
}

// prependFailureDomain is a generic helper that prepends a failure domain to the beginning of a slice.
// If the slice pointer is nil, it creates a new slice with the single element.
func prependFailureDomain[T any](fds *[]T, fd T) []T {
	if fds != nil {
		return append([]T{fd}, *fds...)
	}

	return []T{fd}
}

// changeFailureDomainsOrderInCPMS changes the order of failure domains in a ControlPlaneMachineSet
// by swapping the first two elements. This is used to verify that changing only the order of
// failure domains (without changing the zones themselves) does not trigger a machine rollout.
func changeFailureDomainsOrderInCPMS(cpms *machinev1.ControlPlaneMachineSet, platform configv1.PlatformType) {
	Eventually(komega.Update(cpms, func() {
		if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil ||
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains == nil {
			return
		}

		switch platform {
		case configv1.AWSPlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS; fds != nil {
				swappedFDs := swapFirstTwoFailureDomains(*fds)
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS = &swappedFDs
			}
		case configv1.AzurePlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure; fds != nil {
				swappedFDs := swapFirstTwoFailureDomains(*fds)
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure = &swappedFDs
			}
		case configv1.GCPPlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP; fds != nil {
				swappedFDs := swapFirstTwoFailureDomains(*fds)
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP = &swappedFDs
			}
		}
	})).Should(Succeed(), "control plane machine set should be able to be updated")
}

// RemoveFailureDomainFromCPMS removes a failure domain with the specified zone from the ControlPlaneMachineSet.
// It returns the removed failure domain as a JSON string, which can be used to restore it later.
func RemoveFailureDomainFromCPMS(platform configv1.PlatformType, zone string) string {
	var removedFD string

	cpms := &machinev1.ControlPlaneMachineSet{}
	cpms.Name = framework.ControlPlaneMachineSetName
	cpms.Namespace = framework.MachineAPINamespace

	Eventually(komega.Get(cpms)).Should(Succeed(), "control plane machine set should exist")

	Eventually(komega.Update(cpms, func() {
		if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil ||
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains == nil {
			return
		}

		switch platform {
		case configv1.AWSPlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS; fds != nil {
				newFDs, removedJSON := removeFailureDomainByZone(*fds, zone, func(fd machinev1.AWSFailureDomain) string {
					return fd.Placement.AvailabilityZone
				})
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS = &newFDs
				removedFD = removedJSON
			}
		case configv1.AzurePlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure; fds != nil {
				newFDs, removedJSON := removeFailureDomainByZone(*fds, zone, func(fd machinev1.AzureFailureDomain) string {
					return fd.Zone
				})
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure = &newFDs
				removedFD = removedJSON
			}
		case configv1.GCPPlatformType:
			if fds := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP; fds != nil {
				newFDs, removedJSON := removeFailureDomainByZone(*fds, zone, func(fd machinev1.GCPFailureDomain) string {
					return fd.Zone
				})
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP = &newFDs
				removedFD = removedJSON
			}
		}
	})).Should(Succeed(), "control plane machine set should be able to be updated")

	return removedFD
}

// RestoreFailureDomainToCPMS restores a previously removed failure domain to the ControlPlaneMachineSet.
// The failure domain is prepended to the beginning of the list. The fdJSON parameter should be the
// JSON string returned by RemoveFailureDomainFromCPMS.
func RestoreFailureDomainToCPMS(platform configv1.PlatformType, fdJSON string) {
	if fdJSON == "" {
		return
	}

	cpms := &machinev1.ControlPlaneMachineSet{}
	cpms.Name = framework.ControlPlaneMachineSetName
	cpms.Namespace = framework.MachineAPINamespace

	Eventually(komega.Get(cpms)).Should(Succeed(), "control plane machine set should exist")

	Eventually(komega.Update(cpms, func() {
		if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil ||
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains == nil {
			return
		}

		switch platform {
		case configv1.AWSPlatformType:
			var fd machinev1.AWSFailureDomain
			err := json.Unmarshal([]byte(fdJSON), &fd)
			Expect(err).NotTo(HaveOccurred())

			newFDs := prependFailureDomain(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS, fd)
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.AWS = &newFDs

		case configv1.AzurePlatformType:
			var fd machinev1.AzureFailureDomain
			err := json.Unmarshal([]byte(fdJSON), &fd)
			Expect(err).NotTo(HaveOccurred())
			newFDs := prependFailureDomain(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure, fd)
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Azure = &newFDs

		case configv1.GCPPlatformType:
			var fd machinev1.GCPFailureDomain
			err := json.Unmarshal([]byte(fdJSON), &fd)
			Expect(err).NotTo(HaveOccurred())
			newFDs := prependFailureDomain(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP, fd)
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.GCP = &newFDs
		}
	})).Should(Succeed(), "control plane machine set should be able to be updated")
}

// CheckFailureDomainTestPrerequisites performs the initial setup for failureDomain removal tests.
// It validates the platform, gets the CPMS, retrieves failureDomains, finds a zone with a single machine,
// and returns all necessary information for the test.
// Returns an error if the platform doesn't support failureDomains, if there are insufficient
// failureDomains, or if no suitable zone is found. The caller should check the error and Skip if needed.
func CheckFailureDomainTestPrerequisites(testFramework framework.Framework) (platform configv1.PlatformType, zoneToRemove string, machineInZone string, err error) {
	ctx := testFramework.GetContext()
	k8sClient := testFramework.GetClient()

	// Check for platforms without failureDomain support
	platform = testFramework.GetPlatformType()
	if platform != configv1.AWSPlatformType &&
		platform != configv1.AzurePlatformType &&
		platform != configv1.GCPPlatformType {
		return platform, "", "", ErrPlatformNotSupported
	}

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	// Get failureDomains count
	failureDomains := getFailureDomainsFromCPMS(cpms, platform)
	if len(failureDomains) <= 1 {
		return platform, "", "", fmt.Errorf("%w, found %d", ErrInsufficientFailureDomains, len(failureDomains))
	}

	zoneToRemove, machineInZone = findZoneWithSingleMachine(ctx, k8sClient, failureDomains)
	if zoneToRemove == "" {
		return platform, "", "", ErrNoZoneWithSingleMachine
	}

	return platform, zoneToRemove, machineInZone, nil
}

// WaitForMachineReplacedInDifferentZone verifies that exactly one machine with the given suffix exists,
// is in a different zone than zoneToRemove, and is Running.
// It checks that the total machine count is 3 and the old machine has been deleted.
func WaitForMachineReplacedInDifferentZone(ctx context.Context, k8sClient runtimeclient.Client, machineSuffix, zoneToRemove string, timeout time.Duration) {
	By(fmt.Sprintf("Waiting for machine with suffix %s to be replaced in a different zone than %s", machineSuffix, zoneToRemove))

	var replacementMachineName string

	var replacementZone string

	Eventually(func(g Gomega) {
		machineList := &machinev1beta1.MachineList{}
		machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
		err := k8sClient.List(ctx, machineList, machineSelector, runtimeclient.InNamespace(framework.MachineAPINamespace))
		g.Expect(err).NotTo(HaveOccurred())

		// Should have exactly 3 control plane machines total
		g.Expect(machineList.Items).To(HaveLen(3), "Should have exactly 3 control plane machines")

		// Find all machines with matching suffix
		var machinesWithSuffix []machinev1beta1.Machine
		for _, machine := range machineList.Items {
			if suffix := getMachineSuffixFromName(machine.Name); suffix == machineSuffix {
				machinesWithSuffix = append(machinesWithSuffix, machine)
			}
		}

		// We should have exactly 1 machine with the target suffix (old machine should be deleted)
		g.Expect(machinesWithSuffix).To(HaveLen(1),
			"Should have exactly 1 machine with suffix %s (old machine in zone %s should be deleted)", machineSuffix, zoneToRemove)

		// That single machine should be in a different zone and running
		machine := machinesWithSuffix[0]
		zone, ok := machine.Labels["machine.openshift.io/zone"]
		g.Expect(ok).To(BeTrue(), "Machine %s should have zone label", machine.Name)
		g.Expect(zone).NotTo(Equal(zoneToRemove),
			"Machine %s should be in a different zone than %s, but found in %s", machine.Name, zoneToRemove, zone)
		g.Expect(machine.Status.Phase).NotTo(BeNil(), "Machine %s should have a phase", machine.Name)
		g.Expect(*machine.Status.Phase).To(Equal("Running"),
			"Machine %s should be in Running phase, but is in %s", machine.Name, *machine.Status.Phase)

		// Store for logging
		replacementMachineName = machine.Name
		replacementZone = zone
	}).WithTimeout(timeout).WithPolling(30 * time.Second).Should(Succeed())

	By(fmt.Sprintf("Replacement completed: %s is Running in zone %s (old machine in zone %s deleted)", replacementMachineName, replacementZone, zoneToRemove))
}

// recordInitialMachineNames gets the current list of machines and returns their names as a map.
func recordInitialMachineNames(ctx context.Context, k8sClient runtimeclient.Client) map[string]bool {
	initialMachineList := &machinev1beta1.MachineList{}
	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	err := k8sClient.List(ctx, initialMachineList, machineSelector, runtimeclient.InNamespace(framework.MachineAPINamespace))
	Expect(err).NotTo(HaveOccurred())

	initialMachineNames := make(map[string]bool)
	for _, machine := range initialMachineList.Items {
		initialMachineNames[machine.Name] = true
	}

	return initialMachineNames
}

// verifyMachinesNotReplaced checks that all current machines are from the initial set.
func verifyMachinesNotReplaced(ctx context.Context, k8sClient runtimeclient.Client, initialMachineNames map[string]bool) {
	Consistently(func(g Gomega) {
		machineList := &machinev1beta1.MachineList{}
		machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())
		err := k8sClient.List(ctx, machineList, machineSelector, runtimeclient.InNamespace(framework.MachineAPINamespace))
		g.Expect(err).NotTo(HaveOccurred())

		// All machine names should still be in the initial set
		for _, machine := range machineList.Items {
			g.Expect(initialMachineNames).To(HaveKey(machine.Name),
				"Machine %s should be from the original set", machine.Name)
		}
	}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
}
