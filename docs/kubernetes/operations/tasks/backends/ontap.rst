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
=================

=================== ========
Driver              Protocol
=================== ========
ontap-nas           NFS
ontap-nas-economy   NFS
ontap-nas-flexgroup NFS
ontap-san           iSCSI
ontap-san-economy   iSCSI
=================== ========

.. note::
   Please refer to the `NetApp Hardware Universe <http://hwu.netapp.com>`_
   for up-to-date information on volume limits for your storage cluster.
   Based on the number of nodes, the platform and the version of ONTAP
   being used, these limits will vary.

The ``ontap-nas`` and ``ontap-san`` drivers create an ONTAP FlexVol for each
PV. If your persistent volume requirements fit within
that limitation, those drivers are the preferred solution due to the granular
data management capabilities they afford.

If you need more persistent volumes than may be accommodated by the FlexVol
limits, choose the ``ontap-nas-economy`` or the ``ontap-san-economy`` driver.

The ``ontap-nas-economy`` driver creates PVs as ONTAP
Qtrees within a pool of automatically managed FlexVols. Qtrees offer far
greater scaling at
the expense of granular data management features.

The ``ontap-san-economy`` driver creates PVs as ONTAP LUNs within a pool of
automatically managed FlexVols. Each PV maps to an ONTAP LUN and this driver offers
higher scalability for SAN workloads. Since PVs map to LUNs
within shared FlexVols, Kubernetes VolumeSnapshots are created using ONTAP's FlexClone
technology. FlexClone LUNs and their parent LUNs share blocks, minimizing disk usage.

Choose the ``ontap-nas-flexgroup`` driver to increase parallelism to a single volume
that can grow into the petabyte range with billions of files. Some ideal use cases
for FlexGroups include AI/ML/DL, big data and analytics, software builds, streaming,
file repositories, etc. Trident uses all aggregates assigned to an SVM when
provisioning a FlexGroup Volume. FlexGroup support in Trident also has the following
considerations:

* Requires ONTAP version 9.2 or greater.
* With ONTAP 9.7, FlexGroups work with NFSv4. For ONTAP clusters that are running
  9.6 and below, NFSv3 must be used (required to set
  ``mountOptions: ["nfsvers=3"]`` in the Kubernetes storage class).
* When using NFSv3, it is recommended to enable the 64-bit NFSv3 identifiers
  for the SVM.
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
===========

For all ONTAP backends, Trident requires at least one
`aggregate assigned to the SVM`_.

.. _aggregate assigned to the SVM: https://library.netapp.com/ecmdocs/ECMP1368404/html/GUID-5255E7D8-F420-4BD3-AEFB-7EF65488C65C.html

ontap-nas, ontap-nas-economy, ontap-nas-flexgroups
--------------------------------------------------

All of your Kubernetes worker nodes must have the appropriate NFS tools
installed. See the :ref:`worker configuration guide <NFS>` for more details.

Trident uses NFS `export policies`_ to control access to the volumes that it
provisions.

.. _export policies: https://library.netapp.com/ecmdocs/ECMP1196891/html/GUID-9A2B6C3E-C86A-4125-B778-6072A3A19657.html

Trident provides two options when working with export policies:

1. Trident can **dynamically manage the export policy itself**; in this mode of
   operation, the storage admin specifies a list of CIDR blocks that
   represent admissible IP addresses. Trident adds node IPs that fall in
   these ranges to the export policy automatically. Alternatively, when no
   CIDRs are specified, any global-scoped unicast IP found on the nodes will
   be added to the export policy.

2. Storage admins can create an export policy and add rules manually. Trident uses
   the ``default`` export policy unless a different export policy name is specified
   in the configuration.

With (1), Trident automates the management of export policies, creating an export
policy and taking care of additions and deletions of rules to the export policy based
on the worker nodes it runs on. As and when nodes are removed or added to the
Kubernetes cluster, Trident can be set up to permit access to the nodes, thus
providing a more robust way of managing access to the PVs it creates. Trident
will create one export policy per backend. **This feature requires CSI Trident**.

