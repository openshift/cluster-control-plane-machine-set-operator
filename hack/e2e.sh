#!/bin/bash

set -e
set -o nounset
set -o pipefail

REPO_ROOT=$(dirname "${BASH_SOURCE}")/..

OPENSHIFT_CI=${OPENSHIFT_CI:-""}
ARTIFACT_DIR=${ARTIFACT_DIR:-""}
GINKGO=${GINKGO:-"go run ${REPO_ROOT}/vendor/github.com/onsi/ginkgo/v2/ginkgo"}
GINKGO_ARGS=${GINKGO_ARGS:-"-r -v --fail-fast --trace --timeout=4h"}
GINKGO_EXTRA_ARGS=${GINKGO_EXTRA_ARGS:-""}

if [ "$OPENSHIFT_CI" == "true" ] && [ -n "$ARTIFACT_DIR" ] && [ -d "$ARTIFACT_DIR" ]; then # detect ci environment there
  GINKGO_ARGS="${GINKGO_ARGS} --junit-report=junit_control_plane_machine_set_operator.xml --output-dir=${ARTIFACT_DIR}"
fi

# Print the command we are going to run as Make would.
echo ${GINKGO} ${GINKGO_ARGS} ${GINKGO_EXTRA_ARGS} ./test/e2e
${GINKGO} ${GINKGO_ARGS} ${GINKGO_EXTRA_ARGS} ./test/e2e
