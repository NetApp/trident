ARG ARCH=amd64
ARG DEPS_IMAGE=alpine:3

FROM --platform=linux/${ARCH} $DEPS_IMAGE AS deps

ARG FILE_ALLOWLIST="\
    /etc/os-release \
    /etc/ssl/certs/* \
"

ARG BIN=trident-operator

# Copy required files to rootfs
RUN for f in $FILE_ALLOWLIST; do \
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

COPY LICENSE NOTICE.txt /rootfs/licenses/
COPY ${BIN} /rootfs/trident-operator

FROM scratch

ARG VERSION

LABEL maintainer="The NetApp Trident Team" \
      app="trident-operator.netapp.io" \
      summary="Trident Operator" \
      description="Trident Operator manages the lifecycle of Trident instances in a Kubernetes cluster." \
      name="trident-operator" \
      vendor="NetApp, Inc." \
      version="${VERSION}" \
      release="${VERSION}"

COPY --from=deps /rootfs /

ENTRYPOINT ["/trident-operator"]
