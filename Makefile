# Copyright 2018 NetApp, Inc. All Rights Reserved.


GOOS ?= linux
GOARCH ?= amd64
GOGC ?= ""
TRIDENT_VOLUME = trident_build
TRIDENT_VOLUME_PATH = /go/src/github.com/netapp/trident
TRIDENT_CONFIG_PKG = github.com/netapp/trident/config

## build flags variables
GITHASH ?= `git rev-parse HEAD || echo unknown`
BUILD_TYPE ?= custom
BUILD_TYPE_REV ?= 0
BUILD_TIME = `date`
# Go compiler flags need to be properly encapsulated with double quotes to handle spaces in values
BUILD_FLAGS = "-X \"${TRIDENT_CONFIG_PKG}.BuildHash=$(GITHASH)\" -X \"${TRIDENT_CONFIG_PKG}.BuildType=$(BUILD_TYPE)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTime=$(BUILD_TIME)\""

# common variables
PORT ?= 8000
ROOT = $(shell pwd)
BIN_DIR = ${ROOT}/bin
BIN ?= trident_orchestrator
TARBALL_BIN ?= trident
CLI_BIN ?= tridentctl
CLI_PKG ?= github.com/netapp/trident/cli
K8S ?= ""
BUILD = build

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

.PHONY = default get build trident_build trident_build_all trident_retag tridentctl_build launcher_build launcher_retag dist build_container_tools dist_tar dist_tag test test_core test_other clean fmt install vet

default: dist

## version variables
TRIDENT_VERSION ?= 18.04.0
TRIDENT_IMAGE ?= trident
ifeq ($(BUILD_TYPE),custom)
TRIDENT_VERSION := ${TRIDENT_VERSION}-custom
else ifneq ($(BUILD_TYPE),stable)
TRIDENT_VERSION := ${TRIDENT_VERSION}-${BUILD_TYPE}.${BUILD_TYPE_REV}
endif
LAUNCHER_IMAGE ?= trident-launcher
LAUNCHER_VERSION ?= ${TRIDENT_VERSION}

## tag variables
TRIDENT_TAG := ${TRIDENT_IMAGE}:${TRIDENT_VERSION}
TRIDENT_TAG_OLD := ${TRIDENT_IMAGE}:${TRIDENT_VERSION}_old
LAUNCHER_TAG := ${LAUNCHER_IMAGE}:${LAUNCHER_VERSION}
LAUNCHER_TAG_OLD := ${LAUNCHER_IMAGE}:${LAUNCHER_VERSION}_old
ifdef REGISTRY_ADDR
TRIDENT_TAG := ${REGISTRY_ADDR}/${TRIDENT_TAG}
TRIDENT_TAG_OLD := ${REGISTRY_ADDR}/${TRIDENT_TAG_OLD}
LAUNCHER_TAG := ${REGISTRY_ADDR}/${LAUNCHER_TAG}
LAUNCHER_TAG_OLD := ${REGISTRY_ADDR}/${LAUNCHER_TAG_OLD}
endif
DIST_REGISTRY ?= netapp
TRIDENT_DIST_TAG := ${DIST_REGISTRY}/${TRIDENT_IMAGE}:${TRIDENT_VERSION}
LAUNCHER_DIST_TAG := ${DIST_REGISTRY}/${LAUNCHER_IMAGE}:${LAUNCHER_VERSION}

## etcd variables
ETCD_VERSION ?= v3.1.5
ETCD_PORT ?= 8001
ETCD_SERVER ?= http://localhost:${ETCD_PORT}
ETCD_DIR ?= /tmp/etcd

## Trident build targets
get:
	@mkdir -p vendor
	@chmod 777 vendor
	@go get github.com/Masterminds/glide
	@go install github.com/Masterminds/glide
	@${GOPATH}/bin/glide install -v

trident_retag:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-docker tag ${TRIDENT_TAG} ${TRIDENT_TAG_OLD}
	-docker rmi ${TRIDENT_TAG}

