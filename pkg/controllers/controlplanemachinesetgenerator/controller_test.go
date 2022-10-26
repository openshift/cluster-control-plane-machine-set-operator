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

package controlplanemachinesetgenerator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	usEast1aSubnet = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1a",
				},
			},
		},
	}

	usEast1bSubnet = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1b",
				},
			},
		},
	}

	usEast1cSubnet = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1c",
				},
			},
		},
	}

	usEast1dSubnet = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1d",
				},
			},
		},
	}

	usEast1eSubnet = machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1e",
				},
			},
		},
	}

	usEast1aFailureDomainBuilder = resourcebuilder.AWSFailureDomain().
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

	usEast1bFailureDomainBuilder = resourcebuilder.AWSFailureDomain().
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

	usEast1cFailureDomainBuilder = resourcebuilder.AWSFailureDomain().
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

	usEast1dFailureDomainBuilder = resourcebuilder.AWSFailureDomain().
					WithAvailabilityZone("us-east-1d").
					WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1d"},
				},
			},
		})

	usEast1eFailureDomainBuilder = resourcebuilder.AWSFailureDomain().
					WithAvailabilityZone("us-east-1e").
					WithSubnet(machinev1.AWSResourceReference{
			Type: machinev1.AWSFiltersReferenceType,
			Filters: &[]machinev1.AWSResourceFilter{
				{
					Name:   "tag:Name",
					Values: []string{"subnet-us-east-1e"},
				},
			},
		})

	usEast1aProviderSpecBuilder = resourcebuilder.AWSProviderSpec().
					WithAvailabilityZone("us-east-1a").
					WithSubnet(usEast1aSubnet)

	usEast1bProviderSpecBuilder = resourcebuilder.AWSProviderSpec().
					WithAvailabilityZone("us-east-1b").
					WithSubnet(usEast1bSubnet)

	usEast1cProviderSpecBuilder = resourcebuilder.AWSProviderSpec().
					WithAvailabilityZone("us-east-1c").
					WithSubnet(usEast1cSubnet)

	usEast1dProviderSpecBuilder = resourcebuilder.AWSProviderSpec().
					WithAvailabilityZone("us-east-1d").
					WithSubnet(usEast1dSubnet)

	usEast1eProviderSpecBuilder = resourcebuilder.AWSProviderSpec().
					WithAvailabilityZone("us-east-1e").
					WithSubnet(usEast1eSubnet)

	cpms5FailureDomainsBuilder = resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
		usEast1aFailureDomainBuilder,
		usEast1bFailureDomainBuilder,
		usEast1cFailureDomainBuilder,
		usEast1dFailureDomainBuilder,
		usEast1eFailureDomainBuilder,
	)

	cpmsInactiveOutdatedBuilder = resourcebuilder.ControlPlaneMachineSet().
					WithState(machinev1.ControlPlaneMachineSetStateInactive).
					WithMachineTemplateBuilder(
			resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilder.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.8xlarge"),
				).
				WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				)),
		)

	cpmsInactiveUpToDateBuilder = resourcebuilder.ControlPlaneMachineSet().
					WithState(machinev1.ControlPlaneMachineSetStateInactive).
					WithMachineTemplateBuilder(
			resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilder.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.2xlarge"),
				).
				WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
					usEast1dFailureDomainBuilder,
					usEast1eFailureDomainBuilder,
				)),
		)

	cpmsActiveOutdatedBuilder = resourcebuilder.ControlPlaneMachineSet().
					WithState(machinev1.ControlPlaneMachineSetStateActive).
					WithMachineTemplateBuilder(
			resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilder.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.8xlarge"),
				).
				WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
					usEast1aFailureDomainBuilder,
					usEast1bFailureDomainBuilder,
					usEast1cFailureDomainBuilder,
				)),
		)

	cpmsActiveUpToDateBuilder = resourcebuilder.ControlPlaneMachineSet().
					WithState(machinev1.ControlPlaneMachineSetStateActive).
					WithMachineTemplateBuilder(
			resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilder.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.2xlarge"),
				).
				WithFailureDomainsBuilder(cpms5FailureDomainsBuilder),
		)
)

