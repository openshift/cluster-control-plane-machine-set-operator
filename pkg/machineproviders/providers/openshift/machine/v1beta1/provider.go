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
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apimachineryruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"

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

	// unreadyNodeGracePeriod is the period in which
	// the node is presumed being unready for a reboot (e.g. during upgrades) or a brief hiccup.
	// This leaves some leeway to avoid immediately reacting on brief node unreadiness.
	unreadyNodeGracePeriod = time.Minute * 20
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
func NewMachineProvider(ctx context.Context, logger logr.Logger, cl client.Client, recorder record.EventRecorder, cpms *machinev1.ControlPlaneMachineSet, opts OpenshiftMachineProviderOptions) (machineproviders.MachineProvider, error) {
	if cpms.Spec.Template.MachineType != machinev1.OpenShiftMachineV1Beta1MachineType {
		return nil, fmt.Errorf("%w: %s", errUnexpectedMachineType, cpms.Spec.Template.MachineType)
	}

	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine == nil {
		return nil, errEmptyConfig
	}

	infrastructure, err := util.GetInfrastructure(ctx, cl)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve infrastructure resource: %w", err)
	}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(logger, *cpms.Spec.Template.OpenShiftMachineV1Beta1Machine, infrastructure)
	if err != nil {
		return nil, fmt.Errorf("error building a provider config: %w", err)
	}

	failureDomains, err := failuredomain.NewFailureDomains(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains)
	if err != nil {
		return nil, fmt.Errorf("error constructing failure domain config: %w", err)
	}

	replicas := ptr.Deref(cpms.Spec.Replicas, 0)

	selector, err := metav1.LabelSelectorAsSelector(&cpms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("could not convert label selector to selector: %w", err)
	}

	machineAPIScheme := apimachineryruntime.NewScheme()
	if err := machinev1.Install(machineAPIScheme); err != nil {
		return nil, fmt.Errorf("unable to add machine.openshift.io/v1 scheme: %w", err)
	}

	if err := machinev1beta1.Install(machineAPIScheme); err != nil {
		return nil, fmt.Errorf("unable to add machine.openshift.io/v1beta1 scheme: %w", err)
	}

	o := &openshiftMachineProvider{
		client:                 cl,
		failureDomains:         failureDomains,
		machineSelector:        selector,
		machineTemplate:        *cpms.Spec.Template.OpenShiftMachineV1Beta1Machine,
		ownerMetadata:          cpms.ObjectMeta,
		providerConfig:         providerConfig,
		replicas:               replicas,
		machineNamePrefix:      cpms.Spec.MachineNamePrefix,
		allowMachineNamePrefix: opts.AllowMachineNamePrefix,
		namespace:              cpms.Namespace,
		machineAPIScheme:       machineAPIScheme,
		recorder:               recorder,
		infrastructure:         infrastructure,
	}

	if err := o.updateMachineCache(ctx, logger); err != nil {
		return nil, fmt.Errorf("error updating machine cache: %w", err)
	}

	return o, nil
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

	// failureDomains is the list of failure domains collected from the CPMS spec when
	// the machine provider was constructed.
	failureDomains []failuredomain.FailureDomain

	// machines is the list of machines collected from the API when the cache was built.
	// This should be tied to the lifecycle of indexToFailureDomain since the domain mapping
	// logic is affected by the contents of these machines.
	// Should the machine list ever be refreshed (e.g. by changing client), then the mapping
	// must also be refreshed.
	machines []machinev1beta1.Machine

	// machineSelector is used to identify which Machines should be considered by
	// the machine provider when constructing machine information.
	machineSelector labels.Selector

	// machineTemplate is used to create new Machines.
	machineTemplate machinev1.OpenShiftMachineV1Beta1MachineTemplate

	// ownerMetadata is used to allow newly created Machines to have an owner
	// reference set upon creation.
	ownerMetadata metav1.ObjectMeta

	// providerConfig stores the providerConfig for creating new Machines.
	providerConfig providerconfig.ProviderConfig

	replicas int32

	// machineNamePrefix is the prefix used when creating machine names
	// if `allowMachineNamePrefix` is true. When used, the machine name
	// will consist of this prefix, followed by a randomly generated string
	// of 5 characters, and the index of the machine.
	machineNamePrefix string

	// allowMachineNamePrefix determines whether to allow
	// the machine name to be prefixed with `machineNamePrefix`.
	allowMachineNamePrefix bool

	// namespace store the namespace where new machines will be created.
	namespace string

	// machineAPIScheme contains scheme for Machine API v1 and v1beta1.
	machineAPIScheme *apimachineryruntime.Scheme

	// recorder is used to emit events.
	recorder record.EventRecorder

	// infrastructure contains failure domain information for some platforms.
	infrastructure *v1.Infrastructure
}

// OpenshiftMachineProviderOptions holds configuration options for the OpenShift machine provider.
type OpenshiftMachineProviderOptions struct {
	// AllowMachineNamePrefix option is set when the CPMSMachineNamePrefix
	// feature gate is enabled.
	AllowMachineNamePrefix bool
}

