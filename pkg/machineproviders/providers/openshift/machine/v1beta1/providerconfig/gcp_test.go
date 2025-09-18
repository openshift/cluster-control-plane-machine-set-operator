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
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("GCP Provider Config", func() {
	var logger testutils.TestLogger

	var providerConfig GCPProviderConfig

	usCentral1a := "us-central1-a"
	usCentral1b := "us-central1-b"

	BeforeEach(func() {
		machineProviderConfig := machinev1beta1resourcebuilder.GCPProviderSpec().
			WithZone(usCentral1a).
			Build()

		providerConfig = GCPProviderConfig{
			providerConfig: *machineProviderConfig,
		}
	})

	Context("ExtractFailureDomain", func() {
		It("returns the configured failure domain", func() {
			expected := machinev1resourcebuilder.GCPFailureDomain().
				WithZone(usCentral1a).
				Build()

			Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
		})

		logger = testutils.NewTestLogger()
	})

	Context("when the failuredomain is changed after initialisation", func() {
		var changedProviderConfig GCPProviderConfig

		BeforeEach(func() {
			changedFailureDomain := machinev1resourcebuilder.GCPFailureDomain().
				WithZone(usCentral1b).
				Build()

			changedProviderConfig = providerConfig.InjectFailureDomain(changedFailureDomain)
		})

		Context("ExtractFailureDomain", func() {
			It("returns the changed failure domain from the changed config", func() {
				expected := machinev1resourcebuilder.GCPFailureDomain().
					WithZone(usCentral1b).
					Build()

				Expect(changedProviderConfig.ExtractFailureDomain()).To(Equal(expected))
			})

			It("returns the original failure domain from the original config", func() {
				expected := machinev1resourcebuilder.GCPFailureDomain().
					WithZone(usCentral1a).
					Build()

				Expect(providerConfig.ExtractFailureDomain()).To(Equal(expected))
			})
		})
	})

	Context("newGCPProviderConfig", func() {
		var providerConfig ProviderConfig
		var expectedGCPConfig machinev1beta1.GCPMachineProviderSpec

		BeforeEach(func() {
			configBuilder := machinev1beta1resourcebuilder.GCPProviderSpec()
			expectedGCPConfig = *configBuilder.Build()
			rawConfig := configBuilder.BuildRawExtension()

			var err error
			providerConfig, err = newGCPProviderConfig(logger.Logger(), rawConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the type to GCP", func() {
			Expect(providerConfig.Type()).To(Equal(configv1.GCPPlatformType))
		})

		It("returns the correct GCP config", func() {
			Expect(providerConfig.GCP()).ToNot(BeNil())
			Expect(providerConfig.GCP().Config()).To(Equal(expectedGCPConfig))
		})
	})

	Context("Diff", func() {
		type diffTableInput struct {
			baseConfig    GCPProviderConfig
			compareConfig machinev1beta1.GCPMachineProviderSpec
			expectedDiff  []string
		}

		DescribeTable("should return correct diff", func(in diffTableInput) {
			diff := in.baseConfig.Diff(in.compareConfig)

			Expect(diff).To(Equal(in.expectedDiff))
		},
			Entry("with different GCP disks.images values", diffTableInput{
				baseConfig: GCPProviderConfig{
					providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
						{
							AutoDelete: true,
							Boot:       true,
							SizeGB:     100,
							Type:       "pd-standard",
							Image:      "projects/rhcos-cloud/global/images/rhcos-416-92-202301311551-0-gcp-x86-64",
						},
					}).Build(),
				},
				compareConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
					{
						AutoDelete: true,
						Boot:       true,
						SizeGB:     100,
						Type:       "pd-standard",
						Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
					},
				}).Build(),
				// expectedDiff is nil because disk Image fields are ignored in diff comparison
				expectedDiff: nil,
			}),

			Entry("with different GCP disks.SizeGB values", diffTableInput{
				baseConfig: GCPProviderConfig{
					providerConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
						{
							AutoDelete: true,
							Boot:       true,
							SizeGB:     100,
							Type:       "pd-standard",
							Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
						},
					}).Build(),
				},
				compareConfig: *machinev1beta1resourcebuilder.GCPProviderSpec().WithDisks([]*machinev1beta1.GCPDisk{
					{
						AutoDelete: true,
						Boot:       true,
						SizeGB:     200,
						Type:       "pd-standard",
						Image:      "projects/rhcos-cloud/global/images/rhcos-417-92-202302090245-0-gcp-x86-64",
					},
				}).Build(),
				expectedDiff: []string{"Disks.slice[0].SizeGB: 100 != 200"},
			}),
		)
	})
})
