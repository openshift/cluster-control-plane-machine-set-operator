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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("With a running controller", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}

	var namespaceName string

	const operatorName = "control-plane-machine-set"

	var co *configv1.ClusterOperator

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a manager and controller")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             testScheme,
			MetricsBindAddress: "0",
			Port:               testEnv.WebhookInstallOptions.LocalServingPort,
			Host:               testEnv.WebhookInstallOptions.LocalServingHost,
			CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		reconciler := &ControlPlaneMachineSetReconciler{
			Namespace:    namespaceName,
			OperatorName: operatorName,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

		By("Starting the manager")
		var mgrCtx context.Context
		mgrCtx, mgrCancel = context.WithCancel(context.Background())
		mgrDone = make(chan struct{})

		go func() {
			defer GinkgoRecover()
			defer close(mgrDone)

			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()

		// CVO will create a blank cluster operator for us before the operator starts.
		co = resourcebuilder.ClusterOperator().WithName(operatorName).Build()
		Expect(k8sClient.Create(ctx, co)).To(Succeed())
	})

	AfterEach(func() {
		By("Stopping the manager")
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone

		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&configv1.ClusterOperator{},
			&machinev1beta1.Machine{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("when a new Control Plane Machine Set is created", func() {
		var cpms *machinev1.ControlPlaneMachineSet

		// Create the CPMS just before each test so that we can set up
		// various test cases in BeforeEach blocks.
		JustBeforeEach(func() {
			// The default CPMS should be sufficient for this test.
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).Build()
			Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())
		})

		PIt("should add the controlplanemachineset.machine.openshift.io finalizer", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))
		})
	})

	Context("with an existing ControlPlaneMachineSet", func() {
		var cpms *machinev1.ControlPlaneMachineSet

		BeforeEach(func() {
			// The default CPMS should be sufficient for this test.
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).Build()
			Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())

			// To ensure that at least one reconcile happens, wait for the status to not be empty.
			Eventually(komega.Object(cpms)).Should(HaveField("Status.ObservedGeneration", Not(Equal(int64(0)))))
		})

		Context("if the finalizer is removed", func() {
			BeforeEach(func() {
				// Ensure the finalizer was already added
				Expect(komega.Object(cpms)()).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))

				// Remove the finalizer
				Eventually(komega.Update(cpms, func() {
					cpms.ObjectMeta.Finalizers = []string{}
				})).Should(Succeed())

				// CPMS should now have no finalizers, reflecting the state of the API.
				Expect(cpms.ObjectMeta.Finalizers).To(BeEmpty())
			})

			PIt("should re-add the controlplanemachineset.machine.openshift.io finalizer", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))
			})
		})
	})

	Context("when deleting the ControlPlaneMachineSet", func() {
		var cpms *machinev1.ControlPlaneMachineSet

		BeforeEach(func() {
			By("Creating a ControlPlaneMachineSet")
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).Build()
			cpms.SetFinalizers([]string{controlPlaneMachineSetFinalizer})
			Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())

			By("Creating Machines owned by the ControlPlaneMachineSet")
			machineBuilder := resourcebuilder.Machine().AsMaster().WithGenerateName("delete-test-").WithNamespace(namespaceName)

			for i := 0; i < 3; i++ {
				machine := machineBuilder.Build()
				Expect(controllerutil.SetControllerReference(cpms, machine, testScheme)).To(Succeed())
				Expect(k8sClient.Create(ctx, machine)).To(Succeed())
			}

			machines := &machinev1beta1.MachineList{}
			Expect(k8sClient.List(ctx, machines)).To(Succeed())
			Expect(machines.Items).To(HaveLen(3))

			By("Deleting the ControlPlaneMachineSet")
			Expect(k8sClient.Delete(ctx, cpms)).To(Succeed())
		})

		PIt("should eventually be removed", func() {
			Eventually(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"cluster\" not found"))
		})

		PIt("should remove the owner references from the Machines", func() {
			Eventually(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", SatisfyAll(
				HaveLen(3),
				HaveEach(HaveField("ObjectMeta.OwnerReferences", HaveLen(0))),
			)), "3 Machines should exist, each should have no owner references")
		})
	})
})

