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
	"reflect"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// setClusterOperatorAvailable sets the control-plane-machine-set cluster operator status to available.
// This is used primarily when a ControlPlaneMachineSet doesn't exist.
func (r *ControlPlaneMachineSetReconciler) setClusterOperatorAvailable(ctx context.Context, logger logr.Logger) error {
	co, err := r.getClusterOperator(ctx, logger)
	if err != nil {
		return fmt.Errorf("cannot get cluster operator: %w", err)
	}

	conds := []configv1.ClusterOperatorStatusCondition{
		newClusterOperatorStatusCondition(configv1.OperatorAvailable, configv1.ConditionTrue, reasonAsExpected, "cluster operator is available"),
		newClusterOperatorStatusCondition(configv1.OperatorProgressing, configv1.ConditionFalse, reasonAsExpected, ""),
		newClusterOperatorStatusCondition(configv1.OperatorDegraded, configv1.ConditionFalse, reasonAsExpected, ""),
		newClusterOperatorStatusCondition(configv1.OperatorUpgradeable, configv1.ConditionTrue, reasonAsExpected, "cluster operator is upgradable"),
	}

	return r.patchClusterOperatorStatus(ctx, logger, co, conds)
}

// setClusterOperatorStatus sets the control-plane-machine-set cluster operator status based on the status of the
// ControlPlaneMachine passed into it.
// Notably, when the conditions of the ControlPlaneMachineSet indicate that the operations of the ControlPlaneMachineSet
// are not functioning as expected, the cluster operator should be marked degraded.
func (r *ControlPlaneMachineSetReconciler) updateClusterOperatorStatus(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet) error {
	co, err := r.getClusterOperator(ctx, logger)
	if err != nil {
		return fmt.Errorf("cannot get cluster operator: %w", err)
	}

	// Copying status conditions from control plane machine set to cluster operator
	conds := []configv1.ClusterOperatorStatusCondition{}

	for _, c := range cpms.Status.Conditions {
		switch c.Type {
		case conditionAvailable, conditionDegraded, conditionProgressing:
			conds = append(conds, newClusterOperatorStatusCondition(
				configv1.ClusterStatusConditionType(c.Type),
				configv1.ConditionStatus(c.Status),
				c.Reason,
				c.Message))
		}
	}

	// Define upgradable condition
	if v1helpers.IsStatusConditionPresentAndEqual(conds, configv1.OperatorAvailable, configv1.ConditionTrue) {
		conds = append(conds, newClusterOperatorStatusCondition(
			configv1.OperatorUpgradeable,
			configv1.ConditionTrue,
			reasonAsExpected,
			"cluster operator is upgradable"))
	} else {
		conds = append(conds, newClusterOperatorStatusCondition(
			configv1.OperatorUpgradeable,
			configv1.ConditionFalse,
			reasonAsExpected,
			"cluster operator is not upgradable"))
	}

	return r.patchClusterOperatorStatus(ctx, logger, co, conds)
}

// getClusterOperator returns an instance of Cluster Operator resource for control-plane-machine-set cluster operator.
func (r *ControlPlaneMachineSetReconciler) getClusterOperator(ctx context.Context, logger logr.Logger) (*configv1.ClusterOperator, error) {
	co := &configv1.ClusterOperator{}

	if err := r.Get(ctx, client.ObjectKey{Name: r.OperatorName}, co); err != nil {
		logger.Error(err, "failed to get cluster operator")

		return nil, fmt.Errorf("failed to get cluster operator %s: %w", r.OperatorName, err)
	}

	return co, nil
}

// newClusterOperatorStatusCondition is a helper function to generate a new condition for Cluster Operator.
func newClusterOperatorStatusCondition(conditionType configv1.ClusterStatusConditionType,
	conditionStatus configv1.ConditionStatus, reason, message string) configv1.ClusterOperatorStatusCondition {
	return configv1.ClusterOperatorStatusCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// patchClusterOperatorStatus updates cluster operator status with given conditions.
func (r *ControlPlaneMachineSetReconciler) patchClusterOperatorStatus(ctx context.Context, logger logr.Logger, co *configv1.ClusterOperator, conds []configv1.ClusterOperatorStatusCondition) error {
	// We need to perform update only if conditions or versions have been changed.
	needUpdate := false

	for _, c := range conds {
		if !isStatusConditionPresentAndEqual(co.Status.Conditions, c.Type, c.Status, c.Message, c.Reason) {
			needUpdate = true

			v1helpers.SetStatusCondition(&co.Status.Conditions, c)
		}
	}

	versions := []configv1.OperandVersion{{Name: "operator", Version: r.ReleaseVersion}}
	if !reflect.DeepEqual(co.Status.Versions, versions) {
		needUpdate = true

		co.Status.Versions = versions
	}

	if !needUpdate {
		return nil
	}

	if err := r.Status().Update(ctx, co); err != nil {
		logger.Error(err, "failed to sync status for cluster operator")
		return fmt.Errorf("failed to sync status for cluster operator %s, %w", r.OperatorName, err)
	}

	logger.V(4).Info(
		"Syncing cluster operator status",
		"available", getStatusForConditionType(co.Status.Conditions, configv1.OperatorAvailable),
		"progressing", getStatusForConditionType(co.Status.Conditions, configv1.OperatorProgressing),
		"degraded", getStatusForConditionType(co.Status.Conditions, configv1.OperatorDegraded),
		"upgradable", getStatusForConditionType(co.Status.Conditions, configv1.OperatorUpgradeable),
	)

	return nil
}

// isStatusConditionPresentAndEqual returns true when conditionType is present and has the same status, message and reason.
func isStatusConditionPresentAndEqual(conditions []configv1.ClusterOperatorStatusCondition, conditionType configv1.ClusterStatusConditionType, status configv1.ConditionStatus, message, reason string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status && condition.Message == message && condition.Reason == reason
		}
	}

	return false
}

// getStatusForConditionType returns as a string the status of the condition of the given type if present.
// If no condition of that type exists, it returns unknown for the status.
func getStatusForConditionType(conds []configv1.ClusterOperatorStatusCondition, conditionType configv1.ClusterStatusConditionType) string {
	if status := v1helpers.FindStatusCondition(conds, conditionType); status != nil {
		return string(status.Status)
	}

	return string(configv1.ConditionUnknown)
}
