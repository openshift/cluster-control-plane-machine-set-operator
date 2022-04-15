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
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// machineRoleLabelName is the label used to identify the role of a machine.
	machineRoleLabelName = "machine.openshift.io/cluster-api-machine-role"

	// machineTypeLabelName is the label used to identify the type of a machine.
	machineTypeLabelName = "machine.openshift.io/cluster-api-machine-type"

	// machineMasterRoleLabelName is the label value to identify the role of a control plane machine.
	machineMasterRoleLabelName = "master"

	// machineMasterTypeLabelName is the label value to identify the type of a control plane machine.
	machineMasterTypeLabelName = "master"
)

// clusterOperatorToControlPlaneMachineSet maps the cluster operator to the control
// plane machine set singleton in the namespace provided.
func clusterOperatorToControlPlaneMachineSet(namespace string) func(client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: client.ObjectKey{Namespace: namespace, Name: clusterControlPlaneMachineSetName},
		}}
	}
}

// filterClusterOperator filters cluster operator requests
// to just the one with the name provided.
func filterClusterOperator(name string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		co, ok := obj.(*configv1.ClusterOperator)
		if !ok {
			panic("expected to get an of object of type configv1.ClusterOperator")
		}

		return co.GetName() == name
	})
}

// filterControlPlaneMachineSet filters control plane machine set requests
// to just the singleton within the namespace provided.
func filterControlPlaneMachineSet(namespace string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		cpms, ok := obj.(*machinev1.ControlPlaneMachineSet)
		if !ok {
			panic("expected to get an of object of type machinev1.ControlPlaneMachineSet")
		}

		return cpms.GetNamespace() == namespace && cpms.GetName() == clusterControlPlaneMachineSetName
	})
}

// filterControlPlaneMachines filters machine requests to just the machines that present as control plane machines,
// i.e. they are labelled with the correct labels to identify them as control plane machines.
func filterControlPlaneMachines(namespace string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		machine, ok := obj.(*machinev1beta1.Machine)
		if !ok {
			panic("expected to get an of object of type machinev1beta1.Machine")
		}

		// Check namespace first
		if machine.GetNamespace() != namespace {
			return false
		}

		// Ensuring that this is a master machine by checking required labels
		labels := machine.GetLabels()

		return labels[machineRoleLabelName] == machineMasterRoleLabelName && labels[machineTypeLabelName] == machineMasterTypeLabelName
	})
}
