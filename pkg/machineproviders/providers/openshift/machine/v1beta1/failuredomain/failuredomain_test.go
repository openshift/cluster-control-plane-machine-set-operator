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
		Context("with no failure domains configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				failureDomains, err = NewFailureDomains(machinev1.FailureDomains{})
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a nil list", func() {
				Expect(failureDomains).To(BeNil())
			})
		})

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

			It("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField("String()", "AWSFailureDomain{AvailabilityZone:us-east-1a, Subnet:{Type:ID, Value:subenet-us-east-1a}}"),
					HaveField("String()", "AWSFailureDomain{AvailabilityZone:us-east-1b, Subnet:{Type:ID, Value:subenet-us-east-1b}}"),
					HaveField("String()", "AWSFailureDomain{AvailabilityZone:us-east-1c, Subnet:{Type:ID, Value:subenet-us-east-1c}}"),
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

			It("returns an error", func() {
				Expect(err).To(MatchError("missing failure domain configuration"))
			})

			It("returns an empty list of failure domains", func() {
				Expect(failureDomains).To(BeEmpty())
			})
		})

		Context("With Azure failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.AzureFailureDomains().BuildFailureDomains()

				failureDomains, err = NewFailureDomains(config)
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField("String()", "AzureFailureDomain{Zone:1}"),
					HaveField("String()", "AzureFailureDomain{Zone:2}"),
					HaveField("String()", "AzureFailureDomain{Zone:3}"),
				))
			})
		})

		Context("With invalid Azure failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.AzureFailureDomains().BuildFailureDomains()
				config.Azure = nil

				failureDomains, err = NewFailureDomains(config)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("missing failure domain configuration"))
			})

			It("returns an empty list of failure domains", func() {
				Expect(failureDomains).To(BeEmpty())
			})
		})

		Context("With GCP failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.GCPFailureDomains().BuildFailureDomains()

				failureDomains, err = NewFailureDomains(config)
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField("String()", "GCPFailureDomain{Zone:us-central1-a}"),
					HaveField("String()", "GCPFailureDomain{Zone:us-central1-b}"),
					HaveField("String()", "GCPFailureDomain{Zone:us-central1-c}"),
				))
			})
		})

		Context("With invalid GCP failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := resourcebuilder.GCPFailureDomains().BuildFailureDomains()
				config.GCP = nil

				failureDomains, err = NewFailureDomains(config)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("missing failure domain configuration"))
			})

			It("returns an empty list of failure domains", func() {
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

			It("returns an empty list of failure domains", func() {
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
				Expect(fd.String()).To(Equal("AWSFailureDomain{AvailabilityZone:us-east-1a}"))
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
					Expect(fd.String()).To(Equal("AWSFailureDomain{Subnet:{Type:ARN, Value:subnet-us-east-1a}}"))
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
					Expect(fd.String()).To(Equal("AWSFailureDomain{Subnet:{Type:Filters, Value:&[{Name:tag:Name Values:[subnet-us-east-1b]}]}}"))
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
					Expect(fd.String()).To(Equal("AWSFailureDomain{Subnet:{Type:ID, Value:subnet-us-east-1c}}"))
				})
			})
		})
	})

	Context("an Azure failure domain", func() {
		var fd failureDomain

		BeforeEach(func() {
			fd = failureDomain{
				platformType: configv1.AzurePlatformType,
			}
		})

		Context("with an availability zone", func() {
			BeforeEach(func() {
				fd.azure = resourcebuilder.AzureFailureDomain().WithZone("1").Build()
			})

			It("returns the availability zone for String()", func() {
				Expect(fd.String()).To(Equal("AzureFailureDomain{Zone:1}"))
			})
		})

		Context("with no availability zone", func() {
			BeforeEach(func() {
				fd.azure = resourcebuilder.AzureFailureDomain().Build()
				fd.azure.Zone = ""
			})

			It("returns <unknown> for String()", func() {
				Expect(fd.String()).To(Equal("<unknown>"))
			})
		})
	})

	Context("Equal", func() {
		var fd1 failureDomain
		var fd2 failureDomain

		Context("With two identical AWS failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
			})

			It("returns true", func() {
				Expect(fd1.Equal(fd2)).To(BeTrue())
			})
		})

		Context("With nil failure domain", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(nil)).To(BeFalse())
			})
		})

		Context("With two identical Azure failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
			})

			It("returns true", func() {
				Expect(fd1.Equal(fd2)).To(BeTrue())
			})
		})

		Context("With two different Azure failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        resourcebuilder.AzureFailureDomain().WithZone("2").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(fd2)).To(BeFalse())
			})
		})

		Context("With two identical GCP failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
			})

			It("returns true", func() {
				Expect(fd1.Equal(fd2)).To(BeTrue())
			})
		})

		Context("With two different Azure failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          resourcebuilder.GCPFailureDomain().WithZone("us-central1-b").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(fd2)).To(BeFalse())
			})
		})

		Context("With different failure domains platform", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(fd2)).To(BeFalse())
			})
		})

	})

})
