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

package v1beta1

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/pointer"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// runningPhase defines the phase when the machine operates correctly.
	runningPhase = "Running"

	// deletingPhase defines the phase when the machine is being deleted.
	deletingPhase = "Deleting"

	// openshiftMachineRoleLabel is the OpenShift Machine API machine role label.
	// This must be present on all OpenShift Machine API Machine templates.
	openshiftMachineRoleLabel = "machine.openshift.io/cluster-api-machine-role"
)

var (
	// errCouldNotDetermineMachineIndex is used to denote that the MachineProvider could not infer an
	// index to assign to a Machine based on either the name or the failure domain.
	// This means the Machine has been created in some manor outside of OpenShift norms and is in a failure domain
	// not currently specified in the ControlPlaneMachineSet definition. User intervention is required here.
	errCouldNotDetermineMachineIndex = errors.New("could not determine Machine index from name or failure domain")

	// errCouldNotFindFailureDomain is used to denote that the MachineProvider could not find a failure domain
	// within the mapping for a given index.
	// This means the Machine has an index in its name that is outside the bounds of the current mapping.
	// This may happen if the cluster is trying to horizontally scale down.
	errCouldNotFindFailureDomain = errors.New("could not find failure domain for index")

	// errEmptyConfig is used to denote that the machine provider could not be constructed
	// because no configuration was provided by the user.
	errEmptyConfig = fmt.Errorf("cannot initialise %s provider with empty config", machinev1.OpenShiftMachineV1Beta1MachineType)

	// errMissingClusterIDLabel is used to denote that the cluster ID label, expected to be on the Machine template
	// is not present and therefore a Machine cannot be created. The Cluster ID is required to construct the name for
	// the new Machines.
	errMissingClusterIDLabel = fmt.Errorf("missing required label on machine template metadata: %s", machinev1beta1.MachineClusterIDLabel)

	// errMissingMachineRoleLabel is used to denote that the machine role label, expected to be on the Machine template
	// is not present and therefore a Machine cannot be created. The Machine Role is required to construct the name for
	// the new Machines.
	errMissingMachineRoleLabel = fmt.Errorf("missing required label on machine template metadata: %s", openshiftMachineRoleLabel)

	// errUnexpectedMachineType is used to denote that the machine provider was requested
	// for an unsupported machine provider type (ie not OpenShift Machine v1beta1).
	errUnexpectedMachineType = fmt.Errorf("unexpected machine type while initialising %s provider", machinev1.OpenShiftMachineV1Beta1MachineType)

	// errUnknownGroupVersionResource is used to denote that the machine provider received an
	// unknown GroupVersionResource while processing a Machine deletion request.
	errUnknownGroupVersionResource = fmt.Errorf("unknown group/version/resource")
)

// NewMachineProvider creates a new OpenShift Machine v1beta1 machine provider implementation.
func NewMachineProvider(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet) (machineproviders.MachineProvider, error) {
	if cpms.Spec.Template.MachineType != machinev1.OpenShiftMachineV1Beta1MachineType {
		return nil, fmt.Errorf("%w: %s", errUnexpectedMachineType, cpms.Spec.Template.MachineType)
	}

	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil {
		return nil, errEmptyConfig
	}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(*cpms.Spec.Template.OpenShiftMachineV1Beta1Machine)
	if err != nil {
		return nil, fmt.Errorf("error constructing provider config: %w", err)
	}

	failureDomains, err := failuredomain.NewFailureDomains(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains)
	if err != nil {
		return nil, fmt.Errorf("error constructing failure domain config: %w", err)
	}

	indexToFailureDomain, err := mapMachineIndexesToFailureDomains(ctx, logger, cl, cpms, failureDomains)
	if err != nil && !errors.Is(err, errNoFailureDomains) {
		return nil, fmt.Errorf("error mapping machine indexes: %w", err)
	}

	machineAPIScheme := apimachineryruntime.NewScheme()
	if err := machinev1.Install(machineAPIScheme); err != nil {
		return nil, fmt.Errorf("unable to add machine.openshift.io/v1 scheme: %w", err)
	}

	if err := machinev1beta1.Install(machineAPIScheme); err != nil {
		return nil, fmt.Errorf("unable to add machine.openshift.io/v1beta1 scheme: %w", err)
	}

	return &openshiftMachineProvider{
		client:               cl,
		indexToFailureDomain: indexToFailureDomain,
		machineSelector:      cpms.Spec.Selector,
		machineTemplate:      *cpms.Spec.Template.OpenShiftMachineV1Beta1Machine,
		ownerMetadata:        cpms.ObjectMeta,
		providerConfig:       providerConfig,
		namespace:            cpms.Namespace,
		machineAPIScheme:     machineAPIScheme,
	}, nil
}

