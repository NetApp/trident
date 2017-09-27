# Copyright 2016 NetApp, Inc. All Rights Reserved.

GOOS=linux
GOARCH=amd64
GOGC=""
TRIDENT_VOLUME=trident_build
TRIDENT_VOLUME_PATH=/go/src/github.com/netapp/trident
TRIDENT_CONFIG_PKG=github.com/netapp/trident/config

GITHASH?=`git rev-parse HEAD || echo unknown`
BUILD_TYPE?=custom
BUILD_TYPE_REV?=0
BUILD_TIME=`date`
# Go compiler flags need to be properly encapsulated with double quotes to handle spaces in values
BUILD_FLAGS="-X \"${TRIDENT_CONFIG_PKG}.BuildHash=$(GITHASH)\" -X \"${TRIDENT_CONFIG_PKG}.BuildType=$(BUILD_TYPE)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTime=$(BUILD_TIME)\""

PORT ?= 8000
ROOT = $(shell pwd)
BIN_DIR = ${ROOT}/bin
BIN ?= trident_orchestrator
CLI_BIN ?= tridentctl
CLI_PKG ?= github.com/netapp/trident/cli

DIST_REGISTRY?=netapp

TRIDENT_VERSION ?= 18.01.0

ifeq ($(BUILD_TYPE),custom)
TRIDENT_VERSION := ${TRIDENT_VERSION}-custom
else ifneq ($(BUILD_TYPE),stable)
TRIDENT_VERSION := ${TRIDENT_VERSION}-${BUILD_TYPE}.${BUILD_TYPE_REV}
endif

TRIDENT_IMAGE ?= trident
TRIDENT_DEPLOYMENT_FILE ?= ./kubernetes-yaml/trident-deployment-local.yaml
TRIDENT_DIST_TAG = ${DIST_REGISTRY}/${TRIDENT_IMAGE}:${TRIDENT_VERSION}

ETCD_VERSION ?= v3.1.5
ETCD_PORT ?= 8001
ETCD_SERVER ?= http://localhost:${ETCD_PORT}
ETCD_DIR ?= /tmp/etcd
K8S ?= ""
BUILD = build

LAUNCHER_IMAGE ?= trident-launcher
LAUNCHER_CONFIG_DIR ?= ./launcher/config
LAUNCHER_POD_FILE ?= ./launcher/kubernetes-yaml/launcher-pod-local.yaml
LAUNCHER_VERSION ?= ${TRIDENT_VERSION}
LAUNCHER_DIST_TAG = ${DIST_REGISTRY}/${LAUNCHER_IMAGE}:${TRIDENT_VERSION}

DR=docker run --rm \
	--net=host \
	-e GOOS=$(GOOS) \
	-e GOARCH=$(GOARCH) \
	-e GOGC=$(GOGC) \
	-v $(TRIDENT_VOLUME):/go \
	-v "${ROOT}":"${TRIDENT_VOLUME_PATH}" \
	-w $(TRIDENT_VOLUME_PATH) \
	golang:1.8

GO=${DR} go

.PHONY=default get build docker_get docker_build docker_image clean fmt install test test_core vet launcher_build launcher_start pod_launch prep_pod_template clear_trident

SRCS = $(shell find . -name "*.go")

default: build

check_registry:
ifndef REGISTRY_ADDR
	$(error Must define $$REGISTRY_ADDR to build and launch Trident and Trident launcher pods)
endif

TRIDENT_TAG=${TRIDENT_IMAGE}:${TRIDENT_VERSION}
TRIDENT_TAG_OLD=${TRIDENT_IMAGE}:${TRIDENT_VERSION}_old
ifdef REGISTRY_ADDR
TRIDENT_TAG:=${REGISTRY_ADDR}/${TRIDENT_TAG}
TRIDENT_TAG_OLD:=${REGISTRY_ADDR}/${TRIDENT_TAG_OLD}
endif
LAUNCHER_TAG=${REGISTRY_ADDR}/${LAUNCHER_IMAGE}:${LAUNCHER_VERSION}
LAUNCHER_TAG_OLD=${REGISTRY_ADDR}/${LAUNCHER_IMAGE}:${LAUNCHER_VERSION}_old

get:
	@go get github.com/Masterminds/glide
	@go install github.com/Masterminds/glide
	@${GOPATH}/bin/glide install -v

