# Copyright 2018 NetApp, Inc. All Rights Reserved.


GOARCH ?= amd64
GOGC ?= ""
GOPROXY ?= https://proxy.golang.org
GO_IMAGE = golang:1.14
TRIDENT_VOLUME = trident_build
TRIDENT_VOLUME_PATH = /go/src/github.com/netapp/trident
TRIDENT_CONFIG_PKG = github.com/netapp/trident/config
TRIDENT_KUBERNETES_PKG = github.com/netapp/trident/persistent_store/crd
VERSION_FILE = github.com/netapp/trident/hack/VERSION
K8S_CODE_GENERATOR = code-generator-kubernetes-1.18.2

## build flags variables
GITHASH ?= `git describe --match=NeVeRmAtCh --always --abbrev=40 --dirty || echo unknown`
BUILD_TYPE ?= custom
BUILD_TYPE_REV ?= 0
BUILD_TIME = `date`

# common variables
PORT ?= 8000
ROOT = $(shell pwd)
BIN_DIR = ${ROOT}/bin
MACOS_BIN_DIR = ${BIN_DIR}/macos
COVERAGE_DIR = ${ROOT}/coverage
BIN ?= trident_orchestrator
TARBALL_BIN ?= trident
CLI_BIN ?= tridentctl
CLI_PKG ?= github.com/netapp/trident/cli
K8S ?= ""
BUILD = build
VERSION ?= $(shell cat ${ROOT}/hack/VERSION)

DR_LINUX = docker run --rm \
	--net=host \
	-e GOOS=linux \
	-e GOARCH=$(GOARCH) \
	-e GOGC=$(GOGC) \
	-e GOPROXY=$(GOPROXY) \
	-v $(TRIDENT_VOLUME):/go \
	-v "${ROOT}":"${TRIDENT_VOLUME_PATH}" \
	-w $(TRIDENT_VOLUME_PATH) \
	$(GO_IMAGE)

DR_MACOS = docker run --rm \
	--net=host \
	-e GOOS=darwin \
	-e GOARCH=$(GOARCH) \
	-e GOGC=$(GOGC) \
	-e GOPROXY=$(GOPROXY) \
	-v $(TRIDENT_VOLUME):/go \
	-v "${ROOT}":"${TRIDENT_VOLUME_PATH}" \
	-w $(TRIDENT_VOLUME_PATH) \
	$(GO_IMAGE)

GO_CMD ?= go

GO_LINUX = ${DR_LINUX} ${GO_CMD}

GO_MACOS = ${DR_MACOS} ${GO_CMD}

.PHONY = default build trident_build trident_build_all trident_retag tridentctl_build dist dist_tar dist_tag test test_core test_other test_coverage_report clean fmt install vet

default: dist

## version variables
TRIDENT_VERSION ?= ${VERSION}
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

## trident operator variable
OPERATOR_IMAGE ?= trident-operator
DEFAULT_TRIDENT_OPERATOR_REPO ?= netapp/${OPERATOR_IMAGE}
DEFAULT_TRIDENT_OPERATOR_VERSION ?= ${VERSION}
DEFAULT_TRIDENT_OPERATOR_IMAGE := ${DEFAULT_TRIDENT_OPERATOR_REPO}:${DEFAULT_TRIDENT_OPERATOR_VERSION}
OPERATOR_DIST_TAG := ${DIST_REGISTRY}/${OPERATOR_IMAGE}:${TRIDENT_VERSION}

# Go compiler flags need to be properly encapsulated with double quotes to handle spaces in values
BUILD_FLAGS = "-X \"${TRIDENT_CONFIG_PKG}.BuildHash=$(GITHASH)\" -X \"${TRIDENT_CONFIG_PKG}.BuildType=$(BUILD_TYPE)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"${TRIDENT_CONFIG_PKG}.BuildTime=$(BUILD_TIME)\" -X \"${TRIDENT_CONFIG_PKG}.BuildImage=$(TRIDENT_DIST_TAG)\""

## Trident build targets

trident_retag:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-docker tag ${TRIDENT_TAG} ${TRIDENT_TAG_OLD}
	-docker rmi ${TRIDENT_TAG}

operator_retag:
	cd operator && $(MAKE) retag

trident_build: trident_retag
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@${GO_LINUX} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${BIN}
	@${GO_LINUX} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${CLI_BIN} ${CLI_PKG}
	cp ${BIN_DIR}/${BIN} .
	cp ${BIN_DIR}/${CLI_BIN} .
	docker build --build-arg PORT=${PORT} --build-arg BIN=${BIN} --build-arg CLI_BIN=${CLI_BIN} --build-arg K8S=${K8S} -t ${TRIDENT_TAG} --rm .
ifdef REGISTRY_ADDR
	docker push ${TRIDENT_TAG}
