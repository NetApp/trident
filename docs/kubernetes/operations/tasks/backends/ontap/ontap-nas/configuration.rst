#####################
Backend Configuration
#####################

========================= ================================================================================================= ================================================
Parameter                 Description                                                                                       Default
========================= ================================================================================================= ================================================
version                   Always 1
storageDriverName         "ontap-nas", "ontap-nas-economy", "ontap-nas-flexgroup", "ontap-san", "ontap-san-economy"
backendName               Custom name for the storage backend                                                               Driver name + "_" + dataLIF
managementLIF             IP address of a cluster or SVM management LIF                                                     "10.0.0.1", "[2001:1234:abcd::fefe]"
dataLIF                   IP address of protocol LIF. **Use square brackets for IPv6**. Once set this **cannot be updated** Derived by the SVM unless specified
svm                       Storage virtual machine to use                                                                    Derived if an SVM managementLIF is specified
autoExportPolicy          Enable automatic export policy creation and updating [Boolean]                                    false
autoExportCIDRs           List of CIDRs to filter Kubernetes' node IPs against when autoExportPolicy is enabled             ["0.0.0.0/0", "::/0"]
username                  Username to connect to the cluster/SVM
password                  Password to connect to the cluster/SVM
storagePrefix             Prefix used when provisioning new volumes in the SVM. Once set this **cannot be updated**         "trident"
limitAggregateUsage       Fail provisioning if usage is above this percentage                                               "" (not enforced by default)
limitVolumeSize           Fail provisioning if requested volume size is above this value                                    "" (not enforced by default)
nfsMountOptions           Comma-separated list of NFS mount options                                                         ""
debugTraceFlags           Debug flags to use when troubleshooting. E.g.: {"api":false, "method":true}                       null
========================= ================================================================================================= ================================================

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

For the ``ontap-nas-economy`` driver, the ``limitVolumeSize``
option will also restrict the maximum size of
the volumes it manages for qtrees and LUNs.

The ``nfsMountOptions`` parameter can be used to specify mount options.
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
spaceReserve              Space reservation mode; "none" (thin) or "volume" (thick)       "none"
snapshotPolicy            Snapshot policy to use                                          "none"
snapshotReserve           Percentage of volume reserved for snapshots                     "0" if snapshotPolicy is "none", else ""
splitOnClone              Split a clone from its parent upon creation                     "false"
encryption                Enable NetApp volume encryption                                 "false"
unixPermissions           Mode for new volumes                                            "777"
snapshotDir               Access to the .snapshot directory                               "false"
exportPolicy              Export policy to use                                            "default"
securityStyle             Security style for new volumes                                  "unix"
tieringPolicy             Tiering policy to use                                           "none"; "snapshot-only" for pre-ONTAP 9.5 SVM-DR configuration
========================= =============================================================== ================================================

Here's an example that establishes default values:

.. code-block:: bash

  {
   "version": 1,
   "storageDriverName": "ontap-nas",
   "backendName": "customBackendName",
   "managementLIF": "10.0.0.1",
   "dataLIF": "10.0.0.2",
   "svm": "trident_svm",
   "username": "cluster-admin",
   "password": "password",
   "limitAggregateUsage": "80%",
   "limitVolumeSize": "50Gi",
   "nfsMountOptions": "nfsvers=4",
   "debugTraceFlags": {"api":false, "method":true},
   "defaults": {
       "spaceReserve": "volume",
       "exportPolicy": "myk8scluster",
       "snapshotPolicy": "default",
       "snapshotReserve": "10"
               }
  }
