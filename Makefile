# Copyright 2023 NetApp, Inc. All Rights Reserved.

# Parameters
# PLATFORMS defines which platforms are built for each target. If platforms is set to all, builds all supported platforms.
PLATFORMS ?= linux/amd64
ifeq ($(PLATFORMS),all)
PLATFORMS = $(ALL_PLATFORMS)
endif

# REGISTRY defines the container registry to push or tag images
REGISTRY ?= $(DEFAULT_REGISTRY)

# BUILDX_OUTPUT set to `load` or `push` with docker buildx to load or push images, default load
BUILDX_OUTPUT ?= load

# GO_IMAGE golang image used in default GO_SHELL
GO_IMAGE ?= golang:1.20

# GO_CMD go command used for go build
GO_CMD ?= go

# GOPROXY override default Go proxy
GOPROXY ?=

# GOFLAGS custom flags used in Go commands. See https://pkg.go.dev/cmd/go#hdr-Environment_variables
GOFLAGS ?=

# HELM_IMAGE helm image used in default HELM_CMD
HELM_IMAGE ?= alpine/helm:3.6.1

# DOCKER_CLI the docker-compatible cli used to run and tag images
DOCKER_CLI ?= docker

# BUILD_CLI the docker-compatible cli used to build images. If set to "docker buildx", the image build script will
# ensure an image builder instance exists. Windows builds and the manifest targets require BUILD_CLI set to "docker buildx"
BUILD_CLI ?= docker

# GO_SHELL sets the Go environment. By default uses DOCKER_CLI to create a container using GO_IMAGE. Set to empty string
# to use local shell
GO_SHELL ?= $(DOCKER_CLI) run \
	-e XDG_CACHE_HOME=/go/cache \
	-v $(TRIDENT_VOLUME):/go \
	-v $(ROOT):$(BUILD_ROOT) \
	-w $(BUILD_ROOT) \
	--rm \
	$(GO_IMAGE) \
	sh -c

# HELM_CMD sets the helm command. By default uses DOCKER_CLI to create a container using HELM_IMAGE. Set to 'helm' to
# use local helm command
HELM_CMD ?= $(DOCKER_CLI) run \
	-v $(ROOT):$(BUILD_ROOT) \
	-w $(BUILD_ROOT) \
	--rm \
	$(HELM_IMAGE)

# VERSION app version
VERSION ?= $(shell cat $(ROOT)/hack/VERSION)

# GITHASH git commit hash used in binaries
GITHASH ?= $(shell git describe --match=NeVeRmAtCh --always --abbrev=40 --dirty || echo unknown)

# BUILD_TYPE custom/stable/alpha/beta/empty string, default is custom for dev builds
BUILD_TYPE ?= custom

# BUILD_TYPE_REV build type revision, used by CI
BUILD_TYPE_REV ?= 0

# TRIDENT_IMAGE trident image name
TRIDENT_IMAGE ?= trident

# TRIDENT_IMAGE operator image name
OPERATOR_IMAGE ?= trident-operator

# MANIFEST_TAG tag for trident manifest
MANIFEST_TAG ?= $(TRIDENT_TAG)

# OPERATOR_MANIFEST_TAG tag for operator manifest
OPERATOR_MANIFEST_TAG ?= $(OPERATOR_TAG)

# BUILDX_CONFIG_FILE path to buildkitd config file for docker buildx. Set this to use an insecure registry with
# cross-platform builds, see example config: https://github.com/moby/buildkit/blob/master/docs/buildkitd.toml.md
BUILDX_CONFIG_FILE ?=

