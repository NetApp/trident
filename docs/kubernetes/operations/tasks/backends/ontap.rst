############################
ONTAP (AFF/FAS/Select/Cloud)
############################

To create and use an ONTAP backend, you will need:

* A :ref:`supported ONTAP storage system <Supported backends (storage)>`
* Choose the :ref:`ONTAP storage driver <Choosing a driver>` that you want to
  use
* Complete `ONTAP backend preparation`_ for the driver of your choice
* Credentials to an ONTAP SVM with :ref:`appropriate access <User permissions>`

Choosing a driver
-----------------

=================== ========
Driver              Protocol
=================== ========
ontap-nas           NFS
ontap-nas-economy   NFS
ontap-nas-flexgroup NFS
ontap-san           iSCSI
ontap-san-economy   iSCSI
=================== ========

The ``ontap-nas`` and ``ontap-san`` drivers create an ONTAP FlexVol for each
PV. ONTAP supports up to 1000 FlexVols per cluster node with a cluster
maximum of 12,000 FlexVols. If your persistent volume requirements fit within
that limitation, those drivers are the preferred solution due to the granular
data management capabilities they afford.

If you need more persistent volumes than may be accommodated by the FlexVol
limits, choose the ``ontap-nas-economy`` or the ``ontap-san-economy`` driver.

The ``ontap-nas-economy`` driver creates PVs as ONTAP
Qtrees within a pool of automatically managed FlexVols. Qtrees offer far
greater scaling, up to 100,000 per cluster node and 2,400,000 per cluster, at
the expense of granular data management features.

The ``ontap-san-economy`` driver creates PVs as ONTAP LUNs within a pool of
automatically managed FlexVols. Each PV maps to an ONTAP LUN and this driver offers
higher scalability for SAN workloads. Depending on the storage array, ONTAP supports
up to 8192 LUNs per cluster node and 16384 LUNs for an HA pair. Since PVs map to LUNs
within shared FlexVols, Kubernetes VolumeSnapshots are created using ONTAP's FlexClone
technology. FlexClone LUNs and their parent LUNs share blocks, minimizing disk usage. 

Choose the ``ontap-nas-flexgroup`` driver to increase parallelism to a single volume
that can grow into the petabyte range with billions of files. Some ideal use cases
for FlexGroups include AI/ML/DL, big data and analytics, software builds, streaming,
file repositories, etc. Trident uses all aggregates assigned to an SVM when
provisioning a FlexGroup Volume. FlexGroup support in Trident also has the following
considerations:

.. note::

   The ``ontap-nas-flexgroup`` driver currently does not work with ONTAP 9.7

* Requires ONTAP version 9.2 or greater.
* As of this writing, FlexGroups only support NFSv3 (required to set
  ``mountOptions: ["nfsvers=3"]`` in the Kubernetes storage class).
* Recommended to enable the 64-bit NFSv3 identifiers for the SVM.
* The minimum recommended FlexGroup size is 100GB.
* Cloning is not supported for FlexGroup Volumes.

For information regarding FlexGroups and workloads that are appropriate for FlexGroups see the
`NetApp FlexGroup Volume - Best Practices and Implementation Guide`_.

.. _NetApp FlexGroup Volume - Best Practices and Implementation Guide: https://www.netapp.com/us/media/tr-4571.pdf

Remember that you can also run more than one driver, and create storage
classes that point to one or the other. For example, you could configure a
*Gold* class that uses the ``ontap-nas`` driver and a *Bronze* class that
uses the ``ontap-nas-economy`` one.

.. _ONTAP backend preparation:

Preparation
-----------

For all ONTAP backends, Trident requires at least one
`aggregate assigned to the SVM`_.

.. _aggregate assigned to the SVM: https://library.netapp.com/ecmdocs/ECMP1368404/html/GUID-5255E7D8-F420-4BD3-AEFB-7EF65488C65C.html

ontap-nas, ontap-nas-economy, ontap-nas-flexgroups
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

All of your Kubernetes worker nodes must have the appropriate NFS tools
installed. See the :ref:`worker configuration guide <NFS>` for more details.

