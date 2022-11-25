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

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	machineFinalizer = "machine.openshift.io/finalizer"

	phaseDeleting     = "Deleting"
	phaseProvisioning = "Provisioning"
	phaseProvisioned  = "Provisioned"
	phaseRunning      = "Running"
)

// MachineManagerOptions are options for the machine manager.
// This allows the behaviour of the integration test to be configured.
type MachineManagerOptions struct {
	ActionDelay time.Duration
}

// IntegrationMachineManager is a controller that manages the lifecycle of machines.
// It moves them through the expected lifecycle stages over a short period of time
// to simulate the behaviour of the machine controller.
type IntegrationMachineManager interface {
	SetupWithManager(ctrl.Manager) error
}

// NewIntegrationMachineManager creates a new machine manager.
func NewIntegrationMachineManager(opts MachineManagerOptions) IntegrationMachineManager {
	return &integrationMachineManager{
		delay:      opts.ActionDelay,
		lastAction: make(map[reconcile.Request]action),
	}
}

// action tracks the last action performed on a machine.
type action struct {
	timeStamp time.Time
}

// integrationMachineManager is a controller that manages the lifecycle of machines.
type integrationMachineManager struct {
	client.Client

	delay      time.Duration
	lastAction map[reconcile.Request]action
}

// SetupWithManager sets up the controller with the Manager.
func (r *integrationMachineManager) SetupWithManager(mgr ctrl.Manager) error {
	// Set the client before we start the controller.
	// This avoids any race conditions.
	r.Client = mgr.GetClient()

	// All predicates are executed before the event handler is called
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&machinev1beta1.Machine{}).
		// Override the default log constructor as it makes the logs very chatty.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			return mgr.GetLogger().WithValues(
				"controller", "machinemanager",
			)
		}).
		Complete(r); err != nil {
		return fmt.Errorf("could not set up machine manager: %w", err)
	}

	return nil
}

// Reconcile handles the reconciliation loop for the machine manager.
// It sets the finalizer and then progresses the Machine through the phases until it is running.
func (r *integrationMachineManager) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx, "namespace", req.Namespace, "name", req.Name)

	logger.Info("Reconciling machine")
	defer logger.Info("Finished reconciling machine")

	machine := &machinev1beta1.Machine{}
	machineKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}

	if err := r.Get(ctx, machineKey, machine); apierrors.IsNotFound(err) {
		logger.Info("No machine found, ignoring request")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch machine: %w", err)
	}

	if machine.DeletionTimestamp != nil {
		if machine.Status.Phase != nil && *machine.Status.Phase != phaseDeleting {
			return r.setPhase(ctx, logger, req, machine, phaseDeleting)
		}

		return r.removeFinalizer(ctx, logger, req, machine)
	}

	// All machines should have a finalizer set first.
	if len(machine.GetFinalizers()) == 0 {
		return r.addFinalizer(ctx, logger, req, machine)
	}

	switch pointer.StringDeref(machine.Status.Phase, "") {
	case "":
		return r.setPhase(ctx, logger, req, machine, phaseProvisioning)
	case phaseProvisioning:
		return r.setPhase(ctx, logger, req, machine, phaseProvisioned)
	case phaseProvisioned:
		return r.setPhase(ctx, logger, req, machine, phaseRunning)
	}

	return reconcile.Result{}, nil
}

// addFinalizer sets the finalizer on the machine.
func (r *integrationMachineManager) addFinalizer(ctx context.Context, logger logr.Logger, req reconcile.Request, machine *machinev1beta1.Machine) (reconcile.Result, error) {
	machine.SetFinalizers([]string{machineFinalizer})

	logger.Info("Adding Finalizer")

	if err := r.Update(ctx, machine); err != nil {
		return reconcile.Result{}, fmt.Errorf("could not set machine finalizer: %w", err)
	}

	r.lastAction[req] = action{
		timeStamp: time.Now(),
	}

	return reconcile.Result{}, nil
}

// removeFinalizer removes the finalizer from the machine.
func (r *integrationMachineManager) removeFinalizer(ctx context.Context, logger logr.Logger, req reconcile.Request, machine *machinev1beta1.Machine) (reconcile.Result, error) {
	// Make sure to delay if the last action was less than the delay in the past.
	if act := r.lastAction[req]; shouldRequeue(act, r.delay) {
		return requeueAfter(act.timeStamp, r.delay), nil
	}

	machine.SetFinalizers([]string{})

	logger.Info("Removing Finalizer")

	if err := r.Update(ctx, machine); err != nil {
		return reconcile.Result{}, fmt.Errorf("could not set machine finalizer: %w", err)
	}

	r.lastAction[req] = action{
		timeStamp: time.Now(),
	}

	return reconcile.Result{}, nil
}

// setPhase sets the phase of the machine to the given phase.
func (r *integrationMachineManager) setPhase(ctx context.Context, logger logr.Logger, req reconcile.Request, machine *machinev1beta1.Machine, phase string) (reconcile.Result, error) {
	// Make sure to delay if the last action was less than the delay in the past.
	if act := r.lastAction[req]; shouldRequeue(act, r.delay) {
		return requeueAfter(act.timeStamp, r.delay), nil
	}

	logger.Info("Setting phase", "phase", phase)
	machine.Status.Phase = pointer.String(phase)

	if err := r.Status().Update(ctx, machine); err != nil {
		return reconcile.Result{}, fmt.Errorf("could not set machine phase: %w", err)
	}

	r.lastAction[req] = action{
		timeStamp: time.Now(),
	}

	return reconcile.Result{}, nil
}

// shouldRequeue determines if the last action was less than the delay in the past.
// If it was, then we requeue.
func shouldRequeue(act action, delay time.Duration) bool {
	requeueTime := act.timeStamp.Add(delay)

	return time.Now().Before(requeueTime)
}

// requeueAfter calculates the requested delay and returns a reconcile.Result.
func requeueAfter(base time.Time, delay time.Duration) reconcile.Result {
	requeueTime := base.Add(delay)

	return reconcile.Result{RequeueAfter: time.Until(requeueTime)}
}