var _ = Describe("ensureFinalizer", func() {
	var namespaceName string
	var reconciler *ControlPlaneMachineSetReconciler
	var cpms *machinev1.ControlPlaneMachineSet
	var logger test.TestLogger

	const existingFinalizer = "existingFinalizer"

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		reconciler = &ControlPlaneMachineSetReconciler{
			Client:    k8sClient,
			Scheme:    testScheme,
			Namespace: namespaceName,
		}

		// The ControlPlaneMachineSet should already exist by the time we get here.
		By("Creating a ControlPlaneMachineSet")
		cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).Build()
		cpms.ObjectMeta.Finalizers = []string{existingFinalizer}
		Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())

		logger = test.NewTestLogger()
	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("when the finalizer does not exist", func() {
		var updatedFinalizer bool
		var err error

		BeforeEach(func() {
			updatedFinalizer, err = reconciler.ensureFinalizer(ctx, logger.Logger(), cpms)
		})

		It("does not error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		PIt("returns that it updated the finalizer", func() {
			Expect(updatedFinalizer).To(BeTrue())
		})

		PIt("sets an appropriate log line", func() {
			Expect(logger.Entries()).To(ConsistOf(
				test.LogEntry{
					Level:   2,
					Message: "Added finalizer to control plane machine set",
				},
			))
		})

		PIt("ensures the finalizer is set on the API", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))
		})

		It("does not remove any existing finalizers", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(existingFinalizer)))
		})
	})

	Context("when the finalizer already exists", func() {
		var updatedFinalizer bool
		var err error

		BeforeEach(func() {
			By("Adding the finalizer to the existing object")
			Eventually(komega.Update(cpms, func() {
				cpms.SetFinalizers(append(cpms.GetFinalizers(), controlPlaneMachineSetFinalizer))
			})).Should(Succeed())

			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ConsistOf(controlPlaneMachineSetFinalizer, existingFinalizer)))

			updatedFinalizer, err = reconciler.ensureFinalizer(ctx, logger.Logger(), cpms)
		})

		It("does not error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns that it did not update the finalizer", func() {
			Expect(updatedFinalizer).To(BeFalse())
		})

		PIt("sets an appropriate log line", func() {
			Expect(logger.Entries()).To(ConsistOf(
				test.LogEntry{
					Level:   4,
					Message: "Finalizer already present on control plane machine set",
				},
			))
		})

		It("does not remove any existing finalizers", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ConsistOf(controlPlaneMachineSetFinalizer, existingFinalizer)))
		})
	})

	Context("when the finalizer already exists, but the input is stale", func() {
		var updatedFinalizer bool
		var err error

		BeforeEach(func() {
			By("Adding the finalizer to the existing object")
			originalCPMS := cpms.DeepCopy()
			Eventually(komega.Update(cpms, func() {
				cpms.SetFinalizers(append(cpms.GetFinalizers(), controlPlaneMachineSetFinalizer))
			})).Should(Succeed())

			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ConsistOf(controlPlaneMachineSetFinalizer, existingFinalizer)))

			updatedFinalizer, err = reconciler.ensureFinalizer(ctx, logger.Logger(), originalCPMS)
		})

		PIt("should return a conflict error", func() {
			Expect(err).To(MatchError(ContainSubstring("TODO")))
		})

		It("returns that it did not update the finalizer", func() {
			Expect(updatedFinalizer).To(BeFalse())
		})

		PIt("does not log", func() {
			Expect(logger.Entries()).To(BeEmpty())
		})

		It("does not remove any existing finalizers", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ConsistOf(controlPlaneMachineSetFinalizer, existingFinalizer)))
		})
	})
})

