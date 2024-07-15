/*
Copyright 2023 Red Hat, Inc.

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
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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

	vsphereTemplateValidationPattern = `^/.*?/vm/.*?`

	// warnVSphereTemplateMayBeIgnored is a warning when cpms has template configured when failure domains are in use.
	warnVSphereTemplateMayBeIgnored = "template field is configured and may be ignored if configured in the failure domain."

	// warnVSphereFolderMayBeIgnored is a warning when cpms has folder configured when failure domains are in use.
	warnVSphereFolderMayBeIgnored = "folder field is configured and may be ignored if configured in the failure domain."

	// warnVSphereResourcePoolMayBeIgnored is a warning when cpms has resource pool configured when failure domains are in use.
	warnVSphereResourcePoolMayBeIgnored = "resourcePool field is configured and may be ignored if configured in the failure domain."
)

var (
	// errObjNotCPMS is an error when casting to ControlPlaneMachineSet fails.
	errObjNotCPMS = errors.New("validated object is not of type control plane machine set")

	// errUpdateNilCPMS is an error when update is called with nil ControlPlaneMachineSet.
	errUpdateNilCPMS = errors.New("cannot update nil control plane machine set")

	// errVSphereNetworkNotAllowed is an error when cpms has network configured when failure domains are in use.
	errVSphereNetworkNotAllowed = errors.New("network devices should not be set when control plane nodes are in a failure domain")

	// errVSphereWorkspaceNotAllowed is an error when cpms has workspace configured when failure domains are in use.
	errVSphereWorkspaceNotAllowed = errors.New("workspace fields should not be set when control plane nodes are in a failure domain")
)

// ControlPlaneMachineSetWebhook acts as a webhook validator for the
// machinev1beta1.ControlPlaneMachineSet resource.
type ControlPlaneMachineSetWebhook struct {
	client client.Client
	logger logr.Logger
}

// SetupWebhookWithManager sets up a new ControlPlaneMachineSet webhook with the manager.
func (r *ControlPlaneMachineSetWebhook) SetupWebhookWithManager(mgr ctrl.Manager, logger logr.Logger) error {
	r.client = mgr.GetClient()
	r.logger = logger

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
func (r *ControlPlaneMachineSetWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var errs []error

	var warnings []string

	infrastructure, err := util.GetInfrastructure(ctx, r.client)
	if err != nil {
		return warnings, fmt.Errorf("error getting infrastructure resource: %w", err)
	}

	cpms, ok := obj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return warnings, errObjNotCPMS
	}

	errs = append(errs, validateMetadata(field.NewPath("metadata"), cpms.ObjectMeta)...)
	tmpWarnings, tmpErrs := r.validateSpec(ctx, field.NewPath("spec"), cpms, infrastructure)
	warnings = append(warnings, tmpWarnings...)
	errs = append(errs, tmpErrs...)
	errs = append(errs, r.validateSpecOnCreate(ctx, field.NewPath("spec"), cpms, infrastructure)...)

	if len(errs) > 0 {
		return warnings, utilerrors.NewAggregate(errs)
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var errs []error

	var warnings []string

	if oldObj == nil {
		return warnings, errUpdateNilCPMS
	}

	cpms, ok := newObj.(*machinev1.ControlPlaneMachineSet)
	if !ok {
		return warnings, errObjNotCPMS
	}

	infrastructure, err := util.GetInfrastructure(ctx, r.client)
	if err != nil {
		return warnings, fmt.Errorf("error getting infrastructure resource: %w", err)
	}

	errs = append(errs, validateMetadata(field.NewPath("metadata"), cpms.ObjectMeta)...)
	tmpWarnings, tmpErrs := r.validateSpec(ctx, field.NewPath("spec"), cpms, infrastructure)
	warnings = append(warnings, tmpWarnings...)
	errs = append(errs, tmpErrs...)

	if len(errs) > 0 {
		return warnings, utilerrors.NewAggregate(errs)
	}

	return warnings, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *ControlPlaneMachineSetWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateSpecOnCreate runs the create time validations on the ControlPlaneMachineSet spec.
func (r *ControlPlaneMachineSetWebhook) validateSpecOnCreate(ctx context.Context, parentPath *field.Path, cpms *machinev1.ControlPlaneMachineSet, infrastructure *configv1.Infrastructure) []error {
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

	errs = append(errs, validateTemplateOnCreate(r.logger, parentPath.Child("template"), cpms.Spec.Template, controlPlaneMachines, infrastructure)...)

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
func (r *ControlPlaneMachineSetWebhook) validateSpec(ctx context.Context, parentPath *field.Path, cpms *machinev1.ControlPlaneMachineSet, infrastructure *configv1.Infrastructure) (admission.Warnings, []error) {
	errs := []error{}
	warnings := admission.Warnings{}

	tmpWarnings, tmpErrs := r.validateTemplate(ctx, cpms.Namespace, parentPath.Child("template"), cpms.Spec.Template, cpms.Spec.Selector, infrastructure)
	warnings = append(warnings, tmpWarnings...)
	errs = append(errs, tmpErrs...)

	return warnings, errs
}

// validateTemplate validates the common (on create and update) checks for the ControlPlaneMachineSet template.
func (r *ControlPlaneMachineSetWebhook) validateTemplate(ctx context.Context, namespaceName string, parentPath *field.Path, template machinev1.ControlPlaneMachineSetTemplate, selector metav1.LabelSelector, infrastructure *configv1.Infrastructure) (admission.Warnings, []error) {
	switch template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		openshiftMachineTemplatePath := parentPath.Child(string(machinev1.OpenShiftMachineV1Beta1MachineType))

		if template.OpenShiftMachineV1Beta1Machine == nil {
			// Note this is a rare exception to discriminated union rules for the naming of this field.
			// It matches the discriminator exactly.
			return nil, []error{field.Required(openshiftMachineTemplatePath, fmt.Sprintf("%s is required when machine type is %s", machinev1.OpenShiftMachineV1Beta1MachineType, machinev1.OpenShiftMachineV1Beta1MachineType))}
		}

		return r.validateOpenShiftMachineV1BetaTemplate(ctx, namespaceName, openshiftMachineTemplatePath, *template.OpenShiftMachineV1Beta1Machine, selector, infrastructure)
	default:
		return nil, []error{field.NotSupported(parentPath.Child("machineType"), template.MachineType, []string{string(machinev1.OpenShiftMachineV1Beta1MachineType)})}
	}
}

// validateTemplateOnCreate validates the failure domains defined in the template match up with the Machines
// that already exist within the cluster. This check is only performed on create.
func validateTemplateOnCreate(logger logr.Logger, parentPath *field.Path, template machinev1.ControlPlaneMachineSetTemplate, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) []error {
	switch template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		openshiftMachineTemplatePath := parentPath.Child(string(machinev1.OpenShiftMachineV1Beta1MachineType))

		if template.OpenShiftMachineV1Beta1Machine == nil {
			// Note this is a rare exception to discriminated union rules for the naming of this field.
			// It matches the discriminator exactly.
			return []error{field.Required(openshiftMachineTemplatePath, fmt.Sprintf("%s is required when machine type is %s", machinev1.OpenShiftMachineV1Beta1MachineType, machinev1.OpenShiftMachineV1Beta1MachineType))}
		}

		return validateOpenShiftMachineV1BetaTemplateOnCreate(logger, openshiftMachineTemplatePath, *template.OpenShiftMachineV1Beta1Machine, machines, infrastructure)
	default:
		return []error{field.NotSupported(parentPath.Child("machineType"), template.MachineType, []string{string(machinev1.OpenShiftMachineV1Beta1MachineType)})}
	}
}

// validateOpenShiftMachineV1BetaTemplate validates the OpenShift Machine API v1beta1 template.
func (r *ControlPlaneMachineSetWebhook) validateOpenShiftMachineV1BetaTemplate(ctx context.Context, namespaceName string, parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, selector metav1.LabelSelector, infrastructure *configv1.Infrastructure) (admission.Warnings, []error) {
	errs := []error{}
	warnings := admission.Warnings{}

	errs = append(errs, validateTemplateLabels(parentPath.Child("metadata", "labels"), template.ObjectMeta.Labels, selector)...)
	tempWarns, tempErrs := validateOpenShiftProviderConfig(r.logger, parentPath, template, infrastructure)
	warnings = append(warnings, tempWarns...)
	errs = append(errs, tempErrs...)
	errs = append(errs, r.validateOpenShiftProviderMachineSpec(ctx, namespaceName, parentPath.Child("spec", "providerSpec"), template, infrastructure)...)

	return warnings, errs
}

func (r *ControlPlaneMachineSetWebhook) validateOpenShiftProviderMachineSpec(ctx context.Context, namespaceName string, parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, infrastructure *configv1.Infrastructure) []error {
	errs := []error{}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(r.logger, template, infrastructure)
	if err != nil {
		return []error{field.Invalid(parentPath, template.Spec.ProviderSpec.Value, fmt.Sprintf("error determining provider configuration: %s", err))}
	}

	templateProviderConfig := providerConfig

	if template.FailureDomains == nil || template.FailureDomains.Platform != "" {
		failureDomains, err := failuredomain.NewFailureDomains(template.FailureDomains)
		if err != nil {
			return []error{field.Invalid(parentPath, template.FailureDomains, fmt.Sprintf("error constructing failure domain config: %s", err))}
		}

		if len(failureDomains) > 0 {
			injectedProviderConfig, err := templateProviderConfig.InjectFailureDomain(failureDomains[0])
			if err != nil {
				return []error{field.Invalid(parentPath, template.FailureDomains, fmt.Sprintf("error injecting failure domain into provider config: %s", err))}
			}

			templateProviderConfig = injectedProviderConfig
		}
	}

	dryRunMachine := &machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "tmp-machine-dry-run",
			Namespace:    namespaceName,
			Annotations:  template.ObjectMeta.Annotations,
			Labels:       template.ObjectMeta.Labels,
		},
		Spec: template.Spec,
	}

	rawConfig, err := templateProviderConfig.RawConfig()
	if err != nil {
		return []error{field.Invalid(parentPath, template.Spec.ProviderSpec, fmt.Sprintf("could not fetch raw config from provider config: %s", err))}
	}

	dryRunMachine.Spec.ProviderSpec.Value.Raw = rawConfig

	dryRunClient := client.NewDryRunClient(r.client)
	if err := dryRunClient.Create(ctx, dryRunMachine); err != nil {
		errs = append(errs, fmt.Errorf("invalid machine spec: %w", err))
	}

	return errs
}

// validateOpenShiftMachineV1BetaTemplateOnCreate validates the failure domains in the provided template match up with those
// present in the Machines provided.
func validateOpenShiftMachineV1BetaTemplateOnCreate(logger logr.Logger, parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) []error {
	errs := []error{}

	if template.FailureDomains == nil || template.FailureDomains.Platform == "" {
		errs = append(errs, checkOpenShiftProviderSpecFailureDomainMatchesMachines(logger, parentPath.Child("spec", "providerSpec"), template, machines, infrastructure)...)
	} else {
		providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, template.Spec, infrastructure)
		if err != nil {
			errs = append(errs, field.Invalid(parentPath.Child("spec", "providerSpec"), template.Spec, fmt.Sprintf("could not parse provider spec: %v", err)))
		}

		templateFailureDomain := providerConfig.ExtractFailureDomain()

		errs = append(errs, checkOpenShiftFailureDomainsMatchMachines(logger, parentPath.Child("failureDomains"), templateFailureDomain, template.FailureDomains, machines, infrastructure)...)
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
func validateOpenShiftProviderConfig(logger logr.Logger, parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, infrastructure *configv1.Infrastructure) (admission.Warnings, []error) {
	providerSpecPath := parentPath.Child("spec", "providerSpec")

	providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(logger, template, infrastructure)
	if err != nil {
		return nil, []error{field.Invalid(providerSpecPath, template.Spec.ProviderSpec, fmt.Sprintf("error determining provider configuration: %s", err))}
	}

	switch providerConfig.Type() {
	case configv1.AzurePlatformType:
		return nil, validateOpenShiftAzureProviderConfig(providerSpecPath.Child("value"), providerConfig.Azure())
	case configv1.GCPPlatformType:
		return nil, validateOpenShiftGCPProviderConfig(providerSpecPath.Child("value"), providerConfig.GCP())
	case configv1.OpenStackPlatformType:
		return nil, validateOpenShiftOpenStackProviderConfig(providerSpecPath.Child("value"), providerConfig.OpenStack())
	case configv1.VSpherePlatformType:
		return validateOpenShiftVSphereProviderConfig(providerSpecPath.Child("value"), providerConfig.VSphere(), infrastructure)
	case configv1.NutanixPlatformType:
		return nil, validateOpenShiftNutanixProviderConfig(providerSpecPath.Child("value"), providerConfig.Nutanix(), template.FailureDomains)
	}

	return nil, []error{}
}

// validateOpenShiftNutanixProviderConfig runs Nutanix specific checks on the provider config on the ControlPlaneMachineSet.
// This ensure that the ControlPlaneMachineSet can safely replace Nutanix control plane machines.
func validateOpenShiftNutanixProviderConfig(parentPath *field.Path, providerConfig providerconfig.NutanixProviderConfig, failureDomains *machinev1.FailureDomains) []error {
	errs := []error{}

	if failureDomains == nil {
		return errs
	}

	for _, fdRef := range failureDomains.Nutanix {
		if _, err := providerConfig.GetFailureDomainByName(fdRef.Name); err != nil {
			errs = append(errs, field.Invalid(parentPath.Child("failureDomain", "name"), fdRef.Name, fmt.Sprintf("error: %v", err)))
		}
	}

	return errs
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
func validateOpenShiftGCPProviderConfig(parentPath *field.Path, providerConfig providerconfig.GCPProviderConfig) []error {
	return []error{}
}

// validateOpenShiftOpenStackProviderConfig runs OpenStack specific checks on the provider config on the ControlPlaneMachineSet.
// This ensure that the ControlPlaneMachineSet can safely replace OpenStack control plane machines.
func validateOpenShiftOpenStackProviderConfig(parentPath *field.Path, providerConfig providerconfig.OpenStackProviderConfig) []error {
	errs := []error{}

	config := providerConfig.Config()

	for _, additionalBlockDevice := range config.AdditionalBlockDevices {
		if additionalBlockDevice.Name == "etcd" && additionalBlockDevice.SizeGiB < 10 {
			errs = append(errs, field.Invalid(parentPath.Child("additionalBlockDevices"), additionalBlockDevice.SizeGiB, "etcd block device size must be at least 10 GiB"))
		}
	}

	return errs
}

// validateOpenShiftVSphereProviderConfig runs VSphere specific checks on the provider config on the ControlPlaneMachineSet.
// This ensures that the ControlPlaneMachineSet can safely replace VSphere control plane machines.
func validateOpenShiftVSphereProviderConfig(parentPath *field.Path, providerConfig providerconfig.VSphereProviderConfig, infrastructure *configv1.Infrastructure) (admission.Warnings, []error) {
	warnings := admission.Warnings{}
	errs := []error{}

	infraProviderConfig := infrastructure.Spec.PlatformSpec.VSphere
	if infraProviderConfig != nil && len(infraProviderConfig.FailureDomains) > 0 {
		// If failure domains are configured, we do not want to have user provide any FD overrides
		warn, err := checkForFailureDomainIgnoredFields(parentPath, providerConfig)
		warnings = append(warnings, warn...)
		errs = append(errs, err...)
	} else {
		errs = append(errs, validateTemplate(parentPath, providerConfig.Config().Template)...)
	}

	return warnings, errs
}

// checkForFailureDomainIgnoredFields checks all fields that are controlled by failure domain.  Each infraction will result
// in an error being returned in the error collection.
func checkForFailureDomainIgnoredFields(parentPath *field.Path, providerConfig providerconfig.VSphereProviderConfig) (admission.Warnings, []error) {
	warnings := admission.Warnings{}
	errs := []error{}

	// If failure domains are configured, we do not want to have user provide any FD overrides unless the FD does not define
	// the field.  In most cases, the fields are required.  We'll check each field to verify that FD has defined the value
	// in addition to user providing an override.

	// Template is not required.  We will generate a warning when detected to let user know this template may get ignored.
	if len(providerConfig.Config().Template) > 0 {
		warnings = append(warnings, fmt.Sprintf("%s: %s", parentPath.Child("template"), warnVSphereTemplateMayBeIgnored))
		errs = append(errs, validateTemplate(parentPath, providerConfig.Config().Template)...)
	}

	// Static IP configuration is the only thing we need to allow.  Networks are required to be defined in the FD.
	devices := providerConfig.Config().Network.Devices
	for _, device := range devices {
		if len(device.NetworkName) > 0 {
			errs = append(errs, field.InternalError(parentPath.Child("network"),
				fmt.Errorf("%w: %#v", errVSphereNetworkNotAllowed, providerConfig.Config().Network.Devices)))
		}
	}

	workspace := providerConfig.Config().Workspace
	if workspace != nil {
		workspacePath := parentPath.Child("workspace")

		// In workspace, the Datacenter, Datastore, Server are required to be set in the FD.
		if len(workspace.Datacenter) > 0 || len(workspace.Datastore) > 0 || len(workspace.Server) > 0 {
			errs = append(errs, field.InternalError(workspacePath,
				fmt.Errorf("%w: %#v", errVSphereWorkspaceNotAllowed, workspace)))
		}

		// Folder and ResourcePool are optional in the infrastructure's FDs.  We'll only generate a warning if they are
		// set in the workspace.
		if len(workspace.Folder) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: %s", workspacePath.Child("folder"), warnVSphereFolderMayBeIgnored))
		}

		if len(workspace.ResourcePool) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: %s", workspacePath.Child("resourcePool"), warnVSphereResourcePoolMayBeIgnored))
		}
	}

	return warnings, errs
}

// validateTemplate validates the format of the template.
func validateTemplate(parentPath *field.Path, template string) []error {
	errs := []error{}

	// Check template format.  If "/" found, assume it's a path based definition and verify with pattern.
	// If not a path template, then assume just a template name (older ocp installs)
	if len(template) > 0 && strings.Contains(template, "/") {
		matched, err := regexp.MatchString(vsphereTemplateValidationPattern, template)
		if err != nil {
			errs = append(errs, field.InternalError(parentPath.Child("template"), fmt.Errorf("error checking the validity of the template path: %w", err)))
		}

		if !matched {
			errs = append(errs, field.Invalid(parentPath.Child("template"), template, "template must be provided as the full path"))
		}
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

// checkOpenShiftProviderSpecFailureDomainMatchesMachines ensures that failure domains of the Control Plane Machines match the
// failure domain extracted from the OpenShift Machine template MachineSpec on the ControlPlaneMachineSet.
// This check is performed when no failure domains are defined on the OpenShift Machine template on the ControlPlaneMachineSet.
// For example on single zone AWS deployment.
func checkOpenShiftProviderSpecFailureDomainMatchesMachines(logger logr.Logger, parentPath *field.Path, template machinev1.OpenShiftMachineV1Beta1MachineTemplate, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) []error {
	errs := []error{}

	templateProviderConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(logger, template, infrastructure)
	if err != nil {
		return []error{field.Invalid(parentPath, template, fmt.Sprintf("error parsing provider config from machine template: %v", err))}
	}

	templateProviderSpecFailureDomain := templateProviderConfig.ExtractFailureDomain()

	failureDomains, err := providerconfig.ExtractFailureDomainsFromMachines(logger, machines, infrastructure)
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
func checkOpenShiftFailureDomainsMatchMachines(logger logr.Logger, parentPath *field.Path, templateFailureDomain failuredomain.FailureDomain, failureDomains *machinev1.FailureDomains, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) []error {
	errs := []error{}

	machineFailureDomains, err := getMachineFailureDomains(logger, machines, infrastructure)
	if err != nil {
		return append(errs, field.InternalError(parentPath.Child("platform"), fmt.Errorf("could not get failure domains from cluster machines on platform %s: %w", failureDomains.Platform, err)))
	}

	specifiedFailureDomains, err := failuredomain.NewFailureDomains(failureDomains)
	if err != nil {
		return append(errs, field.Invalid(parentPath, failureDomains, fmt.Sprintf("error getting failure domains from control plane machine set machine template: %v", err)))
	}

	comparableSpecifiedFailureDomains, err := failuredomain.CompleteFailureDomains(specifiedFailureDomains, templateFailureDomain)
	if err != nil {
		return append(errs, field.InternalError(parentPath.Child("platform"), fmt.Errorf("could not make failure domains comparable: %w", err)))
	}

	// Failure domains used by control plane machines but not specified in the control plane machine set
	if missingFailureDomains := missingFailureDomains(machineFailureDomains, comparableSpecifiedFailureDomains); len(missingFailureDomains) > 0 {
		errs = append(errs, field.Forbidden(parentPath, fmt.Sprintf("control plane machines are using unspecified failure domain(s) %s", missingFailureDomains)))
	}

	// Failure domains specified in the control plane machine set but not used by control plane machines
	if missingFailureDomains := missingFailureDomains(comparableSpecifiedFailureDomains, machineFailureDomains); len(missingFailureDomains) > 0 {
		if duplicatedFailureDomains := duplicatedFailureDomains(machineFailureDomains); len(duplicatedFailureDomains) > 0 {
			errs = append(errs, field.Forbidden(parentPath, fmt.Sprintf("no control plane machine is using specified failure domain(s) %s, failure domain(s) %s are duplicated within the control plane machines, please correct failure domains to match control plane machines", missingFailureDomains, duplicatedFailureDomains)))
		}
	}

	return errs
}

// getMachineFailureDomains returns a list of failure domains used by the control plane machines.
// We use this instead of providerconfig.ExtractFailureDomainsFromMachines because we want to
// keep all machines and the providerconfig util deduplicates the failure domains.
func getMachineFailureDomains(logger logr.Logger, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) ([]failuredomain.FailureDomain, error) {
	machineFailureDomains := []failuredomain.FailureDomain{}

	for _, machine := range machines {
		failureDomain, err := providerconfig.ExtractFailureDomainFromMachine(logger, machine, infrastructure)
		if err != nil {
			return nil, fmt.Errorf("could not extract failure domain from machine: %w", err)
		}

		machineFailureDomains = append(machineFailureDomains, failureDomain)
	}

	return machineFailureDomains, nil
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
