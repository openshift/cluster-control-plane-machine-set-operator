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
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1"
	machinev1beta1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"k8s.io/apimachinery/pkg/runtime"
)

// generateControlPlaneMachineSetAzureSpec generates an Azure flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetAzureSpec(machines []machinev1beta1.Machine, machineSets []machinev1beta1.MachineSet) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildAzureFailureDomains(machineSets, machines)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's Azure failure domains: %w", err)
	}

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetAzureMachineSpec(machines)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's Azure spec: %w", err)
	}

	// We want to work with the newest machine.
	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildAzureFailureDomains builds an AzureFailureDomain config for the ControlPaneMachineSet from the cluster's Machines and MachineSets.
func buildAzureFailureDomains(machineSets []machinev1beta1.MachineSet, machines []machinev1beta1.Machine) (*machinev1builder.FailureDomainsApplyConfiguration, error) {
	// Fetch failure domains from the machines
	machineFailureDomains, err := providerconfig.ExtractFailureDomainsFromMachines(machines)
	if err != nil {
		return nil, fmt.Errorf("failed to extract failure domains from machines: %w", err)
	}

	// Fetch failure domains from the machineSets
	machineSetFailureDomains, err := providerconfig.ExtractFailureDomainsFromMachineSets(machineSets)
	if err != nil {
		return nil, fmt.Errorf("failed to extract failure domains from machine sets: %w", err)
	}

	// We have to get rid of duplicates from the failure domains.
	// We construct a set from the failure domains, since a set can't have duplicates.
	failureDomains := failuredomain.NewSet(machineFailureDomains...)
	// Construction of a union of failure domains of machines and machineSets.
	failureDomains.Insert(machineSetFailureDomains...)

	azureFailureDomains := []machinev1.AzureFailureDomain{}
	for _, fd := range failureDomains.List() {
		azureFailureDomains = append(azureFailureDomains, fd.Azure())
	}

	cpmsFailureDomain := machinev1.FailureDomains{
		Azure:    &azureFailureDomains,
		Platform: configv1.AzurePlatformType,
	}

	cpmsFailureDomainsApplyConfig := &machinev1builder.FailureDomainsApplyConfiguration{}
	if err := convertViaJSON(cpmsFailureDomain, cpmsFailureDomainsApplyConfig); err != nil {
		return nil, fmt.Errorf("failed to convert machinev1.FailureDomains to machinev1builder.FailureDomainsApplyConfiguration: %w", err)
	}

	return cpmsFailureDomainsApplyConfig, nil
}

// buildControlPlaneMachineSetAzureMachineSpec builds an Azure flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetAzureMachineSpec(machines []machinev1beta1.Machine) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	// The machines slice is sorted by the creation time.
	// We want to get the provider config for the newest machine.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(machines[0].Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's azure providerSpec: %w", err)
	}

	azureProviderSpec := providerConfig.Azure().Config()
	// Remove field related to the faliure domain.
	azureProviderSpec.Zone = nil

	rawBytes, err := json.Marshal(azureProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("error marshalling azure providerSpec: %w", err)
	}

	re := runtime.RawExtension{
		Raw: rawBytes,
	}

	return &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: &re},
	}, nil
}
