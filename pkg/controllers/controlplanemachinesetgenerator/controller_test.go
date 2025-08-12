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

package controlplanemachinesetgenerator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"
)

const (
	// infrastructureName is the name of the Infrastructure,
	// as Infrastructure is a singleton within the cluster.
	infrastructureName = "cluster"
)

// Helper method to create the ClusterVersion "version" resource.
func createClusterVersion() {
	clusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			Channel:   "stable-4.14",
			ClusterID: configv1.ClusterID("086c77e9-ce27-4fa4-8caa-10ebf8237d53"),
		},
		Status: configv1.ClusterVersionStatus{
			Desired: configv1.Release{
				Image:   "blah",
				URL:     "blah",
				Version: "4.14.0",
			},
		},
	}
	cvStatus := clusterVersion.Status.DeepCopy()
	Expect(k8sClient.Create(ctx, clusterVersion)).To(Succeed())
	clusterVersion.Status = *cvStatus
	Expect(k8sClient.Status().Update(ctx, clusterVersion)).To(Succeed())
}

// Helper method to crate the FeatureGate "cluster" resource.
func createFeatureGate() {
	featureGate := &configv1.FeatureGate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.FeatureGateSpec{
			FeatureGateSelection: configv1.FeatureGateSelection{
				FeatureSet: configv1.FeatureSet("TechPreviewNoUpgrade"),
			},
		},
		Status: configv1.FeatureGateStatus{
			FeatureGates: []configv1.FeatureGateDetails{
				{
					Enabled: []configv1.FeatureGateAttributes{
						{
							Name: "foo",
						},
					},
					Disabled: []configv1.FeatureGateAttributes{
						{
							Name: "bar",
						},
					},
					Version: "4.14.0",
				},
			},
		},
	}
	fgStatus := featureGate.Status.DeepCopy()
	Expect(k8sClient.Create(ctx, featureGate)).To(Succeed())
	featureGate.Status = *fgStatus
	Expect(k8sClient.Status().Update(ctx, featureGate)).To(Succeed())
}

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

		usEast1aFailureDomainBuilderAWS = machinev1resourcebuilder.AWSFailureDomain().
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

		usEast1bFailureDomainBuilderAWS = machinev1resourcebuilder.AWSFailureDomain().
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

		usEast1cFailureDomainBuilderAWS = machinev1resourcebuilder.AWSFailureDomain().
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

		usEast1dFailureDomainBuilderAWS = machinev1resourcebuilder.AWSFailureDomain().
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

		usEast1eFailureDomainBuilderAWS = machinev1resourcebuilder.AWSFailureDomain().
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

		usEast1aProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1a").
						WithSubnet(usEast1aSubnetAWS)

		usEast1bProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1b").
						WithSubnet(usEast1bSubnetAWS)

		usEast1cProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1c").
						WithSubnet(usEast1cSubnetAWS)

		usEast1dProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1d").
						WithSubnet(usEast1dSubnetAWS)

		usEast1eProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1e").
						WithSubnet(usEast1eSubnetAWS)

		cpms3FailureDomainsBuilderAWS = machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAWS,
			usEast1bFailureDomainBuilderAWS,
			usEast1cFailureDomainBuilderAWS,
		)

		cpms5FailureDomainsBuilderAWS = machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAWS,
			usEast1bFailureDomainBuilderAWS,
			usEast1cFailureDomainBuilderAWS,
			usEast1dFailureDomainBuilderAWS,
			usEast1eFailureDomainBuilderAWS,
		)

		cpmsInactive3FDsBuilderAWS = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.4xlarge"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
					)),
			)

		cpmsInactiveOutdated3FDsBuilderAWS = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateInactive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("outdatedInstancetype"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
					)),
			)

		cpmsActiveOutdatedBuilderAWS = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAWS.WithAvailabilityZone("").WithSubnet(machinev1beta1.AWSResourceReference{}).WithInstanceType("c5.8xlarge"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAWS,
						usEast1bFailureDomainBuilderAWS,
						usEast1cFailureDomainBuilderAWS,
					)),
			)

		cpmsActiveUpToDateBuilderAWS = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
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
	var infra *configv1.Infrastructure
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine

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

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAWS.WithInstanceType("c5.2xlarge")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	create3CPMachinesWithVaryingAuthoritativeAPI := func() *[]machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAWS).WithName("master-0").WithAuthoritativeAPI(machinev1beta1.MachineAuthorityMachineAPI).Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAWS).WithName("master-1").WithAuthoritativeAPI(machinev1beta1.MachineAuthorityClusterAPI).Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS).WithName("master-2").WithAuthoritativeAPI(machinev1beta1.MachineAuthorityClusterAPI).Build()

		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAWS.WithInstanceType("c5.xlarge")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra = configv1resourcebuilder.Infrastructure().WithName(infrastructureName).AsAWS("test", "eu-west-2").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, managerOptions)
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		featureGateAccessor, err := util.SetupFeatureGateAccessor(ctx, mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:              mgr.GetClient(),
			Namespace:           namespaceName,
			FeatureGateAccessor: featureGateAccessor,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
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
		Context("with 3 existing control plane machines with varying AuthoritativeAPI values", func() {
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				create3CPMachinesWithVaryingAuthoritativeAPI()
			})

			It("should create stable ControlPlaneMachineSet with the AuthoritativeAPI defaulted to MachineAPI", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())

				By("Checking the Control Plane Machine Set has the AuthoritativeAPI defaulted to MachineAPI")
				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.AuthoritativeAPI).To(Equal(machinev1beta1.MachineAuthorityMachineAPI))

				By("Checking the Control Plane Machine Set is stable")
				cpmsUID := cpms.UID
				Consistently(komega.Object(cpms)).Should(And(HaveField("UID", cpmsUID), HaveField("DeletionTimestamp", BeNil())))
			})
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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				awsMachineProviderConfig := machineProviderSpec.AWS().Config()
				awsMachineProviderConfig.Subnet = machinev1beta1.AWSResourceReference{}
				awsMachineProviderConfig.Placement.AvailabilityZone = ""

				Expect(cpmsProviderSpec.AWS().Config()).To(Equal(awsMachineProviderConfig))

				// The ControlPlaneMachineSet should have the AuthoritativeAPI defaulted to MachineAPI.
				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.AuthoritativeAPI).To(Equal(machinev1beta1.MachineAuthorityMachineAPI))

			})

			It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())

				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms3FailureDomainsBuilderAWS.BuildFailureDomains())))
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

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains())))
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger testutils.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAWS.WithInstanceType("c5.4xlarge")).WithName("master-2").Build()
				Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
				machines := []machinev1beta1.Machine{*machine2}

				By("Invoking the check on whether the number of control plane machines in the cluster is supported")
				logger = testutils.NewTestLogger()
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
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"count", 1},
						Message:       unsupportedNumberOfControlPlaneMachines,
					},
				))
			})

		})

		Context("with an unsupported platform", func() {
			var logger testutils.TestLogger
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				machines := create3CPMachines()

				Eventually(komega.UpdateStatus(infra, func() {
					infra.Status.PlatformStatus.Type = configv1.NonePlatformType
				})).Should(Succeed())

				logger = testutils.NewTestLogger()
				generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), infra, *machines)
				Expect(generatedCPMS).To(BeNil())
				Expect(err).To(MatchError(errUnsupportedPlatform))
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
						Message:       unsupportedPlatform,
					},
				))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machines", func() {
		BeforeEach(func() {
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsInactiveOutdated3FDsBuilderAWS.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
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
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), in, nil)
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

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms), 1*time.Second).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms3FailureDomainsBuilderAWS.BuildFailureDomains()))))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from the created Machines", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms5FailureDomainsBuilderAWS.BuildFailureDomains()))))
				})
			})

		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive3FDsBuilderAWS.WithNamespace(namespaceName).Build()
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

})

