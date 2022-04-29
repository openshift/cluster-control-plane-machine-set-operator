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

package providerconfig

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
)

var _ = Describe("Provider Config", func() {
	Context("NewProviderConfig", func() {
		type providerConfigTableInput struct {
			failureDomainsBuilder resourcebuilder.OpenShiftMachineV1Beta1FailureDomainsBuilder
			modifyTemplate        func(tmpl *machinev1.ControlPlaneMachineSetTemplate)
			providerSpecBuilder   resourcebuilder.RawExtensionBuilder
			providerConfigMatcher types.GomegaMatcher
			expectedPlatformType  configv1.PlatformType
			expectedError         error
		}

		DescribeTable("should extract the config", func(in providerConfigTableInput) {
			tmpl := resourcebuilder.OpenShiftMachineV1Beta1Template().
				WithFailureDomainsBuilder(in.failureDomainsBuilder).
				WithProviderSpecBuilder(in.providerSpecBuilder).
				BuildTemplate()

			if in.modifyTemplate != nil {
				// Modify the template to allow injection of errors where the resource builder does not.
				in.modifyTemplate(&tmpl)
			}

			providerConfig, err := NewProviderConfig(*tmpl.OpenShiftMachineV1Beta1Machine)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
				return
			}
			Expect(err).ToNot(HaveOccurred())

			Expect(providerConfig.Type()).To(Equal(in.expectedPlatformType))
			Expect(providerConfig).To(in.providerConfigMatcher)
		},
			PEntry("with an invalid platform type", providerConfigTableInput{
				modifyTemplate: func(in *machinev1.ControlPlaneMachineSetTemplate) {
					// The platform type should be inferred from here first.
					in.OpenShiftMachineV1Beta1Machine.FailureDomains.Platform = configv1.PlatformType("invalid")
				},
				expectedError: fmt.Errorf("%w: %s", errUnsupportedPlatformType, "invalid"),
			}),
			PEntry("with an AWS config with failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: resourcebuilder.AWSFailureDomains(),
				providerSpecBuilder:   resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField(".AWS().Config()", *resourcebuilder.AWSProviderSpec().Build()),
			}),
			PEntry("with an AWS config without failure domains", providerConfigTableInput{
				expectedPlatformType:  configv1.AWSPlatformType,
				failureDomainsBuilder: nil,
				providerSpecBuilder:   resourcebuilder.AWSProviderSpec(),
				providerConfigMatcher: HaveField(".AWS().Config()", *resourcebuilder.AWSProviderSpec().Build()),
			}),
		)
	})
})
