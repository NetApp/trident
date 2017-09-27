FROM debian:jessie

LABEL maintainer="Ardalan.Kangarlou@netapp.com" \
      app="trident.netapp.io" \
      description="Trident Storage Orchestrator"

RUN apt-get update && apt-get install -y \
	open-iscsi \
	lsscsi \
	sg3-utils \
	scsitools \
	kmod \
	curl \
	jq \
	ca-certificates

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
