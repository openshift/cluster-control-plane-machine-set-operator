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
	"strings"

	configv1 "github.com/openshift/api/config/v1"
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

	// clusterSingletonName is the OpenShift standard name, "cluster", for singleton
	// resources. All ControlPlaneMachineSet resources must use this name.
	clusterSingletonName = "cluster"
)

var (
	// errObjNotCPMS is an error when casting to ControlPlaneMachineSet fails.
	errObjNotCPMS = errors.New("validated object is not of type control plane machine set")

	// errUpdateNilCPMS is an error when update is called with nil ControlPlaneMachineSet.
	errUpdateNilCPMS = errors.New("cannot update nil control plane machine set")

	// errInvalidDiscriminant is an error used when the discriminated union check cannot
	// find the field named as the discriminant.
	errInvalidDiscriminant = errors.New("invalid discriminant name")
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

	errs = append(errs, validateMetadata(field.NewPath("metadata"), cpms.ObjectMeta)...)
	errs = append(errs, validateSpec(field.NewPath("spec"), nil, cpms)...)
	errs = append(errs, r.validateSpecOnCreate(ctx, field.NewPath("spec"), cpms)...)

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

	cpms, ok := newObj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return errObjNotCPMS
	}

	errs = append(errs, validateMetadata(field.NewPath("metadata"), cpms.ObjectMeta)...)
	errs = append(errs, validateSpec(field.NewPath("spec"), oldCPMS, cpms)...)

	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// validateSpecOnCreate runs the create time validations on the ControlPlaneMachineSet spec.
func (r *ControlPlaneMachineSetWebhook) validateSpecOnCreate(ctx context.Context, parentPath *field.Path, cpms *machinev1.ControlPlaneMachineSet) []error {
	// TODO: This should be MachineInfos and should come from the MachineProvider.
	// This is a blocker for adding Cluster API support right now.
	controlPlaneMachines, err := r.fetchControlPlaneMachines(ctx)
	if err != nil {
		return []error{fmt.Errorf("could not fetch existing control plane machines: %w", err)}
	}

	errs := []error{}

	// Ensure Control Plane Machine count matches the ControlPlaneMachineSet replicas
	if cpms.Spec.Replicas == nil {
		errs = append(errs, field.Required(parentPath.Child("replicas"), "replicas field is required"))
	} else if int(*cpms.Spec.Replicas) != len(controlPlaneMachines) {
		errs = append(errs, field.Forbidden(parentPath.Child("replicas"),
			fmt.Sprintf("control plane machine set replicas (%d) does not match the current number of control plane machines (%d)", *cpms.Spec.Replicas, len(controlPlaneMachines))))
	}

	errs = append(errs, validateTemplateOnCreate(parentPath.Child("template"), cpms.Spec.Template, controlPlaneMachines)...)

	return errs
}

// validateMetadata validates the metadata of the ControlPlaneMachineSet resource.
func validateMetadata(parentPath *field.Path, metadata metav1.ObjectMeta) []error {
	errs := []error{}

	if metadata.Name != clusterSingletonName {
		errs = append(errs, field.Invalid(parentPath.Child("name"), metadata.Name, "control plane machine set name must be cluster"))
	}

	return errs
}

// validateSpec validates that the spec of the ControlPlaneMachineSet resource is valid.
func validateSpec(parentPath *field.Path, oldCPMS, cpms *machinev1.ControlPlaneMachineSet) []error {
	errs := []error{}

	errs = append(errs, validateTemplate(parentPath.Child("template"), cpms.Spec.Template, cpms.Spec.Selector)...)

	// Validate immutability on update.
	if oldCPMS != nil {
		// Ensure spec.Replicas is immutable on update.
		if oldCPMS.Spec.Replicas == nil || cpms.Spec.Replicas == nil {
			errs = append(errs, field.Required(parentPath.Child("replicas"), "replicas field is required"))
		} else if *oldCPMS.Spec.Replicas != *cpms.Spec.Replicas {
			errs = append(errs, field.Forbidden(parentPath.Child("replicas"), "control plane machine set replicas cannot be changed"))
		}

		// Ensure selector is immutable on update.
		if !reflect.DeepEqual(oldCPMS.Spec.Selector, cpms.Spec.Selector) {
			errs = append(errs, field.Forbidden(parentPath.Child("selector"), "control plane machine set selector is immutable"))
		}
	}

	return errs
}

