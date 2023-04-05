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

package controlplanemachinesetgenerator

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// infrastructureName is the name of the Infrastructure,
	// as Infrastructure is a singleton within the cluster.
	infrastructureName                   = "cluster"
	machineRoleLabelKey                  = "machine.openshift.io/cluster-api-machine-role"
	clusterIDLabelKey                    = "machine.openshift.io/cluster-api-cluster"
	clusterMachineRoleLabelKey           = "machine.openshift.io/cluster-api-machine-role"
	clusterMachineTypeLabelKey           = "machine.openshift.io/cluster-api-machine-type"
	clusterMachineLabelValueMaster       = "master"
	clusterMachineLabelValueControlPlane = "control-plane"
)

const (
	unsupportedNumberOfControlPlaneMachines     = "Unable to generate control plane machine set, unsupported number of control plane machines"
	unsupportedPlatform                         = "Unable to generate control plane machine set, unsupported platform"
	controlPlaneMachineSetNotFound              = "Control plane machine set not found"
	controlPlaneMachineSetUpToDate              = "Control plane machine set is up to date"
	controlPlaneMachineSetOutdated              = "Control plane machine set is outdated"
	controlPlaneMachineSetCreated               = "Created updated control plane machine set"
	controlPlaneMachineSetDeleted               = "Deleted outdated control plane machine set"
	controlPlaneMachineSetReconciling           = "Reconciling control plane machine set"
	controlPlaneMachineSetReconciliationFinshed = "Finished reconciling control plane machine set"
)

var (
	// errUnsupportedPlatform defines an error for an unsupported platform.
	errUnsupportedPlatform = errors.New("unsupported platform")
	// errNilProviderSpec is an error used when provider spec is nil.
	errNilProviderSpec = errors.New("provider spec is nil")
)

// ControlPlaneMachineSetGeneratorReconciler reconciles a ControlPlaneMachineSet object.
type ControlPlaneMachineSetGeneratorReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RESTMapper meta.RESTMapper

	// Namespace is the namespace in which the ControlPlaneMachineSetGenerator controller should operate.
	// Any ControlPlaneMachineSet not in this namespace should be ignored.
	Namespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ControlPlaneMachineSetGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// All predicates are executed before the event handler is called.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&machinev1.ControlPlaneMachineSet{}, builder.WithPredicates(util.FilterControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace))).
		Watches(&source.Kind{Type: &machinev1beta1.Machine{}}, handler.EnqueueRequestsFromMapFunc(util.ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace))).
		// Override the default log constructor as it makes the logs very chatty.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			return mgr.GetLogger().WithValues(
				"controller", "controlplanemachinesetgenerator",
			)
		}).
		Complete(r); err != nil {
		return fmt.Errorf("could not set up controller for control plane machine set generator: %w", err)
	}

	// Set up API helpers from the manager.
	r.Scheme = mgr.GetScheme()
	r.RESTMapper = mgr.GetRESTMapper()

	return nil
}

// Reconcile reconciles the ControlPlaneMachineSet object.
func (r *ControlPlaneMachineSetGeneratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "namespace", req.Namespace, "name", req.Name)

	logger.V(1).Info(controlPlaneMachineSetReconciling)
	defer logger.V(1).Info(controlPlaneMachineSetReconciliationFinshed)

	cpms := &machinev1.ControlPlaneMachineSet{}
	cpmsKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}

	// Fetch the ControlPlaneMachineSet.
	if err := r.Get(ctx, cpmsKey, cpms); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("unable to fetch control plane machine set: %w", err)
	}

	// If the control plane machine set is being deleted, we need to handle that rather than the regular reconcile flow.
	if cpms.GetDeletionTimestamp() != nil {
		// The ControlPlaneMachineSet is being deleted, requeue.
		// We will create a new one later once it's completely removed.
		return reconcile.Result{}, nil
	}

	if cpms.Spec.State == machinev1.ControlPlaneMachineSetStateActive {
		// If the control plane machine set is already set to active,
		// it's a no-op for this controller.
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, logger, cpms)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling control plane machine set: %w", err)
	}

	return result, nil
}

