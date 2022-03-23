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

package controlplanemachineset

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Cluster Operator Status with a running controller", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}

	var namespaceName string

	const operatorName = "control-plane-machine-set"

	var co *configv1.ClusterOperator

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-cluster-operator-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a manager and controller")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             testScheme,
			MetricsBindAddress: "0",
			Port:               testEnv.WebhookInstallOptions.LocalServingPort,
			Host:               testEnv.WebhookInstallOptions.LocalServingHost,
			CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		reconciler := &ControlPlaneMachineSetReconciler{
			Namespace:    namespaceName,
			OperatorName: operatorName,
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed(), "Reconciler should be able to setup with manager")

		By("Starting the manager")
		var mgrCtx context.Context
		mgrCtx, mgrCancel = context.WithCancel(context.Background())
		mgrDone = make(chan struct{})

		go func() {
			defer GinkgoRecover()
			defer close(mgrDone)

			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()

		// CVO will create a blank cluster operator for us before the operator starts.
		co = resourcebuilder.ClusterOperator().WithName(operatorName).Build()
		Expect(k8sClient.Create(ctx, co)).To(Succeed())
	})

	AfterEach(func() {
		By("Stopping the manager")
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone

		test.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&configv1.ClusterOperator{},
			&machinev1beta1.Machine{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("with no ControlPlaneMachineSet", func() {
		PIt("Set the cluster operator available", func() {
			co := resourcebuilder.ClusterOperator().WithName(operatorName).Build()

			Eventually(komega.Object(co)).Should(HaveField(".Status.Conditions", test.MatchClusterOperatorStatusConditions([]configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorUpgradeable,
					Status: configv1.ConditionTrue,
				},
			})))
		})

		Context("And an invalid cluster operator", func() {
			BeforeEach(func() {
				coStatus := resourcebuilder.ClusterOperatorStatus().Build()
				Eventually(komega.UpdateStatus(co, func() {
					co.Status = coStatus
				})).Should(Succeed())
			})

			PIt("Set the cluster operator available", func() {
				co := resourcebuilder.ClusterOperator().WithName(operatorName).Build()

				Eventually(komega.Object(co)).Should(HaveField(".Status.Conditions", test.MatchClusterOperatorStatusConditions([]configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
					{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionFalse,
					},
					{
						Type:   configv1.OperatorDegraded,
						Status: configv1.ConditionFalse,
					},
					{
						Type:   configv1.OperatorUpgradeable,
						Status: configv1.ConditionTrue,
					},
				})))
			})
		})

	})
})
