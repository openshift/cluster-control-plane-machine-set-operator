/*
Copyright 2024 Red Hat, Inc.

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

package helpers

import (
	"context"
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	errNoFeatureGatesFound  = errors.New("no featuregates found")
	errFeatureNotRegistered = errors.New("feature is not registered in FeatureGates")
)

type featureGateFilter struct {
	enabled  sets.Set[string]
	disabled sets.Set[string]
}

// NewFeatureGateFilter creates a new featureGateFilter from the cluster's FeatureGate.
func NewFeatureGateFilter(ctx context.Context, testFramework framework.Framework) (*featureGateFilter, error) {
	k8sClient := testFramework.GetClient()

	featureGate := &configv1.FeatureGate{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "cluster"}, featureGate); err != nil {
		return nil, fmt.Errorf("failed to get featuregate: %w", err)
	}

	clusterVersion := &configv1.ClusterVersion{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return nil, fmt.Errorf("failed to get clusterversion: %w", err)
	}

	desiredVersion := clusterVersion.Status.Desired.Version
	if len(desiredVersion) == 0 && len(clusterVersion.Status.History) > 0 {
		desiredVersion = clusterVersion.Status.History[0].Version
	}

	ret := &featureGateFilter{
		enabled:  sets.New[string](),
		disabled: sets.New[string](),
	}
	found := false

	for _, featureGateValues := range featureGate.Status.FeatureGates {
		if featureGateValues.Version != desiredVersion {
			continue
		}

		found = true

		for _, enabled := range featureGateValues.Enabled {
			ret.enabled.Insert(string(enabled.Name))
		}

		for _, disabled := range featureGateValues.Disabled {
			ret.disabled.Insert(string(disabled.Name))
		}

		break
	}

	if !found {
		return nil, errNoFeatureGatesFound
	}

	return ret, nil
}

// IsEnabled checks if the given feature gate is enabled on the current cluster.
func (f *featureGateFilter) IsEnabled(featureGateName string) bool {
	if f.enabled.Has(featureGateName) {
		return true
	}

	if f.disabled.Has(featureGateName) {
		return false
	}

	panic(fmt.Errorf("feature %q: %w %v", featureGateName, errFeatureNotRegistered, f.knownFeatures()))
}

func (f *featureGateFilter) knownFeatures() sets.Set[string] {
	return f.enabled.Clone().Union(f.disabled)
}
