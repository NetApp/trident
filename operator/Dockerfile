FROM gcr.io/distroless/static:bab87b539201ccdc479de183c8d311d1a2337b84

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /

ENTRYPOINT ["/$BIN"]
