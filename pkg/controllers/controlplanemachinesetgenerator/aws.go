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
	runtime "k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
)

const (
	// replicas is the number of ControlPlaneMachineSet indexes.
	// We always assume 3 replicas here. There might be very rare occasions where the number differs,
	// but that's an exception that must be manually forced on the applied ControlPlaneMachineSet object.
	replicas int32 = 3
)

// generateControlPlaneMachineSetAWSSpec generates an AWS flavored ControlPlaneMachineSet Spec.
func generateControlPlaneMachineSetAWSSpec(logger logr.Logger, machines []machinev1beta1.Machine) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	controlPlaneMachineSetMachineFailureDomainsApplyConfig, err := buildFailureDomains(logger, machines, nil)
	if errors.Is(err, errNoFailureDomains) {
		// This is a special case where we don't have any failure domains.
	} else if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's AWS failure domains: %w", err)
	}

	controlPlaneMachineSetMachineSpecApplyConfig, err := buildControlPlaneMachineSetAWSMachineSpec(logger, machines)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to build ControlPlaneMachineSet's AWS spec: %w", err)
	}

	controlPlaneMachineSetApplyConfigSpec := genericControlPlaneMachineSetSpec(replicas, machines[0].ObjectMeta.Labels[clusterIDLabelKey])
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = controlPlaneMachineSetMachineFailureDomainsApplyConfig
	controlPlaneMachineSetApplyConfigSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = controlPlaneMachineSetMachineSpecApplyConfig

	return controlPlaneMachineSetApplyConfigSpec, nil
}

// buildControlPlaneMachineSetAWSMachineSpec builds an AWS flavored MachineSpec for the ControlPlaneMachineSet.
func buildControlPlaneMachineSetAWSMachineSpec(logger logr.Logger, machines []machinev1beta1.Machine) (*machinev1beta1builder.MachineSpecApplyConfiguration, error) {
	// Take the Provider Spec of the first in the machines slice
	// as a the one to be put on the ControlPlaneMachineSet spec.
	// Since the `machines` slice is sorted by descending creation time
	// we are guaranteed to get the newest Provider Spec of a machine.
	// This is done so that if there are control plane machines with differing
	// Provider Specs, we will use the most recent one. This is an attempt to try and inferr
	// the spec that the user might want to choose among the different ones found in the cluster.
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(logger, machines[0].Spec, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to extract machine's aws providerSpec: %w", err)
	}

	awsPs := providerConfig.AWS().Config()
	// Remove from the extracted AWS Provider Spec the Failure domains,
	// as those are already present in the ControlPlaneMachineSet spec.
	awsPs.Subnet = machinev1beta1.AWSResourceReference{}
	awsPs.Placement.AvailabilityZone = ""

	rawBytes, err := json.Marshal(awsPs)
	if err != nil {
		return nil, fmt.Errorf("error marshalling aws providerSpec: %w", err)
	}

	re := runtime.RawExtension{
		Raw: rawBytes,
	}

	return &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: &re},
	}, nil
}

// buildAWSFailureDomains builds an AWS flavored FailureDomains for the ControlPlaneMachineSet.
func buildAWSFailureDomains(failureDomains *failuredomain.Set) (machinev1.FailureDomains, error) { //nolint:unparam
	awsFailureDomains := []machinev1.AWSFailureDomain{}

	for _, fd := range failureDomains.List() {
		awsFailureDomains = append(awsFailureDomains, fd.AWS())
	}

	cpmsFailureDomain := machinev1.FailureDomains{
		AWS:      &awsFailureDomains,
		Platform: configv1.AWSPlatformType,
	}

	return cpmsFailureDomain, nil
}
