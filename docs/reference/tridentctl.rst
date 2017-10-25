##########
tridentctl
##########

The `Trident installer bundle`_ includes a command-line utility, ``tridentctl``,
that provides simple access to Trident. It can be used to interact with Trident
directly by any Kubernetes users with sufficient privileges to manage the
namespace that contains the Trident pod.

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
    logs        Print the logs from Trident
    version     Print the version of Trident

  Flags:
    -d, --debug              Debug output
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
    storageclass Get one or more storage classes from Trident
    volume       Get one or more volumes from Trident

logs
----

Print the logs from Trident

.. code-block:: console

  Usage:
    tridentctl logs [flags]

  Flags:
    -a, --archive      Create a support archive with all logs unless otherwise specified.
    -l, --log string   Trident log to display. One of trident|etcd|launcher|ephemeral|auto|all (default "auto")

version
-------

Print the version of tridentctl and the running Trident service

.. code-block:: console

  Usage:
    tridentctl version
