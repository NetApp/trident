.. _upgrading-with-tridentctl:

#############################
Upgrading with ``tridentctl``
#############################

When upgrading to the latest release of Trident, there are a few items to consider:

1. Previous releases of Trident may have supported alpha Volume Snapshots. The 20.01
   release moves forward to only support `beta Volume Snapshots`_. Kubernetes admins
   must take care to safely backup or convert the alpha snapshot objects to the beta
   feature set if they would like to retain the legacy alpha snapshots.
2. The beta feature release of Volume Snapshots introduces a modified set of CRDs and
   a snapshot controller, both of which should be set up before installing Trident.

`This blog`_ discusses the steps involved in migrating alpha Volume Snapshots to the beta
format. It is purely meant to be a guiding document for users to understand the steps
involved should they choose to retain alpha Volume Snapshots.

Initiating an upgrade
---------------------

To upgrade, simply perform an uninstall followed by a reinstall to upgrade to the latest version of Trident.

.. warning::

   When upgrading, it is important you provide ``parameter.fsType`` in
   StorageClasses used by Trident. StorageClasses can be deleted and recreated
   without disrupting pre-existing volumes. This is a **requirement** for
   enforcing `Security Contexts <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_
   for SAN volumes. The `sample-input <https://github.com/NetApp/trident/tree/master/trident-installer/sample-input>`_
   directory contains examples, such as
   `storage-class-basic.yaml.templ <https://github.com/NetApp/trident/blob/master/trident-installer/sample-input/storage-class-basic.yaml.templ>`_,
   and `storage-class-bronze-default.yaml <https://github.com/NetApp/trident/blob/master/trident-installer/sample-input/storage-class-bronze-default.yaml>`_.
   For more information, take a look at the :ref:`Known issues <fstype-fix>` tab.

What happens when you upgrade
-----------------------------

By default the uninstall command will leave all of Trident's state intact by
not deleting the PVC and PV used by the Trident deployment, allowing an
uninstall followed by an install to act as an upgrade.

PVs that have already been provisioned will remain available while Trident is
offline, and Trident will provision volumes for any PVCs that are created in
the interim once it is back online.

.. _installer bundle: https://github.com/NetApp/trident/releases/latest

Next steps
----------

After upgrading, Trident will function as a CSI provisioner and provide a rich set of features that apply for CSI volumes. For legacy PVs (where legacy refers to non-CSI volumes that were created on Kubernetes versions ``1.11`` through ``1.13``), all features made available
by the previous Trident version will be supported.
To use all of Trident's capabilities, (such as :ref:`On-Demand Volume Snapshots <On-Demand Volume Snapshots>`), upgrade volumes using the ``tridentctl upgrade``
command.

The :ref:`Upgrading legacy volumes to CSI volumes <Upgrading legacy volumes to CSI volumes>`
section explains how legacy non-CSI volumes can be upgraded to the CSI type.

If you encounter any issues, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice.

Upgrading legacy volumes to CSI volumes
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When upgrading Trident, there is a possibility of having legacy volumes that need
to be ported to the CSI specification to make use of the complete set of
features made available by Trident. A legacy PV that has been provisioned
by Trident will still support the traditional set of features. For all additional features
that Trident will provide (such as :ref:`On-Demand Volume Snapshots <On-Demand Volume Snapshots>`),
the Trident volume must be upgraded from a NFS/iSCSI
type to the CSI type.

Things to consider when upgrading volumes
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When deciding to upgrade volumes to the CSI type, make sure to consider the
following:

- It may not be required to upgrade all existing volumes. Previously created
  volumes will still continue to be accessible and function normally.

- A PV can be mounted as part of a Deployment/StatefulSet when upgrading. It is
  not required to bring down the Deployment/StatefulSet.

- A PV **cannot** be attached to a standalone pod when upgrading. You will have to
  shut down the pod before upgrading the volume.

- To upgrade a volume, it must be bound to a PVC. Volumes that are not bound to PVCs
  will need to be removed and imported before upgrading.

Example volume upgrade
~~~~~~~~~~~~~~~~~~~~~~

