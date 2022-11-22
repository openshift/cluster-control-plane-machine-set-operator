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

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/common"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/presubmit"
)

var _ = Describe("ControlPlaneMachineSet Operator", framework.PreSubmit(), func() {
	BeforeEach(func() {
		common.EventuallyClusterOperatorsShouldStabilise(10*time.Minute, 10*time.Second)
	}, OncePerOrdered)

	Context("With an active ControlPlaneMachineSet", func() {
		BeforeEach(func() {
			common.EnsureActiveControlPlaneMachineSet(testFramework)
		}, OncePerOrdered)

		Context("and the instance type of index 1 is not as expected", func() {
			BeforeEach(func() {
				presubmit.IncreaseControlPlaneMachineInstanceSize(testFramework, 1)
			})

			presubmit.ItShouldRollingUpdateReplaceTheOutdatedMachine(testFramework, 1)
		})

		Context("with the OnDelete update strategy", func() {
			var originalStrategy machinev1.ControlPlaneMachineSetStrategyType

			BeforeEach(func() {
				originalStrategy = common.EnsureControlPlaneMachineSetUpdateStrategy(testFramework, machinev1.OnDelete)
			}, OncePerOrdered)

			AfterEach(func() {
				common.EnsureControlPlaneMachineSetUpdateStrategy(testFramework, originalStrategy)
			}, OncePerOrdered)

			Context("and the instance type of index 2 is not as expected", Ordered, func() {
				var originalProviderSpec machinev1beta1.ProviderSpec

				BeforeAll(func() {
					originalProviderSpec = presubmit.IncreaseControlPlaneMachineInstanceSize(testFramework, 2)
				})

				AfterAll(func() {
					presubmit.UpdateControlPlaneMachineProviderSpec(testFramework, 2, originalProviderSpec)
				})

				presubmit.ItShouldNotOnDeleteReplaceTheOutdatedMachine(testFramework, 2)

				presubmit.ItShouldOnDeleteReplaceTheOutDatedMachineWhenDeleted(testFramework, 2)

			})
		})

		Context("and the ControlPlaneMachineSet is up to date", Ordered, func() {
			BeforeEach(func() {
				common.EnsureControlPlaneMachineSetUpdated(testFramework)
			})

			Context("and the ControlPlaneMachineSet is deleted", func() {
				BeforeEach(func() {
					common.EnsureControlPlaneMachineSetDeleted(testFramework)
				})

				AfterEach(func() {
					common.EnsureActiveControlPlaneMachineSet(testFramework)
				})

				presubmit.ItShouldUninstallTheControlPlaneMachineSet(testFramework)
				presubmit.ItShouldHaveTheControlPlaneMachineSetReplicasUpdated(testFramework)
			})
		})
	})
})
