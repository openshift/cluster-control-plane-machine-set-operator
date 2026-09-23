/*
Copyright 2026 Red Hat, Inc.

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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("relatedObjects", func() {
	It("should stay in sync with the ClusterOperator manifest", func() {
		manifestPath := filepath.Join("..", "..", "..", "manifests", "0000_30_control-plane-machine-set-operator_04_clusteroperator.yaml")
		data, err := os.ReadFile(manifestPath)
		Expect(err).ToNot(HaveOccurred(), "should be able to read ClusterOperator manifest")

		co := &configv1.ClusterOperator{}
		Expect(yaml.Unmarshal(data, co)).To(Succeed(), "should be able to unmarshal ClusterOperator manifest")

		Expect(co.Status.RelatedObjects).To(Equal(relatedObjects()), "Go relatedObjects() must match the static ClusterOperator manifest")
	})
})
