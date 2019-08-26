################
Managing volumes
################

On-Demand Volume Snapshots
==========================

The 19.07 release of Trident introduces support for the creation of snapshots
of PVs. Snapshots provide an easy method to maintain a copy of the
volume and use them for creating additional volumes (clones). 

.. note::

   Volume snapshot is supported by the ``ontap-nas``,
   ``ontap-san``, ``solidfire-san``, ``aws-cvs`` and ``azure-netapp-files`` drivers.
   This feature requires the CSI Provisioner and :ref:`feature gates <Feature Gates>`
   enabled for it to work.

The example detailed
below explains the constructs required for working with snapshots and
shows how snapshots can be created and used.

Before creating a Volume Snapshot, a :ref:`VolumeSnapshotClass <Kubernetes VolumeSnapshotClass Objects>`
must be set up. 

.. code-block:: bash
     
   $ cat snap-sc.yaml
   apiVersion: snapshot.storage.k8s.io/v1alpha1
   kind: VolumeSnapshotClass
   metadata:
     name: csi-snapclass
   snapshotter: csi.trident.netapp.io

Create a VolumeSnapshot
-----------------------

We can now create a snapshot of an existing PVC.

.. code-block:: bash
   
   $ cat snap.yaml  
   apiVersion: snapshot.storage.k8s.io/v1alpha1 
   kind: VolumeSnapshot
   metadata:
     name: pvc1-snap
   spec:
     snapshotClassName: csi-snapclass
     source:
       name: pvc1
       kind: PersistentVolumeClaim

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
object which serves this snapshot. The ``Ready To Use`` parameter indicates that the
Snapshot can be used to create a new PVC.

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
to create a PVC from the snapshot. Once the PVC is created, it can be attached to
a pod and used just like any other PVC.

.. note::
      When deleting a Persistent Volume with associated snapshots, the corresponding
      Trident volume is updated to a "Deleting state". For the Trident volume to be
      deleted, the snapshots of the volume must be removed.

Resizing an NFS volume
======================

Starting with ``v18.10``, Trident supports volume resize for NFS PVs. More 
specifically, PVs provisioned on ``ontap-nas``, ``ontap-nas-economy``,
``ontap-nas-flexgroup``, ``aws-cvs`` and ``azure-netapp-files`` backends can be expanded.
Volume resize was introduced in
Kubernetes ``v1.8`` as an alpha feature and was promoted to beta in ``v1.11``,
which means this feature is enabled by default starting with Kubernetes
``v1.11``.

To resize an NFS PV, the admin first needs to configure the storage class to
allow volume expansion by setting the ``allowVolumeExpansion`` field to ``true``:

.. code-block:: bash
  
  $ cat storageclass-ontapnas.yaml 
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ontapnas
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: ontap-nas
  allowVolumeExpansion: true
  
If you have already created a storage class without this option, you can simply
edit the existing storage class via ``kubectl edit storageclass`` to allow
volume expansion.

Next, we create a PVC using this storage class:

.. code-block:: bash
  
  $ cat pvc-ontapnas.yaml 
  kind: PersistentVolumeClaim
  apiVersion: v1
  metadata:
    name: ontapnas20mb
  spec:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: 20Mi
    storageClassName: ontapnas

Trident should create a 20MiB NFS PV for this PVC:

.. code-block:: bash
  
    $ kubectl get pvc
    NAME           STATUS   VOLUME                                     CAPACITY     ACCESS MODES   STORAGECLASS    AGE
    ontapnas20mb   Bound    pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7   20Mi         RWO            ontapnas        9s

    $ kubectl get pv pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7
    NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                  STORAGECLASS    REASON   AGE
    pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7   20Mi       RWO            Delete           Bound    default/ontapnas20mb   ontapnas                 2m42s

To resize the newly created 20MiB PV to 1GiB, we edit the PVC and set
``spec.resources.requests.storage`` to 1GB:

.. code-block:: bash

    $ kubectl edit pvc ontapnas20mb
    # Please edit the object below. Lines beginning with a '#' will be ignored,
    # and an empty file will abort the edit. If an error occurs while saving this file will be
    # reopened with the relevant failures.
    #
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      annotations:
        pv.kubernetes.io/bind-completed: "yes"
        pv.kubernetes.io/bound-by-controller: "yes"
        volume.beta.kubernetes.io/storage-provisioner: csi.trident.netapp.io
      creationTimestamp: 2018-08-21T18:26:44Z
      finalizers:
      - kubernetes.io/pvc-protection
      name: ontapnas20mb
      namespace: default
      resourceVersion: "1958015"
      selfLink: /api/v1/namespaces/default/persistentvolumeclaims/ontapnas20mb
      uid: c1bd7fa5-a56f-11e8-b8d7-fa163e59eaab
    spec:
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 1Gi
    ...

We can validate the resize has worked correctly by checking the size of the PVC,
PV, and the Trident volume:

.. code-block:: bash

    $ kubectl get pvc ontapnas20mb
    NAME           STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS    AGE
    ontapnas20mb   Bound    pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7   1Gi        RWO            ontapnas        4m44s
 
    $ kubectl get pv pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7
    NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                  STORAGECLASS    REASON   AGE
    pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7   1Gi        RWO            Delete           Bound    default/ontapnas20mb   ontapnas                 5m35s 

    $ tridentctl get volume pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7 -n trident
    +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
    |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
    +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
    | pvc-08f3d561-b199-11e9-8d9f-5254004dfdb7 | 1.0 GiB | ontapnas      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
    +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

