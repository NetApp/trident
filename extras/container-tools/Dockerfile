FROM ubuntu

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      description="Utilities useful for debugging"

RUN apt-get update && apt-get install -y \
	inetutils-traceroute \
	dnsutils \
	iputils-ping \
	iputils-arping \
	tcpdump \
	curl \
	jq \
	etcd \
	ca-certificates

COPY etcdctl /usr/local/bin
ENV ETCDCTL_API 3
CMD ["/bin/bash -c while true; do echo hello && sleep 10; done"]
