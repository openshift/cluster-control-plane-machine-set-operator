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

package controlplanemachineset

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	configv1builder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	//+kubebuilder:scaffold:imports
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme *runtime.Scheme
var testRESTMapper meta.RESTMapper
var ctx = context.Background()

const releaseVersion = "4.14.0"

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	klog.SetOutput(GinkgoWriter)

	logf.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1", "zz_generated.crd-manifests"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1beta1", "zz_generated.crd-manifests"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1", "zz_generated.crd-manifests"),
		},
		ErrorIfCRDPathMissing:   true,
		ControlPlaneStopTimeout: 2 * time.Minute,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	testScheme = scheme.Scheme
	Expect(machinev1.Install(testScheme)).To(Succeed())
	Expect(machinev1beta1.Install(testScheme)).To(Succeed())
	Expect(configv1.Install(testScheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	infrastructure := configv1builder.Infrastructure().AsAWS("test", "eu-west-2").WithName(util.InfrastructureName).Build()
	Expect(k8sClient.Create(ctx, infrastructure)).To(Succeed())

	httpClient, err := rest.HTTPClientFor(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(httpClient).NotTo(BeNil())

	testRESTMapper, err = apiutil.NewDynamicRESTMapper(cfg, httpClient)
	Expect(err).NotTo(HaveOccurred())

	// Setting a fake version to allow for feature gate / cluster version resolution.
	Expect(os.Setenv("RELEASE_VERSION", releaseVersion)).To(Succeed())

	createClusterVersion(releaseVersion)

	komega.SetClient(k8sClient)
	komega.SetContext(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Helper method to create the ClusterVersion "version" resource.
func createClusterVersion(version string) {
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
				Version: version,
			},
		},
	}
	cvStatus := clusterVersion.Status.DeepCopy()
	Expect(k8sClient.Create(ctx, clusterVersion)).To(Succeed())
	clusterVersion.Status = *cvStatus
	Expect(k8sClient.Status().Update(ctx, clusterVersion)).To(Succeed())
}

// Helper method to crate the FeatureGate "cluster" resource.
func createFeatureGate(version string, enabled []configv1.FeatureGateAttributes, disabled []configv1.FeatureGateAttributes) {
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
					Enabled:  enabled,
					Disabled: disabled,
					Version:  version,
				},
			},
		},
	}
	fgStatus := featureGate.Status.DeepCopy()
	Expect(k8sClient.Create(ctx, featureGate)).To(Succeed())
	featureGate.Status = *fgStatus
	Expect(k8sClient.Status().Update(ctx, featureGate)).To(Succeed())
}
