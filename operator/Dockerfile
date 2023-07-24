ARG ARCH=amd64

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:7198a357ff3a8ef750b041324873960cf2153c11cc50abb9d8d5f8bb089f6b4e

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator

COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
