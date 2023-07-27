#!/usr/bin/env bash

set -euo pipefail

cp "vendor/github.com/openshift/api/machine/v1/0000_10_controlplanemachineset.crd.yaml" "manifests/0000_30_control-plane-machine-set-operator_00_controlplanemachineset.crd.yaml"
