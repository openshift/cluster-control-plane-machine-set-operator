#!/bin/bash

set -o nounset
set -o pipefail

REPO_ROOT=$(dirname "${BASH_SOURCE}")/..

OPENSHIFT_CI=${OPENSHIFT_CI:-""}
ARTIFACT_DIR=${ARTIFACT_DIR:-""}
GINKGO=${GINKGO:-"go run -mod=vendor ${REPO_ROOT}/vendor/github.com/onsi/ginkgo/v2/ginkgo"}
GINKGO_BUILD_ARGS=${GINKGO_BUILD_ARGS:-"--race"}
GINKGO_ARGS=${GINKGO_ARGS:-"-v --randomize-all --randomize-suites --keep-going --trace --timeout=25m"}
GINKGO_EXTRA_ARGS=${GINKGO_EXTRA_ARGS:-""}

# Ensure that some home var is set and that it's not the root.
# This is required for the kubebuilder cache.
export HOME=${HOME:=/tmp/kubebuilder-testing}
if [ $HOME == "/" ]; then
  export HOME=/tmp/kubebuilder-testing
fi

if [ "$OPENSHIFT_CI" == "true" ] && [ -n "$ARTIFACT_DIR" ] && [ -d "$ARTIFACT_DIR" ]; then # detect ci environment there
  GINKGO_BUILD_ARGS="${GINKGO_BUILD_ARGS} --cover"
  GINKGO_ARGS="${GINKGO_ARGS} --junit-report=junit_control_plane_machine_set_operator.xml --coverprofile=test-unit-coverage.out --output-dir=${ARTIFACT_DIR}"
fi

# Exclude the specific E2E package as this is not a unit test.
# This regex should allow packages under the e2e dir to still be tested.
TEST_PACKAGES=${TEST_PACKAGES:-$(go list -f "{{ .Dir }}" ./... | grep -v cluster-control-plane-machine-set-operator/test/e2e$)}

# Build outside --timeout, then run the precompiled binaries.
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) ginkgo build starting"
build_start=$SECONDS
echo ${GINKGO} build ${GINKGO_BUILD_ARGS} "<omitted>"
GOGC=50 ${GINKGO} build ${GINKGO_BUILD_ARGS} ${TEST_PACKAGES} || exit $?
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) ginkgo build finished in $((SECONDS - build_start))s"

TEST_BINARIES=()
for pkg in ${TEST_PACKAGES}; do
  bin="${pkg}/$(basename "${pkg}").test"
  [ -x "${bin}" ] && TEST_BINARIES+=("${bin}")
done

if ((${#TEST_BINARIES[@]} == 0)); then
  echo "ERROR: no test binaries were built" >&2
  exit 1
fi

echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) ginkgo run starting (${#TEST_BINARIES[@]} binaries)"
run_start=$SECONDS
echo ${GINKGO} run ${GINKGO_ARGS} ${GINKGO_EXTRA_ARGS} "<omitted>"
GOGC=50 ${GINKGO} run ${GINKGO_ARGS} ${GINKGO_EXTRA_ARGS} "${TEST_BINARIES[@]}"
TEST_RESULT=$?
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) ginkgo run finished in $((SECONDS - run_start))s"

# Match prior ginkgo run behavior: drop the test binaries after the run.
rm -f "${TEST_BINARIES[@]}"

if [ -f "${ARTIFACT_DIR}/test-unit-coverage.out" ]; then
  # Convert the coverage to html for spyglass.
  go tool cover -html=${ARTIFACT_DIR}/test-unit-coverage.out -o ${ARTIFACT_DIR}/test-unit-coverage.html

  # Report the coverage at the end of the test output.
  echo -n "Coverage "
  go tool cover -func=${ARTIFACT_DIR}/test-unit-coverage.out | tail -n 1
  # Blank new line after the coverage output to make it easier to read when there is an error.
  echo
fi

# Ensure we exit based on the test result, coverage results are supplementary.
exit ${TEST_RESULT}
