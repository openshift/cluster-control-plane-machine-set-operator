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
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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
	default:
		return Unsupported, platformType, nil
	}
}
