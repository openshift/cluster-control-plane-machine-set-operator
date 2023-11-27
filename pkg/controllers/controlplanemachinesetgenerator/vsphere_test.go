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

package controlplanemachinesetgenerator

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func deserializeVsphereProviderSpec(logger logr.Logger, raw *runtime.RawExtension, platformProviderSpec interface{}) error {
	if err := yaml.UnmarshalStrict(raw.Raw, platformProviderSpec); err != nil {
		logger.Error(err, "failed to strictly unmarshal provider config due to unknown field")

		if err := json.Unmarshal(raw.Raw, platformProviderSpec); err != nil {
			return fmt.Errorf("failed to unmarshal provider config: %w", err)
		}
	}

	return nil
}

var _ = Describe("VSphere control plane machineset", func() {
	logger := testutils.NewTestLogger()

	Context("when failure domains are defined", func() {
		infrastructure := configv1resourcebuilder.Infrastructure().AsVSphereWithFailureDomains("vsphere-test", nil).Build()
		It("preserves IPPool", func() {
			providerSpecBuilder := machinev1beta1resourcebuilder.VSphereProviderSpec().WithIPPool()
			machines := []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(providerSpecBuilder).Build(),
			}

			ippool := providerSpecBuilder.Build().Network.Devices[0].AddressesFromPools[0]

			applyConfig, err := buildControlPlaneMachineSetVSphereMachineSpec(logger.Logger(), machines, infrastructure)
			Expect(err).ToNot(HaveOccurred())

			vsphereMachineProviderSpec := &machinev1beta1.VSphereMachineProviderSpec{}

			err = deserializeVsphereProviderSpec(logger.Logger(), applyConfig.ProviderSpec.Value, vsphereMachineProviderSpec)

			Expect(err).ToNot(HaveOccurred())
			Expect(vsphereMachineProviderSpec.Network.Devices[0].AddressesFromPools[0]).To(Equal(ippool))
		})

		It("workspace, network, and template should not be defined", func() {
			providerSpecBuilder := machinev1beta1resourcebuilder.VSphereProviderSpec()
			machines := []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(providerSpecBuilder).Build(),
			}
			applyConfig, err := buildControlPlaneMachineSetVSphereMachineSpec(logger.Logger(), machines, infrastructure)
			Expect(err).ToNot(HaveOccurred())

			vsphereMachineProviderSpec := &machinev1beta1.VSphereMachineProviderSpec{}

			err = deserializeVsphereProviderSpec(logger.Logger(), applyConfig.ProviderSpec.Value, vsphereMachineProviderSpec)

			Expect(err).ToNot(HaveOccurred())
			emptyWorkspace := &machinev1beta1.Workspace{}
			Expect(vsphereMachineProviderSpec.Network.Devices).To(BeEmpty())
			Expect(vsphereMachineProviderSpec.Template).To(BeEmpty())
			Expect(vsphereMachineProviderSpec.Workspace).To(Equal(emptyWorkspace))
		})
	})

	Context("when no failure domains are defined", func() {
		infrastructure := configv1resourcebuilder.Infrastructure().AsVSphere("vsphere-test").Build()
		providerSpecBuilder := machinev1beta1resourcebuilder.VSphereProviderSpec().WithIPPool()
		machines := []machinev1beta1.Machine{
			*machinev1beta1resourcebuilder.Machine().WithProviderSpecBuilder(providerSpecBuilder).Build(),
		}

		networkDevice := providerSpecBuilder.Build().Network.Devices[0]
		It("preserves network device", func() {
			applyConfig, err := buildControlPlaneMachineSetVSphereMachineSpec(logger.Logger(), machines, infrastructure)
			Expect(err).ToNot(HaveOccurred())

			vsphereMachineProviderSpec := &machinev1beta1.VSphereMachineProviderSpec{}

			err = deserializeVsphereProviderSpec(logger.Logger(), applyConfig.ProviderSpec.Value, vsphereMachineProviderSpec)

			Expect(err).ToNot(HaveOccurred())
			Expect(vsphereMachineProviderSpec.Network.Devices[0]).To(Equal(networkDevice))
		})
		It("workspace, network, and template should be defined", func() {
			applyConfig, err := buildControlPlaneMachineSetVSphereMachineSpec(logger.Logger(), machines, infrastructure)
			Expect(err).ToNot(HaveOccurred())

			vsphereMachineProviderSpec := &machinev1beta1.VSphereMachineProviderSpec{}

			err = deserializeVsphereProviderSpec(logger.Logger(), applyConfig.ProviderSpec.Value, vsphereMachineProviderSpec)

			Expect(err).ToNot(HaveOccurred())
			emptyWorkspace := &machinev1beta1.Workspace{}
			Expect(vsphereMachineProviderSpec.Network.Devices).ToNot(BeEmpty())
			Expect(vsphereMachineProviderSpec.Template).ToNot(BeEmpty())
			Expect(vsphereMachineProviderSpec.Workspace).ToNot(Equal(emptyWorkspace))
		})
	})

})
