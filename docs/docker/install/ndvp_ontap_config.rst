ONTAP Configuration
===================

User Permissions
----------------

Trident does not need full permissions on the ONTAP cluster and should not be used with the cluster-level admin account.
Below are the ONTAP CLI comands to create a dedicated user for Trident with specific permissions.

.. code-block:: bash

  # create a new Trident role
  security login role create -vserver [VSERVER] -role trident_role -cmddirname DEFAULT -access none

  # grant common Trident permissions
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "event generate-autosupport-log" -access all
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "network interface" -access readonly
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "version" -access readonly
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "vserver" -access readonly
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "vserver nfs show" -access readonly
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "volume" -access all
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "snapmirror" -access all

  # grant ontap-san Trident permissions
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "vserver iscsi show" -access readonly
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "lun" -access all

  # grant ontap-nas-economy Trident permissions
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "vserver export-policy create" -access all
  security login role create -vserver [VSERVER] -role trident_role -cmddirname "vserver export-policy rule create" -access all

  # create a new Trident user with Trident role
  security login create -vserver [VSERVER] -username trident_user -role trident_role -application ontapi -authmethod password

Configuration File Options
--------------------------

In addition to the global configuration values above, when using ONTAP these top level options are available.

+------------------------------+----------------------------------------------------------------------------+------------+
| Option                       | Description                                                                | Example    |
+==============================+============================================================================+============+
| ``managementLIF``            | IP address of ONTAP management LIF                                         | 10.0.0.1   |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``dataLIF``                  | IP address of protocol LIF; will be derived if not specified               | 10.0.0.2   |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``svm``                      | Storage virtual machine to use (req, if management LIF is a cluster LIF)   | svm_nfs    |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``username``                 | Username to connect to the storage device                                  | vsadmin    |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``password``                 | Password to connect to the storage device                                  | secret     |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``aggregate``                | Aggregate for provisioning (optional; if set, must be assigned to the SVM) | aggr1      |
+------------------------------+----------------------------------------------------------------------------+------------+
| ``limitAggregateUsage``      | Fail provisioning if usage is above this percentage (optional)             | 75%        |
+------------------------------+----------------------------------------------------------------------------+------------+

A fully-qualified domain name (FQDN) can be specified for the ``managementLIF`` option. For the ``ontap-nas*``
drivers only, a FQDN may also be specified for the ``dataLIF`` option, in which case the FQDN will
be used for the NFS mount operations. For the ``ontap-san*`` drivers, the default is to use all data LIF IPs from
the SVM and to use iSCSI multipath. Specifying an IP address for the ``dataLIF`` for the ``ontap-san*``
drivers forces the driver to disable multipath and use only the specified address.

For the ``ontap-nas-flexgroup driver``, the ``aggregate`` option in the configuration file is ignored.
All aggregates assigned to the SVM are used to provision a FlexGroup Volume.

For the ``ontap-nas`` and ``ontap-nas-economy`` drivers, an additional top level option is available.
For NFS host configuration, see also: http://www.netapp.com/us/media/tr-4067.pdf

+------------------------------+--------------------------------------------------------------------------+------------+
| Option                       | Description                                                              | Example    |
+==============================+==========================================================================+============+
| ``nfsMountOptions``          | Fine grained control of NFS mount options; defaults to "-o nfsvers=3"    |-o nfsvers=4|
+------------------------------+--------------------------------------------------------------------------+------------+

For the ``ontap-san*`` drivers, an additional top level option is available to specify an igroup.

+------------------------------+--------------------------------------------------------------------------+------------+
| Option                       | Description                                                              | Example    |
+==============================+==========================================================================+============+
| ``igroupName``               | The igroup used by the plugin; defaults to "netappdvp"                   | myigroup   |
+------------------------------+--------------------------------------------------------------------------+------------+

For the ``ontap-nas-economy`` driver, the ``limitVolumeSize`` option will additionally limit the size of the
FlexVols that it creates.

+------------------------------+--------------------------------------------------------------------------+------------+
| Option                       | Description                                                              | Example    |
+==============================+==========================================================================+============+
| ``limitVolumeSize``          | Maximum requestable volume size and qtree parent volume size             | 300g       |
+------------------------------+--------------------------------------------------------------------------+------------+

Also, when using ONTAP, these default option settings are available to avoid having to specify them on every volume create.

