# ControlPlaneMachineSet testing strategy

This document aims to outline the test strategy for the ControlPlaneMachineSet operator.

It is assumed that, when a cluster is created, a valid CPMS will be installed and Active. We will modify and remove CPMS objects during the course of these tests.

The work to do from this document is covered by [OCPCLOUD-1734](https://issues.redhat.com/browse/OCPCLOUD-1734).

Test levels are defined as:

* Unit: Testing using mocks/input that is locally built
* Integration: Testing using a framework such as Envtest to simulate a real environment
* E2E: Testing within a fully-fledged OpenShift cluster

## PreSubmit Test cases

The following test cases are intended to be executed on PRs to the ControlPlaneMachineSet codebase.

The tests here should ideally be relatively short and inexpensive to run as they will be executed frequently.

<table>
  <tr>
   <td><strong>Case Number</strong></td>
   <td><strong>Description</strong></td>
   <td><strong>Behaviour under test</strong></td>
   <td><strong>Desired Test Level</strong></td>
   <td><strong>Motivation</strong></td>
   <td><strong>Notes</strong></td>
   <td><strong>Existing coverage</strong></td>
  </tr>
  <tr>
   <td>1</td>
   <td>Existing ControlPlaneMachineSet can be removed from the cluster, CPMS uninstalls successfully</td>
   <td>Deletion logic should be tested
<p>
Check that CPMS removes owner references and that the Machines do not get removed.
<p>
Check that the finalizer is removed correctly.
   </td>
   <td>E2E, Integration</td>
   <td>This is already tested with an integration test, but we should also check this in E2E as it is cheap (assuming it works, no machine changes) and may pick up weird interactions with other components.
<p>
Eg in integration we have no GC running.
   </td>
   <td>Once this is completed, we need to reinstall the CPMS, the CPMS generator will be tested here as well as we will have to check the status is as expected before re-activating the CPMS.</td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>2</td>
   <td>When a CPMS is removed from the cluster, the generator will create a new CPMS.
<p>
This new CPMS should be valid and should be able to be activated without any changes happening to the cluster.
   </td>
   <td>Check that the CPMS generator can gather the appropriate information to create a new CPMS once we have removed the previous one.
<p>
Ensure that the generated CPMS is in an Inactive state
   </td>
   <td>E2E, Integration</td>
   <td>This is integration tested already but will be required to allow other tests.
<p>
Having an Inactive CPMS allows us to set up test cases and then see how the CPMS will react.
<p>
Eg we can observe the status and then activate the CPMS to allow it to react.
   </td>
   <td>This is closely tied to case #1, they should be run together in an ordered container
<p>
If we make any modifications to test the output of the CPMS status, we must not modify the newest machine else the spec of the CPMS will be updated.
   </td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>3</td>
   <td>When the newest Machines spec is changed, the generated CPMS is updated</td>
   <td>If we introduce a new Machine (we won’t, but we can update the spec of the newest machine), then the generator should update the existing CPMS to reflect the updated spec</td>
   <td>E2E, Integration</td>
   <td>This is cheap to run in E2E and can be checked easily. Maybe not needed as already in integration tests?</td>
   <td>Must reset the Machine after we have updated it and checked the expectations.
<p>
Must run after case #2.
   </td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>4</td>
   <td>When activated, the generated CPMS should not cause any rollout</td>
   <td>The CPMS generated should be correct, therefore no RollingUpdate action should be triggered</td>
   <td>E2E</td>
   <td>We need to be able to reactivate the CPMS to handle later tests.
<p>
This is difficult integration test as we don’t run the generator and CPMS core controller together in our integration suite today. (There’s also no MAPI running so it’s not a real test).
   </td>
   <td>Must run after case #2.</td>
   <td>E2E</td>
  </tr>
  <tr>
   <td>5</td>
   <td>When a machine doesn’t match the spec, and the strategy is RollingUpdate, it gets replaced</td>
   <td>CPMS should detect a drift in the desired spec, and replace the Machine.
<p>
Can check naming expectations, correct Machine spec.
<p>
Etcd protection and deletion hooks tested here.
   </td>
   <td>E2E, Integration</td>
   <td>We should check that the entire cluster behaves when a Machine is being replaced.
<p>
This will test the protection mechanism and that other control plane components tolerate some churn.
<p>
We already have some coverage of this at the integration test level.
<p>
We need to make sure that load balancer attachment is working correctly when running this as an E2E test. See <a href="#checking-load-balancer-connectivity">Checking Connectivity</a> below for a plan for this.
   </td>
   <td>At the end of this test, should make sure all cluster operators settle down and the cluster stabilises before executing other tests.</td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>6</td>
   <td>When the CPMS spec is updated, and the strategy is rolling update, all machines get replaced, surge limits are observed</td>
   <td>CPMS should detect the drift in the desired spec and start the rolling update.
<p>
It should observe the surge limits set out and only replace one Machine at a time.
   </td>
   <td>Integration</td>
   <td>We don’t need to test replacing 3 machines in a real cluster, the surge limiting behaviour can be tested using an integration test where we simulate removing and creating Machines in a controlled environment.
<p>
Doing this in E2E is expensive and takes approximately an hour to run through on AWS, possibly longer on other platforms.
<p>
On the other hand, we must ensure that load balancer attachment is correct (ie is the Machine receiving traffic from the internal load balancer). The easy way to do this would be to replace all machines and see if the cluster falls over.
<p>
See <a href="#checking-load-balancer-connectivity">Checking Connectivity</a> below for a plan for this.
   </td>
   <td>We already have some small integration tests that test a subset of this but some longer tests that run through actually removing the machines and watching the operator loop through a whole RollingUpdate would also be good.
   </td>
   <td>Integration</td>
  </tr>
  <tr>
   <td>7</td>
   <td>When the machine doesn’t match the spec, and the strategy is OnDelete, the machine does not get replaced</td>
   <td>CPMS should detect the drift and update the status, but should not replace the Machine</td>
   <td>E2E, Integration</td>
   <td>We already have this at the integration level but this is fairly cheap to test at the E2E level as well and is closely related/required for test case #8</td>
   <td>Should be run in an ordered container with case #8</td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>8</td>
   <td>When the machine doesn’t match the spec and the strategy is OnDelete, and the Machine is deleted, it gets replaced</td>
   <td>CPMS should replace the machine once it has been marked for deletion</td>
   <td>E2E, Integration</td>
   <td>We already have this at the integration level, but we should test this in E2E as well. It differs from the RollingUpdate case because the old Machine is in the deleting phase the whole way through, so this exercises the etcd protection mechanism in a different way.
<p>
We need to make sure that load balancer attachment is working correctly when running this as an E2E test. See <a href="##checking-load-balancer-connectivity">Checking Connectivity</a> below for a plan for this.
   </td>
   <td>Should be run ordered with #7. Technically not required to have the spec differ but that would be the normal case.</td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>9</td>
   <td>RollingUpdate strategy should replace a Machine when it is deleted but did not need an updated</td>
   <td>If a Machine is deleted, CPMS should always replace it to restore the cluster health</td>
   <td>Integration</td>
   <td>Case #5 covers RollingUpdate replacements at the E2E level.
<p>
This can be checked at the integration level succinctly.
   </td>
   <td></td>
   <td>Integration</td>
  </tr>
  <tr>
   <td>10</td>
   <td>OnDelete strategy should replace a Machine when it is deleting but did not need an update</td>
   <td>If a Machine is deleted, CPMS should always replace it to restore the cluster health</td>
   <td>Integration</td>
   <td>Case #8 covers OnDelete replacements at the E2E level.
<p>
This can be checked at the integration level succinctly.
   </td>
   <td></td>
   <td>Integration</td>
  </tr>
  <tr>
   <td>11</td>
   <td>With non-standard indexes, the RollingUpdate strategy can still update Machines</td>
   <td>Mapping logic should produce a failure domain map based on Machine indexes.
<p>
RollingUpdate should request to create a new Machine in the same indexes that exist.
   </td>
   <td>Unit, Integration</td>
   <td>This is already tested at the unit and integration level, no need for E2E as it doesn’t involve components outside of the CPMS and Machines, which can be controller in integration tests
   </td>
   <td></td>
   <td>Unit, Integration</td>
  </tr>
  <tr>
   <td>12</td>
   <td>With non-standard indexes, the OnDelete strategy can still update Machines</td>
   <td>Mapping logic should produce a failure domain map based on Machine indexes.
<p>
OnDelete should request to create a new Machine in the same indexes that exist.
   </td>
   <td>Unit, Integration</td>
   <td>This is already tested at the unit and integration level, no need for E2E as it doesn’t involve components outside of the CPMS and Machines, which can be controller in integration tests</td>
   <td></td>
   <td>Unit, Integration</td>
  </tr>
  <tr>
   <td>13</td>
   <td>When activated, the CPMS inserts owner references to control plane machines</td>
   <td>Owner references should be added to control plane machines.
<p>
Owner references are valid.
   </td>
   <td>E2E</td>
   <td>Invalid owner references could cause the GC to remove the Machines, which would be bad.</td>
   <td>Should check that no Machines get a deletion timestamp set.</td>
   <td>E2E</td>
  </tr>
  <tr>
   <td>14</td>
   <td>When the control plane Machines are unbalanced, the RollingUpdate strategy should rebalance the machines</td>
   <td>Mapping logic should balance Machines
<p>
CPMS RollingUpdate should pick a Machine to replace
   </td>
   <td>Unit, Integration</td>
   <td>External factors don’t really influence this logic so we can simulate this effectively in an integration test</td>
   <td>This already has some coverage in the mapping unit tests.</td>
   <td>Unit, Integration</td>
  </tr>
  <tr>
   <td>15</td>
   <td>When the control plane Machines are unbalanced, the OnDelete strategy should rebalance the machines when a Machine is Deleted</td>
   <td>Mapping logic should balance Machines and de-prio deleting Machines</td>
   <td>Unit, Integration</td>
   <td>External factors don’t really influence this logic so we can simulate this effectively in an integration test
<p>
When a Machine is deleted, if it’s in a repeated failure domain, it should be the one to change, not the other Machines
   </td>
   <td>This already has some coverage in the mapping unit tests.</td>
   <td>Unit, Integration</td>
  </tr>
</table>

### Replacing 1, 2 or all Control Plane Machines

To save on time, the E2E test cases that actually intend to replace machines (#5 and #8) are designed not to replace all Machines. Replacing 1 Machine and waiting for the cluster to stabilise will already take 20-25 mins to complete based on observations made running this on AWS.

To repeat this for all Machines over two tests would take up at least 2 hours and bust the 2 hours time limit for a presubmit job.

Since we are testing CPMS behaviour during the E2Es, by replacing 2 machines through the entire suite (one in each of #5 and #8), by deliberately replacing different machines, we should be able to verify that the CPMS behaves correctly when creating Machines (if it can create 1 machine it can create more) but also that when we replace 2 separate control plane machines within a short period, that the cluster recovers and becomes stable again.

A full replacement will be handled as a separate period test suite that will run less often and be monitored separately. This should alert us to any larger issues with the OpenShift product when replacing control plane machines.

### Checking load balancer connectivity

When we replace a Control Plane Machine, it is imperative that we check the connectivity to the instance via the internal and external load balancers where applicable. This should ensure that the load balancer attachments within the cloud provider have been created/updated correctly.

For this, we will need to, for each platform, write a test that checks the load balancer attachment.

For cloud platforms, this will involve connecting to the cloud provider, finding the load balancers and, in whichever way the platform has load balancer attachment, checking the instance is indeed connected to the load balancer. If there are health checks configured and we can review the state of the instance, we should also check the health of the instance according to the load balancer.

For non-cloud platforms, we will need to check that the software level load balancing (eg keepalived) has the correct membership. Investigation will need to be done for each platform in this case.

Credentials for clouds are available in tests, see [https://github.com/openshift/release/blob/master/ci-operator/SECRETS.md](https://github.com/openshift/release/blob/master/ci-operator/SECRETS.md) for details.

## Periodic Test cases

Some E2E test cases we do not intend to run as a part of the presubmit test suite. These test cases are either too disruptive or too long running/expensive to run as a presubmit, as such, we will instead run them on a 72 hour(?) basis and as a release informing job to get some signal as to whether the tests are passing, without having to run them repeatedly on presubmits.

<table>
  <tr>
   <td><strong>Case Number</strong></td>
   <td><strong>Description</strong></td>
   <td><strong>Behaviour under test</strong></td>
   <td><strong>Desired Test Level</strong></td>
   <td><strong>Motivation</strong></td>
   <td><strong>Notes</strong></td>
   <td><strong>Existing coverage</strong></td>
  </tr>
  <tr>
   <td>P1</td>
   <td>Full vertical scaling rolling update of the control plane</td>
   <td>Rolling update behaviour when changing the instance size of the control plane
<p>
Rolling update surge behaviour
<p>
Machine API replacing control plane machines
<p>
Control Plane Components reacting to changes in infrastructure
   </td>
   <td>E2E, Integration</td>
   <td>We want to be able to prove that we can replace the whole Control Plane without interruption and without degrading the cluster.
<p>
This doesn’t run as a presubmit because it is too long running.
<p>
This test in itself will take around 1 hour to execute.
<p>
We can also simulate this with an integration test to test the surging behaviour of CPMS.
   </td>
   <td>Origin has control plane connectivity checks in place that check the API availability during upgrades, we can copy this to check API availability during an update.
<p>
Intention here is to change CPMS spec to a bigger instance and watch it roll out, no fiddling with Machines, this will be exactly as a user would do the vertical scale.
   </td>
   <td>E2E, Integration</td>
  </tr>
  <tr>
   <td>P2</td>
   <td>A control plane machine can be replaced while a cluster wide proxy is in place</td>
   <td>Control plane machine can be replaced while in a proxy scenario
<p>
Control plane components can survive replacement during proxy scenario
   </td>
   <td>E2E</td>
   <td>We want to make sure that the latency added by having a proxy between resources does not affect replacing control plane machines</td>
   <td>Only need to replace a single machine here and check the cluster comes back together correctly.
<p>
This will likely be quite slow as the static pod installers will take a while to roll out.
   </td>
   <td></td>
  </tr>
</table>

### Periodic reporting

Further to a [conversation with Devan](https://coreos.slack.com/archives/C01CQA76KMX/p1665049023900659), the TRT team have set up a way so that we can request alerts to be sent to a slack channel of our choice should our release informing/periodic jobs drop below a certain pass rate.

Once we have set up the release informing jobs, we will request an alert to be sent to #forum-cloud when the pass rate drops below 80%.

This will allow us to keep an eye on the periodic jobs and ensure we quickly catch any issues that arise that don’t come up through presubmits.

Note, this will also help us pick up breaking changes introduced by other parts of the product which would otherwise not be picked up when we aren’t actively working on PRs to the CPMS repository.

### ARM Clusters

Presently, we do not support heterogeneous ARM based clusters within OpenShift. However, we do support an ARM based cluster. To ensure the control plane can be replaced in ARM clusters, we should set up a periodic running the tests on ARM on AWS and verify that there are no ARM specific issues in the control plane replacement process.

In the future, we may also want to test migrating an amd64 control plane to ARM as this is something that we know the ARM team are looking into as a cost saving exercise for service delivery.

## Writing the E2E tests

### Where will the tests live

Because the tests that we want to run are relatively specific to the ControlPlaneMachineSet, we can store the tests in the cluster-control-plane-machine-set-operator repository directly.

We will not include the tests in the origin test suite at first and instead, will use a separate release informing (and eventually blocking) job to provide signal about bad merges from other components.

The signal provided by the periodic jobs should prove whether the control plane replacement process is working end to end without a need to run as part of the origin test suite.

Existing tests in the origin suite prove that Machine API is working and as such, the basic origin test should prove the dependencies of the ControlPlaneMachineSet without having to add additional tests.

### Setup in the release repository

We will need to configure a new set of presubmit jobs for the ControlPlaneMachineSet. Firstly the presubmit jobs should run and be blocking on AWS. The Azure presubmit can be optional but should be expected to pass when changing code specific to the Azure codepath.

We will then need to create two periodic jobs, one for AWS and one for Azure. These will then run at a predefined interval TBD (72 hours?) and provide a release informing signal. Eventually we want to promote these jobs to release blocking.

Reporting will be configured for the periodic jobs so that when the pass rate drops below 80% a notification will prompt us to investigate the failures.

## Alternatives

### Checking load balancer connectivity

This section was rejected because of the non-deterministic nature of the test. While it is platform agnostic, there are many factors of networking (eg sticky sessions) that we may not know about and therefore we expect this test would not be stable.

To achieve this in a generic way, we can create a check based on the following sequence:

* Get the API server internal and external URLs from the Infrastructure status
* Create a client to connect to both the internal and external URLs (internal may need to proxy via a pod?)
* Create a pod to tail the audit logs (definition below) for the newly created Machine’s API server
* Stream the logs of the pod just created and filter based on audit ID
* Make a series of requests to the internal/external endpoint until either the audit ID of the request is found in the audit logs, or some timeout is reached

```yaml
apiVersion: v1
kind: Pod
metadata:
 annotations:
   target.workload.openshift.io/management: '{"effect": "PreferredDuringScheduling"}'
 labels:
   app: audit-log-stream
 name: audit-kube-apiserver-ip-10-0-219-244.us-east-2.compute.internal
 namespace: openshift-kube-apiserver
spec:
 containers:
 - args:
   - tail -f /var/log/kube-apiserver/audit.log
   command:
   - /bin/bash
   - -ec
   image: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:10bc4111b7d6488af7cfacbb540090b4186404372d83dd341b890d89395dbfd2
   imagePullPolicy: IfNotPresent
   name: audit-stream
   resources:
     requests:
       cpu: 10m
       memory: 100Mi
   securityContext:
     privileged: true
   volumeMounts:
   - mountPath: /var/log/kube-apiserver
     name: audit-dir
 nodeName: ip-10-0-219-244.us-east-2.compute.internal
 restartPolicy: Always
 tolerations:
 - operator: Exists
 volumes:
 - hostPath:
     path: /var/log/kube-apiserver
     type: ""
   name: audit-dir
```
