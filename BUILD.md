# Building Trident

## Requirements

### Single-Platform (Linux)

* Docker-compatible container cli, such as nerdctl or docker
* Make, if not building natively or with linker flags
* Go 1.23 or greater, to optionally build Trident natively

### Multi-Platform

* Make
* Docker
* Go 1.23 or greater, to optionally build Trident binaries natively
* jq

## Makefile Parameters

`PLATFORMS`

Default: `linux/amd64`

Space-separated list of which platforms are to be built for each target. If set to the reserved word `all`, all
supported platforms will be built.

`REGISTRY`

Default: `docker.io/netapp`

Container registry used to tag images and manifests, and optionally to push images and manifests to.

`GO_IMAGE`

Default: `golang:1.23`

Container image used by default `$GO_SHELL` to run binary build scripts.

`GO_CMD`

Default: `go`

Go command used for `go build`.

`GOPROXY`

Overrides default Go proxy.

`HELM_IMAGE`

Default: `alpine/helm:3.16.1`

Container image used by default `$HELM_CMD` to package helm chart.

`HELM_CHART_VERSION`

If defined, overrides the default chart version

`DOCKER_CLI`

Default: `docker`

Docker-compatible cli, such as nerdctl or docker, used to run containers and tag single-platform images.

`BUILD_CLI`

Default: `docker`

`docker build` or `docker buildx` compatible cli, used to build container images. Must be set to `docker buildx` to build
Windows images or the manifests targets.

`GO_SHELL`

Shell that can run `go build` and shell scripts, used to build Go binaries. Set to empty string to use local shell and
go command. By default uses `$DOCKER_CLI` and `$GO_IMAGE` to run binary build scripts in a container.

`HELM_CMD`

Helm command used to package chart. By default uses `$DOCKER_CLI` and `$HELM_IMAGE` to package chart in a container.

`HELM_PGP_KEY`

If defined, signs helm package.

`HELM_PGP_KEYRING`

Required if `$HELM_PGP_KEY` is defined.

`VERSION`

Trident app version, used to tag images and in binaries. By default is read from `hack/VERSION`.

`GITHASH`

Git commit hash used in binaries. By default is read by `git` from `HEAD`.

`BUILD_TYPE`

Default: `custom`

Modifies image tags. Dev builds should use `custom`.

`BUILD_TYPE_REV`

Default: empty string

Modifies image tags. Used by CI.

`MANIFEST_TAG`

Default: `$REGISTRY/$TRIDENT_IMAGE:$VERSION`

Overrides default Trident manifest tag.

`OPERATOR_MANIFEST_TAG`

Default: `$REGISTRY/$OPERATOR_IMAGE:$VERSION`

Overrides default operator manifest tag.

`BUILDX_OUTPUT`

Default: `load`

Sets buildx output, `load` or `push`. `load` attempts to load container images to local docker context, `push` attempts
to push container images to `$REGISTRY`. Ignored if `$BUILD_CLI` is not set to `docker buildx`, and should be set to
`push` for multi-platform builds.

`BUILDX_CONFIG_FILE`

Default: `$HOME/.docker/buildx/buildkitd.default.toml`

