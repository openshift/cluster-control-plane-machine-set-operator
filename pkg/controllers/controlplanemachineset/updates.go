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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// createdReplacement is a log message used to inform the user that a new Machine was created to
	// replace an existing Machine.
	createdReplacement = "Created replacement machine"

	// errorCreatingMachine is a log message used to inform the user that an error occurred while
	// attempting to create a replacement Machine.
	errorCreatingMachine = "Error creating machine"

	// errorDeletingMachine is a log message used to inform the user that an error occurred while
	// attempting to delete replacement Machine.
	errorDeletingMachine = "Error deleting machine"

	// invalidStrategyMessage is used to inform the user that they have provided an invalid value
	// for the update strategy.
	invalidStrategyMessage = "invalid value for spec.strategy.type"

	// machineRequiresUpdate is a log message used to inform the user that a Machine requires an update,
	// but that they must first delete the Machine to trigger a replacement.
	// This is used with the OnDelete replacement strategy.
	machineRequiresUpdate = "Machine requires an update, delete the machine to trigger a replacement"

	// noUpdatesRequired is a log message used to inform the user that no updates are required within
	// the current set of Machines.
	noUpdatesRequired = "No updates required"

	// removingOldMachine is a log message used to inform the user that an old Machine has been
	// deleted as a part of the rollout operation.
	removingOldMachine = "Removing old machine"

	// waitingForReady is a log message used to inform the user that no operations are taking
	// place because the rollout is waiting for a Machine to be ready.
	// This is used exclusively when adding a new Machine to a missing index.
	waitingForReady = "Waiting for machine to become ready"

	// waitingForRemoved is a log message used to inform the user that no operations are taking
	// place because the rollout is waiting for a Machine to be removed.
	waitingForRemoved = "Waiting for machine to be removed"

	// waitingForReplacement is a log message used to inform the user that no operations are taking
	// place because the rollout is waiting for a replacement Machine to become ready.
	// This is used when replacing a Machine within an index.
	waitingForReplacement = "Waiting for replacement machine to become ready"
)

var (
	// errRecreateStrategyNotSupported is used to inform users that the Recreate update strategy is not yet supported.
	// It may be supported in a future version.
	errRecreateStrategyNotSupported = fmt.Errorf("update strategy %q is not supported", machinev1.Recreate)

	// errReplicasRequired is used to inform users that the replicas field is currently unset, and
	// must be set to continue operation.
	errReplicasRequired = errors.New("spec.replicas is unset: replicas is required")

	// errUnknownStrategy is used to inform users that the update strategy they have provided is not recognised.
	errUnknownStrategy = errors.New("unknown update strategy")
)

// reconcileMachineUpdates determines if any Machines are in need of an update and then handles those updates as per the
// update strategy within the ControlPlaneMachineSet.
// When a Machine needs an update, this function should create a replacement where appropriate.
func (r *ControlPlaneMachineSetReconciler) reconcileMachineUpdates(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineProvider machineproviders.MachineProvider, machineInfos []machineproviders.MachineInfo) (ctrl.Result, error) {
	indexedMachineInfos, err := machineInfosByIndex(cpms, machineInfos)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not sort machine info by index: %w", err)
	}

	switch cpms.Spec.Strategy.Type {
	case machinev1.RollingUpdate:
		return r.reconcileMachineRollingUpdate(ctx, logger, cpms, machineProvider, indexedMachineInfos)
	case machinev1.OnDelete:
		return r.reconcileMachineOnDeleteUpdate(ctx, logger, cpms, machineProvider, indexedMachineInfos)
	case machinev1.Recreate:
		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:    conditionDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  reasonInvalidStrategy,
			Message: fmt.Sprintf("%s: %s", invalidStrategyMessage, errRecreateStrategyNotSupported),
		})

		logger.Error(errRecreateStrategyNotSupported, invalidStrategyMessage)
	default:
		meta.SetStatusCondition(&cpms.Status.Conditions,
			metav1.Condition{
				Type:    conditionDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonInvalidStrategy,
				Message: fmt.Sprintf("%s: %s: %s", invalidStrategyMessage, errUnknownStrategy, cpms.Spec.Strategy.Type),
			})

		logger.Error(fmt.Errorf("%w: %s", errUnknownStrategy, cpms.Spec.Strategy.Type), invalidStrategyMessage)
	}

	// Do not return an error here as we only return here when the strategy is invalid.
	// This will need user intervention to resolve.
	return ctrl.Result{}, nil
}

// reconcileMachineRollingUpdate implements the rolling update strategy for the ControlPlaneMachineSet. It uses the
// indexed machine information to determine when a new Machine is required to be created. When a new Machine is required,
// it uses the machine provider to create the new Machine.
//
// For rolling updates, a new Machine is required when a machine index has a Machine, which needs an update, but does
// not yet have replacement created. It must also observe the surge semantics of a rolling update, so, if an existing
// index is already going through the process of a rolling update, it should not start the update of any other index.
// At present, the surge is limited to a single Machine instance.
//
// Once a replacement Machine is ready, the strategy should also delete the old Machine to allow it to be removed from
// the cluster.
//
// In certain scenarios, there may be indexes with missing Machines. In these circumstances, the update should attempt
// to create a new Machine to fulfil the requirement of that index.
func (r *ControlPlaneMachineSetReconciler) reconcileMachineRollingUpdate(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineProvider machineproviders.MachineProvider, indexedMachineInfos map[int32][]machineproviders.MachineInfo) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// reconcileMachineOnDeleteUpdate implements the rolling update strategy for the ControlPlaneMachineSet. It uses the
// indexed machine information to determine when a new Machine is required to be created. When a new Machine is required,
// it uses the machine provider to create the new Machine.
//
// For on-delete updates, a new Machine is required when a machine index has a Machine with a non-zero deletion
// timestamp but does not yet have a replacement created.
//
// In certain scenarios, there may be indexes with missing Machines. In these circumstances, the update should attempt
// to create a new Machine to fulfil the requirement of that index.
func (r *ControlPlaneMachineSetReconciler) reconcileMachineOnDeleteUpdate(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineProvider machineproviders.MachineProvider, indexedMachineInfos map[int32][]machineproviders.MachineInfo) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// machineInfosByIndex groups MachineInfo entries by index inside a map of index to MachineInfo.
// This allows the update strategies to process each index in turn.
// It is expected to add an entry for each expected index (0-(replicas-1)) so that later logic of updates can process
// indexes that do not have any associated Machines.
func machineInfosByIndex(cpms *machinev1.ControlPlaneMachineSet, machineInfos []machineproviders.MachineInfo) (map[int32][]machineproviders.MachineInfo, error) {
	out := make(map[int32][]machineproviders.MachineInfo)

	if cpms.Spec.Replicas == nil {
		return nil, errReplicasRequired
	}

	// Make sure that every expected index is accounted for.
	for i := int32(0); i < *cpms.Spec.Replicas; i++ {
		out[i] = []machineproviders.MachineInfo{}
	}

	for _, machineInfo := range machineInfos {
		out[machineInfo.Index] = append(out[machineInfo.Index], machineInfo)
	}

	return out, nil
}
