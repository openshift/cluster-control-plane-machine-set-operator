# Control Plane Machine Set Operator

The Control Plane Machine Set Operator (CPMSO) enables users to automate management of the Control Plane Machine
resources within an OpenShift 4 cluster.

## Installation

Installation instructions for control plane machine set can be found in the [installation docs](./installation.md).

## Overview

The CPMSO is an operator driven by `ControlPlaneMachineSet` resources. Each cluster will have a single control plane machine set in the `openshift-machine-api` namespace. The control plane machine set will be called `cluster`.

The control plane machine set can be in one of the following two states, `Active` or `Inactive` .
The `state` is defined on the control plane machine set's spec.

This state determines the behavior of the control plane machine set.

- when `Inactive`, the control plane machine set will not take any action on the state of the control plane machines within the cluster.
The operator will monitor the state of the cluster and keep the `ControlPlaneMachineSet` resource up to date.

- when `Active`, the control plane machine set will reconcile the control plane machines and will update them as necessary.

Once `Active`, a control plane machine set cannot be made `Inactive` again.
To prevent further action on the control plane machines, users may remove the `ControlPlaneMachineSet` resource from the cluster,
using the following command:
```
oc delete controlplanemachineset.machine.openshift.io --namespace openshift-machine-api cluster
```

Once the control plane machine set operator has performed the necessary clean up operations to disable the control plane machine set, the `ControlPlaneMachineSet` resource will be removed from the cluster.
A new `Inactive` control plane machine set will be created in its place which may be activated later should the user desire.

An overview of a `ControlPlaneMachineSet` resource manifest can be found in
[anatomy of a ControlPlaneMachineSet resource](installation.md#anatomy-of-a-controlplanemachineset).

## Features

### Spreading machines across Failure Domains

The control plane machine set can balance machines across multiple failure domains. This provides high availability and
fault tolerance within the control plane when issues arise within the infrastructure provider.

Failure domains are configured within the machine template of the control plane machine set and are currently supported
by all the [supported platforms](./README.md#supported-platforms) unless otherwise specified in the support matrix. Other platforms will be added over time.

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

The control plane machine set is currently supported for a number of platforms and OpenShift versions.
The matrix shows in detail the support for each specific combination.

| Platform \ OpenShift version |      <=4.11    |      4.12           |      4.13           |      4.14           |
|------------------------------|:---------------|:--------------------|:--------------------|:--------------------|
| AWS                          |  Not Supported | Full                | Full                | Full                |
| Azure                        |  Not Supported | Manual              | Full                | Full                |
| GCP                          |  Not Supported | Not Supported       | Full                | Full                |
| OpenStack                    |  Not Supported | Not Supported       | Not Supported       | Full                |
| VSphere                      |  Not Supported | Manual (Single Zone)| Manual (Single Zone)| Manual (Single Zone)|
| Other Platforms              |  Not Supported | Not Supported       | Not Supported       | Not Supported       |

#### Keys

`Full`: The control plane machine set is fully supported for this combination.\
`Manual`: The control plane machine set is supported for this combination and has to be manually configured and installed.\
`Manual (Single Zone)`: The same as `Manual`, however for this combination only a single failure domain configuration is supported. The failure domain configuration must be embedded within the providerSpec and may not vary between control plane machine indexes.\
`Not Supported`: The control plane machine set is not yet supported for this combination.

For more details on how to install the  control plane machine set for specific combinations, check [installation docs](./installation.md).

## Glossary

### Failure Domain

A failure domain defines a fault boundary within the infrastructure provider. Failure domains are also known as zones
or availability zones depending on the infrastructure provider.

Failure domains are explained in more detail in the [failure domains docs](./failure-domains.md).

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

## FAQs

### How does a control plane machine set differ from a machine set?

Control plane machine sets and machine sets are separate custom resources defined within the OpenShift Machine API.

Machine sets are used for worker machines within OpenShift. They create a number of identical machines and are
responsible for ensuring that, based on the desired replica count, the cluster always has a sufficient number of
machines matching the machine set's template provider spec.

Notably, machine sets should not be used with control plane machines and have no mechanism for updating machines if the
desired provider spec changes.

Control plane machine sets on the other hand, are intended for use specifically with control plane machines.
Like a machine set, they keep a desired number of machines matching the desired provider spec.

Unlike machine sets, control plane machine sets have the ability to spread machines across failure domains.
This enables one control plane machine set to manage all control plane machines, while providing fault tolerance should
the backend infrastructure degrade for some reason.

Failure domains are explained in more detail in the [failure domains docs](./failure-domains.md).

Control plane machine sets also provide the ability to update the control plane machines when the desired provide spec
is changed. These are achieved through the `RollingUpdate` and `OnDelete` update strategies.
This allows users a 1-click, opinionated upgrade process for changing their control plane machines backend
infrastructure, for example, this allows vertically scaling the control plane machines in an automated fashion.

The strategies are explained in more detail in the [update strategy docs](./update-strategies.md).

The control plane machine set adds these features on top of machine sets due to the nature of the control plane machines
having additional requirements as a part of their update process that mean they should be managed under one entity and
not several machine sets.

### How does this fit into the Cluster API ecosystem?

Within Cluster API, a concept exists known as a control plane provider. This component, currently with a single
upstream reference implementation based on KubeADM, is intended to instantiate and manage a control plane within for
the Kubernetes guest cluster.

The control plane provider is responsible not only for creating the infrastructure for the control plane machines but
also etcd and the control plane Kubernetes components (API server, Controller Manager, Scheduler, Cloud Controller
Manager).
Within OpenShift, various different operators implement the management and responsibility of these components, however,
to date we do not have a machine infrastructure operator that fits this role.

In a future version of the control plane machine set, we expect to have it fulfil this role within the Cluster API
project.
A new template version will be introduced to allow the control plane machine set to use Cluster API as its backend.
