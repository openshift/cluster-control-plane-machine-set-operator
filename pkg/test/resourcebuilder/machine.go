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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
)

const (
	machineRoleLabelName = "machine.openshift.io/cluster-api-machine-role"
	machineTypeLabelName = "machine.openshift.io/cluster-api-machine-type"
)

// Machine creates a new machine builder.
func Machine() MachineBuilder {
	return MachineBuilder{}
}

// MachineBuilder is used to build out a machine object.
type MachineBuilder struct {
	generateName        string
	name                string
	namespace           string
	labels              map[string]string
	creationTimestamp   metav1.Time
	providerSpecBuilder RawExtensionBuilder

	// status fields
	errorMessage *string
	nodeRef      *corev1.ObjectReference
	phase        *string
}

// Build builds a new machine based on the configuration provided.
func (m MachineBuilder) Build() *machinev1beta1.Machine {
	machine := &machinev1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:      m.generateName,
			CreationTimestamp: m.creationTimestamp,
			Name:              m.name,
			Namespace:         m.namespace,
			Labels:            m.labels,
		},
		Status: machinev1beta1.MachineStatus{
			ErrorMessage: m.errorMessage,
			Phase:        m.phase,
			NodeRef:      m.nodeRef,
		},
	}

	if m.providerSpecBuilder != nil {
		machine.Spec.ProviderSpec.Value = m.providerSpecBuilder.BuildRawExtension()
	}

	m.WithLabel(machinev1beta1.MachineClusterIDLabel, testClusterIDValue)

	return machine
}

// AsWorker sets the worker role and type on the machine labels for the machine builder.
func (m MachineBuilder) AsWorker() MachineBuilder {
	return m.
		WithLabel(machineRoleLabelName, "worker").
		WithLabel(machineTypeLabelName, "worker")
}

// AsMaster sets the master role and type on the machine labels for the machine builder.
func (m MachineBuilder) AsMaster() MachineBuilder {
	return m.
		WithLabel(machineRoleLabelName, "master").
		WithLabel(machineTypeLabelName, "master")
}

// WithCreationTimestamp sets the creationTimestamp for the machine builder.
func (m MachineBuilder) WithCreationTimestamp(time metav1.Time) MachineBuilder {
	m.creationTimestamp = time
	return m
}

// WithGenerateName sets the generateName for the machine builder.
func (m MachineBuilder) WithGenerateName(generateName string) MachineBuilder {
	m.generateName = generateName
	return m
}

// WithLabel sets the labels for the machine builder.
func (m MachineBuilder) WithLabel(key, value string) MachineBuilder {
	if m.labels == nil {
		m.labels = make(map[string]string)
	}

	m.labels[key] = value

	return m
}

// WithLabels sets the labels for the machine builder.
func (m MachineBuilder) WithLabels(labels map[string]string) MachineBuilder {
	m.labels = labels
	return m
}

// WithName sets the name for the machine builder.
func (m MachineBuilder) WithName(name string) MachineBuilder {
	m.name = name
	return m
}

// WithNamespace sets the namespace for the machine builder.
func (m MachineBuilder) WithNamespace(namespace string) MachineBuilder {
	m.namespace = namespace
	return m
}

// WithProviderSpecBuilder sets the providerSpec builder for the machine builder.
func (m MachineBuilder) WithProviderSpecBuilder(builder RawExtensionBuilder) MachineBuilder {
	m.providerSpecBuilder = builder
	return m
}

// Status Fields

// WithErrorMessage sets the error message status field for the machine builder.
func (m MachineBuilder) WithErrorMessage(errorMsg string) MachineBuilder {
	m.errorMessage = &errorMsg
	return m
}

// WithPhase sets the phase status field for the machine builder.
func (m MachineBuilder) WithPhase(phase string) MachineBuilder {
	m.phase = &phase
	return m
}

// WithNodeRef sets the node ref status field for the machine builder.
func (m MachineBuilder) WithNodeRef(nodeRef corev1.ObjectReference) MachineBuilder {
	m.nodeRef = &nodeRef
	return m
}
