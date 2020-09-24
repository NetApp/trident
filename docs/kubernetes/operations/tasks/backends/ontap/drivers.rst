#################
Choosing a driver
#################

To create and use an ONTAP backend, you will need:

* A :ref:`supported ONTAP storage system <Supported backends (storage)>`
* Choose the :ref:`ONTAP storage driver <Choosing a driver>` that you want to
  use
* Complete ONTAP backend preparation for the driver of your choice
* Credentials to an ONTAP SVM with :ref:`appropriate access <User permissions>`

Trident provides 5 unique storage drivers for communicating with ONTAP
clusters. Each driver handles the creation of volumes and access control
differently and their capabilities are detailed in this section.

=================== ======== ========== ======================
Driver              Protocol Mount Type Access Modes Supported
=================== ======== ========== ======================
ontap-nas           NFS      File       RWO,RWX,ROX
ontap-nas-economy   NFS      File       RWO,RWX,ROX
ontap-nas-flexgroup NFS      File       RWO,RWX,ROX
ontap-san           iSCSI    Block      RWO,ROX
ontap-san-economy   iSCSI    Block      RWO,ROX
=================== ======== ========== ======================

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
* Cloning FlexGroup volumes is supported with ONTAP 9.7 and above.

For information regarding FlexGroups and workloads that are appropriate for FlexGroups see the
`NetApp FlexGroup Volume - Best Practices and Implementation Guide`_.

.. _NetApp FlexGroup Volume - Best Practices and Implementation Guide: https://www.netapp.com/us/media/tr-4571.pdf
