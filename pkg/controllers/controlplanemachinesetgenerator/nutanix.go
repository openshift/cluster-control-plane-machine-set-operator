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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1"
	machinev1beta1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
)

var errInvalidFailureDomainName = errors.New("invalid failure domain name")

// generateControlPlaneMachineSetNutanixSpec generates a Nutanix flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetNutanixSpec(logger logr.Logger, machines []machinev1beta1.Machine, machineSets []machinev1beta1.MachineSet, infrastructure *configv1.Infrastructure) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildFailureDomains(logger, machineSets, machines, infrastructure)
	if err != nil {
		if !errors.Is(err, errNoFailureDomains) {
			return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's Nutanix failure domains: %w", err)
		}

		logger.V(1).Info("No nutanix failureDomain is configured")

		controlPlaneMachineSetMachineFailureDomainsApplyConfig = &machinev1builder.FailureDomainsApplyConfiguration{}
	} else {
		logger.V(1).Info(
			"nutanix failureDomain configuration",
			"platform", controlPlaneMachineSetMachineFailureDomainsApplyConfig.Platform,
			"failureDomains", len(controlPlaneMachineSetMachineFailureDomainsApplyConfig.Nutanix),
		)
	}

	failureDomainsConfigured := len(controlPlaneMachineSetMachineFailureDomainsApplyConfig.Nutanix) > 0

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetNutanixMachineSpec(logger, machines, infrastructure, failureDomainsConfigured)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's Nutanix spec: %w", err)
	}

	// We want to work with the newest machine.
	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildControlPlaneMachineSetNutanixMachineSpec builds a Nutanix flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetNutanixMachineSpec(logger logr.Logger, machines []machinev1beta1.Machine, infrastructure *configv1.Infrastructure, failureDomainsConfigured bool) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	// The machines slice is sorted by the creation time.
	// We want to get the provider config for the newest machine.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machines[0].Spec, infrastructure)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's providerSpec: %w", err)
	}

	// if failure domains are configured, the providerConfig's cluster, subnets are cleared to prevent ambiguity.
	if failureDomainsConfigured {
		providerConfig = providerConfig.Nutanix().ResetFailureDomainRelatedFields()
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

// buildNutanixFailureDomains builds a Nutanix flavored FailureDomains for the ControlPlaneMachineSet.
func buildNutanixFailureDomains(logger logr.Logger, failureDomains *failuredomain.Set, infrastructure *configv1.Infrastructure) (machinev1.FailureDomains, error) {
	logger.V(4).Info("building Nutanix failureDomains for ControlPlaneMachineSet", "baseFailureDomains", failureDomains.List())

	nutanixFdRefs := []machinev1.NutanixFailureDomainReference{}
	fdNameMap := map[string]string{}

	// fdNameMap is used to hold the NutanixFailureDomain names configured in the Infrastructure CR
	for _, nutanixFd := range infrastructure.Spec.PlatformSpec.Nutanix.FailureDomains {
		if _, ok := fdNameMap[nutanixFd.Name]; !ok {
			fdNameMap[nutanixFd.Name] = nutanixFd.Name
		}
	}

	for _, fd := range failureDomains.List() {
		fdName := fd.Nutanix().Name
		if fdName == "" {
			continue
		}

		if _, ok := fdNameMap[fdName]; !ok {
			return machinev1.FailureDomains{}, fmt.Errorf("failure domain name %q is not defined in the Infrastructure resource: %w", fdName, errInvalidFailureDomainName)
		}

		nutanixFdRefs = append(nutanixFdRefs, fd.Nutanix())
	}

	if len(nutanixFdRefs) > 0 {
		return machinev1.FailureDomains{
			Platform: configv1.NutanixPlatformType,
			Nutanix:  nutanixFdRefs,
		}, nil
	}

	return machinev1.FailureDomains{}, nil
}
