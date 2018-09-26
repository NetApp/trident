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

# common variables
PORT ?= 8000
ROOT = $(shell pwd)
BIN_DIR = ${ROOT}/bin
COVERAGE_DIR = ${ROOT}/coverage
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
	golang:1.10

GO=${DR} go

.PHONY = default get build trident_build trident_build_all trident_retag tridentctl_build dist build_container_tools dist_tar dist_tag test test_core test_other test_coverage_report clean fmt install vet

default: dist

## version variables
TRIDENT_VERSION ?= 18.07.1
TRIDENT_IMAGE ?= trident
ifeq ($(BUILD_TYPE),custom)
TRIDENT_VERSION := ${TRIDENT_VERSION}-custom
else ifneq ($(BUILD_TYPE),stable)
TRIDENT_VERSION := ${TRIDENT_VERSION}-${BUILD_TYPE}.${BUILD_TYPE_REV}
endif

## tag variables
TRIDENT_TAG := ${TRIDENT_IMAGE}:${TRIDENT_VERSION}
TRIDENT_TAG_OLD := ${TRIDENT_IMAGE}:${TRIDENT_VERSION}_old
ifdef REGISTRY_ADDR
TRIDENT_TAG := ${REGISTRY_ADDR}/${TRIDENT_TAG}
TRIDENT_TAG_OLD := ${REGISTRY_ADDR}/${TRIDENT_TAG_OLD}
endif
DIST_REGISTRY ?= netapp
TRIDENT_DIST_TAG := ${DIST_REGISTRY}/${TRIDENT_IMAGE}:${TRIDENT_VERSION}

## etcd variables
ifeq ($(ETCD_VERSION),)
ETCD_VERSION := v3.2.19
endif
ETCD_PORT ?= 8001
ETCD_SERVER ?= http://localhost:${ETCD_PORT}
ETCD_DIR ?= /tmp/etcd
ETCD_TAG := quay.io/coreos/etcd:${ETCD_VERSION}

# Go compiler flags need to be properly encapsulated with double quotes to handle spaces in values
BUILD_FLAGS = "-X \"${TRIDENT_CONFIG_PKG}.BuildHash=$(GITHASH)\" -X \"${TRIDENT_CONFIG_PKG}.BuildType=$(BUILD_TYPE)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTime=$(BUILD_TIME)\" -X \"${TRIDENT_CONFIG_PKG}.BuildImage=$(TRIDENT_DIST_TAG)\" -X \"${TRIDENT_CONFIG_PKG}.BuildEtcdVersion=$(ETCD_VERSION)\" -X \"${TRIDENT_CONFIG_PKG}.BuildEtcdImage=$(ETCD_TAG)\""

## Trident build targets
get:
	@mkdir -p vendor
	@chmod 777 vendor
	@go get github.com/Masterminds/glide
	@go install github.com/Masterminds/glide
	@${GOPATH}/bin/glide install --force --strip-vendor

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

## Build targets for constructing trident and the trident installer bundle
build_container_tools:
	make -C extras/container-tools container_tools

dist_tag:
ifneq ($(TRIDENT_DIST_TAG),$(TRIDENT_TAG))
	-docker rmi ${TRIDENT_DIST_TAG}
	@docker tag ${TRIDENT_TAG} ${TRIDENT_DIST_TAG}
endif

