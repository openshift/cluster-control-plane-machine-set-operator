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

package providers

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("MachineProviders", func() {
	Context("NewMachineProvider", func() {
		var cpmsBuilder resourcebuilder.ControlPlaneMachineSetBuilder
		var logger test.TestLogger
		var namespaceName string

		const invalidCPMSType = machinev1.ControlPlaneMachineSetMachineType("invalid")

		BeforeEach(func() {
			By("Setting up a namespace for the test")
			ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			namespaceName = ns.GetName()

			cpmsBuilder = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName)

			logger = test.NewTestLogger()
		})

		AfterEach(func() {
			test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
				&machinev1beta1.Machine{},
			)
		})

		Context("With an invalid machine type", func() {
			var provider machineproviders.MachineProvider
			var err error

			BeforeEach(func() {
				cpms := cpmsBuilder.Build()
				cpms.Spec.Template.MachineType = invalidCPMSType

				provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpms)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(fmt.Errorf("%w: %s", errUnexpectedMachineType, invalidCPMSType)))
			})

			It("returns a nil machine provider", func() {
				Expect(provider).To(BeNil())
			})

			It("does not log", func() {
				Expect(logger.Entries()).To(BeEmpty())
			})
		})

		Context("With an OpenShift Machine v1beta1 machine type", func() {
			BeforeEach(func() {
				cpmsBuilder = cpmsBuilder.WithMachineTemplateBuilder(
					resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(
						resourcebuilder.AWSProviderSpec(),
					),
				)

				// We create a happy path so that the construction is successful.
				// More detailed error cases will happen in the machine provider tests themselves.
				By("Creating some master machines")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				for i := 0; i < 3; i++ {
					machine := machineBuilder.WithName(fmt.Sprintf("master-%d", i)).Build()
					Expect(k8sClient.Create(ctx, machine)).To(Succeed())
				}
			})

			Context("with a valid template", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpmsBuilder.Build())
				})

				PIt("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				PIt("returns an OpenShift Machine v1beta1 implementation of the machine provider", func() {
					typ := reflect.TypeOf(provider)
					Expect(typ.String()).To(Equal("*v1beta1.openshiftMachineProvider"))
				})

				PIt("logs based on the machine info mappings", func() {
					Expect(logger.Entries()).To(ConsistOf(
						test.LogEntry{Message: "TODO"},
					))
				})
			})

			Context("with an invalid template", func() {
				var provider machineproviders.MachineProvider
				var err error

				BeforeEach(func() {
					cpms := cpmsBuilder.Build()
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine = nil

					provider, err = NewMachineProvider(ctx, logger.Logger(), k8sClient, cpms)
				})

				It("returns an error", func() {
					Expect(err).To(MatchError("error constructing machines_v1beta1_machine_openshift_io machine provider: cannot initialise machines_v1beta1_machine_openshift_io provider with empty config"))
				})

				It("returns a nil machine provider", func() {
					Expect(provider).To(BeNil())
				})

				It("does not log", func() {
					Expect(logger.Entries()).To(BeEmpty())
				})
			})
		})
	})
})
