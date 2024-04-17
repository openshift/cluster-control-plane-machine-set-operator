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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	v1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("VSphere Provider Config", Label("vSphereProviderConfig"), func() {
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

	Context("StaticIP", func() {

		BeforeEach(func() {
			machineProviderConfig := machinev1beta1resourcebuilder.VSphereProviderSpec().
				WithZone(usCentral1a).WithIPPool().
				Build()

			providerConfig = VSphereProviderConfig{
				providerConfig: *machineProviderConfig,
				infrastructure: configv1resourcebuilder.Infrastructure().AsVSphereWithFailureDomains("vsphere-test", nil).Build(),
			}
		})

		It("contains an AddressesFromPools block", func() {
			Expect(providerConfig.providerConfig.Network.Devices[0].AddressesFromPools).To(Not(BeEmpty()),
				"expected AddressesFromPools to not be empty as a static IPPool has been configured")
		})

		It("returns networking with AddressesFromPools and the configured network name from failure domain", func() {
			expected, err := providerConfig.InjectFailureDomain(providerConfig.ExtractFailureDomain())
			Expect(err).To(Not(HaveOccurred()))
			Expect(expected.providerConfig.Network.Devices[0].AddressesFromPools).To(Not(BeNil()),
				"expected AddressesFromPools to still be present after injecting Failure Domain")
			Expect(expected.providerConfig.Network.Devices[0].NetworkName).To(Equal(providerConfig.infrastructure.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Networks[0]),
				"expected NetworkName to still be equal to the the original after injection of the Failure Domain")
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

	Context("with manual static ips", func() {
		var expectedFailureDomain v1.VSphereFailureDomain

		BeforeEach(func() {
			providerConfig.providerConfig.Network = machinev1beta1.NetworkSpec{
				Devices: []machinev1beta1.NetworkDeviceSpec{
					{
						IPAddrs: []string{
							"192.168.133.240",
						},
						Gateway: "192.168.133.1",
						Nameservers: []string{
							"8.8.8.8",
						},
						NetworkName: "test-network",
					},
				},
			}
			expectedFailureDomain = machinev1resourcebuilder.VSphereFailureDomain().
				WithZone(usCentral1a).
				Build()

		})

		It("returns the configured failure domain", func() {
			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expectedFailureDomain))
		})

		It("returns expected provider config after injection", func() {
			injectedProviderConfig, err := providerConfig.InjectFailureDomain(expectedFailureDomain)
			Expect(err).ToNot(HaveOccurred())
			Expect(injectedProviderConfig.providerConfig.Network.Devices[0].IPAddrs).To(BeNil())
			Expect(injectedProviderConfig.providerConfig.Network.Devices[0].Gateway).To(Equal(""))
			Expect(injectedProviderConfig.providerConfig.Network.Devices[0].Nameservers).To(BeNil())
		})

		It("returns expected provider config after newVSphereProviderConfig", func() {
			configBuilder := machinev1beta1resourcebuilder.VSphereProviderSpec()

			machine := machinev1beta1resourcebuilder.Machine().AsMaster().WithProviderSpecBuilder(configBuilder).Build()

			vsMachine := &machinev1beta1.VSphereMachineProviderSpec{}
			err := json.Unmarshal(machine.Spec.ProviderSpec.Value.Raw, vsMachine)
			Expect(err).ToNot(HaveOccurred())

			vsMachine.Network.Devices[0].IPAddrs = []string{"192.168.133.14"}
			vsMachine.Network.Devices[0].Gateway = "192.168.133.1"
			vsMachine.Network.Devices[0].Nameservers = []string{"8.8.8.8"}
			vsMachine.Network.Devices[0].NetworkName = "test-network"

			jsonRaw, _ := json.Marshal(vsMachine)
			machine.Spec.ProviderSpec.Value = &runtime.RawExtension{Raw: jsonRaw}

			infrastructure := configv1resourcebuilder.Infrastructure().AsVSphere("vsphere-test").Build()
			newProviderSpec, err := newVSphereProviderConfig(logger.Logger(), machine.Spec.ProviderSpec.Value, infrastructure)

			Expect(err).ToNot(HaveOccurred())
			Expect(newProviderSpec.VSphere().providerConfig.Network.Devices[0].IPAddrs).To(BeNil())
			Expect(newProviderSpec.VSphere().providerConfig.Network.Devices[0].Gateway).To(Equal(""))
			Expect(newProviderSpec.VSphere().providerConfig.Network.Devices[0].Nameservers).To(BeNil())
		})
	})

	Context("no network configured", func() {
		BeforeEach(func() {
			providerConfig.providerConfig.Network = machinev1beta1.NetworkSpec{}
		})

		It("should not fail after injectFailureDomain", func() {
			expected, err := providerConfig.InjectFailureDomain(providerConfig.ExtractFailureDomain())
			Expect(err).To(Not(HaveOccurred()))
			Expect(expected.providerConfig.Network.Devices[0].NetworkName).To(Equal(providerConfig.infrastructure.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Networks[0]),
				"expected NetworkName to still be equal to the the original after injection of the Failure Domain")
		})
	})

	Context("no vsphere platform spec in infrastructure", func() {
		BeforeEach(func() {
			providerConfig.infrastructure.Spec.PlatformSpec.VSphere = nil
		})

		It("should should return empty failure domain", func() {
			expected := providerConfig.ExtractFailureDomain()

			Expect(expected).To(Equal(v1.VSphereFailureDomain{}))
		})
	})
})
