ARG ARCH=amd64

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:41972110a1c1a5c0b6adb283e8aa092c43c31f7c5d79b8656fbffff2c3e61f05

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator

COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
