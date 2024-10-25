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

package controlplanemachineset

import (
	"context"

	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createVSphereControlPlaneMachinesWithZones creates control plane machines which are in corresponding failure domains and returns those failure domains.
func createVSphereControlPlaneMachinesWithZones(
	ctx context.Context,
	k8sClient client.Client,
	namespaceName string,
	infrastructure *configv1.Infrastructure) *machinev1.FailureDomains {
	fdBuilder := machinev1resourcebuilder.VSphereFailureDomains()
	failureDomains := fdBuilder.BuildFailureDomains()

	for _, fd := range failureDomains.VSphere {
		machineProviderSpec := machinev1beta1resourcebuilder.VSphereProviderSpec().WithInfrastructure(*infrastructure).WithZone(fd.Name)

		machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
		controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(machineProviderSpec)

		controlPlaneMachine := controlPlaneMachineBuilder.Build()
		Eventually(k8sClient.Create(ctx, controlPlaneMachine)).Should(Succeed(), "expected to be able to create the control plane machine")
	}

	return &failureDomains
}
