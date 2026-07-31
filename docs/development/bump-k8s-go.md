# Bumping Kubernetes and Go

This document describes how to bump Kubernetes and Go versions across the
cluster-control-plane-machine-set-operator (CPMSO). It is primarily intended to
be consumed by an AI coding agent (e.g. via `/bump-k8s-go 1.36 1.26`), but the
steps can also be followed manually.

The first argument is the target **Kubernetes minor** version (e.g. `1.36`) and
the second is the target **Go minor** version (e.g. `1.26`).

## Repository context

- **Go workspace:** Uses `go.work` with two modules: root (`.`) and `./openshift-tests-extension`
- **Vendoring:** Uses `go work vendor` via `hack/vendor.sh` (NOT `go mod vendor`)
- **Linting:** golangci-lint vendored, invoked via Makefile
- **Testing:** envtest-based integration tests using OpenShift envtest binary index
- **In payload:** Yes (OCP release payload component)

## Prerequisites

This repository depends on several OpenShift repositories that must be bumped
**before** this one. Verify all prerequisites before proceeding.

### openshift/api

```bash
gh pr list --repo openshift/api --state merged \
  --search "bump k8s ${K8S_MINOR} OR k8s ${K8S_MINOR}" --limit 5
```

Alternatively, check the latest pseudo-version:

```bash
curl -s "https://proxy.golang.org/github.com/openshift/api/@latest"
```

Verify its `go.mod` references `k8s.io/api v0.${K8S_MINOR##1.}.x`.

### openshift/client-go

```bash
curl -s "https://proxy.golang.org/github.com/openshift/client-go/@latest"
```

### openshift/library-go

```bash
curl -s "https://proxy.golang.org/github.com/openshift/library-go/@latest"
```

### openshift/controller-runtime-common

```bash
curl -s "https://proxy.golang.org/github.com/openshift/controller-runtime-common/@latest"
```

### openshift/cluster-api-actuator-pkg

```bash
curl -s "https://proxy.golang.org/github.com/openshift/cluster-api-actuator-pkg/testutils/@latest"
```

### Envtest assets

Check availability in the OpenShift envtest index:

```bash
curl -s "https://raw.githubusercontent.com/openshift/api/master/envtest-releases.yaml" | grep "v${K8S_MINOR}"
```

Use the **highest patch version** available. If no entry exists for the target
k8s minor, **stop** — tests will fail in CI. Wait for assets to be published.

### Prerequisite verification

Confirm each dependency's `go.mod` targets the expected k8s version:

```bash
curl -s "https://proxy.golang.org/github.com/openshift/api/@v/<version>.mod" | grep "k8s.io/api"
curl -s "https://proxy.golang.org/github.com/openshift/client-go/@v/<version>.mod" | grep "k8s.io/client-go"
```

If any prerequisite is not yet bumped, **stop** and wait.

## Step 1: Research

### 1a. Kubernetes patch version

```bash
curl -s "https://proxy.golang.org/k8s.io/api/@v/list" | grep "v0.${K8S_MINOR##1.}" | sort -V | tail -1
```

Call this `K8S_MOD_VERSION` (e.g. `v0.36.3`).

### 1b. controller-runtime version

```bash
curl -s "https://proxy.golang.org/sigs.k8s.io/controller-runtime/@v/list" | sort -V | tail -5
```

The mapping is roughly: controller-runtime v0.24.x → K8s 1.36, v0.23.x → K8s 1.35.

### 1c. controller-tools version

```bash
curl -s "https://proxy.golang.org/sigs.k8s.io/controller-tools/@v/list" | sort -V | tail -5
```

### 1d. OpenShift release mapping

| k8s minor | OCP version |
|-----------|-------------|
| 1.34 | 4.21 |
| 1.35 | 4.22 |
| 1.36 | 5.0 |

### 1e. golangci-lint compatibility

If the K8s bump pulls in newer transitive linter dependencies that conflict with
the current golangci-lint version, you may need to upgrade to golangci-lint v2.
Check if `go mod tidy` fails with package-not-found errors for linter sub-packages.

## Step 2: Update go.mod (root module)

### 2a. Bump everything at once

