FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.20 AS builder
WORKDIR /go/src/github.com/openshift/cluster-control-plane-machine-set-operator
COPY . .
RUN make build

FROM registry.ci.openshift.org/ocp/4.20:base-rhel9
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/bin/manager .
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/manifests manifests

LABEL io.openshift.release.operator true