dist_tar:
	-rm -rf /tmp/trident-installer
	@cp -a trident-installer /tmp/
	@cp ${BIN_DIR}/${CLI_BIN} /tmp/trident-installer/
	@cp -a extras /tmp/trident-installer/
	@mkdir -p /tmp/trident-installer/extras/bin
	@cp ${BIN_DIR}/${BIN} /tmp/trident-installer/extras/bin/${TARBALL_BIN}
	-rm -rf /tmp/trident-installer/setup/backend.json /tmp/trident-installer/extras/container-tools
	@rm -rf /tmp/trident-installer/extras/external-etcd/etcd-copy
	-find /tmp/trident-installer -name \*.swp | xargs rm
	@mkdir -p /tmp/trident-installer/setup
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_DIST_TAG}|g" kubernetes-yaml/trident-deployment-external-etcd.yaml.templ > /tmp/trident-installer/extras/external-etcd/trident/trident-deployment-external-etcd.yaml
	@sed "s|__TRIDENT_IMAGE__|${TRIDENT_DIST_TAG}|g" kubernetes-yaml/etcdcopy-job.yaml.templ > /tmp/trident-installer/extras/external-etcd/trident/etcdcopy-job.yaml
	@cp kubernetes-yaml/trident-namespace.yaml /tmp/trident-installer/extras/external-etcd/trident/
	@cp kubernetes-yaml/trident-serviceaccounts.yaml /tmp/trident-installer/extras/external-etcd/trident/
	@cp kubernetes-yaml/trident-clusterrole* /tmp/trident-installer/extras/external-etcd/trident/
	@tar -C /tmp -czf trident-installer-${TRIDENT_VERSION}.tar.gz trident-installer
	-rm -rf /tmp/trident-installer

dist: build dist_tar dist_tag

## Test targets
test_core:
	@mkdir -p ${COVERAGE_DIR}
	@chmod 777 ${COVERAGE_DIR}
	-docker kill etcd-test > /dev/null
	-docker rm etcd-test > /dev/null
	@docker run -d -p ${ETCD_PORT}:${ETCD_PORT} --name etcd-test quay.io/coreos/etcd:${ETCD_VERSION} /usr/local/bin/etcd -name etcd1 -advertise-client-urls http://localhost:${ETCD_PORT} -listen-client-urls http://0.0.0.0:${ETCD_PORT} > /dev/null
	@go test -v -coverprofile=${COVERAGE_DIR}/persistent_store-coverage.out github.com/netapp/trident/persistent_store -args -etcd_v2=${ETCD_SERVER} -etcd_v3=${ETCD_SERVER} -etcd_src=${ETCD_SERVER} -etcd_dest=${ETCD_SERVER}
	@sleep 1
	@go test -cover -v github.com/netapp/trident/core -args -etcd_v2=${ETCD_SERVER}
	@sleep 1
	@go test -cover -v github.com/netapp/trident/core -args -etcd_v3=${ETCD_SERVER}
	@sleep 1
	@go test -v -coverprofile=${COVERAGE_DIR}/core-coverage.out github.com/netapp/trident/core
	@docker kill etcd-test > /dev/null
	@docker rm etcd-test > /dev/null

test_other:
	@go test -v -coverprofile=${COVERAGE_DIR}/coverage.out $(shell go list ./... | grep -v /vendor/ | grep -v core | grep -v persistent_store)

test_coverage_report:
	@sed 1,1d ${COVERAGE_DIR}/persistent_store-coverage.out >>${COVERAGE_DIR}/coverage.out
	@sed 1,1d ${COVERAGE_DIR}/core-coverage.out >>${COVERAGE_DIR}/coverage.out
	@go tool cover -func=${COVERAGE_DIR}/coverage.out -o ${COVERAGE_DIR}/function-coverage.txt
	@go tool cover -html=${COVERAGE_DIR}/coverage.out -o ${COVERAGE_DIR}/coverage.html

test: test_core test_other test_coverage_report

## docker-compose targets
docker_compose_up:
	mkdir -p ${ETCD_DIR}
	PORT=${PORT} ETCD_DIR=${ETCD_DIR} K8S=${K8S} COMPOSE_HTTP_TIMEOUT=1800 docker-compose up

docker_compose_stop:
	-PORT=${PORT} ETCD_DIR=${ETCD_DIR} docker-compose stop

## Misc. targets
build: trident_build_all

clean:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-docker rmi ${TRIDENT_TAG} ${TRIDENT_DIST_TAG} || true
	-docker rmi netapp/container-tools || true
	-rm -f ${BIN_DIR}/${BIN} ${BIN_DIR}/${CLI_BIN} trident-installer-${TRIDENT_VERSION}.tar.gz
	-rm -rf ${COVERAGE_DIR}

fmt:
	@$(GO) fmt

install: build
	@$(GO) install

vet:
	@go vet $(shell go list ./... | grep -v /vendor/)
