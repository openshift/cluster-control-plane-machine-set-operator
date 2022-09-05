# Installation

This document provides instructions for users to configure a control plane machine set within an OpenShift cluster.
This process can be followed on OpenShift clusters that are version 4.12 or higher.

## Installer based installation

From OpenShift version 4.12 onwards, the installer provisioned infrastructure (IPI) installer workflow will create a
ControlPlaneMachineSet resource for newly installed clusters depending on the platform.

As of 4.12, the only supported platform is Amazon Web Services (AWS). Support for other platforms will be added in
future releases.

## Installation into an existing cluster

### Creating a control plane machine set for an existing cluster

A control plane machine set can be installed into any existing OpenShift cluster provided it has existing, and `Running`
control plane machines.
Typically, this would only be true if the cluster was created using Installer-Provisioned infrastructure.

> Note: A `Running` Control Plane Machines means the Machine phase is `Running`. By requiring at least 1 `Running`
> Machine we can ensure that the spec of the Machine is valid and that the CPMSO will be able to create new Machines
> based on that template.

The `ControlPlaneMachineSet` resource, configured for the OpenShift Machine API, for your cluster will look something
like below:

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

Once you have configured the `ControlPlaneMachineSet` resource as above, you should be able to install this into the
cluster and observe the status gathered by the CPMSO. Assuming the `providerSpec` and `failureDomains` are configured
correctly, no rollout should occur by default.

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