// validateTemplate validates the common (on create and update) checks for the ControlPlaneMachineSet template.
func validateTemplate(parentPath *field.Path, template machinev1.ControlPlaneMachineSetTemplate, selector metav1.LabelSelector) []error {
	switch template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		openshiftMachineTemplatePath := parentPath.Child(string(machinev1.OpenShiftMachineV1Beta1MachineType))

		if template.OpenShiftMachineV1Beta1Machine == nil {
			// Note this is a rare exception to discriminated union rules for the naming of this field.
			// It matches the discriminator exactly.
			return []error{field.Required(openshiftMachineTemplatePath, fmt.Sprintf("%s is required when machine type is %s", machinev1.OpenShiftMachineV1Beta1MachineType, machinev1.OpenShiftMachineV1Beta1MachineType))}
		}

		return validateOpenShiftMachineV1BetaTemplate(openshiftMachineTemplatePath, *template.OpenShiftMachineV1Beta1Machine, selector)
	default:
		return []error{field.NotSupported(parentPath.Child("machineType"), template.MachineType, []string{string(machinev1.OpenShiftMachineV1Beta1MachineType)})}
	}
}

// validateTemplateOnCreate validates the failure domains defined in the template match up with the Machines
// that already exist within the cluster. This check is only performed on create.
func validateTemplateOnCreate(parentPath *field.Path, template machinev1.ControlPlaneMachineSetTemplate, machines []machinev1beta1.Machine) []error {
	switch template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		openshiftMachineTemplatePath := parentPath.Child(string(machinev1.OpenShiftMachineV1Beta1MachineType))

		if template.OpenShiftMachineV1Beta1Machine == nil {
			// Note this is a rare exception to discriminated union rules for the naming of this field.
			// It matches the discriminator exactly.
			return []error{field.Required(openshiftMachineTemplatePath, fmt.Sprintf("%s is required when machine type is %s", machinev1.OpenShiftMachineV1Beta1MachineType, machinev1.OpenShiftMachineV1Beta1MachineType))}
		}

		return validateOpenShiftMachineV1BetaTemplateOnCreate(openshiftMachineTemplatePath, *template.OpenShiftMachineV1Beta1Machine, machines)
	default:
		return []error{field.NotSupported(parentPath.Child("machineType"), template.MachineType, []string{string(machinev1.OpenShiftMachineV1Beta1MachineType)})}
	}
}

// validateOpenShiftMachineV1BetaTemplate validates the OpenShift Machine API v1beta1 template.
func validateOpenShiftMachineV1BetaTemplate(parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, selector metav1.LabelSelector) []error {
	errs := []error{}

	errs = append(errs, validateTemplateLabels(parentPath.Child("metadata", "labels"), template.ObjectMeta.Labels, selector)...)
	errs = append(errs, validateOpenShiftFailureDomains(parentPath.Child("failureDomains"), template.FailureDomains)...)

	return errs
}

// validateOpenShiftMachineV1BetaTemplateOnCreate validates the failure domains in the provided template match up with those
// present in the Machines provided.
func validateOpenShiftMachineV1BetaTemplateOnCreate(parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, machines []machinev1beta1.Machine) []error {
	errs := []error{}

	errs = append(errs, checkOpenShiftFailureDomainsMatchMachines(parentPath.Child("failureDomains"), template.FailureDomains, machines)...)

	return errs
}

// validateTemplateLabels validates that the labels passed from the template match the expectations required.
// It checks the role and type labels and ensures that the cluster ID label is also present.
func validateTemplateLabels(labelsPath *field.Path, templateLabels map[string]string, labelSelector metav1.LabelSelector) []error {
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
		value, ok := templateLabels[required.label]
		if !ok || value == "" {
			errs = append(errs, field.Required(labelsPath, fmt.Sprintf("%s label is required", required.label)))
		}

		if required.value != "" && required.value != value {
			errs = append(errs, field.Invalid(labelsPath, value, fmt.Sprintf("%s label must have value: %s", required.label, required.value)))
		}
	}

	// Ensure labels are matched by the selector.
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec", "selector"), selector, fmt.Errorf("could not convert label selector to selector: %w", err).Error()))
	}

	if selector != nil && !selector.Matches(labels.Set(templateLabels)) {
		errs = append(errs, field.Invalid(labelsPath, templateLabels, "selector does not match template labels"))
	}

	return errs
}

// validateOpenShiftFailureDomains checks that the configuration of the failure domains is correct.
// In particular, it checks the discriminated unions are valid and that the platform type is one of the supported
// platform types.
func validateOpenShiftFailureDomains(parentPath *field.Path, failureDomains machinev1.FailureDomains) []error {
	errs := []error{}

	// Check that the union is a valid discriminated union.
	errs = append(errs, validateDiscriminatedUnion(parentPath, failureDomains, "Platform")...)

	// AWS failure domains have extra validation.
	if failureDomains.Platform == configv1.AWSPlatformType {
		errs = append(errs, validateOpenShiftAWSFailureDomains(parentPath.Child("aws"), failureDomains.AWS)...)
	}

	return errs
}

