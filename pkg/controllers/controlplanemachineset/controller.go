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
	"strings"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/features"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	// infrastructureName is the name of the Infrastructure resource. Any changes to this resource will need to be
	// reflected in changes to the ControlPlaneMachineSet configuration.
	infrastructureName = "cluster"
)

var (
	// errFoundUnmanagedControlPlaneNodes is used to inform users that one or more nodes do not have machines associated with them.
	errFoundUnmanagedControlPlaneNodes = errors.New("found unmanaged control plane nodes, the following node(s) do not have associated machines")

	// errNoReadyControlPlaneMachines is used to inform users that no control plane machines in the cluster are ready.
	errNoReadyControlPlaneMachines = errors.New("no ready control plane machines")

	// errFoundErroredReplacementControlPlaneMachine is used to inform users that one or more replacement control plane machines, have been found.
	errFoundErroredReplacementControlPlaneMachine = errors.New("found replacement control plane machines in an error state, the following machines(s) are currently reporting an error")

	// errFoundExcessiveIndexes is used to inform users that an excessive number of indexes has been found.
	errFoundExcessiveIndexes = errors.New("found an excessive number of indexes for the control plane machine set")

	// errFoundExcessiveUpdatedReplicas is used to inform users that an excessive number of updated machines has been found for a single index.
	errFoundExcessiveUpdatedReplicas = errors.New("found an excessive number of updated machines for a single index")
)

// ControlPlaneMachineSetReconciler reconciles a ControlPlaneMachineSet object.
type ControlPlaneMachineSetReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	RESTMapper     meta.RESTMapper
	UncachedClient client.Client

	// Namespace is the namespace in which the ControlPlaneMachineSet controller should operate.
	// Any ControlPlaneMachineSet not in this namespace should be ignored.
	Namespace string

	// OperatorName is the name of the ClusterOperator with which the controller should report
	// its status.
	OperatorName string

	// ReleaseVersion is the version of current cluster operator release.
	ReleaseVersion string

	// lastError allows us to track the last error that occurred during reconciliation.
	lastError *lastErrorTracker

	// FeatureGateAccessor enables checking of enabled featuregates
	FeatureGateAccessor featuregates.FeatureGateAccess
}

// lastErrorTracker tracks the last error that occurred during reconciliation.
type lastErrorTracker struct {
	// lastError is the last error that occurred during reconciliation.
	lastError error

	// lastErrorTime is the time at which the last error occurred.
	lastErrorTime metav1.Time

	// count is the number of times we've observed the same error in a row.
	count int
}

