FROM alpine:3.12

# Use APK mirrors for fault tolerance
RUN printf "http://dl-2.alpinelinux.org/alpine/v3.12/main\nhttp://dl-3.alpinelinux.org/alpine/v3.12/main\nhttp://dl-4.alpinelinux.org/alpine/v3.12/main\nhttp://dl-5.alpinelinux.org/alpine/v3.12/main\n\nhttp://dl-1.alpinelinux.org/alpine/v3.12/community\nhttp://dl-2.alpinelinux.org/alpine/v3.12/community\nhttp://dl-3.alpinelinux.org/alpine/v3.12/community\nhttp://dl-4.alpinelinux.org/alpine/v3.12/community\nhttp://dl-5.alpinelinux.org/alpine/v3.12/community" > /etc/apk/repositories

RUN apk update || true && \
    apk add coreutils nfs-utils util-linux blkid multipath-tools lsscsi e2fsprogs xfsprogs bash

# for go binaries to work inside an alpine container
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

RUN mkdir /netapp
RUN mkdir -p /var/lib/docker-volumes/netapp

ADD container-launch.sh /netapp
ADD chroot-host-wrapper.sh /netapp

RUN    ln -s /netapp/chroot-host-wrapper.sh /netapp/blkid \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/blockdev \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/cat \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/dd \
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
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/multipath \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/multipathd \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/pgrep \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/resize2fs \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/rmdir \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/stat \
    && ln -s /netapp/chroot-host-wrapper.sh /netapp/xfs_growfs

ADD trident /netapp
RUN chmod 777 /netapp/container-launch.sh \
              /netapp/trident

ENV DOCKER_PLUGIN_MODE 1
EXPOSE 8000

CMD ["/netapp/container-launch.sh", "--config=/etc/netappdvp/config.json"]