Trident uses NFS `export policies`_ to control access to the volumes that it
provisions. It uses the ``default`` export policy unless a different export
policy name is specified in the configuration.

.. _export policies: https://library.netapp.com/ecmdocs/ECMP1196891/html/GUID-9A2B6C3E-C86A-4125-B778-6072A3A19657.html

While Trident associates new volumes (or qtrees) with the configured export
policy, it does not create or otherwise manage export policies themselves.
The export policy must exist before the storage backend is added to Trident,
and it needs to be configured to allow access to every worker node in the
Kubernetes cluster.

If the export policy is locked down to specific hosts, it will need to be
updated when new nodes are added to the cluster, and that access should be
removed when nodes are removed as well.

ontap-san, ontap-san-economy
^^^^^^^^^^^^^^^^^^^^^^^^^^^^

All of your Kubernetes worker nodes must have the appropriate iSCSI tools
installed. See the :ref:`worker configuration guide <iSCSI>` for more details.

Trident uses `igroups`_ to control access to the volumes (LUNs) that it
provisions. It expects to find an igroup called ``trident`` unless a different
igroup name is specified in the configuration.

.. _igroups: https://library.netapp.com/ecmdocs/ECMP1196995/html/GUID-CF01DCCD-2C24-4519-A23B-7FEF55A0D9A3.html

While Trident associates new LUNs with the configured igroup, it does not
create or otherwise manage igroups themselves. The igroup must exist before the
storage backend is added to Trident.

If Trident is configured to function as a
CSI Provisioner, Trident manages the addition of IQNs from worker nodes when
mounting PVCs. As and when PVCs are attached to pods running on a given node,
Trident adds the node's IQN to the igroup configured in your backend definition.

If Trident does not run as a CSI Provisioner, the igroup must be manually updated
to contain the iSCSI IQNs from every worker node in the Kubernetes cluster. The
igroup needs to be updated when new nodes are added to the cluster, and
they should be removed when nodes are removed as well.

Backend configuration options
-----------------------------

========================= ========================================================================================= ================================================
Parameter                 Description                                                                               Default
========================= ========================================================================================= ================================================
version                   Always 1
storageDriverName         "ontap-nas", "ontap-nas-economy", "ontap-nas-flexgroup", "ontap-san", "ontap-san-economy"
backendName               Custom name for the storage backend                                                       Driver name + "_" + dataLIF
managementLIF             IP address of a cluster or SVM management LIF                                             "10.0.0.1", "[2001:1234:abcd::fefe]"
dataLIF                   IP address of protocol LIF                                                                Derived by the SVM unless specified
svm                       Storage virtual machine to use                                                            Derived if an SVM managementLIF is specified
igroupName                Name of the igroup for SAN volumes to use                                                 "trident"
username                  Username to connect to the cluster/SVM
password                  Password to connect to the cluster/SVM
storagePrefix             Prefix used when provisioning new volumes in the SVM                                      "trident"
limitAggregateUsage       Fail provisioning if usage is above this percentage                                       "" (not enforced by default)
limitVolumeSize           Fail provisioning if requested volume size is above this value                            "" (not enforced by default)
nfsMountOptions           Comma-separated list of NFS mount options (except ontap-san)                              ""
========================= ========================================================================================= ================================================

A fully-qualified domain name (FQDN) can be specified for the ``managementLIF``
option. For the ``ontap-nas*`` drivers only, a FQDN may also be specified for
the ``dataLIF`` option, in which case the FQDN will be used for the NFS mount
operations.

The ``managementLIF`` and ``dataLIF`` options for all ONTAP drivers can
also be set to IPv6 addresses. Make sure to install Trident with the
``--use-ipv6`` flag. Care must be taken to define the ``managementLIF``
IPv6 address **within square brackets** as shown in the example above.

For the ``ontap-san*`` drivers, the default is to use all data LIF IPs from
the SVM and to use iSCSI multipath. Specifying an IP address for the ``dataLIF``
for the ``ontap-san*`` drivers forces them to disable multipath and use only the
specified address.

