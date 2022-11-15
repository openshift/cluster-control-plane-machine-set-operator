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
	"errors"
	"fmt"
	"regexp"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"

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

	// IncreaseProviderSpecInstanceSize increases the instance size of the
	// providerSpec passed. This is used to trigger updates to the Machines
	// managed by the control plane machine set.
	IncreaseProviderSpecInstanceSize(providerSpec *runtime.RawExtension) error
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
	}, nil
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
	default:
		return fmt.Errorf("%w: %s", errUnsupportedPlatform, f.platform)
	}
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

// nextGCPVMSize returns the next GCP machine size in the series.
// The Machine sizes being used are in format e2-standard-<number>,
// where the number is a factor of 2 - from 2, up to 32.
func nextGCPMachineSize(current string) (string, error) {
	// Regex to match the GCP machine size string.
	// e.g. e2-standard-2 --- e2-standard-32
	re := regexp.MustCompile(`e2-standard-(?P<version>[0-9]+)`)

	values := re.FindStringSubmatch(current)
	if len(values) != 2 {
		return "", fmt.Errorf("%w: %s", errInstanceTypeUnsupportedFormat, current)
	}

	multiplier, err := strconv.Atoi(values[1])
	if err != nil {
		// This is a panic because the multiplier should always be a number.
		panic("failed to convert multiplier to int")
	}

	switch {
	case multiplier == 2:
		multiplier = 4
	case multiplier == 4:
		multiplier = 8
	case multiplier == 8:
		multiplier = 16
	case multiplier == 16:
		multiplier = 32
	case multiplier >= 32:
		return "", fmt.Errorf("%w: %s", errInstanceTypeNotSupported, current)
	default:
		multiplier *= 2
	}

	return fmt.Sprintf("e2-standard-%d", multiplier), nil
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