endif
	rm ${BIN}
	rm ${CLI_BIN}
	-docker rmi ${TRIDENT_TAG_OLD}

tridentctl_build:
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@${GO_LINUX} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${CLI_BIN} ${CLI_PKG}
	cp ${BIN_DIR}/${CLI_BIN} .
	# docker build --build-arg PORT=${PORT} --build-arg CLI_BIN=${CLI_BIN} --build-arg K8S=${K8S} -t ${TRIDENT_TAG} --rm .
	rm ${CLI_BIN}

tridentctl_macos_build:
	@mkdir -p ${MACOS_BIN_DIR}
	@chmod 777 ${MACOS_BIN_DIR}
	@${GO_MACOS} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/macos/${CLI_BIN} ${CLI_PKG}

operator_build:
	cd operator && $(MAKE) build

trident_build_all: *.go trident_build tridentctl_macos_build

dist_tag:
ifneq ($(TRIDENT_DIST_TAG),$(TRIDENT_TAG))
	-docker rmi ${TRIDENT_DIST_TAG}
	@docker tag ${TRIDENT_TAG} ${TRIDENT_DIST_TAG}
endif

operator_dist_tag:
	cd operator && $(MAKE) dist_tag

dist_tar: build
	-rm -rf /tmp/trident-installer
	@cp -a trident-installer /tmp/
	@cp -a deploy /tmp/trident-installer/
	@cp ${BIN_DIR}/${CLI_BIN} /tmp/trident-installer/
	@sed -Ei.bak "s|${DEFAULT_TRIDENT_OPERATOR_IMAGE}|${OPERATOR_DIST_TAG}|g" /tmp/trident-installer/deploy/operator.yaml
	@sed -Ei.bak "s|${DEFAULT_TRIDENT_OPERATOR_IMAGE}|${OPERATOR_DIST_TAG}|g" /tmp/trident-installer/deploy/bundle.yaml
	@rm /tmp/trident-installer/deploy/*.bak
	@mkdir -p /tmp/trident-installer/extras/bin
	@cp ${BIN_DIR}/${BIN} /tmp/trident-installer/extras/bin/${TARBALL_BIN}
	@mkdir -p /tmp/trident-installer/extras/macos/bin
	@cp ${MACOS_BIN_DIR}/${CLI_BIN} /tmp/trident-installer/extras/macos/bin/${CLI_BIN}
	-rm -rf /tmp/trident-installer/setup
	-find /tmp/trident-installer -name \*.swp | xargs -0 -r rm
	@tar -C /tmp -czf trident-installer-${TRIDENT_VERSION}.tar.gz trident-installer
	-rm -rf /tmp/trident-installer

dist: dist_tar dist_tag

## Test targets
test_all:
	@mkdir -p ${COVERAGE_DIR}
	@chmod 777 ${COVERAGE_DIR}
	@go test -v -coverprofile=${COVERAGE_DIR}/coverage.out $(shell go list ./... | grep -v /vendor/)

test_coverage_report:
	@go tool cover -func=${COVERAGE_DIR}/coverage.out -o ${COVERAGE_DIR}/function-coverage.txt
	@go tool cover -html=${COVERAGE_DIR}/coverage.out -o ${COVERAGE_DIR}/coverage.html

test: test_all test_coverage_report

## docker-compose targets
docker_compose_up:
	PORT=${PORT} K8S=${K8S} COMPOSE_HTTP_TIMEOUT=1800 docker-compose up

docker_compose_stop:
	-PORT=${PORT} docker-compose stop

## Misc. targets
build: trident_build_all

clean:
	-docker volume rm $(TRIDENT_VOLUME) || true
	-docker rmi ${TRIDENT_TAG} ${TRIDENT_DIST_TAG} || true
	-docker rmi netapp/container-tools || true
	-rm -f ${BIN_DIR}/${BIN} ${BIN_DIR}/${CLI_BIN} trident-installer-${TRIDENT_VERSION}.tar.gz
	-rm -rf ${COVERAGE_DIR}
	cd operator && $(MAKE) clean

fmt:
	@$(GO_LINUX) fmt

install: build
	@$(GO_LINUX) install

vet:
	@go vet $(shell go list ./... | grep -v /vendor/)

k8s_codegen:
	tar zxvf ${K8S_CODE_GENERATOR}.tar.gz --no-same-owner
	chmod +x ${K8S_CODE_GENERATOR}/generate-groups.sh
	${K8S_CODE_GENERATOR}/generate-groups.sh all ${TRIDENT_KUBERNETES_PKG}/client \
    ${TRIDENT_KUBERNETES_PKG}/apis "netapp:v1" -h ./hack/boilerplate.go.txt
	rm -rf ${K8S_CODE_GENERATOR}

k8s_codegen_operator:
	cd operator && $(MAKE) k8s_codegen
