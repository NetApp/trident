FROM busybox:uclibc as busybox

FROM gcr.io/distroless/static@sha256:9be3fcc6abeaf985b5ecce59451acbcbb15e7be39472320c538d0d55a0834edc

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
