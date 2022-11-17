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

var _ = Describe("controlplanemachinesetgenerator controller on AWS", func() {

	var (
		usEast1aSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1a",
					},
				},
			},
		}

		usEast1bSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1b",
					},
				},
			},
		}

		usEast1cSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1c",
					},
				},
			},
		}

		usEast1dSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1d",
					},
				},
			},
		}

		usEast1eSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1e",
					},
				},
			},
		}

		usEast1aFailureDomainBuilderAWS = resourcebuilder.AWSFailureDomain().
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

		usEast1bFailureDomainBuilderAWS = resourcebuilder.AWSFailureDomain().
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

		usEast1cFailureDomainBuilderAWS = resourcebuilder.AWSFailureDomain().
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

		usEast1dFailureDomainBuilderAWS = resourcebuilder.AWSFailureDomain().
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

		usEast1eFailureDomainBuilderAWS = resourcebuilder.AWSFailureDomain().
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

		usEast1aProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1a").
						WithSubnet(usEast1aSubnetAWS)

		usEast1bProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1b").
						WithSubnet(usEast1bSubnetAWS)

		usEast1cProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1c").
						WithSubnet(usEast1cSubnetAWS)

		usEast1dProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1d").
						WithSubnet(usEast1dSubnetAWS)

		usEast1eProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1e").
						WithSubnet(usEast1eSubnetAWS)

		cpms3FailureDomainsBuilderAWS = resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAWS,
			usEast1bFailureDomainBuilderAWS,
			usEast1cFailureDomainBuilderAWS,
		)

		cpms5FailureDomainsBuilderAWS = resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAWS,
			usEast1bFailureDomainBuilderAWS,
			usEast1cFailureDomainBuilderAWS,
			usEast1dFailureDomainBuilderAWS,
			usEast1eFailureDomainBuilderAWS,
		)

		cpmsInactive3FDsBuilderAWS = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.8xlarge"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
					)),
			)

		cpmsInactive5FDsBuilderAWS = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.2xlarge"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
						usEast1dFailureDomainBuilderAWS,
						usEast1eFailureDomainBuilderAWS,
					)),
			)

		cpmsActiveOutdatedBuilderAWS = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.8xlarge"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
					)),
			)

		cpmsActiveUpToDateBuilderAWS = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.2xlarge"),
					).
					WithFailureDomainsBuilder(cpms5FailureDomainsBuilderAWS),
			)
	)

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
		machineSet0 = machineSetBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAWS).WithGenerateName("machineset-us-east-1a-").Build()
		machineSet1 = machineSetBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAWS).WithGenerateName("machineset-us-east-1b-").Build()
		machineSet2 = machineSetBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS).WithGenerateName("machineset-us-east-1c-").Build()

		Expect(k8sClient.Create(ctx, machineSet0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet2)).To(Succeed())
	}

	create5MachineSets := func() {
		create3MachineSets()

		machineSetBuilder := resourcebuilder.MachineSet().WithNamespace(namespaceName)
		machineSet3 = machineSetBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAWS).WithGenerateName("machineset-us-east-1d-").Build()
		machineSet4 = machineSetBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAWS).WithGenerateName("machineset-us-east-1e-").Build()

		Expect(k8sClient.Create(ctx, machineSet3)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet4)).To(Succeed())
	}

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAWS.WithInstanceType("c5.2xlarge")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
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

		Context("with 5 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create5MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with 3 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create3MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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

				It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms3FailureDomainsBuilderAWS.BuildFailureDomains()))
				})

				Context("With additional Machines adding additional failure domains", func() {
					BeforeEach(func() {
						By("Creating additional Machines")
						createUsEast1dMachine()
						createUsEast1eMachine()
					})

					It("should create the ControlPlaneMachineSet with only one copy of each the 5 failure domains", func() {
						By("Checking the Control Plane Machine Set has been created")
						Eventually(komega.Get(cpms)).Should(Succeed())

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger test.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()
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

	Context("when a Control Plane Machine Set exists with 5 Machine Sets", func() {
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
				cpms = cpmsInactive3FDsBuilderAWS.WithNamespace(namespaceName).Build()
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
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains())))
				})
			})
		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive5FDsBuilderAWS.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveOutdatedBuilderAWS.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveUpToDateBuilderAWS.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machine Sets", func() {
		BeforeEach(func() {
			By("Creating MachineSets")
			create3MachineSets()
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the failure domains configured.
				cpms = cpmsInactive5FDsBuilderAWS.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms3FailureDomainsBuilderAWS.BuildFailureDomains())))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from Machines, not present in the Machine Sets", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains())))
				})
			})
		})
	})
})

