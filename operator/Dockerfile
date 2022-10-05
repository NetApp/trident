FROM gcr.io/distroless/static:f072ab8fd7e21912a30720928cce8ad562e82718

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /

ENTRYPOINT ["/$BIN"]
