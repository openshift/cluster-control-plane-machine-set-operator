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
	"errors"
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/mock"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"
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

	// transientError is used to mimic a temporary failure from the MachineProvider.
	transientError := errors.New("transient error")

	// BEGIN: MachineInfo builders for various types of MachineInfo inputs

	machineGVR := machinev1beta1.GroupVersion.WithResource("machines")
	nodeGVR := corev1.SchemeGroupVersion.WithResource("nodes")

	healthyMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithNodeGVR(nodeGVR).
		WithReady(true).
		WithNeedsUpdate(false)

	pendingMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithReady(false).
		WithNeedsUpdate(false)

	// END: MachineInfo builders

	Context("When the update strategy is RollingUpdate", func() {
		BeforeEach(func() {
			cpms.Spec.Strategy.Type = machinev1.RollingUpdate
		})

		type rollingUpdateTableInput struct {
			machineInfos   []machineproviders.MachineInfo
			setupMock      func()
			expectedError  error
			expectedResult ctrl.Result
			expectedLogs   []test.LogEntry
		}

		DescribeTable("should implement the update strategy based on the MachineInfo", func(in rollingUpdateTableInput) {
			// We setup the mock machine provider on each test with the expected assertions.
			in.setupMock()

			originalCPMS := cpms.DeepCopy()

			result, err := reconciler.reconcileMachineUpdates(ctx, logger.Logger(), cpms, mockMachineProvider, in.machineInfos)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(result).To(Equal(in.expectedResult))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			PEntry("with no updates required", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
						},
						Message: noUpdatesRequired,
					},
				},
			}),
			PEntry("with updates required in a single index", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(1).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
						},
						Message: createdReplacement,
					},
				},
			}),
			PEntry("with updates required in a single index, and an error occurs", rollingUpdateTableInput{
				expectedError: fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(1).Return(transientError).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Error: fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError),
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
						},
						Message: errorCreatingMachine,
					},
				},
			}),
			PEntry("with updates required in a single index, but the replacement machine is pending", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
							"ReplacementName", "machine-replacement-1",
						},
						Message: waitingForReplacement,
					},
				},
			}),
			PEntry("with updates required in a single index, and the replacement machine is ready", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(machineInfo.MachineRef).Return(nil).Times(1)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
						},
						Message: removingOldMachine,
					},
				},
			}),
			PEntry("with updates required in a single index, and the replacement machine is ready, and an error occurs", rollingUpdateTableInput{
				expectedError: fmt.Errorf("error deleting Machine %s/%s: %w", namespaceName, "machine-1", transientError),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(machineInfo.MachineRef).Return(transientError).Times(1)
				},
				expectedLogs: []test.LogEntry{
					{
						Error: fmt.Errorf("error deleting Machine %s/%s: %w", namespaceName, "machine-1", transientError),
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
						},
						Message: errorDeletingMachine,
					},
				},
			}),
			PEntry("with updates required in a single index, and the replacement machine is ready, and the old machine is already deleted", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 1,
							"Namespace", namespaceName,
							"Name", "machine-1",
						},
						Message: waitingForRemoved,
					},
				},
			}),
			PEntry("with updates are required in multiple indexes", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					// Note, in this case it should only create a single machine.
					mockMachineProvider.EXPECT().CreateMachine(0).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 0,
							"Namespace", namespaceName,
							"Name", "machine-0",
						},
						Message: createdReplacement,
					},
				},
			}),
			PEntry("with updates are required in multiple indexes, but the replacement machine is pending", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
					pendingMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					// Note, in this case it should not create anything new because we are at surge capacity.
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 0,
							"Namespace", namespaceName,
							"Name", "machine-0",
							"ReplacementName", "machine-replacement-0",
						},
						Message: waitingForReplacement,
					},
				},
			}),
			PEntry("with updates are required in multiple indexes, and the replacement machine is ready", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					// Note, in this case, it should wait for the old Machine to go away before starting a new update.
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(machineInfo.MachineRef).Return(nil).Times(1)
				},

				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 0,
							"Namespace", namespaceName,
							"Name", "machine-0",
						},
						Message: removingOldMachine,
					},
				},
			}),
			PEntry("with updates required in multiple indexes, and the replacement machine is ready, and the old machine is already deleted", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 0,
							"Namespace", namespaceName,
							"Name", "machine-0",
						},
						Message: waitingForRemoved,
					},
				},
			}),
			PEntry("with a missing index", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					resourcebuilder.MachineInfo().WithIndex(2).Build(), // This is an empty Machine info to show the index is missing.
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(2).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 2,
							"Namespace", namespaceName,
							"Name", "<Unknown>",
						},
						Message: createdReplacement,
					},
				},
			}),
			PEntry("with a pending machine in an index", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 2,
							"Namespace", namespaceName,
							"Name", "machine-replacement-2",
						},
						Message: waitingForReady,
					},
				},
			}),
			PEntry("with a missing index, and other indexes needing updates", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					resourcebuilder.MachineInfo().WithIndex(2).Build(), // This is an empty Machine info to show the index is missing.
				},
				setupMock: func() {
					// The missing index should take priority over the index in need of an update.
					mockMachineProvider.EXPECT().CreateMachine(2).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 2,
							"Namespace", namespaceName,
							"Name", "<Unknown>",
						},
						Message: createdReplacement,
					},
				},
			}),
			PEntry("with a pending machine in an index, and other indexes needing updates", rollingUpdateTableInput{
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build(),
				},
				setupMock: func() {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any()).Times(0)
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 2,
						KeysAndValues: []interface{}{
							"UpdateStrategy", machinev1.RollingUpdate,
							"Index", 2,
							"Namespace", namespaceName,
							"Name", "machine-replacement-2",
						},
						Message: waitingForReady,
					},
				},
			}),
		)
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
