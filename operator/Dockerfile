FROM gcr.io/distroless/static:480a6179aef97ba11817a86d1c7a0fed1e07e4b3

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /

ENTRYPOINT ["/$BIN"]