// SetupWithManager sets up the controller with the Manager.
func (r *ControlPlaneMachineSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// All predicates are executed before the event handler is called
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("controlplanemachineset").
		For(&machinev1.ControlPlaneMachineSet{}, builder.WithPredicates(util.FilterControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace))).
		Watches(
			&machinev1beta1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace)),
			builder.WithPredicates(util.FilterControlPlaneMachines(r.Namespace)),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(util.ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace)),
			builder.WithPredicates(util.FilterControlPlaneNodes()),
		).
		Watches(
			&configv1.ClusterOperator{},
			handler.EnqueueRequestsFromMapFunc(util.ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace)),
			builder.WithPredicates(util.FilterClusterOperator(r.OperatorName)),
		).
		Watches(
			&configv1.Infrastructure{},
			handler.EnqueueRequestsFromMapFunc(util.ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace)),
			builder.WithPredicates(util.FilterInfrastructure(infrastructureName)),
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
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	r.Recorder = mgr.GetEventRecorderFor("control-plane-machine-set-controller")
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

	// Track the reconcile error as our last error.
	r.setLastError(logger, cpms, err)

	if err := r.updateControlPlaneMachineSetStatus(ctx, logger, cpms, patchBase); err != nil {
		// Don't return an error here so that we have an opportunity to update the cluster operator status.
		errs = append(errs, fmt.Errorf("error updating control plane machine set status: %w", err))
	}

	if isActive(cpms) {
		if err := r.updateClusterOperatorStatus(ctx, logger, cpms); err != nil {
			// Don't return an error here so we can aggregate the errors with previous updates.
			errs = append(errs, fmt.Errorf("error updating control plane machine set status: %w", err))
		}
	} else {
		// When inactive, the status of the ControlPlaneMachineSet should not influence the
		// the cluster health, so set the operator to available.
		if err := r.setClusterOperatorAvailable(ctx, logger); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to reconcile cluster operator status: %w", err)
		}
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

	currentFeatureGates, err := r.FeatureGateAccessor.CurrentFeatureGates()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to retrieve current feature gates: %w", err)
	}

	opts := providers.MachineProviderOptions{
		AllowMachineNamePrefix: currentFeatureGates.Enabled(features.FeatureGateCPMSMachineNamePrefix),
	}

	machineProvider, err := providers.NewMachineProvider(ctx, logger, r.Client, r.Recorder, cpms, opts)
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

	if err := r.validateClusterState(ctx, logger, cpms, machineInfos); err != nil {
		return ctrl.Result{}, fmt.Errorf("error validating cluster state: %w", err)
	}

	if isControlPlaneMachineSetDegraded(cpms) {
		logger.V(1).Info(degradedClusterState)
		return ctrl.Result{}, nil
	}

	if !isActive(cpms) {
		// When inactive, we don't want to modify the machines at all so stop processing here.
		return ctrl.Result{}, nil
	}

	if err := r.ensureOwnerReferences(ctx, logger, cpms, machineInfos); err != nil {
		return ctrl.Result{}, fmt.Errorf("error ensuring owner references: %w", err)
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

// setLastError handles the reconcile error and tracks similar errors so that we can set a condition
// when the reconciler is repeatedly failing with the same error.
func (r *ControlPlaneMachineSetReconciler) setLastError(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, err error) {
	switch {
	case err == nil:
		// When no error occurred, stop tracking the last error.
		r.lastError = nil
	case r.lastError != nil && err.Error() == r.lastError.lastError.Error():
		// When the error is the same as the last error, increment the count.
		r.lastError.count++
	default:
		// Either the last error is nil or the error is different from the last error.
		// Track the new error and reset the count.
		r.lastError = &lastErrorTracker{
			lastError:     err,
			lastErrorTime: metav1.Now(),
			count:         1,
		}
	}

	if r.lastError != nil && r.lastError.count > 1 {
		logger.V(4).Info(
			"Reconciler returned error repeatedly",
			"count", r.lastError.count,
			"error", r.lastError.lastError.Error(),
		)
	}

	errorCondition := getErrorCondition(cpms, r.lastError)
	meta.SetStatusCondition(&cpms.Status.Conditions, errorCondition)
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
//   - All Nodes in the cluster claiming to be control plane nodes have a valid machine.
//   - At least 1 of the control plane machines is in the ready state (if there are no ready Machines then the cluster
//     is likely misconfigured).
//   - We have the correct number of indexes:
//     -- Right number of indexes, valid.
//     -- Too few indexes, valid. We will later scale up without user intervention when we perform reconcileMachineUpdates.
//     -- Too many indexes, invalid. We set the operator to degraded and ask the user for manual intervention.
//   - No replacement machines (one that doesn't need update but has an equivalent in the index that needs update) have an error.
func (r *ControlPlaneMachineSetReconciler) validateClusterState(ctx context.Context, logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, machineInfos map[int32][]machineproviders.MachineInfo) error {
	sortedIndexedMs := sortMachineInfosByIndex(machineInfos)

	// Check that at least one of the control plane machines is in the ready state
	// (if there are no ready Machines then the cluster is likely misconfigured).
	if ok := r.checkReadyControlPlaneMachineExists(logger, cpms, sortedIndexedMs); !ok {
		return nil
	}

	// Check that all Nodes in the cluster claiming to be control plane nodes have a valid machine.
	ok, err := r.checkControlPlaneNodesToMachinesMappings(ctx, logger, cpms, sortedIndexedMs)
	if err != nil {
		return fmt.Errorf("failed to check control plane nodes to machines mappings: %w", err)
	} else if !ok {
		return nil
	}

	// Check that the number of the cpms indexes in the cluster is valid.
	if ok := r.checkCorrectNumberOfIndexes(logger, cpms, sortedIndexedMs); !ok {
		return nil
	}

	// Check that the number of Updated (Ready and with Up-to-date spec) Machines in an index is valid.
	if ok := r.checkValidNumerOfUpdatedMachinesPerIndex(logger, cpms, sortedIndexedMs); !ok {
		return nil
	}

	// Check that no replacement machines (one that doesn't need update but has an equivalent in the index that needs update)
	// have an error.
	if ok := r.checkNoErrorForReplacements(logger, cpms, sortedIndexedMs); !ok {
		return nil
	}

	// Normal conditions case.
	if meta.FindStatusCondition(cpms.Status.Conditions, conditionDegraded) == nil {
		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionDegraded,
			Status: metav1.ConditionFalse,
		})
	}

	if meta.FindStatusCondition(cpms.Status.Conditions, conditionProgressing) == nil {
		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionProgressing,
			Status: metav1.ConditionFalse,
		})
	}

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

	for _, machineInfo := range machineInfos {
		out[machineInfo.Index] = append(out[machineInfo.Index], machineInfo)
	}

	// If for any reason there aren't enough indexes to meet the replica count,
	// populate empty indexes starting from index 0 until we have the correct
	// number of indexes.
	for i := int32(0); int32(len(out)) < *cpms.Spec.Replicas; i++ { //nolint:gosec
		if _, ok := out[i]; !ok {
			out[i] = []machineproviders.MachineInfo{}
		}
	}

	return out, nil
}

