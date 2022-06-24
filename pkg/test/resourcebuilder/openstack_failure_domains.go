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

// nolint: dupl // Linter warns about duplicated code with OpenStack, GCP, and Azure FailureDomains builders.
// While the builders are almost identical, we need to keep them separate because they build different objects.
package resourcebuilder

import (
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
)

// OpenStackFailureDomains creates a new failure domains builder for OpenStack.
func OpenStackFailureDomains() OpenStackFailureDomainsBuilder {
	return OpenStackFailureDomainsBuilder{[]OpenStackFailureDomainBuilder{
		OpenStackFailureDomain().WithAvailabilityZone("zone-1"),
		OpenStackFailureDomain().WithAvailabilityZone("zone-2"),
		OpenStackFailureDomain().WithAvailabilityZone("zone-3"),
	}}
}

// OpenStackFailureDomainsBuilder is used to build a failuredomains.
type OpenStackFailureDomainsBuilder struct {
	failureDomainsBuilders []OpenStackFailureDomainBuilder
}

// BuildFailureDomains builds a failuredomains from the configuration.
func (m OpenStackFailureDomainsBuilder) BuildFailureDomains() machinev1.FailureDomains {
	fds := machinev1.FailureDomains{
		Platform:  configv1.OpenStackPlatformType,
		OpenStack: &[]machinev1.OpenStackFailureDomain{},
	}

	for _, builder := range m.failureDomainsBuilders {
		*fds.OpenStack = append(*fds.OpenStack, builder.Build())
	}

	return fds
}

// WithFailureDomainBuilder adds a failure domain builder to the failure domains builder's builders.
func (m OpenStackFailureDomainsBuilder) WithFailureDomainBuilder(fdbuilder OpenStackFailureDomainBuilder) OpenStackFailureDomainsBuilder {
	m.failureDomainsBuilders = append(m.failureDomainsBuilders, fdbuilder)
	return m
}

// WithFailureDomainBuilders replaces the OpenStack failure domains builder's builders with the given builders.
func (m OpenStackFailureDomainsBuilder) WithFailureDomainBuilders(fdbuilders []OpenStackFailureDomainBuilder) OpenStackFailureDomainsBuilder {
	m.failureDomainsBuilders = fdbuilders
	return m
}

// OpenStackFailureDomain creates a new OpenStack failuredomain builder for OpenStack.
func OpenStackFailureDomain() OpenStackFailureDomainBuilder {
	return OpenStackFailureDomainBuilder{}
}

// OpenStackFailureDomainBuilder is used to build an OpenStack failuredomain.
type OpenStackFailureDomainBuilder struct {
	availabilityZone string
}

// Build builds a OpenStack failuredomain from the configuration.
func (m OpenStackFailureDomainBuilder) Build() machinev1.OpenStackFailureDomain {
	return machinev1.OpenStackFailureDomain{
		AvailabilityZone: m.availabilityZone,
	}
}

// WithAvailabilityZone sets the availability zone for the OpenStack failuredomain builder.
func (m OpenStackFailureDomainBuilder) WithAvailabilityZone(availabilityZone string) OpenStackFailureDomainBuilder {
	m.availabilityZone = availabilityZone
	return m
}