var _ = Describe("controlplanemachinesetgenerator controller on Azure", func() {

	var (
		usEast1aFailureDomainBuilderAzure = resourcebuilder.AzureFailureDomain().WithZone("us-east-1a")

		usEast1bFailureDomainBuilderAzure = resourcebuilder.AzureFailureDomain().WithZone("us-east-1b")

		usEast1cFailureDomainBuilderAzure = resourcebuilder.AzureFailureDomain().WithZone("us-east-1c")

		usEast1dFailureDomainBuilderAzure = resourcebuilder.AzureFailureDomain().WithZone("us-east-1d")

		usEast1eFailureDomainBuilderAzure = resourcebuilder.AzureFailureDomain().WithZone("us-east-1e")

		usEast1aProviderSpecBuilderAzure = resourcebuilder.AzureProviderSpec().WithZone("us-east-1a")

		usEast1bProviderSpecBuilderAzure = resourcebuilder.AzureProviderSpec().WithZone("us-east-1b")

		usEast1cProviderSpecBuilderAzure = resourcebuilder.AzureProviderSpec().WithZone("us-east-1c")

		usEast1dProviderSpecBuilderAzure = resourcebuilder.AzureProviderSpec().WithZone("us-east-1d")

		usEast1eProviderSpecBuilderAzure = resourcebuilder.AzureProviderSpec().WithZone("us-east-1e")

		cpms3FailureDomainsBuilderAzure = resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAzure,
			usEast1bFailureDomainBuilderAzure,
			usEast1cFailureDomainBuilderAzure,
		)

		cpms5FailureDomainsBuilderAzure = resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAzure,
			usEast1bFailureDomainBuilderAzure,
			usEast1cFailureDomainBuilderAzure,
			usEast1dFailureDomainBuilderAzure,
			usEast1eFailureDomainBuilderAzure,
		)

		cpmsInactive3FDsBuilderAzure = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
					)),
			)

		cpmsInactive5FDsBuilderAzure = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
						usEast1dFailureDomainBuilderAzure,
						usEast1eFailureDomainBuilderAzure,
					)),
			)

		cpmsActiveOutdatedBuilderAzure = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
					)),
			)

		cpmsActiveUpToDateBuilderAzure = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(cpms5FailureDomainsBuilderAzure),
			)
	)

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
		machineSet0 = machineSetBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAzure).WithGenerateName("machineset-us-east-1a-").Build()
		machineSet1 = machineSetBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAzure).WithGenerateName("machineset-us-east-1b-").Build()
		machineSet2 = machineSetBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAzure).WithGenerateName("machineset-us-east-1c-").Build()

		Expect(k8sClient.Create(ctx, machineSet0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet2)).To(Succeed())
	}

	create5MachineSets := func() {
		create3MachineSets()

		machineSetBuilder := resourcebuilder.MachineSet().WithNamespace(namespaceName)
		machineSet3 = machineSetBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAzure).WithGenerateName("machineset-us-east-1d-").Build()
		machineSet4 = machineSetBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAzure).WithGenerateName("machineset-us-east-1e-").Build()

		Expect(k8sClient.Create(ctx, machineSet3)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet4)).To(Succeed())
	}

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra := resourcebuilder.Infrastructure().WithName(infrastructureName).AsAzure("test").Build()
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

		Context("with 5 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create5MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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
					azureMachineProviderConfig := machineProviderSpec.Azure().Config()
					azureMachineProviderConfig.Zone = nil

					Expect(cpmsProviderSpec.Azure().Config()).To(Equal(azureMachineProviderConfig))
				})

				Context("With additional MachineSets duplicating failure domains", func() {
					BeforeEach(func() {
						By("Creating additional MachineSets")
						create3MachineSets()
					})

					It("should create the ControlPlaneMachineSet with only one copy of each failure domain", func() {
						By("Checking the Control Plane Machine Set has been created")
						Eventually(komega.Get(cpms)).Should(Succeed())

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with 3 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create3MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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
					azureMachineProviderConfig := machineProviderSpec.Azure().Config()
					azureMachineProviderConfig.Zone = nil

					Expect(cpmsProviderSpec.Azure().Config()).To(Equal(azureMachineProviderConfig))
				})

				It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms3FailureDomainsBuilderAzure.BuildFailureDomains()))
				})

				Context("With additional Machines adding additional failure domains", func() {
					BeforeEach(func() {
						By("Creating additional Machines")
						createUsEast1dMachine()
						createUsEast1eMachine()
					})

					It("should create the ControlPlaneMachineSet with only one copy of each the 5 failure domains", func() {
						By("Checking the Control Plane Machine Set has been created")
						Eventually(komega.Get(cpms)).Should(Succeed())

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger test.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-2").Build()
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

	Context("when a Control Plane Machine Set exists with 5 Machine Sets", func() {
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
				cpms = cpmsInactive3FDsBuilderAzure.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(machine2.Spec)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				azureMachineProviderConfig := machineProviderSpec.Azure().Config()
				azureMachineProviderConfig.Zone = nil

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1beta1.AzureMachineProviderSpec {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(in)
							if err != nil {
								return machinev1beta1.AzureMachineProviderSpec{}
							}

							return mPS.Azure().Config()
						}, Equal(azureMachineProviderConfig))),
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
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains())))
				})
			})
		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive5FDsBuilderAzure.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveOutdatedBuilderAzure.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveUpToDateBuilderAzure.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machine Sets", func() {
		BeforeEach(func() {
			By("Creating MachineSets")
			create3MachineSets()
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the failure domains configured.
				cpms = cpmsInactive5FDsBuilderAzure.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms3FailureDomainsBuilderAzure.BuildFailureDomains())))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from Machines, not present in the Machine Sets", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains())))
				})
			})
		})
	})
})

