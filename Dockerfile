FROM alpine:3.6

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

# Use APK mirrors for fault tolerance
RUN printf "http://dl-cdn.alpinelinux.org/alpine/v3.6/main\nhttp://dl-2.alpinelinux.org/alpine/v3.6/main\nhttp://dl-3.alpinelinux.org/alpine/v3.6/main\nhttp://dl-4.alpinelinux.org/alpine/v3.6/main\nhttp://dl-5.alpinelinux.org/alpine/v3.6/main\n\nhttp://dl-cdn.alpinelinux.org/alpine/v3.6/community\nhttp://dl-2.alpinelinux.org/alpine/v3.6/community\nhttp://dl-3.alpinelinux.org/alpine/v3.6/community\nhttp://dl-4.alpinelinux.org/alpine/v3.6/community\nhttp://dl-5.alpinelinux.org/alpine/v3.6/community" > /etc/apk/repositories

RUN apk update || true &&  \
	apk add coreutils util-linux blkid \
	lsscsi \
	e2fsprogs \
	bash \
	kmod \
	curl \
	jq \
	ca-certificates

# for go binaries to work inside an alpine container
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

ARG PORT=8000
ENV PORT $PORT
EXPOSE $PORT
ARG BIN=trident_orchestrator
ENV BIN $BIN
ARG CLI_BIN=tridentctl
ENV CLI_BIN $CLI_BIN
ARG ETCDV3=http://localhost:8001
ENV ETCDV3 $ETCDV3
ARG K8S=""
ENV K8S $K8S
ENV TRIDENT_IP localhost

COPY ./scripts/* $BIN $CLI_BIN ./extras/external-etcd/bin/etcd-copy /usr/local/bin/

RUN mkdir /netapp
ADD chroot-host-wrapper.sh /netapp
RUN chmod 777 /netapp/chroot-host-wrapper.sh
RUN    ln -s /netapp/chroot-host-wrapper.sh /netapp/blkid \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/blockdev \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/cat \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/df \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/free \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/iscsiadm \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/ls \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/lsblk \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/lsscsi \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/mkdir \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/mkfs.ext3 \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/mkfs.ext4 \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/mkfs.xfs \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/mount \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/multipath \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/multipathd \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/pgrep \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/rmdir \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/stat \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/umount


CMD ["/usr/bin/env -i PATH='/netapp:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' /usr/local/bin/$BIN -port $PORT -etcd_v3 $ETCDV3 -k8s_api_server $K8S"]
