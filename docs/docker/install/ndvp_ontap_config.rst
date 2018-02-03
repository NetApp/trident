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

+-----------------------+--------------------------------------------------------------------------+------------+
| Option                | Description                                                              | Example    |
+=======================+==========================================================================+============+
| ``managementLIF``     | IP address of ONTAP management LIF                                       | 10.0.0.1   |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``dataLIF``           | IP address of protocol LIF; will be derived if not specified             | 10.0.0.2   |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``svm``               | Storage virtual machine to use (req, if management LIF is a cluster LIF) | svm_nfs    |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``username``          | Username to connect to the storage device                                | vsadmin    |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``password``          | Password to connect to the storage device                                | netapp123  |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``aggregate``         | Aggregate to use for provisioning; it must be assigned to the SVM        | aggr1      |
+-----------------------+--------------------------------------------------------------------------+------------+

A fully-qualified domain name (FQDN) can be specified for the managementLIF and dataLIF options. The ontap-san driver
selects an IP address from the FQDN lookup for the dataLIF. The ontap-nas and ontap-nas-economy drivers use the
provided FQDN as the dataLIF for NFS mount operations.

For the ontap-nas and ontap-nas-economy drivers, an additional top level option is available.
For NFS host configuration, see also: http://www.netapp.com/us/media/tr-4067.pdf

+-----------------------+--------------------------------------------------------------------------+------------+
| Option                | Description                                                              | Example    |
+=======================+==========================================================================+============+
| ``nfsMountOptions``   | Fine grained control of NFS mount options; defaults to "-o nfsvers=3"    |-o nfsvers=4|
+-----------------------+--------------------------------------------------------------------------+------------+

Also, when using ONTAP, these default option settings are available to avoid having to specify them on every volume create.

+-----------------------+--------------------------------------------------------------------------+------------+
| Defaults Option       | Description                                                              | Example    |
+=======================+==========================================================================+============+
| ``spaceReserve``      | Space reservation mode; "none" (thin provisioned) or "volume" (thick)    | none       |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``snapshotPolicy``    | Snapshot policy to use, default is "none"                                | none       |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``splitOnClone``      | Split a clone from its parent upon creation, defaults to "false"         | false      |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``encryption``        | Enable NetApp Volume Encryption, defaults to "false"                     | true       |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``unixPermissions``   | NAS option for provisioned NFS volumes, defaults to "777"                | 777        |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``snapshotDir``       | NAS option for access to the .snapshot directory, defaults to "false"    | false      |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``exportPolicy``      | NAS option for the NFS export policy to use, defaults to "default"       | default    |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``securityStyle``     | NAS option for access to the provisioned NFS volume, defaults to "unix"  | mixed      |
+-----------------------+--------------------------------------------------------------------------+------------+
| ``fileSystemType``    | SAN option to select the file system type, defaults to "ext4"            | xfs        |
+-----------------------+--------------------------------------------------------------------------+------------+

Scaling Options
---------------
The ontap-nas and ontap-san drivers create an ONTAP FlexVol for each Docker volume. ONTAP supports up to 1000
FlexVols per cluster node with a cluster maximum of 12,000 FlexVols. If your Docker volume requirements fit within
that limitation, the ontap-nas driver is the preferred NAS solution due to the additional features offered by FlexVols
such as Docker-volume-granular snapshots and cloning.

If you need more Docker volumes than may be accommodated by the FlexVol limits, choose the ontap-nas-economy driver,
which creates Docker volumes as ONTAP Qtrees within a pool of automatically managed FlexVols. Qtrees offer far
greater scaling, up to 100,000 per cluster node and 2,400,000 per cluster, at the expense of some features.
The ontap-nas-economy driver does not support Docker-volume-granular snapshots or cloning. The ontap-nas-economy driver
is not currently supported in Docker Swarm, as Swarm does not orchestrate volume creation across multiple nodes.

To get advanced features and huge scale in the same environment, you can run multiple instances of the Docker Volume
Plugin, with one using ontap-nas and another using ontap-nas-economy.

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
        "password": "netapp123",
        "aggregate": "aggr1",
        "defaults": {
          "size": "10G",
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
        "password": "netapp123",
        "aggregate": "aggr1",
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
        "password": "netapp123",
        "aggregate": "aggr1"
    }
