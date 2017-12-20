# Building Trident

Building Trident has following requirements:
* [Glide](https://github.com/Masterminds/glide) 0.12.2 or greater.   
* Docker 1.10 or greater when using the Makefile targets.
* Go 1.7 or greater is optionally required when building Trident natively.

Use `make build` to fetch dependencies, run a containerized build, and generate
Trident images. This is the simplest and the recommended way to build Trident.

## Building Trident Natively 

Before building Trident natively, its dependencies must be fetched using
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

will fetch, build, and install Glide natively and then runs the `glide install`
command above.

Once the dependencies have been fetched, Trident can be built with `go build`.

## Trident Makefile
Multiple Makefile targets, many of which are containerized, are available for 
building Trident. However, [Glide](https://github.com/Masterminds/glide) is a
native dependency.  

The targets take the following environment variables as parameters:

* `$REGISTRY_ADDR`:  IP address and port of a local Docker registry.
* `$TRIDENT_IMAGE`: name for the Trident Docker image; defaults to `trident`.
* `$TRIDENT_VERSION`: tag for the Trident Docker image; defaults to the last
  stable release version for Trident.
* `$BIN`:  name of the output binary.  Defaults to `trident_orchestrator`.

### Targets

* `make get`: Fetches, builds, and installs Glide natively. Then calls 
  `glide install` to fetch dependencies.

* `make build`: Builds Trident and builds Trident launcher. See `trident_build_all`
  and `launcher_build` targets for additional detail.
  
* `make clean`: Removes containers, container images, and build artifacts in 
  `bin/`.
	
* `make dist`: Builds Trident, builds Trident Launcher, and creates the Trident
  installer package.

* `make install`: Calls the `build` target and then runs containerized
  `go install`. 

* `make test`: Starts an etcd container (for use by the tests in `core/` and
  `persistent_store/`) and runs `go test` on the project. Unlike most other
  targets, this performs a native build. 

* `make vet`: Runs `go vet`.

* `make trident_build`: Builds Trident in a Go container, placing the
  trident_orchestrator and tridentctl binaries into `bin/`. A docker image is
  created and tagged as `[$REGISTRY_ADDR/]$TRIDENT_IMAGE:$VERSION_TAG`, with
  the bracketed portion omitted if `$REGISTRY_ADDR` is unset.  This tag
  defaults to `trident`.
  
* `make trident_build_all`: Executes the `get` target, which fetches Trident's
   dependencies, prior to calling `trident_build`.
   
* `make tridentctl_build`: Builds only the tridentctl binary. The `$GOOS` and 
  `$GOARCH` environment variables can be set to compile the tridentctl binary
  for an unsupported target system. 


## Building the Trident Launcher
Additional Makefile targets exist for launching Trident; [Building the Trident
Launcher](#building-the-trident-launcher) describes these and their parameters.

As with Trident, the Trident Launcher can be built using standard Go tools;
e.g.,

```bash
go build -o trident-launcher ./launcher
```

To facilitate building and running the launcher, we provide several targets in
the project's Makefile:

* `make launcher_build`:  Builds the Trident launcher. A docker image is created
  and tagged as `[$REGISTRY_ADDR]/$LAUNCHER_IMAGE:$LAUNCHER_VERSION`. The tag
  defaults to `$REGISTRY_ADDR/trident-launcher:local`
  using a launcher image with the tag
  `$REGISTRY_ADDR/$LAUNCHER_IMAGE:$LAUNCHER_VERSION`.  This automates the
  template modification and ConfigMap creation described above.

These targets take the following environment variables as parameters:

* `$LAUNCHER_IMAGE`:  image name for the launcher's Docker image; defaults to
  `trident-launcher`.
* `$LAUNCHER_VERSION`: tag for the Trident launcher's Docker image; defaults
  to the last stable release version for Trident. 
