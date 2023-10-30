#!/usr/bin/env bash

set -euo pipefail

cp "vendor/github.com/openshift/api/machine/v1/0000_10_controlplanemachineset-Default.crd.yaml" "manifests/0000_30_control-plane-machine-set-operator_00_controlplanemachineset-default.crd.yaml"
cp "vendor/github.com/openshift/api/machine/v1/0000_10_controlplanemachineset-CustomNoUpgrade.crd.yaml" "manifests/0000_30_control-plane-machine-set-operator_00_controlplanemachineset-customnoupgrade.crd.yaml"
cp "vendor/github.com/openshift/api/machine/v1/0000_10_controlplanemachineset-TechPreviewNoUpgrade.crd.yaml" "manifests/0000_30_control-plane-machine-set-operator_00_controlplanemachineset-techpreviewnoupgrade.crd.yaml"
