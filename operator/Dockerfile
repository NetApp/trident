ARG ARCH=amd64

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:69830f29ed7545c762777507426a412f97dad3d8d32bae3e74ad3fb6160917ea

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator

COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
