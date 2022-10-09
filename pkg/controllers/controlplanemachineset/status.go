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

package controlplanemachineset

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// updatingStatus is a log message used to inform users that the ControlPlaneMachineSet status is being updated.
	updatingStatus = "Updating control plane machine set status"

	// notUpdatingStatus is a log message used to inform users that the ControlPlaneMachineSet status is not being updated.
	notUpdatingStatus = "No update to control plane machine set status required"
)

// updateControlPlaneMachineSetStatus ensures that the status of the ControlPlaneMachineSet is up to date after
// the resource has been reconciled.
func (r *ControlPlaneMachineSetReconciler) updateControlPlaneMachineSetStatus(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, patchBase client.Patch) error {
	data, err := patchBase.Data(cpms)
	if err != nil {
		return fmt.Errorf("cannot calculate patch data from control plane machine set object: %w", err)
	}

	// Apply changes only if the patch is not empty
	if string(data) == "{}" {
		logger.V(3).Info(notUpdatingStatus)

		return nil
	}

	if err := r.Status().Update(ctx, cpms); err != nil {
		return fmt.Errorf("failed to sync status for control plane machine set object: %w", err)
	}

	logger.V(3).Info(updatingStatus, "data", string(data))

	return nil
}

// reconcileStatusWithMachineInfo takes the information gathered in the machineInfos and reconciles the status of the
// ControlPlaneMachineSet to match the data gathered.
// In particular, it will update the ObservedGeneration, Replicas, ReadyReplicas, UnavailableReplicas and UpdatedReplicas
// fields based on the information gathered, and then set any relevant conditions if applicable.
// It observes the following rules for setting the status:
//   - Replicas is the number of Machines present
//   - ReadyReplicas is the number of the above Replicas which are reporting as Ready
//   - UpdatedReplicas is the number of Ready Replicas that do not need an update (this should be at most 1 per index).
//   - UnavailableReplicas is the number of Machines required to satisfy the requirement of at least 1 Ready Replica per
//     index. Eg. if one index has no ready replicas, this is 1, if an index has 2 ready replicas, this does not count as
//     2 available replicas.
func reconcileStatusWithMachineInfo(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineInfosByIndex map[int32][]machineproviders.MachineInfo) error {
	replicas := int32(0)
	readyReplicas := int32(0)
	updatedReplicas := int32(0)
	unavailableReplicas := int32(0)

	for _, machineInfosInIndex := range machineInfosByIndex {
		hasUnavailableReplicaInIndex := false
		hasAvailableReplicaInIndex := false

		for _, machineInfo := range machineInfosInIndex {
			replicas += 1

			if machineInfo.Ready {
				readyReplicas += 1
				hasAvailableReplicaInIndex = true

				if !machineInfo.NeedsUpdate {
					updatedReplicas += 1
				}
			} else {
				hasUnavailableReplicaInIndex = true
			}
		}

		if len(machineInfosInIndex) == 0 || hasUnavailableReplicaInIndex && !hasAvailableReplicaInIndex {
			// Count this index as unavailable if it has no machines or if it has machines but all of them are unavailable.
			unavailableReplicas += 1
		}
	}

	cpms.Status.ObservedGeneration = cpms.Generation
	cpms.Status.Replicas = replicas
	cpms.Status.ReadyReplicas = readyReplicas
	cpms.Status.UnavailableReplicas = unavailableReplicas
	cpms.Status.UpdatedReplicas = updatedReplicas

	logger.Info("Observed Machine Configuration",
		"observedGeneration", cpms.Status.ObservedGeneration,
		"replicas", cpms.Status.Replicas,
		"readyReplicas", cpms.Status.ReadyReplicas,
		"updatedReplicas", cpms.Status.UpdatedReplicas,
		"unavailableReplicas", cpms.Status.UnavailableReplicas,
	)

	if err := setConditions(cpms); err != nil {
		return fmt.Errorf("could not set control plane machine set conditions: %w", err)
	}

	return nil
}

// setConditions sets Available, Degraded and Progressing conditions on the ControlPlaneMachineSet.
func setConditions(cpms *machinev1.ControlPlaneMachineSet) error {
	availableCondition := getAvailableCondition(cpms)
	meta.SetStatusCondition(&cpms.Status.Conditions, availableCondition)

	degradedCondition := getDegradedCondition(cpms)
	meta.SetStatusCondition(&cpms.Status.Conditions, degradedCondition)

	progressingCondition, err := getProgressingCondition(cpms)
	if err != nil {
		return fmt.Errorf("could not set progressing condition: %w", err)
	}

	meta.SetStatusCondition(&cpms.Status.Conditions, progressingCondition)

	return nil
}

// getProgressingCondition computes Available condition based on the current ControlPlaneMachineSet status.
func getAvailableCondition(cpms *machinev1.ControlPlaneMachineSet) metav1.Condition {
	if cpms.Status.UnavailableReplicas != 0 {
		return metav1.Condition{
			Type:               conditionAvailable,
			Status:             metav1.ConditionFalse,
			Reason:             reasonUnavailableReplicas,
			Message:            fmt.Sprintf("Missing %d available replica(s)", cpms.Status.UnavailableReplicas),
			ObservedGeneration: cpms.Generation,
		}
	}

	return metav1.Condition{
		Type:               conditionAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             reasonAllReplicasAvailable,
		ObservedGeneration: cpms.Generation,
	}
}

// getProgressingCondition computes Degraded condition based on the current ControlPlaneMachineSet status.
func getDegradedCondition(cpms *machinev1.ControlPlaneMachineSet) metav1.Condition {
	if cpms.Status.ReadyReplicas == 0 {
		return metav1.Condition{
			Type:               conditionDegraded,
			Status:             metav1.ConditionTrue,
			Reason:             reasonNoReadyMachines,
			ObservedGeneration: cpms.Generation,
		}
	}

	return metav1.Condition{
		Type:               conditionDegraded,
		Status:             metav1.ConditionFalse,
		Reason:             reasonAsExpected,
		ObservedGeneration: cpms.Generation,
	}
}

// getProgressingCondition computes Progressing condition based on the current ControlPlaneMachineSet status.
func getProgressingCondition(cpms *machinev1.ControlPlaneMachineSet) (metav1.Condition, error) {
	if cpms.Spec.Replicas == nil {
		return metav1.Condition{}, errReplicasRequired
	}

	desiredReplicas := *cpms.Spec.Replicas

	if desiredReplicas > cpms.Status.UpdatedReplicas {
		return metav1.Condition{
			Type:               conditionProgressing,
			Status:             metav1.ConditionTrue,
			Reason:             reasonNeedsUpdateReplicas,
			Message:            fmt.Sprintf("Observed %d replica(s) in need of update", desiredReplicas-cpms.Status.UpdatedReplicas),
			ObservedGeneration: cpms.Generation,
		}, nil
	}

	if desiredReplicas < cpms.Status.ReadyReplicas {
		return metav1.Condition{
			Type:               conditionProgressing,
			Status:             metav1.ConditionTrue,
			Reason:             reasonExcessReplicas,
			Message:            fmt.Sprintf("Waiting for %d old replica(s) to be removed", cpms.Status.ReadyReplicas-cpms.Status.UpdatedReplicas),
			ObservedGeneration: cpms.Generation,
		}, nil
	}

	return metav1.Condition{
		Type:               conditionProgressing,
		Status:             metav1.ConditionFalse,
		Reason:             reasonAllReplicasUpdated,
		ObservedGeneration: cpms.Generation,
	}, nil
}
