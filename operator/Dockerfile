FROM gcr.io/distroless/static:a9b705d9ff06e9c60f6ceb2d2ef99dab084c17c0

LABEL maintainers="The NetApp Trident Team" \
      app="trident-operator.netapp.io" description="Trident Operator"

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /

ENTRYPOINT ["/$BIN"]
