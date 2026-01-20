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

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift/api/features"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/helpers"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.Periodic(), Label("Disruptive"), Label("Serial"), func() {
	BeforeEach(func() {
		helpers.EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 10*time.Minute, 10*time.Second)
	}, OncePerOrdered)

	Context("With an active ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureActiveControlPlaneMachineSet(framework.GlobalFramework)
		})

		Context("and the provider spec is changed", func() {
			BeforeEach(func() {
				helpers.ModifyControlPlaneMachineSetToTriggerRollout(framework.GlobalFramework)
			})

			helpers.ItShouldPerformARollingUpdate(&helpers.RollingUpdatePeriodicTestOptions{
				TestFramework: framework.GlobalFramework,
			})
		})

		Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", func() {
			prefix := "master-prefix"
			resetPrefix := ""

			BeforeEach(func() {
				// Check if CPMSMachineNamePrefix gate is enabled, skip otherwise.
				// The TechPreview jobs should not skip the test.
				featureGateFilter, err := helpers.NewFeatureGateFilter(context.TODO(), framework.GlobalFramework)
				if err != nil {
					Fail(fmt.Sprintf("failed to get featuregate filter: %v", err))
				}
				if !featureGateFilter.IsEnabled(string(features.FeatureGateCPMSMachineNamePrefix)) {
					Skip(fmt.Sprintf("Skipping test because %q featuregate is not enabled", features.FeatureGateCPMSMachineNamePrefix))
				}

				helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, prefix)
			}, OncePerOrdered)

			Context("and the provider spec of index 1 is not as expected", Ordered, func() {
				BeforeAll(func() {
					helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
				})

				// Machine name should follow prefixed naming convention
				helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(framework.GlobalFramework, 1)

				Context("and again MachineNamePrefix is reset", Ordered, func() {
					BeforeAll(func() {
						helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, resetPrefix)
						helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
					})

					// Machine name should follow general naming convention
					helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(framework.GlobalFramework, 1)
				})
			})
		})

		Context("with the OnDelete update strategy", func() {
			var originalStrategy machinev1.ControlPlaneMachineSetStrategyType

			BeforeEach(func() {
				originalStrategy = helpers.EnsureControlPlaneMachineSetUpdateStrategy(framework.GlobalFramework, machinev1.OnDelete)
			}, OncePerOrdered)

			AfterEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdateStrategy(framework.GlobalFramework, originalStrategy)
			}, OncePerOrdered)

			Context("and all three master machines are deleted simultaneously", Ordered, func() {
				var originalProviderSpec0, originalProviderSpec1, originalProviderSpec2 machinev1beta1.ProviderSpec

				BeforeAll(func() {
					// Modify all three machines to trigger updates
					originalProviderSpec0, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 0)
					originalProviderSpec1, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
					originalProviderSpec2, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 2)
				})

				AfterAll(func() {
					// Restore original provider specs
					helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 0, originalProviderSpec0)
					helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 1, originalProviderSpec1)
					helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 2, originalProviderSpec2)
				})

				helpers.ItShouldOnDeleteReplaceAllThreeMastersWhenDeleted(framework.GlobalFramework)
			})

			Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", Ordered, func() {
				prefix := "master-prefix-on-delete"
				resetPrefix := ""

				BeforeEach(func() {
					// Check if CPMSMachineNamePrefix gate is enabled, skip otherwise.
					// The TechPreview jobs should not skip the test.
					featureGateFilter, err := helpers.NewFeatureGateFilter(context.TODO(), framework.GlobalFramework)
					if err != nil {
						Fail(fmt.Sprintf("failed to get featuregate filter: %v", err))
					}
					if !featureGateFilter.IsEnabled(string(features.FeatureGateCPMSMachineNamePrefix)) {
						Skip(fmt.Sprintf("Skipping test because %q featuregate is not enabled", features.FeatureGateCPMSMachineNamePrefix))
					}

					helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, prefix)
				}, OncePerOrdered)

				Context("and the provider spec of index 2 is not as expected", Ordered, func() {
					var originalProviderSpec machinev1beta1.ProviderSpec

					BeforeAll(func() {
						originalProviderSpec, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 2)
					})

					AfterAll(func() {
						helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 2, originalProviderSpec)
					})

					helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(framework.GlobalFramework, 2)

					// Machine name should follow prefixed naming convention
					helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(framework.GlobalFramework, 2)

					Context("and again MachineNamePrefix is reset", Ordered, func() {
						BeforeAll(func() {
							helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, resetPrefix)
							helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 2)
						})

						helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(framework.GlobalFramework, 2)

						// Machine name should follow general naming convention
						helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(framework.GlobalFramework, 2)
					})
				})
			})
		})

	})
})
