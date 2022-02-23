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

package test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Cleanup", func() {
	var namespaceName string

	BeforeEach(func() {
		By("Setting up a namespace for the test")
		ns := resourcebuilder.Namespace().WithGenerateName("control-plane-machine-set-webhook-").Build()
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		namespaceName = ns.GetName()

		// Creating some Machines to cleanup
		machineBuilder := resourcebuilder.Machine().
			WithGenerateName("cleanup-resources-test-").
			WithNamespace(namespaceName)

		for i := 0; i < 3; i++ {
			Expect(k8sClient.Create(ctx, machineBuilder.Build())).To(Succeed())
		}
	})

	It("should delete all Machines", func() {
		CleanupResources(ctx, cfg, k8sClient, namespaceName,
			&machinev1beta1.Machine{},
		)
		Expect(komega.ObjectList(&machinev1beta1.MachineList{})()).To(HaveField("Items", HaveLen(0)))
	})

	It("should delete the namespace when given", func() {
		CleanupResources(ctx, cfg, k8sClient, namespaceName,
			&machinev1beta1.Machine{},
		)

		ns := resourcebuilder.Namespace().WithName(namespaceName).Build()
		namespaceNotFound := apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, namespaceName)

		Expect(komega.Get(ns)()).To(MatchError(namespaceNotFound))
	})

	It("should not error when no namespace is given", func() {
		// In this case it won't actually delete anything, but that shouldn't cause any errors.
		// Any remaining resources won't affect other tests as they are in a separate namespace.
		CleanupResources(ctx, cfg, k8sClient, "")
	})
})
