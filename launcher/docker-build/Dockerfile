FROM debian:jessie

MAINTAINER Chris Dragga <chris.dragga@netapp.com>

LABEL version="1.0" \
	description="Tool to bootstrap a PVC for Trident's etcd deployment."

COPY launcher /usr/local/bin/launcher
ENTRYPOINT ["/usr/local/bin/launcher"]
