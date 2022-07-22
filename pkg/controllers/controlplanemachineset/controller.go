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
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// clusterControlPlaneMachineSetName is the name of the ControlPlaneMachineSet.
	// As ControlPlaneMachineSets are singletons within the namespace, only ControlPlaneMachineSets
	// with this name should be reconciled.
	clusterControlPlaneMachineSetName = "cluster"

	// controlPlaneMachineSetFinalizer is the finalizer used by the ControlPlaneMachineSet operator
	// to prevent deletion until the operator has cleaned up owner references on the Control Plane Machines.
	controlPlaneMachineSetFinalizer = "controlplanemachineset.machine.openshift.io"

	// degradedClusterState is used to denote that the control plane machine set has detected a degraded cluster.
	// In this case, the controller will not perform any further actions.
	degradedClusterState = "Cluster state is degraded. The control plane machine set will not take any action until issues have been resolved."
)

// ControlPlaneMachineSetReconciler reconciles a ControlPlaneMachineSet object.
type ControlPlaneMachineSetReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RESTMapper meta.RESTMapper

	// Namespace is the namespace in which the ControlPlaneMachineSet controller should operate.
	// Any ControlPlaneMachineSet not in this namespace should be ignored.
	Namespace string

	// OperatorName is the name of the ClusterOperator with which the controller should report
	// its status.
	OperatorName string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ControlPlaneMachineSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// All predicates are executed before the event handler is called
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&machinev1.ControlPlaneMachineSet{}, builder.WithPredicates(filterControlPlaneMachineSet(r.Namespace))).
		Owns(&machinev1beta1.Machine{}, builder.WithPredicates(filterControlPlaneMachines(r.Namespace))).
		Watches(
			&source.Kind{Type: &configv1.ClusterOperator{}},
			handler.EnqueueRequestsFromMapFunc(clusterOperatorToControlPlaneMachineSet(r.Namespace)),
			builder.WithPredicates(filterClusterOperator(r.OperatorName)),
		).
		// Override the default log constructor as it makes the logs very chatty.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			return mgr.GetLogger().WithValues(
				"controller", "controlplanemachineset",
			)
		}).
		Complete(r); err != nil {
		return fmt.Errorf("could not set up controller for control plane machine set: %w", err)
	}

	// Set up API helpers from the manager.
	r.Scheme = mgr.GetScheme()
	r.RESTMapper = mgr.GetRESTMapper()

	return nil
}

// Reconcile reconciles the ControlPlaneMachineSet object.
func (r *ControlPlaneMachineSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "namespace", req.Namespace, "name", req.Name)

	logger.V(1).Info("Reconciling control plane machine set")
	defer logger.V(1).Info("Finished reconciling control plane machine set")

	cpms := &machinev1.ControlPlaneMachineSet{}
	cpmsKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}

	// Fetch the ControlPlaneMachineSet and set the cluster operator to available if it doesn't exist.
	if err := r.Get(ctx, cpmsKey, cpms); apierrors.IsNotFound(err) {
		logger.V(1).Info("No control plane machine set found, setting operator status available")

		if err := r.setClusterOperatorAvailable(ctx, logger); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to reconcile cluster operator status: %w", err)
		}

		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch control plane machine set: %w", err)
	}

	// Take a copy of the original object to be able to create a patch for the status at the end.
	patchBase := client.MergeFrom(cpms.DeepCopy())

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, logger, cpms)
	if err != nil {
		// Don't return an error here so that we have an opportunity to update the status and cluster operator status.
		errs = append(errs, fmt.Errorf("error reconciling control plane machine set: %w", err))
	}

	if err := r.updateControlPlaneMachineSetStatus(ctx, logger, cpms, patchBase); err != nil {
		// Don't return an error here so that we have an opportunity to update the cluster operator status.
		errs = append(errs, fmt.Errorf("error updating control plane machine set status: %w", err))
	}

	if err := r.updateClusterOperatorStatus(ctx, logger, cpms); err != nil {
		// Don't return an error here so we can aggregate the errors with previous updates.
		errs = append(errs, fmt.Errorf("error updating control plane machine set status: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

// reconcile performs the main business logic of the ControlPlaneMachineSet operator.
// Notably it actions the various parts of the business logic without performing any status updates on the
// ControlPlaneMachineSet object itself, these updates are handled at the parent scope.
func (r *ControlPlaneMachineSetReconciler) reconcile(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet) (ctrl.Result, error) {
	// If the control plane machine set is being deleted, we need to handle that rather than the regular reconcile flow.
	if cpms.GetDeletionTimestamp() != nil {
		return r.reconcileDelete(ctx, logger, cpms)
	}

	// Add the finalizer before any updates to the status. This will ensure no status changes on the same reconcile
	// as we add the finalizer. The finalizer must be present on the object before we take any actions.
	if updatedFinalizer, err := r.ensureFinalizer(ctx, logger, cpms); err != nil {
		return ctrl.Result{}, fmt.Errorf("error adding finalizer: %w", err)
	} else if updatedFinalizer {
		return ctrl.Result{Requeue: true}, nil
	}

	machineProvider, err := providers.NewMachineProvider(ctx, logger, r.Client, cpms)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error constructing machine provider: %w", err)
	}

	machineInfos, err := machineProvider.GetMachineInfos(ctx, logger)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error fetching machine info: %w", err)
	}

	indexedMachineInfos, err := machineInfosByIndex(cpms, machineInfos)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not sort machine info by index: %w", err)
	}

	result, err := r.reconcileMachines(ctx, logger, cpms, machineProvider, indexedMachineInfos)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling machines: %w", err)
	}

	return result, nil
}

