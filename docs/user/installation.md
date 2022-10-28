# Installation

This document provides instructions for users to configure a control plane machine set within an OpenShift cluster.
This process can be followed on OpenShift clusters that are version 4.12 or higher.

A `ControlPlaneMachineSet` can be installed on a [supported platforms](./README.md#supported-platforms) provided it has existing, and `Running`
control plane machines.
Typically, this would only be true if the cluster was created using Installer-Provisioned infrastructure.

> Note: A `Running` control plane machine means the machine is in the `Running` phase. By requiring at least 1 `Running`
> machine we can ensure that the spec of the machine is valid and that the control plane machine set will be able to create new machines
> based on that template.

In order to understand what path to take for installing the control plane machine set into the cluster:
1. check [supported platforms](./README.md#supported-platforms) to understand the type of support for the cluster
1. depending on the type of support, follow the corresponding steps:
  - **`Full`**: this cluster combination is supported. If the cluster was born in this version, read [pre-installed](#pre-installed). If the cluster was upgraded into this version, follow the steps for [installation into an existing cluster with a generated resource](#installation-into-an-existing-cluster-with-generated-resource).
  - **`Manual`**: this cluster combination is supported. The `ControlPlaneMachineSet` resource must be manually created and applied. Follow the steps described for [installation into an existing cluster with a manual resource](#installation-into-an-existing-cluster-with-manual-resource).
  - **`Not Supported`**: this cluster combination is not yet supported.

### Pre-installed

For clusters born (installed) with a version/platform combination highlighted as `Full` in the [supported platforms](./README.md#supported-platforms),
the installer provisioned infrastructure (IPI) installer workflow will create a
control plane machine set and set it to `Active`.

No further action is required by the user in this case.

This can be checked by using the following command:
```
oc get controlplanemachineset.machine.openshift.io cluster --namespace openshift-machine-api
```

### Installation into an existing cluster with a generated resource

In this configuration the control plane machine set may already exist in the cluster.

Its state can be checked by using the following command:
```
oc get controlplanemachineset.machine.openshift.io cluster --namespace openshift-machine-api
```

If `Active`, there is nothing to do, as the control plane machine set
has already been activated by a cluster administrator and is operational.

If `Inactive`, the control plane machine set can be activated.
Before doing so, the control plane machine set spec must be thoroughly reviewed to ensure
that the generated spec aligns with the desired specification.

Consult the [anatomy of a ControlPlaneMachineSet resource](#anatomy-of-a-controlplanemachineset)
as a reference for understanding the fields and values within a `ControlPlaneMachineSet` resource.

The generated control plane machine set can be reviewed with the following command:
```
oc --namespace openshift-machine-api edit controlplanemachineset.machine.openshift.io cluster
```

If any of the fields do not match with the expected value, the value may be changed, provided that the edit is done in the
same `oc edit` session where the control plane machine set is activated.

Once the spec of the control plane machine set has been reviewed, activate the control plane machine set by setting the `.spec.state` field to `Active`.

Once activated, the `ControlPlaneMachineSet` operator should start the reconciliation of the resource.

### Installation into an existing cluster with manual resource

The control plane machine set may not exist in the cluster (unless a cluster administrator has created one already),
but it can be manually created and activated.

This can be checked by using the following command:
```
oc get controlplanemachineset.machine.openshift.io cluster --namespace openshift-machine-api
```

To manually create a control plane machine set define a `ControlPlaneMachineSet` resource as described in the [anatomy of a ControlPlaneMachineSet resource](#anatomy-of-a-controlplanemachineset).

## Anatomy of a ControlPlaneMachineSet

The `ControlPlaneMachineSet` resource should look something like below:

```yaml
apiVersion: machine.openshift.io/v1
kind: ControlPlaneMachineSet
metadata:
  name: cluster
  namespace: openshift-machine-api
spec:
  state: Active [1]
  replicas: 3 [2]
  strategy:
    type: RollingUpdate [3]
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: master
      machine.openshift.io/cluster-api-machine-type: master
  template:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      failureDomains:
        platform: <platform> [4]
        <platform failure domains> [5]
      metadata:
        labels:
          machine.openshift.io/cluster-api-machine-role: master
          machine.openshift.io/cluster-api-machine-type: master
          machine.openshift.io/cluster-api-cluster: <cluster-id> [6]
      spec:
        providerSpec:
          value:
            <platform provider spec> [7]
```

1. The state defines whether the ControlPlaneMachineSet is Active or Inactive.
When `Inactive`, the control plane machine set will not take any action on the state of the control plane machines within the cluster.
The operator will monitor the state of the cluster and keep the `ControlPlaneMachineSet` resource up to date.
When `Active`, the control plane machine set will reconcile the control plane machines and will update them as necessary.
Once `Active`, a control plane machine set cannot be made `Inactive` again.
2. Replicas is 3 in most cases. Support exceptions may allow this to be 5 replicas in certain circumstances.
Horizontal scaling is not currently supported and so this field is currently immutable.
This may change in a future release.
3. The strategy defaults to `RollingUpdate`. `OnDelete` is also supported.
4. The ControlPlaneMachineSet spreads Machines across multiple failure domains where possible.
This field must be set to the platform name.
5. The failure domains vary on their configuration per platform, see [configuring provider specific fields](#configuring-provider-specific-fields) for how to configure a failure domain on each platform.
6. The cluster ID is required here. You should be able to find this label on existing Machines in the cluster.
Alternatively, it can be found on the infrastructure resource: `oc get -o jsonpath='{.status.infrastructureName}{"\n"}' infrastructure cluster`
7. The provider spec must match that of the Control Plane Machines created by the installer, except, you can omit any
field set in the failure domains.


### Configuring provider specific fields

The following instructions describe how the failure domains and providerSpec fields should be
constructed depending on the platform of the cluster.

#### Configuring a control plane machine set on Amazon Web Services (AWS)

AWS supports both the `availabilityZone` and `subnet` in its failure domains.
Gather the existing control plane machines and make a note of the values of both the `availabilityZone` and `subnet`.
Aside from these fields, the remaining spec in the machines should be identical.

Copy the value from one of the machines into the `providerSpec.value` (6) on the example above.
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

#### Configuring a control plane machine set on Microsoft Azure

Currently the only field supported by the Azure failure domain is the `zone`.
Gather the existing control plane machines and note the value of the `zone` of each.
Aside from the `zone` field, the remaining in spec the machines should be identical.

Copy the value from one of the machines into the `providerSpec.value` (6) on the example above.
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

> Note: The `internalLoadBalancer` field may not be set on the Azure providerSpec. This field is required for control
plane machines and you should populate this on both the Machine and the ControlPlaneMachineSet resource specs.