// openshiftMachineProvider holds the implementation of the MachineProvider interface.
type openshiftMachineProvider struct {
	// client is used to make API calls to fetch Machines and Nodes.
	client client.Client

	// indexToFailureDomain creates a mapping of failure domains to an arbitrary index.
	// This index is then used in MachineInfo to allow external code to request that a
	// new Machine be created in the same failure domain as an existing Machine.
	// We use a built in type to avoid leaking implementation specific details.
	indexToFailureDomain map[int32]failuredomain.FailureDomain

	// machineSelector is used to identify which Machines should be considered by
	// the machine provider when constructing machine information.
	machineSelector metav1.LabelSelector

	// machineTemplate is used to create new Machines.
	machineTemplate machinev1.OpenShiftMachineV1Beta1MachineTemplate

	// ownerMetadata is used to allow newly created Machines to have an owner
	// reference set upon creation.
	ownerMetadata metav1.ObjectMeta

	// providerConfig stores the providerConfig for creating new Machines.
	providerConfig providerconfig.ProviderConfig

	// namespace store the namespace where new machines will be created.
	namespace string

	// machineAPIScheme contains scheme for Machine API v1 and v1beta1.
	machineAPIScheme *apimachineryruntime.Scheme
}

// GetMachineInfos inspects the current state of the Machines matched by the selector
// and returns information about the Machines in the form of a MachineInfo.
// For each Machine, it identifies the following:
// - Does the Machine have a Node associated with it
// - Is the Machine ready?
// - Does the Machine need an update?
// - Which failure domain index does the Machine represent?
// - Is the Machine in an error state?
func (m *openshiftMachineProvider) GetMachineInfos(ctx context.Context, logger logr.Logger) ([]machineproviders.MachineInfo, error) {
	machineInfos := []machineproviders.MachineInfo{}

	selector, err := metav1.LabelSelectorAsSelector(&m.machineSelector)
	if err != nil {
		return nil, fmt.Errorf("could not convert label selector to selector: %w", err)
	}

	machineList := &machinev1beta1.MachineList{}
	if err := m.client.List(ctx, machineList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}

	for _, machine := range machineList.Items {
		machineInfo, err := m.generateMachineInfo(logger, machine)
		if err != nil {
			return nil, fmt.Errorf("could not generate machine info for machine %s: %w", machine.Name, err)
		}

		machineInfos = append(machineInfos, machineInfo)
	}

	// Print machine infos into logs
	for i, machineInfo := range machineInfos {
		nodeName := ""
		if machineInfo.NodeRef != nil {
			nodeName = machineInfo.NodeRef.ObjectMeta.Name
		}

		logger.V(4).Info(
			"Gathered Machine Info",
			"machineName", machineList.Items[i].Name,
			"nodeName", nodeName,
			"index", machineInfo.Index,
			"ready", machineInfo.Ready,
			"needsUpdate", machineInfo.NeedsUpdate,
			"errorMessage", machineInfo.ErrorMessage,
		)
	}

	return machineInfos, nil
}

// generateMachineInfo creates a MachineInfo object for a given machine.
func (m *openshiftMachineProvider) generateMachineInfo(logger logr.Logger, machine machinev1beta1.Machine) (machineproviders.MachineInfo, error) {
	machineRef := getMachineRef(machine)
	nodeRef := getNodeRef(machine)

	machineIndex, err := m.getMachineIndex(logger, machine)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("could not determine machine index: %w", err)
	}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(machine.Spec)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("could not compare existing and desired provider configs: %w", err)
	}

	templateProviderConfig := m.providerConfig

	if len(m.indexToFailureDomain) > 0 {
		// Make sure to compare using the desired failure domain from the mapping.
		mappedFailureDomain, ok := m.indexToFailureDomain[machineIndex]
		if !ok {
			logger.Error(fmt.Errorf("%w: unknown index %d", errCouldNotFindFailureDomain, machineIndex), "Unknown Index")
		} else {
			injectedProviderConfig, err := m.providerConfig.InjectFailureDomain(mappedFailureDomain)
			if err != nil {
				return machineproviders.MachineInfo{}, fmt.Errorf("error injecting failure domain into provider config: %w", err)
			}

			templateProviderConfig = injectedProviderConfig
		}
	}

	configsEqual, err := templateProviderConfig.Equal(providerConfig)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("cannot compare provider configs: %w", err)
	}

	ready := m.isMachineReady(machine)

	return machineproviders.MachineInfo{
		MachineRef:   machineRef,
		NodeRef:      nodeRef,
		Ready:        ready,
		NeedsUpdate:  !configsEqual,
		Index:        machineIndex,
		ErrorMessage: pointer.StringDeref(machine.Status.ErrorMessage, ""),
	}, nil
}

