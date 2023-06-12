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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	machinev1beta1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("mergeMachineSlices tests", func() {
	type mergeMachineSlicesTableInput struct {
		sliceA   []machinev1beta1.Machine
		sliceB   []machinev1beta1.Machine
		expected []machinev1beta1.Machine
	}

	DescribeTable("should merge two machine slices",
		func(in mergeMachineSlicesTableInput) {
			output := mergeMachineSlices(in.sliceA, in.sliceB)
			Expect(output).To(Equal(in.expected))
		},
		Entry("when there are no duplicates", mergeMachineSlicesTableInput{
			sliceA: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-2").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
			},
			expected: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-2").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
			},
		}),
		Entry("when order is mixed up", mergeMachineSlicesTableInput{
			sliceA: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-2").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").Build(),
			},
			expected: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-2").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").Build(),
			},
		}),
		Entry("when there are duplicates", mergeMachineSlicesTableInput{
			sliceA: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").Build(),
			},
			expected: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-0").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-5").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-4").Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").Build(),
			},
		}),
	)
})

var _ = Describe("compareControlPlaneMachineSets tests", func() {

	var (
		usEast1aSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1a",
					},
				},
			},
		}

		usEast1bSubnetAWS = machinev1beta1.AWSResourceReference{
			Filters: []machinev1beta1.Filter{
				{
					Name: "tag:Name",
					Values: []string{
						"subnet-us-east-1b",
					},
				},
			},
		}

		usEast1aProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1a").
						WithSubnet(usEast1aSubnetAWS)

		usEast1bProviderSpecBuilderAWS = machinev1beta1resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1b").
						WithSubnet(usEast1bSubnetAWS)
	)

	type compareControlPlaneMachineSetsTableInput struct {
		logger        logr.Logger
		platformType  configv1.PlatformType
		cpmsABuilder  machinev1resourcebuilder.ControlPlaneMachineSetInterface
		cpmsBBuilder  machinev1resourcebuilder.ControlPlaneMachineSetInterface
		expectedError error
		expectedDiff  []string
	}

	DescribeTable("when comparing two ControlPlaneMachineSets",
		func(in compareControlPlaneMachineSetsTableInput) {
			diff, err := compareControlPlaneMachineSets(in.logger, in.cpmsABuilder.Build(), in.cpmsBBuilder.Build())
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(diff).To(Equal(in.expectedDiff))
			}
		},
		Entry("with two identical ControlPlaneMachineSets should find no diff", compareControlPlaneMachineSetsTableInput{
			logger:        logr.Logger{},
			platformType:  configv1.AWSPlatformType,
			cpmsABuilder:  machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template()),
			cpmsBBuilder:  machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template()),
			expectedError: nil,
			expectedDiff:  nil,
		}),
		Entry("with two differing ControlPlaneMachineSets should find diff", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS,
				)),
			cpmsBBuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1bProviderSpecBuilderAWS,
				)),
			expectedError: nil,
			expectedDiff: []string{
				"Subnet.Filters.slice[0].Values.slice[0]: subnet-us-east-1a != subnet-us-east-1b",
				"Placement.AvailabilityZone: us-east-1a != us-east-1b",
			},
		}),
		Entry("with two differing ControlPlaneMachineSet machine's provider specs should find diff", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.large"),
				)),
			cpmsBBuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge"),
				)),
			expectedError: nil,
			expectedDiff: []string{
				"InstanceType: c5.large != c5.xlarge",
			},
		}),
		Entry("with the first ControlPlaneMachineSet machine's provider spec being empty it should error", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					nil,
				)),
			cpmsBBuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge"),
				)),
			expectedError: fmt.Errorf("failed to extract providerSpec from MachineSpec: %w",
				fmt.Errorf("could not determine platform type: %w", errNilProviderSpec)),
			expectedDiff: nil,
		}),
		Entry("with the second ControlPlaneMachineSet machine's provider spec being empty it should error", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge"),
				)),
			cpmsBBuilder: machinev1resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(machinev1resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					nil,
				)),
			expectedError: fmt.Errorf("failed to extract providerSpec from MachineSpec: %w",
				fmt.Errorf("could not determine platform type: %w", errNilProviderSpec)),
			expectedDiff: nil,
		}),
	)
})

var _ = Describe("sortMachineSetsByCreationTimeDescending tests", func() {
	type sortMachineSetsByCreationTimeAscendingTableInput struct {
		input    []machinev1beta1.MachineSet
		expected []machinev1beta1.MachineSet
	}

	timeFirst := metav1.NewTime(time.Now().Add(time.Hour * 1))
	timeSecond := metav1.NewTime(time.Now().Add(time.Hour * 2))
	timeThird := metav1.NewTime(time.Now().Add(time.Hour * 3))

	DescribeTable("should sort (ascending) a slice of MachineSets by CreationTime,Name",
		func(in sortMachineSetsByCreationTimeAscendingTableInput) {
			output := sortMachineSetsByCreationTimeAscending(in.input)
			Expect(output).To(Equal(in.expected))
		},
		Entry("when the input is not sorted by time", sortMachineSetsByCreationTimeAscendingTableInput{
			input: []machinev1beta1.MachineSet{
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.MachineSet{
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
			},
		}),
		Entry("when the time is the same, input is not sorted by name", sortMachineSetsByCreationTimeAscendingTableInput{
			input: []machinev1beta1.MachineSet{
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.MachineSet{
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
			},
		}),
	)

})

var _ = Describe("sortMachineByCreationTimeDescending tests", func() {
	type sortMachinesByCreationTimeDescendingTableInput struct {
		input    []machinev1beta1.Machine
		expected []machinev1beta1.Machine
	}

	timeFirst := metav1.NewTime(time.Now().Add(time.Hour * 1))
	timeSecond := metav1.NewTime(time.Now().Add(time.Hour * 2))
	timeThird := metav1.NewTime(time.Now().Add(time.Hour * 3))

	DescribeTable("should sort (descending) a slice of Machines by CreationTime,Name",
		func(in sortMachinesByCreationTimeDescendingTableInput) {
			output := sortMachinesByCreationTimeDescending(in.input)
			Expect(output).To(Equal(in.expected))
		},
		Entry("when the input is not sorted (descending) by time", sortMachinesByCreationTimeDescendingTableInput{
			input: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
			},
			expected: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
		}),
		Entry("when the time is the same, input is not sorted (descending) by name", sortMachinesByCreationTimeDescendingTableInput{
			input: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.Machine{
				*machinev1beta1resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
				*machinev1beta1resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
		}),
	)

})
