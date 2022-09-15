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

package generate

import (
	"context"
	"errors"
	"log"
	"os"

	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ghodss/yaml"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	machinev1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1"
	machinev1beta1builder "github.com/openshift/client-go/machine/applyconfigurations/machine/v1beta1"
	machineclient "github.com/openshift/client-go/machine/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// options holds the options specified via flags.
type options struct {
	To         string
	Kubeconfig string
}

var (
	// ErrInvalidCPMSNumber defines an error for an invalid number of control plane machines.
	ErrInvalidCPMSNumber = errors.New("invalid number of control plane machines")
)

const (
	machineAPINamespace            = "openshift-machine-api"
	masterMachineSelector          = "machine.openshift.io/cluster-api-machine-role=master"
	clusterIDLabelKey              = "machine.openshift.io/cluster-api-cluster"
	clusterMachineRoleLabelKey     = "machine.openshift.io/cluster-api-machine-role"
	clusterMachineTypeLabelKey     = "machine.openshift.io/cluster-api-machine-type"
	clusterMachineLabelValueMaster = "master"
	infrastructureObjectName       = "cluster"
	cpmsObjectName                 = "cluster"
)

// NewGenerateCmd implements the "generate" subcommand for cpms manifest generation.
func NewGenerateCmd() *cobra.Command {
	generateOpts := options{}

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Manage generation of the Control Plane Machine Set manifest",
		Long:  "Generating Control Plane Machine Set manifest",
		Run:   generateCmdFunc(&generateOpts),
	}

	generateCmd.PersistentFlags().StringVar(&generateOpts.To, "to", "",
		"User-defined destination file for the generated manifest, when omitted the result is emitted to stdout")

	generateCmd.PersistentFlags().StringVar(&generateOpts.Kubeconfig, "kubeconfig", "",
		"User-defined kubeconfig path for the cluster, when omitted it falls back to inClusterConfig, then to default config")

	return generateCmd
}

// generateCmdFuc specifies a function to handle the generate command.
func generateCmdFunc(generateOpts *options) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		generatedCPMS, err := generateCPMSManifest(generateOpts)
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}

		// Print to stdout if no output file is specified.
		if generateOpts.To == "" {
			fmt.Printf("%s", generatedCPMS)
			return
		}

		if err := os.WriteFile(generateOpts.To, generatedCPMS, 0600); err != nil {
			log.Fatalf("failed to write file %s: %v", generateOpts.To, err)
		}
	}
}

// generateCPMSManifest generates the cpms manifest.
func generateCPMSManifest(generateOpts *options) ([]byte, error) {
	ctx := context.Background()

	config, err := clientcmd.BuildConfigFromFlags("", generateOpts.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	machineClientSet, err := machineclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	configClientSet, err := configclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	cpmsSpec, err := generateCPMSSpec(ctx, machineClientSet, configClientSet)
	if err != nil {
		return nil, err
	}

	cpms := machinev1builder.ControlPlaneMachineSet(cpmsObjectName, machineAPINamespace).WithSpec(&cpmsSpec)

	yamlCpms, err := yaml.Marshal(cpms)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cpms yaml: %w", err)
	}

	return yamlCpms, nil
}

// generateCPMSSpec generates a CPMS spec with provider specific details by detecting the provider and mapping its failure domains.
func generateCPMSSpec(ctx context.Context, machineClientSet *machineclient.Clientset, configClientSet *configclient.Clientset) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	machineSetClient := machineClientSet.MachineV1beta1().MachineSets(machineAPINamespace)
	machineClient := machineClientSet.MachineV1beta1().Machines(machineAPINamespace)

	machineSets, err := machineSetClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("error listing MachineSets: %w", err)
	}

	machines, err := machineClient.List(ctx, metav1.ListOptions{LabelSelector: masterMachineSelector})
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("error listing Machines: %w", err)
	}

	// Here we map the current number of control plane machines in the cluster
	// to the number of indexes in the Control Plane Machine Set spec.
	// The number of indexes by a CPMS is either 3 or 5,
	// so we error if that's not what the current count
	// of control plane machine in the cluster is.
	indexesCount := len(machines.Items)
	if indexesCount != 3 && indexesCount != 5 {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{},
			fmt.Errorf("%w: %d, supported values are: 3,5", ErrInvalidCPMSNumber, indexesCount)
	}

	infrastructureClient := configClientSet.ConfigV1().Infrastructures()

	infra, err := infrastructureClient.Get(ctx, infrastructureObjectName, metav1.GetOptions{})
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("error getting Infrastructure object: %w", err)
	}

	var spec machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration

	switch infra.Spec.PlatformSpec.Type {
	case configv1.AWSPlatformType:
		spec, err = generateCPMSAWSSpec(machineSets, machines)
		if err != nil {
			return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("error listing Machines: %w", err)
		}
	default:
		fmt.Printf("failed to generate config: unsupported platform %s", infra.Spec.PlatformSpec.Type)
	}

	return spec, nil
}