var _ = Describe("controlplanemachinesetgenerator controller on Azure", func() {

	var (
		usEast1aFailureDomainBuilderAzure = machinev1resourcebuilder.AzureFailureDomain().WithZone("us-east-1a")

		usEast1bFailureDomainBuilderAzure = machinev1resourcebuilder.AzureFailureDomain().WithZone("us-east-1b")

		usEast1cFailureDomainBuilderAzure = machinev1resourcebuilder.AzureFailureDomain().WithZone("us-east-1c")

		usEast1dFailureDomainBuilderAzure = machinev1resourcebuilder.AzureFailureDomain().WithZone("us-east-1d")

		usEast1eFailureDomainBuilderAzure = machinev1resourcebuilder.AzureFailureDomain().WithZone("us-east-1e")

		usEast1aProviderSpecBuilderAzure = machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("us-east-1a")

		usEast1bProviderSpecBuilderAzure = machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("us-east-1b")

		usEast1cProviderSpecBuilderAzure = machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("us-east-1c")

		usEast1dProviderSpecBuilderAzure = machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("us-east-1d")

		usEast1eProviderSpecBuilderAzure = machinev1beta1resourcebuilder.AzureProviderSpec().WithZone("us-east-1e")

		cpms3FailureDomainsBuilderAzure = machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAzure,
			usEast1bFailureDomainBuilderAzure,
			usEast1cFailureDomainBuilderAzure,
		)

		cpms5FailureDomainsBuilderAzure = machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderAzure,
			usEast1bFailureDomainBuilderAzure,
			usEast1cFailureDomainBuilderAzure,
			usEast1dFailureDomainBuilderAzure,
			usEast1eFailureDomainBuilderAzure,
		)

		cpmsInactiveOutdated3FDsBuilderAzure = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateInactive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("outdatedInstancetype"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
					)),
			)

		cpmsInactive3FDsBuilderAzure = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
					)),
			)

		cpmsActiveOutdatedBuilderAzure = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderAzure,
						usEast1bFailureDomainBuilderAzure,
						usEast1cFailureDomainBuilderAzure,
					)),
			)

		cpmsActiveUpToDateBuilderAzure = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderAzure.WithZone("").WithVMSize("defaultinstancetype"),
					).
					WithFailureDomainsBuilder(cpms3FailureDomainsBuilderAzure),
			)
	)

	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}
	var mgr manager.Manager
	var reconciler *ControlPlaneMachineSetGeneratorReconciler

	var namespaceName string
	var infra *configv1.Infrastructure
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine

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

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra = configv1resourcebuilder.Infrastructure().WithName(infrastructureName).AsAzure("test").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, managerOptions)
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		featureGateAccessor, err := util.SetupFeatureGateAccessor(ctx, mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:              mgr.GetClient(),
			Namespace:           namespaceName,
			FeatureGateAccessor: featureGateAccessor,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				azureMachineProviderConfig := machineProviderSpec.Azure().Config()
				azureMachineProviderConfig.Zone = ""

				Expect(cpmsProviderSpec.Azure().Config()).To(Equal(azureMachineProviderConfig))
			})

			It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())

				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms3FailureDomainsBuilderAzure.BuildFailureDomains())))
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

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains())))
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger testutils.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderAzure.WithVMSize("defaultinstancetype")).WithName("master-2").Build()
				Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
				machines := []machinev1beta1.Machine{*machine2}

				By("Invoking the check on whether the number of control plane machines in the cluster is supported")
				logger = testutils.NewTestLogger()
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
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"count", 1},
						Message:       unsupportedNumberOfControlPlaneMachines,
					},
				))
			})

		})

		Context("with an unsupported platform", func() {
			var logger testutils.TestLogger
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				machines := create3CPMachines()

				Eventually(komega.UpdateStatus(infra, func() {
					infra.Status.PlatformStatus.Type = configv1.NonePlatformType
				})).Should(Succeed())

				logger = testutils.NewTestLogger()
				generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), infra, *machines)
				Expect(generatedCPMS).To(BeNil())
				Expect(err).To(MatchError(errUnsupportedPlatform))
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
						Message:       unsupportedPlatform,
					},
				))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machines", func() {
		BeforeEach(func() {
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsInactiveOutdated3FDsBuilderAzure.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				azureMachineProviderConfig := machineProviderSpec.Azure().Config()
				azureMachineProviderConfig.Zone = ""

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1beta1.AzureMachineProviderSpec {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), in, nil)
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

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms3FailureDomainsBuilderAzure.BuildFailureDomains()))))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from the created Machines", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms5FailureDomainsBuilderAzure.BuildFailureDomains()))))
				})
			})

		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive3FDsBuilderAzure.WithNamespace(namespaceName).Build()
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

})