trident_build: trident_retag
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@mkdir -p ${ROOT}/extras/external-etcd/bin
	@chmod 777 ${ROOT}/extras/external-etcd/bin
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${BIN}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${CLI_BIN} ${CLI_PKG}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/extras/external-etcd/bin/etcd-copy github.com/netapp/trident/extras/external-etcd/etcd-copy
	cp ${BIN_DIR}/${BIN} .
	cp ${BIN_DIR}/${CLI_BIN} .
	docker build --build-arg PORT=${PORT} --build-arg BIN=${BIN} --build-arg CLI_BIN=${CLI_BIN} --build-arg ETCDV3=${ETCD_SERVER} --build-arg K8S=${K8S} -t ${TRIDENT_TAG} --rm .
ifdef REGISTRY_ADDR
	docker push ${TRIDENT_TAG}
endif
	rm ${BIN}
	rm ${CLI_BIN}
	-docker rmi ${TRIDENT_TAG_OLD}

tridentctl_build:
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@${GO} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${CLI_BIN} ${CLI_PKG}
	cp ${BIN_DIR}/${CLI_BIN} .
	# docker build --build-arg PORT=${PORT} --build-arg CLI_BIN=${CLI_BIN} --build-arg K8S=${K8S} -t ${TRIDENT_TAG} --rm .
	rm ${CLI_BIN}

trident_build_all: get *.go trident_build

## Launcher build targets
launcher_retag:
	-docker tag ${LAUNCHER_TAG} ${LAUNCHER_TAG_OLD}
	-docker rmi ${LAUNCHER_TAG}

launcher_build: launcher_retag
	@chmod 777 ./launcher/docker-build
	@${GO} ${BUILD} -o ${TRIDENT_VOLUME_PATH}/launcher/docker-build/launcher ./launcher
	docker build -t ${LAUNCHER_TAG} ./launcher/docker-build/
ifdef REGISTRY_ADDR
	docker push ${LAUNCHER_TAG}
endif
	-docker rmi ${LAUNCHER_TAG_OLD}

## Build targets for constructing trident, launcher, and the trident installer bundle
build_container_tools:
	make -C extras/container-tools container_tools

dist_tag:
ifneq ($(TRIDENT_DIST_TAG),$(TRIDENT_TAG))
	-docker rmi ${TRIDENT_DIST_TAG}
	@docker tag ${TRIDENT_TAG} ${TRIDENT_DIST_TAG}
endif
ifneq ($(LAUNCHER_DIST_TAG),$(LAUNCHER_TAG))
	-docker rmi ${LAUNCHER_DIST_TAG}
	@docker tag ${LAUNCHER_TAG} ${LAUNCHER_DIST_TAG}
endif

dist_tar:
	-rm -rf /tmp/trident-installer
	@cp -a trident-installer /tmp/
	@cp ${BIN_DIR}/${CLI_BIN} /tmp/trident-installer/
	@cp -a extras /tmp/trident-installer/
	@mkdir -p /tmp/trident-installer/extras/bin
	@cp ${BIN_DIR}/${BIN} /tmp/trident-installer/extras/bin/${TARBALL_BIN}
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

dist: build dist_tar dist_tag build_container_tools

## Test targets
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

test: test_core test_other

## docker-compose targets
docker_compose_up:
	mkdir -p ${ETCD_DIR}
	PORT=${PORT} ETCD_DIR=${ETCD_DIR} K8S=${K8S} COMPOSE_HTTP_TIMEOUT=1800 docker-compose up

docker_compose_stop:
	-PORT=${PORT} ETCD_DIR=${ETCD_DIR} docker-compose stop

## Misc. targets
build: trident_build_all launcher_build

clean:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-docker rmi ${TRIDENT_TAG} ${TRIDENT_DIST_TAG} || true
	-docker rmi ${LAUNCHER_TAG} ${LAUNCHER_DIST_TAG} || true
	-docker rmi netapp/container-tools || true
	-rm -f ${BIN_DIR}/${BIN} ${BIN_DIR}/${CLI_BIN} trident-installer-${TRIDENT_VERSION}.tar.gz

fmt:
	@$(GO) fmt

install: build
	@$(GO) install

vet:
	@go vet $(shell go list ./... | grep -v /vendor/)
