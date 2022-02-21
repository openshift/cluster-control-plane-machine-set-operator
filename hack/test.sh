#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(dirname "${BASH_SOURCE}")/..

OPENSHIFT_CI=${OPENSHIFT_CI:-""}
ARTIFACT_DIR=${ARTIFACT_DIR:-""}
GINKGO=${GINKGO:-"${REPO_ROOT}/bin/ginkgo"}
GINKGO_ARGS=${ARTIFACT_DIR:-"-r -v --randomize-all --randomize-suites --keep-going --race --trace --timeout=2m"}

if [ "$OPENSHIFT_CI" == "true" ] && [ -n "$ARTIFACT_DIR" ] && [ -d "$ARTIFACT_DIR" ]; then # detect ci environment there
  GINKGO_ARGS="${GINKGO_ARGS} --junit-report=${ARTIFACT_DIR}/junit_control_plane_machine_set_operator.xml --cover --coverprofile=${ARTIFACT_DIR}/cover.out"
fi

${GINKGO} ${GINKGO_ARGS} ./...
