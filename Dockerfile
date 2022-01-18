FROM gcr.io/distroless/static:a9b705d9ff06e9c60f6ceb2d2ef99dab084c17c0

LABEL maintainers="The NetApp Trident Team" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

ARG PORT=8000
ENV PORT $PORT
EXPOSE $PORT
ARG BIN=trident_orchestrator
ENV BIN $BIN
ARG CLI_BIN=tridentctl
ENV CLI_BIN $CLI_BIN
ARG K8S=""
ENV K8S $K8S
ENV TRIDENT_IP localhost
ENV TRIDENT_SERVER 127.0.0.1:$PORT

COPY $BIN /
COPY $CLI_BIN /bin/
ADD chwrap.tar /

ENTRYPOINT ["/bin/$CLI_BIN"]
CMD ["version"]
