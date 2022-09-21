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
	"errors"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("With a running controller", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}

	var namespaceName string

	const operatorName = "control-plane-machine-set"

	var co *configv1.ClusterOperator

	usEast1aFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1a").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1a"},
				},
			},
		})

	usEast1bFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1b").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1b"},
				},
			},
		})

	usEast1cFailureDomainBuilder := resourcebuilder.AWSFailureDomain().
		WithAvailabilityZone("us-east-1c").
		WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1c"},
				},
			},
		})

	tmplBuilder := resourcebuilder.OpenShiftMachineV1Beta1Template().
		WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilder,
			usEast1bFailureDomainBuilder,
			usEast1cFailureDomainBuilder,
		)).
		WithProviderSpecBuilder(resourcebuilder.AWSProviderSpec())

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
			Client:       mgr.GetClient(),
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
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(tmplBuilder).Build()

			Expect(k8sClient.Create(ctx, cpms)).Should(Succeed())
		})

		It("should add the controlplanemachineset.machine.openshift.io finalizer", func() {
			Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))
		})
	})

	Context("with an existing ControlPlaneMachineSet", func() {
		var cpms *machinev1.ControlPlaneMachineSet

		BeforeEach(func() {
			// The default CPMS should be sufficient for this test.
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(tmplBuilder).Build()
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

			It("should re-add the controlplanemachineset.machine.openshift.io finalizer", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("ObjectMeta.Finalizers", ContainElement(controlPlaneMachineSetFinalizer)))
			})
		})
	})

	Context("when deleting the ControlPlaneMachineSet", func() {
		var cpms *machinev1.ControlPlaneMachineSet

		BeforeEach(func() {
			By("Creating a ControlPlaneMachineSet")
			cpms = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(tmplBuilder).Build()
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

			By("Deleting the ControlPlaneMachineSet")
			Expect(k8sClient.Delete(ctx, cpms)).To(Succeed())
		})

		It("should eventually be removed", func() {
			Eventually(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"cluster\" not found"))
		})

		It("should remove the owner references from the Machines", func() {
			Eventually(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", SatisfyAll(
				HaveEach(HaveField("ObjectMeta.OwnerReferences", HaveLen(0))),
			)), "each machine should have no owner references")
		})
	})
})

