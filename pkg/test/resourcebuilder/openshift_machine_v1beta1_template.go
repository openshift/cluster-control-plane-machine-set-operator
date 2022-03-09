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

package resourcebuilder

import (
	machinev1 "github.com/openshift/api/machine/v1"
)

// OpenShiftMachineV1Beta1Template creates a new OpenShift machine template builder.
func OpenShiftMachineV1Beta1Template() OpenShiftMachineV1Beta1TemplateBuilder {
	return OpenShiftMachineV1Beta1TemplateBuilder{
		failureDomainsBuilder: AWSFailureDomains(),
		labels: map[string]string{
			machineRoleLabelName: "master",
			machineTypeLabelName: "master",
		},
	}
}

// OpenShiftMachineV1Beta1TemplateBuilder is used to build out an OpenShift machine template.
type OpenShiftMachineV1Beta1TemplateBuilder struct {
	failureDomainsBuilder OpenShiftMachineV1Beta1FailureDomainsBuilder
	labels                map[string]string
	providerSpecBuilder   RawExtensionBuilder
}

// BuildTemplate builds a new machine template based on the configuration provided.
func (m OpenShiftMachineV1Beta1TemplateBuilder) BuildTemplate() machinev1.ControlPlaneMachineSetTemplate {
	template := machinev1.ControlPlaneMachineSetTemplate{
		MachineType: machinev1.OpenShiftMachineV1Beta1MachineType,
		OpenShiftMachineV1Beta1Machine: &machinev1.OpenShiftMachineV1Beta1MachineTemplate{
			ObjectMeta: machinev1.ControlPlaneMachineSetTemplateObjectMeta{
				Labels: m.labels,
			},
		},
	}

	if m.failureDomainsBuilder != nil {
		template.OpenShiftMachineV1Beta1Machine.FailureDomains = m.failureDomainsBuilder.BuildFailureDomains()
	}

	if m.providerSpecBuilder != nil {
		template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value = m.providerSpecBuilder.BuildRawExtension()
	}

	return template
}

// WithFailureDomainsBuilder sets the failure domains builder for the machine template builder.
func (m OpenShiftMachineV1Beta1TemplateBuilder) WithFailureDomainsBuilder(fdsBuilder OpenShiftMachineV1Beta1FailureDomainsBuilder) OpenShiftMachineV1Beta1TemplateBuilder {
	m.failureDomainsBuilder = fdsBuilder
	return m
}

// WithLabel sets the label on the machine labels for the machine template builder.
func (m OpenShiftMachineV1Beta1TemplateBuilder) WithLabel(key, value string) OpenShiftMachineV1Beta1TemplateBuilder {
	if m.labels == nil {
		m.labels = make(map[string]string)
	}

	m.labels[key] = value

	return m
}

// WithLabels sets the labels for the machine template builder.
func (m OpenShiftMachineV1Beta1TemplateBuilder) WithLabels(labels map[string]string) OpenShiftMachineV1Beta1TemplateBuilder {
	m.labels = labels
	return m
}

// WithProviderSpecBuilder sets the providerSpec builder for the machine template builder.
func (m OpenShiftMachineV1Beta1TemplateBuilder) WithProviderSpecBuilder(builder RawExtensionBuilder) OpenShiftMachineV1Beta1TemplateBuilder {
	m.providerSpecBuilder = builder
	return m
}