var _ = Describe("controlplanemachinesetgenerator controller on GCP", func() {

	var (
		usEast1aFailureDomainBuilderGCP = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-east-1a")

		usEast1bFailureDomainBuilderGCP = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-east-1b")

		usEast1cFailureDomainBuilderGCP = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-east-1c")

		usEast1dFailureDomainBuilderGCP = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-east-1d")

		usEast1eFailureDomainBuilderGCP = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-east-1e")

		usEast1aProviderSpecBuilderGCP = machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-east-1a")

		usEast1bProviderSpecBuilderGCP = machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-east-1b")

		usEast1cProviderSpecBuilderGCP = machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-east-1c")

		usEast1dProviderSpecBuilderGCP = machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-east-1d")

		usEast1eProviderSpecBuilderGCP = machinev1beta1resourcebuilder.GCPProviderSpec().WithZone("us-east-1e")

		cpms3FailureDomainsBuilderGCP = machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderGCP,
			usEast1bFailureDomainBuilderGCP,
			usEast1cFailureDomainBuilderGCP,
		)

		cpms5FailureDomainsBuilderGCP = machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
			usEast1aFailureDomainBuilderGCP,
			usEast1bFailureDomainBuilderGCP,
			usEast1cFailureDomainBuilderGCP,
			usEast1dFailureDomainBuilderGCP,
			usEast1eFailureDomainBuilderGCP,
		)

		cpmsInactive3FDsBuilderGCP = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateInactive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-8"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
					)),
			)

		cpmsInactiveOutdated3FDsBuilderGCP = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateInactive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("outdatedMachineType"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
					)),
			)

		cpmsActiveOutdatedBuilderGCP = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						usEast1aProviderSpecBuilderGCP.WithZone("").WithMachineType("n1-standard-8"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usEast1aFailureDomainBuilderGCP,
						usEast1bFailureDomainBuilderGCP,
						usEast1cFailureDomainBuilderGCP,
					)),
			)

		cpmsActiveUpToDateBuilderGCP = machinev1resourcebuilder.ControlPlaneMachineSet().
						WithState(machinev1.ControlPlaneMachineSetStateActive).
						WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
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
	var infra *configv1.Infrastructure
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine

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

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(usEast1aProviderSpecBuilderGCP.WithMachineType("n1-standard-2")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(usEast1bProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderGCP.WithMachineType("n1-standard-8")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createUsEast1dMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1dProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createUsEast1eMachine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(usEast1eProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra = configv1resourcebuilder.Infrastructure().WithName(infrastructureName).AsGCP("test", "region-1").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, managerOptions)
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		featureGateAccessor, err := util.SetupFeatureGateAccessor(ctx, mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:              mgr.GetClient(),
			Namespace:           namespaceName,
			FeatureGateAccessor: featureGateAccessor,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				GCPMachineProviderConfig := machineProviderSpec.GCP().Config()
				GCPMachineProviderConfig.Zone = ""

				Expect(cpmsProviderSpec.GCP().Config()).To(Equal(GCPMachineProviderConfig))
			})

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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
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

				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms3FailureDomainsBuilderGCP.BuildFailureDomains())))
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

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains())))
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger testutils.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(usEast1cProviderSpecBuilderGCP.WithMachineType("n1-standard-4")).WithName("master-2").Build()
				Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
				machines := []machinev1beta1.Machine{*machine2}

				By("Invoking the check on whether the number of control plane machines in the cluster is supported")
				logger = testutils.NewTestLogger()
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
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"count", 1},
						Message:       unsupportedNumberOfControlPlaneMachines,
					},
				))
			})

		})

		Context("with an unsupported platform", func() {
			var logger testutils.TestLogger
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				machines := create3CPMachines()

				Eventually(komega.UpdateStatus(infra, func() {
					infra.Status.PlatformStatus.Type = configv1.NonePlatformType
				})).Should(Succeed())

				logger = testutils.NewTestLogger()
				generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), infra, *machines)
				Expect(generatedCPMS).To(BeNil())
				Expect(err).To(MatchError(errUnsupportedPlatform))
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
						Message:       unsupportedPlatform,
					},
				))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machines", func() {
		BeforeEach(func() {
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsInactiveOutdated3FDsBuilderGCP.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				gcpMachineProviderConfig := machineProviderSpec.GCP().Config()
				gcpMachineProviderConfig.Zone = ""

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1beta1.GCPMachineProviderSpec {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), in, nil)
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

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms3FailureDomainsBuilderGCP.BuildFailureDomains()))))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createUsEast1dMachine()
					createUsEast1eMachine()
				})

				It("should include additional failure domains from the created Machines", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms5FailureDomainsBuilderGCP.BuildFailureDomains()))))
				})
			})

		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive3FDsBuilderGCP.WithNamespace(namespaceName).Build()
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

})

