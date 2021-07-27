.. _ontap-nas-configuration-parameters:

###############################
ONTAP NAS Backend Configuration
###############################

========================= ================================================================================================= ================================================
Parameter                 Description                                                                                       Default
========================= ================================================================================================= ================================================
version                   Always 1
storageDriverName         "ontap-nas", "ontap-nas-economy", "ontap-nas-flexgroup", "ontap-san", "ontap-san-economy"
backendName               Custom name for the storage backend                                                               Driver name + "_" + dataLIF
managementLIF             IP address of a cluster or SVM management LIF                                                     "10.0.0.1", "[2001:1234:abcd::fefe]"
dataLIF                   IP address of protocol LIF. **Use square brackets for IPv6**. Once set this **cannot be updated** Derived by the SVM unless specified
svm                       Storage virtual machine to use                                                                    Derived if an SVM managementLIF is specified
labels                    Set of arbitrary JSON-formatted labels to apply on volumes.                                       ""
autoExportPolicy          Enable automatic export policy creation and updating [Boolean]                                    false
autoExportCIDRs           List of CIDRs to filter Kubernetes' node IPs against when autoExportPolicy is enabled             ["0.0.0.0/0", "::/0"]
clientCertificate         Base64-encoded value of client certificate. Used for certificate-based auth.                      ""
clientPrivateKey          Base64-encoded value of client private key. Used for certificate-based auth.                      ""
trustedCACertificate      Base64-encoded value of trusted CA certificate. Optional. Used for certificate-based auth.        ""
username                  Username to connect to the cluster/SVM. Used for credential-based auth.
password                  Password to connect to the cluster/SVM. Used for credential-based auth.
storagePrefix             Prefix used when provisioning new volumes in the SVM. Once set this **cannot be updated**         "trident"
limitAggregateUsage       Fail provisioning if usage is above this percentage                                               "" (not enforced by default)
limitVolumeSize           Fail provisioning if requested volume size is above this value                                    "" (not enforced by default)
nfsMountOptions           Comma-separated list of NFS mount options                                                         ""
qtreesPerFlexvol          Maximum qtrees per FlexVol, must be in range [50, 300]                                            "200"
debugTraceFlags           Debug flags to use when troubleshooting. E.g.: {"api":false, "method":true}                       null
useREST                   Boolean parameter to use ONTAP REST APIs. **Tech-preview**                                            false
========================= ================================================================================================= ================================================

.. note::

   ``useREST`` is provided as a **tech-preview** that is recommended for test environments and not for **production workloads**. When set to ``true``, Trident will use ONTAP REST APIs to communicate with the backend. This feature requires ONTAP 9.8 and later. In addition, the ONTAP login role used must have access to the ``ontap`` application. This is satisfied by pre-defined ``vsadmin`` and ``cluster-admin`` roles.

To communicate with the ONTAP cluster, Trident must be provided with authentication
parameters. This could be the username/password to a security login (OR) an
installed certificate. This is fully documented in the
:ref:`Authentication Guide <ontap-nas-authentication>`.

.. warning::

  Do not use ``debugTraceFlags`` unless you are troubleshooting and require a
  detailed log dump.

.. note::

   When creating a backend, remember that the ``dataLIF`` and ``storagePrefix``
   cannot be modified after creation. To update these parameters you will need
   to create a new backend.

A fully-qualified domain name (FQDN) can be specified for the ``managementLIF``
option. A FQDN may also be specified for the ``dataLIF`` option, in which case
the FQDN will be used for the NFS mount operations. This way you can create a
round-robin DNS to load-balance across multiple data LIFs.

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

Using the ``autoExportPolicy`` and ``autoExportCIDRs`` options, CSI Trident can
manage export policies automatically. This is supported for all ``ontap-nas-*``
drivers and explained in the
:ref:`Dynamic Export Policies <Dynamic Export Policies with ONTAP NAS>`
section.

For the ``ontap-nas-economy`` driver, the ``limitVolumeSize`` option will also
restrict the maximum size of the volumes it manages for qtrees and LUNs, and
the ``qtreesPerFlexvol`` option allows customizing the maximum number of qtrees
per FlexVol.

The ``nfsMountOptions`` parameter can be used to specify mount options.
The mount options for Kubernetes persistent volumes are normally specified in
storage classes, but if no mount options are specified in a storage
class, Trident will fall back to using the mount options specified in the
storage backend's config file. If no mount options are specified in either the
storage class or the config file, then Trident will not set any
mount options on an associated persistent volume.

