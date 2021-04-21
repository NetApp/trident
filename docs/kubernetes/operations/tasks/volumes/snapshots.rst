##########################
On-Demand Volume Snapshots
##########################

Beginning with the 20.01 release of Trident, it is now possible to use the
`Volume Snapshot feature`_ to create snapshots of PVs at the Kubernetes
layer. These snapshots can be used to maintain point-in-time copies of
volumes that have been created by Trident and can also be used to schedule
the creation of additional volumes (clones). This feature is available from
Kubernetes ``1.17`` [Beta] and is GA from ``1.20``.

.. note::

	Kubernetes Volume Snapshot is GA from Kubernetes ``1.20``. To understand the
	changes involved in moving from beta to GA, take a look at the `release blog
	<https://kubernetes.io/blog/2020/12/10/kubernetes-1.20-volume-snapshot-moves-to-ga/>`_.
	With the graduation to GA, the ``v1`` API version is introduced and is backward compatible
	with ``v1beta1`` snapshots.

Creating volume snapshots requires an external snapshot controller to be created,
as well as some Custom Resource Definitions (CRDs). **This is the responsibility of
the Kubernetes orchestrator that is being used (E.g., Kubeadm, GKE, OpenShift)**.
Check with your Kubernetes orchestrator to confirm these requirements are met.
You can create an external snapshot-controller and snapshot CRDs using the code
snippet below.

.. code-block:: bash

   $ cat snapshot-setup.sh
   #!/bin/bash
   # Create volume snapshot CRDs
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-3.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
   # Create the snapshot-controller in the desired namespace. Edit the YAML manifests below to modify namespace.
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-3.0/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-3.0/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml

CSI Snapshotter provides a
`Validating Webhook <https://github.com/kubernetes-csi/external-snapshotter#validating-webhook>`_
to help users validate existing ``v1beta1`` snapshots and confirm they are valid
resource objects. The validating webhook will automatically label invalid snapshot
objects and prevent the creation of future invalid objects. As with volume snapshot
CRDs and the snapshot controller, the validating webhook is deployed by the
Kubernetes orchestrator. Instructions to deploy the validating webhook manually
can be found `here <https://github.com/kubernetes-csi/external-snapshotter/blob/release-3.0/deploy/kubernetes/webhook-example/README.md>`_.
Examples of invalid snapshot manifests can be obtained in the
`examples <https://github.com/kubernetes-csi/external-snapshotter/tree/release-3.0/examples/kubernetes>`_
directory of the `external-snapshotter <https://github.com/kubernetes-csi/external-snapshotter/tree/release-3.0>`_.
repository.

.. note::

   Volume snapshot is supported by the ``ontap-nas``, ``ontap-san``,
   ``ontap-san-economy``, ``solidfire-san``, ``aws-cvs``, ``gcp-cvs``
   and ``azure-netapp-files`` drivers. This feature requires Kubernetes
   ``1.17`` and above.

Trident handles the creation of VolumeSnapshots for its drivers as explained
below:

  * For the ``ontap-nas``, ``ontap-san``, ``aws-cvs``, ``gcp-cvs`` and ``azure-netapp-files``
    drivers, each PV maps to a FlexVol. As a result, VolumeSnapshots are created
    as NetApp Snapshots. NetApp's Snapshot technology delivers more stability,
    scalability, recoverability, and performance than competing snapshot
    technologies. These Snapshot copies are extremely efficient both in the time
    needed to create them and in storage space.
  * For the ``ontap-san-economy`` driver, PVs map to LUNs created on shared
    FlexVols. VolumeSnapshots of PVs are achieved by performing FlexClones of
    the associated LUN. ONTAP's FlexClone technology makes it possible to create
    copies of even the largest datasets almost instantaneously. Copies share
    data blocks with their parents, consuming no storage except what is
    required for metadata.
  * For the ``solidfire-san`` driver, each PV maps to a LUN created on the
    Element/HCI cluster. VolumeSnapshots are represented by Element snapshots of
    the underlying LUN. These snapshots are point-in-time copies and only take
    up a small amount of system resources and space.

When working with the ``ontap-nas`` and ``ontap-san`` drivers, ONTAP snapshots are
point-in-time copies of the FlexVol and consume space on the FlexVol itself. This
can result in the amount of writable space in the volume to reduce with time as
snapshots are created/scheduled. One simple way of addressing this is to grow
the volume by :ref:`resizing through Kubernetes<Volume Expansion>`. Another
option is to delete snapshots that are no longer required. When a VolumeSnapshot
created through Kubernetes is deleted, Trident will delete the associated ONTAP
snapshot. ONTAP snapshots that were not created through Kubernetes can also be
deleted.

