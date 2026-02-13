ARG ARCH=amd64
ARG DEPS_IMAGE=alpine:3

FROM --platform=linux/${ARCH} $DEPS_IMAGE AS deps

ARG DEPS_IMAGE
ARG BIN=trident_orchestrator
ARG CLI_BIN=tridentctl
ARG CHWRAP_BIN=chwrap.tar
ARG NODE_PREP_BIN=node_prep
ARG SYSWRAP_BIN=syswrap

ARG BIN_ALLOWLIST="\
    /bin/mount \
    /bin/umount \
    /sbin/mount.nfs \
    /sbin/mount.nfs4 \
"

ARG FILE_ALLOWLIST="\
    /etc/os-release \
    /etc/netconfig \
    /etc/protocols \
    /etc/ssl/certs/* \
    /var/lib/rpm/rpmdb.sqlite \
"

# Install dependencies based on DEPS_IMAGE
RUN --mount=type=secret,id=activation_key,env=ACTIVATION_KEY \
    --mount=type=secret,id=organization,env=ORGANIZATION \
    function unregister() { subscription-manager unregister || true; }; trap unregister EXIT; \
    if [[ $DEPS_IMAGE =~ "alpine" ]]; \
        then apk add --no-scripts nfs-utils; \
        else subscription-manager register --activationkey $ACTIVATION_KEY --org $ORGANIZATION && \
            dnf install \
                --repo=rhel-9-*-baseos-rpms -y \
                --setopt=tsflags=noscripts \
                nfs-utils || { cat /var/log/rhsm/rhsm.log; exit 1; }  \
    fi

# Get dynamic libs for allowed binaries
RUN for bin in $BIN_ALLOWLIST; do \
        ldd $bin | tr -s '[:space:]' '\n' | grep '^/'; \
    done | sort | uniq > /tmp/ld_allowlist.txt

# Minimize RPM database
RUN if [[ $DEPS_IMAGE =~ "alpine" ]]; then exit 0; fi; \
    for f in $BIN_ALLOWLIST $FILE_ALLOWLIST $(cat /tmp/ld_allowlist.txt); do \
        rpm -qf $f; \
    done | sort | uniq > /tmp/rpm_allowlist.txt; \
    rpm --justdb -e --nodeps $(rpm -qa | grep -v -f /tmp/rpm_allowlist.txt)

# Copy required files to rootfs
RUN for f in $BIN_ALLOWLIST $FILE_ALLOWLIST $(cat /tmp/ld_allowlist.txt); do \
        if [ -e "$f" ]; then \
            dest="/rootfs${f}"; \
            mkdir -p "$(dirname "$dest")"; \
            cp -a "$f" "$dest"; \
        fi; \
    done

# Copy symlink targets to rootfs
RUN find /rootfs -type l | while read -r link; do \
        target="$(readlink $link)"; \
        if [[ "$target" =~ ^/ ]]; then \
            dest="/rootfs$target"; \
        else \
            dest="$(dirname $link)/$target"; \
            target=${dest#"/rootfs"}; \
        fi; \
        mkdir -p "$(dirname $dest)"; \
        cp -a "$target" "$dest"; \
    done

COPY ${BIN} /rootfs/trident_orchestrator
COPY ${CLI_BIN} /rootfs/bin/tridentctl
COPY ${NODE_PREP_BIN} /rootfs/node_prep
COPY ${SYSWRAP_BIN} /rootfs/syswrap
ADD ${CHWRAP_BIN} /rootfs/
COPY LICENSE NOTICE.txt /rootfs/licenses/

FROM scratch

ARG VERSION

LABEL maintainer="The NetApp Trident Team" \
      app="trident.netapp.io" \
      summary="Trident Storage Orchestrator" \
      description="Trident Storage Orchestrator manages persistent storage for containerized applications." \
      name="trident" \
      vendor="NetApp, Inc." \
      version="${VERSION}" \
      release="${VERSION}"

COPY --from=deps /rootfs /

ENTRYPOINT ["/bin/tridentctl"]
CMD ["version"]
