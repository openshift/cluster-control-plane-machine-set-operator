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
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	ctrl "sigs.k8s.io/controller-runtime"
)

// reconcileMachineUpdates determines if any Machines are in need of an update and then handles those updates as per the
// update strategy within the ControlPlaneMachineSet.
// When a Machine needs an update, this function should create a replacement where appropriate.
func (r *ControlPlaneMachineSetReconciler) reconcileMachineUpdates(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineProvider machineproviders.MachineProvider, machineInfos []machineproviders.MachineInfo) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
