################
Managing volumes
################

On-Demand Volume Snapshots
==========================

Creating Kubernetes Volume Snapshots (using the beta specification)
is available beginning with Trident ``20.01``. You can take a look at the
documentation `here <https://netapp-trident.readthedocs.io/en/stable-v20.01/kubernetes/operations/tasks/volumes.html#on-demand-volume-snapshots>`_.
You will need to make sure your Kubernetes release is ``1.17`` or above and
this feature requires Trident ``20.01`` or later. 

Expanding an iSCSI volume
=========================

Trident ``19.10`` introduces support for resizing an iSCSI PV using the
CSI provisioner. Provided Trident is configured to function as a CSI
provisioner, you can expand iSCSI PVs that have been created by Trident.
This feature is supported with Kubernetes versions ``1.16`` and above.

.. note::

   iSCSI volume expansion is supported by the ``ontap-san``,
   ``ontap-san-economy``, ``solidfire-san`` and ``eseries-iscsi`` drivers and
   requires Kubernetes ``1.16`` and above.

For resizing an iSCSI PV, you must ensure the following items are taken care of:

* The StorageClass must support volume expansion. This can be done by editing
  the StorageClass definition to set the ``allowVolumeExpansion`` field to
  ``true``.
* To resize a PV, edit the PVC definition and update the ``spec.resources.requests.storage``
  to reflect the newly desired size, which must be greater than the original size.
* The PV **must be attached to a pod** for it to be resized. There are two
  scenarios when resizing an iSCSI PV:

  * If the PV is **attached to a pod**, Trident expands the volume on the storage
    backend, rescans the device and resizes the filesystem.
  * When attempting to **resize an unattached PV**, Trident expands the volume
    on the storage backend. Once the PVC is bound to a pod, Trident rescans the
    device and resizes the filesystem. Kubernetes then updates the PVC size
    after the expand operation has successfully completed.

The example below shows how expanding iSCSI PVs work.

The first step is to create a StorageClass that supports volume expansion.

.. code-block:: bash

  $ cat storageclass-ontapsan.yaml
  ---
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ontap-san
  provisioner: csi.trident.netapp.io
  parameters:
    backendType: "ontap-san"
  allowVolumeExpansion: True

For an already existing StorageClass, you can edit the SC to include the
``allowVolumeExpansion`` parameter.

A PVC can be created with this SC.

.. code-block:: bash

   $ cat pvc-ontapsan.yaml
   kind: PersistentVolumeClaim
   apiVersion: v1
   metadata:
     name: san-pvc
   spec:
     accessModes:
     - ReadWriteOnce
     resources:
       requests:
         storage: 1Gi
     storageClassName: ontap-san


Trident creates a PV and associates with this PVC.

.. code-block:: bash

   $ kubectl get pvc
   NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
   san-pvc   Bound    pvc-8a814d62-bd58-4253-b0d1-82f2885db671   1Gi        RWO            ontap-san      8s

   $ kubectl get pv
   NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM             STORAGECLASS   REASON   AGE
   pvc-8a814d62-bd58-4253-b0d1-82f2885db671   1Gi        RWO            Delete           Bound    default/san-pvc   ontap-san               10s

Define a pod that attaches this PVC. In this example, a pod is created
that uses the ``san-pvc``.

.. code-block:: bash

   $  kubectl get pod
   NAME         READY   STATUS    RESTARTS   AGE
   centos-pod   1/1     Running   0          65s

   $  kubectl describe pvc san-pvc
   Name:          san-pvc
   Namespace:     default
   StorageClass:  ontap-san
   Status:        Bound
   Volume:        pvc-8a814d62-bd58-4253-b0d1-82f2885db671
   Labels:        <none>
   Annotations:   pv.kubernetes.io/bind-completed: yes
                  pv.kubernetes.io/bound-by-controller: yes
                  volume.beta.kubernetes.io/storage-provisioner: csi.trident.netapp.io
   Finalizers:    [kubernetes.io/pvc-protection]
   Capacity:      1Gi
   Access Modes:  RWO
   VolumeMode:    Filesystem
   Mounted By:    centos-pod

