##########
tridentctl
##########

The `Trident installer bundle`_ includes a command-line utility, ``tridentctl``,
that provides simple access to Trident. It can be used to install Trident, as
well as to interact with it directly by any Kubernetes users with sufficient
privileges, to manage the namespace that contains the Trident pod.

.. _Trident installer bundle: https://github.com/NetApp/trident/releases

For full usage information, run ``tridentctl --help``. Here are the available
commands and global options:

.. code-block:: console

  Usage:
    tridentctl [command]

  Available Commands:
    create      Add a resource to Trident
    delete      Remove one or more resources from Trident
    get         Get one or more resources from Trident
    help        Help about any command
    images      Print a table of the container images Trident needs
    import      Import an existing resource to Trident
    install     Install Trident
    logs        Print the logs from Trident
    send        Send a resource from Trident
    uninstall   Uninstall Trident
    update      Modify a resource in Trident
    upgrade     Upgrade a resource in Trident
    version     Print the version of Trident

  Flags:
    -d, --debug              Debug output
    -h, --help               help for tridentctl
    -n, --namespace string   Namespace of Trident deployment
    -o, --output string      Output format. One of json|yaml|name|wide|ps (default)
    -s, --server string      Address/port of Trident REST interface

create
------

Add a resource to Trident

.. code-block:: console

  Usage:
    tridentctl create [command]

  Available Commands:
    backend     Add a backend to Trident

delete
------

Remove one or more resources from Trident

.. code-block:: console

  Usage:
    tridentctl delete [command]

  Available Commands:
    backend      Delete one or more storage backends from Trident
    node         Delete one or more csi nodes from Trident
    snapshot     Delete one or more volume snapshots from Trident
    storageclass Delete one or more storage classes from Trident
    volume       Delete one or more storage volumes from Trident

get
---

Get one or more resources from Trident

.. code-block:: console

  Usage:
    tridentctl get [command]

  Available Commands:
    backend      Get one or more storage backends from Trident
    snapshot     Get one or more snapshots from Trident
    storageclass Get one or more storage classes from Trident
    volume       Get one or more volumes from Trident

import volume
-------------
Import an existing volume to Trident

.. code-block:: console

  Usage:
    tridentctl import volume <backendName> <volumeName> [flags]

  Aliases:
    volume, v

  Flags:
    -f, --filename string   Path to YAML or JSON PVC file
    -h, --help              help for volume
        --no-manage         Create PV/PVC only, don't assume volume lifecycle management

images
-------------
Print a table of the container images Trident needs

.. code-block:: console

  Usage:
    tridentctl images [flags]

  Flags:
    -h, --help                 Help for images
    -v, --k8s-version string   Semantic version of Kubernetes cluster

install
-------

Install Trident

.. code-block:: console

  Usage:
    tridentctl install [flags]

  Flags:
      --autosupport-image string   The container image for Autosupport Telemetry (default "netapp/trident-autosupport:20.07.0")
      --autosupport-proxy string   The address/port of a proxy for sending Autosupport Telemetry
      --csi                        Install CSI Trident (override for Kubernetes 1.13 only, requires feature gates).
      --enable-node-prep           Attempt to install required packages on nodes.
      --generate-custom-yaml       Generate YAML files, but don't install anything.
  -h, --help                       help for install
      --image-registry string      The address/port of an internal image registry.
      --k8s-timeout duration       The timeout for all Kubernetes operations. (default 3m0s)
      --kubelet-dir string         The host location of kubelet's internal state. (default "/var/lib/kubelet")
      --log-format string          The Trident logging format (text, json). (default "text")
      --pv string                  The name of the legacy PV used by Trident, will ensure this does not exist. (default "trident")
      --pvc string                 The name of the legacy PVC used by Trident, will ensure this does not exist. (default "trident")
      --silence-autosupport        Don't send autosupport bundles to NetApp automatically. (default true)
      --silent                     Disable most output during installation.
      --trident-image string       The Trident image to install.
      --use-custom-yaml            Use any existing YAML files that exist in setup directory.
      --use-ipv6                   Use IPv6 for Trident's communication.

logs
----

Print the logs from Trident

.. code-block:: console

  Usage:
    tridentctl logs [flags]

  Flags:
    -a, --archive       Create a support archive with all logs unless otherwise specified.
    -h, --help          help for logs
    -l, --log string    Trident log to display. One of trident|auto|trident-operator|all (default "auto")
        --node string   The kubernetes node name to gather node pod logs from.
    -p, --previous      Get the logs for the previous container instance if it exists.
        --sidecars      Get the logs for the sidecar containers as well.

send
----

Send a resource from Trident

.. code-block:: console

  Usage:
    tridentctl send [command]

  Available Commands:
    autosupport      Send an Autosupport archive to NetApp

uninstall
---------

Uninstall Trident

.. code-block:: console

  Usage:
    tridentctl uninstall [flags]

  Flags:
    -h, --help     help for uninstall
        --silent   Disable most output during uninstallation.

update
------

Modify a resource in Trident

.. code-block:: console

  Usage:
    tridentctl update [command]

  Available Commands:
    backend     Update a backend in Trident

upgrade
-------

Upgrade a resource in Trident

.. code-block:: console

   Usage:
  tridentctl upgrade [command]

   Available Commands:
     volume      Upgrade one or more persistent volumes from NFS/iSCSI to CSI

version
-------

Print the version of tridentctl and the running Trident service

.. code-block:: console

   Usage:
     tridentctl version [flags]

   Flags:
         --client   Client version only (no server required).
     -h, --help     help for version