# Constants
ALL_PLATFORMS = linux/amd64 linux/arm64 windows/amd64/ltsc2022 windows/amd64/1809 darwin/amd64
DEFAULT_REGISTRY = docker.io/netapp
TRIDENT_CONFIG_PKG = github.com/netapp/trident/config
OPERATOR_CONFIG_PKG = github.com/netapp/trident/operator/config
TRIDENT_KUBERNETES_PKG = github.com/netapp/trident/persistent_store/crd
OPERATOR_CONFIG_PKG = github.com/netapp/trident/operator/config
OPERATOR_INSTALLER_CONFIG_PKG = github.com/netapp/trident/operator/controllers/orchestrator/installer
OPERATOR_KUBERNETES_PKG = github.com/netapp/trident/operator/controllers/orchestrator
VERSION_FILE = github.com/netapp/trident/hack/VERSION
BUILD_ROOT = /go/src/github.com/netapp/trident
TRIDENT_VOLUME = trident-build
DOCKER_BUILDX_INSTANCE_NAME = trident-builder
DOCKER_BUILDX_BUILD_CLI = docker buildx
WINDOWS_IMAGE_REPO = mcr.microsoft.com/windows/nanoserver
BUILDX_MANIFEST_DIR = /tmp/trident_buildx_manifests
K8S_CODE_GENERATOR = code-generator-kubernetes-1.18.2

# Calculated values
BUILD_TIME = $(shell date)
ROOT = $(shell pwd)

TRIDENT_VERSION := $(VERSION)
ifeq ($(BUILD_TYPE),custom)
TRIDENT_VERSION := $(VERSION)-custom
else ifneq ($(BUILD_TYPE),stable)
TRIDENT_VERSION := $(VERSION)-$(BUILD_TYPE).$(BUILD_TYPE_REV)
endif

# tag variables
TRIDENT_TAG := $(REGISTRY)/$(TRIDENT_IMAGE):$(TRIDENT_VERSION)
OPERATOR_TAG := $(REGISTRY)/$(OPERATOR_IMAGE):$(TRIDENT_VERSION)
TRIDENT_IMAGE_REPO := $(REGISTRY)/$(TRIDENT_IMAGE):
DEFAULT_OPERATOR_TAG := $(DEFAULT_REGISTRY)/$(OPERATOR_IMAGE):$(VERSION)

# linker flags need to be properly encapsulated with double quotes to handle spaces in values
LINKER_FLAGS = "-s -w -X \"$(TRIDENT_CONFIG_PKG).BuildHash=$(GITHASH)\" -X \"$(TRIDENT_CONFIG_PKG).BuildType=$(BUILD_TYPE)\" -X \"$(TRIDENT_CONFIG_PKG).BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"$(TRIDENT_CONFIG_PKG).BuildTime=$(BUILD_TIME)\" -X \"$(TRIDENT_CONFIG_PKG).BuildImage=$(TRIDENT_TAG)\" -X \"$(OPERATOR_CONFIG_PKG).BuildImage=$(OPERATOR_TAG)\""
OPERATOR_LINKER_FLAGS = "-s -w -X \"$(OPERATOR_CONFIG_PKG).BuildHash=$(GITHASH)\" -X \"$(OPERATOR_CONFIG_PKG).BuildType=$(BUILD_TYPE)\" -X \"$(OPERATOR_CONFIG_PKG).BuildTypeRev=$(BUILD_TYPE_REV)\" -X \"$(OPERATOR_CONFIG_PKG).BuildTime=$(BUILD_TIME)\" -X \"$(OPERATOR_CONFIG_PKG).BuildImage=$(OPERATOR_TAG)\" -X \"$(OPERATOR_INSTALLER_CONFIG_PKG).DefaultTridentVersion=$(TRIDENT_VERSION)\" -X \"$(OPERATOR_INSTALLER_CONFIG_PKG).DefaultTridentRepo=$(TRIDENT_IMAGE_REPO)\""

# Functions

# trident_image_platforms returns a list of platforms that support the trident image. Currently only linux and windows
# are supported.
# usage: $(call trident_image_platforms,$(platforms))
trident_image_platforms = $(filter linux% windows%,$1)