// isControlPlaneMachineSetDegraded determines whether or not the ControlPlaneMachineSet
// has a true, degraded condition.
func isControlPlaneMachineSetDegraded(cpms *machinev1.ControlPlaneMachineSet) bool {
	return meta.IsStatusConditionTrue(cpms.Status.Conditions, conditionDegraded)
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

// checkControlPlaneNodesToMachinesMappings checks that all nodes in the cluster claiming to be control plane nodes are referenced by a control plane machine.
func (r *ControlPlaneMachineSetReconciler) checkControlPlaneNodesToMachinesMappings(ctx context.Context, logger logr.Logger,
	cpms *machinev1.ControlPlaneMachineSet, sortedIndexedMs []indexToMachineInfos) (bool, error) {
	var (
		unmanagedNodeNames []string
	)

	cpmsNodes, err := r.fetchControlPlaneNodes(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to fetch control plane nodes: %w", err)
	}

	for _, node := range cpmsNodes {
		found := false

		for _, indexToMachines := range sortedIndexedMs {
			indexMachines := indexToMachines.machineInfos
			for _, machineInfo := range indexMachines {
				if machineInfo.NodeRef != nil && machineInfo.NodeRef.ObjectMeta.Name == node.ObjectMeta.Name {
					found = true
					break
				}
			}
		}

		if !found {
			unmanagedNodeNames = append(unmanagedNodeNames, node.ObjectMeta.Name)
		}
	}

	if len(unmanagedNodeNames) > 0 {
		logger.Error(
			fmt.Errorf("%w: %s", errFoundUnmanagedControlPlaneNodes, strings.Join(unmanagedNodeNames, ", ")),
			"Observed unmanaged control plane nodes",
			"unmanagedNodes", strings.Join(unmanagedNodeNames, ","),
		)

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionProgressing,
			Status: metav1.ConditionFalse,
			Reason: reasonOperatorDegraded,
		})

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:    conditionDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  reasonUnmanagedNodes,
			Message: fmt.Sprintf("Found %d unmanaged node(s)", len(unmanagedNodeNames)),
		})

		return false, nil
	}

	return true, nil
}

