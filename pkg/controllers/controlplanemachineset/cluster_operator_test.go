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

package controlplanemachineset

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	configv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/config/v1"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"
	machinev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1"
	metav1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/meta/v1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	statusConditionAvailable    = metav1resourcebuilder.Condition().WithType(conditionAvailable).WithStatus(metav1.ConditionTrue).WithReason(reasonAllReplicasAvailable).Build()
	statusConditionNotAvailable = metav1resourcebuilder.Condition().WithType(conditionAvailable).WithStatus(metav1.ConditionFalse).WithReason(reasonUnavailableReplicas).WithMessage("Missing 3 available replica(s)").Build()

	statusConditionProgressing    = metav1resourcebuilder.Condition().WithType(conditionProgressing).WithStatus(metav1.ConditionTrue).WithReason(reasonNeedsUpdateReplicas).WithMessage("Observed 1 replica(s) in need of update").Build()
	statusConditionNotProgressing = metav1resourcebuilder.Condition().WithType(conditionProgressing).WithStatus(metav1.ConditionFalse).WithReason(reasonAllReplicasUpdated).Build()

	statusConditionDegraded    = metav1resourcebuilder.Condition().WithType(conditionDegraded).WithStatus(metav1.ConditionTrue).WithReason(reasonUnmanagedNodes).WithMessage("Found 3 unmanaged node(s)").Build()
	statusConditionNotDegraded = metav1resourcebuilder.Condition().WithType(conditionDegraded).WithStatus(metav1.ConditionFalse).WithReason(reasonAsExpected).Build()
)

var _ = Describe("Cluster Operator Status with a running controller", func() {
	var mgrCancel context.CancelFunc
	var mgrDone chan struct{}

	var namespaceName string

	const operatorName = "control-plane-machine-set"

	var co *configv1.ClusterOperator

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-cluster-operator-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		By("Setting up a manager and controller")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: testScheme,
			Metrics: server.Options{
				BindAddress: "0",
			},
			WebhookServer: webhook.NewServer(webhook.Options{
				Port:    testEnv.WebhookInstallOptions.LocalServingPort,
				Host:    testEnv.WebhookInstallOptions.LocalServingHost,
				CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
			}),
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					namespaceName: {},
				},
			},
			Controller: config.Controller{
				SkipNameValidation: ptr.To(true),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Manager should be able to be created")

		By("Setting up a featureGateAccessor")
		featureGateAccessor, err := util.SetupFeatureGateAccessor(mgr)
		Expect(err).ToNot(HaveOccurred(), "Feature gate accessor should be created")

		reconciler := &ControlPlaneMachineSetReconciler{
			Client:              k8sClient,
			UncachedClient:      k8sClient,
			Namespace:           namespaceName,
			OperatorName:        operatorName,
			FeatureGateAccessor: featureGateAccessor,
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
		co = configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()
		Expect(k8sClient.Create(ctx, co)).To(Succeed())
	})

	AfterEach(func() {
		By("Stopping the manager")
		mgrCancel()
		// Wait for the mgrDone to be closed, which will happen once the mgr has stopped
		<-mgrDone

		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&corev1.Node{},
			&configv1.ClusterOperator{},
			&machinev1beta1.Machine{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("with no ControlPlaneMachineSet", func() {
		It("Set the cluster operator available", func() {
			co := configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()

			Eventually(komega.Object(co)).Should(HaveField("Status.Conditions", testutils.MatchClusterOperatorStatusConditions([]configv1.ClusterOperatorStatusCondition{
				{
					Type:    configv1.OperatorAvailable,
					Status:  configv1.ConditionTrue,
					Reason:  reasonAsExpected,
					Message: "cluster operator is available",
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionFalse,
					Reason: reasonAsExpected,
				},
				{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionFalse,
					Reason: reasonAsExpected,
				},
				{
					Type:    configv1.OperatorUpgradeable,
					Status:  configv1.ConditionTrue,
					Reason:  reasonAsExpected,
					Message: "cluster operator is upgradable",
				},
			})))
		})

		It("Set the cluster operator related objects", func() {
			co := configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()

			Eventually(komega.Object(co)).Should(HaveField("Status.RelatedObjects", ConsistOf(relatedObjects())))
		})

		Context("And an invalid cluster operator", func() {
			BeforeEach(func() {
				coStatus := configv1resourcebuilder.ClusterOperatorStatus().Build()
				Eventually(komega.UpdateStatus(co, func() {
					co.Status = coStatus
				})).Should(Succeed())
			})

			It("Set the cluster operator available", func() {
				Eventually(komega.Object(co)).Should(HaveField("Status.Conditions", testutils.MatchClusterOperatorStatusConditions([]configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Status:  configv1.ConditionTrue,
						Reason:  reasonAsExpected,
						Message: "cluster operator is available",
					},
					{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionFalse,
						Reason: reasonAsExpected,
					},
					{
						Type:   configv1.OperatorDegraded,
						Status: configv1.ConditionFalse,
						Reason: reasonAsExpected,
					},
					{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionTrue,
						Reason:  reasonAsExpected,
						Message: "cluster operator is upgradable",
					},
				})))
			})

			It("Set the cluster operator related objects", func() {
				co := configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()

				Eventually(komega.Object(co)).Should(HaveField("Status.RelatedObjects", ConsistOf(relatedObjects())))
			})
		})
	})
})