# operator_image_platforms returns a list of platforms that support the operator image. Currently only linux is supported.
# usage: $(call operator_image_platforms,$(platforms))
operator_image_platforms = $(filter linux%,$1)

# all_image_platforms returns a list of all platforms supported by all images. Currently an alias for trident_image_platforms.
# usage: $(call all_image_platforms,$(platforms))
all_image_platforms = $(call trident_image_platforms,$1)

# os returns the OS from platform, i.e. 'linux' from 'linux/amd64'
# usage: $(call os,$(platform))
os = $(word 1,$(subst /, ,$1))

# arch returns the architecture from platform, i.e. 'amd64' from 'linux/amd64'
# usage: $(call arch,$(platform))
arch = $(word 2,$(subst /, ,$1))

# os_version returns the OS version from platform, i.e. 'ltsc2022' from 'windows/amd64/ltsc2022'
# usage: $(call os_version,$(platform))
os_version = $(word 3,$(subst /, ,$1))

# slugify converts / to _
# usage: $(call slugify,$(str))
slugify = $(subst /,_,$1)

# image_tag returns the image tag for a platform
# usage: $(call image_tag,$(image_name),$(platform))
image_tag = $1-$(call os,$2)-$(call arch,$2)$(if $(call os_version,$2),-$(call os_version,$2))

# binary_path returns the repo-relative path to a binary, depending on platform
# usage: $(call binary_path,$(binary_name),$(platform))
binary_path = bin/$(call os,$2)/$(call arch,$2)/$(1)$(if $(findstring windows,$(call os,$2)),.exe)

# go_env sets environment variables for go commands
# usage: $(call go_env,$(platform))
go_env = CGO_ENABLED=0 GOOS=$(call os,$1) GOARCH=$(call arch,$1)$(if $(GOPROXY), GOPROXY=$(GOPROXY))$(if $(GOFLAGS), GOFLAGS='$(GOFLAGS)')

# go_build returns the go build command for the named binary, platform, and source
# usage: $(call go_build,$(binary_name),$(source_path),$(platform),$(linker_flags))
go_build = echo $(call binary_path,$1,$3) && $(call go_env,$3) \
	$(GO_CMD) build \
	-o $(call binary_path,$1,$3) \
	-ldflags $4 \
	$2

# chwrap_build returns a script that will build chwrap.tar for the platform
# usage: $(call chwrap_build,$(platform),$(linker_flags))
chwrap_build = $(call go_build,chwrap,./chwrap,$1,$2)\
	&& ./chwrap/make-tarball.sh $(call binary_path,chwrap,$1) $(call binary_path,chwrap.tar,$1)\
	&& rm -f $(call binary_path,chwrap,$1)

# binaries_for_platform returns a script to build all binaries required for platform. The binaries are tridentctl,
# trident_orchestrator, chwrap.tar, and trident_operator. chwrap.tar and trident_operator are only built for linux
# plaforms.
# usage: $(call binaries_for_platform,$(platform),$(linker_flags))
binaries_for_platform = $(call go_build,tridentctl,./cli,$1,$2)\
	$(if $(findstring darwin,$1),,\
		&& $(call go_build,trident_orchestrator,.,$1,$2)\
		$(if $(findstring linux,$1),\
			&& $(call chwrap_build,$1,$2) ))

# build_binaries_for_platforms returns a script to build all binaries for platforms. Attempts to add current directory
# as a safe git directory, in case GO_SHELL uses a different user than the source repo.
# usage: $(call build_binaries_for_platforms,$(platforms),$(go_shell),$(linker_flags))
build_binaries_for_platforms = $(strip $(if $2,$2 'git config --global --add safe.directory $$(pwd) || true; )\
	$(foreach platform,$(call remove_version,$1),$(call binaries_for_platform,$(platform),$3)&&) true$(if $2,'))

