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

package controlplanemachinesetgenerator

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

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme *runtime.Scheme
var testRESTMapper meta.RESTMapper
var ctx = context.Background()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1beta1"),
			// Specify specific CRDs to avoid loading all of openshift/api/config/v1, and to avoid loading non Default featuregate CRDs.
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1", "0000_10_config-operator_01_infrastructure-Default.crd.yaml"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1", "0000_00_cluster-version-operator_01_clusterversion-Default.crd.yaml"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1", "0000_10_config-operator_01_featuregate.crd.yaml"),
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

	httpClient, err := rest.HTTPClientFor(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(httpClient).NotTo(BeNil())

	testRESTMapper, err = apiutil.NewDynamicRESTMapper(cfg, httpClient)
	Expect(err).NotTo(HaveOccurred())

	// Setting a fake version to allow for feature gate / cluster version resolution.
	Expect(os.Setenv("RELEASE_VERSION", "4.14.0")).To(Succeed())

	createClusterVersion()
	createFeatureGate()

	komega.SetClient(k8sClient)
	komega.SetContext(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