```bash
GOWORK=off go get \
  k8s.io/api@${K8S_MOD_VERSION} \
  k8s.io/apimachinery@${K8S_MOD_VERSION} \
  k8s.io/client-go@${K8S_MOD_VERSION} \
  k8s.io/component-base@${K8S_MOD_VERSION} \
  k8s.io/apiextensions-apiserver@${K8S_MOD_VERSION} \
  k8s.io/apiserver@${K8S_MOD_VERSION} \
  k8s.io/code-generator@${K8S_MOD_VERSION} \
  k8s.io/kube-aggregator@${K8S_MOD_VERSION} \
  k8s.io/klog/v2@latest \
  k8s.io/kube-openapi@latest \
  sigs.k8s.io/controller-runtime@${CONTROLLER_RUNTIME_VERSION} \
  sigs.k8s.io/controller-tools@${CONTROLLER_TOOLS_VERSION} \
  github.com/openshift/api@latest \
  github.com/openshift/client-go@latest \
  github.com/openshift/library-go@latest \
  github.com/openshift/controller-runtime-common@latest \
  github.com/openshift/cluster-api-actuator-pkg/testutils@latest
```

### 2b. Tidy

```bash
GOWORK=off go mod tidy
```

If tidy fails with missing package errors from `golangci-lint`, see
"golangci-lint v2 migration" below.

## Step 3: Update go.work and openshift-tests-extension module

### 3a. Update go.work

Set `go` directive to match the minimum required version:

```
go ${GO_MINOR}.0
```

### 3b. Update openshift-tests-extension/go.mod

Bump the `go` directive, `sigs.k8s.io/controller-runtime`, and all K8s indirect
dependencies to match the root module:

```bash
cd openshift-tests-extension
# Edit go.mod to update go directive and deps
GOWORK=off go mod tidy
cd ..
```

### 3c. Sync workspace

```bash
go work sync
```

## Step 4: Vendor

**Critical:** Always use `go work vendor`, never `go mod vendor`.

```bash
go work vendor
```

Or use the helper script:

```bash
./hack/vendor.sh
```

## Step 5: golangci-lint v2 migration (if needed)

If the transitive dependency bump conflicts with golangci-lint v1:

### 5a. Update tools.go

```go
_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
```

### 5b. Update go.mod

Replace the v1 require with v2:

```
github.com/golangci/golangci-lint/v2 v2.X.Y
```

### 5c. Update Makefile

```makefile
GOLANGCI_LINT = go run -mod=vendor ${PROJECT_DIR}/vendor/github.com/golangci/golangci-lint/v2/cmd/golangci-lint
```

### 5d. Migrate config

```bash
go run -mod=vendor ./vendor/github.com/golangci/golangci-lint/v2/cmd/golangci-lint migrate --skip-validation
rm -f .golangci.bck.yaml
```

Key v2 config changes applied by the migrator:
- `linters.disable-all: true` → `linters.default: none`
- `linters-settings` → nested under `linters.settings`
- `issues.exclude-rules` → `linters.exclusions.rules`
- `gofmt`/`goimports` → moved to `formatters` section
- `gosimple`+`stylecheck` → merged into `staticcheck`
- Removed: `typecheck`, `dogsled`, `tenv`
- Renamed: `goerr113` → `err113`
- Added: `version: "2"` at top of file

### 5e. Verify config

```bash
go run -mod=vendor ./vendor/github.com/golangci/golangci-lint/v2/cmd/golangci-lint config verify
```

## Step 6: Update Makefile

### ENVTEST_K8S_VERSION

Set to the highest patch version available in the OpenShift envtest index. If
the target K8s version isn't published yet, use the latest available:

```makefile
ENVTEST_K8S_VERSION = X.Y.Z
```

### BUILD_IMAGE (if present)

```makefile
BUILD_IMAGE ?= registry.ci.openshift.org/openshift/release:golang-${GO_MINOR}
```

## Step 7: Fix test failures

### vSphere Infrastructure validation

The `openshift/api` periodically adds new CRD validations. A common pattern:
vSphere `FailureDomains` requiring matching `VCenters` entries.