// For testing controlplanemachinesetgenerator controller on Nutanix.
var _ = Describe("controlplanemachinesetgenerator controller on Nutanix", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}
	var mgr manager.Manager
	var reconciler *ControlPlaneMachineSetGeneratorReconciler

	var namespaceName string
	var infra *configv1.Infrastructure
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine

	createInfrastructure := func(withFailureDomains bool) *configv1.Infrastructure {
		By("Setting up a new infrastructure for the test")

		infraBuilder := configv1resourcebuilder.Infrastructure().WithName(util.InfrastructureName)
		if withFailureDomains {
			infra = infraBuilder.AsNutanixWithFailureDomains("nutanix-test", nil).Build()
		} else {
			infra = infraBuilder.AsNutanix("nutanix-test").Build()
		}

		Expect(infra).ToNot(BeNil(), "Expected infrastructure object not to be nil")
		Expect(infra.Spec.PlatformSpec.Nutanix).ToNot(BeNil(), "Expected Nutanix platform spec to be populated")
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		if withFailureDomains {
			Expect(infra.Spec.PlatformSpec.Nutanix.FailureDomains).To(HaveLen(3), "Expected 3 failure domains for the nutanix platform spec")
		} else {
			Expect(infra.Spec.PlatformSpec.Nutanix.FailureDomains).To(HaveLen(0), "Expected no failure domains for the nutanix platform spec")
		}

		return infra
	}

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

	create3CPMachines := func(infra *configv1.Infrastructure, withFailureDomain bool) *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		providerSpecBuilder := machinev1resourcebuilder.NewNutanixMachineProviderConfigBuilder()

		fds := infra.Spec.PlatformSpec.Nutanix.FailureDomains
		if withFailureDomain && len(fds) > 0 {
			providerSpecBuilder = providerSpecBuilder.WithFailureDomains(fds)
			machine0 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder.WithFailureDomainName(fds[0%len(fds)].Name)).WithName("master-0").Build()
			machine1 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder.WithFailureDomainName(fds[1%len(fds)].Name)).WithName("master-1").Build()
			machine2 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder.WithFailureDomainName(fds[2%len(fds)].Name)).WithName("master-2").Build()
		} else {
			machine0 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder).WithName("master-0").Build()
			machine1 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder).WithName("master-1").Build()
			machine2 = machineBuilder.WithProviderSpecBuilder(providerSpecBuilder).WithName("master-2").Build()
		}

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	BeforeEach(func() {
		Expect(k8sClient).NotTo(BeNil())
		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()
	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	JustBeforeEach(func() {
		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, managerOptions)
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		featureGateAccessor, err := util.SetupFeatureGateAccessor(ctx, mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:              mgr.GetClient(),
			Namespace:           namespaceName,
			FeatureGateAccessor: featureGateAccessor,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

		By("Starting the manager")
		mgrCancel, mgrDone = startManager(&mgr)
	})

	JustAfterEach(func() {
		By("Stopping the manager")
		stopManager()
	})

	Context("when nutanix failure domains are not defined", func() {
		BeforeEach(func() {
			By("Setting up infrastructure resource without FailureDomains for the test")
			infra = createInfrastructure(false)
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
					By("Creating Control Plane Machines")
					create3CPMachines(infra, false)
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
					cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
					Expect(err).To(BeNil())

					machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
					Expect(err).To(BeNil())

					// Remove from the machine Provider Spec the fields that won't be
					// present on the ControlPlaneMachineSet Provider Spec.
					machineProviderConfig := machineProviderSpec.Generic()

					Expect(cpmsProviderSpec.Generic()).To(Equal(machineProviderConfig))
				})
			})

			Context("with only 1 existing control plane machine", func() {
				var logger testutils.TestLogger
				isSupportedControlPlaneMachinesNumber := false

				BeforeEach(func() {
					By("Creating 1 Control Plane Machine")
					machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
					machine2 = machineBuilder.WithName("master-2").Build()
					Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
					machines := []machinev1beta1.Machine{*machine2}

					By("Invoking the check on whether the number of control plane machines in the cluster is supported")
					logger = testutils.NewTestLogger()
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
						testutils.LogEntry{
							Level:         1,
							KeysAndValues: []interface{}{"count", 1},
							Message:       unsupportedNumberOfControlPlaneMachines,
						},
					))
				})
			})

			Context("with an unsupported platform", func() {
				var logger testutils.TestLogger
				BeforeEach(func() {
					By("Creating Control Plane Machines")
					machines := create3CPMachines(infra, false)

					Eventually(komega.UpdateStatus(infra, func() {
						infra.Status.PlatformStatus.Type = configv1.NonePlatformType
					})).Should(Succeed())

					logger = testutils.NewTestLogger()
					generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), infra, *machines)
					Expect(generatedCPMS).To(BeNil())
					Expect(err).To(MatchError(errUnsupportedPlatform))
				})

				It("should have not created the ControlPlaneMachineSet", func() {
					Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
				})

				It("sets an appropriate log line", func() {
					Eventually(logger.Entries()).Should(ConsistOf(
						testutils.LogEntry{
							Level:         1,
							KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
							Message:       unsupportedPlatform,
						},
					))
				})

			})
		})
	})

	Context("when nutanix failure domains are defined", func() {
		BeforeEach(func() {
			By("Setting up infrastructure resource with FailureDomains for the test")
			infra = createInfrastructure(true)
		})

		Context("when a ControlPlaneMachineSet resource doesn't exist", func() {
			BeforeEach(func() {
				cpms = &machinev1.ControlPlaneMachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterControlPlaneMachineSetName,
						Namespace: namespaceName,
					},
				}
			})

			Context("with 3 existing control plane machines created with failureDomain reference", func() {
				BeforeEach(func() {
					By("Creating Control Plane Machines with failureDomain references")
					create3CPMachines(infra, true)

				})

				It("should create the ControlPlaneMachineSet with the expected fields", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())
					Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateInactive))
					Expect(*cpms.Spec.Replicas).To(Equal(int32(3)))

					cpmsTemplateMachine := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine
					Expect(cpmsTemplateMachine.FailureDomains.Nutanix).To(HaveLen(3), "Should have 3 failureDomain.")

					nmpc := &machinev1.NutanixMachineProviderConfig{}
					Expect(json.Unmarshal(cpmsTemplateMachine.Spec.ProviderSpec.Value.Raw, nmpc)).To(Succeed())
					Expect(nmpc.Cluster.Type).To(BeEmpty(), "The cluster should not be set when failure domains are used.")
					Expect(nmpc.Subnets).To(HaveLen(0), "The subnets should not be set when failure domains are used.")
					Expect(nmpc.FailureDomain).To(BeNil())
				})
			})

			Context("with 3 existing control plane machines created without failureDomain reference", func() {
				BeforeEach(func() {
					By("Creating Control Plane Machines without failureDomain reference")
					create3CPMachines(infra, false)

				})

				It("should create the ControlPlaneMachineSet with the expected fields", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())
					Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateInactive))
					Expect(*cpms.Spec.Replicas).To(Equal(int32(3)))

					cpmsTemplateMachine := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine
					Expect(cpmsTemplateMachine.FailureDomains.Nutanix).To(HaveLen(0))

					// In this case expect the machine Provider Spec of the youngest machine to be used here.
					// In this case it should be `machine-2` given that's the one we created last.
					cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
					Expect(err).To(BeNil())

					machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
					Expect(err).To(BeNil())
					machineProviderConfig := machineProviderSpec.Generic()

					Expect(cpmsProviderSpec.Generic()).To(Equal(machineProviderConfig))
				})
			})
		})
	})
})

