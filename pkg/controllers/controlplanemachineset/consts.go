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

const (
	// masterNodeRoleLabel denotes the master node label for a node.
	masterNodeRoleLabel = "node-role.kubernetes.io/master"
	// controlPlaneNodeRoleLabel denotes the control-plane node label for a node.
	controlPlaneNodeRoleLabel = "node-role.kubernetes.io/control-plane"
)

// Condition types for use in the ControlPlaneMachineSet status.
// These types will define the output of the ContorlPlaneMachineSet status
// as conditions which in turn will influence how the ClusterOperator
// reports the status of the operator.
const (
	// conditionAvailable is used to denote whether the ControlPlaneMachineSet
	// has an available Control Plane.
	// This condition should be true only when each Control Plane Machine index
	// has an ready Machine.
	// If any index is missing or does not have a ready Machine, this condition
	// should be marked false with an appropriate reason and message.
	conditionAvailable = "Available"

	// conditionDegraded is used to denote when the ControlPlaneMachineSet is
	// unable to operate in the manner expected of it. For example, if there are
	// issues creating new Machines or an unexpected state that it doesn't know
	// how to handle, the operator should set this condition to true and add
	// detail with a reason and appropriate message.
	conditionDegraded = "Degraded"

	// conditionProgressing is used to denote when the ControlPlaneMachineSet is
	// going through the process of making updates to the Machines within its
	// management. Typically this condition is expected to be false.
	// This condition may be false with a reason, such as when an update is needed
	// but the rollout strategy is configured to OnDelete.
	conditionProgressing = "Progressing"
)

// Condition reasons for use in the ControlPlaneMachineSet status.
// These reasons reflect the different states of the condition types defined above.
const (
	// BEGIN: Available reasons.

	// reasonAllReplicasAvailable denotes that all of the expected replicas of the
	// Control Plane are reporting as available.
	reasonAllReplicasAvailable = "AllReplicasAvailable"

	// reasonUnavailableReplicas denotes that at least one of the expected replicas
	// of the Control Plane is reporting as unavailable.
	reasonUnavailableReplicas = "UnavailableReplicas"

	// END: Available reasons.

	// BEGIN: Degraded reasons.

	// reasonAsExpected denotes that the ControlPlaneMachineSet is operating as expected
	// and is not currently encountering any issues in operation.
	reasonAsExpected = "AsExpected"

	// reasonFailedReplacement denotes that the ControlPlaneMachineSet has identified that
	// a replacement machines is in an error state. In this case, operations must not
	// continue and manual intervention is required to identify and rectify the cause
	// of the issue.
	reasonFailedReplacement = "FailedReplacement"

	// reasonInvalidStrategy denotes that the ControlPlaneMachineSet has identified an
	// invalid value for the spec.strategy.type field.
	// This must be resolved by the user before operation of the ControlPlaneMachineSet
	// can continue.
	reasonInvalidStrategy = "InvalidStrategy"

	// reasonMachinesAlreadyOwned denotes that the ControlPlaneMachineSet has identified
	// some Control Plane Machines that are already owned by a different controller.
	// In this scenario, the operator must cease operations to prevent possible conflicts
	// with the other controller.
	reasonMachinesAlreadyOwned = "MachinesAlreadyOwned"

	// reasonNoReadyMachines denotes that the ControlPlaneMachineSet has
	// not found any Control Plane Machines that are ready. This normally means that the
	// cluster is not functioning properly, likely due to some misconfiguration of the
	// Control Plane Machines.
	reasonNoReadyMachines = "NoReadyMachines"

	// reasonUnmanagedNodes denotes that the ControlPlaneMachineSet has identified some
	// Control Plane Node that is not currently managed by a Control Plane Machine.
	// In this scenario, to prevent potential for degrading the cluster into an unsupported
	// configuration, the ControlPlaneMachineSet will cease all operations.
	reasonUnmanagedNodes = "UnmanagedNodes"

	// reasonExcessIndexes denotes that the ControlPlaneMachineSet has more indexes
	// than desired.
	// This will typically occur when extra indexes have been created outside of the cpms.
	// In this scenario, to prevent potential for degrading the cluster into an unsupported
	// configuration, the ControlPlaneMachineSet will cease all operations.
	reasonExcessIndexes = "ExcessIndexes"

	// END: Degraded reasons.

	// BEGIN: Progressing reasons.

	// reasonAllReplicasUpdated denotes that the ControlPlaneMachineSet is operating as
	// expected and that all replicas of the Contorl Plane Machines are up to date with
	// the latest version of the desired configuration.
	reasonAllReplicasUpdated = "AllReplicasUpdated"

	// reasonsOperatorDegraded denotes that the ControlPlaneMachineSet is not taking any
	// action towards a rollout because the operator is currently in a degraded state.
	reasonOperatorDegraded = "OperatorDegraded"

	// reasonExcessReplicas denotes that the ControlPlaneMachineSet has the correct number
	// of ready and updated replicas, however, has more replicas than expected.
	// This will typically occur when an old replica has not yet been removed.
	reasonExcessReplicas = "ExcessReplicas"

	// reasonNeedsUpdateReplicas denotes that the ControlPlaneMachineSet has identified
	// replicas under its management that are currently in need of an update.
	reasonNeedsUpdateReplicas = "NeedsUpdateReplicas"

	// END: Progressing reasons.
)