+------------------------------+--------------------------------------------------------------------------+------------+
| Defaults Option              | Description                                                              | Example    |
+==============================+==========================================================================+============+
| ``spaceReserve``             | Space reservation mode; "none" (thin provisioned) or "volume" (thick)    | none       |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``snapshotPolicy``           | Snapshot policy to use, default is "none"                                | none       |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``snapshotReserve``          | Snapshot reserve percentage, default is "" to accept ONTAP's default     | 10         |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``splitOnClone``             | Split a clone from its parent upon creation, defaults to "false"         | false      |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``encryption``               | Enable NetApp Volume Encryption, defaults to "false"                     | true       |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``unixPermissions``          | NAS option for provisioned NFS volumes, defaults to "777"                | 777        |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``snapshotDir``              | NAS option for access to the .snapshot directory, defaults to "false"    | false      |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``exportPolicy``             | NAS option for the NFS export policy to use, defaults to "default"       | default    |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``securityStyle``            | NAS option for access to the provisioned NFS volume, defaults to "unix"  | mixed      |
+------------------------------+--------------------------------------------------------------------------+------------+
| ``fileSystemType``           | SAN option to select the file system type, defaults to "ext4"            | xfs        |
+------------------------------+--------------------------------------------------------------------------+------------+

Scaling Options
---------------
The ``ontap-nas`` and ``ontap-san`` drivers create an ONTAP FlexVol for
each Docker volume. ONTAP supports up to 1000 FlexVols per cluster node
with a cluster maximum of 12,000 FlexVols. If your Docker volume
requirements fit within that limitation, the ``ontap-nas`` driver is the
preferred NAS solution due to the additional features offered by FlexVols
such as Docker-volume-granular snapshots and cloning.

If you need more Docker volumes than may be accommodated by the FlexVol
limits, choose the ``ontap-nas-economy`` or the ``ontap-san-economy`` driver.

The ``ontap-nas-economy`` driver creates Docker volumes as ONTAP Qtrees
within a pool of automatically managed FlexVols. Qtrees offer far
greater scaling, up to 100,000 per cluster node and 2,400,000 per cluster,
at the expense of some features. The ``ontap-nas-economy`` driver does not
support Docker-volume-granular snapshots or cloning.

.. note::
   The ``ontap-nas-economy`` driver is not currently supported in Docker Swarm,
   as Swarm does not orchestrate volume creation across multiple nodes.

The ``ontap-san-economy`` driver creates Docker volumes as ONTAP LUNs within
a shared pool of automatically managed FlexVols. This way, each FlexVol is
not restricted to only one LUN and it offers better scalability for
SAN workloads. Depending on the storage array, ONTAP supports up to 16384
LUNs per cluster. Since the volumes are LUNs underneath, this driver supports
Docker-volume-granular snapshots and cloning.

.. note::
   The ``ontap-san-economy`` driver is not currently supported in Docker Swarm,
   as Swarm does not orchestrate volume creation across multiple nodes.

Choose the ``ontap-nas-flexgroup`` driver to increase parallelism to a single
volume that can grow into the petabyte range with billions of files. Some
ideal use cases for FlexGroups include AI/ML/DL, big data and analytics,
software builds, streaming, file repositories, etc. Trident uses all
aggregates assigned to an SVM when provisioning a FlexGroup Volume. FlexGroup
support in Trident also has the following considerations:

* Requires ONTAP version 9.2 or greater.
* As of this writing, FlexGroups only support NFS v3.
* Recommended to enable the 64-bit NFSv3 identifiers for the SVM.
* The minimum recommended FlexGroup size is 100GB.
* Cloning is not supported for FlexGroup Volumes.

For information regarding FlexGroups and workloads that are appropriate for
FlexGroups see the `NetApp FlexGroup Volume - Best Practices and Implementation Guide`_.

.. _NetApp FlexGroup Volume - Best Practices and Implementation Guide: https://www.netapp.com/us/media/tr-4571.pdf

To get advanced features and huge scale in the same environment, you can run
multiple instances of the Docker Volume Plugin, with one using ``ontap-nas``
and another using ``ontap-nas-economy``.

Example ONTAP Config Files
--------------------------

**NFS Example for ontap-nas driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",
        "aggregate": "aggr1",
        "defaults": {
          "size": "10G",
          "spaceReserve": "none",
          "exportPolicy": "default"
        }
    }

**NFS Example for ontap-nas-flexgroup driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-flexgroup",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",
        "defaults": {
          "size": "100G",
          "spaceReserve": "none",
          "exportPolicy": "default"
        }
    }


**NFS Example for ontap-nas-economy driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas-economy",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "vsadmin",
        "password": "secret",
        "aggregate": "aggr1"
    }

**iSCSI Example for ontap-san driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi",
        "username": "vsadmin",
        "password": "secret",
        "aggregate": "aggr1",
        "igroupName": "myigroup"
    }

**iSCSI Example for ontap-san-economy driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san-economy",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi_eco",
        "username": "vsadmin",
        "password": "secret",
        "aggregate": "aggr1",
        "igroupName": "myigroup"
    }
