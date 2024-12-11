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
	"context"
	"errors"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var (
	errUIDNotChanged = errors.New("UID has not changed")
)

// ExpectControlPlaneMachineSetToBeActive gets the control plane machine set and
// checks that it is active.
func ExpectControlPlaneMachineSetToBeActive(testFramework framework.Framework) {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	k8sClient := testFramework.GetClient()

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateActive), "control plane machine set should be active")
}

// ExpectControlPlaneMachineSetToBeInactive gets the control plane machine set and
// checks that it is active.
func ExpectControlPlaneMachineSetToBeInactive(testFramework framework.Framework) {
	By("Checking the control plane machine set is inactive")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	k8sClient := testFramework.GetClient()

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms)).To(Succeed(), "control plane machine set should exist")

	Expect(cpms.Spec.State).To(Equal(machinev1.ControlPlaneMachineSetStateInactive), "control plane machine set should be inactive")
}

// ExpectControlPlaneMachineSetToBeInactiveOrNotFound gets the control plane machine set and
// checks that it is inactive or not found.
func ExpectControlPlaneMachineSetToBeInactiveOrNotFound(testFramework framework.Framework) {
	By("Checking the control plane machine set is inactive or not found")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	k8sClient := testFramework.GetClient()

	cpms := &machinev1.ControlPlaneMachineSet{}
	if err := k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms); err != nil {
		Expect(err).To(MatchError(ContainSubstring("not found")), "getting control plane machine set should not error")
	} else {
		Expect(cpms).To(HaveField("Spec.State", machinev1.ControlPlaneMachineSetStateInactive), "control plane machine set should be inactive or should not exist")
	}
}

// EnsureActiveControlPlaneMachineSet ensures that there is an active control plane machine set
// within the cluster. For fully supported clusters, this means waiting for the control plane machine set
// to be created and checking that it is active. For manually supported clusters, this means creating the
// control plane machine set, checking its status and then activating it.
func EnsureActiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	switch testFramework.GetPlatformSupportLevel() {
	case framework.Full:
		ensureActiveControlPlaneMachineSet(testFramework, gomegaArgs...)
	case framework.Manual:
		ensureManualActiveControlPlaneMachineSet(testFramework, gomegaArgs...)
	case framework.Unsupported:
		Fail(fmt.Sprintf("control plane machine set does not support platform %s", testFramework.GetPlatformType()))
	}
}

// EnsureInactiveControlPlaneMachineSet ensures that there is an inactive control plane machine set
// within the cluster.
func EnsureInactiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	switch testFramework.GetPlatformSupportLevel() {
	case framework.Full, framework.Manual:
		ensureInactiveControlPlaneMachineSet(testFramework, gomegaArgs...)
	case framework.Unsupported:
		Fail(fmt.Sprintf("control plane machine set does not support platform %s", testFramework.GetPlatformType()))
	}
}

// ensureInactiveControlPlaneMachineSet checks that a CPMS exists and is inactive.
func ensureInactiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	cpms := testFramework.NewEmptyControlPlaneMachineSet()
	ctx := testFramework.GetContext()

	By("Checking the control plane machine set exists")

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateInactive {
		DeleteControlPlaneMachineSet(testFramework, ctx, cpms)
	}

	By("Checking the control plane machine set is inactive")

	Eventually(komega.Object(cpms), gomegaArgs...).Should(HaveField("Spec.State", Equal(machinev1.ControlPlaneMachineSetStateInactive)), "control plane machine set should be inactive")
}

// ensureActiveControlPlaneMachineSet checks that a CPMS exists and then, if it is not active, activates it.
func ensureActiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	cpms := testFramework.NewEmptyControlPlaneMachineSet()

	By("Checking the control plane machine set exists")

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateActive {
		By("Activating the control plane machine set")

		Eventually(komega.Update(cpms, func() {
			cpms.Spec.State = machinev1.ControlPlaneMachineSetStateActive
		}), gomegaArgs...).Should(Succeed(), "control plane machine set should be able to be actived")
	}

	By("Checking the control plane machine set is active")

	Eventually(komega.Object(cpms), gomegaArgs...).Should(HaveField("Spec.State", Equal(machinev1.ControlPlaneMachineSetStateActive)), "control plane machine set should be active")
}

// ensureManualActiveControlPlaneMachineSet creates a CPMS if required and then activates it.
// If the CPMS already exists and is inactive, it will be activated.
func ensureManualActiveControlPlaneMachineSet(testFramework framework.Framework, gomegaArgs ...interface{}) {
	k8sClient := testFramework.GetClient()
	ctx := testFramework.GetContext()

	cpms := testFramework.NewEmptyControlPlaneMachineSet()
	if err := k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms); err != nil && !apierrors.IsNotFound(err) {
		Fail(fmt.Sprintf("error when checking if a control plane machine set exists: %v", err))
	} else if err == nil {
		// The CPMS exists, so we just need to make sure it is active.
		ensureActiveControlPlaneMachineSet(testFramework, gomegaArgs...)
		return
	}

	// A CPMS does not already exist, we must create one and then activate it.
	// TODO: Implement create functions for platforms that don't have generators yet.
	Fail("manual support for the control plane machine set not yet implemented")
}

