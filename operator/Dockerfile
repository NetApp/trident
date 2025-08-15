# Image for dependencies such as CA certificates
ARG DEPS_IMAGE=alpine:3

FROM $DEPS_IMAGE AS deps

RUN mkdir /real-certs; \
    cp -L /etc/ssl/certs/* /real-certs/

FROM scratch

LABEL maintainer="The NetApp Trident Team" \
      app="trident-operator.netapp.io" \
      summary="Trident Operator" \
      description="Trident Operator manages the lifecycle of Trident instances in a Kubernetes cluster." \
      name="trident-operator" \
      vendor="NetApp, Inc." \
      version="25.06.2" \
      release="25.06.2"

ARG BIN=trident-operator

COPY --from=deps /real-certs/ /etc/ssl/certs/
COPY NOTICE.txt NOTICE_ASUP_module.txt /licenses/
COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
