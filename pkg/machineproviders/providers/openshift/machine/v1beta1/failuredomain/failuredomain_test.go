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

package failuredomain

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("FailureDomains", func() {
	Context("NewFailureDomains", func() {
		Context("With AWS failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.AWSFailureDomains().BuildFailureDomains()

				failureDomains, err = NewFailureDomains(config)
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			PIt("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField(".String()", "us-east-1a"),
					HaveField(".String()", "us-east-1b"),
					HaveField(".String()", "us-east-1c"),
				))
			})
		})

		Context("With invalid AWS failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.AWSFailureDomains().BuildFailureDomains()
				config.AWS = nil

				failureDomains, err = NewFailureDomains(config)
			})

			PIt("returns an error", func() {
				Expect(err).To(MatchError("missing configuration for AWS failure domains"))
			})

			PIt("returns an empty list of failure domains", func() {
				Expect(failureDomains).To(BeEmpty())
			})
		})

		Context("With an unsupported platform type", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := machinev1.FailureDomains{
					Platform: configv1.BareMetalPlatformType,
				}

				failureDomains, err = NewFailureDomains(config)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("unsupported platform type: BareMetal"))
			})

			PIt("returns an empty list of failure domains", func() {
				Expect(failureDomains).To(BeEmpty())
			})
		})
	})

	Context("an AWS failure domain", func() {
		var fd failureDomain

		BeforeEach(func() {
			fd = failureDomain{
				platformType: configv1.AWSPlatformType,
			}
		})

		Context("with an availability zone", func() {
			BeforeEach(func() {
				fd.aws = resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build()
			})

			It("returns the availability zone for String()", func() {
				Expect(fd.String()).To(Equal("us-east-1a"))
			})
		})

		Context("with no availability zone", func() {
			Context("with an ARN type subnet", func() {
				BeforeEach(func() {
					subnetARN := "subnet-us-east-1a"

					fd.aws = resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
						Type: machinev1.AWSARNReferenceType,
						ARN:  &subnetARN,
					}).Build()
				})

				It("returns the subnet for String()", func() {
					Expect(fd.String()).To(Equal("subnet-us-east-1a"))
				})
			})

			Context("with a filter type subnet", func() {
				BeforeEach(func() {
					fd.aws = resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
						Type: machinev1.AWSFiltersReferenceType,
						Filters: &[]machinev1.AWSResourceFilter{
							{
								Name:   "tag:Name",
								Values: []string{"subnet-us-east-1b"},
							},
						},
					}).Build()
				})

				It("returns the subnet for String()", func() {
					Expect(fd.String()).To(Equal("[{Name:tag:Name Values:[subnet-us-east-1b]}]"))
				})
			})

			Context("with an ID type subnet", func() {
				BeforeEach(func() {
					subnetID := "subnet-us-east-1c"

					fd.aws = resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
						Type: machinev1.AWSIDReferenceType,
						ID:   &subnetID,
					}).Build()
				})

				It("returns the subnet for String()", func() {
					Expect(fd.String()).To(Equal("subnet-us-east-1c"))
				})
			})
		})
	})
})
