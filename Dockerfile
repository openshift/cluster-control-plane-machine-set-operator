FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.18-openshift-4.11 AS builder
WORKDIR /go/src/github.com/openshift/cluster-control-plane-machine-set-operator
COPY . .
RUN make build

FROM registry.ci.openshift.org/ocp/4.11:base
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/bin/manager .
COPY --from=builder /go/src/github.com/openshift/cluster-control-plane-machine-set-operator/manifests manifests

LABEL io.openshift.release.operator true
