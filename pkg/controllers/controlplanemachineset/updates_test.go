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

package controlplanemachineset

import (
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/mock"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("reconcileMachineUpdates", func() {
	var namespaceName string
	var logger test.TestLogger
	var reconciler *ControlPlaneMachineSetReconciler
	var cpms *machinev1.ControlPlaneMachineSet
	var machineInfos []machineproviders.MachineInfo

	var mockCtrl *gomock.Controller
	var mockMachineProvider *mock.MockMachineProvider

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-cluster-operator-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up the reconciler")
		logger = test.NewTestLogger()
		reconciler = &ControlPlaneMachineSetReconciler{
			Namespace: namespaceName,
			Scheme:    testScheme,
		}

		By("Setting up supporting resources")
		cpms = resourcebuilder.ControlPlaneMachineSet().Build()

		mockCtrl = gomock.NewController(GinkgoT())
		mockMachineProvider = mock.NewMockMachineProvider(mockCtrl)

		machineInfos = []machineproviders.MachineInfo{}
	})

	Context("When the update strategy is RollingUpdate", func() {
		BeforeEach(func() {
			cpms.Spec.Strategy.Type = machinev1.RollingUpdate
		})
	})

	Context("When the update strategy is OnDelete", func() {
		BeforeEach(func() {
			cpms.Spec.Strategy.Type = machinev1.OnDelete
		})
	})

	Context("When the update strategy is Recreate", func() {
		var result ctrl.Result
		var err error

		BeforeEach(func() {
			cpms.Spec.Strategy.Type = machinev1.Recreate

			result, err = reconciler.reconcileMachineUpdates(ctx, logger.Logger(), cpms, mockMachineProvider, machineInfos)
		})

		It("Returns an empty result", func() {
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Does not return an error", func() {
			Expect(err).ToNot(HaveOccurred(), "This is a terminal error, returning an error would force a requeue which is not desired")
		})

		It("Logs that the strategy is invalid", func() {
			Expect(logger.Entries()).To(ConsistOf(test.LogEntry{
				Error:   errRecreateStrategyNotSupported,
				Message: invalidStrategyMessage,
			}))
		})

		PIt("Sets the degraded condition", func() {
			Expect(cpms.Status.Conditions).To(ConsistOf(test.MatchCondition(metav1.Condition{
				Type:    conditionDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonInvalidStrategy,
				Message: fmt.Sprintf("%s: %s", invalidStrategyMessage, errRecreateStrategyNotSupported),
			})))
		})
	})

	Context("When the update strategy is invalid", func() {
		var result ctrl.Result
		var err error

		BeforeEach(func() {
			cpms.Spec.Strategy.Type = machinev1.ControlPlaneMachineSetStrategyType("invalid")

			result, err = reconciler.reconcileMachineUpdates(ctx, logger.Logger(), cpms, mockMachineProvider, machineInfos)
		})

		It("Returns an empty result", func() {
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Does not return an error", func() {
			Expect(err).ToNot(HaveOccurred(), "This is a terminal error, returning an error would force a requeue which is not desired")
		})

		It("Logs that the strategy is invalid", func() {
			Expect(logger.Entries()).To(ConsistOf(test.LogEntry{
				Error:   fmt.Errorf("%w: %s", errUnknownStrategy, "invalid"),
				Message: invalidStrategyMessage,
			}))
		})

		PIt("Sets the degraded condition", func() {
			Expect(cpms.Status.Conditions).To(ConsistOf(test.MatchCondition(metav1.Condition{
				Type:    conditionDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonInvalidStrategy,
				Message: fmt.Sprintf("%s: %s: %s", invalidStrategyMessage, errUnknownStrategy, "invalid"),
			})))
		})
	})
})

var _ = Describe("machineInfosByIndex", func() {
	i0m0 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-0").Build()
	i0m1 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-1").Build()
	i0m2 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-2").Build()
	i1m0 := resourcebuilder.MachineInfo().WithIndex(1).WithMachineName("machine-1-0").Build()
	i1m1 := resourcebuilder.MachineInfo().WithIndex(1).WithMachineName("machine-1-1").Build()
	i2m0 := resourcebuilder.MachineInfo().WithIndex(2).WithMachineName("machine-2-0").Build()

	type tableInput struct {
		in       []machineproviders.MachineInfo
		expected map[int32][]machineproviders.MachineInfo
	}

	DescribeTable("should sort Machine Infos by index", func(in tableInput) {
		out := machineInfosByIndex(in.in)

		Expect(out).To(HaveLen(len(in.expected)))
		// Check each key and its values separately to avoid ordering within the lists
		// from causing issues.
		for key, values := range in.expected {
			Expect(out).To(HaveKeyWithValue(key, ConsistOf(values)))
		}
	},
		Entry("no input", tableInput{
			in:       []machineproviders.MachineInfo{},
			expected: map[int32][]machineproviders.MachineInfo{},
		}),
		PEntry("separately indexed machines", tableInput{
			in: []machineproviders.MachineInfo{i0m0, i1m0, i2m0},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0},
				1: {i1m0},
				2: {i2m0},
			},
		}),
		PEntry("a mixture of indexed machines", tableInput{
			in: []machineproviders.MachineInfo{i0m0, i1m0, i2m0, i0m1, i1m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
				1: {i1m0, i1m1},
				2: {i2m0},
			},
		}),
		PEntry("all machines in the same index", tableInput{
			in: []machineproviders.MachineInfo{i0m0, i0m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
			},
		}),
	)
})