Importing a volume
==================

Trident version 19.04 and above allows importing an existing storage volume into Kubernetes with the ``ontap-nas``,
``ontap-nas-flexgroup``, ``solidfire-san``, and ``aws-cvs`` drivers.

There are several use cases for importing a volume into Trident:

         * Containerizing an application and reusing its existing data set
         * Using a clone of a data set for an ephemeral application
         * Rebuilding a failed Kubernetes cluster
         * Migrating application data during disaster recovery

The ``tridentctl`` client is used to import an existing storage volume. Trident imports the volume by persisting volume
metadata and creating the PVC and PV.

.. code-block:: bash

  $ tridentctl import volume <backendName> <volumeName> -f <path-to-pvc-file>

To import an existing storage volume, specify the name of the Trident backend containing the volume, as well as the name
that uniquely identifies the volume on the storage (i.e. ONTAP FlexVol, Element Volume, CVS Volume path'). The storage
volume must allow read/write access and be accessible by the specified Trident backend.

The ``-f string`` argument is required and specifies the path to the YAML or JSON PVC file. The PVC file is
used by the volume import process to create the PVC. At a minimum, the PVC file must include the name, namespace,
accessModes, and storageClassName fields as shown in the following example.

.. code-block:: yaml

  kind: PersistentVolumeClaim
  apiVersion: v1
  metadata:
    name: my_claim
    namespace: my_namespace
  spec:
    accessModes:
      - ReadWriteOnce
    storageClassName: my_storage_class

When Trident receives the import volume request the existing volume size is determined and set in the PVC. Once the
volume is imported by the storage driver the PV is created with a ClaimRef to the PVC. The reclaim policy is initially
set to ``retain`` in the PV. Once Kubernetes successfully binds the PVC and PV the reclaim policy is updated to match
the reclaim policy of the Storage Class. If the reclaim policy of the Storage Class is ``delete`` then the storage
volume will be deleted when the PV is deleted.

When a volume is imported with the ``--no-manage`` argument, Trident will not perform any additional operations
on the PVC or PV for the lifecycle of the objects. Since Trident ignores PV and PVC events for ``--no-manage`` objects
the storage volume is not deleted when the PV is deleted. Other operations such as volume clone and volume resize are
also ignored. This option is provided for those that want to use Kubernetes for containerized workloads but otherwise
want to manage the lifecycle of the storage volume outside of Kubernetes.

An annotation is added to the PVC and PV that serves a dual purpose of indicating that the volume was imported and
if the PVC and PV are managed. This annotation should not be modified or removed.

Trident ``19.07`` handles the attachment of PVs and mounts the volume as part of importing it. For imports using earlier versions
of Trident,
there will not be any operations in the data path and the volume import will not verify if the
volume can be mounted. If a mistake is made with volume import (e.g. the StorageClass is incorrect), you can recover by
changing the reclaim policy on the PV to "Retain", deleting the PVC and PV, and retrying the volume import command.

.. note::
    The Element driver supports duplicate volume names. If there are duplicate volume names Trident's volume import process
    will return an error. As a workaround, clone the volume and provide a unique volume name. Then import
    the cloned volume.

For example, to import a volume named ``managed_volume`` on a backend named ``ontap_nas`` use the following command:

.. code-block:: bash

   $ tridentctl import volume ontap_nas managed_volume -f <path-to-pvc-file>
 
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-bf5ad463-afbb-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

To import a volume named ``unmanaged_volume`` (on the ``ontap_nas`` backend) which Trident will not manage, use the
following command:

.. code-block:: bash

   $ tridentctl import volume nas_blog unmanaged_volume -f <path-to-pvc-file> --no-manage
 
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-df07d542-afbc-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | false   |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

When using the ``--no-manage`` flag, Trident renames the volume, but it does not validate if the volume was mounted.
The import operation will fail if the volume was not mounted manually.

To import an ``aws-cvs`` volume on the backend called `awscvs_YEppr` with the volume path of `adroit-jolly-swift`
use the following command:

.. code-block:: bash

    $ tridentctl import volume awscvs_YEppr adroit-jolly-swift -f <path-to-pvc-file> -n trident

    +----------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
    |            NAME            |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
    +----------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
    | trident-aws-claim01-41970  | 1.0 GiB | aws-sc        | file     | b570e4af-f38c-4504-9d05-02dcc14bb95d | online | false   |
    +----------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

.. note::
  The AWS volume path is the portion of the volume's export path after the `:/`. For example, if the export path is
  ``10.0.0.1:/adroit-jolly-swift`` then the volume path is ``adroit-jolly-swift``.

Behavior of Drivers for Volume Import
-------------------------------------

  * The ``ontap-nas`` and ``ontap-nas-flexgroup`` drivers do not allow duplicate volume names.
  * To import a volume backed by the NetApp Cloud Volumes Service in AWS, identify the volume by its volume path instead
    of its name. An example is provided in the previous section.
  * An ONTAP volume must be of type `rw` to be imported by Trident. If a volume is of type `dp` it is a SnapMirror
    destination volume; you must break the mirror relationship before importing the volume into Trident.
