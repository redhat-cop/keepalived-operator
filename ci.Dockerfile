# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace

RUN go install github.com/gen2brain/keepalived_exporter@0.5.0 && \
    cp ${GOPATH}/bin/keepalived_exporter ./
RUN go install github.com/rjeczalik/cmd/notify@1.0.3 && \
    cp ${GOPATH}/bin/notify ./

FROM registry.access.redhat.com/ubi8/ubi
WORKDIR /
COPY --from=builder /workspace/notify /usr/local/bin
COPY --from=builder /workspace/keepalived_exporter /usr/local/bin
COPY bin/manager .
COPY config/templates /templates
COPY config/docker /usr/local/bin
RUN yum -y install --disableplugin=subscription-manager kmod iproute && yum clean all
USER 65532:65532

ENTRYPOINT ["/manager"]