var _ = Describe("controlplanemachinesetgenerator controller", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}
	var mgr manager.Manager
	var reconciler *ControlPlaneMachineSetGeneratorReconciler

	var namespaceName string
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine
	var machineSet0, machineSet1, machineSet2, machineSet3, machineSet4 *machinev1beta1.MachineSet

	startManager := func(mgr *manager.Manager) (context.CancelFunc, chan struct{}) {
		mgrCtx, mgrCancel := context.WithCancel(context.Background())
		mgrDone := make(chan struct{})

		go func() {
			defer GinkgoRecover()
			defer close(mgrDone)

			Expect((*mgr).Start(mgrCtx)).To(Succeed())
		}()

		return mgrCancel, mgrDone
	}

	stopManager := func() {
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone
	}

	create3MachineSets := func() {
		machineSetBuilder := resourcebuilder.MachineSet().WithNamespace(namespaceName)
		machineSet0 = machineSetBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilder).WithGenerateName("machineset-us-east-1a-").Build()
		machineSet1 = machineSetBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilder).WithGenerateName("machineset-us-east-1b-").Build()
		machineSet2 = machineSetBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilder).WithGenerateName("machineset-us-east-1c-").Build()

		Expect(k8sClient.Create(ctx, machineSet0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet2)).To(Succeed())
	}

	create5MachineSets := func() {
		create3MachineSets()

		machineSetBuilder := resourcebuilder.MachineSet().WithNamespace(namespaceName)
		machineSet3 = machineSetBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilder).WithGenerateName("machineset-us-east-1d-").Build()
		machineSet4 = machineSetBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilder).WithGenerateName("machineset-us-east-1e-").Build()

		Expect(k8sClient.Create(ctx, machineSet3)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet4)).To(Succeed())
	}

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilder.WithInstanceType("c5.xlarge")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilder.WithInstanceType("c5.2xlarge")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilder.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra := resourcebuilder.Infrastructure().WithName(infrastructureName).AsAWS("test", "eu-west-2").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             testScheme,
			MetricsBindAddress: "0",
			Port:               testEnv.WebhookInstallOptions.LocalServingPort,
			Host:               testEnv.WebhookInstallOptions.LocalServingHost,
			CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")
		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:    mgr.GetClient(),
			Namespace: namespaceName,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

	})

	AfterEach(func() {
		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
			&machinev1beta1.MachineSet{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	JustBeforeEach(func() {
		By("Starting the manager")
		mgrCancel, mgrDone = startManager(&mgr)
	})

	JustAfterEach(func() {
		By("Stopping the manager")
		stopManager()
	})

	Context("when a Control Plane Machine Set doesn't exist", func() {
		BeforeEach(func() {
			cpms = &machinev1.ControlPlaneMachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterControlPlaneMachineSetName,
					Namespace: namespaceName,
				},
			}
		})

		Context("with 3 existing control plane machines", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create5MachineSets()

				By("Creating Control Plane Machines")
				create3CPMachines()
			})

			It("should create the ControlPlaneMachineSet with the expected fields", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())
				Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateInactive))
				Expect(*cpms.Spec.Replicas).To(Equal(int32(3)))
			})

			It("should create the ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-2` given that's the one we created last.
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(machine2.Spec)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				awsMachineProviderConfig := machineProviderSpec.AWS().Config()
				awsMachineProviderConfig.Subnet = machinev1beta1.AWSResourceReference{}
				awsMachineProviderConfig.Placement.AvailabilityZone = ""

				Expect(cpmsProviderSpec.AWS().Config()).To(Equal(awsMachineProviderConfig))
			})

			Context("With additional MachineSets duplicating failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					create3MachineSets()
				})

				It("should create the ControlPlaneMachineSet with only one copy of each failure domain", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilder.BuildFailureDomains()))
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger test.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilder.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()
				Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
				machines := []machinev1beta1.Machine{*machine2}

				By("Invoking the check on whether the number of control plane machines in the cluster is supported")
				logger = test.NewTestLogger()
				isSupportedControlPlaneMachinesNumber = reconciler.isSupportedControlPlaneMachinesNumber(logger.Logger(), machines)
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("should detect the cluster has an unsupported number of control plane machines", func() {
				Expect(isSupportedControlPlaneMachinesNumber).To(BeFalse())
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					test.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"count", 1},
						Message:       unsupportedNumberOfControlPlaneMachines,
					},
				))
			})

		})

		Context("with an unsupported platform", func() {
			var logger test.TestLogger
			BeforeEach(func() {
				By("Creating MachineSets")
				create5MachineSets()

				By("Creating Control Plane Machines")
				machines := create3CPMachines()

				logger = test.NewTestLogger()
				generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), configv1.NonePlatformType, *machines, nil)
				Expect(generatedCPMS).To(BeNil())
				Expect(err).To(MatchError(errUnsupportedPlatform))
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					test.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
						Message:       unsupportedPlatform,
					},
				))
			})

		})
	})

	Context("when a Control Plane Machine Set exists", func() {
		BeforeEach(func() {
			By("Creating MachineSets")
			create5MachineSets()
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsInactiveOutdatedBuilder.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(machine2.Spec)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				awsMachineProviderConfig := machineProviderSpec.AWS().Config()
				awsMachineProviderConfig.Subnet = machinev1beta1.AWSResourceReference{}
				awsMachineProviderConfig.Placement.AvailabilityZone = ""

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1beta1.AWSMachineProviderConfig {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(in)
							if err != nil {
								return machinev1beta1.AWSMachineProviderConfig{}
							}

							return mPS.AWS().Config()
						}, Equal(awsMachineProviderConfig))),
					"The control plane machine provider spec should match the youngest machine's provider spec",
				)

				Expect(oldUID).NotTo(Equal(cpms.UID),
					"The control plane machine set UID should differ with the old one, as it should've been deleted and recreated")
			})

			Context("With additional MachineSets duplicating failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					create3MachineSets()
				})

				It("should update, but not duplicate the failure domains on the ControlPlaneMachineSet", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilder.BuildFailureDomains())))
				})
			})
		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactiveUpToDateBuilder.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet up to date and not change it", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})

		Context("with state Active and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Active Control Plane Machine Set")
				// Create an Active ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsActiveOutdatedBuilder.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the CPMS unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})
		})

		Context("with state Active and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Active Control Plane Machine Set")
				// Create an Active ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsActiveUpToDateBuilder.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})
	})
})
