FROM alpine:3.12

LABEL app="trident-operator.netapp.io" description="Trident Operator"

# Use APK mirrors for fault tolerance
RUN printf "http://dl-2.alpinelinux.org/alpine/v3.12/main\nhttp://dl-3.alpinelinux.org/alpine/v3.12/main\nhttp://dl-4.alpinelinux.org/alpine/v3.12/main\nhttp://dl-5.alpinelinux.org/alpine/v3.12/main\n\nhttp://dl-1.alpinelinux.org/alpine/v3.12/community\nhttp://dl-2.alpinelinux.org/alpine/v3.12/community\nhttp://dl-3.alpinelinux.org/alpine/v3.12/community\nhttp://dl-4.alpinelinux.org/alpine/v3.12/community\nhttp://dl-5.alpinelinux.org/alpine/v3.12/community" > /etc/apk/repositories

RUN apk update || true && \
	apk add coreutils util-linux bash curl jq ca-certificates

# for go binaries to work inside an alpine container
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

ARG BIN=trident-operator
ENV BIN $BIN
ARG K8S=""
ENV K8S $K8S

COPY $BIN /usr/local/bin/

RUN mkdir /netapp
ADD container-launch.sh /netapp
RUN chmod 777 /netapp/container-launch.sh

CMD ["/netapp/container-launch.sh"]