Path to buildkitd config file for docker buildx. Used to configure buildx context, for example insecure registries and
registry mirrors. See docker (https://docs.docker.com/engine/reference/commandline/buildx_create/#config) and buildkit
(https://github.com/moby/buildkit/blob/master/docs/buildkitd.toml.md) docs for more options and examples.

`DEFAULT_AUTOSUPPORT_IMAGE`

Override the default asup image in tridentctl and operator

`DEFAULT_ACP_IMAGE`

Override the default acp image in tridentctl and operator

Example file:
```shell
# insecure, local/private registry, set $PRIVATE_REGISTRY to appropriate value
[registry."$PRIVATE_REGISTRY"]
  http = true
  insecure = true

# docker.io mirror, set $MIRROR to appropriate value
[registry."docker.io"]
  mirrors = ["$MIRROR"]
```

`TRIDENT_IMAGE`

Default: `trident`

Trident image name used to create image tag.

`OPERATOR_IMAGE`

Default: `trident-operator`

Trident operator image name used to create operator image tag.

## Makefile Targets

### Build Targets

The following targets are not PHONY, and can be skipped with `touch $target`. Use `make --just-print | -n` to see
exactly what steps each target will perform.

`default`

Default target, builds trident image and binaries, and installer bundle.

`binaries`

Builds trident binaries (`tridentctl`, `trident_orchestrator`, `chwrap.tar`) for specified `$PLATFORMS`.

`operator_binaries`

Builds operator binaries (`trident-operator`) for specified `$PLATFORMS`.

`images`

Builds trident container images and binaries for specified `$PLATFORMS`. As a special case, if only one platform is
specified, the resulting image is tagged with `$MANIFEST_TAG`. The `manifest` target may still be used with a single
platform.

`operator_images`

Builds operator images and binaries for specified `$PLATFORMS`. As a special case, if only one platform is
specified, the resulting image is tagged with `$MANIFEST_TAG`. The `manifest` target may still be used with a single
platform.

`manifest`

Builds multi-platform manifest and trident images and binaries for specified `$PLATFORMS`.

`operator_manifest`

Builds multi-platform manifest and operator images and binaries for specified `$PLATFORMS`.

`chart`

Packages helm chart using `$HELM_CMD`.

`installer`

Builds installer tarball and binaries. Attempts to include linux/amd64 tridentctl and trident, and darwin/amd64 tridentctl. Unbuilt binaries are skipped.

`all`

Builds all images, binaries, and the installer bundle for specified `$PLATFORMS`.

### Developer Tools

`linker_flags`

Prints linker flags that can be used with `go build`

`operator_linker_flags`

Prints linker flags for operator binary that can be used with `go build`

`tridentctl_images`

Creates trident-required-images.md which contains a table of all required container images.

`k8s_codegen`

Generates k8s client code.

`k8s_codegen_operator`

Generates k8s client code for operator.

`mocks`

Installs mockgen and generate mocks for unit tests.

`.git/hooks`

Creates git hooks directory

`install-lint`

Installs linting tool, `golangci-lint`

`lint-precommit`

Sets up `golangci-lint` as pre-commit linting tool.

`lint-prepush`

Sets up `golangci-lint` as pre-push linting tool.

## Examples

### Build all dependencies for single-platform development

```shell
# using docker or docker-compatible cli with docker alias
make

# non-docker cli not aliased to docker
DOCKER_CLI=nerdctl BUILD_CLI=nerdctl make

# using local go command
GO_SHELL= make
```

### Build all images and binaries for all supported platforms

Note: requires `docker buildx` and target registry

```shell
PLATFORMS=all \
REGISTRY=$PRIVATE_REGISTRY \
BUILD_CLI='docker buildx' \
BUILDX_OUTPUT=push \
make all
```

### Build linux amd64 and latest Windows with operator

Note: requires `docker buildx` and target registry

```shell
PLATFORMS='linux/amd64 windows/amd64/ltsc2022' \
REGISTRY=$PRIVATE_REGISTRY \
BUILD_CLI='docker buildx' \
BUILDX_OUTPUT=push \
make all
```

### Build linux amd64 and latest Windows without operator

Note: requires `docker buildx` and target registry

```shell
PLATFORMS='linux/amd64 windows/amd64/ltsc2022' \
REGISTRY=$PRIVATE_REGISTRY \
BUILD_CLI='docker buildx' \
BUILDX_OUTPUT=push \
make manifest
```

### Build tridentctl natively

```shell
# with optional linker flags, note the quotes around make
go build -o tridentctl -ldflags "$(make linker_flags)" ./cli

# without linker flags
go build -o tridentctl ./cli
```

### Build trident orchestrator natively

```shell
# with optional linker flags
go build -o trident -ldflags "$(make linker_flags)" .

# without linker flags
go build -o trident .
```

### Build trident operator natively

```shell
# with optional linker flags
go build -o trident-operator -ldflags "$(make operator_linker_flags)" ./operator

# without linker flags
go build -o trident-operator ./operator
```

### Build target without dependencies

Any build target can be skipped by creating a file with that name.

```shell
# rebuild manifest without images or binaries
touch binaries images
make manifest
```
