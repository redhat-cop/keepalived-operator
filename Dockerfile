# Build the manager binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
RUN  CGO_ENABLED=0 GOOS=linux go build -a -o manager main.go
RUN go get -u github.com/gen2brain/keepalived_exporter@0.5.0 && \
    cp ${GOPATH}/bin/keepalived_exporter ./
RUN go get -u github.com/rjeczalik/cmd/notify@1.0.3 && \
    cp ${GOPATH}/bin/notify ./

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/notify /usr/local/bin
COPY --from=builder /workspace/keepalived_exporter /usr/local/bin
COPY config/templates /templates
COPY config/docker /usr/local/bin
RUN yum -y install --disableplugin=subscription-manager kmod iproute && yum clean all
USER 65532:65532

ENTRYPOINT ["/manager"]
