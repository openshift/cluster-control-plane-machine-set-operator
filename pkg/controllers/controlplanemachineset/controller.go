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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ControlPlaneMachineSetReconciler reconciles a ControlPlaneMachineSet object.
type ControlPlaneMachineSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile reconciles the ControlPlaneMachineSet object.
func (r *ControlPlaneMachineSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ControlPlaneMachineSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r); err != nil {
		return fmt.Errorf("could not set up controller for ControlPlaneMachineSet: %w", err)
	}

	return nil
}
