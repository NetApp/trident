# Trident - NetApp Docker Volume Plugin

Storage Orchestrator for Containers

## Building Trident Plugin

### Build Requirements

Building the Trident Plugin has the following requirements:

* Must have completed the [BUILD](https://github.com/NetApp/trident/blob/master/BUILD.md) process.
  * Docker 1.10 or greater when using the Makefile targets.
  * Go 1.9 or greater is optionally required when building Trident natively.
  * [Glide](https://github.com/Masterminds/glide) 0.12.2 or greater.
* And then _`make build`_ (the recommended build process)

> note: the trident repo should be checked out into `$GOPATH/src/github.com/netapp/`

### Build Trident Binary

From the .../netapp/trident/ base directory, execute:

```none
go build -v
```

After the build finishes, there should be a file named `trident`. The binary file can be tested by running:

```none
./trident -h
```

### Build the Trident Plugin Filesystem

Navigate to the plugin directory.

```none
cd contrib/docker/plugin
```

Run the build script.

```none
make
```

### Package Files and Push the Trident Plugin to a Local Registry

Requirement:

* A Docker Registry ([https://docs.docker.com/registry/deploying/](https://docs.docker.com/registry/deploying/))

Run the develop plugin script.

```none
./devPlugin
```

### Confirm Plugin is Available

```none
docker plugin ls
```

## Deploy Plugin

* [https://github.com/NetApp/trident/blob/master/docs/docker/deploying.rst](https://github.com/NetApp/trident/blob/master/docs/docker/deploying.rst)
* [https://netapp-trident.readthedocs.io/en/stable-v18.04/docker/deploying.html](https://netapp-trident.readthedocs.io/en/stable-v18.04/docker/deploying.html)
