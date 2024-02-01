ARG ARCH=amd64

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:9be3fcc6abeaf985b5ecce59451acbcbb15e7be39472320c538d0d55a0834edc

LABEL maintainers="The NetApp Trident Team" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

ARG BIN=trident_orchestrator
ARG CLI_BIN=tridentctl
ARG CHWRAP_BIN=chwrap.tar

COPY ${BIN} /trident_orchestrator
COPY ${CLI_BIN} /bin/tridentctl
ADD ${CHWRAP_BIN} /

ENTRYPOINT ["/bin/tridentctl"]
CMD ["version"]