// reconcileMachines uses the gathered machine info to set the status of the ControlPlaneMachineSet and then,
// after validating that the cluster state is as expected, uses the machine provider to take appropriate actions
// to perform any requied roll outs.
func (r *ControlPlaneMachineSetReconciler) reconcileMachines(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineProvider machineproviders.MachineProvider, machineInfos map[int32][]machineproviders.MachineInfo) (ctrl.Result, error) {
	if err := reconcileStatusWithMachineInfo(logger, cpms, machineInfos); err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling machine info with status: %w", err)
	}

	if err := r.ensureOwnerReferences(ctx, logger, cpms, machineInfos); err != nil {
		return ctrl.Result{}, fmt.Errorf("error ensuring owner references: %w", err)
	}

	if err := r.validateClusterState(ctx, logger, cpms, machineInfos); err != nil {
		return ctrl.Result{}, fmt.Errorf("error validating cluster state: %w", err)
	}

	if isControlPlaneMachineSetDegraded(cpms) {
		logger.V(1).Info(degradedClusterState)
		return ctrl.Result{}, nil
	}

	result, err := r.reconcileMachineUpdates(ctx, logger, cpms, machineProvider, machineInfos)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling machine updates: %w", err)
	}

	return result, nil
}

// reconcileDelete handles the removal logic for the ControlPlaneMachineSet resource.
// During the deletion process, the controller is expected to remove any owner references from Machines
// that are owned by the ControlPlaneMachineSet.
// Once the owner references are removed, it removes the finalizer to allow the garbage collector to reap
// the deleted ControlPlaneMachineSet.
func (r *ControlPlaneMachineSetReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet) (ctrl.Result, error) {
	logger.V(1).Info("Reconciling control plane machine set deletion")

	machineTypeMeta, err := providers.GetMachineTypeMeta(cpms.Spec.Template.MachineType)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not determine machines type: %w", err)
	}

	logger.V(4).Info("Detected machines type", "kind", machineTypeMeta.Kind, "apiVersion", machineTypeMeta.APIVersion)

	selector, err := metav1.LabelSelectorAsSelector(&cpms.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not convert label selector to selector: %w", err)
	}

	machinesMeta := &metav1.PartialObjectMetadataList{
		TypeMeta: machineTypeMeta,
	}
	if err := r.List(ctx, machinesMeta, &client.ListOptions{LabelSelector: selector}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list machines: %w", err)
	}

	var errs []error

	for i := range machinesMeta.Items {
		machineObjectMeta := machinesMeta.Items[i]
		logger.V(4).Info("Removing owner reference", "machine", machineObjectMeta.Name)
		patchBase := client.MergeFrom(machineObjectMeta.DeepCopy())

		if !removeOwnerRef(&machineObjectMeta, cpms) {
			continue
		}

		if err := r.Patch(ctx, &machineObjectMeta, patchBase); err != nil {
			errs = append(errs, fmt.Errorf("error removing owner reference from machine %s: %w", machineObjectMeta.Name, err))
		}
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	cpmsCopy := cpms.DeepCopy()

	finalizerUpdated := controllerutil.RemoveFinalizer(cpmsCopy, controlPlaneMachineSetFinalizer)
	if finalizerUpdated {
		logger.V(4).Info("Removing finalizer from the control plane machine set")

		if err := r.Update(ctx, cpmsCopy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update control plane machine set: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// hasOwnerRef returns true if target has an ownerRef to owner.
func hasOwnerRef(target, owner client.Object) bool {
	ownerUID, ownerName := owner.GetUID(), owner.GetName()
	for _, ownerRef := range target.GetOwnerReferences() {
		if ownerRef.UID == ownerUID &&
			ownerRef.Name == ownerName {
			return true
		}
	}

	return false
}

// removeOwnerRef removes owner from the ownerRefs of target, if necessary. Returns true if target needs
// to be updated and false otherwise.
func removeOwnerRef(target, owner client.Object) bool {
	if !hasOwnerRef(target, owner) {
		return false
	}

	ownerUID := owner.GetUID()
	newRefs := []metav1.OwnerReference{}

	for _, ref := range target.GetOwnerReferences() {
		if ref.UID == ownerUID {
			continue
		}

		newRefs = append(newRefs, ref)
	}

	target.SetOwnerReferences(newRefs)

	return true
}

// ensureFinalizer adds a finalizer to the ControlPlaneMachineSet if required.
// If the finalizer already exists, this function should be a no-op.
// If the finalizer is added, the function will return true so that the reconciler can requeue the object.
// Adding the finalizer in a separate reconcile ensures that spec updates are separate from status updates.
func (r *ControlPlaneMachineSetReconciler) ensureFinalizer(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet) (bool, error) {
	// Check if we need to add the finalizer.
	for _, finalizer := range cpms.GetFinalizers() {
		if finalizer == controlPlaneMachineSetFinalizer {
			logger.V(4).Info("Finalizer already present on control plane machine set")
			return false, nil
		}
	}

	cpms.SetFinalizers(append(cpms.GetFinalizers(), controlPlaneMachineSetFinalizer))

	if err := r.Client.Update(ctx, cpms); err != nil {
		return false, fmt.Errorf("error updating control plane machine set: %w", err)
	}

	logger.V(2).Info("Added finalizer to control plane machine set")

	return true, nil
}

// ensureOwnerReferences determines if any of the Machines within the machineInfos require a new controller owner
// reference to be added, and then uses PartialObjectMetadata to ensure that the owner reference is added.
func (r *ControlPlaneMachineSetReconciler) ensureOwnerReferences(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineInfos map[int32][]machineproviders.MachineInfo) error {
	for _, machineInfo := range machineInfos {
		for _, mInfo := range machineInfo {
			if mInfo.MachineRef == nil {
				continue
			}

			mObjectMeta := mInfo.MachineRef.ObjectMeta
			mLogger := logger.WithValues("machineNamespace", mObjectMeta.GetNamespace(), "machineName", mObjectMeta.GetName())

			machineGVK, err := r.RESTMapper.KindFor(mInfo.MachineRef.GroupVersionResource)
			if err != nil {
				return fmt.Errorf("error getting GVK for machine: %w", err)
			}

			machine := &metav1.PartialObjectMetadata{}
			machine.SetGroupVersionKind(machineGVK)
			machine.ObjectMeta = mObjectMeta

			if isOwnedByCurrentCPMS(cpms, machine) {
				mLogger.V(4).Info("Owner reference already present on machine")

				continue
			}

			patchBase := client.MergeFrom(machine.DeepCopy())

			if err := controllerutil.SetControllerReference(cpms, machine, r.Scheme); err != nil {
				mLogger.Error(err, "Cannot add owner reference to machine")

				var alreadyOwnedErr *controllerutil.AlreadyOwnedError
				if errors.As(err, &alreadyOwnedErr) {
					meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
						Type:               conditionDegraded,
						Status:             metav1.ConditionTrue,
						Reason:             reasonMachinesAlreadyOwned,
						ObservedGeneration: cpms.Generation,
						Message:            "Observed already owned machine(s) in target machines",
					})

					// don't return an error here and continue iterating through the machines
					continue
				}

				return fmt.Errorf("error setting owner reference: %w", err)
			}

			if err := r.Client.Patch(ctx, machine, patchBase); err != nil {
				return fmt.Errorf("error patching machine: %w", err)
			}

			mLogger.V(2).Info("Added owner reference to machine")
		}
	}

	return nil
}

// validateClusterState uses the machineInfos to validate that:
// - All Nodes in the cluster claiming to be control plane nodes have a valid machine
// - At least 1 of the control plane machines is in the ready state (if there are no ready Machines then the cluster
//   is likely misconfigured)
// - We have the correct number of indexes:
//   + Right number of indexes, valid
//   + Too few indexes, valid. We will later scale up without user intervention when we perform reconcileMachineUpdates
//   + Too many indexes, invalid. We set the operator to degraded and ask the user for manual intervention
func (r *ControlPlaneMachineSetReconciler) validateClusterState(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineInfos map[int32][]machineproviders.MachineInfo) error {
	return nil
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

// isControlPlaneMachineSetDegraded determines whether or not the ControlPlaneMachineSet
// has a true, degraded condition.
func isControlPlaneMachineSetDegraded(cpms *machinev1.ControlPlaneMachineSet) bool {
	return false
}

// isOwnedByCurrentCPMS determines if the given machine is owned by the ControlPlaneMachineSet.
func isOwnedByCurrentCPMS(cpms *machinev1.ControlPlaneMachineSet, machineObjectMeta *metav1.PartialObjectMetadata) bool {
	if machineObjectMeta.OwnerReferences == nil {
		return false
	}

	if controlledBy := metav1.GetControllerOf(machineObjectMeta); controlledBy != nil {
		currentCpmsGV, err := schema.ParseGroupVersion(cpms.APIVersion)
		if err != nil {
			return false
		}

		machineOwnerGV, err := schema.ParseGroupVersion(controlledBy.APIVersion)
		if err != nil {
			return false
		}

		return currentCpmsGV.Group == machineOwnerGV.Group && cpms.Kind == controlledBy.Kind && cpms.Name == controlledBy.Name
	}

	return false
}
