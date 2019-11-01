# Building Trident

Building Trident has following requirements:
* Docker 1.10 or greater when using the Makefile targets.
* Go 1.13 or greater is optionally required when building Trident natively.

Use `make build` to run a containerized build, and generate
Trident images. This is the simplest and the recommended way to build Trident.

## Building Trident Natively 

Trident can be built with `go build`.

## Trident Makefile
Multiple Makefile targets, many of which are containerized, are available for 
building Trident.

The targets take the following environment variables as parameters:

* `$REGISTRY_ADDR`:  IP address and port of a local Docker registry.
* `$TRIDENT_IMAGE`: name for the Trident Docker image; defaults to `trident`.
* `$TRIDENT_VERSION`: tag for the Trident Docker image; defaults to the last
  stable release version for Trident.
* `$BIN`:  name of the output binary.  Defaults to `trident_orchestrator`.

### Targets

* `make build`: Builds Trident.
  
* `make clean`: Removes containers, container images, and build artifacts in 
  `bin/`.
	
* `make dist`: Builds Trident and creates the Trident installer package.

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
  
* `make trident_build_all`: Executes the `trident_build` target as well as
  building the MacOS version of tridentctl.
   
* `make tridentctl_build`: Builds only the tridentctl binary. The `$GOOS` and 
  `$GOARCH` environment variables can be set to compile the tridentctl binary
  for an unsupported target system. 
