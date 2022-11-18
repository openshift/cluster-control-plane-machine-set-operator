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

package controlplanemachinesetgenerator

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
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
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-2").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-3").Build(),
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
			},
			expected: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-2").Build(),
				*resourcebuilder.Machine().WithName("machine-3").Build(),
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
			},
		}),
		Entry("when order is mixed up", mergeMachineSlicesTableInput{
			sliceA: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-2").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-5").Build(),
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-6").Build(),
			},
			expected: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-2").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-6").Build(),
			},
		}),
		Entry("when there are duplicates", mergeMachineSlicesTableInput{
			sliceA: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
			},
			sliceB: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
				*resourcebuilder.Machine().WithName("machine-6").Build(),
			},
			expected: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-0").Build(),
				*resourcebuilder.Machine().WithName("machine-1").Build(),
				*resourcebuilder.Machine().WithName("machine-5").Build(),
				*resourcebuilder.Machine().WithName("machine-4").Build(),
				*resourcebuilder.Machine().WithName("machine-6").Build(),
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

		usEast1aProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1a").
						WithSubnet(usEast1aSubnetAWS)

		usEast1bProviderSpecBuilderAWS = resourcebuilder.AWSProviderSpec().
						WithAvailabilityZone("us-east-1b").
						WithSubnet(usEast1bSubnetAWS)
	)

	type compareControlPlaneMachineSetsTableInput struct {
		platformType  configv1.PlatformType
		cpmsABuilder  resourcebuilder.ControlPlaneMachineSetInterface
		cpmsBBuilder  resourcebuilder.ControlPlaneMachineSetInterface
		expectedError error
		expectedDiff  []string
	}

	DescribeTable("when comparing two ControlPlaneMachineSets",
		func(in compareControlPlaneMachineSetsTableInput) {
			diff, err := compareControlPlaneMachineSets(in.cpmsABuilder.Build(), in.cpmsBBuilder.Build())
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(diff).To(Equal(in.expectedDiff))
			}
		},
		Entry("with two identical ControlPlaneMachineSets should find no diff", compareControlPlaneMachineSetsTableInput{
			platformType:  configv1.AWSPlatformType,
			cpmsABuilder:  resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template()),
			cpmsBBuilder:  resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template()),
			expectedError: nil,
			expectedDiff:  nil,
		}),
		Entry("with two differing ControlPlaneMachineSets should find diff", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS,
				)),
			cpmsBBuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
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
			cpmsABuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.large"),
				)),
			cpmsBBuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
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
			cpmsABuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					nil,
				)),
			cpmsBBuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge"),
				)),
			expectedError: fmt.Errorf("failed to extract providerSpec from MachineSpec: %w",
				fmt.Errorf("could not determine platform type: %w", errNilProviderSpec)),
			expectedDiff: nil,
		}),
		Entry("with the second ControlPlaneMachineSet machine's provider spec being empty it should error", compareControlPlaneMachineSetsTableInput{
			platformType: configv1.AWSPlatformType,
			cpmsABuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithProviderSpecBuilder(
					usEast1aProviderSpecBuilderAWS.WithInstanceType("c5.xlarge"),
				)),
			cpmsBBuilder: resourcebuilder.ControlPlaneMachineSet().WithMachineTemplateBuilder(resourcebuilder.OpenShiftMachineV1Beta1Template().
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
				*resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
				*resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.MachineSet{
				*resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
			},
		}),
		Entry("when the time is the same, input is not sorted by name", sortMachineSetsByCreationTimeAscendingTableInput{
			input: []machinev1beta1.MachineSet{
				*resourcebuilder.MachineSet().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.MachineSet{
				*resourcebuilder.MachineSet().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.MachineSet().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.MachineSet().WithName("machine-6").
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
				*resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
			},
			expected: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeThird).Build(),
				*resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeSecond).Build(),
				*resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
		}),
		Entry("when the time is the same, input is not sorted (descending) by name", sortMachinesByCreationTimeDescendingTableInput{
			input: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
			},
			expected: []machinev1beta1.Machine{
				*resourcebuilder.Machine().WithName("machine-6").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.Machine().WithName("machine-3").
					WithCreationTimestamp(timeFirst).Build(),
				*resourcebuilder.Machine().WithName("machine-1").
					WithCreationTimestamp(timeFirst).Build(),
			},
		}),
	)

})
