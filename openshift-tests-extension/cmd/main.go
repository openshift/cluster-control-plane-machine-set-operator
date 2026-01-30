/*
Copyright 2025 Red Hat, Inc.

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

package main

import (
	"fmt"
	"os"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
	"github.com/spf13/cobra"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	// If using ginkgo, import your tests here.
	_ "github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e"
	"github.com/openshift/cluster-control-plane-machine-set-operator/test/e2e/framework"
)

func main() {
	extensionRegistry := e.NewRegistry()
	ext := e.NewExtension("openshift", "payload", "cluster-control-plane-machine-set-operator")

	ext.AddSuite(e.Suite{
		Name:       "cpmso/periodic",
		Qualifiers: []string{`labels.exists(l, l == "Periodic")`},
	})

	ext.AddSuite(e.Suite{
		Name:       "cpmso/presubmit",
		Qualifiers: []string{`labels.exists(l, l == "PreSubmit")`},
	})

//by zhaohua 
	if err := framework.InitFramework(); err != nil {
		panic(fmt.Sprintf("failed to initialize framework: %v", err))
	}

	komega.SetClient(framework.GlobalFramework.GetClient())
	komega.SetContext(framework.GlobalFramework.GetContext())

	specs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(fmt.Sprintf("couldn't build extension test specs from ginkgo: %+v", err.Error()))
	}

	// Initialize framework before running tests
	specs.AddBeforeAll(func() {
		if err := framework.InitFramework(); err != nil {
			panic(fmt.Sprintf("failed to initialize framework: %v", err))
		}

		komega.SetClient(framework.GlobalFramework.GetClient())
		komega.SetContext(framework.GlobalFramework.GetContext())
	})

	ext.AddSpecs(specs)
	extensionRegistry.Register(ext)

	root := &cobra.Command{
		Long: "cluster-control-plane-machine-set-operator tests extension for OpenShift",
	}

	root.AddCommand(cmd.DefaultExtensionCommands(extensionRegistry)...)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		os.Exit(1)
	}
}
