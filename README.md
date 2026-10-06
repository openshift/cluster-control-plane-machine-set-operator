# Cluster Control Plane Machine Set operator

The Cluster Control Plane Machine Set Operator (CPMSO) is responsible for the lifecycle of Control Plane Machines within
an OpenShift High Availability cluster, sometimes know as an IPI or UPI cluster.

The operator is configured using a ControlPlaneMachineSet custom resource which is a singleton within the cluster.
It defines the desired number of Control Plane Machines and their specification. From there, the CPMSO can maintain
the state of the Control Plane and, when required, perform updates by replacing the Machines within the Control Plane
in a safe and controlled manner, respecting the principles of immutable infrastructure.

## Background

The ControlPlaneMachineSet ([enhancement][cpms-enhancement]) is intended to enable vertical scaling of Control Plane
Machines through a safe and controlled rolling update strategy.
Currently, horizontal scaling of the Control Plane is not supported and any updates to the replica count of the
Control Plane will be rejected.

The ControlPlaneMachineSet compares the existing specification of each Control Plane Machine with that of the desired
specification defined with the ControlPlaneMachineSet `spec.template`. Should any Machine not meet the desired
specification, this Machine is considered to need an update and the CPMSO will then attempt to update it by replacing it
based on the update strategy.

There are two update strategies supported by the ControlPlaneMachineSet, these are `RollingUpdate` and `OnDelete`.
The `RollingUpdate` strategy is an automatic rolling update similar to that of a Deployment within Kubernetes.
This option is suitable for most deployments of the ControlPlaneMachineSet.

The alternative, `OnDelete` is a semi-automated replacement which requires Admin intervention to trigger the replacement
of each Machine. When a Control Plane Machine is deleted by the Admin, the ControlPlaneMachineSet will then replace it
and handle the migration of the workloads from the deleted Machine onto the new Machine.
The `OnDelete` strategy can be used to have more control over the rollout of updates should there be concerns about the
changes from the Admin making the changes.

A key part of updating the Control Plane Machines is ensuring that the etcd cluster quorum is protected.
The ControlPlaneMachineSet is not responsible for the quorum of etcd and the etcd operator manages this using its own
[protection mechanism][etcd-protection].
The protection mechanism ensures that Machine API cannot remove a Machine from the cluster until the etcd member on it
has been migrated onto a new Machine.

## Development

#### Prerequisites

* Go language 1.18+
* GNU make

Standard development tasks can be performed using the [Makefile](/Makefile) in the root of the repository.
Tooling is vendored and executed using `go run` so no additional tooling should be needed for these Make targets.

### Common targets

* `make build`: Build the operator binary into `bin/manager`
* `make test`: Run the project tests. Tests are written using ginkgo and the ginkgo tooling is used to execute the
tests. In CI, this target outputs JUnit and code coverage reports. Depends on `generate`, `fmt` and `vet` tasks.
* `make unit`: Run only the project tests, without the `generate`, `fmt`, or `vet` tasks. The `GINKGO_EXTRA_ARGS`
environment variable can be used to pass more options to the ginkgo runner.
* `make lint`: Runs `golangci-lint` based on the project linter configuration. It is recommend to run this target before
committing any code changes.
* `make vendor`: Update the vendor directory when there are changes to the `go.mod` file. Runs tidy, vendor and verify.

### Project Layout

The project follows a conventional Go project layout with [cmd](/cmd) and [pkg](/pkg) folders.

The business logic for the project is divided into 4 key areas:
- [pkg/controllers](/pkg/controllers): The main controllers of the CPMSO, handling the core functionality of gathering
status information and handling update decisions.
- [pkg/machineproviders](/pkg/machineproviders): Machine abstractions responsible for handling all specifics for the
Machine backend. Responsible for gathering data about and creating and deleting Machines.
- [pkg/test](/pkg/test): A series of test utilities used within the test suites in the project.
- [pkg/webhooks](/pkg/webhooks): The validation webhook implemented here validates the ControlPlaneMachineSet resource
on create and update operations with a cluster.

More detail about these areas can be found within the code comments.

## Deployment

The CPMSO is deployed as a part of the core OpenShift payload from OpenShift 4.12 onwards.
It is deployed based on the [`manifests`](/manifests) defined in this repository.

### Installation within an existing cluster

Installation instructions for control plane machine set can be found in the [installation docs](./docs/user/installation.md).

### Serving TLS configuration

The HTTPS metrics endpoint and admission webhook use the cluster's
`apiservers.config.openshift.io/cluster` TLS security profile when `spec.tlsAdherence` is
`StrictAllComponents`. Otherwise, they use the default Intermediate profile.
Changes to the profile or adherence policy trigger a graceful operator restart to reload the configuration.

With the `TLSGroupPreferences` feature gate enabled, a custom profile can specify key-exchange groups, for example:

```yaml
spec:
  tlsAdherence: StrictAllComponents
  tlsSecurityProfile:
    type: Custom
    custom:
      minTLSVersion: VersionTLS13
      ciphers:
        - TLS_AES_128_GCM_SHA256
      groups:
        - secp256r1
        - secp384r1
```

Groups restrict the allowed key-exchange algorithms for TLS 1.2 and TLS 1.3; Go chooses its own negotiation order.
Supported groups include classical curves and ML-KEM hybrids supported by the operator's Go runtime.
When a custom profile omits groups, Go's curve defaults are retained. Built-in profiles use their predefined groups.
Unsupported cipher or group names are logged and ignored. If no supported groups remain, curve defaults are retained.
TLS 1.3 cipher suites are not configurable in Go, independently of group configuration.

The existing `--tls-min-version` and `--tls-cipher-suites` flags must still be provided together.
The optional `--tls-curve-preferences` flag accepts comma-separated numeric Go `crypto/tls.CurveID` values,
for example `--tls-curve-preferences=23,24` allows P-256 and P-384. IDs must be unique and supported by the Go runtime.

Curve preferences may be supplied alone or alongside the minimum-version/cipher-suite pair.
**Any TLS CLI override bypasses the entire cluster profile and disables the profile watcher**; it is not merged
with cluster settings. A curve-only override uses a TLS 1.2 minimum and Go's default cipher suites.
When the curve flag is omitted from an existing CLI override, Go's curve defaults are retained.
Go chooses its own negotiation order from the allowed curve list.

## Contributing

Please review the dedicated [contributing guide](/docs/contributing.md) for code conventions and other pointers on
contributing to this project.

[cpms-enhancement]: https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machine-set.md
[etcd-protection]: https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md
