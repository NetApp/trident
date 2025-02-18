FROM alpine:3 AS deps

RUN mkdir /netapp
RUN mkdir -p /var/lib/docker-volumes/netapp

FROM scratch

LABEL maintainers="The NetApp Trident Team" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

COPY --from=deps /etc/ssl/certs/ /etc/ssl/certs/
COPY --from=deps /netapp /netapp
COPY --from=deps /var/lib/docker-volumes/netapp /var/lib/docker-volumes/netapp

ARG BIN=trident
ENV BIN=$BIN
ENV DOCKER_PLUGIN_MODE=1

COPY $BIN /netapp
ADD chwrap.tar /

# this image is only intended to be used as a Docker Plugin image
ENTRYPOINT ["/netapp/$BIN"]
CMD ["--help"]
