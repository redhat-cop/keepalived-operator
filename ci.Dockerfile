# Build the manager binary
FROM golang:1.22@sha256:094e47ef90125eb49dfbc67d3480b56ee82ea9b05f50b750b5e85fab9606c2de as builder

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