To resize the PV that has been created from 1Gi to 2Gi, edit the PVC definition and
update the ``spec.resources.requests.storage`` to 2Gi.

.. code-block:: bash

   $ kubectl edit pvc san-pvc
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
     creationTimestamp: "2019-10-10T17:32:29Z"
     finalizers:
     - kubernetes.io/pvc-protection
     name: san-pvc
     namespace: default
     resourceVersion: "16609"
     selfLink: /api/v1/namespaces/default/persistentvolumeclaims/san-pvc
     uid: 8a814d62-bd58-4253-b0d1-82f2885db671
   spec:
     accessModes:
     - ReadWriteOnce
     resources:
       requests:
         storage: 2Gi
    ...

We can validate the resize has worked correctly by checking the size of the
PVC, PV, and the Trident volume:

.. code-block:: bash

   $ kubectl get pvc san-pvc
   NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
   san-pvc   Bound    pvc-8a814d62-bd58-4253-b0d1-82f2885db671   2Gi        RWO            ontap-san      11m
   $ kubectl get pv
   NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM             STORAGECLASS   REASON   AGE
   pvc-8a814d62-bd58-4253-b0d1-82f2885db671   2Gi        RWO            Delete           Bound    default/san-pvc   ontap-san               12m
   $ tridentctl get volumes -n trident
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-8a814d62-bd58-4253-b0d1-82f2885db671 | 2.0 GiB | ontap-san     | block    | a9b7bfff-0505-4e31-b6c5-59f492e02d33 | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

Expanding an NFS volume
=======================

Starting with ``v18.10``, Trident supports volume resize for NFS PVs. More
specifically, PVs provisioned on ``ontap-nas``, ``ontap-nas-economy``,
``ontap-nas-flexgroup``, ``aws-cvs``, ``gcp-cvs``, and ``azure-netapp-files``
backends can be expanded.

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

Trident version 19.04 and above allows importing an existing storage volume
into Kubernetes with the ``ontap-nas``, ``ontap-nas-flexgroup``, ``solidfire-san``,
``azure-netapp-files``, ``aws-cvs``, and ``gcp-cvs`` drivers.

There are several use cases for importing a volume into Trident:

         * Containerizing an application and reusing its existing data set
         * Using a clone of a data set for an ephemeral application
         * Rebuilding a failed Kubernetes cluster
         * Migrating application data during disaster recovery

The ``tridentctl`` client is used to import an existing storage volume. Trident
imports the volume by persisting volume metadata and creating the PVC and PV.

.. code-block:: bash

  $ tridentctl import volume <backendName> <volumeName> -f <path-to-pvc-file>

