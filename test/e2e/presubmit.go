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
	"time"

	. "github.com/onsi/ginkgo/v2"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/helpers"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.PreSubmit(), Label("Disruptive"), Label("Serial"), func() {
	BeforeEach(func() {
		helpers.EventuallyClusterOperatorsShouldStabilise(1*time.Minute, 10*time.Minute, 10*time.Second)
	}, OncePerOrdered)

	Context("With an active ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureActiveControlPlaneMachineSet(framework.GlobalFramework)
		}, OncePerOrdered)

		Context("and the provider spec of index 1 is not as expected", func() {
			BeforeEach(func() {
				helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
			})

			helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(1)
		})

		Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", func() {
			prefix := "master-prefix"
			resetPrefix := ""

			BeforeEach(func() {
				helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, prefix)
			}, OncePerOrdered)

			Context("and the provider spec of index 1 is not as expected", Ordered, func() {
				BeforeAll(func() {
					helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
				})

				// Machine name should follow prefixed naming convention
				helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(1)

				Context("and again MachineNamePrefix is reset", Ordered, func() {
					BeforeAll(func() {
						helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, resetPrefix)
						helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
					})

					// Machine name should follow general naming convention
					helpers.ItShouldRollingUpdateReplaceTheOutdatedMachine(1)
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

			Context("and the provider spec of index 2 is not as expected", Ordered, func() {
				var originalProviderSpec machinev1beta1.ProviderSpec

				BeforeAll(func() {
					originalProviderSpec, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 2)
				})

				AfterAll(func() {
					helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 2, originalProviderSpec)
				})

				helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(2)

				helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(2)
			})

			Context("and ControlPlaneMachineSet is updated to set MachineNamePrefix [OCPFeatureGate:CPMSMachineNamePrefix]", Ordered, func() {
				prefix := "master-prefix-on-delete"
				resetPrefix := ""

				BeforeEach(func() {
					helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, prefix)
				}, OncePerOrdered)

				Context("and the provider spec of index 1 is not as expected", Ordered, func() {
					var originalProviderSpec machinev1beta1.ProviderSpec

					BeforeAll(func() {
						originalProviderSpec, _ = helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
					})

					AfterAll(func() {
						helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, 1, originalProviderSpec)
					})

					helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(1)

					// Machine name should follow prefixed naming convention
					helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(1)

					Context("and again MachineNamePrefix is reset", Ordered, func() {
						BeforeAll(func() {
							helpers.UpdateControlPlaneMachineSetMachineNamePrefix(framework.GlobalFramework, resetPrefix)
							helpers.ModifyMachineProviderSpecToTriggerRollout(framework.GlobalFramework, 1)
						})

						helpers.ItShouldNotOnDeleteReplaceTheOutdatedMachine(1)

						// Machine name should follow general naming convention
						helpers.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(1)
					})
				})
			})
		})

		Context("and the ControlPlaneMachineSet is up to date", Ordered, func() {
			BeforeEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(framework.GlobalFramework)
			})

			Context("and the ControlPlaneMachineSet is deleted", func() {
				BeforeEach(func() {
					helpers.EnsureControlPlaneMachineSetDeleted(framework.GlobalFramework)
				})

				AfterEach(func() {
					helpers.EnsureActiveControlPlaneMachineSet(framework.GlobalFramework)
				})

				helpers.ItShouldUninstallTheControlPlaneMachineSet()
				helpers.ItShouldHaveTheControlPlaneMachineSetReplicasUpdated()

				Context("and the ControlPlaneMachineSet is reactivated", func() {
					BeforeEach(func() {
						helpers.EnsureControlPlaneMachineSetUpdated(framework.GlobalFramework)
						helpers.EnsureActiveControlPlaneMachineSet(framework.GlobalFramework)
					})

					helpers.ItShouldNotCauseARollout()
					helpers.ItShouldCheckAllControlPlaneMachinesHaveCorrectOwnerReferences()
				})
			})
		})

		Context("and a defaulted value is deleted from the ControlPlaneMachineSet", func() {
			var originalProviderSpec machinev1beta1.ProviderSpec
			BeforeEach(func() {
				// There is no defaulting webhook for the machines running on the following platforms.
				switch framework.GlobalFramework.GetPlatformType() {
				case configv1.OpenStackPlatformType:
					Skip("Skipping test on OpenStack platform")
				}

				_ = helpers.EnsureControlPlaneMachineSetUpdateStrategy(framework.GlobalFramework, machinev1.RollingUpdate)
				originalProviderSpec = helpers.UpdateDefaultedValueFromControlPlaneMachineSetProviderConfig(framework.GlobalFramework)
			})

			AfterEach(func() {
				helpers.EnsureActiveControlPlaneMachineSet(framework.GlobalFramework)
				helpers.UpdateControlPlaneMachineSetProviderSpec(framework.GlobalFramework, originalProviderSpec)
			})

			helpers.ItShouldNotCauseARollout()
		})
	})

	Context("With an inactive ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			helpers.EnsureInactiveControlPlaneMachineSet(framework.GlobalFramework)
		})

		Context("and the ControlPlaneMachineSet is up to date", func() {
			BeforeEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(framework.GlobalFramework)
			})

			AfterEach(func() {
				helpers.EnsureControlPlaneMachineSetUpdated(framework.GlobalFramework)
			})

			Context("and there is diff in the providerSpec of the newest, alphabetically last machine", func() {
				var opts helpers.ControlPlaneMachineSetRegenerationTestOptions

				BeforeEach(func() {
					opts.TestFramework = framework.GlobalFramework
					opts.UID = helpers.GetControlPlaneMachineSetUID(framework.GlobalFramework)
					opts.Index, opts.OriginalProviderSpec, opts.UpdatedProviderSpec = helpers.ModifyNewestMachineProviderSpecToTriggerRollout(framework.GlobalFramework)
				})

				AfterEach(func() {
					helpers.UpdateControlPlaneMachineProviderSpec(framework.GlobalFramework, opts.Index, opts.OriginalProviderSpec)
				})

				helpers.ItShouldPerformControlPlaneMachineSetRegeneration(&opts)
			})
		})
	})
})
