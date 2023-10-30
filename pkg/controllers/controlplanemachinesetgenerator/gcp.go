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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1"
	machinev1beta1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"k8s.io/apimachinery/pkg/runtime"
)

// generateControlPlaneMachineSetGCPSpec generates an GCP flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetGCPSpec(logger logr.Logger, machines []machinev1beta1.Machine, machineSets []machinev1beta1.MachineSet) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildFailureDomains(logger, machineSets, machines, nil)
	if errors.Is(err, errNoFailureDomains) {
		// This is a special case where we don't have any failure domains.
	} else if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's GCP failure domains: %w", err)
	}

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetGCPMachineSpec(logger, machines)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's GCP spec: %w", err)
	}

	// We want to work with the newest machine.
	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildControlPlaneMachineSetGCPMachineSpec builds an GCP flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetGCPMachineSpec(logger logr.Logger, machines []machinev1beta1.Machine) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	// The machines slice is sorted by the creation time.
	// We want to get the provider config for the newest machine.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machines[0].Spec, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's GCP providerSpec: %w", err)
	}

	gcpProviderSpec := providerConfig.GCP().Config()
	// Remove field related to the faliure domain.
	gcpProviderSpec.Zone = ""

	rawBytes, err := json.Marshal(gcpProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("error marshalling GCP providerSpec: %w", err)
	}

	re := runtime.RawExtension{
		Raw: rawBytes,
	}

	return &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: &re},
	}, nil
}

// buildGCPFailureDomains builds a GCP flavored FailureDomains for the ControlPlaneMachineSet.
func buildGCPFailureDomains(failureDomains *failuredomain.Set) (machinev1.FailureDomains, error) { //nolint:unparam
	gcpFailureDomains := []machinev1.GCPFailureDomain{}

	for _, fd := range failureDomains.List() {
		gcpFailureDomains = append(gcpFailureDomains, fd.GCP())
	}

	cpmsFailureDomains := machinev1.FailureDomains{
		GCP:      &gcpFailureDomains,
		Platform: configv1.GCPPlatformType,
	}

	return cpmsFailureDomains, nil
}
