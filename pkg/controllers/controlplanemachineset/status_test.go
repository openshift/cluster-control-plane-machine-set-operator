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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Status", func() {
	Context("reconcileStatusWithMachineInfo", func() {
		type reconcileStatusTableInput struct {
			cpms           *machinev1.ControlPlaneMachineSet
			machineInfos   []machineproviders.MachineInfo
			expectedError  error
			expectedStatus machinev1.ControlPlaneMachineSetStatus
			expectedLogs   []test.LogEntry
		}

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

		missingMachineBuilder := resourcebuilder.MachineInfo().
			WithNodeGVR(nodeGVR).
			WithReady(false).
			WithNeedsUpdate(false)

		// If a node is removed from the cloud provider, the machine should report an error
		// and the error should be propogated to the MachineInfo so that the controller
		// can handle this error state.
		missingNodeBuilder := pendingMachineBuilder.
			WithErrorMessage("node removed from cloud provider")

		DescribeTable("correctly sets the status based on the machine info", func(in *reconcileStatusTableInput) {
			logger := test.NewTestLogger()
			cpms := in.cpms.DeepCopy()

			err := reconcileStatusWithMachineInfo(logger.Logger(), cpms, in.machineInfos)
			if in.expectedError != nil {
				Expect(err).To(MatchError(ContainSubstring(in.expectedError.Error())))
				return
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(cpms.Status.Conditions).To(test.MatchConditions(in.expectedStatus.Conditions))
			Expect(cpms.Status.ObservedGeneration).To(Equal(in.expectedStatus.ObservedGeneration))
			Expect(cpms.Status.Replicas).To(Equal(in.expectedStatus.Replicas))
			Expect(cpms.Status.ReadyReplicas).To(Equal(in.expectedStatus.ReadyReplicas))
			Expect(cpms.Status.UpdatedReplicas).To(Equal(in.expectedStatus.UpdatedReplicas))
			Expect(cpms.Status.UnavailableReplicas).To(Equal(in.expectedStatus.UnavailableReplicas))
		},
			PEntry("with up to date Machines", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(1).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionTrue,
							Reason:             reasonAllReplicasAvailable,
							ObservedGeneration: 1,
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 1,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAllReplicasUpdated,
							ObservedGeneration: 1,
						},
					},
					ObservedGeneration:  1,
					Replicas:            3,
					ReadyReplicas:       3,
					UpdatedReplicas:     3,
					UnavailableReplicas: 0,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "1",
							"Replicas", "3",
							"ReadyReplicas", "3",
							"UpdatedReplicas", "3",
							"UnavailableReplicas", "0",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("when Machines need updates", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(2).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithNeedsUpdate(true).Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionTrue,
							Reason:             reasonAllReplicasAvailable,
							ObservedGeneration: 2,
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 2,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonNeedsUpdateReplicas,
							ObservedGeneration: 2,
							Message:            "Observed 2 replica(s) in need of update",
						},
					},
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UpdatedReplicas:     1,
					UnavailableReplicas: 0,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "2",
							"Replicas", "3",
							"ReadyReplicas", "3",
							"UpdatedReplicas", "1",
							"UnavailableReplicas", "0",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("with pending replacement replicas", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(3).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithNeedsUpdate(true).Build(),
					pendingMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").Build(),
					pendingMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionTrue,
							Reason:             reasonAllReplicasAvailable,
							ObservedGeneration: 3,
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 3,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonNeedsUpdateReplicas,
							ObservedGeneration: 3,
							Message:            "Observed 2 replica(s) in need of update",
						},
					},
					ObservedGeneration:  3,
					Replicas:            5,
					ReadyReplicas:       3,
					UpdatedReplicas:     1,
					UnavailableReplicas: 0,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "3",
							"Replicas", "5",
							"ReadyReplicas", "3",
							"UpdatedReplicas", "1",
							"UnavailableReplicas", "0",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("with ready replacement replicas", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(4).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("node-2").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					healthyMachineBuilder.WithIndex(2).WithMachineName("machine-replacement-2").WithNodeName("node-replacement-2").Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionTrue,
							Reason:             reasonAllReplicasAvailable,
							ObservedGeneration: 4,
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 4,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonExcessReplicas,
							ObservedGeneration: 4,
							Message:            "Waiting for 2 old replica(s) to be removed",
						},
					},
					ObservedGeneration:  4,
					Replicas:            5,
					ReadyReplicas:       5,
					UpdatedReplicas:     3,
					UnavailableReplicas: 0,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "4",
							"Replicas", "5",
							"ReadyReplicas", "5",
							"UpdatedReplicas", "3",
							"UnavailableReplicas", "0",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("with no managed Nodes", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(5).Build(),
				machineInfos: []machineproviders.MachineInfo{
					missingMachineBuilder.WithIndex(0).WithNodeName("node-0").Build(),
					missingMachineBuilder.WithIndex(1).WithNodeName("node-1").Build(),
					missingMachineBuilder.WithIndex(2).WithNodeName("node-2").Build(),
				},
				expectedError: errors.New("found unmanaged control plane nodes, the following node(s) do not have associated machines: node-0, node-1, node-2"),
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							Reason:             reasonUnavailableReplicas,
							ObservedGeneration: 5,
							Message:            "Missing 3 available replica(s)",
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionTrue,
							Reason:             reasonUnmanagedNodes,
							ObservedGeneration: 5,
							Message:            "Found 3 unmanaged node(s)",
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionFalse,
							Reason:             reasonOperatorDegraded,
							ObservedGeneration: 5,
						},
					},
					ObservedGeneration:  5,
					Replicas:            0,
					ReadyReplicas:       0,
					UpdatedReplicas:     0,
					UnavailableReplicas: 3,
				},
				expectedLogs: []test.LogEntry{
					{
						Error: errors.New("found unmanaged control plane nodes, the following node(s) do not have associated machines: node-0, node-1, node-2"),
						KeysAndValues: []interface{}{
							"UnmanagedNodes", "node-0,node-1,node-2",
						},
						Message: "Observed unmanaged control plane nodes",
					},
				},
			}),
			PEntry("with an unmanaged Node", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(6).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					missingMachineBuilder.WithIndex(2).WithNodeName("node-2").Build(),
				},
				expectedError: errors.New("found unmanaged control plane nodes, the following node(s) do not have associated machines: node-2"),
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							Reason:             reasonUnavailableReplicas,
							ObservedGeneration: 6,
							Message:            "Missing 1 available replica(s)",
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionTrue,
							Reason:             reasonUnmanagedNodes,
							ObservedGeneration: 6,
							Message:            "Found 1 unmanaged node(s)",
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionFalse,
							Reason:             reasonOperatorDegraded,
							ObservedGeneration: 6,
						},
					},
					ObservedGeneration:  6,
					Replicas:            2,
					ReadyReplicas:       2,
					UpdatedReplicas:     2,
					UnavailableReplicas: 1,
				},
				expectedLogs: []test.LogEntry{
					{
						Error: errors.New("found unmanaged control plane nodes, the following node(s) do not have associated machines: node-2"),
						KeysAndValues: []interface{}{
							"UnmanagedNodes", "node-2",
						},
						Message: "Observed unmanaged control plane nodes",
					},
				},
			}),
			PEntry("with an unhealthy Machine", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(7).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					missingNodeBuilder.WithIndex(2).WithMachineName("machine-2").Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							Reason:             reasonUnavailableReplicas,
							ObservedGeneration: 7,
							Message:            "Missing 1 available replica(s)",
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 7,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonNeedsUpdateReplicas,
							ObservedGeneration: 7,
							Message:            "Observed 1 replica(s) in need of update",
						},
					},
					ObservedGeneration:  7,
					Replicas:            3,
					ReadyReplicas:       2,
					UpdatedReplicas:     2,
					UnavailableReplicas: 1,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "7",
							"Replicas", "3",
							"ReadyReplicas", "2",
							"UpdatedReplicas", "2",
							"UnavailableReplicas", "1",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("with an unhealthy index (failure domain)", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(8).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").WithNeedsUpdate(true).Build(),
					missingNodeBuilder.WithIndex(2).WithMachineName("machine-2").WithNeedsUpdate(true).Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithNodeName("node-replacement-1").Build(),
					missingNodeBuilder.WithIndex(2).WithMachineName("machine-replacement-2").Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							Reason:             reasonUnavailableReplicas,
							ObservedGeneration: 8,
							Message:            "Missing 1 available replica(s)",
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 8,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonNeedsUpdateReplicas,
							ObservedGeneration: 8,
							Message:            "Observed 1 replica(s) in need of update",
						},
					},
					ObservedGeneration:  4,
					Replicas:            5,
					ReadyReplicas:       5,
					UpdatedReplicas:     3,
					UnavailableReplicas: 0,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "8",
							"Replicas", "5",
							"ReadyReplicas", "3",
							"UpdatedReplicas", "2",
							"UnavailableReplicas", "1",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
			PEntry("with a missing index", &reconcileStatusTableInput{
				cpms: resourcebuilder.ControlPlaneMachineSet().WithGeneration(9).Build(),
				machineInfos: []machineproviders.MachineInfo{
					healthyMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("node-0").Build(),
					healthyMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("node-1").Build(),
					resourcebuilder.MachineInfo().WithIndex(2).Build(),
				},
				expectedError: nil,
				expectedStatus: machinev1.ControlPlaneMachineSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							Reason:             reasonUnavailableReplicas,
							ObservedGeneration: 8,
							Message:            "Missing 1 available replica(s)",
						},
						{
							Type:               conditionDegraded,
							Status:             metav1.ConditionFalse,
							Reason:             reasonAsExpected,
							ObservedGeneration: 8,
						},
						{
							Type:               conditionProgressing,
							Status:             metav1.ConditionTrue,
							Reason:             reasonNeedsUpdateReplicas,
							ObservedGeneration: 8,
							Message:            "Observed 1 replica(s) in need of update",
						},
					},
					ObservedGeneration:  9,
					Replicas:            2,
					ReadyReplicas:       2,
					UpdatedReplicas:     2,
					UnavailableReplicas: 1,
				},
				expectedLogs: []test.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"ObservedGeneration", "7",
							"Replicas", "2",
							"ReadyReplicas", "2",
							"UpdatedReplicas", "2",
							"UnavailableReplicas", "1",
						},
						Message: "Observed Machine Configuration",
					},
				},
			}),
		)
	})
})