build:
	@mkdir -p ${BIN_DIR}
	@go ${BUILD} -ldflags $(BUILD_FLAGS) -o ${BIN_DIR}/${BIN}
	@go ${BUILD} -ldflags $(BUILD_FLAGS) -o ${BIN_DIR}/${CLI_BIN} ${CLI_PKG}
	@go ${BUILD} -ldflags $(BUILD_FLAGS) -o ${ROOT}/extras/external-etcd/bin/etcd-copy github.com/netapp/trident/extras/external-etcd/etcd-copy

vendor:
	@mkdir -p vendor
	@chmod 777 vendor
	@$(GO) get github.com/Masterminds/glide
	@$(GO) install github.com/Masterminds/glide
	@${DR} glide install -v

docker_build: vendor *.go
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${BIN}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${CLI_BIN} ${CLI_PKG}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/extras/external-etcd/bin/etcd-copy github.com/netapp/trident/extras/external-etcd/etcd-copy
	
docker_image: docker_retag docker_build
	cp ${BIN_DIR}/${BIN} .
	cp ${BIN_DIR}/${CLI_BIN} .
	docker build --build-arg PORT=${PORT} --build-arg BIN=${BIN} --build-arg CLI_BIN=${CLI_BIN} --build-arg ETCDV3=${ETCD_SERVER} --build-arg K8S=${K8S} -t ${TRIDENT_TAG} --rm .
	rm ${BIN}
	rm ${CLI_BIN}
	-docker rmi ${TRIDENT_TAG_OLD}

docker_retag:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-PORT=${PORT} ETCD_DIR=${ETCD_DIR} docker-compose rm -f || true
	-docker tag ${TRIDENT_TAG} ${TRIDENT_TAG_OLD}
	-docker rmi ${TRIDENT_TAG}

docker_clean:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-PORT=${PORT} ETCD_DIR=${ETCD_DIR} docker-compose rm -f || true
	-docker rmi ${TRIDENT_TAG} || true

docker_run:
	mkdir -p ${ETCD_DIR}
	PORT=${PORT} ETCD_DIR=${ETCD_DIR} K8S=${K8S} COMPOSE_HTTP_TIMEOUT=1800 docker-compose up

docker_stop:
	-PORT=${PORT} ETCD_DIR=${ETCD_DIR} docker-compose stop

push:
	docker push ${TRIDENT_TAG}

prep_pod_template:
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_TAG}|g" kubernetes-yaml/trident-deployment.yaml.templ > ${TRIDENT_DEPLOYMENT_FILE}
	@echo "Usable Trident pod definition available at ${TRIDENT_DEPLOYMENT_FILE}"

pod: check_registry docker_image push prep_pod_template

test: test_core test_other

test_core:
	-docker kill etcd-test > /dev/null
	-docker rm etcd-test > /dev/null
	@docker run -d -p ${ETCD_PORT}:${ETCD_PORT} --name etcd-test quay.io/coreos/etcd:${ETCD_VERSION} /usr/local/bin/etcd -name etcd1 -advertise-client-urls http://localhost:${ETCD_PORT} -listen-client-urls http://0.0.0.0:${ETCD_PORT} > /dev/null
	@go test -cover -v github.com/netapp/trident/persistent_store -args -etcd_v2=${ETCD_SERVER} -etcd_v3=${ETCD_SERVER} -etcd_src=${ETCD_SERVER} -etcd_dest=${ETCD_SERVER}
	@sleep 1
	@go test -cover -v github.com/netapp/trident/core -args -etcd_v2=${ETCD_SERVER}
	@sleep 1
	@go test -cover -v github.com/netapp/trident/core -args -etcd_v3=${ETCD_SERVER}
	@sleep 1
	@go test -cover -v github.com/netapp/trident/core
	@docker kill etcd-test > /dev/null
	@docker rm etcd-test > /dev/null

test_other:
	@go test -cover -v $(shell go list ./... | grep -v /vendor/ | grep -v core | grep -v persistent_store)

vet:
	@go vet $(shell go list ./... | grep -v /vendor/)

install: build
	@$(GO) install

clean: docker_clean
	-rm -f ${BIN_DIR}/${BIN}
	-rm -f ${BIN_DIR}/${CLI_BIN}

fmt:
	@$(GO) fmt

clear_trident:
	-kubectl delete --ignore-not-found=true pod trident

launcher_retag:
	-docker tag ${LAUNCHER_TAG} ${LAUNCHER_TAG_OLD}
	-docker rmi ${LAUNCHER_TAG}

launcher_build: check_registry launcher_retag
	go build -o ./launcher/docker-build/launcher ./launcher
	docker build -t ${LAUNCHER_TAG} ./launcher/docker-build/
	docker push ${LAUNCHER_TAG}
	-docker rmi ${LAUNCHER_TAG_OLD}

