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
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	uberZap "go.uber.org/zap"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
var ctx = context.Background()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var targetPoolsNotSetWarningCounter = NewMessageCounter(fmt.Sprintf("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.targetPools: %s", warnTargetPoolsNotSet))
var vSphereTemplateWarningCounter = NewMessageCounter(fmt.Sprintf("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.template: %s", warnVSphereTemplateMayBeIgnored))
var vSphereFolderWarningCounter = NewMessageCounter(fmt.Sprintf("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.workspace.folder: %s", warnVSphereFolderMayBeIgnored))
var vSphereResourcePoolWarningCounter = NewMessageCounter(fmt.Sprintf("spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.workspace.resourcePool: %s", warnVSphereResourcePoolMayBeIgnored))

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.RawZapOpts(uberZap.Hooks(targetPoolsNotSetWarningCounter.Trigger,
		vSphereTemplateWarningCounter.Trigger, vSphereFolderWarningCounter.Trigger, vSphereResourcePoolWarningCounter.Trigger)), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1", "zz_generated.crd-manifests"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "machine", "v1beta1", "zz_generated.crd-manifests"),
			filepath.Join("..", "..", "..", "vendor", "github.com", "openshift", "api", "config", "v1", "zz_generated.crd-manifests"),
		},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{"testdata"},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Tweak the warnings behaviour at a config level.
	cfg.WarningHandlerWithContext = logf.NewKubeAPIWarningLogger(
		logf.KubeAPIWarningLoggerOptions{
			Deduplicate: false,
		},
	)

	testScheme = scheme.Scheme
	Expect(machinev1.Install(testScheme)).To(Succeed())
	Expect(machinev1beta1.Install(testScheme)).To(Succeed())
	Expect(configv1.Install(testScheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// CEL requires Kube 1.25 and above, so check for the minimum server version.
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	serverVersion, err := discoveryClient.ServerVersion()
	Expect(err).ToNot(HaveOccurred())

	Expect(serverVersion.Major).To(Equal("1"))

	minorInt, err := strconv.Atoi(strings.Split(serverVersion.Minor, "+")[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(minorInt).To(BeNumerically(">=", 25), fmt.Sprintf("This test suite requires a Kube API server of at least version 1.25, current version is 1.%s", serverVersion.Minor))

	komega.SetClient(k8sClient)
	komega.SetContext(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
