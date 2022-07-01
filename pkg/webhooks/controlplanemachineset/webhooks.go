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
	"reflect"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	// openshiftMachineRoleLabel is the OpenShift Machine API machine role label.
	// This must be present on all OpenShift Machine API Machine templates.
	openshiftMachineRoleLabel = "machine.openshift.io/cluster-api-machine-role"

	// openshiftMachineRoleLabel is the OpenShift Machine API machine type label.
	// This must be present on all OpenShift Machine API Machine templates.
	openshiftMachineTypeLabel = "machine.openshift.io/cluster-api-machine-type"

	// masterMachineRole is the master role/type that is required to be set on
	// all OpenShift Machine API Machine templates.
	masterMachineRole = "master"
)

var (
	// errObjNotCPMS is an error when casting to ControlPlaneMachineSet fails.
	errObjNotCPMS = errors.New("validated object is not of type control plane machine set")

	// errUpdateNilCPMS is an error when update is called with nil ControlPlaneMachineSet.
	errUpdateNilCPMS = errors.New("cannot update nil control plane machine set")
)

// ControlPlaneMachineSetWebhook acts as a webhook validator for the
// machinev1beta1.ControlPlaneMachineSet resource.
type ControlPlaneMachineSetWebhook struct {
	client client.Client
}

// SetupWebhookWithManager sets up a new ControlPlaneMachineSet webhook with the manager.
func (r *ControlPlaneMachineSetWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
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
	var errs []error

	cpms, ok := obj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return errObjNotCPMS
	}

	controlPlaneMachines, err := r.fetchControlPlaneMachines(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch existing control plane machines: %w", err)
	}

	// Ensure Control Plane Machine count matches the ControlPlaneMachineSet replicas
	if cpms.Spec.Replicas == nil {
		errs = append(errs, field.Required(field.NewPath("spec", "replicas"), "replicas field is required"))
	} else if int(*cpms.Spec.Replicas) != len(controlPlaneMachines) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "replicas"),
			fmt.Sprintf("control plane machine set replicas (%d) does not match the current number of control plane machines (%d)", *cpms.Spec.Replicas, len(controlPlaneMachines))))
	}

	// Ensure CPMS created with invalid name is not allowed
	if cpms.Name != "cluster" {
		errs = append(errs, field.Invalid(field.NewPath("name"), cpms.Name, "control plane machine set name must be cluster"))
	}

	// Ensure required labels are set and all machines are matching the label selector
	errs = append(errs, checkMachineLabels(cpms)...)

	// Ensure failure domains of Control Plane Machines match the ControlPlaneMachineSet on create
	switch cpms.Spec.Template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		errs = append(errs, checkFailureDomains(cpms, controlPlaneMachines)...)
	default:
		errs = append(errs, field.NotSupported(field.NewPath("spec", "template", "machineType"), cpms.Spec.Template.MachineType,
			[]string{string(machinev1.OpenShiftMachineV1Beta1MachineType)}))
	}

	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	errs := []error{}

	if oldObj == nil {
		return errUpdateNilCPMS
	}

	oldCPMS, ok := oldObj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return errObjNotCPMS
	}

	newCPMS, ok := newObj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return errObjNotCPMS
	}

	// Ensure spec.Replicas is immutable on update
	if oldCPMS.Spec.Replicas == nil || newCPMS.Spec.Replicas == nil {
		errs = append(errs, field.Required(field.NewPath("spec", "replicas"), "replicas field is required"))
	} else if *oldCPMS.Spec.Replicas != *newCPMS.Spec.Replicas {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "replicas"), "control plane machine set replicas cannot be changed"))
	}

	// Ensure selector is immutable on update
	if !reflect.DeepEqual(oldCPMS.Spec.Selector, newCPMS.Spec.Selector) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "selector"), "control plane machine set selector is immutable"))
	}

	// Ensure required labels are set and all machines are matching the label selector
	errs = append(errs, checkMachineLabels(newCPMS)...)

	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// fetchControlPlaneMachines returns all control plane machines in the cluster.
func (r *ControlPlaneMachineSetWebhook) fetchControlPlaneMachines(ctx context.Context) ([]machinev1beta1.Machine, error) {
	machineList := machinev1beta1.MachineList{}
	if err := r.client.List(ctx, &machineList); err != nil {
		return nil, fmt.Errorf("error querying api for machines: %w", err)
	}

	controlPlaneMachines := []machinev1beta1.Machine{}

	for _, machine := range machineList.Items {
		if machine.Labels[openshiftMachineRoleLabel] == masterMachineRole && machine.Labels[openshiftMachineTypeLabel] == masterMachineRole {
			controlPlaneMachines = append(controlPlaneMachines, machine)
		}
	}

	return controlPlaneMachines, nil
}

