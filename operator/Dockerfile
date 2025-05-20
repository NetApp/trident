# Image for dependencies such as CA certificates
ARG DEPS_IMAGE=alpine:3

FROM $DEPS_IMAGE AS deps

RUN mkdir /real-certs; \
    cp -L /etc/ssl/certs/* /real-certs/

FROM scratch

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator

COPY --from=deps /real-certs/ /etc/ssl/certs/
COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
