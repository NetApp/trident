# Building Trident

Trident supports both native and Dockerized builds, each of which has different
base requirements.
* Go 1.7 or greater, if building natively.
* Docker 1.10 or greater, if using the Makefile targets that perform a
  Dockerized build.

Before building Trident, its dependencies must be fetched using
[Glide](https://github.com/Masterminds/glide) 0.12.2 or greater.  If Glide is
already installed, run

```bash
glide install -v
```

The `-v` flag is necessary to strip vendored dependencies from the
Kubernetes package.  Alternatively, if Glide is not installed, running

```bash
make get
```

will fetch, build, and install Glide natively and then run the `glide install`
command above.

Once the dependencies have been fetched, Trident can be built with `go build`.
However, we also provide a Makefile with several targets.  These are as follows:

* `make docker_build`:  Fetches Trident's dependencies if they are not present
  and builds Trident in a Go container, placing the binary into `bin/`.  This
  does not construct a docker image, however.
* `make docker_image`:  Calls `make docker_build` and creates a Docker image
  for it, tagging it as `[$REGISTRY_ADDR/]$TRIDENT_IMAGE:$VERSION_TAG`, with
  the bracketed portion omitted if `$REGISTRY_ADDR` is unset.  This tag
  defaults to `trident:local`.
* `make pod`:  Calls `make docker_image`, tags the resultant image as
  `$REGISTRY_ADDR/$TRIDENT_IMAGE:$VERSION_TAG` and pushes it to the local
  docker registry referenced in `$REGISTRY_ADDR`, described below.
  `$REGISTRY_ADDR` must be set for this target.  The image tag defaults to
  `$REGISTRY_ADDR/trident:local`.
* `make test`:  Starts an etcd container (for use by the tests in `core/` and
  `persistent_store/`) and runs `go test` on the project.  Unlike the other
  targets, this performs a native build.

These targets take several environment variables as parameters:

* `$REGISTRY_ADDR`:  IP address and port of a local Docker registry.  Required
  for `make pod`.
* `$TRIDENT_IMAGE`: name for the Trident Docker image; defaults to `trident`.
* `$VERSION_TAG`: tag for the Trident Docker image; defaults to `local`.
* `$BIN`:  name of the output binary.  Defaults to `trident_orchestrator`.

Additional Makefile targets exist for launching Trident; [Building the Trident
Launcher](#building-the-trident-launcher) describes these and their parameters.

## Building the Trident Launcher

As with Trident, the Trident Launcher can be built using standard Go tools;
e.g.,

```bash
go build -o trident-launcher ./launcher
```

To facilitate building and running the launcher, we provide several targets in
the project's Makefile:

* `make launcher_build`:  Builds the launcher, creates a Docker image for it,
  tagging it as `$REGISTRY_ADDR/$LAUNCHER_IMAGE:$LAUNCHER_VERSION`, and pushes
  it to a local repository.  The tag defaults to
  `$REGISTRY_ADDR/trident-launcher:local`
* `make docker_launcher_build`:  As `make launcher_build`, but build the
  launcher in a Go container.
* `make launcher_start`:  Starts the launcher as a pod without building it,
  using a launcher image with the tag
  `$REGISTRY_ADDR/$LAUNCHER_IMAGE:$LAUNCHER_VERSION`.  This automates the
  template modification and ConfigMap creation described above.
* `make launcher`:  Combines `make launcher_build` and `make launcher_start`.
* `make build_and_launch`:  Combines `pod` (see [Building
  Trident](#building-trident)) and `make launcher`.
* `make launch`:  Starts the launcher as per `make launcher_start`, but uses
  images from the Docker Hub.

These targets take the following environment variables as parameters:

* `$REGISTRY_ADDR`:  IP address and port of a local Docker registry.  Required
  for all targets except `make launch`.
* `$LAUNCHER_BACKEND`:  location of the backend configuration file on the host.
  Required for all targets except `launcher_build`.
* `$LAUNCHER_IMAGE`:  image name for the launcher's Docker image; defaults to
  `trident-launcher`.
* `$LAUNCHER_VERSION`: tag for the launcher's Docker image; defaults to
  `local`.

