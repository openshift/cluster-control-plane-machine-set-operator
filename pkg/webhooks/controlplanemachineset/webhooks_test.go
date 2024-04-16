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

package controlplanemachineset

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1builder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	"github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// stringPtr returns a pointer to the string value.
func stringPtr(s string) *string {
	return &s
}

var _ = Describe("Webhooks", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}

	var namespaceName string

	const (
		dummyValue = "value"
	)

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-webhook-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		namespaceName = ns.GetName()

		By("Setting up a manager and webhook")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: testScheme,
			Metrics: server.Options{
				BindAddress: "0",
			},
			WebhookServer: webhook.NewServer(webhook.Options{
				Port:    testEnv.WebhookInstallOptions.LocalServingPort,
				Host:    testEnv.WebhookInstallOptions.LocalServingHost,
				CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
			}),
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		wh := &ControlPlaneMachineSetWebhook{}
		Expect(wh.SetupWebhookWithManager(mgr, mgr.GetLogger())).To(Succeed(), "Webhook should be able to register with manager")

		By("Starting the manager")
		var mgrCtx context.Context
		mgrCtx, mgrCancel = context.WithCancel(context.Background())
		mgrDone = make(chan struct{})

		go func() {
			defer GinkgoRecover()
			defer close(mgrDone)

			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
	})

	AfterEach(func() {
		By("Stopping the manager")
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone

		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&machinev1beta1.Machine{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("on vSphere", Ordered, func() {

		Context("when validating without failure domains", func() {
			var infrastructure *configv1.Infrastructure

			BeforeAll(func() {
				infrastructure = configv1builder.Infrastructure().AsVSphere("vsphere-test").WithName("cluster").Build()
				Expect(infrastructure.Spec.PlatformSpec.VSphere.FailureDomains).To(BeNil(), "Failure domains should be nil")
				Expect(k8sClient.Create(ctx, infrastructure)).To(Succeed())
			})

			AfterAll(func() {
				Expect(k8sClient.Delete(ctx, infrastructure)).To(Succeed())
			})

			It("when providing template with no path in vSphere configuration", func() {
				providerSpec := machinev1beta1resourcebuilder.VSphereProviderSpec().WithInfrastructure(*infrastructure).WithTemplate("no-path-template")
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)

				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed(), "expected to be able to create the control plane machine set")
				}

				cpmsBuilder := machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)
				cpms := cpmsBuilder.Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed(), "CPMS must succeed during creation")
			})

			It("when providing template with valid path in vSphere configuration", func() {
				providerSpec := machinev1beta1resourcebuilder.VSphereProviderSpec().WithTemplate("/datacenter/vm/user-upi-l8x7x-rhcos-us-east-us-east-3a")
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)

				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed(), "expected to be able to create the control plane machine set")
				}

				cpmsBuilder := machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)
				cpms := cpmsBuilder.Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed(), "CPMS must succeed during creation")
			})

			It("when providing template with invalid path in vSphere configuration", func() {
				providerSpec := machinev1beta1resourcebuilder.VSphereProviderSpec().WithTemplate("/datacenter/what-is-this/user-upi-l8x7x-rhcos-us-east-us-east-3a")
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)

				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed(), "expected to be able to create the control plane machine set")
				}

				cpmsBuilder := machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)
				cpms := cpmsBuilder.Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
					ContainSubstring("admission webhook \"controlplanemachineset.machine.openshift.io\" denied the request: spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.template: Invalid value: \"/datacenter/what-is-this/user-upi-l8x7x-rhcos-us-east-us-east-3a\": template must be provided as the full path"),
				)))
			})
		})
	})

	Context("on create", func() {
		var builder machinev1resourcebuilder.ControlPlaneMachineSetBuilder
		var machineTemplate machinev1resourcebuilder.OpenShiftMachineV1Beta1TemplateBuilder
		var infrastructure *configv1.Infrastructure

		BeforeEach(func() {
			infrastructure = configv1builder.Infrastructure().AsAWS("cluster", "us-east-1").WithName("cluster").Build()
			Expect(k8sClient.Create(ctx, infrastructure)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, infrastructure)).To(Succeed())
		})

		Context("when validating without failure domains", func() {
			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1")
				machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid, individual tests will override to make it invalid
				builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)
				By("Creating a selection of Machines")
				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}
			})

			It("with a valid spec", func() {
				cpms := builder.Build()
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with a disallowed name", func() {
				cpms := builder.WithName("disallowed").Build()
				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("metadata.name: Invalid value: \"disallowed\": control plane machine set name must be cluster")))
			})

			It("with 4 replicas", func() {
				// This is an openapi validation but it makes sense to include it here as well
				cpms := builder.WithReplicas(4).Build()
				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("Unsupported value: 4: supported values: \"3\", \"5\"")))
			})

			It("with mismatched selector and machine labels", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: "different-id",
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: map[string]string{\"machine.openshift.io/cluster-api-cluster\":\"different-id\", \"machine.openshift.io/cluster-api-machine-role\":\"master\", \"machine.openshift.io/cluster-api-machine-type\":\"master\"}: selector does not match template labels")))
			})

			It("with no cluster ID label is set", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineRoleLabel: masterMachineRole,
						openshiftMachineTypeLabel: masterMachineRole,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineRoleLabel: masterMachineRole,
						openshiftMachineTypeLabel: masterMachineRole,
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-cluster' is required"))
			})

			It("with no master role label on the template", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-machine-role' is required, and must have value 'master'")))
			})

			It("with an incorrect role label on the template", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineRoleLabel:            "worker",
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineRoleLabel:            "worker",
						openshiftMachineTypeLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-machine-role' is required, and must have value 'master'")))
			})

			It("with no master type label on the template", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-machine-type' is required, and must have value 'master'")))
			})

			It("with an incorrect type label on the template", func() {
				cpms := builder.WithSelector(metav1.LabelSelector{
					MatchLabels: map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            "worker",
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					},
				}).WithMachineTemplateBuilder(
					machineTemplate.WithLabels(map[string]string{
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            "worker",
						machinev1beta1.MachineClusterIDLabel: resourcebuilder.TestClusterIDValue,
					}),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-machine-type' is required, and must have value 'master'")))
			})

			It("with no machine template", func() {
				cpms := builder.WithMachineTemplateBuilder(nil).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("spec.template.machineType: Required value")))
			})

			It("with no machine template value", func() {
				cpms := builder.Build()
				// Leave the union discriminator but set no values.
				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine = nil

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template: Invalid value: \"object\": machines_v1beta1_machine_openshift_io configuration is required when machineType is machines_v1beta1_machine_openshift_io, and forbidden otherwise")))
			})

			It("with machine template zone not matching machines", func() {
				tempateProviderSpec := machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("different-zone-1")
				templateBuilder := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(tempateProviderSpec)
				cpms := builder.WithMachineTemplateBuilder(templateBuilder).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec: Invalid value: AWSFailureDomain{AvailabilityZone:different-zone-1, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}: Failure domain extracted from machine template providerSpec does not match failure domain of all control plane machines")))
			})

			It("with invalid failure domain information", func() {
				cpms := builder.Build()

				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1.FailureDomains{
					Platform: configv1.AWSPlatformType,
					Azure: &[]machinev1.AzureFailureDomain{
						{
							Zone: "us-central-1",
						},
					},
				}

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Invalid value: \"object\": aws configuration is required when platform is AWS, and forbidden otherwise"),
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Invalid value: \"object\": azure configuration is required when platform is Azure, and forbidden otherwise"),
				)))
			})

			It("when adding invalid subnets in the faliure domains", func() {
				cpms := builder.Build()

				cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1.FailureDomains{
					Platform: configv1.AWSPlatformType,
					AWS: &[]machinev1.AWSFailureDomain{
						{
							Subnet: &machinev1.AWSResourceReference{
								Type: machinev1.AWSARNReferenceType,
								ID:   ptr.To[string]("id-123"),
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[0].subnet: Invalid value: \"object\": id is required when type is ID, and forbidden otherwise"),
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[0].subnet: Invalid value: \"object\": arn is required when type is ARN, and forbidden otherwise"),
				)))
			})
		})

		Context("when validating failure domains on AWS", func() {
			var builder machinev1resourcebuilder.ControlPlaneMachineSetBuilder
			var filterSubnet = machinev1.AWSResourceReference{
				Type: machinev1.AWSFiltersReferenceType,
				Filters: &[]machinev1.AWSResourceFilter{{
					Name:   "tag:Name",
					Values: []string{"aws-subnet-12345678"},
				}},
			}

			var filterSubnetDifferent = machinev1.AWSResourceReference{
				Type: machinev1.AWSFiltersReferenceType,
				Filters: &[]machinev1.AWSResourceFilter{{
					Name:   "tag:Name",
					Values: []string{"aws-subnet-different"},
				}},
			}

			var idSubnet = machinev1.AWSResourceReference{
				Type: machinev1.AWSIDReferenceType,
				ID:   stringPtr("subnet-us-east-1c"),
			}

			var usEast1aBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(filterSubnet)
			var usEast1bBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(filterSubnet)
			var usEast1cBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(filterSubnet)
			var usEast1cBuilderWithSubnet = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(filterSubnetDifferent)
			var usEast1cBuilderWithIDSubnet = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(idSubnet)
			var usEast1dBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1d").WithSubnet(filterSubnet)
			var usEast1eBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1e").WithSubnet(filterSubnet)
			var usEast1fBuilder = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1f").WithSubnet(filterSubnet)

			BeforeEach(func() {
				By("Setting up a namespace for the test")
				ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-webhook-").Build()
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				namespaceName = ns.GetName()
			})

			Context("with machines spread evenly across failure domains", func() {
				BeforeEach(func() {
					providerSpec := machinev1beta1resourcebuilder.AWSProviderSpec()
					machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
					machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
					controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster()
					workerMachineBuilder := machineBuilder.WithGenerateName("worker-machine-").AsWorker()
					machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)

					builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

					var filterSubnet = machinev1beta1.AWSResourceReference{
						Filters: []machinev1beta1.Filter{{
							Name:   "tag:Name",
							Values: []string{"aws-subnet-12345678"},
						}},
					}

					By("Creating a selection of Machines")
					for _, az := range []string{"us-east-1a", "us-east-1b", "us-east-1c"} {
						ps := providerSpec.WithAvailabilityZone(az).WithSubnet(filterSubnet)
						worker := workerMachineBuilder.WithProviderSpecBuilder(ps).Build()
						controlPlane := controlPlaneMachineBuilder.WithProviderSpecBuilder(ps).Build()

						Expect(k8sClient.Create(ctx, worker)).To(Succeed())
						Expect(k8sClient.Create(ctx, controlPlane)).To(Succeed())
					}
					for _, az := range []string{"us-east-1d", "us-east-1e", "us-east-1f"} {
						ps := providerSpec.WithAvailabilityZone(az)
						worker := workerMachineBuilder.WithProviderSpecBuilder(ps).Build()

						Expect(k8sClient.Create(ctx, worker)).To(Succeed())
					}
				})

				It("with a valid failure domains spec", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
							usEast1cBuilder,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
				})

				It("with a invalid subnet filter - different value", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
							usEast1cBuilderWithSubnet,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
						ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
					)))
				})

				It("with a invalid subnet type - different type", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
							usEast1cBuilderWithIDSubnet,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
						ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}]"),
					)))
				})

				It("when reducing the availability", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
						ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s)"),
						ContainSubstring("AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
						ContainSubstring("AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
					)))
				})

				It("when increasing the availability", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
							usEast1cBuilder,
							usEast1dBuilder,
						),
					)).Build()

					// We allow additional failure domains to be present to allow expansion horizontally if required later.
					// The load balancing algorithm for the failure domain mapping should ensure the failure domains are stable
					// so this shouldn't cause any issues with install.
					Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
				})

				It("when the availability zones don't match", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1dBuilder,
							usEast1eBuilder,
							usEast1fBuilder,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
						ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s)"),
						ContainSubstring("AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
						ContainSubstring("AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
						ContainSubstring("AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}"),
					)))
				})
			})

			Context("with machines spread unevenly across failure domains", func() {
				BeforeEach(func() {
					providerSpec := machinev1beta1resourcebuilder.AWSProviderSpec()
					machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
					machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
					controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster()
					workerMachineBuilder := machineBuilder.WithGenerateName("worker-machine-").AsWorker()
					machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)

					builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

					var filterSubnet = machinev1beta1.AWSResourceReference{
						Filters: []machinev1beta1.Filter{{
							Name:   "tag:Name",
							Values: []string{"aws-subnet-12345678"},
						}},
					}

					By("Creating a selection of Machines")
					for _, az := range []string{"us-east-1a", "us-east-1b", "us-east-1b"} {
						ps := providerSpec.WithAvailabilityZone(az).WithSubnet(filterSubnet)
						worker := workerMachineBuilder.WithProviderSpecBuilder(ps).Build()
						controlPlane := controlPlaneMachineBuilder.WithProviderSpecBuilder(ps).Build()

						Expect(k8sClient.Create(ctx, worker)).To(Succeed())
						Expect(k8sClient.Create(ctx, controlPlane)).To(Succeed())
					}
					for _, az := range []string{"us-east-1c", "us-east-1d", "us-east-1e", "us-east-1f"} {
						ps := providerSpec.WithAvailabilityZone(az)
						worker := workerMachineBuilder.WithProviderSpecBuilder(ps).Build()

						Expect(k8sClient.Create(ctx, worker)).To(Succeed())
					}
				})

				It("with matching failure domains", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
				})

				It("with an additional failure domain in the spec", func() {
					cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
						machinev1resourcebuilder.AWSFailureDomains().WithFailureDomainBuilders(
							usEast1aBuilder,
							usEast1bBuilder,
							usEast1cBuilder,
						),
					)).Build()

					Expect(k8sClient.Create(ctx, cpms)).To(MatchError(SatisfyAll(
						ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: no control plane machine is using specified failure domain(s) [AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}], failure domain(s) [AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[aws-subnet-12345678]}]}}] are duplicated within the control plane machines, please correct failure domains to match control plane machines"),
					)))
				})
			})
		})

		Context("on Azure", func() {
			zone1Builder := machinev1resourcebuilder.AzureFailureDomain().WithZone("1")
			zone2Builder := machinev1resourcebuilder.AzureFailureDomain().WithZone("2")
			zone3Builder := machinev1resourcebuilder.AzureFailureDomain().WithZone("3")
			zone4Builder := machinev1resourcebuilder.AzureFailureDomain().WithZone("4")

			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.AzureProviderSpec()
				machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid, individual tests will override to make it invalid
				builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)

				By("Creating a selection of Machines")
				for i := 1; i <= 3; i++ {
					controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec.WithZone(fmt.Sprintf("%d", i)))

					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}
			})

			It("with a valid failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with a mismatched failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone4Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [AzureFailureDomain{Zone:3, Subnet:cluster-subnet-12345678}]"),
				))
			})

			It("when reducing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [AzureFailureDomain{Zone:3, Subnet:cluster-subnet-12345678}]"),
				))
			})

			It("when increasing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
						zone4Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with an internal load balancer", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
					),
				).WithProviderSpecBuilder(
					machinev1beta1resourcebuilder.AzureProviderSpec().WithInternalLoadBalancer("internal-load-balancer-12345678"),
				),
				).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("without an internal load balancer", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.AzureFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
					),
				).WithProviderSpecBuilder(
					machinev1beta1resourcebuilder.AzureProviderSpec().WithInternalLoadBalancer(""), // Set to the empty string to remove it.
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.internalLoadBalancer: Required value: internalLoadBalancer is required for control plane machines"),
				))
			})
		})

		Context("on GCP", func() {
			var usCentral1aBuilder = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central-1a")
			var usCentral1bBuilder = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central-1b")
			var usCentral1cBuilder = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central-1c")
			var usCentral1dBuilder = machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central-1d")

			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.GCPProviderSpec()
				machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid, individual tests will override to make it invalid
				builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)

				By("Creating a selection of Machines")
				for _, az := range []string{"us-central-1a", "us-central-1b", "us-central-1c"} {
					controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec.WithZone(az))

					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}
			})

			It("with a valid failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usCentral1aBuilder,
						usCentral1bBuilder,
						usCentral1cBuilder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with a mismatched failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usCentral1aBuilder,
						usCentral1bBuilder,
						usCentral1dBuilder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [GCPFailureDomain{Zone:us-central-1c}]"),
				))
			})

			It("when reducing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usCentral1aBuilder,
						usCentral1bBuilder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [GCPFailureDomain{Zone:us-central-1c}]"),
				))
			})

			It("when increasing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.GCPFailureDomains().WithFailureDomainBuilders(
						usCentral1aBuilder,
						usCentral1bBuilder,
						usCentral1cBuilder,
						usCentral1dBuilder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})
		})

		Context("on OpenStack", func() {

			var filterRootVolumeOne = machinev1.RootVolume{
				AvailabilityZone: "cinder-az1",
				VolumeType:       "fast-az1",
			}
			var filterRootVolumeTwo = machinev1.RootVolume{
				AvailabilityZone: "cinder-az2",
				VolumeType:       "fast-az2",
			}
			var filterRootVolumeThree = machinev1.RootVolume{
				AvailabilityZone: "cinder-az3",
				VolumeType:       "fast-az3",
			}
			var filterRootVolumeFour = machinev1.RootVolume{
				AvailabilityZone: "cinder-az4",
				VolumeType:       "fast-az4",
			}
			var zone1Builder = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az1").WithRootVolume(&filterRootVolumeOne)
			var zone2Builder = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az2").WithRootVolume(&filterRootVolumeTwo)
			var zone3Builder = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az3").WithRootVolume(&filterRootVolumeThree)
			var zone4Builder = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az4").WithRootVolume(&filterRootVolumeFour)

			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.OpenStackProviderSpec()
				machineTemplate = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid, individual tests will override to make it invalid
				builder = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate)

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)

				By("Creating a selection of Machines")
				for _, az := range []string{"az1", "az2", "az3"} {
					rootVolume := &machinev1alpha1.RootVolume{
						VolumeType: "fast-" + az,
						Zone:       "cinder-" + az,
					}
					controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec.WithZone("nova-" + az).WithRootVolume(rootVolume))

					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}
			})

			It("with a valid failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with a mismatched failure domains spec", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone4Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [OpenStackFailureDomain{AvailabilityZone:nova-az3, RootVolume:{AvailabilityZone:cinder-az3, VolumeType:fast-az3}}]"),
				))
			})

			It("when reducing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Forbidden: control plane machines are using unspecified failure domain(s) [OpenStackFailureDomain{AvailabilityZone:nova-az3, RootVolume:{AvailabilityZone:cinder-az3, VolumeType:fast-az3}}]"),
				))
			})

			It("when increasing the availability", func() {
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
						zone4Builder,
					),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with wrong additional block device for etcd", func() {
				additionalBlockDevicesWithWrongEtcd := []machinev1alpha1.AdditionalBlockDevice{
					{
						Name:    "etcd",
						SizeGiB: 9,
						Storage: machinev1alpha1.BlockDeviceStorage{
							Type: "Local",
						},
					},
				}
				cpms := builder.WithMachineTemplateBuilder(machineTemplate.WithFailureDomainsBuilder(
					machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
						zone1Builder,
						zone2Builder,
						zone3Builder,
					),
				).WithProviderSpecBuilder(
					machinev1beta1resourcebuilder.OpenStackProviderSpec().WithAdditionalBlockDevices(additionalBlockDevicesWithWrongEtcd),
				)).Build()

				Expect(k8sClient.Create(ctx, cpms)).To(MatchError(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.additionalBlockDevices: Invalid value: 9: etcd block device size must be at least 10 GiB"),
				))
			})

		})

	})

	Context("on update", func() {
		var cpms *machinev1.ControlPlaneMachineSet
		var infrastructure *configv1.Infrastructure

		BeforeEach(func() {
			infrastructure = configv1builder.Infrastructure().AsAWS("cluster", "us-east-1").WithName("cluster").Build()
			Expect(k8sClient.Create(ctx, infrastructure)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, infrastructure)).To(Succeed())
		})

		Context("on AWS", func() {
			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-1")
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid
				cpms = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate).Build()

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)
				By("Creating a selection of Machines")
				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}

				By("Creating a valid ControlPlaneMachineSet")
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with an update to the providerSpec", func() {
				// Change the providerSpec, expect the update to be successful
				rawProviderSpec := machinev1beta1resourcebuilder.AWSProviderSpec().WithAvailabilityZone("us-east-2").BuildRawExtension()

				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value = rawProviderSpec
				})()).Should(Succeed())
			})

			It("with 4 replicas", func() {
				// This is an openapi validation but it makes sense to include it here as well
				Expect(komega.Update(cpms, func() {
					four := int32(4)
					cpms.Spec.Replicas = &four
				})()).Should(MatchError(ContainSubstring("Unsupported value: 4: supported values: \"3\", \"5\"")))
			})

			It("with 5 replicas", func() {
				// Five replicas is a valid value but the existing CPMS has three replicas
				Expect(komega.Update(cpms, func() {
					five := int32(5)
					cpms.Spec.Replicas = &five
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.replicas: Invalid value: \"integer\": replicas is immutable")), "Replicas should be immutable")
			})

			It("when modifying the machine labels and the selector still matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels["new"] = dummyValue
				})()).Should(Succeed(), "Machine label updates are allowed provided the selector still matches")
			})

			It("when modifying the machine labels so that the selector no longer matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels = map[string]string{
						"different":                          "labels",
						machinev1beta1.MachineClusterIDLabel: "cpms-cluster-test-id-different",
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
					}
				})()).Should(MatchError(ContainSubstring("selector does not match template labels")), "The selector must always match the machine labels")
			})

			It("when modifying the machine labels to remove the cluster ID label", func() {
				Expect(komega.Update(cpms, func() {
					delete(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-cluster' is required")), "The labels must always contain a cluster ID label")
			})

			It("when mutating the selector", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Selector.MatchLabels["new"] = dummyValue
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.selector: Invalid value: \"object\": selector is immutable")), "The selector should be immutable")
			})

			It("when adding invalid failure domain information", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1.FailureDomains{
						Platform: configv1.AWSPlatformType,
						Azure: &[]machinev1.AzureFailureDomain{
							{
								Zone: "us-central-1",
							},
						},
					}
				})()).To(MatchError(SatisfyAll(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Invalid value: \"object\": aws configuration is required when platform is AWS, and forbidden otherwise"),
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains: Invalid value: \"object\": azure configuration is required when platform is Azure, and forbidden otherwise"),
				)))
			})

			It("when adding invalid subnets in the faliure domains", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1.FailureDomains{
						Platform: configv1.AWSPlatformType,
						AWS: &[]machinev1.AWSFailureDomain{
							{
								Subnet: &machinev1.AWSResourceReference{
									Type: machinev1.AWSARNReferenceType,
									ID:   ptr.To[string]("id-123"),
								},
							},
						},
					}
				})()).To(MatchError(SatisfyAll(
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[0].subnet: Invalid value: \"object\": id is required when type is ID, and forbidden otherwise"),
					ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[0].subnet: Invalid value: \"object\": arn is required when type is ARN, and forbidden otherwise"),
				)))
			})
		})

		Context("on Azure", func() {
			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.AzureProviderSpec()
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid
				cpms = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate).Build()

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)
				By("Creating a selection of Machines")
				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}

				By("Creating a valid ControlPlaneMachineSet")
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with 4 replicas", func() {
				// This is an openapi validation but it makes sense to include it here as well
				Expect(komega.Update(cpms, func() {
					four := int32(4)
					cpms.Spec.Replicas = &four
				})()).Should(MatchError(ContainSubstring("Unsupported value: 4: supported values: \"3\", \"5\"")))
			})

			It("with 5 replicas", func() {
				// Five replicas is a valid value but the existing CPMS has three replicas
				Expect(komega.Update(cpms, func() {
					five := int32(5)
					cpms.Spec.Replicas = &five
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.replicas: Invalid value: \"integer\": replicas is immutable")), "Replicas should be immutable")
			})

			It("when modifying the machine labels and the selector still matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels["new"] = dummyValue
				})()).Should(Succeed(), "Machine label updates are allowed provided the selector still matches")
			})

			It("when modifying the machine labels so that the selector no longer matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels = map[string]string{
						"different":                          "labels",
						machinev1beta1.MachineClusterIDLabel: "cpms-cluster-test-id-different",
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
					}
				})()).Should(MatchError(ContainSubstring("selector does not match template labels")), "The selector must always match the machine labels")
			})

			It("when modifying the machine labels to remove the cluster ID label", func() {
				Expect(komega.Update(cpms, func() {
					delete(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-cluster' is required")), "The labels must always contain a cluster ID label")
			})

			It("when mutating the selector", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Selector.MatchLabels["new"] = dummyValue
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.selector: Invalid value: \"object\": selector is immutable")), "The selector should be immutable")
			})

			It("when removing the internal load balancer", func() {
				// Change the providerSpec, expect the update to be successful
				rawProviderSpec := machinev1beta1resourcebuilder.AzureProviderSpec().WithInternalLoadBalancer("").BuildRawExtension()

				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value = rawProviderSpec
				})()).Should(MatchError(ContainSubstring("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.internalLoadBalancer: Required value: internalLoadBalancer is required for control plane machines")))
			})
		})

		Context("on GCP", func() {
			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.GCPProviderSpec()
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid
				cpms = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate).Build()

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)
				By("Creating a selection of Machines")
				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}

				By("Creating a valid ControlPlaneMachineSet")
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with 4 replicas", func() {
				// This is an openapi validation but it makes sense to include it here as well
				Expect(komega.Update(cpms, func() {
					four := int32(4)
					cpms.Spec.Replicas = &four
				})()).Should(MatchError(ContainSubstring("Unsupported value: 4: supported values: \"3\", \"5\"")))
			})

			It("with 5 replicas", func() {
				// Five replicas is a valid value but the existing CPMS has three replicas
				Expect(komega.Update(cpms, func() {
					five := int32(5)
					cpms.Spec.Replicas = &five
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.replicas: Invalid value: \"integer\": replicas is immutable")), "Replicas should be immutable")
			})

			It("when modifying the machine labels and the selector still matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels["new"] = dummyValue
				})()).Should(Succeed(), "Machine label updates are allowed provided the selector still matches")
			})

			It("when modifying the machine labels so that the selector no longer matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels = map[string]string{
						"different":                          "labels",
						machinev1beta1.MachineClusterIDLabel: "cpms-cluster-test-id-different",
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
					}
				})()).Should(MatchError(ContainSubstring("selector does not match template labels")), "The selector must always match the machine labels")
			})

			It("when modifying the machine labels to remove the cluster ID label", func() {
				Expect(komega.Update(cpms, func() {
					delete(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-cluster' is required")), "The labels must always contain a cluster ID label")
			})

			It("when mutating the selector", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Selector.MatchLabels["new"] = dummyValue
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.selector: Invalid value: \"object\": selector is immutable")), "The selector should be immutable")
			})
		})

		Context("on OpenStack", func() {
			BeforeEach(func() {
				providerSpec := machinev1beta1resourcebuilder.OpenStackProviderSpec()
				machineTemplate := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().WithProviderSpecBuilder(providerSpec)
				// Default CPMS builder should be valid
				cpms = machinev1resourcebuilder.ControlPlaneMachineSet().WithNamespace(namespaceName).WithMachineTemplateBuilder(machineTemplate).Build()

				machineBuilder := machinev1beta1resourcebuilder.Machine().WithNamespace(namespaceName)
				controlPlaneMachineBuilder := machineBuilder.WithGenerateName("control-plane-machine-").AsMaster().WithProviderSpecBuilder(providerSpec)
				By("Creating a selection of Machines")
				for i := 0; i < 3; i++ {
					controlPlaneMachine := controlPlaneMachineBuilder.Build()
					Expect(k8sClient.Create(ctx, controlPlaneMachine)).To(Succeed())
				}

				By("Creating a valid ControlPlaneMachineSet")
				Expect(k8sClient.Create(ctx, cpms)).To(Succeed())
			})

			It("with 4 replicas", func() {
				// This is an openapi validation but it makes sense to include it here as well
				Expect(komega.Update(cpms, func() {
					four := int32(4)
					cpms.Spec.Replicas = &four
				})()).Should(MatchError(ContainSubstring("Unsupported value: 4: supported values: \"3\", \"5\"")))
			})

			It("with 5 replicas", func() {
				// Five replicas is a valid value but the existing CPMS has three replicas
				Expect(komega.Update(cpms, func() {
					five := int32(5)
					cpms.Spec.Replicas = &five
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.replicas: Invalid value: \"integer\": replicas is immutable")), "Replicas should be immutable")
			})

			It("when modifying the machine labels and the selector still matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels["new"] = dummyValue
				})()).Should(Succeed(), "Machine label updates are allowed provided the selector still matches")
			})

			It("when modifying the machine labels so that the selector no longer matches", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels = map[string]string{
						"different":                          "labels",
						machinev1beta1.MachineClusterIDLabel: "cpms-cluster-test-id-different",
						openshiftMachineRoleLabel:            masterMachineRole,
						openshiftMachineTypeLabel:            masterMachineRole,
					}
				})()).Should(MatchError(ContainSubstring("selector does not match template labels")), "The selector must always match the machine labels")
			})

			It("when modifying the machine labels to remove the cluster ID label", func() {
				Expect(komega.Update(cpms, func() {
					delete(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.template.machines_v1beta1_machine_openshift_io.metadata.labels: Invalid value: \"object\": label 'machine.openshift.io/cluster-api-cluster' is required")), "The labels must always contain a cluster ID label")
			})

			It("when mutating the selector", func() {
				Expect(komega.Update(cpms, func() {
					cpms.Spec.Selector.MatchLabels["new"] = dummyValue
				})()).Should(MatchError(ContainSubstring("ControlPlaneMachineSet.machine.openshift.io \"cluster\" is invalid: spec.selector: Invalid value: \"object\": selector is immutable")), "The selector should be immutable")
			})
		})
	})
})