Fix by adding VCenter specs to test Infrastructure objects:

```go
infrastructure.Spec.PlatformSpec.VSphere.VCenters = []configv1.VSpherePlatformVCenterSpec{
    {
        Server:      "vcenter.test.com",
        Datacenters: []string{"test-dc1", "test-dc2", "test-dc3"},
    },
}
```

### Validation error message format changes

K8s validation messages periodically change format. The type annotation (e.g.
`"object"`, `"integer"`) may be dropped or replaced with the actual value.
Update test assertions to use `ContainSubstring` with the stable portion of
the message rather than exact matches.

### Eventually timeout for controller tests

controller-runtime version bumps may change informer sync timing. If controller
tests timeout with gomega's default `Eventually` (1s), increase the suite
default:

```go
SetDefaultEventuallyTimeout(5 * time.Second)
SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
```

### CRD manifest path changes

When `openshift/api` reorganizes CRD manifest directories, test suites
referencing `CRDDirectoryPaths` may need updates. Check:

- `pkg/webhooks/controlplanemachineset/suite_test.go`
- `pkg/controllers/controlplanemachinesetgenerator/suite_test.go`
- `pkg/machineproviders/providers/suite_test.go`

## Step 8: .gitignore and vendor

The CI `verify-deps` check does NOT respect `.gitignore`. If vendored files are
hidden by gitignore patterns (e.g. `*.swp`), the check will fail:

```gitignore
*.swp
!vendor/**/*.swp
```

Force-add hidden files: `git add -f vendor/path/to/file`

## Step 9: Validate

```bash
go build -mod=vendor ./...
go vet -mod=vendor ./...
KUBEBUILDER_ASSETS="$(go run -mod=vendor ./vendor/sigs.k8s.io/controller-runtime/tools/setup-envtest use ${ENVTEST_K8S_VERSION} -p path --bin-dir ./bin --index https://raw.githubusercontent.com/openshift/api/master/envtest-releases.yaml)" go test -mod=vendor ./pkg/...
```

## Step 10: Commit

Split into separate commits:

1. **Version bump** (go.mod, go.sum, go.work, go.work.sum,
   openshift-tests-extension/, .golangci.yaml, Makefile, tools.go, test fixes):
   ```
   Bump Kubernetes dependencies to ${K8S_MINOR}
   ```

2. **Vendor update** (vendor/ directory only):
   ```
   vendor: update vendored dependencies for K8s ${K8S_MINOR}
   ```

Do NOT push or create a PR unless the user asks.

## CI checks

Key Prow CI jobs to monitor:

| Job | What it checks |
|-----|----------------|
| `ci/prow/vendor` | `go work vendor` output matches committed vendor |
| `ci/prow/unit` | Unit tests (envtest-based) |
| `ci/prow/lint` | golangci-lint |
| `ci/prow/vet` | go vet |
| `ci/prow/generate` | controller-gen output |
| `ci/prow/images` | Container image build |
| `ci/prow/fmt` | gofmt |

## Troubleshooting

### `go.work lists go 1.X but module requires go >= 1.Y`

Update `go.work` to at least the version required by the highest module.

### `unable to find archive for X.Y.Z (linux,amd64)` in CI unit tests

Envtest binaries for that version aren't published. Use the latest available
version in `ENVTEST_K8S_VERSION`.

### `module ... does not contain package ...`

Transitive dependency conflict, typically from golangci-lint v1 conflicting
with newer linter versions. Upgrade to golangci-lint v2 (see Step 5).

### Vendor check finds untracked files

Run `go work vendor` (not `go mod vendor`) and check `.gitignore` isn't hiding
files in `vendor/`.

### golangci-lint config error: `unsupported version of the configuration`

The config file needs `version: "2"` at the top. Run the migrator (Step 5d).

## Previous art

| K8s Version | Epic | Notes |
|-------------|------|-------|
| 1.36 | [OCPCLOUD-3609](https://redhat.atlassian.net/browse/OCPCLOUD-3609) | golangci-lint v1→v2 migration |
| 1.35 | [OCPCLOUD-3284](https://redhat.atlassian.net/browse/OCPCLOUD-3284) | |