.. note::

  Trident sets provisioning labels in the "Comments" field of all volumes
  created using the ``ontap-nas`` and ``ontap-nas-flexgroup``. Based on the driver
  used, the comments are set on the FlexVol (``ontap-nas``) or FlexGroup
  (``ontap-nas-flexgroup``). Trident will copy all labels present on a storage
  pool to the storage volume at the time it is provisioned.
  Storage admins can define labels per storage pool and group all volumes
  created in a storage pool. This provides a convenient way of differentiating
  volumes based on a set of customizable labels that are provided in the backend
  configuration.

You can control how each volume is provisioned by default using these options
in a special section of the configuration. For an example, see the
configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
spaceReserve              Space reservation mode; "none" (thin) or "volume" (thick)       "none"
snapshotPolicy            Snapshot policy to use                                          "none"
qosPolicy                 QoS policy group to assign for volumes created.
                          Choose one of ``qosPolicy`` or ``adaptiveQosPolicy`` per
                          storage pool/backend.                                           ""
adaptiveQosPolicy         Adaptive QoS policy group to assign for volumes created.
                          **Not supported by ontap-nas-economy**. Choose one of
                          ``qosPolicy`` or ``adaptiveQosPolicy`` per storage
                          pool/backend.                                                   ""
snapshotReserve           Percentage of volume reserved for snapshots                     "0" if snapshotPolicy is "none", else ""
splitOnClone              Split a clone from its parent upon creation                     "false"
encryption                Enable NetApp volume encryption                                 "false"
unixPermissions           Mode for new volumes                                            "777"
snapshotDir               Controls visibility of the .snapshot directory                  "false"
exportPolicy              Export policy to use                                            "default"
securityStyle             Security style for new volumes                                  "unix"
tieringPolicy             Tiering policy to use                                           "none"; "snapshot-only" for pre-ONTAP 9.5 SVM-DR configuration
========================= =============================================================== ================================================

.. _ontap-nas-snapshot-reserve:

When ``snapshotReserve`` is set on a backend using the ``ontap-nas`` storage
driver, volumes created by Trident will have a portion of the requested size
dedicated for volume snapshots. This includes ONTAP snapshots scheduled through ONTAP,
as well as :ref:`Kubernetes VolumeSnapshots<On-Demand Volume Snapshots>`.
For example, a 2GiB PVC request will always result in the creation of a 2GiB FlexVol.
If ``snapshotReserve=20``, the amount of available space visible to the end user
will be 1.6GiB. Users are required to calculate the size of the PVC by factoring
the amount of ``snapshotReserve`` configured. Existing volumes can be
:ref:`resized<Volume Expansion>` through Trident to grow usable space available
on the volume. Because ``snapshotReserve`` is a soft limit on the amount of space
reserved for ONTAP snapshots, it is also possible that the filesystem space gets eaten into when
the space used by snapshots grows beyond the reserve. This applies to ONTAP snapshots
taken on the storage volume, as well as :ref:`Kubernetes VolumeSnapshots<On-Demand Volume Snapshots>`.
To accommodate this behavior, users can choose to grow their volumes by resizing.
Alternatively, users can also free up space by deleting snapshots that are not
required.

.. note::

  Using QoS policy groups with Trident requires ONTAP 9.8 or later.
  It is recommended to use a **non-shared** QoS policy group and ensure the policy
  group is applied to each constituent **individually**. A shared QoS policy group
  will enforce the ceiling for the **total throughput** of all workloads.

Here's an example that establishes default values:

.. code-block:: bash

  {
    "version": 1,
    "storageDriverName": "ontap-nas",
    "backendName": "customBackendName",
    "managementLIF": "10.0.0.1",
    "dataLIF": "10.0.0.2",
    "labels": {"k8scluster": "dev1", "backend": "dev1-nasbackend"},
    "svm": "trident_svm",
    "username": "cluster-admin",
    "password": "password",
    "limitAggregateUsage": "80%",
    "limitVolumeSize": "50Gi",
    "nfsMountOptions": "nfsvers=4",
    "debugTraceFlags": {"api":false, "method":true},
    "defaults": {
      "spaceReserve": "volume",
      "qosPolicy": "premium",
      "exportPolicy": "myk8scluster",
      "snapshotPolicy": "default",
      "snapshotReserve": "10"
    }
  }