# build_operator_binaries_for_platforms returns a script to build all operator binaries for platforms.  Attempts to add
# BUILD_ROOT as a safe git directory, in case GO_SHELL uses a different user than the source repo.
# usage: $(call build_operator_binaries_for_platforms,$(platforms),$(go_shell),$(operator_linker_flags))
build_operator_binaries_for_platforms = $(strip $(if $2,$2 'git config --global --add safe.directory $(BUILD_ROOT) || true; )\
	$(foreach platform,$(call remove_version,$1),\
		$(call go_build,trident-operator,./operator,$(platform),$3) &&) true$(if $2,'))

# remove_version removes os_version from platforms
# usage: $(call remove_version,$(platforms))
remove_version = $(sort $(foreach platform,$1,$(call os,$(platform))/$(call arch,$(platform))))

# buildx_create_instance creates a docker buildx builder instance capable of building all supported platforms
# usage: $(call buildx_create_instance,$(buildx_config_file)
sep=,
empty=
sp=$(empty) $(empty)
buildx_create_instance = $(DOCKER_BUILDX_BUILD_CLI) create --name $(DOCKER_BUILDX_INSTANCE_NAME) --use --append \
	$(if $1,--config $1) \
	--platform $(subst $(sp),$(sep),$(call remove_version,$(call all_image_platforms,$(ALL_PLATFORMS))))

# docker_build_linux returns the docker build command for linux images. Set output to `load` or `push` to load
# or push with docker buildx
# usage: $(call docker_build_linux,$(build_cli),$(platform),$(tag),$(output))
docker_build_linux = $1 build \
	--platform $2 \
	--build-arg ARCH=$(call arch,$2) \
	--build-arg BIN=$(call binary_path,trident_orchestrator,$2) \
	--build-arg CLI_BIN=$(call binary_path,tridentctl,$2) \
	--build-arg CHWRAP_BIN=$(call binary_path,chwrap.tar,$2) \
	--tag $3 \
	--rm \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--builder trident-builder) \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--$4) \
	.

# docker_build_windows returns the docker build command for windows images. Set output to `load` or `push` to load
# or push with docker buildx
# usage: $(call docker_build_windows,$(build_cli),$(platform),$(tag),$(output))
docker_build_windows = $1 build \
	--platform $(call os,$2)/$(call arch,$2) \
	--file Dockerfile.Windows \
	--build-arg ARCH=$(call arch,$2) \
	--build-arg WINDOWS_VERSION=$(call os_version,$2) \
	--build-arg BIN=$(call binary_path,trident_orchestrator,$2) \
	--build-arg CLI_BIN=$(call binary_path,tridentctl,$2) \
	--tag $3 \
	--rm \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--builder trident-builder) \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--$4) \
	.

# docker_build_operator returns the docker build command for the operator image. Set output to `load` or `push` to load
# or push with docker buildx
# usage: $(call docker_build_operator,$(build_cli),$(platform),$(tag),$(output))
docker_build_operator = $1 build \
	--platform $2 \
	--file operator/Dockerfile \
	--build-arg ARCH=$(call arch,$2) \
	--build-arg BIN=$(call binary_path,trident-operator,$2) \
	--tag $3 \
	--rm \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--builder trident-builder) \
	$(if $(findstring $(DOCKER_BUILDX_BUILD_CLI),$1),--$4) \
	.

# build_images_for_platforms returns a script that will build container images for platforms.
# usage: $(call build_images_for_platforms,$(platforms),$(build_cli),$(trident_tag),$(buildx_output))
build_images_for_platforms = $(foreach platform,$1,\
	$(if $(findstring linux,$(platform)),\
		$(call docker_build_linux,$2,$(platform),$(call image_tag,$3,$(platform)),$4))\
	$(if $(findstring windows,$(platform)),\
		$(call docker_build_windows,$2,$(platform),$(call image_tag,$3,$(platform)),$4)) &&) true

