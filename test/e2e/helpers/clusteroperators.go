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

package helpers

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// EventuallyClusterOperatorsShouldStabilise checks that the cluster operators stabilise over time.
// Stabilise means that they are available, are not progressing, and are not degraded.
func EventuallyClusterOperatorsShouldStabilise(minimumAvailability, timeout, interval time.Duration) {
	key := format.RegisterCustomFormatter(formatClusterOperatorsCondtions)
	defer format.UnregisterCustomFormatter(key)

	// The following assertion checks:
	// The list "Items", all (ConsistOf) have a field "Status.Conditions",
	// that contain elements that are both "Type" something and "Status" something.
	clusterOperators := &configv1.ClusterOperatorList{}

	By("Waiting for the cluster operators to stabilise (minimum availability time: " + minimumAvailability.String() + ", timeout: " + timeout.String() + ", polling interval: " + interval.String() + ")")

	Eventually(komega.ObjectList(clusterOperators), timeout, interval).Should(HaveField("Items", HaveEach(HaveField("Status.Conditions",
		SatisfyAll(
			ContainElement(
				And(
					HaveField("Type", Equal(configv1.OperatorAvailable)),
					HaveField("Status", Equal(configv1.ConditionTrue)),
					HaveField("LastTransitionTime.Time", millisecondsSince(BeNumerically(">", minimumAvailability.Milliseconds()))),
				),
			),
			ContainElement(
				And(
					HaveField("Type", Equal(configv1.OperatorProgressing)),
					HaveField("Status", Equal(configv1.ConditionFalse)),
					HaveField("LastTransitionTime.Time", millisecondsSince(BeNumerically(">", minimumAvailability.Milliseconds()))),
				),
			),
			ContainElement(
				And(
					HaveField("Type", Equal(configv1.OperatorDegraded)),
					HaveField("Status", Equal(configv1.ConditionFalse)),
					HaveField("LastTransitionTime.Time", millisecondsSince(BeNumerically(">", minimumAvailability.Milliseconds()))),
				),
			),
		),
	))), "cluster operators should all be available, not progressing and not degraded")
}

// millisecondsSince returns a transform matcher that transforms a time.Time into an int64 representing a duration in milliseconds.
// The duration is the time since the time.Time was created.
// If the duration is negative, the time.Time is in the future.
func millisecondsSince(matcher OmegaMatcher) OmegaMatcher {
	return WithTransform(func(t time.Time) int64 {
		return time.Since(t).Milliseconds()
	}, matcher)
}