With (2), Trident does not create or otherwise manage export policies themselves.
The export policy must exist before the storage backend is added to Trident,
and it needs to be configured to allow access to every worker node in the
Kubernetes cluster. If the export policy is locked down to specific hosts,
it will need to be updated when new nodes are added to the cluster
and that access should be removed when nodes are removed as well.

Dynamic Export Policies with ONTAP NAS
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

The 20.04 release of CSI Trident provides the ability to dynamically manage
export policies for ONTAP backends. This provides the storage administrator
the ability to specify a permissible address space for worker node
IPs, rather than defining explicit rules manually. Since Trident automates the
export policy creation and configuration, it greatly simplifies export policy management for
the storage admin and the Kubernetes admin; modifications to the export policy no
longer require manual intervention on the storage cluster. Moreover, this helps
restrict access to the storage cluster only to worker nodes that have IPs in the
range specified, supporting a finegrained and automated managment.


Prerequisites
"""""""""""""

.. warning::

   The auto-management of export policies is only available for CSI Trident.
   It is important to ensure that **the worker nodes are not being NATed**.
   For Trident to discover the node IPs and add rules to the export policy
   dynamically, it **must be able to discover the node IPs**.

There are two configuration options that must be used. Here's an example backend
definition:

.. code::

   {
       "version": 1,
       "storageDriverName": "ontap-nas",
       "backendName": "ontap_nas_auto_export,
       "managementLIF": "192.168.0.135",
       "svm": "svm1",
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "autoExportCIDRs": ["192.168.0.0/24"],
       "autoExportPolicy": true
   }

.. warning::

   When using auto export policies, you must ensure that the root junction
   in your SVM has a pre-created export policy with an export rule that
   permits the node CIDR block (such as the ``default`` export policy). All
   volumes created by Trident are mounted under the root junction. Always
   follow NetApp's recommended best practice of dedicating a SVM for Trident.

How it works
""""""""""""

From the example shown above:

1. ``autoExportPolicy`` is set to ``true``. This indicates that Trident will
   create an export policy for the ``svm1`` SVM and handle the addition and
   deletion of rules using the ``autoExportCIDRs`` address blocks. The export
   policy will be named using the format ``trident-<uuid>``. For example, a backend
   with UUID ``403b5326-8482-40db-96d0-d83fb3f4daec`` and ``autoExportPolicy`` set
   to ``true`` will see Trident create an export policy named
   ``trident-403b5326-8482-40db-96d0-d83fb3f4daec`` on the SVM.

2. ``autoExportCIDRs`` contains a list of address blocks. **This field is
   optional and it defaults to** ``["0.0.0.0/0", "::/0"]``. **If not defined,
   Trident adds all globally-scoped unicast addresses found on the worker
   nodes**.

   In this example, the ``192.168.0.0/24`` address space is provided.
   This indicates that Kubernetes node IPs that fall within this address range
   will be added by Trident to the export policy it creates in (1).
   When Trident registers a node it runs on,
   it retrieves the IP addresses of the node and checks them against the address
   blocks provided in ``autoExportCIDRs``. After filtering the IPs, Trident creates
   export policy rules for the client IPs it discovers, with one rule for each node
   it identifies.

   The ``autoExportPolicy`` and ``autoExportCIDRs`` parameters can be updated for
   backends after they are created. You can append new CIDRs for a backend that's
   automatically managed or delete existing CIDRs. Exercise care **when deleting
   CIDRs to ensure that existing connections are not dropped**. You can also choose to disable
   ``autoExportPolicy`` for a backend and fall back to a manually created export
   policy. This will require setting the ``exportPolicy`` parameter in your backend
   config.

After Trident creates/updates a backend, you can check the backend using ``tridentctl``
or the corresponding tridentbackend CRD:

.. code-block:: bash

   $ ./tridentctl get backends ontap_nas_auto_export -n trident -o yaml
   items:
   - backendUUID: 403b5326-8482-40db-96d0-d83fb3f4daec
     config:
       aggregate: ""
       autoExportCIDRs:
       - 192.168.0.0/24
       autoExportPolicy: true
       backendName: ontap_nas_auto_export
       chapInitiatorSecret: ""
       chapTargetInitiatorSecret: ""
       chapTargetUsername: ""
       chapUsername: ""
       dataLIF: 192.168.0.135
       debug: false
       debugTraceFlags: null
       defaults:
         encryption: "false"
         exportPolicy: <automatic>
         fileSystemType: ext4

