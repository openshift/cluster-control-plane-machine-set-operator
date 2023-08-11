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

package failuredomain

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
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
				config := machinev1resourcebuilder.AWSFailureDomains().BuildFailureDomains()

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
				config := machinev1resourcebuilder.AWSFailureDomains().BuildFailureDomains()
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
				config := machinev1resourcebuilder.AzureFailureDomains().BuildFailureDomains()

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
				config := machinev1resourcebuilder.AzureFailureDomains().BuildFailureDomains()
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
				config := machinev1resourcebuilder.GCPFailureDomains().BuildFailureDomains()

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
				config := machinev1resourcebuilder.GCPFailureDomains().BuildFailureDomains()
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

		Context("With OpenStack failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := machinev1resourcebuilder.OpenStackFailureDomains().BuildFailureDomains()

				failureDomains, err = NewFailureDomains(config)
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az0, RootVolume:{AvailabilityZone:cinder-az0, VolumeType:fast-az0}}"),
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az1, RootVolume:{AvailabilityZone:cinder-az1, VolumeType:fast-az1}}"),
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az2, RootVolume:{AvailabilityZone:cinder-az2, VolumeType:fast-az2}}"),
				))
			})
		})

		Context("With OpenStack failure domain configuration with volumeType", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := machinev1resourcebuilder.OpenStackFailureDomains().WithFailureDomainBuilders(
					machinev1resourcebuilder.OpenStackFailureDomainBuilder{AvailabilityZone: "nova-az0", RootVolume: &machinev1.RootVolume{VolumeType: "volume.hostA"}},
					machinev1resourcebuilder.OpenStackFailureDomainBuilder{AvailabilityZone: "nova-az1", RootVolume: &machinev1.RootVolume{VolumeType: "volume.hostB"}},
					machinev1resourcebuilder.OpenStackFailureDomainBuilder{AvailabilityZone: "nova-az2", RootVolume: &machinev1.RootVolume{VolumeType: "volume.hostC"}},
				).BuildFailureDomains()

				failureDomains, err = NewFailureDomains(config)
			})

			It("should not error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should construct a list of failure domains", func() {
				Expect(failureDomains).To(ConsistOf(
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az0, RootVolume:{VolumeType:volume.hostA}}"),
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az1, RootVolume:{VolumeType:volume.hostB}}"),
					HaveField("String()", "OpenStackFailureDomain{AvailabilityZone:nova-az2, RootVolume:{VolumeType:volume.hostC}}"),
				))
			})
		})

		Context("With invalid OpenStack failure domain configuration", func() {
			var failureDomains []FailureDomain
			var err error

			BeforeEach(func() {
				config := machinev1resourcebuilder.OpenStackFailureDomains().BuildFailureDomains()
				config.OpenStack = nil

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
				fd.aws = machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build()
			})

			It("returns the availability zone for String()", func() {
				Expect(fd.String()).To(Equal("AWSFailureDomain{AvailabilityZone:us-east-1a}"))
			})
		})

		Context("with no availability zone", func() {
			Context("with an ARN type subnet", func() {
				BeforeEach(func() {
					subnetARN := "subnet-us-east-1a"

					fd.aws = machinev1resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
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
					fd.aws = machinev1resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
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

					fd.aws = machinev1resourcebuilder.AWSFailureDomain().WithSubnet(machinev1.AWSResourceReference{
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
				fd.azure = machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build()
			})

			It("returns the availability zone for String()", func() {
				Expect(fd.String()).To(Equal("AzureFailureDomain{Zone:1}"))
			})
		})

		Context("with no availability zone", func() {
			BeforeEach(func() {
				fd.azure = machinev1resourcebuilder.AzureFailureDomain().Build()
				fd.azure.Zone = ""
			})

			It("returns <unknown> for String()", func() {
				Expect(fd.String()).To(Equal("<unknown>"))
			})
		})
	})

	Context("an OpenStack failure domain", func() {
		var fd failureDomain
		var filterRootVolume = machinev1.RootVolume{
			AvailabilityZone: "cinder-az0",
			VolumeType:       "fast-az0",
		}

		BeforeEach(func() {
			fd = failureDomain{
				platformType: configv1.OpenStackPlatformType,
			}
		})

		Context("with a Compute and Storage availability zone", func() {
			BeforeEach(func() {
				fd.openstack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
					WithRootVolume(&filterRootVolume).Build()
			})

			It("returns the Compute and Storage availability zones for String()", func() {
				Expect(fd.String()).To(Equal("OpenStackFailureDomain{AvailabilityZone:nova-az0, RootVolume:{AvailabilityZone:cinder-az0, VolumeType:fast-az0}}"))
			})
		})

		Context("with a Compute availability zone only", func() {
			BeforeEach(func() {
				fd.openstack = machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").Build()
			})

			It("returns the Compute availability zone for String()", func() {
				Expect(fd.String()).To(Equal("OpenStackFailureDomain{AvailabilityZone:nova-az0}"))
			})
		})
		Context("with a Storage availability zone only", func() {
			BeforeEach(func() {
				fd.openstack = machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&filterRootVolume).Build()
			})

			It("returns the Storage availability zone for String()", func() {
				Expect(fd.String()).To(Equal("OpenStackFailureDomain{RootVolume:{AvailabilityZone:cinder-az0, VolumeType:fast-az0}}"))
			})
		})
		Context("with no availability zones", func() {
			BeforeEach(func() {
				fd.openstack = machinev1resourcebuilder.OpenStackFailureDomain().Build()
			})

			It("returns <unknown> for String()", func() {
				Expect(fd.String()).To(Equal("<unknown>"))
			})
		})
	})

	Context("Equal", func() {
		var fd1 failureDomain
		var fd2 failureDomain
		var filterRootVolume = machinev1.RootVolume{
			AvailabilityZone: "cinder-az0",
			VolumeType:       "fast-az0",
		}

		Context("With two identical AWS failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AWSPlatformType,
					aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
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
					aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
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
					azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
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
					azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("2").Build(),
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
					gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
			})

			It("returns true", func() {
				Expect(fd1.Equal(fd2)).To(BeTrue())
			})
		})

		Context("With two identical OpenStack failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.OpenStackPlatformType,
					openstack:    machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&filterRootVolume).Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.OpenStackPlatformType,
					openstack:    machinev1resourcebuilder.OpenStackFailureDomain().WithRootVolume(&filterRootVolume).Build(),
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
					gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.GCPPlatformType,
					gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("us-central1-b").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(fd2)).To(BeFalse())
			})
		})

		Context("With two different OpenStack failure domains", func() {
			BeforeEach(func() {
				fd1 = failureDomain{
					platformType: configv1.OpenStackPlatformType,
					openstack:    machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.GCPPlatformType,
					openstack:    machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az1").Build(),
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
					aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-east-1a").Build(),
				}
				fd2 = failureDomain{
					platformType: configv1.AzurePlatformType,
					azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
				}
			})

			It("returns false", func() {
				Expect(fd1.Equal(fd2)).To(BeFalse())
			})
		})

	})

	Context("Complete", func() {
		type testCase struct {
			failureDomain         FailureDomain
			templateFailureDomain FailureDomain
			expectedResult        FailureDomain
			expectedError         error
		}

		DescribeTable("can combine failure domain with template failure domain",
			func(tc testCase) {
				result, err := tc.failureDomain.Complete(tc.templateFailureDomain)
				if tc.expectedError != nil {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(tc.expectedError))

					return
				}

				Expect(err).ToNot(HaveOccurred())
				Expect(result.String()).To(Equal(tc.expectedResult.String()))
				Expect(result.Equal(tc.expectedResult)).To(BeTrue())
			},
			Entry("AWS failure domain",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type: machinev1.AWSARNReferenceType,
							ARN:  pointer.String("arn-subnet-1"),
						}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type: machinev1.AWSARNReferenceType,
							ARN:  pointer.String("arn-subnet-1"),
						}).Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type: machinev1.AWSARNReferenceType,
							ARN:  pointer.String("arn-subnet-1"),
						}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("AWS failure domain with empty template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type: machinev1.AWSARNReferenceType,
							ARN:  pointer.String("arn-subnet-1"),
						}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws:          machinev1resourcebuilder.AWSFailureDomain().Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type: machinev1.AWSARNReferenceType,
							ARN:  pointer.String("arn-subnet-1"),
						}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("AWS failure domain with different zone, subnet from template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1c").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1a").WithSubnet(machinev1.AWSResourceReference{
							Type:    machinev1.AWSFiltersReferenceType,
							Filters: &[]machinev1.AWSResourceFilter{{Name: "tag:Name", Values: []string{"subnet-1"}}},
						}).Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws: machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("us-west-1c").WithSubnet(machinev1.AWSResourceReference{
							Type:    machinev1.AWSFiltersReferenceType,
							Filters: &[]machinev1.AWSResourceFilter{{Name: "tag:Name", Values: []string{"subnet-1"}}},
						}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("Azure failure domain with subnet from failuredomain",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").WithSubnet("subnet-1").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").WithSubnet("subnet-1").Build(),
					},
					expectedError: nil,
				},
			),
			Entry("Azure failure domain with empty template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").WithSubnet("subnet-1").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").WithSubnet("subnet-1").Build(),
					},
					expectedError: nil,
				},
			),
			Entry("Azure failure domain with different zone, subnet from template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("2").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("1").WithSubnet("subnet-1").Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.AzurePlatformType,
						azure:        machinev1resourcebuilder.AzureFailureDomain().WithZone("2").WithSubnet("subnet-1").Build(),
					},
					expectedError: nil,
				},
			),
			Entry("GCP failure domain",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					expectedError: nil,
				},
			),
			Entry("GCP failure domain with empty template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					expectedError: nil,
				},
			),
			Entry("GCP failure domain with different zone",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("2").Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("1").Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.GCPPlatformType,
						gcp:          machinev1resourcebuilder.GCPFailureDomain().WithZone("2").Build(),
					},
				},
			),
			Entry("OpenStack failure domain",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("OpenStack failure domain with empty template",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack:    machinev1resourcebuilder.OpenStackFailureDomain().Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("OpenStack failure domain with different volume type",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "different-az2",
							}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "fast-az2",
							}).Build(),
					},
					expectedResult: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "different-az2",
							}).Build(),
					},
					expectedError: nil,
				},
			),
			Entry("Nil template failure domain",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("1").Build(),
					},
					templateFailureDomain: nil,
					expectedResult: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("1").Build(),
					},
					expectedError: errMissingTemplateFailureDomain,
				},
			),
			Entry("Mismatched platform types",
				testCase{
					failureDomain: failureDomain{
						platformType: configv1.OpenStackPlatformType,
						openstack: machinev1resourcebuilder.OpenStackFailureDomain().WithComputeAvailabilityZone("nova-az0").
							WithRootVolume(&machinev1.RootVolume{
								AvailabilityZone: "cinder-az2",
								VolumeType:       "different-az2",
							}).Build(),
					},
					templateFailureDomain: failureDomain{
						platformType: configv1.AWSPlatformType,
						aws:          machinev1resourcebuilder.AWSFailureDomain().WithAvailabilityZone("1").Build(),
					},
					expectedResult: nil,
					expectedError:  errMismatchedPlatformType,
				},
			),
			Entry("Generic failure domain",
				testCase{
					failureDomain:         NewGenericFailureDomain(),
					templateFailureDomain: NewGenericFailureDomain(),
					expectedResult:        NewGenericFailureDomain(),
					expectedError:         nil,
				},
			),
		)
	})
})
