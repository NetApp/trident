FROM busybox:latest as busybox

FROM gcr.io/distroless/static:a9b705d9ff06e9c60f6ceb2d2ef99dab084c17c0

LABEL maintainers="The NetApp Trident Team" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

COPY --from=busybox /bin/sh /bin/mkdir /bin/ln /bin/rm /usr/bin/
SHELL ["/usr/bin/sh", "-c"]

RUN /usr/bin/mkdir /netapp
RUN /usr/bin/mkdir -p /var/lib/docker-volumes/netapp

RUN /usr/bin/rm /usr/bin/*

ARG BIN=trident
ENV BIN $BIN
ENV DOCKER_PLUGIN_MODE 1

COPY $BIN /netapp
ADD chwrap.tar /

# this image is only intended to be used as a Docker Plugin image
ENTRYPOINT ["/netapp/$BIN"]
CMD ["--help"]