With Trident, you can use VolumeSnapshots to create new PVs from them. Creating
PVs from these snapshots is performed by using the FlexClone technology for
:ref:`supported <On-Demand Volume Snapshots>` ONTAP & CVS backends.
When creating a PV from a snapshot, the backing volume is a FlexClone of the
snapshot's parent volume. The ``solidfire-san`` driver uses ElementOS volume
clones to create PVs from snapshots. Here it creates a clone from the Element
snapshot.

The example detailed below explains the constructs required for working with
snapshots and shows how snapshots can be created and used.

Before creating a Volume Snapshot, a
:ref:`VolumeSnapshotClass <Kubernetes VolumeSnapshotClass Objects>` must be
set up.

.. code-block:: bash

   $ cat snap-sc.yaml
   #Use apiVersion v1 for Kubernetes 1.20 and above. For Kubernetes 1.17 - 1.19, use apiVersion v1beta1.
   apiVersion: snapshot.storage.k8s.io/v1
   kind: VolumeSnapshotClass
   metadata:
     name: csi-snapclass
   driver: csi.trident.netapp.io
   deletionPolicy: Delete

The ``driver`` points to Trident's CSI driver. The ``deletionPolicy`` can be set
to ``Delete`` or ``Retain``. When set to ``Retain``, the underlying physical snapshot
on the storage cluster is retained even when the ``VolumeSnapshot`` object is deleted.

Create a VolumeSnapshot
-----------------------

We can now create a snapshot of an existing PVC.

.. code-block:: bash

   $ cat snap.yaml
   #Use apiVersion v1 for Kubernetes 1.20 and above. For Kubernetes 1.17 - 1.19, use apiVersion v1beta1.
   apiVersion: snapshot.storage.k8s.io/v1
   kind: VolumeSnapshot
   metadata:
     name: pvc1-snap
   spec:
     volumeSnapshotClassName: csi-snapclass
     source:
       persistentVolumeClaimName: pvc1

The snapshot is being created for a PVC named ``pvc1``, and the
name of the snapshot is set to ``pvc1-snap``.

.. code-block:: bash

   $ kubectl create -f snap.yaml
   volumesnapshot.snapshot.storage.k8s.io/pvc1-snap created

   $ kubectl get volumesnapshots
   NAME                   AGE
   pvc1-snap              50s

This created a :ref:`VolumeSnapshot <Kubernetes VolumeSnapshot Objects>`
object. A VolumeSnapshot is analogous to a PVC and is associated with a
:ref:`VolumeSnapshotContent <Kubernetes VolumeSnapshotContent Objects>`
object that represents the actual snapshot.

It is possible to identify the VolumeSnapshotContent object for the
``pvc1-snap`` VolumeSnapshot by describing it.

.. code-block:: bash

   $ kubectl describe volumesnapshots pvc1-snap
   Name:         pvc1-snap
   Namespace:    default
   .
   .
   .
   Spec:
     Snapshot Class Name:    pvc1-snap
     Snapshot Content Name:  snapcontent-e8d8a0ca-9826-11e9-9807-525400f3f660
     Source:
       API Group:
       Kind:       PersistentVolumeClaim
       Name:       pvc1
   Status:
     Creation Time:  2019-06-26T15:27:29Z
     Ready To Use:   true
     Restore Size:   3Gi
   .
   .

The ``Snapshot Content Name`` identifies the VolumeSnapshotContent
object which serves this snapshot. The ``Ready To Use`` parameter indicates
that the Snapshot can be used to create a new PVC.

Create PVCs from VolumeSnapshots
--------------------------------

A PVC can be created using the snapshot as shown in the example below:

.. code-block:: bash

   $ cat pvc-from-snap.yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: pvc-from-snap
   spec:
     accessModes:
       - ReadWriteOnce
     storageClassName: golden
     resources:
       requests:
         storage: 3Gi
     dataSource:
       name: pvc1-snap
       kind: VolumeSnapshot
       apiGroup: snapshot.storage.k8s.io

The ``dataSource`` shows that the PVC must be created using a VolumeSnapshot
named ``pvc1-snap`` as the source of the data. This instructs Trident
to create a PVC from the snapshot. Once the PVC is created, it can be attached
to a pod and used just like any other PVC.

.. note::
      When deleting a Persistent Volume with associated snapshots, the
      corresponding Trident volume is updated to a "Deleting state". For the
      Trident volume to be deleted, the snapshots of the volume must be removed.

.. _Volume Snapshot feature: https://kubernetes.io/docs/concepts/storage/volume-snapshots/
