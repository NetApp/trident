# Image for dependencies such as CA certificates
ARG DEPS_IMAGE=alpine:3

FROM $DEPS_IMAGE AS deps

RUN mkdir /real-certs; \
    cp -L /etc/ssl/certs/* /real-certs/

FROM scratch

ARG VERSION

LABEL maintainer="The NetApp Trident Team" \
      app="trident-operator.netapp.io" \
      summary="Trident Operator" \
      description="Trident Operator manages the lifecycle of Trident instances in a Kubernetes cluster." \
      name="trident-operator" \
      vendor="NetApp, Inc." \
      version="${VERSION}" \
      release="${VERSION}"

ARG BIN=trident-operator

COPY --from=deps /real-certs/ /etc/ssl/certs/
COPY LICENSE NOTICE.txt /licenses/
COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
