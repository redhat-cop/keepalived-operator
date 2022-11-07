FROM registry.access.redhat.com/ubi8/ubi-minimal
WORKDIR /
COPY bin/manager .
COPY config/templates /templates
USER 65532:65532

ENTRYPOINT ["/manager"]
