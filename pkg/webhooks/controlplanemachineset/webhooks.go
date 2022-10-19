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
	errs = append(errs, validateSpec(field.NewPath("spec"), cpms)...)
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

	cpms, ok := newObj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return errObjNotCPMS
	}

	errs = append(errs, validateMetadata(field.NewPath("metadata"), cpms.ObjectMeta)...)
	errs = append(errs, validateSpec(field.NewPath("spec"), cpms)...)

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

	// If the CPMS is Active, ensure the Control Plane Machine count
	// matches the ControlPlaneMachineSet replicas.
	if cpms.Spec.State == machinev1.ControlPlaneMachineSetStateActive &&
		cpms.Spec.Replicas != nil &&
		int(*cpms.Spec.Replicas) != len(controlPlaneMachines) {
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
func validateSpec(parentPath *field.Path, cpms *machinev1.ControlPlaneMachineSet) []error {
	errs := []error{}

	errs = append(errs, validateTemplate(parentPath.Child("template"), cpms.Spec.Template, cpms.Spec.Selector)...)

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
	errs = append(errs, validateOpenShiftProviderConfig(parentPath, template)...)

	return errs
}

// validateOpenShiftMachineV1BetaTemplateOnCreate validates the failure domains in the provided template match up with those
// present in the Machines provided.
func validateOpenShiftMachineV1BetaTemplateOnCreate(parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, machines []machinev1beta1.Machine) []error {
	errs := []error{}

	if template.FailureDomains.Platform == "" {
		errs = append(errs, checkOpenShiftProviderSpecFailureDomainMatchesMachines(parentPath.Child("spec", "providerSpec"), template, machines)...)
	} else {
		errs = append(errs, checkOpenShiftFailureDomainsMatchMachines(parentPath.Child("failureDomains"), template.FailureDomains, machines)...)
	}

	return errs
}

// validateTemplateLabels validates that the labels passed from the template match the expectations required.
// It checks the role and type labels and ensures that the cluster ID label is also present.
func validateTemplateLabels(labelsPath *field.Path, templateLabels map[string]string, labelSelector metav1.LabelSelector) []error {
	errs := []error{}

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

// validateOpenShiftProviderConfig checks the provider config on the ControlPlaneMachineSet to ensure that the
// ControlPlaneMachineSet can safely replace control plane machines.
func validateOpenShiftProviderConfig(parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate) []error {
	providerSpecPath := parentPath.Child("spec", "providerSpec")

	providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(template)
	if err != nil {
		return []error{field.Invalid(providerSpecPath, template.Spec.ProviderSpec, fmt.Sprintf("error determining provider configuration: %s", err))}
	}

	switch providerConfig.Type() {
	case configv1.AzurePlatformType:
		return validateOpenShiftAzureProviderConfig(providerSpecPath.Child("value"), providerConfig.Azure())
	case configv1.GCPPlatformType:
		return validateOpenShiftGCPProviderConfig(providerSpecPath.Child("value"), providerConfig.GCP())
	}

	return []error{}
}

// validateOpenShiftAzureProviderConfig runs Azure specific checks on the provider config on the ControlPlaneMachineSet.
// This ensure that the ControlPlaneMachineSet can safely replace Azure control plane machines.
func validateOpenShiftAzureProviderConfig(parentPath *field.Path, providerConfig providerconfig.AzureProviderConfig) []error {
	errs := []error{}

	config := providerConfig.Config()

	if config.InternalLoadBalancer == "" {
		errs = append(errs, field.Required(parentPath.Child("internalLoadBalancer"), "internalLoadBalancer is required for control plane machines"))
	}

	return errs
}

// validateOpenShiftGCPProviderConfig runs GCP specific checks on the provider config on the ControlPlaneMachineSet.
// This ensure that the ControlPlaneMachineSet can safely replace GCP control plane machines.
func validateOpenShiftGCPProviderConfig(parentPath *field.Path, _ providerconfig.GCPProviderConfig) []error {
	// On GCP, we do not currently update the backend pools for the inernal load balancer.
	// Until this is supported in Machine API Provider GCP, we cannot safely replace control plane machines.
	return []error{field.Forbidden(parentPath, "automatic replacement of control plane machines on GCP is not currently supported")}
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

// checkOpenShiftProviderSpecFailureDomainMatchesMachines ensures that failure domains of the Control Plane Machines match the
// failure domain extracted from the OpenShift Machine template MachineSpec on the ControlPlaneMachineSet.
// This check is performed when no failure domains are defined on the OpenShift Machine template on the ControlPlaneMachineSet.
// For example on single zone AWS deployment.
func checkOpenShiftProviderSpecFailureDomainMatchesMachines(parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, machines []machinev1beta1.Machine) []error {
	errs := []error{}

	templateProviderConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(template)
	if err != nil {
		return []error{field.Invalid(parentPath, template, fmt.Sprintf("error parsing provider config from machine template: %v", err))}
	}

	templateProviderSpecFailureDomain := templateProviderConfig.ExtractFailureDomain()

	failureDomains, err := providerconfig.ExtractFailureDomainsFromMachines(machines)
	if err != nil {
		return append(errs, field.InternalError(parentPath, fmt.Errorf("could not get failure domains from cluster machines: %w", err)))
	}

	for _, failureDomain := range failureDomains {
		if !templateProviderSpecFailureDomain.Equal(failureDomain) {
			errs = append(errs, field.Invalid(parentPath, templateProviderSpecFailureDomain, "Failure domain extracted from machine template providerSpec does not match failure domain of all control plane machines"))
		}
	}

	return errs
}

// checkOpenShiftFailureDomainsMatchMachines ensures that failure domains of the Control Plane Machines match the
// failure domains defined on the OpenShift Machine template on the ControlPlaneMachineSet.
func checkOpenShiftFailureDomainsMatchMachines(parentPath *field.Path, failureDomains machinev1.FailureDomains, machines []machinev1beta1.Machine) []error {
	errs := []error{}

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
		if duplicatedFailureDomains := duplicatedFailureDomains(machineFailureDomains); len(duplicatedFailureDomains) > 0 {
			errs = append(errs, field.Forbidden(parentPath, fmt.Sprintf("no control plane machine is using specified failure domain(s) %s, failure domain(s) %s are duplicated within the control plane machines, please correct failure domains to match control plane machines", missingFailureDomains, duplicatedFailureDomains)))
		}
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

// duplicatedFailureDomains returns failure domains that are duplicated within the given list.
func duplicatedFailureDomains(in []failuredomain.FailureDomain) []failuredomain.FailureDomain {
	seen := []failuredomain.FailureDomain{}
	duplicated := []failuredomain.FailureDomain{}

	for _, failureDomain := range in {
		if !contains(seen, failureDomain) {
			seen = append(seen, failureDomain)
			continue
		}

		if !contains(duplicated, failureDomain) {
			duplicated = append(duplicated, failureDomain)
		}
	}

	return duplicated
}

// contains checks if a failure domain is already present within a list of failure domains.
func contains(in []failuredomain.FailureDomain, fd failuredomain.FailureDomain) bool {
	for _, failureDomain := range in {
		if fd.Equal(failureDomain) {
			return true
		}
	}

	return false
}
