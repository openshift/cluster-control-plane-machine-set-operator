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

package machineproviders

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MachineInfo collates information about a Control Plane Machine and Node.
// This is used by the core of the ControlPlaneMachineSet controller to determine
// actions required to be taken on the Machines within its control.
type MachineInfo struct {
	// MachineRef is a reference to the Machine that is the subject of this MachineInfo.
	MachineRef *ObjectRef

	// NodeRef is a reference to the Node backing the Machine that is the subject of this MachineInfo.
	NodeRef *ObjectRef

	// Ready determines whether the Machine is marked as ready. When true, this means the Machine is up and running, a
	// Node has joined the cluster and is operating as expected.
	Ready bool

	// NeedsUpdate is set true when the existing spec of the Machine does not match the desired spec of the Machine.
	// This is used to inform the controller about decisions related to rolling out new machines.
	NeedsUpdate bool

	// Diff is the computed difference between the existing spec of the Machine and the desired spec of the Machine.
	// This is only ever populated when NeedsUpdate is true.
	Diff []string

	// Index denotes the Control Plane Machine index. Each Control Plane Machine replica is index (typically 0-2 in a
	// three node cluster) and the Index will be needed to generate a replacement of this replica,  if a replacement is
	// required.
	Index int32

	// ErrorMessage is used to provide information about any errors that have occurred with the Machine. For example, if
	// the Machine has an error state within its status, it should be propagated up via this error message.
	ErrorMessage string
}

// ObjectRef allows you to uniquely identify a resource within a cluster.
type ObjectRef struct {
	// GroupVersionResource allows the object API path to be constructed by
	// a dynamic client. It will provide the API Group, Version and the Resource name.
	GroupVersionResource schema.GroupVersionResource

	// ObjectMeta contains information about the object.
	// This can be used by the caller to identify the object and inspect
	// details such as owner references.
	ObjectMeta metav1.ObjectMeta
}

// MachineProvider defines an interface for implementing the Machine specific
// functions related to the ControlPlaneMachineSet controller.
type MachineProvider interface {
	// GetMachineInfos is used to collect information about the Control Plane Machines and Nodes that currently exist
	// within the Cluster, as referred to by the ControlPlaneMachineSet.
	GetMachineInfos(context.Context, logr.Logger) ([]MachineInfo, error)

	// WithClient is used to set API client for the Machine Provider.
	// It should not mutate the state of the existing provider but return
	// a copy of the provider with the new client.
	WithClient(context.Context, logr.Logger, client.Client) (MachineProvider, error)

	// CreateMachine is used to instruct the Machine Provider to create a new Machine. The only input is the index for
	// the new Machine. During construction of the MachineProvider, it should map indexes to failure domains so that it
	// has all the required information for creating a new Machine stored, based solely on the index.
	CreateMachine(context.Context, logr.Logger, int32) error

	// DeleteMachine is used to instruct the Machine Provider to delete a particular Machine. This is used by the
	// RollingUpdate strategy of the ControlPlaneMachineSet so that it can remove old Machines once they have been
	// replaced.
	DeleteMachine(context.Context, logr.Logger, *ObjectRef) error
}