var _ = Describe("controlplanemachinesetgenerator controller on OpenStack", func() {

	var (
		az1FailureDomainBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az1").WithRootVolume(&machinev1.RootVolume{
			AvailabilityZone: "cinder-az1",
			VolumeType:       "fast-az1",
		})

		az2FailureDomainBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az2").WithRootVolume(&machinev1.RootVolume{
			AvailabilityZone: "cinder-az2",
			VolumeType:       "fast-az2",
		})

		az3FailureDomainBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az3").WithRootVolume(&machinev1.RootVolume{
			AvailabilityZone: "cinder-az3",
			VolumeType:       "fast-az3",
		})

		az4FailureDomainBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az4")

		az5FailureDomainBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&machinev1.RootVolume{
			AvailabilityZone: "cinder-az5",
			VolumeType:       "fast-az5",
		})

		defaultProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec()

		az1ProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az1").WithRootVolume(&machinev1alpha1.RootVolume{
			VolumeType: "fast-az1",
			Zone:       "cinder-az1",
		})

		az2ProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az2").WithRootVolume(&machinev1alpha1.RootVolume{
			VolumeType: "fast-az2",
			Zone:       "cinder-az2",
		})

		az3ProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az3").WithRootVolume(&machinev1alpha1.RootVolume{
			VolumeType: "fast-az3",
			Zone:       "cinder-az3",
		})

		az4ProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec().WithZone("nova-az4")

		az5ProviderSpecBuilderOpenStack = machinev1beta1resourcebuilder.OpenStackProviderSpec().WithRootVolume(&machinev1alpha1.RootVolume{
			VolumeType: "fast-az5",
			Zone:       "cinder-az5",
		})

		cpms3FailureDomainsBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
			az1FailureDomainBuilderOpenStack,
			az2FailureDomainBuilderOpenStack,
			az3FailureDomainBuilderOpenStack,
		)

		cpms5FailureDomainsBuilderOpenStack = machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
			az1FailureDomainBuilderOpenStack,
			az2FailureDomainBuilderOpenStack,
			az3FailureDomainBuilderOpenStack,
			az4FailureDomainBuilderOpenStack,
			az5FailureDomainBuilderOpenStack,
		)

		cpmsInactive3FDsBuilderOpenStack = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateInactive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						az1ProviderSpecBuilderOpenStack.WithZone("").WithRootVolume(&machinev1alpha1.RootVolume{}).WithFlavor("m1.xlarge"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						az1FailureDomainBuilderOpenStack,
						az2FailureDomainBuilderOpenStack,
						az3FailureDomainBuilderOpenStack,
					)),
			)

		cpmsInactiveOutdated3FDsBuilderOpenStack = machinev1resourcebuilder.ControlPlaneMachineSet().
								WithState(machinev1.ControlPlaneMachineSetStateInactive).
								WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						az1ProviderSpecBuilderOpenStack.WithFlavor("outdatedFlavour"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						az1FailureDomainBuilderOpenStack,
						az2FailureDomainBuilderOpenStack,
						az3FailureDomainBuilderOpenStack,
					)),
			)

		cpmsActiveOutdatedBuilderOpenStack = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateActive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						az1ProviderSpecBuilderOpenStack.WithFlavor("m1.xlarge"),
					).
					WithFailureDomainsBuilder(machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						az1FailureDomainBuilderOpenStack,
						az2FailureDomainBuilderOpenStack,
						az3FailureDomainBuilderOpenStack,
					)),
			)

		cpmsActiveUpToDateBuilderOpenStack = machinev1resourcebuilder.ControlPlaneMachineSet().
							WithState(machinev1.ControlPlaneMachineSetStateActive).
							WithMachineTemplateBuilder(
				machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(
						az1ProviderSpecBuilderOpenStack.WithFlavor("m1.large"),
					).
					WithFailureDomainsBuilder(cpms5FailureDomainsBuilderOpenStack),
			)
	)

	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}
	var mgr manager.Manager
	var reconciler *ControlPlaneMachineSetGeneratorReconciler

	var namespaceName string
	var infra *configv1.Infrastructure
	var cpms *machinev1.ControlPlaneMachineSet
	var machine0, machine1, machine2 *machinev1beta1.Machine

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

	create3DefaultCPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with the same Provider Spec (no failure domain),
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(defaultProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(defaultProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(defaultProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	create3CPMachinesWithDifferentServerGroups := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// with three different server groups, so then we can reliably check that ControlPlaneMachineSet Spec won't be generated.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(az1ProviderSpecBuilderOpenStack.WithFlavor("m1.large").WithServerGroupName("master-latest")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(az1ProviderSpecBuilderOpenStack.WithFlavor("m1.large").WithServerGroupName("master-old")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(az1ProviderSpecBuilderOpenStack.WithFlavor("m1.large").WithServerGroupName("master-old")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	create3CPMachines := func() *[]machinev1beta1.Machine {
		// Create 3 control plane machines with differing Provider Specs,
		// so then we can reliably check which machine Provider Spec is picked for the ControlPlaneMachineSet.
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine0 = machineBuilder.WithProviderSpecBuilder(az1ProviderSpecBuilderOpenStack.WithFlavor("m1.medium")).WithName("master-0").Build()
		machine1 = machineBuilder.WithProviderSpecBuilder(az2ProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-1").Build()
		machine2 = machineBuilder.WithProviderSpecBuilder(az3ProviderSpecBuilderOpenStack.WithFlavor("m1.xlarge")).WithName("master-2").Build()

		// Create Machines with some wait time between them
		// to achieve staggered CreationTimestamp(s).
		Expect(k8sClient.Create(ctx, machine0)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine1)).To(Succeed())
		time.Sleep(1 * time.Second)
		Expect(k8sClient.Create(ctx, machine2)).To(Succeed())

		// Check that the creation timestamps are staggered
		Expect(machine0.CreationTimestamp.Time).To(BeTemporally("<", machine1.CreationTimestamp.Time))
		Expect(machine1.CreationTimestamp.Time).To(BeTemporally("<", machine2.CreationTimestamp.Time))

		return &[]machinev1beta1.Machine{*machine0, *machine1, *machine2}
	}

	createAZ4Machine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(az4ProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-3").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	createAZ5Machine := func() *machinev1beta1.Machine {
		machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
		machine := machineBuilder.WithProviderSpecBuilder(az5ProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-4").Build()

		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		return machine
	}

	BeforeEach(func() {

		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a new infrastructure for the test")
		// Create infrastructure object.
		infra = configv1resourcebuilder.Infrastructure().WithName(infrastructureName).AsOpenStack("test").Build()
		infraStatus := infra.Status.DeepCopy()
		Expect(k8sClient.Create(ctx, infra)).To(Succeed())
		// Update Infrastructure Status.
		Eventually(komega.UpdateStatus(infra, func() {
			infra.Status = *infraStatus
		})).Should(Succeed())

		By("Setting up a manager and controller")
		var err error
		mgr, err = ctrl.NewManager(cfg, managerOptions)
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		featureGateAccessor, err := util.SetupFeatureGateAccessor(ctx, mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler = &ControlPlaneMachineSetGeneratorReconciler{
			Client:              mgr.GetClient(),
			Namespace:           namespaceName,
			FeatureGateAccessor: featureGateAccessor,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
			&configv1.Infrastructure{},
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

		Context("with 3 different server group names", func() {
			BeforeEach(func() {
				By("Creating Machines")
				create3CPMachinesWithDifferentServerGroups()
			})

			It("should not create the ControlPlaneMachineSet", func() {
				By("Checking the Control Plane Machine Set has not been created")
				Eventually(komega.Get(cpms)).ShouldNot(Succeed())
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})
		})

		Context("with 1 existing control plane machines", func() {
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				create3DefaultCPMachines()
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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				openStackMachineProviderConfig := machineProviderSpec.OpenStack().Config()
				Expect(cpmsProviderSpec.OpenStack().Config()).To(Equal(openStackMachineProviderConfig))
			})

			It("should create the ControlPlaneMachineSet with no failure domain", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())

				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(BeNil())
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional Machines")
					createAZ4Machine()
					createAZ5Machine()
				})

				It("should have not created the ControlPlaneMachineSet with a mix of empty and non empty failure domains", func() {
					Eventually(komega.Get(cpms)).ShouldNot(Succeed())
					Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
				})
			})
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
				cpmsProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec, nil)
				Expect(err).To(BeNil())

				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				openStackMachineProviderConfig := machineProviderSpec.OpenStack().Config()
				if openStackMachineProviderConfig.AvailabilityZone != "" {
					openStackMachineProviderConfig.AvailabilityZone = ""
				}
				if openStackMachineProviderConfig.RootVolume != nil {
					if openStackMachineProviderConfig.RootVolume.VolumeType != "" {
						openStackMachineProviderConfig.RootVolume.VolumeType = ""
					}
					if openStackMachineProviderConfig.RootVolume.Zone != "" {
						openStackMachineProviderConfig.RootVolume.Zone = ""
					}
				}

				Expect(cpmsProviderSpec.OpenStack().Config()).To(Equal(openStackMachineProviderConfig))
			})

			It("should create the ControlPlaneMachineSet with only one copy of each of the 3 failure domains", func() {
				By("Checking the Control Plane Machine Set has been created")
				Eventually(komega.Get(cpms)).Should(Succeed())

				Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms3FailureDomainsBuilderOpenStack.BuildFailureDomains())))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional Machines")
					createAZ4Machine()
					createAZ5Machine()
				})

				It("should create the ControlPlaneMachineSet with only one copy of each the 5 failure domains", func() {
					By("Checking the Control Plane Machine Set has been created")
					Eventually(komega.Get(cpms)).Should(Succeed())

					Expect(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains).To(HaveValue(Equal(cpms5FailureDomainsBuilderOpenStack.BuildFailureDomains())))
				})
			})
		})

		Context("with only 1 existing control plane machine", func() {
			var logger testutils.TestLogger
			isSupportedControlPlaneMachinesNumber := false

			BeforeEach(func() {
				By("Creating 1 Control Plane Machine")
				machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithNamespace(namespaceName)
				machine2 = machineBuilder.WithProviderSpecBuilder(az3ProviderSpecBuilderOpenStack.WithFlavor("m1.large")).WithName("master-2").Build()
				Expect(k8sClient.Create(ctx, machine2)).To(Succeed())
				machines := []machinev1beta1.Machine{*machine2}

				By("Invoking the check on whether the number of control plane machines in the cluster is supported")
				logger = testutils.NewTestLogger()
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
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"count", 1},
						Message:       unsupportedNumberOfControlPlaneMachines,
					},
				))
			})

		})

		Context("with an unsupported platform", func() {
			var logger testutils.TestLogger
			BeforeEach(func() {
				By("Creating Control Plane Machines")
				machines := create3CPMachines()

				Eventually(komega.UpdateStatus(infra, func() {
					infra.Status.PlatformStatus.Type = configv1.NonePlatformType
				})).Should(Succeed())

				logger = testutils.NewTestLogger()
				generatedCPMS, err := reconciler.generateControlPlaneMachineSet(logger.Logger(), infra, *machines)
				Expect(generatedCPMS).To(BeNil())
				Expect(err).To(MatchError(errUnsupportedPlatform))
			})

			It("should have not created the ControlPlaneMachineSet", func() {
				Consistently(komega.Get(cpms)).Should(MatchError("controlplanemachinesets.machine.openshift.io \"" + clusterControlPlaneMachineSetName + "\" not found"))
			})

			It("sets an appropriate log line", func() {
				Eventually(logger.Entries()).Should(ConsistOf(
					testutils.LogEntry{
						Level:         1,
						KeysAndValues: []interface{}{"platform", configv1.NonePlatformType},
						Message:       unsupportedPlatform,
					},
				))
			})

		})
	})

	Context("when a Control Plane Machine Set exists with 3 Machines", func() {
		BeforeEach(func() {
			By("Creating Control Plane Machines")
			create3CPMachines()
		})

		Context("with state Inactive and outdated", func() {
			BeforeEach(func() {
				By("Creating an outdated and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// doesn't match the one of the youngest control plane machine (i.e. it's outdated).
				cpms = cpmsInactiveOutdated3FDsBuilderOpenStack.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should recreate ControlPlaneMachineSet with the provider spec matching the youngest machine provider spec", func() {
				// In this case expect the machine Provider Spec of the youngest machine to be used here.
				// In this case it should be `machine-1` given that's the one we created last.
				machineProviderSpec, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), machine2.Spec, nil)
				Expect(err).To(BeNil())

				// Remove from the machine Provider Spec the fields that won't be
				// present on the ControlPlaneMachineSet Provider Spec.
				openStackMachineProviderConfig := machineProviderSpec.OpenStack().Config()
				if openStackMachineProviderConfig.AvailabilityZone != "" {
					openStackMachineProviderConfig.AvailabilityZone = ""
				}
				if openStackMachineProviderConfig.RootVolume != nil {
					if openStackMachineProviderConfig.RootVolume.VolumeType != "" {
						openStackMachineProviderConfig.RootVolume.VolumeType = ""
					}
					if openStackMachineProviderConfig.RootVolume.Zone != "" {
						openStackMachineProviderConfig.RootVolume.Zone = ""
					}
				}

				oldUID := cpms.UID

				Eventually(komega.Object(cpms), time.Second*30).Should(
					HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.Spec",
						WithTransform(func(in machinev1beta1.MachineSpec) machinev1alpha1.OpenstackProviderSpec {
							mPS, err := providerconfig.NewProviderConfigFromMachineSpec(mgr.GetLogger(), in, nil)
							if err != nil {
								return machinev1alpha1.OpenstackProviderSpec{}
							}

							return mPS.OpenStack().Config()
						}, Equal(openStackMachineProviderConfig))),
					"The control plane machine provider spec should match the youngest machine's provider spec",
				)

				Expect(oldUID).NotTo(Equal(cpms.UID),
					"The control plane machine set UID should differ with the old one, as it should've been deleted and recreated")
			})

			It("should update ControlPlaneMachineSet with the expected failure domains", func() {
				Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms3FailureDomainsBuilderOpenStack.BuildFailureDomains()))))
			})

			Context("With additional Machines adding additional failure domains", func() {
				BeforeEach(func() {
					By("Creating additional MachineSets")
					createAZ4Machine()
					createAZ5Machine()
				})

				It("should include additional failure domains from the created Machines", func() {
					Eventually(komega.Object(cpms)).Should(HaveField("Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains", HaveValue(Equal(cpms5FailureDomainsBuilderOpenStack.BuildFailureDomains()))))
				})
			})

		})

		Context("with state Inactive and up to date", func() {
			BeforeEach(func() {
				By("Creating an up to date and Inactive Control Plane Machine Set")
				// Create an Inactive ControlPlaneMachineSet with a Provider Spec that
				// match the youngest control plane machine (i.e. it's up to date).
				cpms = cpmsInactive3FDsBuilderOpenStack.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveOutdatedBuilderOpenStack.WithNamespace(namespaceName).Build()
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
				cpms = cpmsActiveUpToDateBuilderOpenStack.WithNamespace(namespaceName).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("should keep the ControlPlaneMachineSet unchanged", func() {
				cpmsVersion := cpms.ObjectMeta.ResourceVersion
				Consistently(komega.Object(cpms)).Should(HaveField("ObjectMeta.ResourceVersion", cpmsVersion))
			})

		})
	})
})
