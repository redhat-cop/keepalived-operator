FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/keepalived-operator \
    USER_UID=1001 \
    USER_NAME=keepalived-operator

# install operator binary
COPY build/_output/bin/keepalived-operator ${OPERATOR}
COPY build/_output/bin/notify /usr/local/bin
COPY build/scripts /scripts
COPY build/templates /templates

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
