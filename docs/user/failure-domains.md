# Failure Domains

Failure domains, within the control plane machine set, are used to spread the control plane machines out through
the infrastructure failure domain/availability zones and are intended to be used to provide some fault tolerance should
there be an issue within the backend infrastructure.

## How are failure domains used within the control plane machine set?

Failure domains are used within the control plane machine set when gathering information about, and creating new
machines.

While processing the control plane machine set, the controller, when defined, will map the failure domains to the
existing machines based on their index. This mapping is then used to calculate whether or not a particular control plane
machine requires an update.

The mapped failure domain for the index is injected into the control plane machine set's machine template provider spec.
The resulting provider spec is then compared with the existing control plane machine's provider spec.
When a difference is detected, the machine is considered to need an update.

It is important to map the failure domains based on the existing machines to prevent unnecessary updates.

The mapping logic stabilises, within a given index, the failure domain unless there is an imbalance within the mapping.
For example, if the cluster was originally created using only a single failure domain, and later additional failure
domains were created, the control plane machine set will move one or more indexes over to the new failure domain(s) to
ensure appropriate fault tolerance. Using each of the failure domains equally where possible.

## What happens if I don't provide any failure domains?

When no failure domains are configured, the control plane machine set assumes that all control plane machines should
be within a single failure domain, and, that this failure domain is already configured within the provider spec within
the control plane machine set's template provider spec.

If failure domains are added at a later date, the control plane machine set will attempt to rebalance the control plane
machines across the newly added failure domains.

## Amazon Web Services (AWS)

On Amazon Web Services (AWS), the failure domains represented in the control plane machine set can be considered to be
analogous to the availability zones within an AWS region

> An Availability Zone (AZ) is one or more discrete data centers with redundant power, networking, and connectivity in
an AWS Region. AZs give customers the ability to operate production applications and databases that are more highly
available, fault tolerant, and scalable than would be possible from a single data center.

Amazon describes the availability zones as a discrete data center, providing a more fault tolerant deployment than a
single data center.

When configuring AWS failure domains in the control plane machine set, there are currently two options that must be
selected; the availability zone name itself, and the subnet to use.

AWS subnet configuration is linked to the availability zone, so the control plane machine set needs to know which
subnet to use when creating a machine in the availability zone.

An AWS failure domain will look something like the example below:
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

## Microsoft Azure

On Microsoft Azure, the failure domains represented in the control plane machine set can be considered analogous to the
Azure availability zones.

> Azure availability zones are physically separate locations within each Azure region that are tolerant to local
failures. Failures can range from software and hardware failures to events such as earthquakes, floods, and fires.
Tolerance to failures is achieved because of redundancy and logical isolation of Azure services. To ensure resiliency,
a minimum of three separate availability zones are present in all availability zone-enabled regions.

Azure describes the availability zones as separate locations within each region, providing more tolerance to local
failures.

When configuring Azure failure domains in the control plane machine set, there are currently two options that must be
selected; the availability zone name, and the subnet to use.

An Azure failure domain will look something like the example below:
```yaml
- zone: "<zone>"
  subnet: "<subnet>"
```

## OpenStack

On OpenStack, the failure domains represented in the control plane machine set
include the OpenStack Nova availability zone for instance placement, as well as
the Cinder availability zone and the volume type for root volume placement.

> OpenStack Availability Zones are an end-user visible logical abstraction for partitioning an OpenStack cloud without
> knowing the physical infrastructure. They are used to partition a cloud on arbitrary factors, such as location (country, datacenter, rack),
> network layout and/or power source.
> Compute (Nova) Availability Zones are presented under Host Aggregates and can help to group the compute nodes
> associated with a particular Failure Domain.
> Storage (Cinder) Availability Zones are presented under Availability Zones and help to group the storage backend types by Failure Domain.
> Depending on how the cloud is deployed, a storage backend can expand across multiple Failure Domains or be limited to a single Failure Domain.
> The name of the Availability Zones depend on the cloud deployment and can be retrieved from the OpenStack administrator.

An OpenStack failure domain will look something like the example below:
```yaml
- availabilityZone: "<nova availability zone>"
  rootVolume:
    availabilityZone: "<cinder availability zone>"
    volumeType: "<cinder volume type>"
```

## vSphere

On vSphere, the failure domains are represented by the infrastructure resource spec. A vSphere failure domain
represents a combination of network, datastore, compute cluster, and datacenter. This allows an administrator
to deploy machines in to separate hardware configurations.

A vSphere failure domain will look something like the example below in the infrastructure resource:
```yaml
  spec:
    cloudConfig:
      key: config
      name: cloud-provider-config
    platformSpec:
      type: VSphere
      vsphere:
        failureDomains:
        - name: us-east-1
          region: us-east
          server: vcs8e-vc.ocp2.dev.cluster.com
          topology:
            computeCluster: /IBMCloud/host/vcs-mdcnc-workload-1
            datacenter: IBMCloud
            datastore: /IBMCloud/datastore/mdcnc-ds-1
            networks:
            - ci-vlan-1289
            resourcePool: /IBMCloud/host/vcs-mdcnc-workload-1/Resources
          zone: us-east-1a
        - name: us-east-2
          region: us-east
          server: vcs8e-vc.ocp2.dev.cluster.com
          topology:
            computeCluster: /IBMCloud/host/vcs-mdcnc-workload-2
            datacenter: IBMCloud
            datastore: /IBMCloud/datastore/mdcnc-ds-2
            networks:
            - ci-vlan-1289
            resourcePool: /IBMCloud/host/vcs-mdcnc-workload-2/Resources
```

The control plane machine set for vSphere refers to failure domains by their name as defined in the infrastructure
spec. vSphere failure domains defined in the control plane machine set will look something like the example below:
```yaml
  template:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      failureDomains:
        platform: VSphere
        vsphere:
        - name: us-east-1
        - name: us-east-2
```