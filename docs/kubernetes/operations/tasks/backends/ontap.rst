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

================= ========
Driver            Protocol
================= ========
ontap-nas         NFS
ontap-nas-economy NFS
ontap-san         iSCSI
================= ========

The ``ontap-nas`` and ``ontap-san`` drivers create an ONTAP FlexVol for each
volume. ONTAP supports up to 1000 FlexVols per cluster node with a cluster
maximum of 12,000 FlexVols. If your persistent volume requirements fit within
that limitation, those drivers are the preferred solution due to the granular
data management capabilities they afford.

If you need more persistent volumes than may be accommodated by the FlexVol
limits, choose the ``ontap-nas-economy`` driver, which creates volumes as ONTAP
Qtrees within a pool of automatically managed FlexVols. Qtrees offer far
greater scaling, up to 100,000 per cluster node and 2,400,000 per cluster, at
the expense of granular data management features.

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

ontap-nas and ontap-nas-economy
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

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

ontap-san
^^^^^^^^^

All of your Kubernetes worker nodes must have the appropriate iSCSI tools
installed. See the :ref:`worker configuration guide <iSCSI>` for more details.

Trident uses `igroups`_ to control access to the volumes (LUNs) that it
provisions. It expects to find an igroup called ``trident`` unless a different
igroup name is specified in the configuration.

.. _igroups: https://library.netapp.com/ecmdocs/ECMP1196995/html/GUID-CF01DCCD-2C24-4519-A23B-7FEF55A0D9A3.html

While Trident associates new LUNs with the configured igroup, it does not
create or otherwise manage igroups themselves. The igroup must exist before the
storage backend is added to Trident, and it needs to contain the iSCSI IQNs
from every worker node in the Kubernetes cluster.

The igroup needs to be updated when new nodes are added to the cluster, and
they should be removed when nodes are removed as well.

Backend configuration options
-----------------------------

================== =============================================================== ================================================
Parameter          Description                                                     Default
================== =============================================================== ================================================
version            Always 1
storageDriverName  "ontap-nas", "ontap-nas-economy" or "ontap-san"
backendName        Custom name for the storage backend                             Driver name + "_" + dataLIF
managementLIF      IP address of a cluster or SVM management LIF                   "10.0.0.1"
dataLIF            IP address of protocol LIF                                      Derived by the SVM unless specified
svm                Storage virtual machine to use                                  Derived if an SVM managementLIF is specified
igroupName         Name of the igroup for SAN volumes to use                       "trident"
username           Username to connect to the cluster/SVM
password           Password to connect to the cluster/SVM
storagePrefix      Prefix used when provisioning new volumes in the SVM            "trident"
failureDomainZone  Failure domain zone corresponding to the backend
================== =============================================================== ================================================

A fully-qualified domain name (FQDN) can be specified for the managementLIF and dataLIF options. The ontap-san driver
selects an IP address from the FQDN lookup for the dataLIF. The ontap-nas and ontap-nas-economy drivers use the
provided FQDN as the dataLIF for NFS mount operations.

You can control how each volume is provisioned by default using these options
in a special section of the configuration. For an example, see the
configuration examples below.

================== =============================================================== ================================================
Parameter          Description                                                     Default
================== =============================================================== ================================================
spaceReserve       Space reservation mode; "none" (thin) or "volume" (thick)       "none"
snapshotPolicy     Snapshot policy to use                                          "none"
splitOnClone       Split a clone from its parent upon creation                     false
encryption         Enable NetApp volume encryption                                 false
unixPermissions    ontap-nas* only: mode for new volumes                           "777"
snapshotDir        ontap-nas* only: access to the .snapshot directory              false
exportPolicy       ontap-nas* only: export policy to use                           "default"
securityStyle      ontap-nas* only: security style for new volumes                 "unix"
================== =============================================================== ================================================

Example configuration
---------------------

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
        "defaults": {
          "spaceReserve": "volume",
          "exportPolicy": "myk8scluster"
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
        "password": "netapp123"
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
        "password": "netapp123"
    }

User permissions
----------------

Trident expects to be run as either an ONTAP or SVM administrator, typically
using the ``admin`` cluster user or a ``vsadmin`` SVM user, or a user with a
different name that has the same role.

While it is possible to create a more restrictive role within ONTAP that a
Trident driver can use, we don't recommend it. Most new releases of Trident
will call additional APIs that would have to be accounted for, making upgrades
difficult and error-prone.