var _ = Describe("controlplanemachinesetgenerator controller on GCP", func() {

	var (
		usEast1aFailureDomainBuilderGCP = resourcebuilder.GCPFailureDomain().WithZone("us-east-1a")

		usEast1bFailureDomainBuilderGCP = resourcebuilder.GCPFailureDomain().WithZone("us-east-1b")

		usEast1cFailureDomainBuilderGCP = resourcebuilder.GCPFailureDomain().WithZone("us-east-1c")

		usEast1dFailureDomainBuilderGCP = resourcebuilder.GCPFailureDomain().WithZone("us-east-1d")

		usEast1eFailureDomainBuilderGCP = resourcebuilder.GCPFailureDomain().WithZone("us-east-1e")

		usEast1aProviderSpecBuilderGCP = resourcebuilder.GCPProviderSpec().WithZone("us-east-1a")

		usEast1bProviderSpecBuilderGCP = resourcebuilder.GCPProviderSpec().WithZone("us-east-1b")

		usEast1cProviderSpecBuilderGCP = resourcebuilder.GCPProviderSpec().WithZone("us-east-1c")

		usEast1dProviderSpecBuilderGCP = resourcebuilder.GCPProviderSpec().WithZone("us-east-1d")

		usEast1eProviderSpecBuilderGCP = resourcebuilder.GCPProviderSpec().WithZone("us-east-1e")

		cpms3FailureDomainsBuilderGCP = resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderGCP,
			usEast1bFailureDomainBuilderGCP,
			usEast1cFailureDomainBuilderGCP,
		)

		cpms5FailureDomainsBuilderGCP = resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderGCP,
			usEast1bFailureDomainBuilderGCP,
			usEast1cFailureDomainBuilderGCP,
			usEast1dFailureDomainBuilderGCP,
			usEast1eFailureDomainBuilderGCP,
		)

		cpmsInactive3FDsBuilderGCP = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-8"),
					).
					WithFailureDomainsBuilder(resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
					)),
			)

		cpmsInactive5FDsBuilderGCP = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-4"),
					).
					WithFailureDomainsBuilder(resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
						usEast1dFailureDomainBuilderGCP,
						usEast1eFailureDomainBuilderGCP,
					)),
			)

		cpmsActiveOutdatedBuilderGCP = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-8"),
					).
					WithFailureDomainsBuilder(resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
					)),
			)

		cpmsActiveUpToDateBuilderGCP = resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-4"),
					).
					WithFailureDomainsBuilder(cpms5FailureDomainsBuilderGCP),
			)
	)

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
		machineSet0 = machineSetBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderGCP).WithGenerateName("machineset-us-east-1a-").Build()
		machineSet1 = machineSetBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderGCP).WithGenerateName("machineset-us-east-1b-").Build()
		machineSet2 = machineSetBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderGCP).WithGenerateName("machineset-us-east-1c-").Build()

		Expect(k8sClient.Create(ctx, machineSet0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet2)).To(Succeed())
	}

	create5MachineSets := func() {
		create3MachineSets()

		machineSetBuilder := resourcebuilder.MachineSet().WithNamespace(namespaceName)
		machineSet3 = machineSetBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderGCP).WithGenerateName("machineset-us-east-1d-").Build()
		machineSet4 = machineSetBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderGCP).WithGenerateName("machineset-us-east-1e-").Build()

		Expect(k8sClient.Create(ctx, machineSet3)).To(Succeed())
		Expect(k8sClient.Create(ctx, machineSet4)).To(Succeed())
	}

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra := resourcebuilder.Infrastructure().WithName(infrastructureName).AsGCP("test", "region-1").Build()
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

		Context("with 5 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create5MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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
					GCPMachineProviderConfig := machineProviderSpec.GCP().Config()
					GCPMachineProviderConfig.Zone = ""

					Expect(cpmsProviderSpec.GCP().Config()).To(Equal(GCPMachineProviderConfig))
				})

				Context("With additional MachineSets duplicating failure domains", func() {
					BeforeEach(func() {
						By("Creating additional MachineSets")
						create3MachineSets()
					})

					It("should create the ControlPlaneMachineSet with only one copy of each failure domain", func() {
						By("Checking the Control Plane Machine Set has been created")
						Eventually(komega.Get(cpms)).Should(Succeed())

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with 3 Machine Sets", func() {
			BeforeEach(func() {
				By("Creating MachineSets")
				create3MachineSets()
			})

			Context("with 3 existing control plane machines", func() {
				BeforeEach(func() {
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
					GCPMachineProviderConfig := machineProviderSpec.GCP().Config()
					GCPMachineProviderConfig.Zone = ""

					Expect(cpmsProviderSpec.GCP().Config()).To(Equal(GCPMachineProviderConfig))
				})

				It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms3FailureDomainsBuilderGCP.BuildFailureDomains()))
				})

				Context("With additional Machines adding additional failure domains", func() {
					BeforeEach(func() {
						By("Creating additional Machines")
						createUsEast1dMachine()
						createUsEast1eMachine()
					})

					It("should create the ControlPlaneMachineSet with only one copy of each the 5 failure domains", func() {
						By("Checking the Control Plane Machine Set has been created")
						Eventually(komega.Get(cpms)).Should(Succeed())

						Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains()))
					})
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger test.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-2").Build()
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

	Context("when a Control Plane Machine Set exists with 5 Machine Sets", func() {
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
				cpms = cpmsInactive3FDsBuilderGCP.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(machine2.Spec)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				gcpMachineProviderConfig := machineProviderSpec.GCP().Config()
				gcpMachineProviderConfig.Zone = ""

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1beta1.GCPMachineProviderSpec {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(in)
							if err != nil {
								return machinev1beta1.GCPMachineProviderSpec{}
							}

							return mPS.GCP().Config()
						}, Equal(gcpMachineProviderConfig))),
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
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains())))
				})
			})
		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive5FDsBuilderGCP.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveOutdatedBuilderGCP.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveUpToDateBuilderGCP.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machine Sets", func() {
		BeforeEach(func() {
			By("Creating MachineSets")
			create3MachineSets()
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the failure domains configured.
				cpms = cpmsInactive5FDsBuilderGCP.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms3FailureDomainsBuilderGCP.BuildFailureDomains())))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from Machines, not present in the Machine Sets", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains())))
				})
			})
		})
	})
})
