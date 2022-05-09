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
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
)

const (
	// unknownFailureDomain is used as the string representation of a failure
	// domain when the platform type is unrecognised.
	unknownFailureDomain = "<unknown>"
)

var (
	// errUnsupportedPlatformType is an error used when an unknown platform
	// type is configured within the failure domain config.
	errUnsupportedPlatformType = errors.New("unsupported platform type")
)

// FailureDomain is an interface that allows external code to interact with
// failure domains across different platform types.
type FailureDomain interface {
	// String returns a string representation of the failure domain.
	String() string

	// Type returns the platform type of the failure domain.
	Type() configv1.PlatformType

	// AWS returns the AWSFailureDomain if the platform type is AWS.
	AWS() machinev1.AWSFailureDomain
}

// failureDomain holds an implementation of the FailureDomain interface.
type failureDomain struct {
	platformType configv1.PlatformType

	aws machinev1.AWSFailureDomain
}

// String returns a string representation of the failure domain.
func (f failureDomain) String() string {
	switch f.platformType {
	case configv1.AWSPlatformType:
		return awsFailureDomainToString(f.aws)
	default:
		return unknownFailureDomain
	}
}

// Type returns the platform type of the failure domain.
func (f failureDomain) Type() configv1.PlatformType {
	return f.platformType
}

// AWS returns the AWSFailureDomain if the platform type is AWS.
func (f failureDomain) AWS() machinev1.AWSFailureDomain {
	return f.aws
}

// NewFailureDomains creates a set of FailureDomains representing the input failure
// domains held within the ControlPlaneMachineSet.
func NewFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	switch failureDomains.Platform {
	case configv1.AWSPlatformType:
		return newAWSFailureDomains(failureDomains)
	case configv1.PlatformType(""):
		// An empty failure domains definition is allowed.
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, failureDomains.Platform)
	}
}

// newAWSFailureDomains constructs a list of AWSFailureDomains from the provided
// failure domains configuration.
func newAWSFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	// TODO: replace this with actual logic to create failure domains from the failure domains.
	// This is here as a dummy to keep the linter happy.
	dummyFailureDomains := failureDomain{
		platformType: failureDomains.Platform,
	}

	return []FailureDomain{dummyFailureDomains}, nil
}

// awsFailureDomainToString converts the AWSFailureDomain into a string.
// Typically most failure domains are represented by their availability zone,
// so we return the AWS AvailabilityZone if it is set.
// Else return the Subnet which should be set if the AvailabilityZone is not.
func awsFailureDomainToString(fd machinev1.AWSFailureDomain) string {
	if fd.Placement.AvailabilityZone != "" {
		return fd.Placement.AvailabilityZone
	}

	if fd.Subnet != nil {
		switch fd.Subnet.Type {
		case machinev1.AWSARNReferenceType:
			if fd.Subnet.ARN != nil {
				return *fd.Subnet.ARN
			}
		case machinev1.AWSFiltersReferenceType:
			if fd.Subnet.Filters != nil {
				return fmt.Sprintf("%+v", *fd.Subnet.Filters)
			}
		case machinev1.AWSIDReferenceType:
			if fd.Subnet.ID != nil {
				return *fd.Subnet.ID
			}
		}
	}

	// If the previous attempts to find a suitable string do not work,
	// this should catch the fallthrough.
	return unknownFailureDomain
}