// checkReadyControlPlaneMachineExists checks that at least one ready control plane machine exists in the cluster.
func (r *ControlPlaneMachineSetReconciler) checkReadyControlPlaneMachineExists(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, sortedIndexedMs []indexToMachineInfos) bool {
	var nonReadyMachineNames []string

	for _, indexToMachines := range sortedIndexedMs {
		indexMachines := indexToMachines.machineInfos
		if len(indexMachines) > 0 && isEmpty(readyMachines(indexMachines)) {
			nonReadyMachineNames = append(nonReadyMachineNames, indexMachines[0].MachineRef.ObjectMeta.Name)
		}
	}

	if len(nonReadyMachineNames) == len(sortedIndexedMs) {
		logger.Error(
			errNoReadyControlPlaneMachines,
			"No ready control plane machines found",
			"unreadyMachines", strings.Join(nonReadyMachineNames, ","),
		)

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionProgressing,
			Status: metav1.ConditionFalse,
			Reason: reasonOperatorDegraded,
		})

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:    conditionDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  reasonNoReadyMachines,
			Message: "No ready control plane machines found",
		})

		return false
	}

	return true
}

// checkCorrectNumberOfIndexes checks that the number of control plane machine set indexes found in the cluster is valid.
func (r *ControlPlaneMachineSetReconciler) checkCorrectNumberOfIndexes(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, sortedIndexedMs []indexToMachineInfos) bool {
	currentIndexesCount := int32(len(sortedIndexedMs)) //nolint:gosec

	switch {
	case currentIndexesCount == *cpms.Spec.Replicas:
		// Right number of indexes. The cluster state is valid.
	case currentIndexesCount < *cpms.Spec.Replicas:
		// Too few indexes. The cluster state is valid.
		// We will later scale up without user intervention when we perform reconcileMachineUpdates.
	case currentIndexesCount > *cpms.Spec.Replicas:
		// Too many indexes. The cluster state is invalid.
		// We set the operator to degraded and ask the user for manual intervention.
		excessiveIndexes := currentIndexesCount - *cpms.Spec.Replicas
		logger.Error(
			fmt.Errorf("%w: %d index(es) are in excess", errFoundExcessiveIndexes, excessiveIndexes),
			"Observed an excessive number of control plane machine indexes",
			"excessIndexes", excessiveIndexes,
		)

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionProgressing,
			Status: metav1.ConditionFalse,
			Reason: reasonOperatorDegraded,
		})

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:    conditionDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  reasonExcessIndexes,
			Message: fmt.Sprintf("Observed %d index(es) in excess", excessiveIndexes),
		})

		return false
	}

	return true
}

// checkValidNumerOfUpdatedMachinesPerIndex checks that the number of updated machines in an index is valid.
func (r *ControlPlaneMachineSetReconciler) checkValidNumerOfUpdatedMachinesPerIndex(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, sortedIndexedMs []indexToMachineInfos) bool {
	for _, indexToMachines := range sortedIndexedMs {
		updatedMachinesCount := len(updatedMachines(indexToMachines.machineInfos))

		switch {
		case updatedMachinesCount <= 1:
			// Valid numer of Updated Machines. The cluster state is valid.
		case updatedMachinesCount > 1:
			switch cpms.Spec.Strategy.Type {
			case machinev1.RollingUpdate:
				// Even though there is an excess number of Updated Machines,
				// with the RollingUpdate update strategy we consider this a valid state for the cluster.
				// We carry on and let the RollingUpdate reconciliation handle the excess in Updated Machines.
			case machinev1.Recreate:
				// Even though there is an excess number of Updated Machines,
				// with the Recreate update strategy we consider this a valid state for the cluster.
				// We carry on and let the Recreate reconciliation handle the excess in Updated Machines.
			case machinev1.OnDelete:
				// Too many Updated Machines. With OnDelete update strategy,
				// if there are an excess number of Updated Machines
				// we set the operator to degraded and ask the user for manual intervention,
				// to remove the excess replicas.
				excessiveUpdatedReplicas := updatedMachinesCount - 1

				logger.Error(
					fmt.Errorf("%w: %d updated replica(s) are in excess for index %d",
						errFoundExcessiveUpdatedReplicas, excessiveUpdatedReplicas, indexToMachines.index),
					"Observed an excessive number of updated replica(s) for a single index",
					"excessUpdatedReplicas", excessiveUpdatedReplicas,
				)

				meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
					Type:   conditionProgressing,
					Status: metav1.ConditionFalse,
					Reason: reasonOperatorDegraded,
				})

				meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
					Type:    conditionDegraded,
					Status:  metav1.ConditionTrue,
					Reason:  reasonExcessUpdatedReplicas,
					Message: fmt.Sprintf("Observed %d updated machine(s) in excess for index %d", excessiveUpdatedReplicas, indexToMachines.index),
				})

				return false
			}
		}
	}

	return true
}

