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
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/test/resourcebuilder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// CleanupResources will clean up resources of the types of objects given in a particular namespace if given.
// The namespace will then be removed once it has been emptied.
// This utility is intended to be used in AfterEach blocks to clean up from a specific test.
// It calls various gomega assertions so will fail a test if there are any errors.
func CleanupResources(ctx context.Context, cfg *rest.Config, k8sClient client.Client, namespace string, objs ...client.Object) {
	for _, obj := range objs {
		cleanupResource(ctx, k8sClient, namespace, obj)
	}

	if namespace != "" {
		removeNamespace(ctx, cfg, k8sClient, namespace)
	}
}

// cleanupResource removes all of a particular resource within a namespace.
func cleanupResource(ctx context.Context, k8sClient client.Client, namespace string, obj client.Object) {
	Expect(k8sClient.DeleteAllOf(ctx, obj, client.InNamespace(namespace))).To(Succeed())

	listObj := newListFromObject(k8sClient, obj)
	Eventually(komega.ObjectList(listObj)).Should(HaveField("Items", HaveLen(0)))
}

// newListFromObject converts an individual object type into a list object type to allow the
// the list to be checked for emptiness.
func newListFromObject(k8sClient client.Client, obj client.Object) client.ObjectList {
	objGVKs, _, err := k8sClient.Scheme().ObjectKinds(obj)
	Expect(err).ToNot(HaveOccurred())
	Expect(objGVKs).To(HaveLen(1))

	listGVK := objGVKs[0]
	listGVK.Kind = fmt.Sprintf("%sList", listGVK.Kind)

	newObj, err := k8sClient.Scheme().New(listGVK)
	Expect(err).ToNot(HaveOccurred())

	listObj, ok := newObj.(client.ObjectList)
	Expect(ok).To(BeTrue())

	return listObj
}

// removeNamespace handles the namespace finalization act that is normally performed by the garbage collector
// once it is happy that the namespace has no objects left within it.
func removeNamespace(ctx context.Context, cfg *rest.Config, k8sClient client.Client, namespace string) {
	coreClient, err := coreclient.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())

	nsBuilder := resourcebuilder.Namespace().WithName(namespace)

	// Delete the namespace
	ns := nsBuilder.Build()
	Expect(k8sClient.Delete(ctx, ns)).To(Succeed())

	// Remove the finalizer
	Eventually(func() error {
		if err := komega.Get(ns)(); err != nil {
			return fmt.Errorf("could not get namespace: %w", err)
		}
		ns.Spec.Finalizers = []corev1.FinalizerName{}

		_, err := coreClient.Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("could not finalize namespace: %w", err)
		}

		return nil
	}).Should(Succeed())
}