// WaitForControlPlaneMachineSetDesiredReplicas waits for the control plane machine set to have the desired number of replicas.
// It first waits for the updated replicas to equal the desired number, and then waits for the final replica
// count to equal the desired number.
func WaitForControlPlaneMachineSetDesiredReplicas(ctx context.Context, cpms *machinev1.ControlPlaneMachineSet) bool {
	if ok := Expect(cpms.Spec.Replicas).ToNot(BeNil(), "replicas should always be set"); !ok {
		return false
	}

	desiredReplicas := *cpms.Spec.Replicas

	By("Waiting for the updated replicas to equal desired replicas")

	if ok := Eventually(komega.Object(cpms)).WithContext(ctx).Should(HaveField("Status.UpdatedReplicas", Equal(desiredReplicas)), "control plane machine set should have updated all replicas"); !ok {
		return false
	}

	By("Updated replicas is now equal to desired replicas")

	// Once the updated replicas equals the desired replicas, we need
	// to wait for the total replicas to go back to the desired replicas.
	// This will check the final machine gets removed before we end the test.
	By("Waiting for the replicas to equal desired replicas")

	if ok := Eventually(komega.Object(cpms)).WithContext(ctx).Should(HaveField("Status.Replicas", Equal(desiredReplicas)), "control plane machine set should have the desired number of replicas"); !ok {
		return false
	}

	By("Replicas is now equal to desired replicas")

	return true
}

// EnsureControlPlaneMachineSetUpdateStrategy ensures that the control plane machine set has the specified update strategy.
func EnsureControlPlaneMachineSetUpdateStrategy(testFramework framework.Framework, strategy machinev1.ControlPlaneMachineSetStrategyType, gomegaArgs ...interface{}) machinev1.ControlPlaneMachineSetStrategyType {
	k8sClient := testFramework.GetClient()
	ctx := testFramework.GetContext()

	cpms := testFramework.NewEmptyControlPlaneMachineSet()
	if err := k8sClient.Get(ctx, testFramework.ControlPlaneMachineSetKey(), cpms); apierrors.IsNotFound(err) {
		Fail("control plane machine set does not exist")
	} else if err != nil {
		Fail(fmt.Sprintf("error when checking if a control plane machine set exists: %v", err))
	}

	originalStrategy := cpms.Spec.Strategy.Type
	if originalStrategy == strategy {
		return originalStrategy
	}

	By(fmt.Sprintf("Updating the control plane machine set strategy to %s", strategy))

	Eventually(komega.Update(cpms, func() {
		cpms.Spec.Strategy.Type = strategy
	}), gomegaArgs...).Should(Succeed(), "control plane machine set should be able to be updated")

	return originalStrategy
}

// EnsureControlPlaneMachineSetUpdated ensures the control plane machine set is up to date.
func EnsureControlPlaneMachineSetUpdated(testFramework framework.Framework) {
	By("Checking the control plane machine set is up to date")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	ctx := testFramework.GetContext()

	cpms := testFramework.NewEmptyControlPlaneMachineSet()
	Eventually(komega.Get(cpms)).Should(Succeed(), "control plane machine set should exist")

	WaitForControlPlaneMachineSetDesiredReplicas(ctx, cpms)
}

// EnsureControlPlaneMachineSetDeleted ensures the control plane machine set
// is deleted and properly removed/recreated.
func EnsureControlPlaneMachineSetDeleted(testFramework framework.Framework) {
	By("Ensuring the control plane machine set is deleted")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	k8sClient := testFramework.GetClient()
	ctx := testFramework.GetContext()

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms)).
		To(Succeed(), "control plane machine set should exist")

	DeleteControlPlaneMachineSet(testFramework, ctx, cpms)

	WaitForControlPlaneMachineSetRemovedOrRecreated(ctx, testFramework, cpms.ObjectMeta.UID)
}

// DeleteControlPlaneMachineSet deletes the control plane machine set.
func DeleteControlPlaneMachineSet(testFramework framework.Framework, ctx context.Context, cpms *machinev1.ControlPlaneMachineSet) {
	By("Deleting the control plane machine set")

	k8sClient := testFramework.GetClient()
	Expect(k8sClient.Delete(ctx, cpms)).To(Succeed(), "control plane machine set should have been deleted")
}

