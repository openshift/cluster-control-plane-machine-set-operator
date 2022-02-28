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

	machinev1 "github.com/openshift/api/machine/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// ControlPlaneMachineSetWebhook acts as a webhook validator for the
// machinev1beta1.ControlPlaneMachineSet resource.
type ControlPlaneMachineSetWebhook struct{}

// SetupWebhookWithManager sets up a new ControlPlaneMachineSet webhook with the manager.
func (r *ControlPlaneMachineSetWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewWebhookManagedBy(mgr).
		WithValidator(r).
		For(&machinev1.ControlPlaneMachineSet{}).
		Complete(); err != nil {
		return fmt.Errorf("error constructing ControlPlaneMachineSet webhook: %w", err)
	}

	return nil
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-machine-openshift-io-v1-controlplanemachineset,mutating=false,failurePolicy=fail,groups=machine.openshift.io,resources=controlplanemachinesets,versions=v1,name=controlplanemachineset.machine.openshift.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ControlPlaneMachineSetWebhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
