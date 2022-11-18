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

package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders"
	openshiftmachinev1beta1 "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1"
)

var (
	// errUnexpectedMachineType is used to denote that the machine provider could not be
	// constructed because an unknown machine type was requested.
	errUnexpectedMachineType = errors.New("unexpected value for spec.template.machineType")
)

// NewMachineProvider constructs a MachineProvider based on the machine type passed.
// This can then be used to access and manipulate machines within the cluster.
func NewMachineProvider(ctx context.Context, logger logr.Logger, cl client.Client, cpms *machinev1.ControlPlaneMachineSet) (machineproviders.MachineProvider, error) {
	switch cpms.Spec.Template.MachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		provider, err := openshiftmachinev1beta1.NewMachineProvider(ctx, logger, cl, cpms)
		if err != nil {
			return nil, fmt.Errorf("error constructing %s machine provider: %w", machinev1.OpenShiftMachineV1Beta1MachineType, err)
		}

		return provider, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnexpectedMachineType, cpms.Spec.Template.MachineType)
	}
}

// GetMachineTypeMeta returns proper TypeMeta from ControlPlaneMachineSetMachineType.
func GetMachineTypeMeta(cpmsMachineType machinev1.ControlPlaneMachineSetMachineType) (metav1.TypeMeta, error) {
	switch cpmsMachineType {
	case machinev1.OpenShiftMachineV1Beta1MachineType:
		gv := machinev1beta1.GroupVersion

		return metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		}, nil
	default:
		return metav1.TypeMeta{}, fmt.Errorf("%w: %s", errUnexpectedMachineType, cpmsMachineType)
	}
}
