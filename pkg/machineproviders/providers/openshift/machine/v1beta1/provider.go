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

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// errEmptyConfig is used to denote that the machine provider could not be constructed
	// because no configuration was provided by the user.
	errEmptyConfig = fmt.Errorf("cannot initialise %s provider with empty config", machinev1.OpenShiftMachineV1Beta1MachineType)

	// errMissingClusterIDLabel is used to denote that the cluster ID label, expected to be on the Machine template
	// is not present and therefore a Machine cannot be created. The Cluster ID is required to construct the name for
	// the new Machines.
	errMissingClusterIDLabel = fmt.Errorf("missing required label on machine template metadata: %s", machinev1beta1.MachineClusterIDLabel)

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

	providerConfig, err := providerconfig.NewProviderConfig(*cpms.Spec.Template.OpenShiftMachineV1Beta1Machine)
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

	return &openshiftMachineProvider{
		client:               cl,
		indexToFailureDomain: indexToFailureDomain,
		machineSelector:      cpms.Spec.Selector,
		machineTemplate:      *cpms.Spec.Template.OpenShiftMachineV1Beta1Machine,
		ownerMetadata:        cpms.ObjectMeta,
		providerConfig:       providerConfig,
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
	return nil, nil
}

// CreateMachine creates a new Machine from the template provider config based on the
// failure domain index provided.
func (m *openshiftMachineProvider) CreateMachine(ctx context.Context, logger logr.Logger, index int32) error {
	return nil
}

// DeleteMachine deletes the Machine references in the machineRef provided.
func (m *openshiftMachineProvider) DeleteMachine(ctx context.Context, logger logr.Logger, machineRef *machineproviders.ObjectRef) error {
	return nil
}