// WaitForControlPlaneMachineSetRemovedOrRecreated waits for the control plane machine set to be removed.
func WaitForControlPlaneMachineSetRemovedOrRecreated(ctx context.Context, testFramework framework.Framework, oldCPMSUID types.UID) bool {
	By("Waiting for the deleted control plane machine set to be removed/recreated")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")
	k8sClient := testFramework.GetClient()

	if ok := Eventually(func() error {
		newCPMS := &machinev1.ControlPlaneMachineSet{}
		if err := k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), newCPMS); err != nil {
			return fmt.Errorf("error getting control plane machine set: %w", err)
		}

		if newCPMS.GetUID() == oldCPMSUID {
			return errUIDNotChanged
		}

		return nil
	}).Should(SatisfyAny(
		BeNil(),
		MatchError("not found"),
	), "control plane machine set should be inactive or should not exist"); !ok {
		return false
	}

	By("Control plane machine set is now removed/recreated")

	return true
}

// IncreaseControlPlaneMachineSetInstanceSize increases the instance size of the control plane machine set.
// This should trigger the control plane machine set to update the machines based on the
// update strategy.
func IncreaseControlPlaneMachineSetInstanceSize(testFramework framework.Framework, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	cpms := testFramework.NewEmptyControlPlaneMachineSet()

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	originalProviderSpec := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec
	updatedProviderSpec := originalProviderSpec.DeepCopy()

	platformType := testFramework.GetPlatformType()

	switch platformType {
	case configv1.OpenStackPlatformType:
		// OpenStack flavors are not predictable. So if OPENSTACK_CONTROLPLANE_FLAVOR_ALTERNATE is set in the environment, we'll use it
		// to change the instance flavor, otherwise we just tag the instance with a new tag, which will trigger the redeployment.
		if os.Getenv("OPENSTACK_CONTROLPLANE_FLAVOR_ALTERNATE") == "" {
			Expect(testFramework.TagInstanceInProviderSpec(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with a new tag")
		} else {
			Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")
		}
	default:
		Expect(testFramework.IncreaseProviderSpecInstanceSize(updatedProviderSpec.Value)).To(Succeed(), "provider spec should be updated with bigger instance size")
	}

	By("Increasing the control plane machine set instance size")

	Eventually(komega.Update(cpms, func() {
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec = *updatedProviderSpec
	}), gomegaArgs...).Should(Succeed(), "control plane machine set should be able to be updated")

	return originalProviderSpec
}

// GetControlPlaneMachineSetUID gets the UID of the control plane machine set.
func GetControlPlaneMachineSetUID(testFramework framework.Framework) types.UID {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	k8sClient := testFramework.GetClient()

	cpms := &machinev1.ControlPlaneMachineSet{}
	Expect(k8sClient.Get(testFramework.GetContext(), testFramework.ControlPlaneMachineSetKey(), cpms)).
		To(Succeed(), "control plane machine set should exist")

	return cpms.ObjectMeta.UID
}

// UpdateDefaultedValueFromControlPlaneMachineSetProviderConfig updates a defaulted field value from the Control Plane Machine Set's
// provider config to test defaulting on such value.
func UpdateDefaultedValueFromControlPlaneMachineSetProviderConfig(testFramework framework.Framework, gomegaArgs ...interface{}) machinev1beta1.ProviderSpec {
	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	cpms := testFramework.NewEmptyControlPlaneMachineSet()

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	originalProviderSpec := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec

	updatedProviderSpec := originalProviderSpec.DeepCopy()
	rawExtension, err := testFramework.UpdateDefaultedValueFromCPMS(updatedProviderSpec.Value)
	Expect(err).NotTo(HaveOccurred())

	updatedProviderSpec.Value = rawExtension

	By("Removing the defaulted field from the control plane machine set")

	Eventually(komega.Update(cpms, func() {
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec = *updatedProviderSpec
	}), gomegaArgs...).Should(Succeed(), "control plane machine set should be able to be updated")

	return originalProviderSpec
}

// UpdateControlPlaneMachineSetProviderSpec updates the provider spec of the control plane machine set to match the provider spec given.
func UpdateControlPlaneMachineSetProviderSpec(testFramework framework.Framework, updatedProviderSpec machinev1beta1.ProviderSpec, gomegaArgs ...interface{}) {
	By("Updating the provider spec of the control plane machine set")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	cpms := testFramework.NewEmptyControlPlaneMachineSet()

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	Eventually(komega.Update(cpms, func() {
		cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec = updatedProviderSpec
	}), gomegaArgs...).Should(Succeed(), "control plane machine should be able to be updated")
}

// UpdateControlPlaneMachineSetMachineNamePrefix updates the machine name prefix of the control plane machine set to match the machineNamePrefix given.
func UpdateControlPlaneMachineSetMachineNamePrefix(testFramework framework.Framework, machineNamePrefix string, gomegaArgs ...interface{}) {
	By("Updating the machine name prefix of the control plane machine set")

	Expect(testFramework).ToNot(BeNil(), "test framework should not be nil")

	cpms := testFramework.NewEmptyControlPlaneMachineSet()

	Eventually(komega.Get(cpms), gomegaArgs...).Should(Succeed(), "control plane machine set should exist")

	Eventually(komega.Update(cpms, func() {
		cpms.Spec.MachineNamePrefix = machineNamePrefix
	}), gomegaArgs...).Should(Succeed(), "control plane machine should be able to be updated")
}
