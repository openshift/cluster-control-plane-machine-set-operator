package helpers

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type featureGateFilter struct {
	enabled  sets.String
	disabled sets.String
}

func newFeatureGateFilter(ctx context.Context, testFramework framework.Framework) (*featureGateFilter, error) {
	k8sClient := testFramework.GetClient()

	featureGate := &configv1.FeatureGate{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "cluster"}, featureGate); err != nil {
		return nil, err
	}
	clusterVersion := &configv1.ClusterVersion{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return nil, err
	}

	desiredVersion := clusterVersion.Status.Desired.Version
	if len(desiredVersion) == 0 && len(clusterVersion.Status.History) > 0 {
		desiredVersion = clusterVersion.Status.History[0].Version
	}

	ret := &featureGateFilter{
		enabled:  sets.NewString(),
		disabled: sets.NewString(),
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
		return nil, fmt.Errorf("no featuregates found")
	}

	return ret, nil
}

func (f *featureGateFilter) isEnabled(featureGateName string) bool {
	if f.enabled.Has(featureGateName) {
		return true
	}

	if f.disabled.Has(featureGateName) {
		return false
	}

	panic(fmt.Errorf("feature %q is not registered in FeatureGates %v", featureGateName, f.knownFeatures()))
}

func (f *featureGateFilter) knownFeatures() sets.String {
	return f.enabled.Clone().Union(f.disabled)
}
