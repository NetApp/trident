# Copyright 2020 NetApp, Inc. All Rights Reserved.

GOARCH ?= amd64
GOGC ?= ""
GOPROXY ?= https://proxy.golang.org
GO_IMAGE = golang:1.14
OPERATOR_VOLUME = operator_build
TRIDENT_VOLUME_PATH = /go/src/github.com/netapp/trident
OPERATOR_VOLUME_PATH = /go/src/github.com/netapp/trident/operator
OPERATOR_CONFIG_PKG = github.com/netapp/trident/operator/config
TRIDENT_PROVISIONER_PKG = github.com/netapp/trident/operator/controllers/provisioner
K8S_CODE_GENERATOR = ./../code-generator-kubernetes-1.18.2

## build flags variables
GITHASH ?= `git rev-parse HEAD || echo unknown`
BUILD_TYPE ?= custom
BUILD_TYPE_REV ?= 0
BUILD_TIME = `date`

# common variables
ROOT = $(shell pwd)/..
BIN_DIR = ${ROOT}/bin
COVERAGE_DIR = ${ROOT}/coverage
BIN ?= trident-operator
TARBALL_BIN ?= trident-operator
K8S ?= ""
BUILD = build

DR_LINUX = docker run --rm \
	--net=host \
	-e GOOS=linux \
	-e GOARCH=$(GOARCH) \
	-e GOGC=$(GOGC) \
	-e GOPROXY=$(GOPROXY) \
	-v $(OPERATOR_VOLUME):/go \
	-v "${ROOT}":"${TRIDENT_VOLUME_PATH}" \
	-w $(OPERATOR_VOLUME_PATH) \
	$(GO_IMAGE)

GO_LINUX = ${DR_LINUX} go

.PHONY = default build retag dist dist_tar dist_tag clean vet

## version variables
OPERATOR_VERSION ?= 20.07.0
OPERATOR_IMAGE ?= trident-operator
ifeq ($(BUILD_TYPE),custom)
OPERATOR_VERSION := ${OPERATOR_VERSION}-custom
else ifneq ($(BUILD_TYPE),stable)
OPERATOR_VERSION := ${OPERATOR_VERSION}-${BUILD_TYPE}.${BUILD_TYPE_REV}
endif

## tag variables
OPERATOR_TAG := ${OPERATOR_IMAGE}:${OPERATOR_VERSION}
OPERATOR_TAG_OLD := ${OPERATOR_IMAGE}:${OPERATOR_VERSION}_old
ifdef REGISTRY_ADDR
OPERATOR_TAG := ${REGISTRY_ADDR}/${OPERATOR_TAG}
OPERATOR_TAG_OLD := ${REGISTRY_ADDR}/${OPERATOR_TAG_OLD}
endif
DIST_REGISTRY ?= netapp
OPERATOR_DIST_TAG := ${DIST_REGISTRY}/${OPERATOR_IMAGE}:${OPERATOR_VERSION}

## etcd variables
ifeq ($(ETCD_VERSION),)
ETCD_VERSION := v3.3.13
endif
ETCD_PORT ?= 8001
ETCD_SERVER ?= http://localhost:${ETCD_PORT}
ETCD_TAG := quay.io/coreos/etcd:${ETCD_VERSION}

# Go compiler flags need to be properly encapsulated with double quotes to handle spaces in values
BUILD_FLAGS = "-X \"${OPERATOR_CONFIG_PKG}.BuildHash=$(GITHASH)\" -X \"${OPERATOR_CONFIG_PKG}.BuildType=$(BUILD_TYPE)\" -X \"${OPERATOR_CONFIG_PKG}.BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"${OPERATOR_CONFIG_PKG}.BuildTime=$(BUILD_TIME)\" -X \"${OPERATOR_CONFIG_PKG}.BuildImage=$(OPERATOR_DIST_TAG)\" -X \"${OPERATOR_CONFIG_PKG}.BuildEtcdVersion=$(ETCD_VERSION)\" -X \"${OPERATOR_CONFIG_PKG}.BuildEtcdImage=$(ETCD_TAG)\""

## Operator build targets

default: dist

retag:
	-docker volume rm $(OPERATOR_VOLUME) || true
	-docker tag ${OPERATOR_TAG} ${OPERATOR_TAG_OLD}
	-docker rmi ${OPERATOR_TAG}

build: retag
    #$(info ${GO_LINUX})
	@mkdir -p ${BIN_DIR}
	@chmod 777 ${BIN_DIR}
	@${GO_LINUX} ${BUILD} -ldflags $(BUILD_FLAGS) -o ${TRIDENT_VOLUME_PATH}/bin/${BIN}
	cp ${BIN_DIR}/${BIN} .
	docker build --build-arg BIN=${BIN} --build-arg K8S=${K8S} -t ${OPERATOR_TAG} --rm .
ifdef REGISTRY_ADDR
	docker push ${OPERATOR_TAG}
endif
	rm ${BIN}
	-docker rmi ${OPERATOR_TAG_OLD}

dist_tag:
ifneq ($(OPERATOR_DIST_TAG),$(OPERATOR_TAG))
	-docker rmi ${OPERATOR_DIST_TAG}
	@docker tag ${OPERATOR_TAG} ${OPERATOR_DIST_TAG}
endif

dist_tar:
# 	-rm -rf /tmp/trident-installer
# 	@cp -a trident-installer /tmp/
# 	@mkdir -p /tmp/trident-installer/extras/bin
# 	@cp ${BIN_DIR}/${BIN} /tmp/trident-installer/extras/bin/${TARBALL_BIN}
# 	-rm -rf /tmp/trident-installer/setup
# 	-find /tmp/trident-installer -name \*.swp | xargs -0 -r rm
# 	@tar -C /tmp -czf trident-installer-${OPERATOR_VERSION}.tar.gz trident-installer
# 	-rm -rf /tmp/trident-installer

dist: build dist_tar dist_tag

## Test targets
#test:

## Misc. targets

clean:
	-docker volume rm $(OPERATOR_VOLUME) || true
	-docker rmi ${OPERATOR_TAG} ${OPERATOR_DIST_TAG} || true
	-rm -f ${BIN_DIR}/${BIN} trident-installer-${OPERATOR_VERSION}.tar.gz
	-rm -rf ${COVERAGE_DIR}

vet:
	@go vet $(shell go list ./... | grep -v /vendor/)

k8s_codegen:
	tar zxvf ${K8S_CODE_GENERATOR}.tar.gz -C ./../ --no-same-owner
	chmod +x ${K8S_CODE_GENERATOR}/generate-groups.sh
	${K8S_CODE_GENERATOR}/generate-groups.sh all ${TRIDENT_PROVISIONER_PKG}/client \
    ${TRIDENT_PROVISIONER_PKG}/apis "netapp:v1" -h ./../hack/boilerplate.go.txt
	rm -rf ${K8S_CODE_GENERATOR}
