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
	var cpmsBuilder resourcebuilder.ControlPlaneMachineSetBuilder

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
		cpmsBuilder = resourcebuilder.ControlPlaneMachineSet()

		mockCtrl = gomock.NewController(GinkgoT())
		mockMachineProvider = mock.NewMockMachineProvider(mockCtrl)
	})

	// transientError is used to mimic a temporary failure from the MachineProvider.
	transientError := errors.New("transient error")

	// BEGIN: MachineInfo builders for various types of MachineInfo inputs

	machineGVR := machinev1beta1.GroupVersion.WithResource("machines")
	nodeGVR := corev1.SchemeGroupVersion.WithResource("nodes")

	updatedMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithNodeGVR(nodeGVR).
		WithReady(true).
		WithNeedsUpdate(false)

	pendingMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithReady(false).
		WithNeedsUpdate(false)

	outdatedNonReadyMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithReady(false).
		WithNeedsUpdate(true)
	// END: MachineInfo builders

	Context("When the update strategy is RollingUpdate", func() {
		BeforeEach(func() {
			cpmsBuilder = cpmsBuilder.WithStrategyType(machinev1.RollingUpdate)
		})

		type rollingUpdateTableInput struct {
			cpmsBuilder          resourcebuilder.ControlPlaneMachineSetInterface
			machineInfos         map[int32][]machineproviders.MachineInfo
			setupMock            func(machineInfos map[int32][]machineproviders.MachineInfo)
			expectedErrorBuilder func() error
			expectedResult       ctrl.Result
			expectedLogsBuilder  func() []test.LogEntry
		}

		DescribeTable("should implement the update strategy based on the MachineInfo", func(in rollingUpdateTableInput) {
			// We setup the mock machine provider on each test with the expected assertions.
			in.setupMock(in.machineInfos)

			var errExpected error
			cpms := cpmsBuilder.Build()
			originalCPMS := cpms.DeepCopy()
			if in.expectedErrorBuilder != nil {
				errExpected = in.expectedErrorBuilder()
			}

			result, err := reconciler.reconcileMachineUpdates(ctx, logger.Logger(), cpms, mockMachineProvider, in.machineInfos)
			if errExpected != nil {
				Expect(err).To(MatchError(errExpected))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(result).To(Equal(in.expectedResult))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogsBuilder()))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			Entry("with no updates required", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 4,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
							},
							Message: noUpdatesRequired,
						},
					}
				},
			}),
			Entry("with updates required in a single index", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with updates required in a single index, and an error occurs", rollingUpdateTableInput{
				cpmsBuilder:          cpmsBuilder.WithReplicas(3),
				expectedErrorBuilder: func() error { return fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError) },
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(transientError).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Error: fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError),
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: errorCreatingMachine,
						},
					}
				},
			}),
			Entry("with updates required in a single index, but the replacement machine is pending", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
						pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.RollingUpdate,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
							"replacementName", "machine-replacement-1",
						},
						Message: waitingForReplacement,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and the replacement machine is ready", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo.MachineRef).Return(nil).Times(1)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: removingOldMachine,
						},
					}
				},
			}),
			Entry("with updates required in a single index, and the replacement machine is ready, and an error occurs", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				expectedErrorBuilder: func() error {
					return fmt.Errorf("error deleting Machine %s/%s: %w", namespaceName, "machine-1", transientError)
				},
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo.MachineRef).Return(transientError).Times(1)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Error: fmt.Errorf("error deleting Machine %s/%s: %w", namespaceName, "machine-1", transientError),
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: errorDeletingMachine,
						},
					}
				},
			}),
			Entry("with updates required in a single index, and the replacement machine is ready, and the old machine is already deleted", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.RollingUpdate,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: waitingForRemoved,
					},
					}
				},
			}),
			Entry("with updates are required in multiple indexes", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					// Note, in this case it should only create a single machine.
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(0)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: createdReplacement,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with updates are required in multiple indexes, but the replacement machine is pending", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
						pendingMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build(),
					},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					// Note, in this case it should not create anything new because we are at surge capacity.
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
								"replacementName", "machine-replacement-0",
							},
							Message: waitingForReplacement,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with updates are required in multiple indexes, and the replacement machine is ready", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					// Note, in this case, it should wait for the old Machine to go away before starting a new update.
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

					// We expect this particular machine to be called for deletion.
					machineInfo := updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo.MachineRef).Return(nil).Times(1)
				},

				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: removingOldMachine,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and the replacement machine is ready, and the old machine is already deleted", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: waitingForRemoved,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with an empty index", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.RollingUpdate,
							"index", int32(2),
							"namespace", namespaceName,
							"name", "<Unknown>",
						},
						Message: createdReplacement,
					},
					}
				},
			}),
			Entry("with a pending machine in an index", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.RollingUpdate,
							"index", int32(2),
							"namespace", namespaceName,
							"name", "machine-replacement-2",
						},
						Message: waitingForReady,
					},
					}
				},
			}),
			Entry("with a missing index, and other indexes needing updates", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					// The missing index should take priority over the index in need of an update.
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "<Unknown>",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with a pending machine in an index, and other indexes needing updates", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "machine-replacement-2",
							},
							Message: waitingForReady,
						},
					}
				},
			}),
			Entry("with a missing index, and other indexes needing updates, and their replacement machines are ready", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Times(0)
					// We expect this particular machine to be called for deletion.
					machineInfo0 := updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo0.MachineRef).Return(nil).Times(1)
					// We expect this particular machine to be called for deletion.
					machineInfo1 := updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo1.MachineRef).Return(nil).Times(1)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: removingOldMachine,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: removingOldMachine,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "<Unknown>",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with a missing index, and other indexes needing updates, and their replacement machines are ready, and one machine is already deleted", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					// We expect this particular machine to be called for deletion.
					machineInfo1 := updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo1.MachineRef).Return(nil).Times(1)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: waitingForRemoved,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: removingOldMachine,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "<Unknown>",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with an extra index, hitting maxSurge", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithNeedsUpdate(true).Build()},
					3: {updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("node-3").WithNeedsUpdate(true).Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: noCapacityForExpansion,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "machine-2",
							},
							Message: noCapacityForExpansion,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(3),
								"namespace", namespaceName,
								"name", "machine-3",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with an index not starting at zero, and no updates required", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
					3: {updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("node-3").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 4,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
							},
							Message: noUpdatesRequired,
						},
					}
				},
			}),
			Entry("with a working configuration, then a faulty configuration applied, and now rolled back", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						// first generation spec, outdated and ready
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build(),
						// second generation spec, outdated and broken replacement, never became ready, to be removed first
						outdatedNonReadyMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build(),
					},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithNeedsUpdate(true).Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					machineInfo0 := outdatedNonReadyMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build()
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), machineInfo0.MachineRef).Times(1)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-replacement-0",
							},
							Message: removingOldMachine,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(0),
								"namespace", namespaceName,
								"name", "machine-0",
							},
							Message: noCapacityForExpansion,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: noCapacityForExpansion,
						},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "machine-2",
							},
							Message: noCapacityForExpansion,
						},
					}
				},
			}),
			Entry("with no updates required, but a machine has been deleted", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						// We wouldn't normally continue operation when a Machine is pending removal, however,
						// when a user has manually deleted a Machine and the etcd deletion hook is present,
						// we still need to handle creating the replacement Machine to unblock the rollout.
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with no updates required, but a machine has been deleted, and its replacement is pending", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						// We wouldn't normally continue operation when a Machine is pending removal, however,
						// when a user has manually deleted a Machine and the etcd deletion hook is present,
						// we still need to handle creating the replacement Machine to unblock the rollout.
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
								"replacementName", "machine-replacement-1",
							},
							Message: waitingForReplacement,
						},
					}
				},
			}),
			Entry("with no updates required, but a machine has been deleted, and its replacement is ready", rollingUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.RollingUpdate,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: waitingForRemoved,
						},
					}
				},
			}),
		)
	})

	Context("When the update strategy is OnDelete", func() {
		BeforeEach(func() {
			cpmsBuilder = cpmsBuilder.WithStrategyType(machinev1.OnDelete)
		})

		type onDeleteUpdateTableInput struct {
			cpmsBuilder          resourcebuilder.ControlPlaneMachineSetInterface
			machineInfos         map[int32][]machineproviders.MachineInfo
			setupMock            func(machineInfos map[int32][]machineproviders.MachineInfo)
			expectedErrorBuilder func() error
			expectedResult       ctrl.Result
			expectedLogsBuilder  func() []test.LogEntry
		}

		DescribeTable("should implement the update strategy based on the MachineInfo", func(in onDeleteUpdateTableInput) {
			// We setup the mock machine provider on each test with the expected assertions.
			in.setupMock(in.machineInfos)

			cpms := cpmsBuilder.Build()
			originalCPMS := cpms.DeepCopy()

			result, err := reconciler.reconcileMachineUpdates(ctx, logger.Logger(), cpms, mockMachineProvider, in.machineInfos)
			if in.expectedErrorBuilder != nil {
				Expect(err).To(MatchError(in.expectedErrorBuilder()))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(result).To(Equal(in.expectedResult))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogsBuilder()))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			Entry("with no updates required", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 4,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
							},
							Message: noUpdatesRequired,
						},
					}
				},
			}),
			Entry("with updates required in a single index, and the machine is not yet deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: machineRequiresUpdate,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and the machine has been deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: createdReplacement,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and the machine has been deleted (with no standard machine indexes)", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					3: {updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("node-3").Build()},
					4: {updatedMachineBuilder.WithIndex(4).WithMachineName("machine-4").WithNodeName("node-4").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					5: {updatedMachineBuilder.WithIndex(5).WithMachineName("machine-5").WithNodeName("node-5").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(4)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(4),
							"namespace", namespaceName,
							"name", "machine-4",
						},
						Message: createdReplacement,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and the machine has been deleted, and an error occurrs", onDeleteUpdateTableInput{
				cpmsBuilder:          cpmsBuilder.WithReplicas(3),
				expectedErrorBuilder: func() error { return fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError) },
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(transientError).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Error: fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError),
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: errorCreatingMachine,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and replacement machine is pending", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(0)).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
							"replacementName", "machine-replacement-1",
						},
						Message: waitingForReplacement,
					},
					}
				},
			}),
			Entry("with updates required in a single index, and replacement machine is ready", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(0)).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: waitingForRemoved,
					},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and the machines are not yet deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: machineRequiresUpdate,
						},
					}
				},
			}),
			Entry("with updates required in a multiple indexes, and machine has been deleted, and an error occurrs", onDeleteUpdateTableInput{
				cpmsBuilder:          cpmsBuilder.WithReplicas(3),
				expectedErrorBuilder: func() error { return fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError) },
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(transientError).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: machineRequiresUpdate,
					},
						{
							Error: fmt.Errorf("error creating new Machine for index %d: %w", 1, transientError),
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: errorCreatingMachine,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and a machine has been deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and multiple machines have been deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(0)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(1)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: createdReplacement,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and a single machine has been deleted, and the replacement machine is pending", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
								"replacementName", "machine-replacement-1",
							},
							Message: waitingForReplacement,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and multiple machines have been deleted, and the replacement machines are pending", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build(),
					},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
							"replacementName", "machine-replacement-0",
						},
						Message: waitingForReplacement,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
								"replacementName", "machine-replacement-1",
							},
							Message: waitingForReplacement,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and a single machine has been deleted, and the replacement machine is ready", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).Build()},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: waitingForRemoved,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and multiple machines have been deleted, and a replacement machine is ready, and a replacement machine is pending", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						pendingMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").Build(),
					},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
							"replacementName", "machine-replacement-0",
						},
						Message: waitingForReplacement,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: waitingForRemoved,
						},
					}
				},
			}),
			Entry("with updates required in multiple indexes, and multiple machines have been deleted, and all replacement machines are ready", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithNodeName("node-replacement-0").Build(),
					},
					1: {
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).WithMachineDeletionTimestamp(metav1.Now()).Build(),
						updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(0),
							"namespace", namespaceName,
							"name", "machine-0",
						},
						Message: waitingForRemoved,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(1),
								"namespace", namespaceName,
								"name", "machine-1",
							},
							Message: waitingForRemoved,
						},
					}
				},
			}),
			Entry("with an empty index", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(2),
							"namespace", namespaceName,
							"name", "<Unknown>",
						},
						Message: createdReplacement,
					},
					}
				},
			}),
			Entry("with a pending machine in an index", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(2),
							"namespace", namespaceName,
							"name", "machine-replacement-2",
						},
						Message: waitingForReady,
					},
					}
				},
			}),
			Entry("with a missing index, and other indexes need updating", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Return(nil).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "<Unknown>",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
			Entry("with a pending machine in an index, and other indexes need updating", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build()},
					2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{{
						Level: 2,
						KeysAndValues: []interface{}{
							"updateStrategy", machinev1.OnDelete,
							"index", int32(1),
							"namespace", namespaceName,
							"name", "machine-1",
						},
						Message: machineRequiresUpdate,
					},
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "machine-replacement-2",
							},
							Message: waitingForReady,
						},
					}
				},
			}),
			Entry("with no updates required, but a Machine has been deleted", onDeleteUpdateTableInput{
				cpmsBuilder: cpmsBuilder.WithReplicas(3),
				machineInfos: map[int32][]machineproviders.MachineInfo{
					0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
					1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
					2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithMachineDeletionTimestamp(metav1.Now()).Build()},
				},
				setupMock: func(machineInfos map[int32][]machineproviders.MachineInfo) {
					mockMachineProvider.EXPECT().WithClient(gomock.Any()).Return(mockMachineProvider).AnyTimes()
					mockMachineProvider.EXPECT().GetMachineInfos(gomock.Any(), gomock.Any()).Return(machineInfosMaptoSlice(machineInfos), nil).AnyTimes()
					mockMachineProvider.EXPECT().CreateMachine(gomock.Any(), gomock.Any(), int32(2)).Times(1)
					mockMachineProvider.EXPECT().DeleteMachine(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				},
				expectedLogsBuilder: func() []test.LogEntry {
					return []test.LogEntry{
						{
							Level: 2,
							KeysAndValues: []interface{}{
								"updateStrategy", machinev1.OnDelete,
								"index", int32(2),
								"namespace", namespaceName,
								"name", "machine-2",
							},
							Message: createdReplacement,
						},
					}
				},
			}),
		)
	})

	Context("When the update strategy is Recreate", func() {
		var cpms *machinev1.ControlPlaneMachineSet
		var result ctrl.Result
		var err error

		BeforeEach(func() {
			cpms = cpmsBuilder.WithStrategyType(machinev1.Recreate).Build()

			machineInfos := map[int32][]machineproviders.MachineInfo{}

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

		It("Sets the degraded condition", func() {
			Expect(cpms.Status.Conditions).To(ConsistOf(test.MatchCondition(metav1.Condition{
				Type:    conditionDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonInvalidStrategy,
				Message: fmt.Sprintf("%s: %s", invalidStrategyMessage, errRecreateStrategyNotSupported),
			})))
		})
	})

	Context("When the update strategy is invalid", func() {
		var cpms *machinev1.ControlPlaneMachineSet
		var result ctrl.Result
		var err error

		BeforeEach(func() {
			cpms = cpmsBuilder.WithStrategyType(machinev1.ControlPlaneMachineSetStrategyType("invalid")).Build()

			machineInfos := map[int32][]machineproviders.MachineInfo{}

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

		It("Sets the degraded condition", func() {
			Expect(cpms.Status.Conditions).To(ConsistOf(test.MatchCondition(metav1.Condition{
				Type:    conditionDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonInvalidStrategy,
				Message: fmt.Sprintf("%s: %s: %s", invalidStrategyMessage, errUnknownStrategy, "invalid"),
			})))
		})
	})
})

var _ = Describe("utils tests", func() {
	machineGVR := machinev1beta1.GroupVersion.WithResource("machines")
	nodeGVR := corev1.SchemeGroupVersion.WithResource("nodes")
	updatedMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithNodeGVR(nodeGVR).
		WithReady(true).
		WithNeedsUpdate(false)

	DescribeTable("should convert an indexed map of MachineInfo into a slice sorted by index",
		func(input map[int32][]machineproviders.MachineInfo, expected []indexToMachineInfos) {
			output := sortMachineInfosByIndex(input)
			Expect(output).To(Equal(expected))
		},
		Entry("when index starts at 0",
			map[int32][]machineproviders.MachineInfo{
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
			},
			[]indexToMachineInfos{
				{index: 0, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()}},
				{index: 1, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()}},
				{index: 2, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()}},
			},
		),
		Entry("when index starts at 1",
			map[int32][]machineproviders.MachineInfo{
				3: {updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("node-3").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()},
			},
			[]indexToMachineInfos{
				{index: 1, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build()}},
				{index: 2, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()}},
				{index: 3, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("node-3").Build()}},
			},
		),
		Entry("when one index is missing",
			map[int32][]machineproviders.MachineInfo{
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()},
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()},
			},
			[]indexToMachineInfos{
				{index: 0, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build()}},
				{index: 2, machineInfos: []machineproviders.MachineInfo{updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build()}},
			},
		),
	)
})