// reconcile performs the main business logic of the ControlPlaneMachineSet operator.
// Notably it actions the various parts of the business logic without performing any status updates on the
// ControlPlaneMachineSet object itself, these updates are handled at the parent scope.
//
// The cyclomatic complexity on this function is just above the allowed limit, we don't want to shake up the logic
// and would like to keep it this way.
//
//nolint:cyclop
func (r *ControlPlaneMachineSetGeneratorReconciler) reconcile(ctx context.Context, logger logr.Logger,
	cpms *machinev1.ControlPlaneMachineSet) (ctrl.Result, error) {
	machines, err := r.getControlPlaneMachines(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get control plane machines: %w", err)
	}

	if !r.isSupportedControlPlaneMachinesNumber(logger, machines) {
		return reconcile.Result{}, nil
	}

	machineSets, err := r.getMachineSets(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get machinesets: %w", err)
	}

	infrastructure, err := r.getInfrastructure(ctx, infrastructureName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get infrastructure object: %w", err)
	}

	// generate an up to date ControlPlaneMachineSet based on the current cluster state.
	generatedCPMS, err := r.generateControlPlaneMachineSet(logger, infrastructure.Status.PlatformStatus.Type, machines, machineSets)
	if errors.Is(err, errUnsupportedPlatform) {
		// Do not requeue if the platform is not supported.
		// Nothing to do in this case.
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to generate control plane machine set: %w", err)
	}

	// Ensure that the ControlPlaneMachineSet singleton exists, if it doesn't, create one and requeue.
	if done, result, err := r.ensureControlPlaneMachineSet(ctx, logger, cpms, generatedCPMS); err != nil {
		return result, fmt.Errorf("unable to create control plane machine set: %w", err)
	} else if done {
		return result, nil
	}

	// Compare if the current and the newly generated ControlPlaneMachineSet spec match.
	if diff, err := compareControlPlaneMachineSets(cpms, generatedCPMS); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to compare control plane machine sets: %w", err)
	} else if diff != nil {
		// The two ControlPlaneMachineSets don't match.
		logger.V(1).WithValues("diff", diff).Info(controlPlaneMachineSetOutdated)
		// Recreate the ControlPlaneMachineSet (Delete the currently applied and outdated ControlPlaneMachineSet
		// and Create a new one by applying the newly generated one).
		return r.recreateControlPlaneMachineSet(ctx, logger, cpms, generatedCPMS)
	}

	logger.V(3).Info(controlPlaneMachineSetUpToDate)

	return reconcile.Result{}, nil
}

// generateControlPlaneMachineSet generates a control plane machine set based on the current cluster state.
func (r *ControlPlaneMachineSetGeneratorReconciler) generateControlPlaneMachineSet(logger logr.Logger,
	platformType configv1.PlatformType, machines []machinev1beta1.Machine, machineSets []machinev1beta1.MachineSet) (*machinev1.ControlPlaneMachineSet, error) {
	var (
		cpmsSpecApplyConfig machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration
		err                 error
	)

	switch platformType {
	case configv1.AWSPlatformType:
		cpmsSpecApplyConfig, err = generateControlPlaneMachineSetAWSSpec(machines, machineSets)
		if err != nil {
			return nil, fmt.Errorf("unable to generate control plane machine set spec: %w", err)
		}
	case configv1.AzurePlatformType:
		cpmsSpecApplyConfig, err = generateControlPlaneMachineSetAzureSpec(machines, machineSets)
		if err != nil {
			return nil, fmt.Errorf("unable to generate control plane machine set spec: %w", err)
		}
	case configv1.GCPPlatformType:
		cpmsSpecApplyConfig, err = generateControlPlaneMachineSetGCPSpec(machines, machineSets)
		if err != nil {
			return nil, fmt.Errorf("unable to generate control plane machine set spec: %w", err)
		}
	default:
		logger.V(1).WithValues("platform", platformType).Info(unsupportedPlatform)
		return nil, errUnsupportedPlatform
	}

	cpmsApplyConfig := machinev1builder.ControlPlaneMachineSet(clusterControlPlaneMachineSetName, r.Namespace).WithSpec(&cpmsSpecApplyConfig)

	newCPMS := &machinev1.ControlPlaneMachineSet{}
	if err := convertViaJSON(*cpmsApplyConfig, newCPMS); err != nil {
		return nil, fmt.Errorf("unable to convert ControlPlaneMachineSetApplyConfig to ControlPlaneMachineSet: %w", err)
	}

	return newCPMS, nil
}

// ensureControlPlaneMachineSet ensures that the ControlPlaneMachineSet has been created.
func (r *ControlPlaneMachineSetGeneratorReconciler) ensureControlPlaneMachineSet(ctx context.Context, logger logr.Logger,
	cpms *machinev1.ControlPlaneMachineSet, generatedCPMS *machinev1.ControlPlaneMachineSet) (bool, ctrl.Result, error) {
	if cpms.Name == "" {
		logger.V(1).Info(controlPlaneMachineSetNotFound)

		if err := r.Create(ctx, generatedCPMS); err != nil {
			return true, ctrl.Result{}, fmt.Errorf("unable to create control plane machine set: %w", err)
		}

		logger.V(1).Info(controlPlaneMachineSetCreated)

		// The CPMS resource has been created successfully, requeue.
		return true, ctrl.Result{}, nil
	}

	return false, ctrl.Result{}, nil
}

