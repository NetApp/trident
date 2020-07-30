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
useCHAP                   Use CHAP to authenticate iSCSI for ONTAP SAN drivers [Boolean]                                    false
chapInitiatorSecret       CHAP initiator secret. Required if ``useCHAP=true``                                               ""
chapTargetInitiatorSecret CHAP target initiator secret. Required if ``useCHAP=true``                                        ""
chapUsername              Inbound username. Required if ``useCHAP=true``                                                    ""
chapTargetUsername        Target username. Required if ``useCHAP=true``                                                     ""
svm                       Storage virtual machine to use                                                                    Derived if an SVM managementLIF is specified
igroupName                Name of the igroup for SAN volumes to use                                                         "trident"username                  Username to connect to the cluster/SVM
password                  Password to connect to the cluster/SVM
storagePrefix             Prefix used when provisioning new volumes in the SVM. Once set this **cannot be updated**         "trident"
limitAggregateUsage       Fail provisioning if usage is above this percentage                                               "" (not enforced by default)
limitVolumeSize           Fail provisioning if requested volume size is above this value for the economy driver             "" (not enforced by default)
========================= ================================================================================================= ================================================

For the ``ontap-san*`` drivers, the default is to use all data LIF IPs from
the SVM and to use iSCSI multipath. Specifying an IP address for the ``dataLIF``
for the ``ontap-san*`` drivers forces them to disable multipath and use only the
specified address.

.. note::

   When creating a backend, remember that the ``dataLIF`` and ``storagePrefix``
   cannot be modified after creation. To update these parameters you will need
   to create a new backend.

A fully-qualified domain name (FQDN) can be specified for the ``managementLIF``
option.

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

To enable the ``ontap-san*`` drivers to use CHAP, set the ``useCHAP`` parameter to
``true`` in your backend definition. Trident will then configure and use
bidirectional CHAP as the default authentication for the SVM given in the backend.
The :ref:`CHAP with ONTAP SAN drivers<Using CHAP with ONTAP SAN drivers>`
section explains how this works.

For the ``ontap-san-economy`` driver, the ``limitVolumeSize``
option will also restrict the maximum size of
the volumes it manages for qtrees and LUNs.

You can control how each volume is provisioned by default using these options
in a special section of the configuration. For an example, see the
configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
spaceAllocation           Space-allocation for LUNs                                       "true"
spaceReserve              Space reservation mode; "none" (thin) or "volume" (thick)       "none"
snapshotPolicy            Snapshot policy to use                                          "none"
snapshotReserve           Percentage of volume reserved for snapshots                     "0" if snapshotPolicy is "none", else ""
splitOnClone              Split a clone from its parent upon creation                     "false"
encryption                Enable NetApp volume encryption                                 "false"
securityStyle             Security style for new volumes                                  "unix"
tieringPolicy             Tiering policy to use                                           "none"; "snapshot-only" for pre-ONTAP 9.5 SVM-DR configuration
========================= =============================================================== ================================================
