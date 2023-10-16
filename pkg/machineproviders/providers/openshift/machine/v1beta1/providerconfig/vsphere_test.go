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

package providerconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("VSphere Provider Config", func() {
	var logger testutils.TestLogger

	var providerConfig VSphereProviderConfig

	usCentral1a := "us-central1-a"
	usCentral1b := "us-central1-b"

	logger = testutils.NewTestLogger()

	BeforeEach(func() {
		machineProviderConfig := machinev1beta1resourcebuilder.VSphereProviderSpec().
			WithZone(usCentral1a).
			Build()

		providerConfig = VSphereProviderConfig{
			providerConfig: *machineProviderConfig,
			infrastructure: configv1resourcebuilder.Infrastructure().AsVSphereWithFailureDomains("vsphere-test", nil).Build(),
		}
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.VSphereFailureDomain().
				WithZone(usCentral1a).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig VSphereProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.VSphereFailureDomain().
				WithZone(usCentral1b).
				Build()

			var err error
			changedProviderConfig, err = providerConfig.InjectFailureDomain(changedFailureDomain)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := machinev1resourcebuilder.VSphereFailureDomain().
					WithZone(usCentral1b).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.VSphereFailureDomain().
					WithZone(usCentral1a).
					Build()

				Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
			})
		})
	})

	Context("newVSphereProviderConfig", func() {
		var providerConfig ProviderConfig
		var expectedVSphereConfig machinev1beta1.VSphereMachineProviderSpec

		BeforeEach(func() {
			configBuilder := machinev1beta1resourcebuilder.VSphereProviderSpec()
			expectedVSphereConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			infrastructure := configv1resourcebuilder.Infrastructure().AsVSphere("vsphere-test").Build()
			var err error
			providerConfig, err = newVSphereProviderConfig(logger.Logger(), rawConfig, infrastructure)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the type to VSphere", func() {
			Expect(providerConfig.Type()).To(Equal(configv1.VSpherePlatformType))
		})

		It("returns the correct VSphere config", func() {
			Expect(providerConfig.VSphere()).ToNot(BeNil())
			Expect(providerConfig.VSphere().Config()).To(Equal(expectedVSphereConfig))
		})
	})
})
