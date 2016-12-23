FROM debian:jessie  
MAINTAINER Ardalan Kangarlou <ardalan.kangarlou@netapp.com>

LABEL version="1.0" \
	  description="Kubernetes storage orchestrator"

RUN apt-get update && apt-get install -y \
	open-iscsi \
	lsscsi \
	sg3-utils \
	scsitools \
	kmod \
	curl \
	aeson-pretty \
	ca-certificates
ARG PORT=8000
ENV PORT $PORT
EXPOSE $PORT
ARG BIN=trident_orchestrator
ENV BIN $BIN
ARG ETCDV2=http://localhost:8001
ENV ETCDV2 $ETCDV2
ARG K8S=""
ENV K8S $K8S
ENV TRIDENT_IP localhost
COPY ./scripts/* /usr/local/sbin/
COPY $BIN /usr/local/bin
CMD ["/usr/local/bin/$BIN -port $PORT -etcd_v2 $ETCDV2 -k8s_api_server $K8S"]
