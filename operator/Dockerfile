FROM gcr.io/distroless/static:e3da552fd2013b93c6ff63a02741c687c1f690c1

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /

ENTRYPOINT ["/$BIN"]
