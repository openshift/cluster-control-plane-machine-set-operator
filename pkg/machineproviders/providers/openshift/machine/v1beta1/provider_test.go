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

package v1beta1

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	"github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	machineprovidersresourcebuilder "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder/machineproviders"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("MachineProvider", func() {
	const ownerUID = "uid-1234abcd"
	const ownerName = "machineOwner"

	var namespaceName string
	var logger testutils.TestLogger
	var nilDiff []string
	instanceDiff := []string{"InstanceType: m6i.xlarge != different"}

	usEast1aSubnet := machinev1.AWSResourceReference{
		Type: machinev1.AWSFiltersReferenceType,
		Filters: &[]machinev1.AWSResourceFilter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1a",
				},
			},
		},
	}

	usEast1bSubnet := machinev1.AWSResourceReference{
		Type: machinev1.AWSFiltersReferenceType,
		Filters: &[]machinev1.AWSResourceFilter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1b",
				},
			},
		},
	}

	usEast1cSubnet := machinev1.AWSResourceReference{
		Type: machinev1.AWSFiltersReferenceType,
		Filters: &[]machinev1.AWSResourceFilter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1c",
				},
			},
		},
	}

	usEast1aSubnetbeta1 := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1a",
				},
			},
		},
	}

	usEast1bSubnetbeta1 := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1b",
				},
			},
		},
	}

	usEast1cSubnetbeta1 := machinev1beta1.AWSResourceReference{
		Filters: []machinev1beta1.Filter{
			{
				Name: "tag:Name",
				Values: []string{
					"subnet-us-east-1c",
				},
			},
		},
	}

	BeforeEach(OncePerOrdered, func() {
		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-controller-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		logger = testutils.NewTestLogger()
	})

	AfterEach(OncePerOrdered, func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&machinev1beta1.Machine{},
		)
	})

	Context("GetMachineInfos", func() {
		providerSpecBuilder := machinev1beta1resourcebuilder.AWSProviderSpec()
		masterMachineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().WithLabel(machinev1beta1.MachineClusterIDLabel, resourcebuilder.TestClusterIDValue).WithNamespace(namespaceName)
		masterNodeBuilder := corev1resourcebuilder.Node().AsMaster().AsReady()

		machineGVR := machinev1beta1.GroupVersion.WithResource("machines")
		nodeGVR := corev1.SchemeGroupVersion.WithResource("nodes")

		masterLabels := resourcebuilder.NewMachineRoleLabels("master")
		masterLabels[machinev1beta1.MachineClusterIDLabel] = resourcebuilder.TestClusterIDValue

		unreadyMachineInfoBuilder := machineprovidersresourcebuilder.MachineInfo().
			WithMachineGVR(machineGVR).
			WithMachineLabels(masterLabels).
			WithMachineNamespace(namespaceName).
			WithNodeGVR(nodeGVR).
			WithReady(false).
			WithNeedsUpdate(false)

		readyMachineInfoBuilder := machineprovidersresourcebuilder.MachineInfo().
			WithMachineGVR(machineGVR).
			WithMachineLabels(masterLabels).
			WithMachineNamespace(namespaceName).
			WithNodeGVR(nodeGVR).
			WithReady(true).
			WithNeedsUpdate(false)

		masterMachineName := func(suffix string) string {
			return fmt.Sprintf("%s-master-%s", resourcebuilder.TestClusterIDValue, suffix)
		}

		type getMachineInfosTableInput struct {
			machines             []*machinev1beta1.Machine
			nodes                []*corev1.Node
			failureDomains       map[int32]failuredomain.FailureDomain
			expectedError        error
			expectedMachineInfos []machineproviders.MachineInfo
			expectedLogs         []testutils.LogEntry
		}

		DescribeTable("builds machine info based on the cluster state", func(in getMachineInfosTableInput) {
			for _, machine := range in.machines {
				machine.SetNamespace(namespaceName)

				status := machine.Status.DeepCopy()

				Expect(k8sClient.Create(ctx, machine)).To(Succeed())

				machine.Status = *status
				Expect(k8sClient.Status().Update(ctx, machine)).To(Succeed())
			}

			for _, node := range in.nodes {
				status := node.Status.DeepCopy()

				Expect(k8sClient.Create(ctx, node)).To(Succeed())

				node.Status = *status
				Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
			}

			// Inject namespace in the expected machine infos since it doesn't happen during
			// initial building.
			for i := 0; i < len(in.expectedMachineInfos); i++ {
				in.expectedMachineInfos[i].MachineRef.ObjectMeta.SetNamespace(namespaceName)
			}

			providerSpec := providerSpecBuilder
			if len(in.failureDomains) == 0 {
				// When no failure domain information is provided, we assume all machines are in us-east-1a.
				providerSpec = providerSpec.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)
			}

			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().Build()

			template := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(providerSpec).
				WithLabel(machinev1beta1.MachineClusterIDLabel, resourcebuilder.TestClusterIDValue).
				BuildTemplate().OpenShiftMachineV1Beta1Machine
			Expect(template).ToNot(BeNil())

			providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(*template)
			Expect(err).ToNot(HaveOccurred())

			provider := &openshiftMachineProvider{
				client:               k8sClient,
				indexToFailureDomain: in.failureDomains,
				machineSelector:      cpms.Spec.Selector,
				machineTemplate:      *template,
				providerConfig:       providerConfig,
				namespace:            namespaceName,
			}

			machineInfos, err := provider.GetMachineInfos(ctx, logger.Logger())

			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			// We have to explicitly unset these parameters on the resulted machine infos
			// because otherwise they would differ from the expected ones.
			for i := 0; i < len(machineInfos); i++ {
				machineInfos[i].MachineRef.ObjectMeta.ManagedFields = nil
				machineInfos[i].MachineRef.ObjectMeta.UID = ""
				machineInfos[i].MachineRef.ObjectMeta.Generation = 0
				machineInfos[i].MachineRef.ObjectMeta.ResourceVersion = ""
				machineInfos[i].MachineRef.ObjectMeta.CreationTimestamp = metav1.Time{}
			}

			Expect(machineInfos).To(ConsistOf(in.expectedMachineInfos))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
		},
			Entry("with no Machines", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{},
				nodes:    []*corev1.Node{},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{},
				expectedLogs:         []testutils.LogEntry{},
			}),
			Entry("with unready Machines", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("").Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Provisioning").Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Provisioned").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					unreadyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).Build(),
					unreadyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).Build(),
					unreadyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "",
							"index", int32(0),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "",
							"index", int32(1),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "",
							"index", int32(2),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machines", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},

				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machine that has now been deleted", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Deleting").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with a Machine that was never ready, and has now been deleted", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Deleting").Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					unreadyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "",
							"index", int32(2),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with Machines using the random suffix pattern", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("abcde-0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("fghij-1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("abcde-0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("fghij-1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("abcde-0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("fghij-1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with one Machine with a different instance type", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithInstanceType("different").WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").WithNeedsUpdate(true).WithDiff(
						instanceDiff).Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", true,
							"diff", instanceDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with one Machine with an unknown failure domain", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1d")).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").
						WithDiff(
							[]string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1a != aws-subnet-12345678",
								"Placement.AvailabilityZone: us-east-1a != us-east-1d",
							},
						).WithNeedsUpdate(true).Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1a != aws-subnet-12345678",
								"Placement.AvailabilityZone: us-east-1a != us-east-1d",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with multiple Machines in an index in different states", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithInstanceType("different")).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("abcde-2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-replacement-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
					masterNodeBuilder.WithName("node-replacement-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").WithNeedsUpdate(true).
						WithDiff([]string{"InstanceType: m6i.xlarge != different", "Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != aws-subnet-12345678"}).Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("abcde-2")).WithNodeName("node-replacement-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"InstanceType: m6i.xlarge != different",
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != aws-subnet-12345678",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("abcde-2"),
							"nodeName", "node-replacement-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("when the failure domain mapping does not match, the mapping takes precedence for indexing", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					// The failure domain mapping logic is trusted as the source of truth for the failure domain.
					// It is responsible for mapping the machine indexes to failure domains.
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").WithNeedsUpdate(true).WithDiff(
						[]string{
							"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1b != subnet-us-east-1a",
							"Placement.AvailabilityZone: us-east-1b != us-east-1a",
						},
					).Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").WithNeedsUpdate(true).WithDiff(
						[]string{
							"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != subnet-us-east-1b",
							"Placement.AvailabilityZone: us-east-1c != us-east-1b",
						},
					).Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").WithNeedsUpdate(true).WithDiff(
						[]string{
							"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1a != subnet-us-east-1c",
							"Placement.AvailabilityZone: us-east-1a != us-east-1c",
						},
					).Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1b != subnet-us-east-1a",
								"Placement.AvailabilityZone: us-east-1b != us-east-1a",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != subnet-us-east-1b",
								"Placement.AvailabilityZone: us-east-1c != us-east-1b",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1a != subnet-us-east-1c",
								"Placement.AvailabilityZone: us-east-1a != us-east-1c",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("when the machine names do not fit the pattern, fall back to matching on failure domains", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-machine-a").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-machine-0").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-master-c").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(resourcebuilder.TestClusterIDValue + "-machine-0").WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(resourcebuilder.TestClusterIDValue + "-master-c").WithNodeName("node-2").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(resourcebuilder.TestClusterIDValue + "-machine-a").WithNodeName("node-1").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", resourcebuilder.TestClusterIDValue + "-machine-0",
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", resourcebuilder.TestClusterIDValue + "-master-c",
							"nodeName", "node-2",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", resourcebuilder.TestClusterIDValue + "-machine-a",
							"nodeName", "node-1",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("when the machine names do not fit the pattern, and the failure domains are not recognised, returns an error", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-machine-a").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-machine-1").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1f")).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(resourcebuilder.TestClusterIDValue + "-master-c").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1d")).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
				},
				expectedError:        errCouldNotDetermineMachineIndex,
				expectedMachineInfos: []machineproviders.MachineInfo{},
				expectedLogs: []testutils.LogEntry{
					{
						Error:   errCouldNotDetermineMachineIndex,
						Message: "Could not gather Machine Info",
					},
				},
			}),
			Entry("with Machines that have errored in some way", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Failed").WithErrorMessage("Node missing").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Failed").WithErrorMessage("Cannot create VM").Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					unreadyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithReady(false).WithErrorMessage("Node missing").WithNodeName("node-0").Build(),
					unreadyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithReady(false).WithErrorMessage("Cannot create VM").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "Node missing",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "",
							"index", int32(1),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "Cannot create VM",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with additional Machines, not matched by the selector, ignores them", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					machinev1beta1resourcebuilder.Machine().AsWorker().WithNamespace(namespaceName).WithName("worker-abcde").WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with a Machine whose failure domain does not match the mapping, should update the Machine", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").WithNeedsUpdate(true).
						WithDiff(
							[]string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != subnet-us-east-1a",
								"Placement.AvailabilityZone: us-east-1c != us-east-1a",
							},
						).Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{
								"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1c != subnet-us-east-1a",
								"Placement.AvailabilityZone: us-east-1c != us-east-1a",
							},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with no failure domain mapping and all Machines are in the correct availability zone", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with no failure domain mapping and not all Machines are in the correct availability zone", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").WithNeedsUpdate(true).
						WithDiff([]string{"Placement.AvailabilityZone: us-east-1a != us-east-1b"}).Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", true,
							"diff", []string{"Placement.AvailabilityZone: us-east-1a != us-east-1b"},
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machines that are indexed from 3", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("3")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-3"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("4")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-4"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("5")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-5"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-3").Build(),
					masterNodeBuilder.WithName("node-4").Build(),
					masterNodeBuilder.WithName("node-5").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					3: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					4: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					5: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(3).WithMachineName(masterMachineName("3")).WithNodeName("node-3").Build(),
					readyMachineInfoBuilder.WithIndex(4).WithMachineName(masterMachineName("4")).WithNodeName("node-4").Build(),
					readyMachineInfoBuilder.WithIndex(5).WithMachineName(masterMachineName("5")).WithNodeName("node-5").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("3"),
							"nodeName", "node-3",
							"index", int32(3),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("4"),
							"nodeName", "node-4",
							"index", int32(4),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("5"),
							"nodeName", "node-5",
							"index", int32(5),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machines that are not sequntially indexed", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("4")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-4"}).Build(),
				},
				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
					masterNodeBuilder.WithName("node-4").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					4: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
					readyMachineInfoBuilder.WithIndex(4).WithMachineName(masterMachineName("4")).WithNodeName("node-4").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("4"),
							"nodeName", "node-4",
							"index", int32(4),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machines but one of them has the Node unready for less than unreadyNodeGracePeriod", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},

				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").WithConditions(
						[]corev1.NodeCondition{
							{
								Type:               corev1.NodeReady,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.NewTime(time.Now().Add(-(unreadyNodeGracePeriod / 2))),
							},
						},
					).Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
			Entry("with ready Machines but one of them has the Node unready for more than unreadyNodeGracePeriod", getMachineInfosTableInput{
				machines: []*machinev1beta1.Machine{
					masterMachineBuilder.WithName(masterMachineName("0")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-0"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("1")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-1"}).Build(),
					masterMachineBuilder.WithName(masterMachineName("2")).WithProviderSpecBuilder(providerSpecBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1)).
						WithPhase("Running").WithNodeRef(corev1.ObjectReference{Name: "node-2"}).Build(),
				},

				nodes: []*corev1.Node{
					masterNodeBuilder.WithName("node-0").WithConditions(
						[]corev1.NodeCondition{
							{
								Type:               corev1.NodeReady,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.NewTime(time.Now().Add(-(unreadyNodeGracePeriod + time.Minute*1))),
							},
						},
					).Build(),
					masterNodeBuilder.WithName("node-1").Build(),
					masterNodeBuilder.WithName("node-2").Build(),
				},
				failureDomains: map[int32]failuredomain.FailureDomain{
					0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
					1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
					2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
				},
				expectedMachineInfos: []machineproviders.MachineInfo{
					readyMachineInfoBuilder.WithIndex(0).WithMachineName(masterMachineName("0")).WithNodeName("node-0").WithReady(false).Build(),
					readyMachineInfoBuilder.WithIndex(1).WithMachineName(masterMachineName("1")).WithNodeName("node-1").Build(),
					readyMachineInfoBuilder.WithIndex(2).WithMachineName(masterMachineName("2")).WithNodeName("node-2").Build(),
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("0"),
							"nodeName", "node-0",
							"index", int32(0),
							"ready", false,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("1"),
							"nodeName", "node-1",
							"index", int32(1),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
					{
						Level: 4,
						KeysAndValues: []interface{}{
							"machineName", masterMachineName("2"),
							"nodeName", "node-2",
							"index", int32(2),
							"ready", true,
							"needsUpdate", false,
							"diff", nilDiff,
							"errorMessage", "",
						},
						Message: "Gathered Machine Info",
					},
				},
			}),
		)
	})

	Context("CreateMachine", func() {
		var provider machineproviders.MachineProvider
		var template machinev1.ControlPlaneMachineSetTemplate

		assertCreatesMachine := func(index int32, expectedProviderConfig resourcebuilder.RawExtensionBuilder, clusterID, failureDomain string) {
			Context(fmt.Sprintf("creating a machine in index %d", index), Ordered, func() {
				// NOTE: this is an ordered container, each assertion in this
				// function will run ordered, rather than in parallel.
				// This means state can be shared between these.
				// We use this so that we can break up individual assertions
				// on the Machine state into separate containers.

				var err error
				var machine machinev1beta1.Machine

				BeforeAll(func() {
					err = provider.CreateMachine(ctx, logger.Logger(), index)
				})

				It("should not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				Context("should create a machine", func() {
					It("with a name in the correct format", func() {
						nameMatcher := MatchRegexp(fmt.Sprintf("%s-master-[a-z0-9]{5}-%d", clusterID, index))

						machineList := &machinev1beta1.MachineList{}
						Eventually(komega.ObjectList(machineList, client.InNamespace(namespaceName))).Should(HaveField("Items", ContainElement(
							HaveField("ObjectMeta.Name", nameMatcher),
						)))

						// Set the machine variable to the new Machine so that we can
						// inspect it on subsequent tests.
						for _, m := range machineList.Items {
							if ok, err := nameMatcher.Match(m.Name); err == nil && ok {
								machine = m
								break
							}
						}
					})

					It("with the labels from the Machine template", func() {
						Expect(machine.Labels).To(Equal(
							template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Labels,
						))
					})

					It("with annotations from the Machine template", func() {
						Expect(machine.Annotations).To(Equal(
							template.OpenShiftMachineV1Beta1Machine.ObjectMeta.Annotations,
						))
					})

					It("with the correct owner reference", func() {
						Expect(machine.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
							APIVersion:         machinev1.GroupVersion.String(),
							Kind:               "ControlPlaneMachineSet",
							Name:               ownerName,
							UID:                ownerUID,
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						}))
					})

					It("with the correct provider spec", func() {
						Expect(machine.Spec.ProviderSpec.Value).To(SatisfyAll(
							Not(BeNil()),
							HaveField("Raw", MatchJSON(expectedProviderConfig.BuildRawExtension().Raw)),
						))
					})

					It("with no providerID set", func() {
						Expect(machine.Spec.ProviderID).To(BeNil())
					})

					It("logs that the machine was created", func() {
						Expect(logger.Entries()).To(ConsistOf(
							testutils.LogEntry{
								Level: 2,
								KeysAndValues: []interface{}{
									"index", index,
									"machineName", machine.Name,
									"failureDomain", fmt.Sprintf("AWSFailureDomain{AvailabilityZone:%s, Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-%s]}]}}", failureDomain, failureDomain),
								},
								Message: "Created machine",
							},
						))
					})
				})
			})
		}

		Context("with an AWS template", func() {
			providerConfigBuilder := machinev1beta1resourcebuilder.AWSProviderSpec()

			BeforeEach(OncePerOrdered, func() {
				template = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
					WithProviderSpecBuilder(providerConfigBuilder).
					WithLabel(machinev1beta1.MachineClusterIDLabel, "cpms-aws-cluster-id").
					BuildTemplate()

				providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(*template.OpenShiftMachineV1Beta1Machine)
				Expect(err).ToNot(HaveOccurred())

				provider = &openshiftMachineProvider{
					client: k8sClient,
					indexToFailureDomain: map[int32]failuredomain.FailureDomain{
						0: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnet).Build()),
						1: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnet).Build()),
						2: failuredomain.NewAWSFailureDomain(machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnet).Build()),
					},
					machineSelector: machinev1resourcebuilder.ControlPlaneMachineSet().Build().Spec.Selector,
					machineTemplate: *template.OpenShiftMachineV1Beta1Machine,
					ownerMetadata: metav1.ObjectMeta{
						Name: ownerName,
						UID:  ownerUID,
					},
					providerConfig:   providerConfig,
					namespace:        namespaceName,
					machineAPIScheme: testScheme,
				}
			})

			assertCreatesMachine(0, providerConfigBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1), "cpms-aws-cluster-id", "us-east-1a")
			assertCreatesMachine(1, providerConfigBuilder.WithAvailabilityZone("us-east-1b").WithSubnet(usEast1bSubnetbeta1), "cpms-aws-cluster-id", "us-east-1b")
			assertCreatesMachine(2, providerConfigBuilder.WithAvailabilityZone("us-east-1c").WithSubnet(usEast1cSubnetbeta1), "cpms-aws-cluster-id", "us-east-1c")

			Context("if the Machine template is missing the cluster ID label", func() {
				var err error

				BeforeEach(func() {
					p, ok := provider.(*openshiftMachineProvider)
					Expect(ok).To(BeTrue())

					delete(p.machineTemplate.ObjectMeta.Labels, machinev1beta1.MachineClusterIDLabel)

					err = provider.CreateMachine(ctx, logger.Logger(), 0)
				})

				It("returns an error", func() {
					Expect(err).To(MatchError(errMissingClusterIDLabel))
				})

				It("does not create any Machines", func() {
					Consistently(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", BeEmpty()))
				})
			})

			Context("if the Machine template is missing the machine role label", func() {
				var err error

				BeforeEach(func() {
					p, ok := provider.(*openshiftMachineProvider)
					Expect(ok).To(BeTrue())

					delete(p.machineTemplate.ObjectMeta.Labels, openshiftMachineRoleLabel)

					err = provider.CreateMachine(ctx, logger.Logger(), 0)
				})

				It("returns an error", func() {
					Expect(err).To(MatchError(errMissingMachineRoleLabel))
				})

				It("does not create any Machines", func() {
					Consistently(komega.ObjectList(&machinev1beta1.MachineList{})).Should(HaveField("Items", BeEmpty()))
				})
			})

			Context("if the MachineProvider has no failure domains configure", func() {
				usEast1aBuilder := providerConfigBuilder.WithAvailabilityZone("us-east-1a").WithSubnet(usEast1aSubnetbeta1)

				BeforeEach(func() {
					template = machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
						WithProviderSpecBuilder(usEast1aBuilder).
						WithLabel(machinev1beta1.MachineClusterIDLabel, "cpms-aws-cluster-id").
						BuildTemplate()

					providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(*template.OpenShiftMachineV1Beta1Machine)
					Expect(err).ToNot(HaveOccurred())

					openshiftProvider, ok := provider.(*openshiftMachineProvider)
					Expect(ok).To(BeTrue(), "provider should be an openshiftMachineProvider")

					openshiftProvider.providerConfig = providerConfig
					openshiftProvider.indexToFailureDomain = nil
				})

				assertCreatesMachine(0, usEast1aBuilder, "cpms-aws-cluster-id", "us-east-1a")
				assertCreatesMachine(1, usEast1aBuilder, "cpms-aws-cluster-id", "us-east-1a")
				assertCreatesMachine(2, usEast1aBuilder, "cpms-aws-cluster-id", "us-east-1a")
			})
		})

	})

	Context("DeleteMachine", func() {
		var machineName string
		var machineRef *machineproviders.ObjectRef
		var machineProvider machineproviders.MachineProvider

		BeforeEach(func() {
			By("Setting up the MachineProvider")
			cpms := machinev1resourcebuilder.ControlPlaneMachineSet().Build()

			template := machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(machinev1beta1resourcebuilder.AWSProviderSpec()).
				BuildTemplate().OpenShiftMachineV1Beta1Machine
			Expect(template).ToNot(BeNil())

			providerConfig, err := providerconfig.NewProviderConfigFromMachineTemplate(*template)
			Expect(err).ToNot(HaveOccurred())

			machineProvider = &openshiftMachineProvider{
				client:               k8sClient,
				indexToFailureDomain: map[int32]failuredomain.FailureDomain{},
				machineSelector:      cpms.Spec.Selector,
				machineTemplate:      *template,
				ownerMetadata: metav1.ObjectMeta{
					UID: types.UID(ownerUID),
				},
				providerConfig: providerConfig,
				namespace:      namespaceName,
			}

			machineBuilder := machinev1beta1resourcebuilder.Machine().AsMaster().
				WithGenerateName("control-plane-machine-").
				WithNamespace(namespaceName)

			machine := machineBuilder.Build()
			Expect(k8sClient.Create(ctx, machine)).To(Succeed())
			machineName = machine.Name
		})

		Context("with the correct GVR", func() {
			BeforeEach(func() {
				machineRef = &machineproviders.ObjectRef{
					GroupVersionResource: machinev1beta1.GroupVersion.WithResource("machines"),
				}
			})

			Context("with an existing machine", func() {
				var err error

				BeforeEach(func() {
					machineRef.ObjectMeta.Name = machineName
					machineRef.ObjectMeta.Namespace = namespaceName

					err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
				})

				It("deletes the Machine", func() {
					machine := machinev1beta1resourcebuilder.Machine().
						WithNamespace(namespaceName).
						WithName(machineName).
						Build()

					notFoundErr := apierrors.NewNotFound(machineRef.GroupVersionResource.GroupResource(), machineName)

					Eventually(komega.Get(machine)).Should(MatchError(notFoundErr))
				})

				It("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				It("logs that the machine was deleted", func() {
					Expect(logger.Entries()).To(ConsistOf(
						testutils.LogEntry{
							Level: 2,
							KeysAndValues: []interface{}{
								"namespace", namespaceName,
								"machineName", machineName,
								"group", machinev1beta1.GroupVersion.Group,
								"version", machinev1beta1.GroupVersion.Version,
							},
							Message: "Deleted machine",
						},
					))
				})
			})

			Context("with an non-existent machine", func() {
				var err error
				const unknown = "unknown"

				BeforeEach(func() {
					machineRef.ObjectMeta.Name = unknown
					machineRef.ObjectMeta.Namespace = namespaceName

					err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
				})

				It("does not delete the existing Machine", func() {
					machine := machinev1beta1resourcebuilder.Machine().
						WithNamespace(namespaceName).
						WithName(machineName).
						Build()

					Consistently(komega.Get(machine)).Should(Succeed())
				})

				It("does not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})

				It("logs that the machine was already deleted", func() {
					Expect(logger.Entries()).To(ConsistOf(
						testutils.LogEntry{
							Level: 2,
							KeysAndValues: []interface{}{
								"namespace", namespaceName,
								"machineName", unknown,
								"group", machinev1beta1.GroupVersion.Group,
								"version", machinev1beta1.GroupVersion.Version,
							},
							Message: "Machine not found",
						},
					))
				})
			})
		})

		Context("with an incorrect GVR", func() {
			var err error

			BeforeEach(func() {
				machineRef := &machineproviders.ObjectRef{
					GroupVersionResource: machinev1.GroupVersion.WithResource("machines"),
					ObjectMeta: metav1.ObjectMeta{
						Name: machineName,
					},
				}

				err = machineProvider.DeleteMachine(ctx, logger.Logger(), machineRef)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(fmt.Errorf("%w: expected %s, got %s", errUnknownGroupVersionResource, machinev1beta1.GroupVersion.WithResource("machines").String(), machinev1.GroupVersion.WithResource("machines").String())))
			})

			It("logs the error", func() {
				Expect(logger.Entries()).To(ConsistOf(
					testutils.LogEntry{
						Error: errUnknownGroupVersionResource,
						KeysAndValues: []interface{}{
							"expectedGVR", machinev1beta1.GroupVersion.WithResource("machines").String(),
							"gotGVR", machinev1.GroupVersion.WithResource("machines").String(),
						},
						Message: "Could not delete machine",
					},
				))
			})
		})
	})
})
