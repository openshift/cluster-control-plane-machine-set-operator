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

package controlplanemachinesetgenerator

import (
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

// generateControlPlaneMachineSetVSphereSpec generates an VSphere flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetVSphereSpec(logger logr.Logger, machines []machinev1beta1.Machine, machineSets []machinev1beta1.MachineSet, infrastructure *configv1.Infrastructure) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildFailureDomains(logger, machineSets, machines, infrastructure)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's VSphere failure domains: %w", err)
	}

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetVSphereMachineSpec(logger, machines, infrastructure)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's VSphere spec: %w", err)
	}

	// We want to work with the newest machine.
	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])

	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	if controlPlaneMachineSetMachineFailureDomainsApplyConfig != nil {
		logger.V(1).Info(
			"vsphere failureDomain configuration",
			"platform", controlPlaneMachineSetMachineFailureDomainsApplyConfig.Platform,
			"count", controlPlaneMachineSetMachineFailureDomainsApplyConfig.VSphere,
		)

		controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	}

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildControlPlaneMachineSetVSphereMachineSpec builds an VSphere flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetVSphereMachineSpec(logger logr.Logger, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	templateMachine := machines[0].Spec

	// The machines slice is sorted by the creation time.
	// We want to get the provider config for the newest machine.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, templateMachine, infrastructure)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's providerSpec: %w", err)
	}

	// if failure domains are defined the workspace, networks, and template are cleared to prevent ambiguity.
	if len(infrastructure.Spec.PlatformSpec.VSphere.FailureDomains) > 0 {
		providerConfig = providerConfig.VSphere().ResetTopologyRelatedFields()
	}

	rawBytes, err := providerConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %w", err)
	}

	re := runtime.RawExtension{
		Raw: rawBytes,
	}

	msac := &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: &re},
	}

	return msac, nil
}

// buildVSphereFailureDomains builds a VSphere flavored FailureDomains for the ControlPlaneMachineSet.
func buildVSphereFailureDomains(failureDomains *failuredomain.Set) machinev1.FailureDomains {
	VSphereFailureDomains := []machinev1.VSphereFailureDomain{}

	for _, fd := range failureDomains.List() {
		if len(fd.VSphere().Name) == 0 {
			continue
		}

		VSphereFailureDomains = append(VSphereFailureDomains, fd.VSphere())
	}

	if len(VSphereFailureDomains) > 0 {
		return machinev1.FailureDomains{
			VSphere:  VSphereFailureDomains,
			Platform: configv1.VSpherePlatformType,
		}
	}

	return machinev1.FailureDomains{}
}
