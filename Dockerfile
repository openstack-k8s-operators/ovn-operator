# golang-builder is used in OSBS build
ARG GOLANG_BUILDER=golang:1.13
ARG OPERATOR_BASE_IMAGE=registry.access.redhat.com/ubi8/ubi-minimal:latest

FROM ${GOLANG_BUILDER} as builder

ARG REMOTE_SOURCE=.
ARG GO_BUILD_EXTRA_ARGS="-v"
ARG OPERATOR_SDK_URL=https://github.com/operator-framework/operator-sdk/releases/download/v1.0.1/operator-sdk-v1.0.1-x86_64-linux-gnu

WORKDIR /workspace

# Download operator-sdk
ADD ${OPERATOR_SDK_URL} /usr/local/bin/operator-sdk
RUN chmod 755 /usr/local/bin/operator-sdk

# Copy the Go Modules manifests
COPY ${REMOTE_SOURCE}/go.mod go.mod
COPY ${REMOTE_SOURCE}/go.sum go.sum
# cache deps before building and copying source so that we don't need to
# re-download as much and so that source changes don't invalidate our
# downloaded layer
RUN go mod download

# Copy only sufficient dependencies to build kustomize and controller-gen so
# they are cached.
COPY ${REMOTE_SOURCE}/Makefile .
COPY ${REMOTE_SOURCE}/Makefile.registry .

RUN make kustomize controller-gen

# Copy the rest of the source
COPY ${REMOTE_SOURCE}/PROJECT ./
COPY ${REMOTE_SOURCE}/main.go ./
COPY ${REMOTE_SOURCE}/controllers/ controllers/
COPY ${REMOTE_SOURCE}/util/ util/
COPY ${REMOTE_SOURCE}/tools/ tools/
COPY ${REMOTE_SOURCE}/config/ config/
# zz_generated.deepcopy.go is automatically generated, which can
# invalidate this cache layer even when the source hasn't changed.
COPY ${REMOTE_SOURCE}/api/ api/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build ${GO_BUILD_EXTRA_ARGS} -a -o manager main.go
RUN CGO_ENABLED=0 GO111MODULE=on go build ${GO_BUILD_EXTRA_ARGS} -a -o csv-generator tools/csv-generator.go

# Builds CRDs and base CSV
RUN make bundle

FROM ${OPERATOR_BASE_IMAGE}

ENV USER_UID=1001 \
    OPERATOR_BUNDLE=/usr/share/ovn-operator/bundle/
ENV BASE_CSV=${OPERATOR_BUNDLE}/ovn-operator.clusterserviceversion.yaml

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/csv-generator /usr/local/bin/
COPY --from=builder /workspace/bundle/manifests/ ${OPERATOR_BUNDLE}/

LABEL   com.redhat.component="ovn-operator-container" \
        name="ovn-operator" \
        version="0.1" \
        summary="OVN Operator" \
        io.k8s.name="ovn-operator" \
        io.k8s.description="This image includes the OVN operator"

USER ${USER_UID}

ENTRYPOINT ["/manager"]
