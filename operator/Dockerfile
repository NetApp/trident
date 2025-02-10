FROM scratch

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator

COPY ${BIN} /trident-operator

ENTRYPOINT ["/trident-operator"]
