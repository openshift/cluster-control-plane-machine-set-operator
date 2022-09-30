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

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// clusterControlPlaneMachineSetName is the name of the ControlPlaneMachineSet.
	// As ControlPlaneMachineSets are singletons within the namespace, only ControlPlaneMachineSets
	// with this name should be reconciled.
	clusterControlPlaneMachineSetName = "cluster"
)

var _ = Describe("Watch Filters", func() {
	Context("objToControlPlaneMachineSet", func() {
		const testNamespace = "test"
		const operatorName = "control-plane-machine-set"

		var clusterOperatorFilter func(client.Object) []reconcile.Request

		BeforeEach(func() {
			clusterOperatorFilter = ObjToControlPlaneMachineSet(clusterControlPlaneMachineSetName, testNamespace)
		})

		It("returns a correct request for the cluster ControlPlaneMachineSet", func() {
			co := resourcebuilder.ClusterOperator().WithName(operatorName).Build()

			Expect(clusterOperatorFilter(co)).To(ConsistOf(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      clusterControlPlaneMachineSetName,
				},
			}))
		})
	})

	// createEvent is used to pass objects to the predicate Create function.
	createEvent := func(obj client.Object) event.CreateEvent {
		return event.CreateEvent{
			Object: obj,
		}
	}

	// updateEvent is used to pass objects to the predicate Update function.
	updateEvent := func(obj client.Object) event.UpdateEvent {
		return event.UpdateEvent{
			ObjectNew: obj,
		}
	}

	// deleteEvent is used to pass objects to the predicate Delete function.
	deleteEvent := func(obj client.Object) event.DeleteEvent {
		return event.DeleteEvent{
			Object: obj,
		}
	}

	// genericEvent is used to pass objects to the predicate Generic function.
	genericEvent := func(obj client.Object) event.GenericEvent {
		return event.GenericEvent{
			Object: obj,
		}
	}

	Context("filterClusterOperator", func() {
		const operatorName = "control-plane-machine-set"

		var clusterOperatorPredicate predicate.Predicate

		BeforeEach(func() {
			clusterOperatorPredicate = FilterClusterOperator(operatorName)
		})

		It("Panics with the wrong object kind", func() {
			expectedMessage := "expected to get an of object of type configv1.ClusterOperator"
			machine := resourcebuilder.Machine().Build()

			Expect(func() {
				clusterOperatorPredicate.Create(createEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				clusterOperatorPredicate.Update(updateEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				clusterOperatorPredicate.Delete(deleteEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				clusterOperatorPredicate.Generic(genericEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")
		})

		It("returns false when the wrong cluster operator is provided", func() {
			co := resourcebuilder.ClusterOperator().WithName("machine-api-operator").Build()

			Expect(clusterOperatorPredicate.Create(createEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Update(updateEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Delete(deleteEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Generic(genericEvent(co))).To(BeFalse())
		})

		It("returns true when the correct cluster operator is provided", func() {
			co := resourcebuilder.ClusterOperator().WithName(operatorName).Build()

			Expect(clusterOperatorPredicate.Create(createEvent(co))).To(BeTrue())
			Expect(clusterOperatorPredicate.Update(updateEvent(co))).To(BeTrue())
			Expect(clusterOperatorPredicate.Delete(deleteEvent(co))).To(BeTrue())
			Expect(clusterOperatorPredicate.Generic(genericEvent(co))).To(BeTrue())
		})
	})

	Context("filterControlPlaneMachineSet", func() {
		const testNamespace = "test"

		var cpmsPredicate predicate.Predicate

		BeforeEach(func() {
			cpmsPredicate = FilterControlPlaneMachineSet(clusterControlPlaneMachineSetName, testNamespace)
		})

		It("Panics with the wrong object kind", func() {
			expectedMessage := "expected to get an of object of type machinev1.ControlPlaneMachineSet"
			machine := resourcebuilder.Machine().Build()
			Expect(func() {
				cpmsPredicate.Create(createEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				cpmsPredicate.Update(updateEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				cpmsPredicate.Delete(deleteEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				cpmsPredicate.Generic(genericEvent(machine))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")
		})

		It("Returns false with the wrong namespace", func() {
			cpms := resourcebuilder.ControlPlaneMachineSet().
				WithName(clusterControlPlaneMachineSetName).
				WithNamespace("wrong-namespace").
				Build()

			Expect(cpmsPredicate.Create(createEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Update(updateEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Delete(deleteEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Generic(genericEvent(cpms))).To(BeFalse())
		})

		It("Returns false with the wrong name", func() {
			cpms := resourcebuilder.ControlPlaneMachineSet().
				WithName("wrong-name").
				WithNamespace(testNamespace).
				Build()

			Expect(cpmsPredicate.Create(createEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Update(updateEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Delete(deleteEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Generic(genericEvent(cpms))).To(BeFalse())
		})

		It("Returns true with the correct namespace and name", func() {
			cpms := resourcebuilder.ControlPlaneMachineSet().
				WithName(clusterControlPlaneMachineSetName).
				WithNamespace(testNamespace).
				Build()

			Expect(cpmsPredicate.Create(createEvent(cpms))).To(BeTrue())
			Expect(cpmsPredicate.Update(updateEvent(cpms))).To(BeTrue())
			Expect(cpmsPredicate.Delete(deleteEvent(cpms))).To(BeTrue())
			Expect(cpmsPredicate.Generic(genericEvent(cpms))).To(BeTrue())
		})
	})

	Context("filterControlPlaneMachines", func() {
		const testNamespace = "test"

		var machinePredicate predicate.Predicate

		BeforeEach(func() {
			machinePredicate = FilterControlPlaneMachines(testNamespace)
		})

		It("Panics with the wrong object kind", func() {
			expectedMessage := "expected to get an of object of type machinev1beta1.Machine: got type *v1.ControlPlaneMachineSet"
			cpms := resourcebuilder.ControlPlaneMachineSet().Build()

			Expect(func() {
				machinePredicate.Create(createEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				machinePredicate.Update(updateEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				machinePredicate.Delete(deleteEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				machinePredicate.Generic(genericEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")
		})

		It("Returns false with the wrong namespace", func() {
			machine := resourcebuilder.Machine().
				WithNamespace("wrong-namespace").
				AsMaster().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns false with worker machines", func() {
			machine := resourcebuilder.Machine().
				WithNamespace(testNamespace).
				AsWorker().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns false when missing the machine type label", func() {
			machine := resourcebuilder.Machine().
				WithNamespace(testNamespace).
				WithLabels(map[string]string{
					"machine.openshift.io/cluster-api-machine-role": "master",
				}).
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns false when missing the machine role label", func() {
			machine := resourcebuilder.Machine().
				WithNamespace(testNamespace).
				WithLabels(map[string]string{
					"machine.openshift.io/cluster-api-machine-type": "master",
				}).
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns true with the correct namespace and labels", func() {
			machine := resourcebuilder.Machine().
				WithNamespace(testNamespace).
				AsMaster().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeTrue())
		})
	})
})
