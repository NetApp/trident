ARG ARCH=amd64
# Image for dependencies such as NFS binaries
ARG DEPS_IMAGE=alpine:3

FROM --platform=linux/${ARCH} $DEPS_IMAGE AS deps

ARG DEPS_IMAGE

# Installs nfs-utils based on DEPS_IMAGE
RUN --mount=type=secret,id=activation_key,env=ACTIVATION_KEY \
    --mount=type=secret,id=organization,env=ORGANIZATION \
    function unregister() { subscription-manager unregister || true; }; trap unregister EXIT; \
    if [[ $DEPS_IMAGE =~ "alpine" ]]; \
        then apk add nfs-utils; \
        else subscription-manager register --activationkey $ACTIVATION_KEY --org $ORGANIZATION && \
            yum install --repo=rhel-9-*-baseos-rpms -y nfs-utils; \
    fi

# Get the mount.nfs4 dependency
RUN ldd /sbin/mount.nfs4 | tr -s '[:space:]' '\n' | grep '^/' | xargs -I % sh -c 'mkdir -p /nfs-deps/$(dirname %) && cp -L % /nfs-deps/%'
RUN ldd /sbin/mount.nfs | tr -s '[:space:]' '\n' | grep '^/' | xargs -I % sh -c 'mkdir -p /nfs-deps/$(dirname %) && cp -r -u -L % /nfs-deps/%'

FROM scratch

LABEL maintainers="The NetApp Trident Team" \
    app="trident.netapp.io" \
    description="Trident Storage Orchestrator"

COPY --from=deps /bin/mount /bin/umount /bin/
COPY --from=deps /sbin/mount.nfs /sbin/mount.nfs4 /sbin/
COPY --from=deps /etc/netconfig /etc/
COPY --from=deps /nfs-deps/ /

COPY --from=deps /etc/ssl/certs/ /etc/ssl/certs/

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