var _ = Describe("ownerRef helpers", Ordered, func() {
	var (
		ownerOne, ownerTwo, ownerThree, target client.Object
		namespaceName                          string
	)

	BeforeAll(func() {
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		ownerOne = resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).Build()
		ownerTwo = resourcebuilder.ClusterOperator().WithName("bar").Build()
		ownerThree = resourcebuilder.ClusterOperator().WithName("fizz").Build()
		// Create objects to populate UIDs
		Expect(k8sClient.Create(ctx, ownerOne)).To(Succeed())
		Expect(k8sClient.Create(ctx, ownerTwo)).To(Succeed())
		Expect(k8sClient.Create(ctx, ownerThree)).To(Succeed())

		target = resourcebuilder.Machine().AsMaster().WithGenerateName("owner-ref-test-").WithNamespace(namespaceName).Build()
		Expect(controllerutil.SetControllerReference(ownerOne, target, testScheme)).To(Succeed())
		Expect(controllerutil.SetOwnerReference(ownerTwo, target, testScheme)).To(Succeed())
		Expect(target.GetOwnerReferences()).To(HaveLen(2))
	})

	It("hasOwnerRef should return true if object is target's owner", func() {
		Expect(hasOwnerRef(target, ownerOne)).Should(BeTrue())
		Expect(hasOwnerRef(target, ownerTwo)).Should(BeTrue())
	})

	It("hasOwnerRef should return false if object is not target's owner", func() {
		Expect(hasOwnerRef(target, ownerThree)).Should(BeFalse())
	})

	It("removeOwnerRef should return false if object is not target's owner", func() {
		Expect(removeOwnerRef(target, ownerThree)).Should(BeFalse())
		Expect(target.GetOwnerReferences()).To(HaveLen(2))
	})

	It("removeOwnerRef should remove only passed owner and return true", func() {
		Expect(removeOwnerRef(target, ownerOne)).Should(BeTrue())
		Expect(target.GetOwnerReferences()).To(HaveLen(1))
		Expect(target.GetOwnerReferences()[0].UID).To(BeEquivalentTo(ownerTwo.GetUID()))
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

		It("returns that it updated the finalizer", func() {
			Expect(updatedFinalizer).To(BeTrue())
		})

		It("sets an appropriate log line", func() {
			Expect(logger.Entries()).To(ConsistOf(
				test.LogEntry{
					Level:   2,
					Message: "Added finalizer to control plane machine set",
				},
			))
		})

		It("ensures the finalizer is set on the API", func() {
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

		It("sets an appropriate log line", func() {
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

		It("should return a conflict error", func() {
			Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonConflict))
		})

		It("returns that it did not update the finalizer", func() {
			Expect(updatedFinalizer).To(BeFalse())
		})

		It("does not log", func() {
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
		// Set TypeMeta because Create() call removes it from the object.
		cpms.TypeMeta = metav1.TypeMeta{
			Kind:       "ControlPlaneMachineSet",
			APIVersion: "machine.openshift.io/v1",
		}

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

			machineInfo := resourcebuilder.MachineInfo().WithMachineGVR(machineGVR).WithMachineName(machine.GetName()).WithMachineNamespace(namespaceName).Build()
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

		It("should add the expected owner references", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		It("should log that it has updated the owner references", func() {
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

			Expect(machineInfos).To(HaveLen(3))
			Expect(machines).To(HaveLen(3))

			for i := range machineInfos {
				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}
				patchBase := client.MergeFrom(machines[i].DeepCopy())
				machines[i].SetOwnerReferences([]metav1.OwnerReference{expectedOwnerReference})
				Expect(k8sClient.Patch(ctx, machines[i], patchBase)).To(Succeed())
			}

			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not update the owner references", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		It("should log that no update was needed", func() {
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
			Expect(machineInfos).To(HaveLen(3))
			Expect(machines).To(HaveLen(3))

			for i := range machineInfos {
				if i == 0 {
					continue
				}

				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}

				patchBase := client.MergeFrom(machines[i].DeepCopy())
				machines[i].SetOwnerReferences([]metav1.OwnerReference{expectedOwnerReference})
				Expect(k8sClient.Patch(ctx, machines[i], patchBase)).To(Succeed())
			}

			err := reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should update the owner reference where needed", func() {
			for _, machine := range machines {
				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		It("should log the update that was needed", func() {
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

			for i, machineInfo := range machineInfos {
				if i == 0 {
					continue
				}

				for j := range machineInfo {
					Expect(machineInfo[j].MachineRef).ToNot(BeNil())
					expectedEntries = append(expectedEntries, test.LogEntry{
						KeysAndValues: []interface{}{"machineNamespace", machineInfo[j].MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo[j].MachineRef.ObjectMeta.GetName()},
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

		It("should add the expected owner references", func() {
			for _, machine := range machines {
				if machine.GetName() == skipMachine {
					continue
				}

				Eventually(komega.Object(machine)).Should(HaveField("ObjectMeta.OwnerReferences", ConsistOf(expectedOwnerReference)))
			}
		})

		It("should log that it has updated the owner references", func() {
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
			Expect(machines).To(HaveLen(3))

			badOwnerReference := expectedOwnerReference
			badOwnerReference.Name = "different-owner"

			for i := range machineInfos {
				if i == 0 {
					By("Adding an owner reference for an alternative controller owner")
					Expect(machineInfos[0][0].MachineRef).ToNot(BeNil())
					machineInfos[i][i].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{badOwnerReference}

					continue
				}

				for j := range machineInfos[i] {
					Expect(machineInfos[i][j].MachineRef).ToNot(BeNil())
					machineInfos[i][j].MachineRef.ObjectMeta.OwnerReferences = []metav1.OwnerReference{expectedOwnerReference}
				}

				patchBase := client.MergeFrom(machines[i].DeepCopy())
				machines[i].SetOwnerReferences([]metav1.OwnerReference{expectedOwnerReference})
				Expect(k8sClient.Patch(ctx, machines[i], patchBase)).To(Succeed())
			}

			By("Formulating the expected error")
			machine := &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{Kind: "Machine",
					APIVersion: "machine.openshift.io/v1beta1",
				},
				ObjectMeta: machineInfos[0][0].MachineRef.ObjectMeta,
			}
			machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{badOwnerReference}

			expectedError = controllerutil.SetControllerReference(cpms, machine, testScheme)
			Expect(expectedError).To(HaveOccurred())

			err = reconciler.ensureOwnerReferences(ctx, logger.Logger(), cpms, machineInfos)
		})

		It("should not return an error", func() {
			// Do not return an error as this would cause the controller to requeue.
			// The degraded conditions check will ensure we do not take any further action.
			Expect(err).ToNot(HaveOccurred())
		})

		It("should add an error log", func() {
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

			for i, machineInfo := range machineInfos {
				if i == 0 {
					continue
				}
				for j := range machineInfo {
					Expect(machineInfo[j].MachineRef).ToNot(BeNil())
					expectedEntries = append(expectedEntries, test.LogEntry{
						KeysAndValues: []interface{}{"machineNamespace", machineInfo[j].MachineRef.ObjectMeta.GetNamespace(), "machineName", machineInfo[j].MachineRef.ObjectMeta.GetName()},
						Level:         4,
						Message:       "Owner reference already present on machine",
					})
				}
			}

			Expect(logger.Entries()).To(ConsistOf(expectedEntries))
		})

		It("should set the degraded condition on the ControlPlaneMachineSet", func() {
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
		cpmsBuilder   resourcebuilder.ControlPlaneMachineSetInterface
		machineInfos  []machineproviders.MachineInfo
		expected      map[int32][]machineproviders.MachineInfo
		expectedError error
	}

	DescribeTable("should sort Machine Infos by index", func(in tableInput) {
		cpms := in.cpmsBuilder.Build()
		out, err := machineInfosByIndex(cpms, in.machineInfos)
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
			// Use a custom BuildFunc to set Spec.Replicas to nil,
			// as that's not possbile with the standard Builder.
			cpmsBuilder: &resourcebuilder.ControlPlaneMachineSetFuncs{
				BuildFunc: func() *machinev1.ControlPlaneMachineSet {
					cpmsBuilder := resourcebuilder.ControlPlaneMachineSet()
					cpms := cpmsBuilder.Build()
					cpms.Spec.Replicas = nil

					return cpms
				},
			},
			expectedError: errReplicasRequired,
		}),
		Entry("no machine infos with 3 replicas", tableInput{
			cpmsBuilder:  resourcebuilder.ControlPlaneMachineSet().WithReplicas(3),
			machineInfos: []machineproviders.MachineInfo{},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {},
				1: {},
				2: {},
			},
		}),
		Entry("separately indexed machines", tableInput{
			cpmsBuilder:  resourcebuilder.ControlPlaneMachineSet().WithReplicas(3),
			machineInfos: []machineproviders.MachineInfo{i0m0, i1m0, i2m0},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0},
				1: {i1m0},
				2: {i2m0},
			},
		}),
		Entry("a mixture of indexed machines", tableInput{
			cpmsBuilder:  resourcebuilder.ControlPlaneMachineSet().WithReplicas(3),
			machineInfos: []machineproviders.MachineInfo{i0m0, i1m0, i2m0, i0m1, i1m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
				1: {i1m0, i1m1},
				2: {i2m0},
			},
		}),
		Entry("all machines in the same index with 1 replica", tableInput{
			cpmsBuilder:  resourcebuilder.ControlPlaneMachineSet().WithReplicas(1),
			machineInfos: []machineproviders.MachineInfo{i0m0, i0m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
			},
		}),
		Entry("all machines in the same index with 3 replicas", tableInput{
			cpmsBuilder:  resourcebuilder.ControlPlaneMachineSet().WithReplicas(3),
			machineInfos: []machineproviders.MachineInfo{i0m0, i0m1, i0m2},
			expected: map[int32][]machineproviders.MachineInfo{
				0: {i0m0, i0m1, i0m2},
				1: {},
				2: {},
			},
		}),
	)
})

var _ = Describe("validateClusterState", func() {
	var namespaceName string

	cpmsBuilder := resourcebuilder.ControlPlaneMachineSet()
	masterNodeBuilder := resourcebuilder.Node().AsMaster()
	workerNodeBuilder := resourcebuilder.Node().AsWorker()
	degradedConditionBuilder := resourcebuilder.StatusCondition().WithType(conditionDegraded)
	progressingConditionBuilder := resourcebuilder.StatusCondition().WithType(conditionProgressing)

	machineGVR := machinev1beta1.GroupVersion.WithResource("machines")
	nodeGVR := corev1.SchemeGroupVersion.WithResource("nodes")

	updatedMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithNodeGVR(nodeGVR).
		WithReady(true).
		WithNeedsUpdate(false)

	pendingMachineBuilder := resourcebuilder.MachineInfo().
		WithMachineGVR(machineGVR).
		WithReady(false).
		WithNeedsUpdate(false)

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()
	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
		)
	})

	type validateClusterTableInput struct {
		cpmsBuilder        resourcebuilder.ControlPlaneMachineSetInterface
		machineInfos       map[int32][]machineproviders.MachineInfo
		nodes              []*corev1.Node
		expectedError      error
		expectedConditions []metav1.Condition
		expectedLogs       []test.LogEntry
	}

	DescribeTable("should validate the cluster state", func(in validateClusterTableInput) {
		logger := test.NewTestLogger()

		for _, node := range in.nodes {
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
		}

		reconciler := &ControlPlaneMachineSetReconciler{
			Client:    k8sClient,
			Namespace: namespaceName,
		}

		cpms := in.cpmsBuilder.Build()

		err := reconciler.validateClusterState(ctx, logger.Logger(), cpms, in.machineInfos)

		if in.expectedError != nil {
			Expect(err).To(MatchError(in.expectedError))
		} else {
			Expect(err).ToNot(HaveOccurred())
		}

		Expect(cpms.Status.Conditions).To(test.MatchConditions(in.expectedConditions))
		Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
	},
		Entry("with a valid cluster state", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with a valid cluster state and pre-existing conditions", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonMachinesAlreadyOwned).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonMachinesAlreadyOwned).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with no machines are ready", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionTrue).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {pendingMachineBuilder.WithIndex(0).WithMachineName("machine-0").Build()},
				1: {pendingMachineBuilder.WithIndex(1).WithMachineName("machine-1").Build()},
				2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonNoReadyMachines).WithMessage("No ready control plane machines found").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: errors.New("no ready control plane machines"),
					KeysAndValues: []interface{}{
						"unreadyMachines", "machine-0,machine-1,machine-2",
					},
					Message: "No ready control plane machines found",
				},
			},
		}),
		Entry("with only 1 machine is ready", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionTrue).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {pendingMachineBuilder.WithIndex(0).WithMachineName("machine-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {pendingMachineBuilder.WithIndex(2).WithMachineName("machine-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonUnmanagedNodes).WithMessage("Found 2 unmanaged node(s)").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundUnmanagedControlPlaneNodes, "master-0, master-2"),
					KeysAndValues: []interface{}{
						"unmanagedNodes", "master-0,master-2",
					},
					Message: "Observed unmanaged control plane nodes",
				},
			},
		}),
		Entry("with an additional unowned master node", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionTrue).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				masterNodeBuilder.WithName("master-3").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonUnmanagedNodes).WithMessage("Found 1 unmanaged node(s)").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundUnmanagedControlPlaneNodes, "master-3"),
					KeysAndValues: []interface{}{
						"unmanagedNodes", "master-3",
					},
					Message: "Observed unmanaged control plane nodes",
				},
			},
		}),
		Entry("with a failed machine", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").WithErrorMessage("Instance missing").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with a failed replacement machine", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").WithNeedsUpdate(true).Build(),
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithErrorMessage("Could not create new instance").WithReady(false).Build(),
				},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").WithNeedsUpdate(true).Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").WithNeedsUpdate(true).Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonFailedReplacement).WithMessage("Observed 1 replacement machine(s) in error state").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundErroredReplacementControlPlaneMachine, "machine-replacement-0"),
					KeysAndValues: []interface{}{
						"failedReplacements", "machine-replacement-0",
					},
					Message: "Observed failed replacement control plane machines",
				},
			},
		}),
		Entry("with multiple failed replacement machines", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").WithNeedsUpdate(true).Build(),
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-replacement-0").WithErrorMessage("Could not create new instance").WithReady(false).Build(),
				},
				1: {
					updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").WithNeedsUpdate(true).Build(),
					updatedMachineBuilder.WithIndex(1).WithMachineName("machine-replacement-1").WithErrorMessage("Could not create new instance").WithReady(false).Build(),
				},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").WithNeedsUpdate(true).Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonFailedReplacement).WithMessage("Observed 2 replacement machine(s) in error state").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundErroredReplacementControlPlaneMachine, "machine-replacement-0, machine-replacement-1"),
					KeysAndValues: []interface{}{
						"failedReplacements", "machine-replacement-0,machine-replacement-1",
					},
					Message: "Observed failed replacement control plane machines",
				},
			},
		}),
		Entry("with multiple updated machines in a single index and RollingUpdate strategy", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}).WithReplicas(3),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build(),
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build(),
				},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with multiple updated machines in a single index and OnDelete strategy", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}).WithReplicas(3).WithStrategyType(machinev1.OnDelete),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build(),
					updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build(),
				},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonExcessUpdatedReplicas).WithMessage("Observed 1 updated machine(s) in excess for index 0").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},

			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundExcessiveUpdatedReplicas, "1 updated replica(s) are in excess for index 0"),
					KeysAndValues: []interface{}{
						"excessUpdatedReplicas", 1,
					},
					Message: "Observed an excessive number of updated replica(s) for a single index",
				},
			},
		}),
		Entry("with the desired number of control plane indexes", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}).WithReplicas(3),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with a less than desired number of control plane indexes", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}).WithReplicas(3),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			},
			expectedLogs: []test.LogEntry{},
		}),
		Entry("with an excess in number of control plane indexes", validateClusterTableInput{
			cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).Build(),
			}).WithReplicas(3),
			machineInfos: map[int32][]machineproviders.MachineInfo{
				0: {updatedMachineBuilder.WithIndex(0).WithMachineName("machine-0").WithNodeName("master-0").Build()},
				1: {updatedMachineBuilder.WithIndex(1).WithMachineName("machine-1").WithNodeName("master-1").Build()},
				2: {updatedMachineBuilder.WithIndex(2).WithMachineName("machine-2").WithNodeName("master-2").Build()},
				3: {updatedMachineBuilder.WithIndex(3).WithMachineName("machine-3").WithNodeName("master-3").Build()},
			},
			nodes: []*corev1.Node{
				masterNodeBuilder.WithName("master-0").Build(),
				masterNodeBuilder.WithName("master-1").Build(),
				masterNodeBuilder.WithName("master-2").Build(),
				masterNodeBuilder.WithName("master-3").Build(),
				workerNodeBuilder.WithName("worker-0").Build(),
				workerNodeBuilder.WithName("worker-1").Build(),
				workerNodeBuilder.WithName("worker-2").Build(),
			},
			expectedError: nil,
			expectedConditions: []metav1.Condition{
				degradedConditionBuilder.WithStatus(metav1.ConditionTrue).WithReason(reasonExcessIndexes).WithMessage("Observed 1 index(es) in excess").Build(),
				progressingConditionBuilder.WithStatus(metav1.ConditionFalse).WithReason(reasonOperatorDegraded).Build(),
			},
			expectedLogs: []test.LogEntry{
				{
					Error: fmt.Errorf("%w: %s", errFoundExcessiveIndexes, "1 index(es) are in excess"),
					KeysAndValues: []interface{}{
						"excessIndexes", int32(1),
					},
					Message: "Observed an excessive number of control plane machine indexes",
				},
			},
		}),
	)
})

var _ = Describe("isControlPlaneMachineSetDegraded", func() {
	cpmsBuilder := resourcebuilder.ControlPlaneMachineSet()
	degradedConditionBuilder := resourcebuilder.StatusCondition().WithType(conditionDegraded)

	DescribeTable("should determine if the ControlPlaneMachineSet is degraded", func(cpms *machinev1.ControlPlaneMachineSet, expectDegraded bool) {
		Expect(isControlPlaneMachineSetDegraded(cpms)).To(Equal(expectDegraded), "Degraded state of ControlPlaneMachineSet was not as expected")
	},
		Entry("with a CPMS without a degraded condition",
			cpmsBuilder.WithConditions([]metav1.Condition{}).Build(),
			false,
		),
		Entry("with a CPMS with a degraded condition with status false",
			cpmsBuilder.WithConditions([]metav1.Condition{degradedConditionBuilder.WithStatus(metav1.ConditionFalse).Build()}).Build(),
			false,
		),
		Entry("with a CPMS with a degraded condition with status true",
			cpmsBuilder.WithConditions([]metav1.Condition{degradedConditionBuilder.WithStatus(metav1.ConditionTrue).Build()}).Build(),
			true,
		),
	)
})