var _ = Describe("ensureOwnerRefrences", func() {
	var namespaceName string
	var reconciler *ControlPlaneMachineSetReconciler
	var cpms *machinev1.ControlPlaneMachineSet
	var logger test.TestLogger

	var expectedOwnerReference metav1.OwnerReference
	var machines []*machinev1beta1.Machine
	var machineInfos map[int32][]machineproviders.MachineInfo
	machineGVR := machinev1beta1.GroupVersion.WithResource("machines")

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-ensure-owner-references-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		reconciler = &ControlPlaneMachineSetReconciler{
			Client:     k8sClient,
			Scheme:     testScheme,
			RESTMapper: testRESTMapper,
			Namespace:  namespaceName,
		}

		// The ControlPlaneMachineSet should already exist by the time we get here.
		By("Creating a ControlPlaneMachineSet")
		cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithGeneration(2).Build()
		Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())

		logger = test.NewTestLogger()

		machine := resourcebuilder.Machine().WithNamespace(namespaceName).Build()

		Expect(controllerutil.SetControllerReference(cpms, machine, testScheme)).To(Succeed())
		Expect(machine.GetOwnerReferences()).To(HaveLen(1))

		// We know the owner reference we expect to find at the end as the controller util is
		// already tested and trusted.
		expectedOwnerReference = machine.GetOwnerReferences()[0]

		By("Creating machines to add owner references to")
		machines = []*machinev1beta1.Machine{}
		machineInfos = map[int32][]machineproviders.MachineInfo{}
		machineBuilder := resourcebuilder.Machine().WithNamespace(namespaceName).WithGenerateName("ensure-owner-references-test-")

		for i := 0; i < 3; i++ {
			machine := machineBuilder.Build()
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())

			machines = append(machines, machine)

			machineInfo := resourcebuilder.MachineInfo().WithMachineGVR(machineGVR).WithMachineName(machine.GetName()).Build()
			machineInfos[int32(i)] = append(machineInfos[int32(i)], machineInfo)
		}
	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&machinev1beta1.Machine{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("when the machines do not have existing owner references", func() {
		BeforeEach(func() {
			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		PIt("should add the expected owner references", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		PIt("should log that it has updated the owner references", func() {
			expectedEntries := []test.LogEntry{}

			for _, machine := range machines {
				expectedEntries = append(expectedEntries, test.LogEntry{
					KeysAndValues: []interface{}{"machineNamespace", machine.GetNamespace(), "machineName", machine.GetName()},
					Level:         2,
					Message:       "Added owner reference to machine",
				})
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})
	})

	Context("when the machines already have an existing owner references", func() {
		BeforeEach(func() {
			By("Setting up the appropriate MachineInfos")
			machineInfos = map[int32][]machineproviders.MachineInfo{}

			for i := range machineInfos {
				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}
			}

			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		PIt("should not update the owner references", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		PIt("should log that no update was needed", func() {
			expectedEntries := []test.LogEntry{}

			for _, machine := range machines {
				expectedEntries = append(expectedEntries, test.LogEntry{
					KeysAndValues: []interface{}{"machineNamespace", machine.GetNamespace(), "machineName", machine.GetName()},
					Level:         4,
					Message:       "Owner reference already present on machine",
				})
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})
	})

	Context("when some machines already have an existing owner reference", func() {
		BeforeEach(func() {
			By("Adding an owner reference to some of the machine infos")
			skippedOne := false
			Expect(machineInfos).To(HaveLen(3))
			for i := range machineInfos {
				if !skippedOne {
					skippedOne = true
					continue
				}

				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}
			}

			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		PIt("should update the owner reference where needed", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		PIt("should log the update that was needed", func() {
			expectedEntries := []test.LogEntry{}

			machineInfos0 := machineInfos[0]
			Expect(machineInfos0).To(HaveLen(1))

			machineInfo := machineInfos0[0]
			Expect(machineInfo.MachineRef).ToNot(BeNil())
			expectedEntries = append(expectedEntries, test.LogEntry{
				KeysAndValues: []interface{}{"machineNamespace", machineInfo.MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo.MachineRef.ObjectMeta.GetName()},
				Level:         2,
				Message:       "Added owner reference to machine",
			})

			skippedOne := false
			for _, machineInfo := range machineInfos {
				if !skippedOne {
					skippedOne = true
					continue
				}

				for i := range machineInfo {
					Expect(machineInfo[i].MachineRef).ToNot(BeNil())
					expectedEntries = append(expectedEntries, test.LogEntry{
						KeysAndValues: []interface{}{"machineNamespace", machineInfo[i].MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo[i].MachineRef.ObjectMeta.GetName()},
						Level:         4,
						Message:       "Owner reference already present on machine",
					})
				}
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})
	})

	Context("when not all MachineInfos contain a Machine", func() {
		var skipMachine string

		BeforeEach(func() {
			Expect(machineInfos).To(HaveKeyWithValue(int32(0), HaveLen(1)))
			Expect(machineInfos[0][0].MachineRef).ToNot(BeNil())
			skipMachine = machineInfos[0][0].MachineRef.ObjectMeta.GetName()
			machineInfos[0][0].MachineRef = nil

			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		PIt("should add the expected owner references", func() {
			for _, machine := range machines {
				if machine.GetName() == skipMachine {
					continue
				}

				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		PIt("should log that it has updated the owner references", func() {
			expectedEntries := []test.LogEntry{}

			for _, machine := range machines {
				if machine.GetName() == skipMachine {
					continue
				}

				expectedEntries = append(expectedEntries, test.LogEntry{
					KeysAndValues: []interface{}{"machineNamespace", machine.GetNamespace(), "machineName", machine.GetName()},
					Level:         2,
					Message:       "Added owner reference to machine",
				})
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})
	})

	Context("when a machine has a conflicting controller owner reference", func() {
		var err, expectedError error

		BeforeEach(func() {
			By("Adding an owner reference to some of the machine infos")
			Expect(machineInfos).To(HaveLen(3))

			skippedOne := false
			for i := range machineInfos {
				if !skippedOne {
					skippedOne = true
					continue
				}
				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}
			}

			By("Adding an owner reference for an alternative controller owner")
			badOwnerReference := expectedOwnerReference
			badOwnerReference.Name = "different-owner"

			Expect(machineInfos[0][0].MachineRef).ToNot(BeNil())
			machineInfos[0][0].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{badOwnerReference}

			By("Formulating the expected error")
			machine := resourcebuilder.Machine().WithNamespace(namespaceName).Build()
			machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{badOwnerReference}

			setControllerErr := controllerutil.SetControllerReference(cpms, machine, testScheme)
			Expect(setControllerErr).To(HaveOccurred())
			expectedError = fmt.Errorf("error setting owner reference: %w", setControllerErr)

			err = reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
		})

		PIt("should return an error", func() {
			Expect(err).To(MatchError(expectedError))
		})

		PIt("should add an error log", func() {
			expectedEntries := []test.LogEntry{}

			machineInfo0 := machineInfos[0]
			Expect(machineInfo0).To(HaveLen(1))

			machineInfo := machineInfo0[0]
			Expect(machineInfo.MachineRef).ToNot(BeNil())
			expectedEntries = append(expectedEntries, test.LogEntry{
				Error:         expectedError,
				KeysAndValues: []interface{}{"machineNamespace", machineInfo.MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo.MachineRef.ObjectMeta.GetName()},
				Message:       "Cannot add owner reference to machine",
			})

			for _, machineInfo := range machineInfos {
				for i := range machineInfo {
					Expect(machineInfo[i].MachineRef).ToNot(BeNil())
					expectedEntries = append(expectedEntries, test.LogEntry{
						KeysAndValues: []interface{}{"machineNamespace", machineInfo[i].MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo[i].MachineRef.ObjectMeta.GetName()},
						Level:         4,
						Message:       "Owner reference already present on machine",
					})
				}
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})

		PIt("should set the degraded condition on the ControlPlaneMachineSet", func() {
			Expect(cpms.Status.Conditions).To(ContainElement(test.MatchCondition(
				metav1.Condition{
					Type:               conditionDegraded,
					Status:             metav1.ConditionTrue,
					Reason:             reasonMachinesAlreadyOwned,
					ObservedGeneration: 2,
					Message:            "Observed already owned machine(s) in target machines",
				},
			)))
		})
	})
})

var _ = Describe("machineInfosByIndex", func() {
	i0m0 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-0").Build()
	i0m1 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-1").Build()
	i0m2 := resourcebuilder.MachineInfo().WithIndex(0).WithMachineName("machine-0-2").Build()
	i1m0 := resourcebuilder.MachineInfo().WithIndex(1).WithMachineName("machine-1-0").Build()
	i1m1 := resourcebuilder.MachineInfo().WithIndex(1).WithMachineName("machine-1-1").Build()
	i2m0 := resourcebuilder.MachineInfo().WithIndex(2).WithMachineName("machine-2-0").Build()

	type tableInput struct {
		cpms          *machinev1.ControlPlaneMachineSet
		machineInfos  []machineproviders.MachineInfo
		expected      map[int32][]machineproviders.MachineInfo
		expectedError error
	}

	DescribeTable("should sort Machine Infos by index", func(in tableInput) {
		out, err := machineInfosByIndex(in.cpms, in.machineInfos)
		if in.expectedError != nil {
			Expect(err).To(MatchError(in.expectedError))
			return
		}
		Expect(err).ToNot(HaveOccurred())

		Expect(out).To(HaveLen(len(in.expected)))
		// Check each key and its values separately to avoid ordering within the lists
		// from causing issues.
		for key, values := range in.expected {
			Expect(out).To(HaveKeyWithValue(key, ConsistOf(values)))
		}
	},
		Entry("with no replicas in the ControlPlaneMachineSet", tableInput{
			cpms:          &machinev1.ControlPlaneMachineSet{},
			expectedError: errReplicasRequired,
		}),
		Entry("no machine infos with 3 replicas", tableInput{
			cpms:         resourcebuilder.ControlPlaneMachineSet().WithReplicas(3).Build(),
			machineInfos: []machineproviders.MachineInfo{},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {},
				1: {},
				2: {},
			},
		}),
		Entry("separately indexed machines", tableInput{
			cpms:         resourcebuilder.ControlPlaneMachineSet().WithReplicas(3).Build(),
			machineInfos: []machineproviders.MachineInfo{i0m0, i1m0, i2m0},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0},
				1: {i1m0},
				2: {i2m0},
			},
		}),
		Entry("a mixture of indexed machines", tableInput{
			cpms:         resourcebuilder.ControlPlaneMachineSet().WithReplicas(3).Build(),
			machineInfos: []machineproviders.MachineInfo{i0m0, i1m0, i2m0, i0m1, i1m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
				1: {i1m0, i1m1},
				2: {i2m0},
			},
		}),
		Entry("all machines in the same index with 1 replica", tableInput{
			cpms:         resourcebuilder.ControlPlaneMachineSet().WithReplicas(1).Build(),
			machineInfos: []machineproviders.MachineInfo{i0m0, i0m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
			},
		}),
		Entry("all machines in the same index with 3 replicas", tableInput{
			cpms:         resourcebuilder.ControlPlaneMachineSet().WithReplicas(3).Build(),
			machineInfos: []machineproviders.MachineInfo{i0m0, i0m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
				1: {},
				2: {},
			},
		}),
	)
})