Updating your Kubernetes cluster configuration
""""""""""""""""""""""""""""""""""""""""""""""

As nodes are added to a Kubernetes cluster and registered with the Trident controller,
export policies of existing backends are updated (provided they fall in the address
range specified in the ``autoExportCIDRs`` for the backend). The CSI Trident daemonset
spins up a pod on all available nodes in the Kuberentes cluster.
Upon registering an eligible node, Trident checks if it contains IP addresses in the
CIDR block that is allowed on a per-backend basis. Trident then updates the export policies
of all possible backends, adding a rule for each node that meets the criteria.

A similar workflow is observed when nodes are deregistered from the Kubernetes cluster.
When a node is removed, Trident checks all backends that are online to remove the access rule
for the node. By removing this node IP from the export policies of managed backends, Trident
prevents rogue mounts, unless this IP is reused by a new node in the cluster.

Updating legacy backends
""""""""""""""""""""""""

For previously existing backends, updating the backend with ``tridentctl update backend``
will ensure Trident manages the export policies automatically. This will create a new export
policy named after the backend's UUID and volumes that are present on the backend will use
the newly created export policy when they are mounted again.

.. note::

   Deleting a backend with auto managed export policies will delete the dynamically
   created export policy. If the backend is recreated, it is treated as a new backend
   and will result in the creation of a new export policy.

.. note::

   If the IP address of a live node is updated, you must restart the Trident pod
   on the node. Trident will then update the export policy for backends it manages
   to reflect this IP change.

ontap-san, ontap-san-economy
----------------------------

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

Trident can authenticate iSCSI sessions with CHAP beginning with 20.04
for the ``ontap-san`` and ``ontap-san-economy`` drivers. This requires enabling the
``useCHAP`` option in your backend definition. When set to ``true``, Trident
configures the SVM's default initiator security to CHAP and set
the username and secrets from the backend file. The section below explains this
in detail.

Using CHAP with ONTAP SAN drivers
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. warning::

  A bug with ``20.04`` prevents bidirectional CHAP from working as expected.
  This is fixed with ``20.07``. Users can still define all 4 CHAP parameters in
  their backend config and have Trident maintain **unidirectional CHAP** for
  authentication with ``20.04``. Trident will only use the ``chapUsername`` and
  ``chapInitiatorSecret``. By defining all 4 parameters in the backend, you can
  directly upgrade to ``20.07`` to use bidirectional CHAP.
  You can find more details about the bug `here <https://github.com/NetApp/trident/issues/404>`_.

Trident 20.04 introduces CHAP support for the ``ontap-san`` and
``ontap-san-economy`` drivers. This simplifies the configuration of CHAP on the ONTAP
cluster and provides a convenient method of creating CHAP credentials and rotating
them using ``tridentctl``. Enabling CHAP on the ONTAP backend requires adding the
``useCHAP`` option and the CHAP secrets in your backend configuration as shown below:

Configuration
"""""""""""""

.. code::

   {
       "version": 1,
       "storageDriverName": "ontap-san",
       "backendName": "ontap_san_chap",
       "managementLIF": "192.168.0.135",
       "svm": "ontap_iscsi_svm",
       "useCHAP": true,
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "igroupName": "trident",
       "chapInitiatorSecret": "cl9qxIm36DKyawxy",
       "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
       "chapTargetUsername": "iJF4heBRT0TCwxyz",
       "chapUsername": "uh2aNCLSd6cNwxyz",
   }

.. warning::

   The ``useCHAP`` parameter is a Boolean option that can be configured only once.
   It is set to ``false`` by default. Once set to ``true``, it cannot be set to
   ``false``. NetApp recommends using Bidirectional CHAP to authenticate connections.

In addition to ``useCHAP=true``, the ``chapInitiatorSecret``,
``chapTargetInitiatorSecret``, ``chapTargetUsername`` and ``chapUsername``
fields **must be included** in the backend definition. The secrets can
be changed after a backend is created using ``tridentctl update``.

How it works
""""""""""""

By setting ``useCHAP`` to ``true``, the storage administrator instructs Trident to
configure CHAP on the storage backend. This includes:

1. Setting up CHAP on the SVM:

   a. If the SVM's default initiator security type is ``none`` (set by default)
      **AND** there are no pre-existing LUNs already present in the volume,
      Trident will set the default security type to ``CHAP`` and proceed to
      step 2.
   b. If the SVM contains LUNs, Trident **will not enable CHAP** on the SVM.
      This ensures that access to LUNs that are already present on the SVM isn't
      restricted.

2. Configuring the CHAP initiator and target username and secrets; these options must
   be specified in the backend configuration (as shown above).
3. Managing the addition of inititators to the ``igroupName`` given in the backend. If
   unspecified, this defaults to ``trident``.

Once the backend is created, Trident creates a corresponding ``tridentbackend`` CRD
and stores the CHAP secrets and usernames as Kubernetes secrets. All PVs that are created
by Trident on this backend will be mounted and attached over CHAP.

Rotating credentials and updating backends
""""""""""""""""""""""""""""""""""""""""""

The CHAP credentials can be rotated by updating the CHAP parameters in
the ``backend.json`` file. This will require updating the CHAP secrets
and using the ``tridentctl update`` command to reflect these changes.

.. warning::

   When updating the CHAP secrets for a backend, you **must use**
   ``tridentctl`` to update the backend. **Do not** update the credentials
   on the storage cluster through the CLI/ONTAP UI as Trident
   will not be able to pick up these changes.

.. code-block:: console

   $ cat backend-san.json
   {
       "version": 1,
       "storageDriverName": "ontap-san",
       "backendName": "ontap_san_chap",
       "managementLIF": "192.168.0.135",
       "svm": "ontap_iscsi_svm",
       "useCHAP": true,
       "username": "vsadmin",
       "password": "FaKePaSsWoRd",
       "igroupName": "trident",
       "chapInitiatorSecret": "cl9qxUpDaTeD",
       "chapTargetInitiatorSecret": "rqxigXgkeUpDaTeD",
       "chapTargetUsername": "iJF4heBRT0TCwxyz",
       "chapUsername": "uh2aNCLSd6cNwxyz",
   }

   $ ./tridentctl update backend ontap_san_chap -f backend-san.json -n trident
   +----------------+----------------+--------------------------------------+--------+---------+
   |   NAME         | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
   +----------------+----------------+--------------------------------------+--------+---------+
   | ontap_san_chap | ontap-san      | aa458f3b-ad2d-4378-8a33-1a472ffbeb5c | online |       7 |
   +----------------+----------------+--------------------------------------+--------+---------+

Existing connections will remain unaffected; they will continue to remain active if the credentials
are updated by Trident on the SVM. New connections will use the updated credentials and existing
connections continue to remain active. Disconnecting and reconnecting old PVs will result in them
using the updated credentials.

Backend configuration options
=============================

========================= ========================================================================================= ================================================
Parameter                 Description                                                                               Default
========================= ========================================================================================= ================================================
version                   Always 1
storageDriverName         "ontap-nas", "ontap-nas-economy", "ontap-nas-flexgroup", "ontap-san", "ontap-san-economy"
backendName               Custom name for the storage backend                                                       Driver name + "_" + dataLIF
managementLIF             IP address of a cluster or SVM management LIF                                             "10.0.0.1", "[2001:1234:abcd::fefe]"
dataLIF                   IP address of protocol LIF. **Use square brackets for IPv6**                              Derived by the SVM unless specified
useCHAP                   Use CHAP to authenticate iSCSI for ONTAP SAN drivers [Boolean]                            false
chapInitiatorSecret       CHAP initiator secret. Required if ``useCHAP=true``                                       ""
chapTargetInitiatorSecret CHAP target initiator secret. Required if ``useCHAP=true``                                ""
chapUsername              Inbound username. Required if ``useCHAP=true``                                            ""
chapTargetUsername        Target username. Required if ``useCHAP=true``                                             ""
svm                       Storage virtual machine to use                                                            Derived if an SVM managementLIF is specified
igroupName                Name of the igroup for SAN volumes to use                                                 "trident"
autoExportPolicy          Enable automatic export policy creation and updating [Boolean]                            false
autoExportCIDRs           List of CIDRs to filter Kubernetes' node IPs against when autoExportPolicy is enabled     ["0.0.0.0/0", "::/0"]
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

The ``managementLIF`` for all ONTAP drivers can
also be set to IPv6 addresses. Make sure to install Trident with the
``--use-ipv6`` flag. Care must be taken to define the ``managementLIF``
IPv6 address **within square brackets**.

.. warning::

   When using IPv6 addresses, make sure the ``managementLIF`` and ``dataLIF``
   [if included in your backend defition] are defined
   within square brackets, such as ``[28e8:d9fb:a825:b7bf:69a8:d02f:9e7b:3555]``.
   If the ``dataLIF`` is not provided, Trident will fetch the IPv6 data LIFs
   from the SVM.

For the ``ontap-san*`` drivers, the default is to use all data LIF IPs from
the SVM and to use iSCSI multipath. Specifying an IP address for the ``dataLIF``
for the ``ontap-san*`` drivers forces them to disable multipath and use only the
specified address.

Using the ``autoExportPolicy`` and ``autoExportCIDRs`` options, CSI Trident can
manage export policies automatically. This is supported for the ``ontap-nas-*``
drivers and explained in the
:ref:`Dynamic Export Policies <Dynamic Export Policies with ONTAP NAS>`
section.

To enable the ``ontap-san*`` drivers to use CHAP, set the ``useCHAP`` parameter to
``true`` in your backend definition. Trident will then configure and use
CHAP as the default authentication for the SVM given in the backend.
The :ref:`CHAP with ONTAP SAN drivers<Using CHAP with ONTAP SAN drivers>`
section explains how this works.

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
======================

**Example 1 - Minimal backend configuration for ontap drivers**

**NFS Example for ontap-nas driver with auto export policy**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "ontap-nas",
        "managementLIF": "10.0.0.1",
        "dataLIF": "10.0.0.2",
        "svm": "svm_nfs",
        "autoExportPolicy": true,
        "autoExportCIDRs": ["10.0.0.0/24"],
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

**NFS Example for ontap-nas driver with IPv6**

.. code-block:: json

   {
    "version": 1,
    "storageDriverName": "ontap-nas",
    "backendName": "nas_ipv6_backend",
    "managementLIF": "[5c5d:5edf:8f:7657:bef8:109b:1b41:d491]",
    "svm": "nas_ipv6_svm",
    "username": "vsadmin",
    "password": "netapp123"
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
        "useCHAP": true,
        "chapInitiatorSecret": "cl9qxIm36DKyawxy",
        "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
        "chapTargetUsername": "iJF4heBRT0TCwxyz",
        "chapUsername": "uh2aNCLSd6cNwxyz",
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
        "useCHAP": true,
        "chapInitiatorSecret": "cl9qxIm36DKyawxy",
        "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
        "chapTargetUsername": "iJF4heBRT0TCwxyz",
        "chapUsername": "uh2aNCLSd6cNwxyz",
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
        "useCHAP": true,
        "chapInitiatorSecret": "cl9qxIm36DKyawxy",
        "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
        "chapTargetUsername": "iJF4heBRT0TCwxyz",
        "chapUsername": "uh2aNCLSd6cNwxyz",
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
        "useCHAP": true,
        "chapInitiatorSecret": "cl9qxIm36DKyawxy",
        "chapTargetInitiatorSecret": "rqxigXgkesIpwxyz",
        "chapTargetUsername": "iJF4heBRT0TCwxyz",
        "chapUsername": "uh2aNCLSd6cNwxyz",
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
================

Trident expects to be run as either an ONTAP or SVM administrator, typically
using the ``admin`` cluster user or a ``vsadmin`` SVM user, or a user with a
different name that has the same role.

.. note::
  If you use the "limitAggregateUsage" option, cluster admin permissions are required.

While it is possible to create a more restrictive role within ONTAP that a
Trident driver can use, we don't recommend it. Most new releases of Trident
will call additional APIs that would have to be accounted for, making upgrades
difficult and error-prone.
