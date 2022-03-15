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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// clusterOperatorToControlPlaneMachineSet maps the cluster operator to the control
// plane machine set singleton in the namespace provided.
func clusterOperatorToControlPlaneMachineSet(namespace string, operatorName string) func(client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		// TODO: Implement this function to filter the object just to the ClusterOperator
		// for this operator and to map to a reconcile of the CPMS singleton.
		return []reconcile.Request{}
	}
}

// filterControlPlaneMachineSet filters control plane machine set requests
// to just the singleton within the namespace provided.
// TODO: remove noline once implemented.
func filterControlPlaneMachineSet(namespace string) predicate.Predicate { //nolint:unparam
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		// TODO: Implement this function to filter the object to just the CPMS singleton
		// within the namespace provided.
		return false
	})
}

// filterControlPlaneMachines filters machine requests to just the machines that present as control plane machines,
//  i.e. they are labelled with the correct labels to identify them as control plane machines.
// TODO: remove noline once implemented.
func filterControlPlaneMachines(namespace string) predicate.Predicate { //nolint:unparam
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		// TODO: Implement this function to filter the object to just the
		// control plane machines in the namespace provided.
		return false
	})
}
