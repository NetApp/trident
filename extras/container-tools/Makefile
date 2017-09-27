download_etcdctl:
	mkdir -p /tmp/trident-extras/bin
	wget -O /tmp/trident-extras/etcd-v3.1.5-linux-amd64.tar.gz https://github.com/coreos/etcd/releases/download/v3.1.5/etcd-v3.1.5-linux-amd64.tar.gz
	tar -C /tmp/trident-extras -xvf /tmp/trident-extras/etcd-v3.1.5-linux-amd64.tar.gz
	mkdir -p /tmp/trident-extras/bin
	cp /tmp/trident-extras/etcd-v3.1.5-linux-amd64/etcdctl /tmp/trident-extras/bin

build_image:
	-docker tag netapp/container-tools netapp/container-tools:old
	-docker rmi netapp/container-tools
	cp /tmp/trident-extras/bin/etcdctl .
	docker build -t netapp/container-tools .
	#docker push netapp/container-tools
	-docker rmi netapp/container-tools:old

clean_etcdctl:
	-rm -rf /tmp/trident-extras ./etcdctl 

container_tools: download_etcdctl build_image clean_etcdctl
	@echo "Image netapp/container-tools was successfully built!"

