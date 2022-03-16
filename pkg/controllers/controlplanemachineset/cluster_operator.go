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

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
)

// setClusterOperatorAvailable sets the control-plane-machine-set cluster operator status to available.
// This is used primarily when a ControlPlaneMachineSet doesn't exist.
func (r *ControlPlaneMachineSetReconciler) setClusterOperatorAvailable(ctx context.Context, logger logr.Logger) error {
	return nil
}

// setClusterOperatorStatus sets the control-plane-machine-set clsuter operator status based on the status of the
// ControlPlaneMachine passed into it.
// Notably, when the conditions of the ControlPlaneMachineSet indicate that the operations of the ControlPlaneMachineSet
// are not functioning as expected, the cluster operator should be marked degraded.
func (r *ControlPlaneMachineSetReconciler) updateClusterOperatorStatus(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet) error {
	return nil
}
