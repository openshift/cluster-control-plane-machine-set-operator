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
)

var _ = Describe("Set suite", func() {
	Context("when creating a new set", func() {
		Context("with AWS failure domains", func() {
			usEast1aFailureDomain := &failureDomain{
				platformType: configv1.AWSPlatformType,
				aws: machinev1.AWSFailureDomain{
					Placement: machinev1.AWSFailureDomainPlacement{
						AvailabilityZone: "us-east-1a",
					},
				},
			}

			usEast1bFailureDomain := &failureDomain{
				platformType: configv1.AWSPlatformType,
				aws: machinev1.AWSFailureDomain{
					Placement: machinev1.AWSFailureDomainPlacement{
						AvailabilityZone: "us-east-1b",
					},
				},
			}

			usEast1cFailureDomain := &failureDomain{
				platformType: configv1.AWSPlatformType,
				aws: machinev1.AWSFailureDomain{
					Placement: machinev1.AWSFailureDomainPlacement{
						AvailabilityZone: "us-east-1c",
					},
				},
			}

			usEast1dFailureDomain := &failureDomain{
				platformType: configv1.AWSPlatformType,
				aws: machinev1.AWSFailureDomain{
					Placement: machinev1.AWSFailureDomainPlacement{
						AvailabilityZone: "us-east-1d",
					},
				},
			}

			var set *Set

			BeforeEach(func() {
				By("creating a new set with 2 failure domains")
				set = NewSet(usEast1aFailureDomain, usEast1cFailureDomain)
			})

			It("should have the initial failure domains", func() {
				Expect(set.Has(usEast1aFailureDomain)).To(BeTrue(), "Set should contain the us-east-1a failure domain")
				Expect(set.Has(usEast1cFailureDomain)).To(BeTrue(), "Set should contain the us-east-1c failure domain")
			})

			It("should not have other failure domains", func() {
				Expect(set.Has(usEast1bFailureDomain)).ToNot(BeTrue(), "Set not should contain the us-east-1b failure domain")
				Expect(set.Has(usEast1dFailureDomain)).ToNot(BeTrue(), "Set not should contain the us-east-1d failure domain")
			})

			It("should return a sorted list of the failure domains", func() {
				Expect(set.List()).To(Equal([]FailureDomain{
					usEast1aFailureDomain,
					usEast1cFailureDomain,
				}))
			})

			Context("when adding additional failure domains", func() {
				Context("that are not already in the set", func() {
					BeforeEach(func() {
						set.Insert(usEast1bFailureDomain)
						set.Insert(usEast1dFailureDomain)
					})

					It("should have the initial failure domains and the new failure domains", func() {
						Expect(set.Has(usEast1aFailureDomain)).To(BeTrue(), "Set should contain the us-east-1a failure domain")
						Expect(set.Has(usEast1bFailureDomain)).To(BeTrue(), "Set should contain the us-east-1b failure domain")
						Expect(set.Has(usEast1cFailureDomain)).To(BeTrue(), "Set should contain the us-east-1c failure domain")
						Expect(set.Has(usEast1dFailureDomain)).To(BeTrue(), "Set should contain the us-east-1d failure domain")
					})

					It("should return a sorted list of the failure domains without duplicates", func() {
						Expect(set.List()).To(Equal([]FailureDomain{
							usEast1aFailureDomain,
							usEast1bFailureDomain,
							usEast1cFailureDomain,
							usEast1dFailureDomain,
						}))
					})
				})

				Context("that are already in the set", func() {
					BeforeEach(func() {
						set.Insert(usEast1aFailureDomain)
						set.Insert(usEast1cFailureDomain)
					})

					It("should still have the initial failure domains", func() {
						Expect(set.Has(usEast1aFailureDomain)).To(BeTrue(), "Set should contain the us-east-1a failure domain")
						Expect(set.Has(usEast1cFailureDomain)).To(BeTrue(), "Set should contain the us-east-1c failure domain")
					})

					It("should return a sorted list of the failure domains without duplicates", func() {
						Expect(set.List()).To(Equal([]FailureDomain{
							usEast1aFailureDomain,
							usEast1cFailureDomain,
						}))
					})
				})
			})
		})
	})
})
