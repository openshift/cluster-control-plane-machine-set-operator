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

package common

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("Machine tests", func() {
	Context("machineIndex", func() {
		type machineIndexTableInput struct {
			machine       *machinev1beta1.Machine
			expectedIndex int
			expectedError error
		}

		DescribeTable("should return the correct index", func(in machineIndexTableInput) {
			Expect(in.machine).ToNot(BeNil())

			index, err := machineIndex(*in.machine)

			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(index).To(Equal(in.expectedIndex))
		},
			Entry("with no name", machineIndexTableInput{
				machine:       &machinev1beta1.Machine{},
				expectedIndex: -1,
				expectedError: fmt.Errorf("%w: ", errMachineNameFormatInvalid),
			}),
			Entry("with a single digit numeric suffix", machineIndexTableInput{
				machine:       resourcebuilder.Machine().WithName("machine-worker-foo-2").Build(),
				expectedIndex: 2,
			}),
			Entry("with a two digit numeric suffix", machineIndexTableInput{
				machine:       resourcebuilder.Machine().WithName("machine-worker-foo-23").Build(),
				expectedIndex: 23,
			}),
			Entry("with a three digit numeric suffix", machineIndexTableInput{
				machine:       resourcebuilder.Machine().WithName("machine-worker-foo-234").Build(),
				expectedIndex: 234,
			}),
			Entry("with a non-digit suffix", machineIndexTableInput{
				machine:       resourcebuilder.Machine().WithName("machine-worker-foo-a").Build(),
				expectedIndex: -1,
				expectedError: fmt.Errorf("%w: machine-worker-foo-a", errMachineNameFormatInvalid),
			}),
		)
	})
})