To import an existing storage volume, specify the name of the Trident backend
containing the volume, as well as the name that uniquely identifies the volume
on the storage (i.e. ONTAP FlexVol, Element Volume, CVS Volume path'). The storage
volume must allow read/write access and be accessible by the specified Trident backend.

The ``-f string`` argument is required and specifies the path to the YAML or JSON PVC
file. The PVC file is used by the volume import process to create the PVC. At a
minimum, the PVC file must include the name, namespace, accessModes, and
storageClassName fields as shown in the following example.

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

When Trident receives the import volume request the existing volume size is
determined and set in the PVC. Once the volume is imported by the storage
driver the PV is created with a ClaimRef to the PVC. The reclaim policy is initially
set to ``retain`` in the PV. Once Kubernetes successfully binds the PVC and PV the
reclaim policy is updated to match the reclaim policy of the Storage Class. If the
reclaim policy of the Storage Class is ``delete`` then the storage volume will be
deleted when the PV is deleted.

When a volume is imported with the ``--no-manage`` argument, Trident will not
perform any additional operations on the PVC or PV for the lifecycle of the
objects. Since Trident ignores PV and PVC events for ``--no-manage`` objects
the storage volume is not deleted when the PV is deleted. Other operations such as
volume clone and volume resize are also ignored. This option is provided for
those that want to use Kubernetes for containerized workloads but otherwise
want to manage the lifecycle of the storage volume outside of Kubernetes.

An annotation is added to the PVC and PV that serves a dual purpose of
indicating that the volume was imported and if the PVC and PV are managed.
This annotation should not be modified or removed.

Trident ``19.07`` handles the attachment of PVs and mounts the volume as
part of importing it. For imports using earlier versions of Trident,
there will not be any operations in the data path and the volume import will
not verify if the volume can be mounted. If a mistake is made with volume
import (e.g. the StorageClass is incorrect), you can recover by changing the
reclaim policy on the PV to "Retain", deleting the PVC and PV, and retrying
the volume import command.

.. note::
    The Element driver supports duplicate volume names. If there are duplicate
    volume names Trident's volume import process
    will return an error. As a workaround, clone the volume and provide a
    unique volume name. Then import the cloned volume.

For example, to import a volume named ``managed_volume`` on a backend named
``ontap_nas`` use the following command:

.. code-block:: bash

   $ tridentctl import volume ontap_nas managed_volume -f <path-to-pvc-file>

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-bf5ad463-afbb-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

To import a volume named ``unmanaged_volume`` (on the ``ontap_nas`` backend)
which Trident will not manage, use the following command:

.. code-block:: bash

   $ tridentctl import volume nas_blog unmanaged_volume -f <path-to-pvc-file> --no-manage

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-df07d542-afbc-11e9-8d9f-5254004dfdb7 | 1.0 GiB | standard      | file     | c5a6f6a4-b052-423b-80d4-8fb491a14a22 | online | false   |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

w
When using the ``--no-manage`` flag, Trident renames the volume, but it
does not validate if the volume was mounted. The import operation will fail
if the volume was not mounted manually.

To import an ``aws-cvs`` volume on the backend called ``awscvs_YEppr`` with
the volume path of ``adroit-jolly-swift`` use the following command:

.. code-block:: bash

    $ tridentctl import volume awscvs_YEppr adroit-jolly-swift -f <path-to-pvc-file> -n trident

    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
    |                   NAME                   |  SIZE  | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+
    | pvc-a46ccab7-44aa-4433-94b1-e47fc8c0fa55 | 93 GiB | aws-storage   | file     | e1a6e65b-299e-4568-ad05-4f0a105c888f | online | true    |
    +------------------------------------------+--------+---------------+----------+--------------------------------------+--------+---------+

.. note::
  The volume path is the portion of the volume's export path after the `:/`. For example, if the export path is
  ``10.0.0.1:/adroit-jolly-swift`` then the volume path is ``adroit-jolly-swift``.

Importing a ``gcp-cvs`` volume works the same as importing an ``aws-cvs`` volume.

To import an ``azure-netapp-files`` volume on the backend called
``azurenetappfiles_40517`` with the volume path ``importvol1``, you will use
the following command:

.. code-block:: bash

   $ tridentctl import volume azurenetappfiles_40517 importvol1 -f <path-to-pvc-file> -n trident

   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   |                   NAME                   |  SIZE   | STORAGE CLASS | PROTOCOL |             BACKEND UUID             | STATE  | MANAGED |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+
   | pvc-0ee95d60-fd5c-448d-b505-b72901b3a4ab | 100 GiB | anf-storage   | file     | 1c01274f-d94b-44a3-98a3-04c953c9a51e | online | true    |
   +------------------------------------------+---------+---------------+----------+--------------------------------------+--------+---------+

.. note::
   The volume path for the ANF volume is present in the mount path after the `:/`. For example, if the mount path is
   ``10.0.0.2:/importvol1``, the volume path is ``importvol1``.

Behavior of Drivers for Volume Import
-------------------------------------

  * The ``ontap-nas`` and ``ontap-nas-flexgroup`` drivers do not allow
    duplicate volume names.
  * To import a volume backed by the NetApp Cloud Volumes Service in AWS,
    identify the volume by its volume path instead of its name. An example
    is provided in the previous section.
  * An ONTAP volume must be of type `rw` to be imported by Trident. If a
    volume is of type `dp` it is a SnapMirror destination volume; you must
    break the mirror relationship before importing the volume into Trident.
