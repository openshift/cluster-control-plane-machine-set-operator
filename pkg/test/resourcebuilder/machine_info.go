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

package resourcebuilder

import (
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MachineInfo creates a new machineinfo builder.
func MachineInfo() MachineInfoBuilder {
	return MachineInfoBuilder{
		needsUpdate: false,
		ready:       true,
	}
}

// MachineInfoBuilder is used to build out a machineinfo object.
type MachineInfoBuilder struct {
	machineDeletiontimestamp *metav1.Time
	machineGVR               schema.GroupVersionResource
	machineName              string
	machineOwnerRefs         []metav1.OwnerReference

	nodeGVR  schema.GroupVersionResource
	nodeName string

	errorMessage string
	index        int32
	needsUpdate  bool
	ready        bool
}

// Build builds a new machineinfo based on the configuration provided.
func (m MachineInfoBuilder) Build() machineproviders.MachineInfo {
	info := machineproviders.MachineInfo{
		ErrorMessage: m.errorMessage,
		Index:        m.index,
		Ready:        m.ready,
		NeedsUpdate:  m.needsUpdate,
	}

	if m.machineName != "" {
		info.MachineRef = &machineproviders.ObjectRef{
			GroupVersionResource: m.machineGVR,
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: m.machineDeletiontimestamp,
				Name:              m.machineName,
				OwnerReferences:   m.machineOwnerRefs,
			},
		}
	}

	if m.nodeName != "" {
		info.NodeRef = &machineproviders.ObjectRef{
			GroupVersionResource: m.nodeGVR,
			ObjectMeta: metav1.ObjectMeta{
				Name: m.nodeName,
			},
		}
	}

	return info
}

// WithMachineDeletionTimestamp sets the machine deletion timestamp for the machineinfo builder.
func (m MachineInfoBuilder) WithMachineDeletionTimestamp(deletion metav1.Time) MachineInfoBuilder {
	m.machineDeletiontimestamp = &deletion
	return m
}

// WithMachineGVR sets the machine groupversionresource for the machineinfo builder.
func (m MachineInfoBuilder) WithMachineGVR(gvr schema.GroupVersionResource) MachineInfoBuilder {
	m.machineGVR = gvr
	return m
}

// WithMachineName sets the machine name for the machineinfo builder.
func (m MachineInfoBuilder) WithMachineName(name string) MachineInfoBuilder {
	m.machineName = name
	return m
}

// WithMachineOwnerReference adds an owner reference for the machine for the machineinfo builder.
func (m MachineInfoBuilder) WithMachineOwnerReference(or metav1.OwnerReference) MachineInfoBuilder {
	m.machineOwnerRefs = append(m.machineOwnerRefs, or)
	return m
}

// WithMachineOwnerReferences replaces the owner references for the machine for the machineinfo builder.
func (m MachineInfoBuilder) WithMachineOwnerReferences(ors []metav1.OwnerReference) MachineInfoBuilder {
	m.machineOwnerRefs = ors
	return m
}

// WithNodeGVR sets the node groupversionresource for the machineinfo builder.
func (m MachineInfoBuilder) WithNodeGVR(gvr schema.GroupVersionResource) MachineInfoBuilder {
	m.nodeGVR = gvr
	return m
}

// WithNodeName sets the node name for the machineinfo builder.
func (m MachineInfoBuilder) WithNodeName(name string) MachineInfoBuilder {
	m.nodeName = name
	return m
}

// WithErrorMessage sets the error message for the machineinfo builder.
func (m MachineInfoBuilder) WithErrorMessage(errorMsg string) MachineInfoBuilder {
	m.errorMessage = errorMsg
	return m
}

// WithIndex sets the index for the machineinfo builder.
func (m MachineInfoBuilder) WithIndex(index int32) MachineInfoBuilder {
	m.index = index
	return m
}

// WithNeedsUpdate sets the needsupdate for the machineinfo builder.
func (m MachineInfoBuilder) WithNeedsUpdate(needsUpdate bool) MachineInfoBuilder {
	m.needsUpdate = needsUpdate
	return m
}

// WithReady sets the ready for the machineinfo builder.
func (m MachineInfoBuilder) WithReady(ready bool) MachineInfoBuilder {
	m.ready = ready
	return m
}
