FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.22 AS builder
WORKDIR /go/src/github.com/openshift/cluster-control-plane-machine-set-operator
COPY . .
RUN make build && \
    gzip bin/cluster-control-plane-machine-set-operator-ext

FROM registry.ci.openshift.org/ocp/4.22:base-rhel9-minimal
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/bin/manager .
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/manifests manifests
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/bin/cluster-control-plane-machine-set-operator-ext.gz .

LABEL io.k8s.display-name="OpenShift Cluster Control Plane Machine Set Operator" \
    io.openshift.release.operator=true \
    io.openshift.tags="openshift,tests,e2e,e2e-extension"