// checkNoErrorForReplacements checks that there is no errored replacement machine.
func (r *ControlPlaneMachineSetReconciler) checkNoErrorForReplacements(logger logr.Logger, cpms *machinev1.ControlPlaneMachineSet, sortedIndexedMs []indexToMachineInfos) bool {
	var erroredReplacementMachineNames []string

	for _, indexToMachines := range sortedIndexedMs {
		machines := indexToMachines.machineInfos
		machinesPending := pendingMachines(machines)
		machinesOutdated := needReplacementMachines(machines)

		if hasAny(machinesOutdated) && hasAny(machinesPending) {
			for _, m := range machinesPending {
				if m.ErrorMessage != "" {
					erroredReplacementMachineNames = append(erroredReplacementMachineNames, m.MachineRef.ObjectMeta.Name)
				}
			}
		}
	}

	if len(erroredReplacementMachineNames) > 0 {
		logger.Error(
			fmt.Errorf("%w: %s", errFoundErroredReplacementControlPlaneMachine, strings.Join(erroredReplacementMachineNames, ", ")),
			"Observed failed replacement control plane machines",
			"failedReplacements", strings.Join(erroredReplacementMachineNames, ","),
		)

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:   conditionProgressing,
			Status: metav1.ConditionFalse,
			Reason: reasonOperatorDegraded,
		})

		meta.SetStatusCondition(&cpms.Status.Conditions, metav1.Condition{
			Type:    conditionDegraded,
			Status:  metav1.ConditionTrue,
			Reason:  reasonFailedReplacement,
			Message: fmt.Sprintf("Observed %d replacement machine(s) in error state", len(erroredReplacementMachineNames)),
		})

		return false
	}

	return true
}

// fetchControlPlaneNodes fetches a sorted list of unique nodes that have the "control-plane" (and/or legacy "master") labels.
func (r *ControlPlaneMachineSetReconciler) fetchControlPlaneNodes(ctx context.Context) ([]corev1.Node, error) {
	cpmsNodesLookup := make(map[string]struct{})
	sortedCpmsNodes := []corev1.Node{}

	for _, label := range []string{masterNodeRoleLabel, controlPlaneNodeRoleLabel} {
		nodesList := &corev1.NodeList{}
		if err := r.List(ctx, nodesList, &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{label: ""}),
		}); err != nil {
			return nil, fmt.Errorf("failed to get Nodes: %w", err)
		}

		for _, n := range nodesList.Items {
			if _, exists := cpmsNodesLookup[n.ObjectMeta.Name]; !exists {
				cpmsNodesLookup[n.ObjectMeta.Name] = struct{}{}

				sortedCpmsNodes = append(sortedCpmsNodes, n)
			}
		}
	}

	return sortedCpmsNodes, nil
}

// isActive determines whether the ControlPlaneMachineSet is marked active.
func isActive(cpms *machinev1.ControlPlaneMachineSet) bool {
	return cpms.Spec.State == machinev1.ControlPlaneMachineSetStateActive
}
