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

package framework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	// errUnsupportedPlatform is returned when the platform is not supported.
	errUnsupportedPlatform = errors.New("unsupported platform")

	// errUnsupportedInstanceSize is returned when the instance size did not match the expected format.
	// Each platform will have it's own format for the instance size, and if we do not recognise the instance
	// size we cannot increase it.
	errInstanceTypeUnsupportedFormat = errors.New("instance type did not match expected format")

	// errUnsupportedInstanceSize is returned when the instance size is not supported.
	// This means that even though the format is correct, we haven't implemented the logic to increase
	// this instance size.
	errInstanceTypeNotSupported = errors.New("instance type is not supported")
)

// Framework is an interface for getting clients and information
// about the environment within test cases.
type Framework interface {
	// ControlPlaneMachineSetKey returns the object key for fetching a control plane
	// machine set.
	ControlPlaneMachineSetKey() runtimeclient.ObjectKey

	// LoadClient returns a new controller-runtime client.
	GetClient() runtimeclient.Client

	// GetContext returns a context.
	GetContext() context.Context

	// GetPlatformType returns the platform type.
	GetPlatformType() configv1.PlatformType

	// GetPlatformSupportLevel returns the support level for the current platform.
	GetPlatformSupportLevel() PlatformSupportLevel

	// GetScheme returns the scheme.
	GetScheme() *runtime.Scheme

	// NewEmptyControlPlaneMachineSet returns a new control plane machine set with
	// just the name and namespace set.
	NewEmptyControlPlaneMachineSet() *machinev1.ControlPlaneMachineSet

	// IncreaseProviderSpecInstanceSize increases the instance size of the
	// providerSpec passed. This is used to trigger updates to the Machines
	// managed by the control plane machine set.
	IncreaseProviderSpecInstanceSize(providerSpec *runtime.RawExtension) error

	// ConvertToControlPlaneMachineSetProviderSpec converts a control plane machine provider spec
	// to a control plane machine set suitable provider spec.
	ConvertToControlPlaneMachineSetProviderSpec(providerSpec machinev1beta1.ProviderSpec) (*runtime.RawExtension, error)

	// UpdateDefaultedValueFromCPMS updates a field that is defaulted by the defaulting webhook in the MAO with a desired value.
	UpdateDefaultedValueFromCPMS(rawProviderSpec *runtime.RawExtension) (*runtime.RawExtension, error)
}

// PlatformSupportLevel is used to identify which tests should run
// based on the platform.
type PlatformSupportLevel int

const (
	// Unsupported means that the platform is not supported
	// by CPMS.
	Unsupported PlatformSupportLevel = iota
	// Manual means that the platform is supported by CPMS,
	// but the CPMS must be created manually.
	Manual
	// Full means that the platform is supported by CPMS,
	// and the CPMS will be created automatically.
	Full
)

// framework is an implementation of the Framework interface.
// It is used to provide a common set of functionality to all of the
// test cases.
type framework struct {
	client       runtimeclient.Client
	platform     configv1.PlatformType
	supportLevel PlatformSupportLevel
	scheme       *runtime.Scheme
	namespace    string
}

// NewFramework initialises a new test framework for the E2E suite.
func NewFramework() (Framework, error) {
	sch, err := loadScheme()
	if err != nil {
		return nil, err
	}

	client, err := loadClient(sch)
	if err != nil {
		return nil, err
	}

	supportLevel, platform, err := getPlatformSupportLevel(client)
	if err != nil {
		return nil, err
	}

	return &framework{
		client:       client,
		platform:     platform,
		supportLevel: supportLevel,
		scheme:       sch,
		namespace:    MachineAPINamespace,
	}, nil
}

// NewFrameworkWith initialises a new test framework for the E2E suite
// using the existing scheme, client, platform and support level provided.
func NewFrameworkWith(sch *runtime.Scheme, client runtimeclient.Client, platform configv1.PlatformType, supportLevel PlatformSupportLevel, namespace string) Framework {
	return &framework{
		client:       client,
		platform:     platform,
		supportLevel: supportLevel,
		scheme:       sch,
		namespace:    namespace,
	}
}

