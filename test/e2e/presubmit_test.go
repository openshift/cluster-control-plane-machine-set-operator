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

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/features"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/helpers"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.PreSubmit(), func() {
	BeforeEach(func() {
		helpers.EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 10*time.Minute, 10*time.Second)
	}, OncePerOrdered)

	Context("With an active ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureActiveControlPlaneMachineSet(testFramework)
		}, OncePerOrdered)

		Context("and the provider spec of index 1 is not as expected", func() {
			BeforeEach(func() {
				helpers.ModifyMachineProviderSpecToTriggerRollout(testFramework, 1)
			})

			helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(testFramework, 1)
		})

		Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", func() {
			prefix := "master-prefix"
			resetPrefix := ""

			BeforeEach(func() {
				// Check if CPMSMachineNamePrefix gate is enabled, skip otherwise.
				// The TechPreview jobs should not skip the test.
				featureGateFilter, err := helpers.NewFeatureGateFilter(context.TODO(), testFramework)
				if err != nil {
					Fail(fmt.Sprintf("failed to get featuregate filter: %v", err))
				}
				if !featureGateFilter.IsEnabled(string(features.FeatureGateCPMSMachineNamePrefix)) {
					Skip(fmt.Sprintf("Skipping test because %q featuregate is not enabled", features.FeatureGateCPMSMachineNamePrefix))
				}

				helpers.UpdateControlPlaneMachineSetMachineNamePrefix(testFramework, prefix)
			})

			AfterEach(func() {
				helpers.UpdateControlPlaneMachineSetMachineNamePrefix(testFramework, resetPrefix)
			})

			Context("and the provider spec of index 1 is not as expected", func() {
				BeforeEach(func() {
					helpers.ModifyMachineProviderSpecToTriggerRollout(testFramework, 1)
				})

				helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(testFramework, 1)
			})
		})

		Context("with the OnDelete update strategy", func() {
			var originalStrategy machinev1.ControlPlaneMachineSetStrategyType

			BeforeEach(func() {
				originalStrategy = helpers.EnsureControlPlaneMachineSetUpdateStrategy(testFramework, machinev1.OnDelete)
			}, OncePerOrdered)

			AfterEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdateStrategy(testFramework, originalStrategy)
			}, OncePerOrdered)

			Context("and the provider spec of index 2 is not as expected", Ordered, func() {
				var originalProviderSpec machinev1beta1.ProviderSpec

				BeforeAll(func() {
					originalProviderSpec, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(testFramework, 2)
				})

				AfterAll(func() {
					helpers.UpdateControlPlaneMachineProviderSpec(testFramework, 2, originalProviderSpec)
				})

				helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(testFramework, 2)

				helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(testFramework, 2)
			})

			Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", Ordered, func() {
				prefix := "master-prefix-on-delete"
				resetPrefix := ""

				BeforeEach(func() {
					// Check if CPMSMachineNamePrefix gate is enabled, skip otherwise.
					// The TechPreview jobs should not skip the test.
					featureGateFilter, err := helpers.NewFeatureGateFilter(context.TODO(), testFramework)
					if err != nil {
						Fail(fmt.Sprintf("failed to get featuregate filter: %v", err))
					}
					if !featureGateFilter.IsEnabled(string(features.FeatureGateCPMSMachineNamePrefix)) {
						Skip(fmt.Sprintf("Skipping test because %q featuregate is not enabled", features.FeatureGateCPMSMachineNamePrefix))
					}

					helpers.UpdateControlPlaneMachineSetMachineNamePrefix(testFramework, prefix)
				}, OncePerOrdered)

				AfterEach(func() {
					helpers.UpdateControlPlaneMachineSetMachineNamePrefix(testFramework, resetPrefix)
				}, OncePerOrdered)

				Context("and the provider spec of index 1 is not as expected", Ordered, func() {
					var originalProviderSpec machinev1beta1.ProviderSpec

					BeforeAll(func() {
						originalProviderSpec, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(testFramework, 1)
					})

					AfterAll(func() {
						helpers.UpdateControlPlaneMachineProviderSpec(testFramework, 1, originalProviderSpec)
					})

					helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(testFramework, 1)

					helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(testFramework, 1)
				})
			})
		})

		Context("and the ControlPlaneMachineSet is up to date", Ordered, func() {
			BeforeEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(testFramework)
			})

			Context("and the ControlPlaneMachineSet is deleted", func() {
				BeforeEach(func() {
					helpers.EnsureControlPlaneMachineSetDeleted(testFramework)
				})

				AfterEach(func() {
					helpers.EnsureActiveControlPlaneMachineSet(testFramework)
				})

				helpers.ItShouldUninstallTheControlPlaneMachineSet(testFramework)
				helpers.ItShouldHaveTheControlPlaneMachineSetReplicasUpdated(testFramework)

				Context("and the ControlPlaneMachineSet is reactivated", func() {
					BeforeEach(func() {
						helpers.EnsureControlPlaneMachineSetUpdated(testFramework)
						helpers.EnsureActiveControlPlaneMachineSet(testFramework)
					})

					helpers.ItShouldNotCauseARollout(testFramework)
					helpers.ItShouldCheckAllControlPlaneMachinesHaveCorrectOwnerReferences(testFramework)
				})
			})
		})

		Context("and a defaulted value is deleted from the ControlPlaneMachineSet", func() {
			var originalProviderSpec machinev1beta1.ProviderSpec
			BeforeEach(func() {
				// There is no defaulting webhook for the machines running on the following platforms.
				switch testFramework.GetPlatformType() {
				case configv1.OpenStackPlatformType:
					Skip("Skipping test on OpenStack platform")
				}

				_ = helpers.EnsureControlPlaneMachineSetUpdateStrategy(testFramework, machinev1.RollingUpdate)
				originalProviderSpec = helpers.UpdateDefaultedValueFromControlPlaneMachineSetProviderConfig(testFramework)
			})

			AfterEach(func() {
				helpers.EnsureActiveControlPlaneMachineSet(testFramework)
				helpers.UpdateControlPlaneMachineSetProviderSpec(testFramework, originalProviderSpec)
			})

			helpers.ItShouldNotCauseARollout(testFramework)
		})
	})

	Context("With an inactive ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureInactiveControlPlaneMachineSet(testFramework)
		})

		Context("and the ControlPlaneMachineSet is up to date", func() {
			BeforeEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(testFramework)
			})

			AfterEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(testFramework)
			})

			Context("and there is diff in the providerSpec of the newest, alphabetically last machine", func() {
				var opts helpers.ControlPlaneMachineSetRegenerationTestOptions

				BeforeEach(func() {
					opts.TestFramework = testFramework
					opts.UID = helpers.GetControlPlaneMachineSetUID(testFramework)
					opts.Index, opts.OriginalProviderSpec, opts.UpdatedProviderSpec = helpers.ModifyNewestMachineProviderSpecToTriggerRollout(testFramework)
				})

				AfterEach(func() {
					helpers.UpdateControlPlaneMachineProviderSpec(testFramework, opts.Index, opts.OriginalProviderSpec)
				})

				helpers.ItShouldPerformControlPlaneMachineSetRegeneration(&opts)
			})
		})
	})
})
