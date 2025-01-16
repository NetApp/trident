ARG ARCH=amd64

FROM --platform=linux/${ARCH} alpine:latest AS baseimage

RUN apk add nfs-utils

#Get the mount.nfs4 dependency
RUN ldd /sbin/mount.nfs4 | tr -s '[:space:]' '\n' | grep '^/' | xargs -I % sh -c 'mkdir -p /nfs-deps/$(dirname %) && cp -L % /nfs-deps/%'
RUN ldd /sbin/mount.nfs | tr -s '[:space:]' '\n' | grep '^/' | xargs -I % sh -c 'mkdir -p /nfs-deps/$(dirname %) && cp -r -u -L % /nfs-deps/%'

FROM --platform=linux/${ARCH} gcr.io/distroless/static@sha256:69830f29ed7545c762777507426a412f97dad3d8d32bae3e74ad3fb6160917ea

LABEL maintainers="The NetApp Trident Team" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

COPY --from=baseimage /bin/mount /bin/umount /bin/
COPY --from=baseimage /sbin/mount.nfs /sbin/mount.nfs4 /sbin/
COPY --from=baseimage /etc/netconfig /etc/
COPY --from=baseimage /nfs-deps/ /

ARG BIN=trident_orchestrator
ARG CLI_BIN=tridentctl
ARG CHWRAP_BIN=chwrap.tar
ARG NODE_PREP_BIN=node_prep
ARG SYSWRAP_BIN=syswrap

COPY ${BIN} /trident_orchestrator
COPY ${CLI_BIN} /bin/tridentctl
COPY ${NODE_PREP_BIN} /node_prep
COPY ${SYSWRAP_BIN} /syswrap
ADD ${CHWRAP_BIN} /

ENTRYPOINT ["/bin/tridentctl"]
CMD ["version"]