// ControlPlaneMachineSetKey is the object key for fetching a control plane
// machine set.
func (f *framework) ControlPlaneMachineSetKey() runtimeclient.ObjectKey {
	return runtimeclient.ObjectKey{
		Namespace: f.namespace,
		Name:      ControlPlaneMachineSetName,
	}
}

// GetClient returns a controller-runtime client.
func (f *framework) GetClient() runtimeclient.Client {
	return f.client
}

// GetContext returns a context.
func (f *framework) GetContext() context.Context {
	return context.Background()
}

// GetPlatformType returns the platform type.
func (f *framework) GetPlatformType() configv1.PlatformType {
	return f.platform
}

// GetPlatformSupportLevel returns the support level for the current platform.
func (f *framework) GetPlatformSupportLevel() PlatformSupportLevel {
	return f.supportLevel
}

// GetScheme returns the scheme.
func (f *framework) GetScheme() *runtime.Scheme {
	return f.scheme
}

// NewEmptyControlPlaneMachineSet returns a new control plane machine set with
// just the name and namespace set.
func (f *framework) NewEmptyControlPlaneMachineSet() *machinev1.ControlPlaneMachineSet {
	return &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControlPlaneMachineSetName,
			Namespace: f.namespace,
		},
	}
}

// IncreaseProviderSpecInstanceSize increases the instance size of the instance on the providerSpec
// that is passed.
func (f *framework) IncreaseProviderSpecInstanceSize(rawProviderSpec *runtime.RawExtension) error {
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(machinev1beta1.MachineSpec{
		ProviderSpec: machinev1beta1.ProviderSpec{
			Value: rawProviderSpec,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get provider config: %w", err)
	}

	switch f.platform {
	case configv1.AWSPlatformType:
		return increaseAWSInstanceSize(rawProviderSpec, providerConfig)
	case configv1.AzurePlatformType:
		return increaseAzureInstanceSize(rawProviderSpec, providerConfig)
	case configv1.GCPPlatformType:
		return increaseGCPInstanceSize(rawProviderSpec, providerConfig)
	case configv1.NutanixPlatformType:
		return increaseNutanixInstanceSize(rawProviderSpec, providerConfig)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedPlatform, f.platform)
	}
}

// UpdateDefaultedValueFromCPMS updates a defaulted value from the ControlPlaneMachineSet
// for either AWS, Azure or GCP.
func (f *framework) UpdateDefaultedValueFromCPMS(rawProviderSpec *runtime.RawExtension) (*runtime.RawExtension, error) {
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(machinev1beta1.MachineSpec{
		ProviderSpec: machinev1beta1.ProviderSpec{
			Value: rawProviderSpec,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	switch f.platform {
	case configv1.AzurePlatformType:
		return updateCredentialsSecretNameAzure(providerConfig)
	case configv1.AWSPlatformType:
		return updateCredentialsSecretNameAWS(providerConfig)
	case configv1.GCPPlatformType:
		return updateCredentialsSecretNameGCP(providerConfig)
	case configv1.NutanixPlatformType:
		return updateCredentialsSecretNameNutanix(providerConfig)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatform, f.platform)
	}
}

// updateCredentialsSecretNameAzure updates the credentialSecret field from the ControlPlaneMachineSet.
func updateCredentialsSecretNameAzure(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	cfg := providerConfig.Azure().Config()
	cfg.CredentialsSecret = nil

	rawBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("error marshalling azure providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// updateCredentialsSecretNameAWS updates the credentialSecret field from the ControlPlaneMachineSet.
func updateCredentialsSecretNameAWS(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	cfg := providerConfig.AWS().Config()
	cfg.CredentialsSecret = nil

	rawBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("error marshalling aws providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// updateCredentialsSecretNameGCP updates the credentialSecret field from the ControlPlaneMachineSet.
func updateCredentialsSecretNameGCP(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	cfg := providerConfig.GCP().Config()
	cfg.CredentialsSecret = nil

	rawBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("error marshalling gcp providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// updateCredentialsSecretNameNutanix updates the credentialSecret field from the ControlPlaneMachineSet.
func updateCredentialsSecretNameNutanix(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	cfg := providerConfig.Nutanix().Config()
	cfg.CredentialsSecret = nil

	rawBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("error marshalling nutanix providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// ConvertToControlPlaneMachineSetProviderSpec converts a control plane machine provider spec
// to a raw, control plane machine set suitable provider spec.
func (f *framework) ConvertToControlPlaneMachineSetProviderSpec(providerSpec machinev1beta1.ProviderSpec) (*runtime.RawExtension, error) {
	providerConfig, err := providerconfig.NewProviderConfigFromMachineSpec(machinev1beta1.MachineSpec{
		ProviderSpec: providerSpec,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	switch f.platform {
	case configv1.AWSPlatformType:
		return convertAWSProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig)
	case configv1.AzurePlatformType:
		return convertAzureProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig)
	case configv1.GCPPlatformType:
		return convertGCPProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig)
	case configv1.NutanixPlatformType:
		return convertNutanixProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatform, f.platform)
	}
}

// convertAWSProviderConfigToControlPlaneMachineSetProviderSpec converts an AWS providerConfig into a
// raw control plane machine set provider spec.
func convertAWSProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	awsPs := providerConfig.AWS().Config()
	awsPs.Subnet = machinev1beta1.AWSResourceReference{}
	awsPs.Placement.AvailabilityZone = ""

	rawBytes, err := json.Marshal(awsPs)
	if err != nil {
		return nil, fmt.Errorf("error marshalling aws providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// convertGCPProviderConfigToControlPlaneMachineSetProviderSpec converts a GCP providerConfig into a
// raw control plane machine set provider spec.
func convertGCPProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	gcpPs := providerConfig.GCP().Config()
	gcpPs.Zone = ""

	rawBytes, err := json.Marshal(gcpPs)
	if err != nil {
		return nil, fmt.Errorf("error marshalling gcp providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// convertAzureProviderConfigToControlPlaneMachineSetProviderSpec converts an Azure providerConfig into a
// raw control plane machine set provider spec.
func convertAzureProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	azurePs := providerConfig.Azure().Config()
	azurePs.Zone = nil

	rawBytes, err := json.Marshal(azurePs)
	if err != nil {
		return nil, fmt.Errorf("error marshalling azure providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// convertNutanixProviderConfigToControlPlaneMachineSetProviderSpec converts a Nutanix providerConfig into a
// raw control plane machine set provider spec.
func convertNutanixProviderConfigToControlPlaneMachineSetProviderSpec(providerConfig providerconfig.ProviderConfig) (*runtime.RawExtension, error) {
	nutanixProviderConfig := providerConfig.Nutanix().Config()

	rawBytes, err := json.Marshal(nutanixProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("error marshalling nutanix providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// loadClient returns a new controller-runtime client.
func loadClient(sch *runtime.Scheme) (runtimeclient.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	client, err := runtimeclient.New(cfg, runtimeclient.Options{
		Scheme: sch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}

// addToSchemeFunc is an alias for a function that will add types to the scheme.
// We use this to loop and handle the errors for each scheme.
type addToSchemeFunc func(*runtime.Scheme) error

// loadScheme creates a scheme with all of the required types for the
// tests, pre-registered.
func loadScheme() (*runtime.Scheme, error) {
	sch := scheme.Scheme

	var errs []error

	for _, f := range []addToSchemeFunc{
		configv1.AddToScheme,
		machinev1.AddToScheme,
		machinev1beta1.AddToScheme,
	} {
		if err := f(sch); err != nil {
			errs = append(errs, fmt.Errorf("failed to add to scheme: %w", err))
		}
	}

	if len(errs) > 0 {
		return nil, kerrors.NewAggregate(errs)
	}

	return sch, nil
}

// getPlatformSupportLevel returns the support level for the current platform.
func getPlatformSupportLevel(k8sClient runtimeclient.Client) (PlatformSupportLevel, configv1.PlatformType, error) {
	infra := &configv1.Infrastructure{}

	if err := k8sClient.Get(context.Background(), runtimeclient.ObjectKey{Name: "cluster"}, infra); err != nil {
		return Unsupported, configv1.NonePlatformType, fmt.Errorf("failed to get infrastructure resource: %w", err)
	}

	platformType := infra.Status.PlatformStatus.Type

	switch platformType {
	case configv1.AWSPlatformType:
		return Full, platformType, nil
	case configv1.AzurePlatformType:
		return Manual, platformType, nil
	case configv1.GCPPlatformType:
		return Manual, platformType, nil
	case configv1.NutanixPlatformType:
		return Manual, platformType, nil
	default:
		return Unsupported, platformType, nil
	}
}

// increaseAWSInstanceSize increases the instance size of the instance on the providerSpec for an AWS providerSpec.
func increaseAWSInstanceSize(rawProviderSpec *runtime.RawExtension, providerConfig providerconfig.ProviderConfig) error {
	cfg := providerConfig.AWS().Config()

	var err error

	cfg.InstanceType, err = nextAWSInstanceSize(cfg.InstanceType)
	if err != nil {
		return fmt.Errorf("failed to get next instance size: %w", err)
	}

	if err := setProviderSpecValue(rawProviderSpec, cfg); err != nil {
		return fmt.Errorf("failed to set provider spec value: %w", err)
	}

	return nil
}

// nextAWSInstanceSize returns the next AWS instance size in the series.
// In AWS terms this normally means doubling the size of the underlying instance.
// For example:
// - m6i.large -> m6i.xlarge
// - m6i.xlarge -> m6i.2xlarge
// - m6i.2xlarge -> m6i.4xlarge
// This should mean we do not need to update this when the installer changes the default instance size.
func nextAWSInstanceSize(current string) (string, error) {
	// Regex to match the AWS instance type string.
	re := regexp.MustCompile(`(?P<family>[a-z0-9]+)\.(?P<multiplier>\d)?(?P<size>[a-z]+)`)

	values := re.FindStringSubmatch(current)
	if len(values) != 4 {
		return "", fmt.Errorf("%w: %s", errInstanceTypeUnsupportedFormat, current)
	}

	family := values[1]
	size := values[3]

	if multiplier := values[2]; multiplier == "" {
		switch size {
		case "large":
			return fmt.Sprintf("%s.xlarge", family), nil
		case "xlarge":
			return fmt.Sprintf("%s.2xlarge", family), nil
		default:
			return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
		}
	}

	multiplierInt, err := strconv.Atoi(values[2])
	if err != nil {
		// This is a panic because the multiplier should always be a number.
		panic("failed to convert multiplier to int")
	}

	return fmt.Sprintf("%s.%d%s", family, multiplierInt*2, size), nil
}

// increaseAzureInstanceSize increases the instance size of the instance on the providerSpec for an Azure providerSpec.
func increaseAzureInstanceSize(rawProviderSpec *runtime.RawExtension, providerConfig providerconfig.ProviderConfig) error {
	cfg := providerConfig.Azure().Config()

	var err error

	cfg.VMSize, err = nextAzureVMSize(cfg.VMSize)
	if err != nil {
		return fmt.Errorf("failed to get next instance size: %w", err)
	}

	if err := setProviderSpecValue(rawProviderSpec, cfg); err != nil {
		return fmt.Errorf("failed to set provider spec value: %w", err)
	}

	return nil
}

// nextAzureVMSize returns the next Azure VM size in the series.
// In Azure terms this normally means doubling the size of the underlying instance.
// This should mean we do not need to update this when the installer changes the default instance size.
func nextAzureVMSize(current string) (string, error) {
	// Regex to match the Azure VM size string.
	re := regexp.MustCompile(`Standard_(?P<family>[a-zA-Z]+)(?P<multiplier>[0-9]+)(?P<subfamily>[a-z]*)(?P<version>_v[0-9]+)?`)

	values := re.FindStringSubmatch(current)
	if len(values) != 5 {
		return "", fmt.Errorf("%w: %s", errInstanceTypeUnsupportedFormat, current)
	}

	family := values[1]
	subfamily := values[3]
	version := values[4]

	multiplier, err := strconv.Atoi(values[2])
	if err != nil {
		// This is a panic because the multiplier should always be a number.
		panic("failed to convert multiplier to int")
	}

	switch {
	case multiplier == 32:
		multiplier = 48
	case multiplier == 48:
		multiplier = 64
	case multiplier >= 64:
		return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
	default:
		multiplier *= 2
	}

	return fmt.Sprintf("Standard_%s%d%s%s", family, multiplier, subfamily, version), nil
}

// increaseGCPInstanceSize increases the instance size of the instance on the providerSpec for an GCP providerSpec.
func increaseGCPInstanceSize(rawProviderSpec *runtime.RawExtension, providerConfig providerconfig.ProviderConfig) error {
	cfg := providerConfig.GCP().Config()

	var err error

	cfg.MachineType, err = nextGCPMachineSize(cfg.MachineType)
	if err != nil {
		return fmt.Errorf("failed to get next instance size: %w", err)
	}

	if err := setProviderSpecValue(rawProviderSpec, cfg); err != nil {
		return fmt.Errorf("failed to set provider spec value: %w", err)
	}

	return nil
}

// increateNutanixInstanceSize increases the instance size of the instance on the providerSpec for an Nutanix providerSpec.
func increaseNutanixInstanceSize(rawProviderSpec *runtime.RawExtension, providerConfig providerconfig.ProviderConfig) error {
	cfg := providerConfig.Nutanix().Config()
	cfg.VCPUSockets++

	if err := setProviderSpecValue(rawProviderSpec, cfg); err != nil {
		return fmt.Errorf("failed to set provider spec value: %w", err)
	}

	return nil
}

// nextGCPVMSize returns the next GCP machine size in the series.
// The Machine sizes being used are in format <e2|n2|n1>-standard-<number>.
func nextGCPMachineSize(current string) (string, error) {
	// Regex to match the GCP machine size string.
	re := regexp.MustCompile(`(?P<family>[0-9a-z]+)-standard-(?P<multiplier>[0-9]+)`)

	values := re.FindStringSubmatch(current)
	if len(values) != 3 {
		return "", fmt.Errorf("%w: %s", errInstanceTypeUnsupportedFormat, current)
	}

	multiplier, err := strconv.Atoi(values[2])
	if err != nil {
		// This is a panic because the multiplier should always be a number.
		panic("failed to convert multiplier to int")
	}

	family := values[1]

	return setNextGCPMachineSize(current, family, multiplier)
}

// setNextGCPMachineSize returns the new GCP machine size in the series
// according to the family supported (e2, n1, n2).
//
//nolint:cyclop
func setNextGCPMachineSize(current, family string, multiplier int) (string, error) {
	switch {
	case multiplier >= 32 && family == "e2":
		return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
	case multiplier == 32 && family == "n2":
		multiplier = 48
	case multiplier == 64 && family == "n2":
		multiplier = 80
	case multiplier == 64 || multiplier == 80:
		multiplier = 96
	case multiplier >= 96 && family == "n1":
		return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
	case multiplier == 96:
		multiplier = 128
	case multiplier >= 128:
		return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
	default:
		multiplier *= 2
	}

	return fmt.Sprintf("%s-standard-%d", family, multiplier), nil
}

// setProviderSpecValue sets the value of the provider spec to the value that is passed.
func setProviderSpecValue(rawProviderSpec *runtime.RawExtension, value interface{}) error {
	providerSpecValue, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&value)
	if err != nil {
		return fmt.Errorf("failed to convert provider spec to unstructured: %w", err)
	}

	rawProviderSpec.Object = &unstructured.Unstructured{
		Object: providerSpecValue,
	}
	rawProviderSpec.Raw = nil

	return nil
}