For the ``ontap-nas-economy`` and the ``ontap-san-economy``
drivers, the ``limitVolumeSize`` option will also restrict the maximum size of
the volumes it manages for qtrees and LUNs.

The ``nfsMountOptions`` parameter applies to all ONTAP drivers except ``ontap-san*``.
The mount options for Kubernetes persistent volumes are normally specified in
storage classes, but if no mount options are specified in a storage
class, Trident will fall back to using the mount options specified in the
storage backend's config file. If no mount options are specified in either the
storage class or the config file, then Trident will not set any
mount options on an associated persistent volume.

You can control how each volume is provisioned by default using these options
in a special section of the configuration. For an example, see the
configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
spaceAllocation           ontap-san* only: space-allocation for LUNs                      "true"
spaceReserve              Space reservation mode; "none" (thin) or "volume" (thick)       "none"
snapshotPolicy            Snapshot policy to use                                          "none"
snapshotReserve           Percentage of volume reserved for snapshots                     "0" if snapshotPolicy is "none", else ""
splitOnClone              Split a clone from its parent upon creation                     "false"
encryption                Enable NetApp volume encryption                                 "false"
unixPermissions           ontap-nas* only: mode for new volumes                           "777"
snapshotDir               ontap-nas* only: access to the .snapshot directory              "false"
exportPolicy              ontap-nas* only: export policy to use                           "default"
securityStyle             ontap-nas* only: security style for new volumes                 "unix"
tieringPolicy             Tiering policy to use                                           "none"; "snapshot-only" for pre-ONTAP 9.5 SVM-DR configuration
========================= =============================================================== ================================================

Example configurations
----------------------

**Example 1 - Minimal backend configuration for ontap drivers**

**NFS Example for ontap-nas driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "admin",
        "password": "secret",
        "nfsMountOptions": "nfsvers=4",
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
        "password": "secret"
    }

**iSCSI Example for ontap-san driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi",
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret"
    }

**iSCSI Example for ontap-san-economy driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san-economy",
        "managementLIF": "10.0.0.1",
        "svm": "svm_iscsi_eco",
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret"
    }


**Example 2 - Backend and storage class configuration for ontap drivers with virtual storage pools**

This example shows the backend definition file configured with virtual storage pools along with StorageClasses that
refer back to them.

In the sample backend definition file shown below, specific defaults are set for all storage pools, such as
``spaceReserve`` at ``none``, ``spaceAllocation`` at ``false``, and ``encryption`` at ``false``. The virtual storage
pools are defined in the ``storage`` section. In this example, some of the storage pool sets their own
``spaceReserve``, ``spaceAllocation``, and ``encryption`` values, and some pools overwrite the default values set above.

**NFS Example for ontap-nas driver with Virtual Pools**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "username": "admin",
        "password": "secret",
        "nfsMountOptions": "nfsvers=4",

        "defaults": {
              "spaceReserve": "none",
              "encryption": "false"
        },
        "labels":{"store":"nas_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"app":"msoffice", "cost":"100"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"app":"slack", "cost":"75"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"app":"wordpress", "cost":"50"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"app":"mysqldb", "cost":"25"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
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
              "spaceReserve": "none",
              "encryption": "false"
        },
        "labels":{"store":"flexgroup_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"protection":"gold", "creditpoints":"50000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"protection":"gold", "creditpoints":"30000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"protection":"silver", "creditpoints":"20000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"protection":"bronze", "creditpoints":"10000"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
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

        "defaults": {
              "spaceReserve": "none",
              "encryption": "false"
        },
        "labels":{"store":"nas_economy_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"department":"finance", "creditpoints":"6000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"department":"legal", "creditpoints":"5000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0755"
                }
            },
            {
                "labels":{"department":"engineering", "creditpoints":"3000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceReserve": "none",
                    "encryption": "true",
                    "unixPermissions": "0775"
                }
            },
            {
                "labels":{"department":"humanresource", "creditpoints":"2000"},
                "zone":"us_east_1d",
                "defaults": {
                    "spaceReserve": "volume",
                    "encryption": "false",
                    "unixPermissions": "0775"
                }
            }
        ]
    }

