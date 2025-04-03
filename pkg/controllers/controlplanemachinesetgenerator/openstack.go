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

// checkOpenStackMachinesServerGroups checks if all machines have the same ServerGroup (the reference is the newest machine's ServerGroup).
func checkOpenStackMachinesServerGroups(logger logr.Logger, machines []machinev1beta1.Machine) error {
	newestMachineProviderConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machines[0].Spec, nil)
	if err != nil {
		return fmt.Errorf("failed to extract newest machine's OpenStack providerSpec: %w", err)
	}

	newestOpenStackProviderSpec := newestMachineProviderConfig.OpenStack().Config()
	newestServerGroup := newestOpenStackProviderSpec.ServerGroupName

	for _, machine := range machines {
		// get the providerSpec from the machine
		providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machine.Spec, nil)
		if err != nil {
			return fmt.Errorf("failed to extract machine's OpenStack providerSpec: %w", err)
		}

		openStackProviderSpec := providerConfig.OpenStack().Config()
		// Return an error if the ServerGroup is not the same as the newest machine's ServerGroup.
		if openStackProviderSpec.ServerGroupName != newestServerGroup {
			return fmt.Errorf("%w: machine %s has a different ServerGroup than the newest machine. Check this KCS article for more information: https://access.redhat.com/solutions/7013893", errInconsistentProviderSpec, machine.Name)
		}
	}

	return nil
}

// generateControlPlaneMachineSetOpenStackSpec generates an OpenStack flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetOpenStackSpec(logger logr.Logger, machines []machinev1beta1.Machine) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	// We want to make sure that the machines are ready to be used for generating a ControlPlaneMachineSet.
	if err := checkOpenStackMachinesServerGroups(logger, machines); err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to check OpenStack machines ServerGroup: %w", err)
	}

	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildFailureDomains(logger, machines, nil)
	if errors.Is(err, errNoFailureDomains) {
		// This is a special case where we don't have any failure domains.
	} else if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's OpenStack failure domains: %w", err)
	}

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetOpenStackMachineSpec(logger, machines, controlPlaneMachineSetMachineFailureDomainsApplyConfig)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's OpenStack spec: %w", err)
	}

	// We want to work with the newest machine.
	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildControlPlaneMachineSetOpenStackMachineSpec builds an OpenStack flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetOpenStackMachineSpec(logger logr.Logger, machines []machinev1beta1.Machine, failureDomains *machinev1builder.FailureDomainsApplyConfiguration) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	// The machines slice is sorted by the creation time.
	// We want to get the provider config for the newest machine.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machines[0].Spec, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's OpenStack providerSpec: %w", err)
	}

	openStackProviderSpec := providerConfig.OpenStack().Config()

	// If there are failure domains, remove the corresponding managed fields from the providerSpec.
	if failureDomains != nil && len(failureDomains.OpenStack) > 0 {
		openStackProviderSpec.AvailabilityZone = ""

		if openStackProviderSpec.RootVolume != nil {
			openStackProviderSpec.RootVolume.Zone = ""
			openStackProviderSpec.RootVolume.VolumeType = ""
		}
	}

	rawBytes, err := json.Marshal(openStackProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("error marshalling OpenStack providerSpec: %w", err)
	}

	re := runtime.RawExtension{
		Raw: rawBytes,
	}

	return &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: &re},
	}, nil
}

// buildOpenStackFailureDomains builds an OpenStack flavored FailureDomains for the ControlPlaneMachineSet.
func buildOpenStackFailureDomains(failureDomains *failuredomain.Set) (machinev1.FailureDomains, error) {
	openStackFailureDomains := []machinev1.OpenStackFailureDomain{}
	for _, fd := range failureDomains.List() {
		openStackFailureDomains = append(openStackFailureDomains, fd.OpenStack())
	}

	emptyOpenStackFailureDomain := machinev1.OpenStackFailureDomain{}
	emptyFailureDomain := machinev1.FailureDomains{}

	// We want to make sure that if a failure domain is empty, it is the only one.
	if len(openStackFailureDomains) > 1 {
		for _, fd := range openStackFailureDomains {
			if fd == emptyOpenStackFailureDomain {
				return emptyFailureDomain, fmt.Errorf("error building OpenStack failure domains: %w", errMixedEmptyFailureDomains)
			}
		}
	}

	// We want to make sure that if a failure domain is empty, we don't create it.
	if len(openStackFailureDomains) == 1 && openStackFailureDomains[0] == emptyOpenStackFailureDomain {
		openStackFailureDomains = nil
	}

	return machinev1.FailureDomains{
		OpenStack: openStackFailureDomains,
		Platform:  configv1.OpenStackPlatformType,
	}, nil
}