Here is an example that shows how a volume upgrade is performed.

.. code-block:: bash

   $ kubectl get pv
   NAME                         CAPACITY     ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                  STORAGECLASS    REASON   AGE
   default-pvc-1-a8475          1073741824   RWO            Delete           Bound    default/pvc-1          standard                 19h
   default-pvc-2-a8486          1073741824   RWO            Delete           Bound    default/pvc-2          standard                 19h
   default-pvc-3-a849e          1073741824   RWO            Delete           Bound    default/pvc-3          standard                 19h
   default-pvc-4-a84de          1073741824   RWO            Delete           Bound    default/pvc-4          standard                 19h
   trident                      2Gi          RWO            Retain           Bound    trident/trident                                 19h

There are currently 4 PVs that have been created by Trident ``20.07``, using the
``netapp.io/trident`` provisioner.

.. code-block:: bash

   $ kubectl describe pv default-pvc-2-a8486

   Name:            default-pvc-2-a8486
   Labels:          <none>
   Annotations:     pv.kubernetes.io/provisioned-by: netapp.io/trident
                    volume.beta.kubernetes.io/storage-class: standard
   Finalizers:      [kubernetes.io/pv-protection]
   StorageClass:    standard
   Status:          Bound
   Claim:           default/pvc-2
   Reclaim Policy:  Delete
   Access Modes:    RWO
   VolumeMode:      Filesystem
   Capacity:        1073741824
   Node Affinity:   <none>
   Message:
   Source:
       Type:      NFS (an NFS mount that lasts the lifetime of a pod)
       Server:    10.xx.xx.xx
       Path:      /trid_1907_alpha_default_pvc_2_a8486
       ReadOnly:  false

The PV was created using the ``netapp.io/trident`` provisioner and is of type ``NFS``.
To support all new features provided by Trident, this PV will need to be upgraded
to the ``CSI`` type.

To upgrade a legacy Trident volume to the CSI spec, you must execute the
``tridenctl upgrade volume <name-of-trident-volume>`` command.

.. code-block:: bash

   $ ./tridentctl get volumes -n trident
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |            NAME     |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | default-pvc-2-a8486 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   | default-pvc-3-a849e | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   | default-pvc-1-a8475 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   | default-pvc-4-a84de | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+

   $ ./tridentctl upgrade volume default-pvc-2-a8486 -n trident
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |            NAME     |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | default-pvc-2-a8486 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   +---------------------+---------+---------------+----------+--------------------------------------+--------+---------+

After upgrading the PV, performing a ``kubectl describe pv`` will show
that the volume is a CSI volume.

.. code-block:: bash

   $ kubectl describe pv default-pvc-2-a8486
   Name:            default-pvc-2-a8486
   Labels:          <none>
   Annotations:     pv.kubernetes.io/provisioned-by: csi.trident.netapp.io
                    volume.beta.kubernetes.io/storage-class: standard
   Finalizers:      [kubernetes.io/pv-protection]
   StorageClass:    standard
   Status:          Bound
   Claim:           default/pvc-2
   Reclaim Policy:  Delete
   Access Modes:    RWO
   VolumeMode:      Filesystem
   Capacity:        1073741824
   Node Affinity:   <none>
   Message:
   Source:
       Type:              CSI (a Container Storage Interface (CSI) volume source)
       Driver:            csi.trident.netapp.io
       VolumeHandle:      default-pvc-2-a8486
       ReadOnly:          false
       VolumeAttributes:      backendUUID=c5a6f6a4-b052-423b-80d4-8fb491a14a22
                              internalName=trid_1907_alpha_default_pvc_2_a8486
                              name=default-pvc-2-a8486
                              protocol=file
   Events:                <none>

In this manner, volumes of the NFS/iSCSI type that were created by Trident
can be upgraded to the CSI type, on a per-volume basis.

.. _beta Volume Snapshots: https://kubernetes.io/docs/concepts/storage/volume-snapshots/
.. _this blog: https://netapp.io/2020/01/30/alpha-to-beta-snapshots/