// validateOpenShiftAWSFailureDomains checks that the subnet discriminated union of each AWS failure domain is valid.
func validateOpenShiftAWSFailureDomains(parentPath *field.Path, failureDomains *[]machinev1.AWSFailureDomain) []error {
	errs := []error{}

	if failureDomains == nil {
		// The validation of the discriminanted union should pick up this error.
		// Don't duplicate the error here.
		return []error{}
	}

	for i, failureDomain := range *failureDomains {
		errs = append(errs, validateDiscriminatedUnion(parentPath.Index(i).Child("subnet"), failureDomain.Subnet, "Type")...)
	}

	return errs
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

// checkOpenShiftFailureDomainsMatchMachines ensures that failure domains of the Control Plane Machines match the
// failure domains defined on the OpenShift Machine template on the ControlPlaneMachineSet.
func checkOpenShiftFailureDomainsMatchMachines(parentPath *field.Path, failureDomains machinev1.FailureDomains, machines []machinev1beta1.Machine) []error {
	errs := []error{}

	if failureDomains.Platform == "" {
		return nil
	}

	machineFailureDomains, err := providerconfig.ExtractFailureDomainsFromMachines(machines)
	if err != nil {
		return append(errs, field.InternalError(parentPath.Child("platform"), fmt.Errorf("could not get failure domains from cluster machines on platform %s: %w", failureDomains.Platform, err)))
	}

	specifiedFailureDomains, err := failuredomain.NewFailureDomains(failureDomains)
	if err != nil {
		return append(errs, field.Invalid(parentPath, failureDomains, fmt.Sprintf("error getting failure domains from control plane machine set machine template: %v", err)))
	}

	// Failure domains used by control plane machines but not specified in the control plane machine set
	if missingFailureDomains := missingFailureDomains(machineFailureDomains, specifiedFailureDomains); len(missingFailureDomains) > 0 {
		errs = append(errs, field.Forbidden(parentPath, fmt.Sprintf("control plane machines are using unspecified failure domain(s) %s", missingFailureDomains)))
	}

	// Failure domains specified in the control plane machine set but not used by control plane machines
	if missingFailureDomains := missingFailureDomains(specifiedFailureDomains, machineFailureDomains); len(missingFailureDomains) > 0 {
		errs = append(errs, field.Forbidden(parentPath, fmt.Sprintf("no control plane machine is using specified failure domain(s) %s", missingFailureDomains)))
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

// validateDiscriminatedUnion checks that the discriminated union is valid.
// That is, the the discriminant, if set, matches with the field that is configured, and no extra fields
// are configured.
func validateDiscriminatedUnion(parentPath *field.Path, union interface{}, discriminant string) []error {
	errs := []error{}

	if union == nil {
		// The union is nil so nothing to check.
		return []error{}
	}

	unionValue := reflect.ValueOf(union)

	// If it's a pointer, dereference it before we continue.
	if unionValue.Kind() == reflect.Pointer {
		unionValue = unionValue.Elem()
	}

	discriminantStructField, ok := unionValue.Type().FieldByName(discriminant)
	if !ok {
		return []error{fmt.Errorf("%w: union does not contain a field %q", errInvalidDiscriminant, discriminant)}
	}

	discriminantJSONName := getJSONName(discriminantStructField)
	discriminantValue := unionValue.FieldByName(discriminant).String()

	// Check each field in the struct.
	// Ignore the discriminant.
	// Check only the field matching the discriminant value is non-nil.
	for i := 0; i < unionValue.NumField(); i++ {
		fieldValue := unionValue.Field(i)
		fieldType := unionValue.Type().Field(i)

		fieldJSONName := getJSONName(fieldType)

		if fieldType.Name == discriminant {
			continue
		}

		// TODO: Once the AWSResourceReference is updated, remove the second clause here.
		// The names _should_ match on the field name, not on the json name.
		if fieldType.Name == discriminantValue || fieldJSONName == discriminantValue {
			if fieldValue.IsNil() {
				errs = append(errs, field.Required(parentPath.Child(fieldJSONName), fmt.Sprintf("value required when %s is %q", discriminantJSONName, discriminantValue)))
			}

			continue
		}

		if !fieldValue.IsNil() {
			errs = append(errs, field.Forbidden(parentPath.Child(fieldJSONName), fmt.Sprintf("value not allowed when %s is %q", discriminantJSONName, discriminantValue)))
		}
	}

	return errs
}

// getJSONName gets the JSON name of the field from the struct tag.
// This is used so that errors show the name as the end user would use.
func getJSONName(structField reflect.StructField) string {
	return strings.Split(structField.Tag.Get("json"), ",")[0]
}