// checkMachineLabels ensures that required labels are set and all machines are matching the label selector.
func checkMachineLabels(cpms *machinev1.ControlPlaneMachineSet) []error {
	machineTemplatePath := field.NewPath("spec", "template", "machines_v1beta1_machine_openshift_io")
	errs := []error{}

	// Ensure machine template has all required labels, and where required, required values
	requiredLabels := []struct {
		label string
		value string
	}{
		{label: machinev1beta1.MachineClusterIDLabel},
		{label: openshiftMachineRoleLabel, value: masterMachineRole},
		{label: openshiftMachineTypeLabel, value: masterMachineRole},
	}

	for _, required := range requiredLabels {
		value, ok := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels[required.label]
		if !ok || value == "" {
			errs = append(errs, field.Required(machineTemplatePath.Child("metadata", "labels"), fmt.Sprintf("%s label is required", required.label)))
		}

		if required.value != "" && required.value != value {
			errs = append(errs, field.Invalid(machineTemplatePath.Child("metadata", "labels"), value, fmt.Sprintf("%s label must have value: %s", required.label, required.value)))
		}
	}

	// Ensure machines are matched by selectors
	selector, err := metav1.LabelSelectorAsSelector(&cpms.Spec.Selector)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec", "selector"), cpms.Spec.Selector, fmt.Errorf("could not convert label selector to selector: %w", err).Error()))
	}

	if selector != nil && !selector.Matches(labels.Set(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels)) {
		errs = append(errs, field.Invalid(machineTemplatePath.Child("metadata", "labels"),
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels, "selector does not match template labels"))
	}

	return errs
}

// checkFailureDomains ensures that failure domains of Control Plane Machines match the ControlPlaneMachineSet.
func checkFailureDomains(cpms *machinev1.ControlPlaneMachineSet, controlPlaneMachines []machinev1beta1.Machine) []error {
	machineTemplatePath := field.NewPath("spec", "template", "machines_v1beta1_machine_openshift_io")
	errs := []error{}

	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil {
		return append(errs, field.Required(machineTemplatePath,
			fmt.Sprintf("Specified template type %s has nil value", machinev1.OpenShiftMachineV1Beta1MachineType)))
	}

	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform == "" {
		return nil
	}

	machineFailureDomains, err := providerconfig.ExtractFailureDomainsFromMachines(controlPlaneMachines)
	if err != nil {
		return append(errs, field.InternalError(machineTemplatePath.Child("failureDomains", "platform"),
			fmt.Errorf("could not get failure domains from cluster machines on platform %s: %w", cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform, err)))
	}

	specifiedFailureDomains, err := failuredomain.NewFailureDomains(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains)
	if err != nil {
		return append(errs, field.Invalid(machineTemplatePath.Child("failureDomains"),
			cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains,
			fmt.Sprintf("error getting failure domains from control plane machine set machine template: %v", err)))
	}

	// Failure domains used by control plane machines but not specified in the control plane machine set
	if missingFailureDomains := missingFailureDomains(machineFailureDomains, specifiedFailureDomains); len(missingFailureDomains) > 0 {
		errs = append(errs, field.Forbidden(machineTemplatePath.Child("failureDomains"), fmt.Sprintf("control plane machines are using unspecified failure domain(s) %s", missingFailureDomains)))
	}

	// Failure domains specified in the control plane machine set but not used by control plane machines
	if missingFailureDomains := missingFailureDomains(specifiedFailureDomains, machineFailureDomains); len(missingFailureDomains) > 0 {
		errs = append(errs, field.Forbidden(machineTemplatePath.Child("failureDomains"), fmt.Sprintf("no control plane machine is using specified failure domain(s) %s", missingFailureDomains)))
	}

	return errs
}

// missingFailureDomains returns failure domains from list1 that are not in list2.
func missingFailureDomains(list1 []failuredomain.FailureDomain, list2 []failuredomain.FailureDomain) []failuredomain.FailureDomain {
	missing := []failuredomain.FailureDomain{}

	for _, outerItem := range list1 {
		found := false

		for _, innerItem := range list2 {
			if outerItem.Equal(innerItem) {
				found = true
				break
			}
		}

		if !found {
			missing = append(missing, outerItem)
		}
	}

	return missing
}