var _ = Describe("Cluster Operator Status", func() {
	const operatorName = "control-plane-machine-set"
	var co *configv1.ClusterOperator
	var reconciler *ControlPlaneMachineSetReconciler
	var logger testutils.TestLogger
	var namespaceName string

	var cpmsBuilder machinev1resourcebuilder.ControlPlaneMachineSetBuilder

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := corev1resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-cluster-operator-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		cpmsBuilder = machinev1resourcebuilder.ControlPlaneMachineSet().WithName(clusterControlPlaneMachineSetName).WithNamespace(namespaceName)

		reconciler = &ControlPlaneMachineSetReconciler{
			Client:         k8sClient,
			UncachedClient: k8sClient,
			Namespace:      namespaceName,
			OperatorName:   operatorName,
		}

		// CVO will create a blank cluster operator for us before the operator starts.
		co = configv1resourcebuilder.ClusterOperator().WithName(operatorName).Build()
		Expect(k8sClient.Create(ctx, co)).To(Succeed())

		logger = testutils.NewTestLogger()
	})

	AfterEach(func() {
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, namespaceName,
			&configv1.ClusterOperator{},
			&machinev1.ControlPlaneMachineSet{},
		)
	})

	Context("updateClusterOperatorStatus", func() {
		type updateClusterOperatorStatusTableInput struct {
			cpmsBuilder        machinev1resourcebuilder.ControlPlaneMachineSetInterface
			expectedConditions []configv1.ClusterOperatorStatusCondition
			expectedError      error
			expectedLogs       []testutils.LogEntry
		}

		DescribeTable("should update the cluster operator status based on the ControlPlaneMachineSet conditions", func(in updateClusterOperatorStatusTableInput) {
			cpms := in.cpmsBuilder.Build()
			originalCPMS := cpms.DeepCopy()

			err := reconciler.updateClusterOperatorStatus(ctx, logger.Logger(), cpms)
			if in.expectedError != nil {
				Expect(err).To(MatchError(in.expectedError))
				return
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Eventually(komega.Object(co)).Should(HaveField("Status.Conditions", testutils.MatchClusterOperatorStatusConditions(in.expectedConditions)))
			Expect(logger.Entries()).To(ConsistOf(in.expectedLogs))
			Expect(cpms).To(Equal(originalCPMS), "The update functions should not modify the ControlPlaneMachineSet in any way")
		},
			Entry("with an available control plane machine set", updateClusterOperatorStatusTableInput{
				cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{statusConditionAvailable, statusConditionNotProgressing, statusConditionNotDegraded}),
				expectedConditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Status:  configv1.ConditionTrue,
						Reason:  reasonAllReplicasAvailable,
						Message: "",
					},
					{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionFalse,
						Reason: reasonAllReplicasUpdated,
					},
					{
						Type:   configv1.OperatorDegraded,
						Status: configv1.ConditionFalse,
						Reason: reasonAsExpected,
					},
					{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionTrue,
						Reason:  reasonAsExpected,
						Message: "cluster operator is upgradable",
					},
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level:   4,
						Message: "Syncing cluster operator status",
						KeysAndValues: []interface{}{
							"available", string(metav1.ConditionTrue),
							"progressing", string(metav1.ConditionFalse),
							"degraded", string(metav1.ConditionFalse),
							"upgradable", string(metav1.ConditionTrue),
						},
					},
				},
			}),
			Entry("with a degraded control plane machine set", updateClusterOperatorStatusTableInput{
				cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{statusConditionNotAvailable, statusConditionNotProgressing, statusConditionDegraded}),
				expectedConditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Status:  configv1.ConditionFalse,
						Reason:  reasonUnavailableReplicas,
						Message: "Missing 3 available replica(s)",
					},
					{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionFalse,
						Reason: reasonAllReplicasUpdated,
					},
					{
						Type:    configv1.OperatorDegraded,
						Status:  configv1.ConditionTrue,
						Reason:  reasonUnmanagedNodes,
						Message: "Found 3 unmanaged node(s)",
					},
					{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Reason:  reasonAsExpected,
						Message: "cluster operator is not upgradable",
					},
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level:   4,
						Message: "Syncing cluster operator status",
						KeysAndValues: []interface{}{
							"available", string(metav1.ConditionFalse),
							"progressing", string(metav1.ConditionFalse),
							"degraded", string(metav1.ConditionTrue),
							"upgradable", string(metav1.ConditionFalse),
						},
					},
				},
			}),
			Entry("with a progressing control plane machine set", updateClusterOperatorStatusTableInput{
				cpmsBuilder: cpmsBuilder.WithConditions([]metav1.Condition{statusConditionNotAvailable, statusConditionProgressing, statusConditionNotDegraded}),
				expectedConditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorAvailable,
						Status:  configv1.ConditionFalse,
						Reason:  reasonUnavailableReplicas,
						Message: "Missing 3 available replica(s)",
					},
					{
						Type:    configv1.OperatorProgressing,
						Status:  configv1.ConditionTrue,
						Reason:  reasonNeedsUpdateReplicas,
						Message: "Observed 1 replica(s) in need of update",
					},
					{
						Type:   configv1.OperatorDegraded,
						Status: configv1.ConditionFalse,
						Reason: reasonAsExpected,
					},
					{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Reason:  reasonAsExpected,
						Message: "cluster operator is not upgradable",
					},
				},
				expectedLogs: []testutils.LogEntry{
					{
						Level:   4,
						Message: "Syncing cluster operator status",
						KeysAndValues: []interface{}{
							"available", string(metav1.ConditionFalse),
							"progressing", string(metav1.ConditionTrue),
							"degraded", string(metav1.ConditionFalse),
							"upgradable", string(metav1.ConditionFalse),
						},
					},
				},
			}),
		)
	})

	Context("relatedObjectsChanged", func() {
		type relatedObjectsChangedTableInput struct {
			currentRelatedObjects []configv1.ObjectReference
			expected              bool
		}

		DescribeTable("should return true if the related objects on the ClusterOperator have changed", func(in relatedObjectsChangedTableInput) {
			co.Status.RelatedObjects = in.currentRelatedObjects
			Expect(relatedObjectsChanged(co)).To(Equal(in.expected), "relatedObjectsChanged should return %t", in.expected)
		},
			Entry("with no related objects", relatedObjectsChangedTableInput{
				currentRelatedObjects: nil,
				expected:              true,
			}),
			Entry("with too many related objects", relatedObjectsChangedTableInput{
				currentRelatedObjects: append(relatedObjects(), configv1.ObjectReference{Group: "test", Resource: "test", Name: "test"}),
				expected:              true,
			}),
			Entry("with too few related objects", relatedObjectsChangedTableInput{
				currentRelatedObjects: relatedObjects()[:len(relatedObjects())-1],
				expected:              true,
			}),
			Entry("with the correct number of related objects, but one is mismatched", relatedObjectsChangedTableInput{
				currentRelatedObjects: append(relatedObjects()[:len(relatedObjects())-1], configv1.ObjectReference{Group: "test", Resource: "test", Name: "test"}),
				expected:              true,
			}),
			Entry("with the correct related objects", relatedObjectsChangedTableInput{
				currentRelatedObjects: relatedObjects(),
				expected:              false,
			}),
		)
	})
})
