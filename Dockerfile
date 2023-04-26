ARG ARCH=amd64

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:a01d47d4036cae5a67a9619e3d06fa14a6811a2247b4da72b4233ece4efebd57

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
