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

package framework

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Framwork", func() {
	Context("IncreaseProviderSpecInstanceSize", func() {
		Context("on AWS", func() {
			Context("nextAWSInstanceSize", func() {
				type nextInstanceSizeTableInput struct {
					currentInstanceSize string
					expectedNextSize    string
					expectedError       error
				}

				DescribeTable("should return the next instance size", func(in nextInstanceSizeTableInput) {
					nextInstanceSize, err := nextAWSInstanceSize(in.currentInstanceSize)
					if in.expectedError != nil {
						Expect(err).To(MatchError(in.expectedError))
					} else {
						Expect(err).ToNot(HaveOccurred())
					}

					Expect(nextInstanceSize).To(Equal(in.expectedNextSize))
				},
					Entry("when the current instance size is m5.xlarge", nextInstanceSizeTableInput{
						currentInstanceSize: "m5.xlarge",
						expectedNextSize:    "m5.2xlarge",
					}),
					Entry("when the current instance size is m6i.large", nextInstanceSizeTableInput{
						currentInstanceSize: "m6i.large",
						expectedNextSize:    "m6i.xlarge",
					}),
					Entry("when the current instance size is r4.4xlarge", nextInstanceSizeTableInput{
						currentInstanceSize: "r4.4xlarge",
						expectedNextSize:    "r4.8xlarge",
					}),
					Entry("when the current instance size is t3.micro (unsupported)", nextInstanceSizeTableInput{
						currentInstanceSize: "t3.micro",
						expectedNextSize:    "",
						expectedError:       fmt.Errorf("%w: t3.micro", errInstanceTypeNotSupported),
					}),
					Entry("when the current instance size is not a valid format", nextInstanceSizeTableInput{
						currentInstanceSize: "m6a4xlarge",
						expectedNextSize:    "",
						expectedError:       fmt.Errorf("%w: m6a4xlarge", errInstanceTypeUnsupportedFormat),
					}),
				)
			})
		})

		Context("on Azure", func() {
			Context("nextAzureVMSize", func() {
				type nextInstanceSizeTableInput struct {
					currentVMSize    string
					expectedNextSize string
					expectedError    error
				}

				DescribeTable("should return the next VM size", func(in nextInstanceSizeTableInput) {
					nextInstanceSize, err := nextAzureVMSize(in.currentVMSize)
					if in.expectedError != nil {
						Expect(err).To(MatchError(in.expectedError))
					} else {
						Expect(err).ToNot(HaveOccurred())
					}

					Expect(nextInstanceSize).To(Equal(in.expectedNextSize))
				},
					Entry("when the current VM size is Standard_D2as_v5", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D2as_v5",
						expectedNextSize: "Standard_D4as_v5",
					}),
					Entry("when the current VM size is Standard_B4ms", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_B4ms",
						expectedNextSize: "Standard_B8ms",
					}),
					Entry("when the current VM size is Standard_D8s_v4", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D8s_v4",
						expectedNextSize: "Standard_D16s_v4",
					}),
					Entry("when the current VM size is Standard_D16a_v4", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D16a_v4",
						expectedNextSize: "Standard_D32a_v4",
					}),
					Entry("when the current VM size is Standard_D32s_v3", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D32s_v3",
						expectedNextSize: "Standard_D48s_v3",
					}),
					Entry("when the current VM size is Standard_D48s_v3", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D48s_v3",
						expectedNextSize: "Standard_D64s_v3",
					}),
					Entry("when the current VM size is Standard_D64s_v3", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D64s_v3",
						expectedNextSize: "",
						expectedError:    fmt.Errorf("%w: Standard_D64s_v3", errInstanceTypeNotSupported),
					}),
					Entry("when the current VM size is Standard_D96s_v3", nextInstanceSizeTableInput{
						currentVMSize:    "Standard_D96s_v3",
						expectedNextSize: "",
						expectedError:    fmt.Errorf("%w: Standard_D96s_v3", errInstanceTypeNotSupported),
					}),
				)
			})
		})

		Context("On GCP", func() {
			Context("NextGCPMachineSize", func() {
				type nextInstanceSizeTableInput struct {
					currentMachineSize string
					expectedNextSize   string
					expectedError      error
				}

				DescribeTable("should return the next VM size", func(in nextInstanceSizeTableInput) {
					nextInstanceSize, err := nextGCPMachineSize(in.currentMachineSize)
					if in.expectedError != nil {
						Expect(err).To(MatchError(in.expectedError))
					} else {
						Expect(err).ToNot(HaveOccurred())
					}

					Expect(nextInstanceSize).To(Equal(in.expectedNextSize))
				},
					Entry("when the current Machine size is n1-standard-1", nextInstanceSizeTableInput{
						currentMachineSize: "n1-standard-1",
						expectedNextSize:   "n1-standard-2",
					}),
					Entry("when the current Machine size is n1-standard-2", nextInstanceSizeTableInput{
						currentMachineSize: "n1-standard-2",
						expectedNextSize:   "n1-standard-4",
					}),
					Entry("when the current Machine size is n1-standard-32", nextInstanceSizeTableInput{
						currentMachineSize: "n1-standard-32",
						expectedNextSize:   "n1-standard-64",
					}),
					Entry("when the current Machine size is n1-standard-64", nextInstanceSizeTableInput{
						currentMachineSize: "n1-standard-64",
						expectedNextSize:   "n1-standard-96",
					}),
					Entry("when the current Machine size is n1-standard-96", nextInstanceSizeTableInput{
						currentMachineSize: "n1-standard-96",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: n1-standard-96", errInstanceTypeNotSupported),
					}),
					Entry("when the current Machine size is n2-standard-64", nextInstanceSizeTableInput{
						currentMachineSize: "n2-standard-64",
						expectedNextSize:   "n2-standard-80",
					}),
					Entry("when the current Machine size is n2-standard-96", nextInstanceSizeTableInput{
						currentMachineSize: "n2-standard-96",
						expectedNextSize:   "n2-standard-128",
					}),
					Entry("when the current Machine size is n2-standard-128", nextInstanceSizeTableInput{
						currentMachineSize: "n2-standard-128",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: n2-standard-128", errInstanceTypeNotSupported),
					}),
					Entry("when the current Machine size is e2-standard-2", nextInstanceSizeTableInput{
						currentMachineSize: "e2-standard-2",
						expectedNextSize:   "e2-standard-4",
					}),
					Entry("when the current Machine size is e2-standard-4", nextInstanceSizeTableInput{
						currentMachineSize: "e2-standard-4",
						expectedNextSize:   "e2-standard-8",
					}),
					Entry("when the current Machine size is e2-standard-8", nextInstanceSizeTableInput{
						currentMachineSize: "e2-standard-8",
						expectedNextSize:   "e2-standard-16",
					}),
					Entry("when the current Machine size is e2-standard-16", nextInstanceSizeTableInput{
						currentMachineSize: "e2-standard-16",
						expectedNextSize:   "e2-standard-32",
					}),
					Entry("when the current Machine size is e2-standard-32", nextInstanceSizeTableInput{
						currentMachineSize: "e2-standard-32",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-standard-32", errInstanceTypeNotSupported),
					}),
					Entry("when the current Machine size is n1-custom-16-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n1-custom-16-1024",
						expectedNextSize:   "n1-custom-18-55296",
					}),
					Entry("when the current Machine size is n1-custom-64-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n1-custom-64-1024",
						expectedNextSize:   "n1-custom-64-196608",
					}),
					Entry("when the current Machine size is n2-custom-16-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2-custom-16-1024",
						expectedNextSize:   "n2-custom-18-55296",
					}),
					Entry("when the current Machine size is n2-custom-32-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2-custom-32-1024",
						expectedNextSize:   "n2-custom-36-110592",
					}),
					Entry("when the current Machine size is n2d-custom-2-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2d-custom-2-1024",
						expectedNextSize:   "n2d-custom-4-12288",
					}),
					Entry("when the current Machine size is n2d-custom-4-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2d-custom-4-1024",
						expectedNextSize:   "n2d-custom-8-24576",
					}),
					Entry("when the current Machine size is n2d-custom-8-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2d-custom-8-1024",
						expectedNextSize:   "n2d-custom-16-49152",
					}),
					Entry("when the current Machine size is n2d-custom-16-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2d-custom-16-1024",
						expectedNextSize:   "n2d-custom-32-98304",
					}),
					Entry("when the current Machine size is n2d-custom-32-1024", nextInstanceSizeTableInput{
						currentMachineSize: "n2d-custom-32-1024",
						expectedNextSize:   "n2d-custom-48-147456",
					}),
					Entry("when the current Machine size is e2-custom-16", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-16",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-custom-16", errInstanceTypeNotSupported),
					}),
					Entry("when the current Machine size is e2-custom-2-1024", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-2-1024",
						expectedNextSize:   "e2-custom-4-12288",
					}),
					Entry("when the current Machine size is e2-custom-micro-0.25-1024", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-micro-0.25-1024",
						expectedNextSize:   "e2-custom-micro-0.25-2048",
					}),
					Entry("when the current Machine size is e2-custom-micro-o-2-1024", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-micro-o-2-1024",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-custom-micro-o-2-1024", errInstanceTypeUnsupportedFormat),
					}),
					Entry("when the current Machine size is e2-custom-micro2-2-1024", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-micro2-2-1024",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-custom-micro2-2-1024", errInstanceTypeUnsupportedFormat),
					}),
					Entry("when the current Machine size is e2-custom-small-0.5-2048", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-small-0.50-2048",
						expectedNextSize:   "e2-custom-small-0.50-3072",
					}),
					Entry("when the current Machine size is e2-custom-small-0.50-4096", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-small-0.50-4096",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-custom-small-0.50-4096", errInstanceTypeNotSupported),
					}),
					Entry("when the current Machine size is e2-custom-medium-1-4096", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-medium-1-4096",
						expectedNextSize:   "e2-custom-medium-1-5120",
					}),
					Entry("when the current Machine size is e2-custom-medium-1-8192", nextInstanceSizeTableInput{
						currentMachineSize: "e2-custom-medium-1-8192",
						expectedNextSize:   "",
						expectedError:      fmt.Errorf("%w: e2-custom-medium-1-8192", errInstanceTypeNotSupported),
					}),
				)
			})
		})
	})
})