# build_operator_images_for_platforms returns a script that will build operator images for platforms
# usage: $(call build_operator_images_for_platforms,$(platforms),$(build_cli),$(operator_tag),$(buildx_output)
build_operator_images_for_platforms = $(foreach platform,$1,\
	$(if $(findstring linux,$(platform)),\
		$(call docker_build_operator,$2,$(platform),$(call image_tag,$3,$(platform)),$4)) &&) true

# manifest_files_linux writes the attestation and image manifests to dir for platform and tag
# usage: $(call manifest_files_linux,$(output_dir),$(platform),$(tag))
manifest_files_linux = $(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $(call image_tag,$3,$2) | jq '.manifests[0]' > $1/image_$(call slugify,$2) &&\
	$(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $(call image_tag,$3,$2) | jq '.manifests[1]' > $1/attestation_$(call slugify,$2)

# manifest_files_windows writes the attestation and image manifests to dir for platform and tag, adding windows os version
# usage: $(call manifest_files_windows,$(output_dir),$(platform),$(tag),$(windows_image_repo))
manifest_files_windows = export os_version=$$($(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $4:$(call os_version,$2) | jq -r '.manifests[0].platform."os.version"') &&\
	$(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $(call image_tag,$3,$2) | jq '.manifests[0] | .platform."os.version"=env.os_version' > $1/image_$(call slugify,$2) &&\
	$(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $(call image_tag,$3,$2) | jq '.manifests[1]' > $1/attestation_$(call slugify,$2)

# manifest_files_for_platforms writes the attestation and image manifests for all platforms
# usage: $(call manifest_files_for_platforms,$(platforms),$(output_dir),$(tag),$(windows_image_repo))
manifest_files_for_platforms = $(foreach platform,$1,\
	$(if $(findstring linux,$(platform)),\
		$(call manifest_files_linux,$2,$(platform),$3))\
	$(if $(findstring windows,$(platform)),\
		$(call manifest_files_windows,$2,$(platform),$3,$4)) &&) true

# create_manifest_for_platforms creates manifest from platform manifests in dir
# usage: $(call create_manifest_for_platforms,$(platforms),$(dir),$(tag))
create_manifest_for_platforms = $(DOCKER_BUILDX_BUILD_CLI) imagetools create -t $3 $(foreach platform,$1,\
	-f $2/image_$(call slugify,$(platform)) -f $2/attestation_$(call slugify,$(platform)))

# image_digests returns a json list of image digests and other info for platforms, in the form {image: "$image_tag",
# digest: "$digest", os: "$os", architecture: "$arch"}
# usage: $(call image_digests,$(platforms),$(tag))
image_digests = for image in $(foreach platform,$1,$(call image_tag,$2,$(platform))); do\
		$(DOCKER_BUILDX_BUILD_CLI) imagetools inspect --raw $$image |\
		jq ".manifests[0] | {image: \"$$image\", digest: .digest, os: .platform.os, architecture: .platform.architecture }";\
	done | jq -n '{manifest: "$2", images: [inputs]}'

# Build targets

default: installer images

# builds binaries using configured build tool (docker or go) for platforms
binaries:
	@$(call build_binaries_for_platforms,$(PLATFORMS),$(GO_SHELL),$(LINKER_FLAGS))

operator_binaries:
	@$(call build_operator_binaries_for_platforms,$(call operator_image_platforms,$(PLATFORMS)),$(GO_SHELL),$(OPERATOR_LINKER_FLAGS))

# builds docker images for platforms. As a special case, if only one platform is provided, the images are retagged
# without platform.
images: binaries
ifeq ($(BUILD_CLI),$(DOCKER_BUILDX_BUILD_CLI))
	-@$(call buildx_create_instance,$(BUILDX_CONFIG_FILE))
endif
	@$(call build_images_for_platforms,$(call all_image_platforms,$(PLATFORMS)),$(BUILD_CLI),$(TRIDENT_TAG),$(BUILDX_OUTPUT))
# if a single image platform is specified, retag image without platform
ifeq (1,$(words $(call all_image_platforms,$(PLATFORMS))))
	@$(DOCKER_CLI) tag $(call image_tag,$(TRIDENT_TAG),$(call all_image_platforms,$(PLATFORMS))) $(MANIFEST_TAG)
endif

operator_images: operator_binaries
ifeq ($(BUILD_CLI),$(DOCKER_BUILDX_BUILD_CLI))
	-@$(call buildx_create_instance,$(BUILDX_CONFIG_FILE))
endif
	@$(call build_operator_images_for_platforms,$(call operator_image_platforms,$(PLATFORMS)),$(BUILD_CLI),$(OPERATOR_TAG),$(BUILDX_OUTPUT))
# if a single operator image platform is specified, retag image without platform
ifeq (1,$(words $(call operator_image_platforms,$(PLATFORMS))))
	@$(DOCKER_CLI) tag $(call image_tag,$(OPERATOR_TAG),$(call operator_image_platforms,$(PLATFORMS))) $(OPERATOR_MANIFEST_TAG)
endif

# creates multi-platform image manifest
manifest: images
	@rm -rf $(BUILDX_MANIFEST_DIR) && mkdir $(BUILDX_MANIFEST_DIR)
	@$(call manifest_files_for_platforms,$(call all_image_platforms,$(PLATFORMS)),$(BUILDX_MANIFEST_DIR),$(TRIDENT_TAG),$(WINDOWS_IMAGE_REPO))
	@$(call create_manifest_for_platforms,$(call all_image_platforms,$(PLATFORMS)),$(BUILDX_MANIFEST_DIR),$(MANIFEST_TAG))
	@rm -rf $(BUILDX_MANIFEST_DIR)

# creates multi-platform operator image manifest
operator_manifest: operator_images
	@rm -rf $(BUILDX_MANIFEST_DIR)_operator && mkdir $(BUILDX_MANIFEST_DIR)_operator
	@$(call manifest_files_for_platforms,$(call operator_image_platforms,$(PLATFORMS)),$(BUILDX_MANIFEST_DIR)_operator,$(OPERATOR_TAG))
	@$(call create_manifest_for_platforms,$(call operator_image_platforms,$(PLATFORMS)),$(BUILDX_MANIFEST_DIR)_operator,$(OPERATOR_MANIFEST_TAG))
	@rm -rf $(BUILDX_MANIFEST_DIR)_operator

# packages helm chart
chart:
	@cp README.md ./helm/trident-operator/
	@$(HELM_CMD) package --app-version $(TRIDENT_VERSION) --version $(TRIDENT_VERSION) ./helm/trident-operator \
		$(if $(HELM_PGP_KEY),--sign --key "$(HELM_PGP_KEY)" --keyring "$(HELM_PGP_KEYRING)")
	@rm -f ./helm/trident-operator/README.md

# builds installer bundle. Skips binaries that have not been built.
installer: binaries chart
	@rm -rf /tmp/trident-installer
	@cp -a trident-installer /tmp/
	@cp -a deploy /tmp/trident-installer/
	-@cp -a $(call binary_path,tridentctl,linux/amd64) /tmp/trident-installer/
	@sed -Ei "s|${DEFAULT_OPERATOR_TAG}|${OPERATOR_TAG}|g" /tmp/trident-installer/deploy/operator.yaml
	@sed -Ei "s|${DEFAULT_OPERATOR_TAG}|${OPERATOR_TAG}|g" /tmp/trident-installer/deploy/bundle_pre_1_25.yaml
	@sed -Ei "s|${DEFAULT_OPERATOR_TAG}|${OPERATOR_TAG}|g" /tmp/trident-installer/deploy/bundle_post_1_25.yaml
	@mkdir -p /tmp/trident-installer/helm
	@cp -a trident-operator-*.tgz /tmp/trident-installer/helm/
	@mkdir -p /tmp/trident-installer/extras/bin
	@cp $(call binary_path,trident_orchestrator,linux/amd64) /tmp/trident-installer/extras/bin/trident
	@mkdir -p /tmp/trident-installer/extras/macos/bin
	-@cp $(call binary_path,tridentctl,darwin/amd64) /tmp/trident-installer/extras/macos/bin/
	@tar -C /tmp -czf trident-installer-${TRIDENT_VERSION}.tar.gz trident-installer
	@rm -rf /tmp/trident-installer

# builds installer, images, and manifests for platforms
all: installer images manifest operator_images operator_manifest

# Developer tools
.PHONY: k8s_codegen k8s_codegen_operator mocks install-lint lint-precommit lint-prepush linker_flags operator_linker_flags clean

image_digests.json: images
	@$(call image_digests,$(call trident_image_platforms,$(PLATFORMS)),$(TRIDENT_TAG)) > image_digests.json

operator_image_digests.json: operator_images
	@$(call image_digests,$(call operator_image_platforms,$(PLATFORMS)),$(OPERATOR_TAG)) > operator_image_digests.json

linker_flags:
	@echo $(LINKER_FLAGS)

operator_linker_flags:
	@echo $(OPERATOR_LINKER_FLAGS)

tridentctl_images:
	@$(call binary_path,tridentctl,linux/amd64) images -o markdown > trident-required-images.md

clean:
	@rm -rf \
		bin \
		$(BUILDX_MANIFEST_DIR) \
		$(BUILDX_MANIFEST_DIR)_operator \
		trident-installer-*.tar.gz \
		trident-operator-*.tgz \
		image_digests.json \
		operator_image_digests.json \
		default binaries operator_binaries images operator_images manifest operator_manifest chart installer all

k8s_codegen:
	@tar zxvf $(K8S_CODE_GENERATOR).tar.gz --no-same-owner
	@chmod +x $(K8S_CODE_GENERATOR)/generate-groups.sh
	@$(K8S_CODE_GENERATOR)/generate-groups.sh all $(TRIDENT_KUBERNETES_PKG)/client \
		$(TRIDENT_KUBERNETES_PKG)/apis "netapp:v1" -h ./hack/boilerplate.go.txt
	@rm -rf $(K8S_CODE_GENERATOR)

k8s_codegen_operator:
	@tar zxvf $(K8S_CODE_GENERATOR).tar.gz --no-same-owner
	@chmod +x $(K8S_CODE_GENERATOR)/generate-groups.sh
	@$(K8S_CODE_GENERATOR)/generate-groups.sh all $(OPERATOR_KUBERNETES_PKG)/client \
		$(OPERATOR_KUBERNETES_PKG)/apis "netapp:v1" -h ./hack/boilerplate.go.txt
	@rm -rf $(K8S_CODE_GENERATOR)
	@rm -rf ./operator/controllers/orchestrator/client/*
	@mv $(OPERATOR_KUBERNETES_PKG)/client/* ./operator/controllers/orchestrator/client/
	@rm -rf ./operator/controllers/orchestrator/apis/netapp/v1/zz_generated.deepcopy.go
	@mv $(OPERATOR_KUBERNETES_PKG)/apis/netapp/v1/zz_generated.deepcopy.go ./operator/controllers/orchestrator/apis/netapp/v1/

mocks:
	@go install github.com/golang/mock/mockgen@v1.6.0
	@go generate ./...

.git/hooks:
	@mkdir -p $@

install-lint:
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin ${GOLANGCI-LINT_VERSION}

lint-precommit: .git/hooks install-lint
	@cp hooks/golangci-lint.sh .git/hooks/pre-commit

lint-prepush: .git/hooks install-lint .git/hooks/pre-push
	@cp hooks/golangci-lint.sh .git/hooks/pre-push
