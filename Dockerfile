ARG ARCH=amd64
# Image for dependencies such as NFS binaries
ARG DEPS_IMAGE=alpine:3

FROM --platform=linux/${ARCH} $DEPS_IMAGE AS deps

ARG DEPS_IMAGE
ARG BIN=trident_orchestrator
ARG CLI_BIN=tridentctl
ARG CHWRAP_BIN=chwrap.tar
ARG NODE_PREP_BIN=node_prep
ARG SYSWRAP_BIN=syswrap

# Installs nfs-utils based on DEPS_IMAGE
RUN --mount=type=secret,id=activation_key,env=ACTIVATION_KEY \
    --mount=type=secret,id=organization,env=ORGANIZATION \
    function unregister() { subscription-manager unregister || true; }; trap unregister EXIT; \
    if [[ $DEPS_IMAGE =~ "alpine" ]]; \
        then apk add nfs-utils; \
        else subscription-manager register --activationkey $ACTIVATION_KEY --org $ORGANIZATION && \
            yum install --repo=rhel-9-*-baseos-rpms -y nfs-utils || { cat /var/log/rhsm/rhsm.log; exit 1; }  \
    fi;

# Copy dependencies to the root filesystem
RUN for dep in /bin/mount /bin/umount /sbin/mount.nfs /sbin/mount.nfs4 /etc/netconfig /etc/protocols /etc/ssl/certs/*; do \
        mkdir -p /rootfs/$(dirname $dep) && cp -L $dep /rootfs/$dep; \
    done

# Copy NFS dependencies to the root filesystem
RUN for bin in /sbin/mount.nfs /sbin/mount.nfs4; do \
        ldd $bin | tr -s '[:space:]' '\n' | grep '^/' | xargs -I % sh -c 'mkdir -p /rootfs/$(dirname %) && cp -L % /rootfs/%'; \
    done

COPY ${BIN} /rootfs/trident_orchestrator
COPY ${CLI_BIN} /rootfs/bin/tridentctl
COPY ${NODE_PREP_BIN} /rootfs/node_prep
COPY ${SYSWRAP_BIN} /rootfs/syswrap
ADD ${CHWRAP_BIN} /rootfs/

FROM scratch

LABEL maintainers="The NetApp Trident Team" \
    app="trident.netapp.io" \
    description="Trident Storage Orchestrator"

COPY --from=deps /rootfs /

ENTRYPOINT ["/bin/tridentctl"]
CMD ["version"]
