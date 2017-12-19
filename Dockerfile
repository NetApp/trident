FROM alpine:3.6

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

# Use APK mirrors for fault tolerance
RUN printf "http://dl-1.alpinelinux.org/alpine/v3.6/main\nhttp://dl-2.alpinelinux.org/alpine/v3.6/main\nhttp://dl-3.alpinelinux.org/alpine/v3.6/main\nhttp://dl-4.alpinelinux.org/alpine/v3.6/main\nhttp://dl-5.alpinelinux.org/alpine/v3.6/main\n\nhttp://dl-1.alpinelinux.org/alpine/v3.6/community\nhttp://dl-2.alpinelinux.org/alpine/v3.6/community\nhttp://dl-3.alpinelinux.org/alpine/v3.6/community\nhttp://dl-4.alpinelinux.org/alpine/v3.6/community\nhttp://dl-5.alpinelinux.org/alpine/v3.6/community" > /etc/apk/repositories

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

CMD ["/usr/local/bin/$BIN -port $PORT -etcd_v3 $ETCDV3 -k8s_api_server $K8S"]
