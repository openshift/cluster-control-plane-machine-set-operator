/*
Copyright 2023 Red Hat, Inc.

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
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"

	corev1 "k8s.io/api/core/v1"
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
			co := configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()

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

	// updateEventFull is used to pass objects to the predicate Update function.
	updateEventFull := func(objOld client.Object, objNew client.Object) event.UpdateEvent {
		return event.UpdateEvent{
			ObjectNew: objNew,
			ObjectOld: objOld,
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
			machine := machinev1beta1resourcebuilder.Machine().Build()

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
			co := configv1resourcebuilder.ClusterOperator().WithName("machine-api-operator").Build()

			Expect(clusterOperatorPredicate.Create(createEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Update(updateEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Delete(deleteEvent(co))).To(BeFalse())
			Expect(clusterOperatorPredicate.Generic(genericEvent(co))).To(BeFalse())
		})

		It("returns true when the correct cluster operator is provided", func() {
			co := configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()

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
			machine := machinev1beta1resourcebuilder.Machine().Build()
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
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().
				WithName(clusterControlPlaneMachineSetName).
				WithNamespace("wrong-namespace").
				Build()

			Expect(cpmsPredicate.Create(createEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Update(updateEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Delete(deleteEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Generic(genericEvent(cpms))).To(BeFalse())
		})

		It("Returns false with the wrong name", func() {
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().
				WithName("wrong-name").
				WithNamespace(testNamespace).
				Build()

			Expect(cpmsPredicate.Create(createEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Update(updateEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Delete(deleteEvent(cpms))).To(BeFalse())
			Expect(cpmsPredicate.Generic(genericEvent(cpms))).To(BeFalse())
		})

		It("Returns true with the correct namespace and name", func() {
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().
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
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().Build()

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
			machine := machinev1beta1resourcebuilder.Machine().
				WithNamespace("wrong-namespace").
				AsMaster().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns false with worker machines", func() {
			machine := machinev1beta1resourcebuilder.Machine().
				WithNamespace(testNamespace).
				AsWorker().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeFalse())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeFalse())
		})

		It("Returns false when missing the machine type label", func() {
			machine := machinev1beta1resourcebuilder.Machine().
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
			machine := machinev1beta1resourcebuilder.Machine().
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
			machine := machinev1beta1resourcebuilder.Machine().
				WithNamespace(testNamespace).
				AsMaster().
				Build()

			Expect(machinePredicate.Create(createEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Update(updateEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Delete(deleteEvent(machine))).To(BeTrue())
			Expect(machinePredicate.Generic(genericEvent(machine))).To(BeTrue())
		})
	})

	Context("filterControlPlaneNodes", func() {
		var nodesPredicate predicate.Predicate

		BeforeEach(func() {
			nodesPredicate = FilterControlPlaneNodes()
		})

		It("Panics with the wrong object kind", func() {
			expectedMessage := "expected to get an of object of type corev1.Node: got type *v1.ControlPlaneMachineSet"
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().Build()

			Expect(func() {
				nodesPredicate.Create(createEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				nodesPredicate.Update(updateEventFull(cpms, cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				nodesPredicate.Delete(deleteEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")

			Expect(func() {
				nodesPredicate.Generic(genericEvent(cpms))
			}).To(PanicWith(expectedMessage), "A programming error occurs when passing the wrong object, the function should panic")
		})

		It("Returns false with worker node", func() {
			node := corev1resourcebuilder.Node().AsWorker().Build()

			Expect(nodesPredicate.Create(createEvent(node))).To(BeFalse())
			Expect(nodesPredicate.Update(updateEvent(node))).To(BeFalse())
			Expect(nodesPredicate.Delete(deleteEvent(node))).To(BeFalse())
			Expect(nodesPredicate.Generic(genericEvent(node))).To(BeFalse())
		})

		It("Returns true with master node transitioned from NotReady to Ready", func() {
			oldNode := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			).AsMaster().Build()
			node := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			).AsMaster().Build()

			Expect(nodesPredicate.Create(createEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Update(updateEventFull(oldNode, node))).To(BeTrue())
			Expect(nodesPredicate.Delete(deleteEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Generic(genericEvent(node))).To(BeFalse())
		})

		It("Returns true with master node transitioned from Ready to NotReady", func() {
			oldNode := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			).AsMaster().Build()
			node := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			).AsMaster().Build()

			Expect(nodesPredicate.Create(createEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Update(updateEventFull(oldNode, node))).To(BeTrue())
			Expect(nodesPredicate.Delete(deleteEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Generic(genericEvent(node))).To(BeFalse())
		})

		It("Returns false with master node stayed in Ready", func() {
			oldNode := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			).AsMaster().Build()
			node := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			).AsMaster().Build()

			Expect(nodesPredicate.Create(createEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Update(updateEventFull(oldNode, node))).To(BeFalse())
			Expect(nodesPredicate.Delete(deleteEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Generic(genericEvent(node))).To(BeFalse())
		})

		It("Returns false with master node stayed in NotReady", func() {
			oldNode := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			).AsMaster().Build()
			node := corev1resourcebuilder.Node().WithConditions(
				[]corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			).AsMaster().Build()

			Expect(nodesPredicate.Create(createEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Update(updateEventFull(oldNode, node))).To(BeFalse())
			Expect(nodesPredicate.Delete(deleteEvent(node))).To(BeTrue())
			Expect(nodesPredicate.Generic(genericEvent(node))).To(BeFalse())
		})
	})
})
