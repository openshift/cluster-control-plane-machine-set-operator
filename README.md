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

### Creating a ControlPlaneMachineSet for an existing cluster

A ControlPlaneMachineSet can be installed into any existing OpenShift cluster provided it has existing, and `Running`
Control Plane Machines. Typically, this would only be true if the cluster was created using Installer-Provisioned
infrastructure.

Currently, the ControlPlaneMachineSet only supports AWS and Azure.
Support for other platforms is planned in later releases.

> Note: A `Running` Control Plane Machines means the Machine phase is `Running`. By requiring at least 1 `Running`
> Machine we can ensure that the spec of the Machine is valid and that the CPMSO will be able to create new Machines
> based on that template.

The ControlPlaneMachineSet, configured for the OpenShift Machine API, for you cluster will look something like below:

```yaml
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
  name: cluster
  namespace: openshift-machine-api
spec:
  replicas: 3 [1]
  strategy:
    type: RollingUpdate [2]
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: master
      machine.openshift.io/cluster-api-machine-type: master
  template:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      failureDomains:
        platform: <platform> [3]
        <platform failure domains> [4]
      metadata:
        labels:
          machine.openshift.io/cluster-api-machine-role: master
          machine.openshift.io/cluster-api-machine-type: master
          machine.openshift.io/cluster-api-cluster: <cluster-id> [5]
      spec:
        providerSpec:
          value:
            <platform provider spec> [6]
```

1. Replicas is 3 in most cases. Support exceptions may allow this to be 5 replicas in certain circumstances.
Horizontal scaling is not currently supported and so this field is currently immutable.
This may change in a future release.
2. The strategy defaults to `RollingUpdate`. `OnDelete` is also supported.
3. The ControlPlaneMachineSet spreads Machines across multiple failure domains where possible.
This field must be set to the platform name.
4. The failure domains vary on their configuration per platform, see below for how to configure a failure domain on
each platform.
5. The cluster ID is required here. You should be able to find this label on existing Machines in the cluster.
6. The provider spec must match that of the Control Plane Machines created by the installer, except, you can omit any
field set in the failure domains.

Once you have configured the ControlPlaneMachineSet as above, you should be able to install this into the cluster and
observe the status gathered by the CPMSO. Assuming the `providerSpec` and `failureDomains` are configured correctly,
no rollout should occur by default.

#### Configuring a ControlPlaneMachineSet on AWS

AWS supports both the `availabilityZone` and `subnet` in its failure domains.
Gather the existing Control Plane Machines and make a note of the values of both the `availabilityZone` and `subnet`.
Aside from these fields, the remaining spec in the Machines should be identical.

Copy the value from one of the Machines into the `providerSpec.value` (6) on the example above.
Remove the `avialabilityZone` and `subnet` fields from the `providerSpec.value` once you have done that.

For each failure domain you have in the cluster (normally 3-6 on AWS), configure a failure domain like below:
```yaml
- placement:
    availabilityZone: <zone>
  subnet:
    type: Filters
    filters:
    - name: tag:Name
      values:
      - <subnet>
```

The complete `failureDomains` (3 and 4) on the example above should look something like below:
```yaml
failureDomains:
  platform: AWS
  aws:
  - placement:
      availabilityZone: <zone-1>
    subnet:
      type: Filters
      filters:
      - name: tag:Name
        values:
        - <zone-1-subnet>
  - placement:
      availabilityZone: <zone-2>
    subnet:
      type: Filters
      filters:
      - name: tag:Name
        values:
        - <zone-2-subnet>
  - placement:
      availabilityZone: <zone-3>
    subnet:
      type: Filters
      filters:
      - name: tag:Name
        values:
        - <zone-3-subnet>
```

#### Configuring a ControlPlaneMachineSet on Azure

Currently the only field supported by the Azure failure domain is the `zone`.
Gather the existing Control Plane Machines and note the value of the `zone` of each.
Aside from the `zone` field, the remaining in spec the Machines should be identical.

Copy the value from one of the Machines into the `providerSpec.value` (6) on the example above.
Remove the `zone` field from the `providerSpec.value` once you have done that.

For each `zone` you have in the cluster (normally 3), configure a failure domain like below:
```yaml
- zone: "<zone>"
```

With these zones, the complete `failureDomains` (3 and 4) on the example above should look something like below:
```yaml
failureDomains:
  platform: Azure
  azure:
  - zone: "1"
  - zone: "2"
  - zone: "3"
```

> Note: The `internalLoadBalancer` field may not be set on the Azure providerSpec. This field is required for Control
Plane Machines and you should populate this on both the Machines and the ControlPlaneMachineSet spec.

## Contributing

Please review the dedicated [contributing guide](/docs/contributing.md) for code conventions and other pointers on
contributing to this project.

[cpms-enhancement]: https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machine-set.md
[etcd-protection]: https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md