func (m *openshiftMachineProvider) getMachineIndex(logger logr.Logger, machine machinev1beta1.Machine) (int32, error) {
	machineNameIndex, correctFormat := getMachineNameIndex(machine)
	if correctFormat {
		// If the machine name has the correct format we implicitly trust it to be correct.
		return machineNameIndex, nil
	}

	// If the Machine name format doesn't match, try to fallback to matching based on the failure domain
	// with the Machine's provider spec.
	failureDomain, err := providerconfig.ExtractFailureDomainFromMachine(machine)
	if err != nil {
		return 0, fmt.Errorf("cannot extract failure domain from machine: %w", err)
	}

	index, indexFound := m.failureDomainToIndex(failureDomain)
	if !indexFound {
		// When the machine names do not fit the pattern, and the failure domains are
		// not recognised, returns an error.
		// Example:
		//   Machine "master-a" has a name which format doesn't allow to determine its name index.
		//   Additionally, machine's failure domain "domain-z" is not present in the domain index
		//   mapping, and we can't get its index either. In this case we don't know how to map the
		//   machine and user intervention is required.
		logger.Error(errCouldNotDetermineMachineIndex,
			"Could not gather Machine Info",
		)

		return 0, errCouldNotDetermineMachineIndex
	}

	return index, nil
}

// failureDomainToIndex returns the index of failure domain in the list. If there is nothing found, it returns false as
// a second parameter.
func (m *openshiftMachineProvider) failureDomainToIndex(failureDomain failuredomain.FailureDomain) (int32, bool) {
	for i, fd := range m.indexToFailureDomain {
		if fd.Equal(failureDomain) {
			return i, true
		}
	}

	return 0, false
}

// isMachineReady determines whether a machine is ready or not.
func (m *openshiftMachineProvider) isMachineReady(machine machinev1beta1.Machine) bool {
	if pointer.StringDeref(machine.Status.Phase, "") == runningPhase {
		// The machine is running so everything is working as expected.
		return true
	}

	if pointer.StringDeref(machine.Status.Phase, "") == deletingPhase && machine.Status.NodeRef != nil {
		// The machine was previously running but is now being deleted.
		// The machine is still ready until the node is drained and removed from the cluster.
		return true
	}

	return false
}

// getMachineNameIndex tries to fetch machine index from its name. If it's not possible,
// it returns false as a second parameter.
func getMachineNameIndex(machine machinev1beta1.Machine) (int32, bool) {
	// Get a substring after the last occurrence of "-" to find the machine index suffix.
	// Then try to convert it to a number.
	machineNameIndex, err := strconv.ParseInt(machine.Name[strings.LastIndex(machine.Name, "-")+1:], 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(machineNameIndex), true
}

// getMachineRef returns returns machine object reference for the given machine.
func getMachineRef(machine machinev1beta1.Machine) *machineproviders.ObjectRef {
	return &machineproviders.ObjectRef{
		ObjectMeta:           machine.ObjectMeta,
		GroupVersionResource: machinev1beta1.GroupVersion.WithResource("machines"),
	}
}

// getNodeRef returns node object reference for the given machine.
func getNodeRef(machine machinev1beta1.Machine) *machineproviders.ObjectRef {
	if machine.Status.NodeRef == nil {
		return nil
	}

	return &machineproviders.ObjectRef{
		ObjectMeta: metav1.ObjectMeta{
			Name: machine.Status.NodeRef.Name,
		},
		GroupVersionResource: corev1.SchemeGroupVersion.WithResource("nodes"),
	}
}

// CreateMachine creates a new Machine from the template provider config based on the
// failure domain index provided.
func (m *openshiftMachineProvider) CreateMachine(ctx context.Context, logger logr.Logger, index int32) error {
	machineName, err := m.getMachineName(index)
	if err != nil {
		return fmt.Errorf("could not generate machine name: %w", err)
	}

	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: m.ownerMetadata,
	}

	machine := &machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        machineName,
			Namespace:   m.namespace,
			Annotations: m.machineTemplate.ObjectMeta.Annotations,
			Labels:      m.machineTemplate.ObjectMeta.Labels,
		},
		Spec: m.machineTemplate.Spec,
	}

	providerConfig, err := m.getProviderConfigForIndex(index)
	if err != nil {
		return fmt.Errorf("could not get provider config for index %d: %w", index, err)
	}

	rawConfig, err := providerConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("cannot fetch raw config from provider config: %w", err)
	}

	machine.Spec.ProviderSpec.Value.Raw = rawConfig

	if err := controllerutil.SetControllerReference(cpms, machine, m.machineAPIScheme); err != nil {
		return fmt.Errorf("could not set owner reference: %w", err)
	}

	if err := m.client.Create(ctx, machine); err != nil {
		logger.Error(err,
			"Could not create machine",
			"namespace", machine.ObjectMeta.Namespace,
			"machineName", machine.ObjectMeta.Name,
			"group", machinev1beta1.GroupVersion.Group,
			"version", machinev1beta1.GroupVersion.Version,
		)

		return fmt.Errorf("cannot create machine %s in namespace %s: %w", machine.Name, machine.Namespace, err)
	}

	logger.V(2).Info(
		"Created machine",
		"index", index,
		"machineName", machine.Name,
		"failureDomain", providerConfig.ExtractFailureDomain().String(),
	)

	return nil
}