// generateCPMSAWSSpec generates a CPMS spec with AWS specific failure domain details.
func generateCPMSAWSSpec(msetList *machinev1beta1.MachineSetList, mList *machinev1beta1.MachineList) (machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration, error) {
	clusterID := mList.Items[0].ObjectMeta.Labels[clusterIDLabelKey]
	replicas := int32(len(mList.Items))

	// Decode the ProviderSpec for modification.
	ps, err := providerSpecFromRawExtension(mList.Items[0].Spec.ProviderSpec.Value)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to decode machine providerSpec: %w", err)
	}

	// Remove Failure domains already present in the Control Plane Machine Set spec.
	ps.Subnet = machinev1beta1.AWSResourceReference{}
	ps.Placement.AvailabilityZone = ""

	// Re-encode the ProviderSpec.
	re, err := rawExtensionFromProviderSpec(ps)
	if err != nil {
		return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to encode machine providerSpec: %w", err)
	}

	cpmsSpec := genericCPMSSpec(replicas, clusterID)

	subnetType := machinev1.AWSFiltersReferenceType
	aws := []machinev1builder.AWSFailureDomainApplyConfiguration{}

	for _, machineSet := range msetList.Items {
		ps, err := providerSpecFromRawExtension(machineSet.Spec.Template.Spec.ProviderSpec.Value)
		if err != nil {
			return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{}, fmt.Errorf("failed to encode machine providerSpec: %w", err)
		}

		cf := machinev1builder.AWSFailureDomainApplyConfiguration{}
		cf.Placement = &machinev1builder.AWSFailureDomainPlacementApplyConfiguration{
			AvailabilityZone: &ps.Placement.AvailabilityZone,
		}

		filters := []machinev1builder.AWSResourceFilterApplyConfiguration{}
		for _, f := range ps.Subnet.Filters {
			filters = append(filters, machinev1builder.AWSResourceFilterApplyConfiguration{Values: f.Values, Name: &(f.Name)})
		}

		cf.Subnet = &machinev1builder.AWSResourceReferenceApplyConfiguration{
			Type:    &subnetType,
			Filters: &filters,
		}

		aws = append(aws, cf)
	}

	platform := configv1.AWSPlatformType
	cpmsSpec.Template.OpenShiftMachineV1Beta1Machine.FailureDomains = &machinev1builder.FailureDomainsApplyConfiguration{
		AWS:      &aws,
		Platform: &platform,
	}

	cpmsSpec.Template.OpenShiftMachineV1Beta1Machine.Spec = &machinev1beta1builder.MachineSpecApplyConfiguration{
		ProviderSpec: &machinev1beta1builder.ProviderSpecApplyConfiguration{Value: re},
	}

	return cpmsSpec, nil
}

// genericCPMSSpec returns a generic Control Plane Machine Set spec, without provider specific details.
func genericCPMSSpec(replicas int32, clusterID string) machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration {
	cpmsLabels := map[string]string{
		clusterMachineRoleLabelKey: clusterMachineLabelValueMaster,
		clusterMachineTypeLabelKey: clusterMachineLabelValueMaster,
	}
	labels := map[string]string{
		clusterIDLabelKey:          clusterID,
		clusterMachineRoleLabelKey: clusterMachineLabelValueMaster,
		clusterMachineTypeLabelKey: clusterMachineLabelValueMaster,
	}
	machineType := machinev1.OpenShiftMachineV1Beta1MachineType
	strategy := machinev1.RollingUpdate

	return machinev1builder.ControlPlaneMachineSetSpecApplyConfiguration{
		Replicas: &replicas,
		Strategy: &machinev1builder.ControlPlaneMachineSetStrategyApplyConfiguration{
			Type: &strategy,
		},
		Selector: &metav1.LabelSelector{
			MatchLabels: cpmsLabels,
		},
		Template: &machinev1builder.ControlPlaneMachineSetTemplateApplyConfiguration{
			MachineType: &machineType,
			OpenShiftMachineV1Beta1Machine: &machinev1builder.OpenShiftMachineV1Beta1MachineTemplateApplyConfiguration{
				ObjectMeta: &machinev1builder.ControlPlaneMachineSetTemplateObjectMetaApplyConfiguration{
					Labels: labels,
				},
			},
		},
	}
}
