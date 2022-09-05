# Control Plane Machine Set Operator

The Control Plane Machine Set Operator (CPMSO) enables users to automate management of the Control Plane Machine
resources within an OpenShift 4 cluster.

## Overview

The CPMSO is an operator driven by `ControlPlaneMachineSet` resources. Each cluster will have a single control plane machine set in the `openshift-machine-api` namespace. The control plane machine set will be called `cluster`.

For clusters created before OpenShift version 4.12, this resource may not exist, though it can be created post-
installation. For clusters created from OpenShift version 4.12 onwards, this resource may exist, dependent on the
platform.

The `ControlPlaneMachineSet` resource should look something like below:

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
Alternatively, it can be found on the infrastructure resource: `oc get -o jsonpath='{.status.infrastructureName}{"\n"}' infrastructure cluster`
6. The provider spec must match that of the Control Plane Machines created by the installer, except, you can omit any
field set in the failure domains.

## Features

### Spreading machines across Failure Domains

The control plane machine set can balance machines across multiple failure domains. This provides high availability and
fault tolerance within the control plane when issues arise within the infrastructure provider.

Failure domains are configured within the machine template of the control plane machine set and are currently supported
on Amazon Web Services (AWS) and Microsoft Azure only. Other platforms will be added over time.

The control plane machine set will balance the machines across the failure domains provided, weighting towards the
alphabetically first failure domains when a failure domain must be re-used.
When the list of failure domains is changed within the control plane machine set, you may see a rollout as the
control plane machine set rebalances the control plane machines.

### Automated replacement of machine infrastructure

The control plane machine set constantly monitors the control plane machines within the cluster and compares their
configured specification with that of the machine template embedded within it.
When a machine is considered to need an update (either the specification differs or it has been deleted), the control
plane machine set will replace the machine with an updated instance based on the update strategy defined within the
control plane machine set spec.

### Integration with machine health check

As the control plane machine set can now create replacement machines, control plane machines may be targeted by a
machine health check.
It is advised to set the machine health check `maxUnhealthy` value to `1` when configuring machine health checks with
control plane machines as a target.
Any machine health check targeting control plane machines should target only control plane machines and should not be
used to also cover worker machines.

Note: If the machine health check deletes a machine hosting etcd, and the etcd member is not reachable, manual
intervention is currently required to restore the cluster state. Remove all `lifecycleHooks` from the deleted machine
to force the etcd operator to remove the failed member from the cluster. At this point it can safely add new members.

## Limitations

### Horizontal scaling

The control plane machine set does not currently support horizontal scaling of the control plane.
This means that the replicas value of the spec is immutable once created.

When creating a new control plane machine set the operator will perform safety checks and ensure that the number of
control plane machines in the cluster matches the number of replicas defined in the spec.
Should this validation fail the creation of the control plane machine set will be rejected.

Please ensure that you have 3 (or 5) control plane machines before creating the control plane machine set.

### Supported platforms

The control plane machine set is currently only supported on Amazon Web Services (AWS) and Microsoft Azure.
Other platforms may be supported at a later date.

Google Cloud Platform and OpenStack are planned for inclusion from OpenShift version 4.13 onwards.

## Installation

Installation instructions for control plane machine set can be found in the [installation docs](./installation.md).

## Glossary

### Failure Domain

A failure domain defines a fault boundary within the infrastructure provider. Failure domains are also known as zones
or availability zones depending on the infrastructure provider.

### Index

Each control plane machine is considered to exist within an index of the control plane.
This is a concept internal to the control plane machine set that allows the operator to define behaviour such as
replacing a machine in an index with a machine that exists in the same availability zone.
It also allows the control plane machine set to track replacements of machines as two machines in the same index
indicates that a replacement is in progress.

The index of each machine is typically the last digit of the machine name.
Control plane machines are indexed from 0, so are typically indexed as 0, 1 and 2 (and 3 and 4 in the case of a 5
member control plane).

The control plane machine set replaces machines index by index in ascending order, therefore, when an update is in
progress, you may see multiple machines in the same index. The newer machine is created to replace the older machine.

The failure domain of an index should be stable through the lifetime of a cluster unless additional failure domains
are added, or failure domains are removed from the machine template.

### Update strategy

Update strategies define how the control plane machine set behaves when it identifies that a machine needs to be
replaced.

There are currently two supported strategies, `RollingUpdate` (default) and `OnDelete`.

The strategies are explained in more detail in the [update strategy docs](./update-strategies.md).