**iSCSI Example for ontap-san driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.3",
        "svm": "svm_iscsi",
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceAllocation": "false",
              "encryption": "false"
        },
        "labels":{"store":"san_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"protection":"gold", "creditpoints":"40000"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "true"
                }
            },
            {
                "labels":{"protection":"silver", "creditpoints":"20000"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceAllocation": "false",
                    "encryption": "true"
                }
            },
            {
                "labels":{"protection":"bronze", "creditpoints":"5000"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "false"
                }
            }
        ]
    }

**iSCSI Example for ontap-san-economy driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-san-economy",
        "managementLIF": "10.0.0.1",
        "svm": "svm_iscsi_eco",
        "igroupName": "trident",
        "username": "vsadmin",
        "password": "secret",

        "defaults": {
              "spaceAllocation": "false",
              "encryption": "false"
        },
        "labels":{"store":"san_economy_store"},
        "region": "us_east_1",
        "storage": [
            {
                "labels":{"app":"oracledb", "cost":"30"},
                "zone":"us_east_1a",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "true"
                }
            },
            {
                "labels":{"app":"postgresdb", "cost":"20"},
                "zone":"us_east_1b",
                "defaults": {
                    "spaceAllocation": "false",
                    "encryption": "true"
                }
            },
            {
                "labels":{"app":"mysqldb", "cost":"10"},
                "zone":"us_east_1c",
                "defaults": {
                    "spaceAllocation": "true",
                    "encryption": "false"
                }
            }
        ]
    }

The following StorageClass definitions refer to the above virtual storage pools. Using the ``parameters.selector`` field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the aspects defined in the chosen virtual pool.

* The first StorageClass (``protection-gold``) will map to the first, second virtual storage pool in ``ontap-nas-flexgroup`` backend and the first virtual storage pool in ``ontap-san`` backend . These are the only pool offering gold level protection.
* The second StorageClass (``protection-not-gold``) will map to the third, fourth virtual storage pool in ``ontap-nas-flexgroup`` backend and the second, third virtual storage pool in ``ontap-san`` backend . These are the only pool offering protection level other than gold.
* The third StorageClass (``app-mysqldb``) will map to the fourth virtual storage pool in ``ontap-nas`` backend and the third virtual storage pool in ``ontap-san-economy`` backend . These are the only pool offering storage pool configuration for mysqldb type app.
* The fourth StorageClass (``protection-silver-creditpoints-20k``) will map to the third virtual storage pool in ``ontap-nas-flexgroup`` backend and the second virtual storage pool in ``ontap-san`` backend . These are the only pool offering gold level protection at 20000 creditpoints.
* The fifth StorageClass (``creditpoints-5k``) will map to the second virtual storage pool in ``ontap-nas-economy`` backend and the third virtual storage pool in ``ontap-san`` backend. These are the only pool offerings at 5000 creditpoints.

Trident will decide which virtual storage pool is selected and will ensure the storage requirement is met.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-gold
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=gold"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-not-gold
    provisioner: netapp.io/trident
    parameters:
      selector: "protection!=gold"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: app-mysqldb
    provisioner: netapp.io/trident
    parameters:
      selector: "app=mysqldb"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: protection-silver-creditpoints-20k
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=silver; creditpoints=20000"
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: creditpoints-5k
    provisioner: netapp.io/trident
    parameters:
      selector: "creditpoints=5000"

User permissions
----------------

Trident expects to be run as either an ONTAP or SVM administrator, typically
using the ``admin`` cluster user or a ``vsadmin`` SVM user, or a user with a
different name that has the same role.

.. note::
  If you use the "limitAggregateUsage" option, cluster admin permissions are required.

While it is possible to create a more restrictive role within ONTAP that a
Trident driver can use, we don't recommend it. Most new releases of Trident
will call additional APIs that would have to be accounted for, making upgrades
difficult and error-prone.
