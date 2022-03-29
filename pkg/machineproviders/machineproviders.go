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

package machineproviders

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	machinev1 "github.com/openshift/api/machine/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errNotImplemented = errors.New("not implemented")

// NewMachineProvider constructs a MachineProvider based on the machine type passed.
// This can then be used to access and manipulate machines within the cluster.
func NewMachineProvider(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet) (MachineProvider, error) {
	return nil, errNotImplemented
}