docker_launcher_build: check_registry launcher_retag
	@chmod 777 ./launcher/docker-build
	@${GO} ${BUILD} -o ${TRIDENT_VOLUME_PATH}/launcher/docker-build/launcher ./launcher
	docker build -t ${LAUNCHER_TAG} ./launcher/docker-build/
	docker push ${LAUNCHER_TAG}
	-docker rmi ${LAUNCHER_TAG_OLD}

launcher_start: prep_pod_template
ifndef LAUNCHER_BACKEND
	$(error Must define LAUNCHER_BACKEND to start the launcher.)
endif
	-kubectl delete --ignore-not-found=true configmap trident-launcher-config
	-kubectl delete --ignore-not-found=true pod trident-launcher
	@mkdir -p ${LAUNCHER_CONFIG_DIR}
	@cp ${LAUNCHER_BACKEND} ${LAUNCHER_CONFIG_DIR}/backend.json
	@cp ${TRIDENT_DEPLOYMENT_FILE} ${LAUNCHER_CONFIG_DIR}/trident-deployment.yaml
	@kubectl create configmap trident-launcher-config --from-file=${LAUNCHER_CONFIG_DIR}
	@sed "s|__LAUNCHER_TAG__|${LAUNCHER_TAG}|g" ./launcher/kubernetes-yaml/launcher-pod.yaml.templ > ${LAUNCHER_POD_FILE}
	@kubectl create -f ${LAUNCHER_POD_FILE}
	@echo "Trident Launcher started; pod definition in ${LAUNCHER_POD_FILE}"

launcher: docker_launcher_build launcher_start

dist_tar:
	-rm -rf /tmp/trident-installer
	@cp -a trident-installer /tmp/
	@cp ${BIN_DIR}/${CLI_BIN} /tmp/trident-installer/
	@cp -a extras /tmp/trident-installer/
	@mkdir -p /tmp/trident-installer/extras/bin
	@cp ${BIN_DIR}/${BIN} /tmp/trident-installer/extras/bin
	@cp launcher/docker-build/launcher /tmp/trident-installer/extras/bin
	-rm -rf /tmp/trident-installer/setup/backend.json /tmp/trident-installer/extras/container-tools
	@rm -rf /tmp/trident-installer/extras/external-etcd/etcd-copy
	-find /tmp/trident-installer -name \*.swp | xargs rm
	@mkdir -p /tmp/trident-installer/setup
	@sed "s|__LAUNCHER_TAG__|${LAUNCHER_DIST_TAG}|g" ./launcher/kubernetes-yaml/launcher-pod.yaml.templ > /tmp/trident-installer/launcher-pod.yaml
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_DIST_TAG}|g" kubernetes-yaml/trident-deployment.yaml.templ > /tmp/trident-installer/setup/trident-deployment.yaml
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_DIST_TAG}|g" kubernetes-yaml/trident-deployment-external-etcd.yaml.templ > /tmp/trident-installer/extras/external-etcd/trident/trident-deployment-external-etcd.yaml
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_DIST_TAG}|g" kubernetes-yaml/etcdcopy-job.yaml.templ > /tmp/trident-installer/extras/external-etcd/trident/etcdcopy-job.yaml
	@cp kubernetes-yaml/trident-namespace.yaml /tmp/trident-installer/
	@cp kubernetes-yaml/trident-serviceaccounts.yaml /tmp/trident-installer/
	@cp kubernetes-yaml/trident-clusterrole* /tmp/trident-installer/
	@tar -C /tmp -czf trident-installer-${TRIDENT_VERSION}.tar.gz trident-installer
	-rm -rf /tmp/trident-installer

dist_tag:
ifneq ($(TRIDENT_DIST_TAG),$(TRIDENT_TAG))
	-docker rmi ${TRIDENT_DIST_TAG}
	@docker tag ${TRIDENT_TAG} ${TRIDENT_DIST_TAG}
endif
ifneq ($(LAUNCHER_DIST_TAG),$(LAUNCHER_TAG))
	-docker rmi ${LAUNCHER_DIST_TAG}
	@docker tag ${LAUNCHER_TAG} ${LAUNCHER_DIST_TAG}
endif

build_all: pod docker_launcher_build

build_and_launch: clear_trident pod launcher

build_container_tools:
	make -C extras/container-tools container_tools

dist: build_all dist_tar dist_tag build_container_tools

