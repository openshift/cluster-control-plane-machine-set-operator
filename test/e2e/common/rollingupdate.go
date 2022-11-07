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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

// CheckRolloutForIndex first checks that a new machine is created in the correct index,
// and then checks that the new machine in the index is replaced correctly.
func CheckRolloutForIndex(testFramework framework.Framework, ctx context.Context, idx int) bool {
	By(fmt.Sprintf("Waiting for the index %d to be replaced", idx))
	// Don't provide additional timeouts here, the default should be enough.
	if ok := EventuallyIndexIsBeingReplaced(ctx, idx); !ok {
		return false
	}

	By(fmt.Sprintf("Index %d replacement created", idx))
	By(fmt.Sprintf("Checking the replacement machine for index %d", idx))

	if ok := CheckControlPlaneMachineRollingReplacement(testFramework, idx, ctx); !ok {
		return false
	}

	By(fmt.Sprintf("Replacement for index %d is complete", idx))

	return true
}

// CheckReplicasDoesNotExceedSurgeCapacity checks that, during a rolling update,
// the number of replicas within the control plane machine set never
// exceeds the desired number of replicas plus 1 additional machine for surge.
func CheckReplicasDoesNotExceedSurgeCapacity(ctx context.Context) bool {
	By("Checking the number of control plane machines never goes above 4 replicas")

	machineSelector := runtimeclient.MatchingLabels(framework.ControlPlaneMachineSetSelectorLabels())

	// For now, we are checking that the surge is limited to just 1 instance. So 3 + 1 = 4 maximum replicas.
	return Consistently(komega.ObjectList(&machinev1beta1.MachineList{}, machineSelector), ctx).Should(HaveField("Items", SatisfyAny(
		HaveLen(3),
		HaveLen(4),
	)), "control plane machines should never go above 4 replicas, or below 3 replicas")
}