// updateMachineCache fetches the current list of Machines and calculates from these the appropriate index
// to failure domain mapping. The machine list and index to failure domain must be updated in lock-step since
// the mapping relies on the content of the machines.
func (m *openshiftMachineProvider) updateMachineCache(ctx context.Context, logger logr.Logger) error {
	machineList := &machinev1beta1.MachineList{}
	if err := m.client.List(ctx, machineList, &client.ListOptions{LabelSelector: m.machineSelector}); err != nil {
		return fmt.Errorf("failed to list machines: %w", err)
	}

	config, err := providerconfig.NewProviderConfigFromMachineSpec(logger, m.machineTemplate.Spec, m.infrastructure)
	if err != nil {
		return fmt.Errorf("error building provider config from machine template: %w", err)
	}

	templateFailureDomain := config.ExtractFailureDomain()

	// Since the mapping depends on the state of the machines, we must re-map the failure domains if the machines have changed.
	indexToFailureDomain, err := mapMachineIndexesToFailureDomains(logger, machineList.Items, m.replicas, m.failureDomains, templateFailureDomain, m.infrastructure)
	if err != nil && !errors.Is(err, errNoFailureDomains) {
		return fmt.Errorf("error mapping machine indexes: %w", err)
	}

	m.machines = machineList.Items
	m.indexToFailureDomain = indexToFailureDomain

	return nil
}

// WithClient sets the desired client to the Machine Provider.
func (m *openshiftMachineProvider) WithClient(ctx context.Context, logger logr.Logger, cl client.Client) (machineproviders.MachineProvider, error) {
	// Take a shallow copy, this should be sufficient for the usage of the provider.
	o := &openshiftMachineProvider{}
	*o = *m

	o.client = cl

	// Make sure to update the cached machine data now that we have a new client.
	if err := o.updateMachineCache(ctx, logger); err != nil {
		return nil, fmt.Errorf("error updating machine cache: %w", err)
	}

	return o, nil
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

	for _, machine := range m.machines {
		machineInfo, err := m.generateMachineInfo(ctx, logger, machine)
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
			"machineName", m.machines[i].Name,
			"nodeName", nodeName,
			"index", machineInfo.Index,
			"ready", machineInfo.Ready,
			"needsUpdate", machineInfo.NeedsUpdate,
			"diff", machineInfo.Diff,
			"errorMessage", machineInfo.ErrorMessage,
		)
	}

	return machineInfos, nil
}

// generateMachineInfo creates a MachineInfo object for a given machine.
func (m *openshiftMachineProvider) generateMachineInfo(ctx context.Context, logger logr.Logger, machine machinev1beta1.Machine) (machineproviders.MachineInfo, error) {
	machineRef := getMachineRef(machine)
	nodeRef := getNodeRef(machine)

	machineIndex, err := m.getMachineIndex(logger, machine)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("could not determine machine index: %w", err)
	}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machine.Spec, m.infrastructure)
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

	validProviderConfig, err := m.ensureValidProviderConfig(ctx, logger, templateProviderConfig)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("cannot ensure that the provider config is valid: %w", err)
	}

	diff, err := validProviderConfig.Diff(providerConfig)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("cannot compare provider configs: %w", err)
	}

	configsEqual := len(diff) == 0

	ready, err := m.isMachineReady(ctx, machine)
	if err != nil {
		return machineproviders.MachineInfo{}, fmt.Errorf("error checking machine readiness: %w", err)
	}

	return machineproviders.MachineInfo{
		MachineRef:   machineRef,
		NodeRef:      nodeRef,
		Ready:        ready,
		NeedsUpdate:  !configsEqual,
		Diff:         diff,
		Index:        machineIndex,
		ErrorMessage: ptr.Deref(machine.Status.ErrorMessage, ""),
	}, nil
}

// ensureValidProviderConfig makes sure that the provider config is valid by dry-run creating a machine.
func (m *openshiftMachineProvider) ensureValidProviderConfig(ctx context.Context, logger logr.Logger, providerConfig providerconfig.ProviderConfig) (providerconfig.ProviderConfig, error) {
	dryRunMachine := &machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "tmp-machine-dry-run",
			Namespace:    m.namespace,
			Annotations:  m.ownerMetadata.Annotations,
			Labels:       m.ownerMetadata.Labels,
		},
		Spec: m.machineTemplate.Spec,
	}

	rawConfig, err := providerConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("could not fetch raw config from provider config: %w", err)
	}

	dryRunMachine.Spec.ProviderSpec.Value.Raw = rawConfig

	dryRunClient := client.NewDryRunClient(m.client)
	if err := dryRunClient.Create(ctx, dryRunMachine); err != nil {
		return nil, fmt.Errorf("could not dry-run create the machine: %w", err)
	}

	machineProviderConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, dryRunMachine.Spec, nil)
	if err != nil {
		return nil, fmt.Errorf("could not get provider config for machine: %w", err)
	}

	return machineProviderConfig, nil
}