// recreateControlPlaneMachineSet deletes and creates and up to date ControlPlaneMachineSet.
func (r *ControlPlaneMachineSetGeneratorReconciler) recreateControlPlaneMachineSet(ctx context.Context, logger logr.Logger,
	cpms *machinev1.ControlPlaneMachineSet, generatedCPMS *machinev1.ControlPlaneMachineSet) (ctrl.Result, error) {
	// Delete the current ControlPlaneMachineSet object, as it's out of date.
	// Use preconditions to check if the last seen ResourceVersion
	// is the actual current ResourceVersion on the API,
	// to avoid Deleting the ControlPlaneMachineSet if we just saw a stale (cached) version of it.
	deleteOptions := client.DeleteOptions{Preconditions: &metav1.Preconditions{ResourceVersion: &cpms.ResourceVersion}}
	if err := r.Delete(ctx, cpms, &deleteOptions); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to delete outdated control plane machine set: %w", err)
	}

	logger.V(1).Info(controlPlaneMachineSetDeleted)

	// Create a new ControlPlaneMachineSet object with
	// the freshly generated configuration.
	if err := r.Create(ctx, generatedCPMS); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to re-create updated control plane machine set: %w", err)
	}

	logger.V(1).Info(controlPlaneMachineSetCreated)

	return reconcile.Result{}, nil
}

// getControlPlaneMachines returns a sorted slice of Control Plane Machines.
func (r *ControlPlaneMachineSetGeneratorReconciler) getControlPlaneMachines(ctx context.Context) ([]machinev1beta1.Machine, error) {
	masterMachinesListOptions := []client.ListOption{
		client.InNamespace(r.Namespace),
		client.MatchingLabels{machineRoleLabelKey: clusterMachineLabelValueMaster},
	}

	machinesLabeledMaster := &machinev1beta1.MachineList{}
	if err := r.List(ctx, machinesLabeledMaster, masterMachinesListOptions...); err != nil {
		return nil, fmt.Errorf("unable to list control plane machines: %w", err)
	}

	controlPlaneMachinesListOptions := []client.ListOption{
		client.InNamespace(r.Namespace),
		client.MatchingLabels{machineRoleLabelKey: clusterMachineLabelValueControlPlane},
	}

	machinesLabeledControlPlane := &machinev1beta1.MachineList{}
	if err := r.List(ctx, machinesLabeledControlPlane, controlPlaneMachinesListOptions...); err != nil {
		return nil, fmt.Errorf("unable to list control plane machines: %w", err)
	}

	machines := mergeMachineSlices(machinesLabeledMaster.Items, machinesLabeledControlPlane.Items)

	return sortMachinesByCreationTimeDescending(machines), nil
}

// getMachineSets returns a sorted slice of MachineSets.
func (r *ControlPlaneMachineSetGeneratorReconciler) getMachineSets(ctx context.Context) ([]machinev1beta1.MachineSet, error) {
	machineSets := &machinev1beta1.MachineSetList{}
	if err := r.List(ctx, machineSets, client.InNamespace(r.Namespace)); err != nil {
		return nil, fmt.Errorf("unable to list control plane machines: %w", err)
	}

	return sortMachineSetsByCreationTimeAscending(machineSets.Items), nil
}

// getInfrastructure returns the infrastructure matching the infrastructureName.
func (r *ControlPlaneMachineSetGeneratorReconciler) getInfrastructure(ctx context.Context, infrastructureName string) (*configv1.Infrastructure, error) {
	infrastructure := &configv1.Infrastructure{}
	if err := r.Get(ctx, client.ObjectKey{Name: infrastructureName}, infrastructure); err != nil {
		return nil, fmt.Errorf("unable to get infrastructure object: %w", err)
	}

	return infrastructure, nil
}

// isSupportedControlPlaneMachinesNumber checks if the number of control plane machines in the cluster is supported by the ControlPlaneMachineSet.
func (r *ControlPlaneMachineSetGeneratorReconciler) isSupportedControlPlaneMachinesNumber(logger logr.Logger, machines []machinev1beta1.Machine) bool {
	// Single Control Plane Machine Clusters are not supported by control plane machine set.
	if len(machines) <= 1 {
		logger.V(1).WithValues("count", len(machines)).Info(unsupportedNumberOfControlPlaneMachines)
		return false
	}

	return true
}
