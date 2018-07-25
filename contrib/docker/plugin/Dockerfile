FROM alpine:3.6

# Use APK mirrors for fault tolerance
RUN printf "http://dl-cdn.alpinelinux.org/alpine/v3.6/main\nhttp://dl-2.alpinelinux.org/alpine/v3.6/main\nhttp://dl-3.alpinelinux.org/alpine/v3.6/main\nhttp://dl-4.alpinelinux.org/alpine/v3.6/main\nhttp://dl-5.alpinelinux.org/alpine/v3.6/main\n\nhttp://dl-cdn.alpinelinux.org/alpine/v3.6/community\nhttp://dl-2.alpinelinux.org/alpine/v3.6/community\nhttp://dl-3.alpinelinux.org/alpine/v3.6/community\nhttp://dl-4.alpinelinux.org/alpine/v3.6/community\nhttp://dl-5.alpinelinux.org/alpine/v3.6/community" > /etc/apk/repositories

RUN apk update || true && \
    apk add coreutils nfs-utils util-linux blkid multipath-tools lsscsi e2fsprogs xfsprogs bash

# for go binaries to work inside an alpine container
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

RUN mkdir /host /netapp
RUN mkdir -p /var/lib/docker-volumes/netapp

RUN mkdir -p /usr/local/sbin
ADD iscsiadm /usr/local/sbin
ADD rescan-scsi-bus /sbin
ADD docker /sbin
ADD container-launch.sh /netapp

ADD trident /netapp
RUN chmod 777 /sbin/rescan-scsi-bus \
              /sbin/docker \
              /usr/local/sbin/iscsiadm \
              /netapp/container-launch.sh \
              /netapp/trident

RUN ln -s /host/etc/iscsi /etc/iscsi

ENV DOCKER_PLUGIN_MODE 1
EXPOSE 8000

CMD ["/netapp/container-launch.sh", "--config=/etc/netappdvp/config.json"]
