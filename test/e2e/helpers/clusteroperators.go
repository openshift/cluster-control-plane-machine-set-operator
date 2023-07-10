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

package helpers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// EventuallyClusterOperatorsShouldStabilise checks that the cluster operators stabilise over time.
// Stabilise means that they are available, are not progressing, and are not degraded.
func EventuallyClusterOperatorsShouldStabilise(gomegaArgs ...interface{}) {
	key := format.RegisterCustomFormatter(formatClusterOperatorsCondtions)
	defer format.UnregisterCustomFormatter(key)

	// The following assertion checks:
	// The list "Items", all (ConsistOf) have a field "Status.Conditions",
	// that contain elements that are both "Type" something and "Status" something.
	clusterOperators := &configv1.ClusterOperatorList{}

	By("Waiting for the cluster operators to stabilise")

	Eventually(komega.ObjectList(clusterOperators), gomegaArgs...).Should(HaveField("Items", HaveEach(HaveField("Status.Conditions",
		SatisfyAll(
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorAvailable)), HaveField("Status", Equal(configv1.ConditionTrue)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorProgressing)), HaveField("Status", Equal(configv1.ConditionFalse)))),
			ContainElement(And(HaveField("Type", Equal(configv1.OperatorDegraded)), HaveField("Status", Equal(configv1.ConditionFalse)))),
		),
	))), "cluster operators should all be available, not progressing and not degraded")
}