func (m *openshiftMachineProvider) getMachineIndex(logger logr.Logger, machine machinev1beta1.Machine) (int32, error) {
	machineNameIndex, correctFormat := getMachineNameIndex(machine)
	if correctFormat {
		// If the machine name has the correct format we implicitly trust it to be correct.
		return machineNameIndex, nil
	}

	// If the Machine name format doesn't match, try to fallback to matching based on the failure domain
	// with the Machine's provider spec.
	failureDomain, err := providerconfig.ExtractFailureDomainFromMachine(logger, machine, m.infrastructure)
	if err != nil {
		return 0, fmt.Errorf("cannot extract failure domain from machine: %w", err)
	}

	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, m.machineTemplate.Spec, m.infrastructure)
	if err != nil {
		return 0, fmt.Errorf("cannot get provider config for machine: %w", err)
	}
	// templateFailureDomain is a failure domain from the machine template provider spec.
	templateFailureDomain := providerConfig.ExtractFailureDomain()

	// To be able to compare specified failure domains with failure domains from machines we need to combine them with fields set from the template.
	comparableFailureDomain, err := failureDomain.Complete(templateFailureDomain)
	if err != nil {
		return 0, fmt.Errorf("cannot combine failure domain with template failure domain: %w", err)
	}

	index, indexFound := m.failureDomainToIndex(comparableFailureDomain)
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

// isMachineReady determines whether a CPMS Machine is Ready or not.
// A CPMS Machine is considered Ready when:
// - the underlying Machine is Running and its Node is Ready
// - the underlying Machine is Deleting and is still has a NodeRef.
func (m *openshiftMachineProvider) isMachineReady(ctx context.Context, machine machinev1beta1.Machine) (bool, error) {
	if machine.Status.NodeRef == nil {
		return false, nil
	}

	nodeName := machine.Status.NodeRef.Name

	node := &corev1.Node{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return false, fmt.Errorf("failed to get Node %q: %w", nodeName, err)
	}

	if ptr.Deref(machine.Status.Phase, "") == runningPhase && isNodeReady(node) {
		// The machine is running and its node is ready, so everything is working as expected.
		return true, nil
	}

	if ptr.Deref(machine.Status.Phase, "") == runningPhase && isNodeNotReadyWithinNodeGracePeriod(node) {
		// The machine is running and its node has been notReady only for less than the unreadyNodeGracePeriod, so we consider the CPMS Machine to be ready for now.
		// We do so to allow some leeway for intermittent blips, hiccups and node reboots (e.g. for node upgrades).
		// This is due to https://issues.redhat.com/browse/OCPBUGS-20061
		return true, nil
	}

	if ptr.Deref(machine.Status.Phase, "") == deletingPhase && isNodeReady(node) {
		// The machine was previously running but is now being deleted.
		// The machine is still ready until the node is drained and removed from the cluster.
		return true, nil
	}

	return false, nil
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

// isNodeReady returns true if a node is ready; false otherwise.
func isNodeReady(node *corev1.Node) bool {
	if node == nil {
		return false
	}

	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}

	return false
}

// isNodeNotReadyWithinNodeGracePeriod returns true if a node is NotReady but it has been so only for less than the unreadyNodeGracePeriod; false otherwise.
func isNodeNotReadyWithinNodeGracePeriod(node *corev1.Node) bool {
	if node == nil {
		return false
	}

	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue {
				return true
			}

			isWithinUnreadyNodeGracePeriod := c.LastTransitionTime.Add(unreadyNodeGracePeriod).After(time.Now())

			if (c.Status == corev1.ConditionFalse || c.Status == corev1.ConditionUnknown) && isWithinUnreadyNodeGracePeriod {
				// The node is not ready but the last transition time from ready -> not ready was less than the unreadyNodeGracePeriod,
				// hence we consider the node to be presumably in rebooting stage (e.g. due to an upgrade).
				return true
			}
		}
	}

	return false
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

		return fmt.Errorf("cannot create machine: %w", err)
	}

	logger.V(2).Info(
		"Created machine",
		"index", index,
		"machineName", machine.Name,
		"failureDomain", providerConfig.ExtractFailureDomain().String(),
	)

	m.recorder.Eventf(cpms, corev1.EventTypeNormal, "Created", "created machine: %s", machine.Name)

	return nil
}

// getMachineName generates a machine name based on the index.
func (m *openshiftMachineProvider) getMachineName(index int32) (string, error) {
	if m.allowMachineNamePrefix && len(m.machineNamePrefix) > 0 {
		return fmt.Sprintf("%s-%s-%d", m.machineNamePrefix, rand.String(5), index), nil
	}

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

	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: m.ownerMetadata,
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

	m.recorder.Eventf(cpms, corev1.EventTypeNormal, "Deleted", "deleted machine: %s", machine.Name)

	return nil
}