// getMachineName generates a machine name based on the index.
func (m *openshiftMachineProvider) getMachineName(index int32) (string, error) {
	clusterID, ok := m.machineTemplate.ObjectMeta.Labels[machinev1beta1.MachineClusterIDLabel]
	if !ok {
		return "", errMissingClusterIDLabel
	}

	machineRole, ok := m.machineTemplate.ObjectMeta.Labels[openshiftMachineRoleLabel]
	if !ok {
		return "", errMissingMachineRoleLabel
	}

	return fmt.Sprintf("%s-%s-%s-%d", clusterID, machineRole, rand.String(5), index), nil
}

// getProviderConfigForIndex returns the appropriate provider configuration for the index based on the failure domain
// mapping in the machine provider.
// If no failure domains are present it returns the base provider configuration.
func (m *openshiftMachineProvider) getProviderConfigForIndex(index int32) (providerconfig.ProviderConfig, error) {
	if len(m.indexToFailureDomain) == 0 {
		return m.providerConfig, nil
	}

	providerConfig, err := m.providerConfig.InjectFailureDomain(m.indexToFailureDomain[index])
	if err != nil {
		return nil, fmt.Errorf("cannot inject failure domain in the provider config: %w", err)
	}

	return providerConfig, nil
}

// DeleteMachine deletes the Machine references in the machineRef provided.
func (m *openshiftMachineProvider) DeleteMachine(ctx context.Context, logger logr.Logger, machineRef *machineproviders.ObjectRef) error {
	machinesGVR := machinev1beta1.GroupVersion.WithResource("machines")

	if machineRef.GroupVersionResource != machinesGVR {
		logger.Error(errUnknownGroupVersionResource,
			"Could not delete machine",
			"expectedGVR", machinesGVR.String(),
			"gotGVR", machineRef.GroupVersionResource.String(),
		)

		return fmt.Errorf("%w: expected %s, got %s", errUnknownGroupVersionResource, machinesGVR.String(), machineRef.GroupVersionResource.String())
	}

	machine := machinev1beta1.Machine{
		ObjectMeta: machineRef.ObjectMeta,
	}

	if err := m.client.Delete(ctx, &machine); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(2).Info(
				"Machine not found",
				"namespace", machineRef.ObjectMeta.Namespace,
				"machineName", machineRef.ObjectMeta.Name,
				"group", machinev1beta1.GroupVersion.Group,
				"version", machinev1beta1.GroupVersion.Version,
			)

			return nil
		}

		logger.Error(err,
			"Could not delete machine",
			"namespace", machineRef.ObjectMeta.Namespace,
			"machineName", machineRef.ObjectMeta.Name,
			"group", machinev1beta1.GroupVersion.Group,
			"version", machinev1beta1.GroupVersion.Version,
		)

		return fmt.Errorf("could not delete machine %s in namespace %s: %w", machineRef.ObjectMeta.Name, machineRef.ObjectMeta.Namespace, err)
	}

	logger.V(2).Info(
		"Deleted machine",
		"namespace", machineRef.ObjectMeta.Namespace,
		"machineName", machineRef.ObjectMeta.Name,
		"group", machinev1beta1.GroupVersion.Group,
		"version", machinev1beta1.GroupVersion.Version,
	)

	return nil
}
