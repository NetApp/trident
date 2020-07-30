################
Volume Expansion
################

Trident provides Kubernetes users the ability to expand their volumes
after they are created. This section of the documentation explains how
iSCSI and NFS volumes can be expanded and examines the configuration
required to do so.

Expanding an iSCSI volume
-------------------------

Trident ``19.10`` introduces support for expanding an iSCSI PV using the
CSI provisioner. Provided Trident is configured to function as a CSI
provisioner, you can expand iSCSI PVs that have been created by Trident.
This feature is supported with Kubernetes versions ``1.16`` and above.

.. note::

   iSCSI volume expansion is supported by the ``ontap-san``,
   ``ontap-san-economy``, ``solidfire-san`` and ``eseries-iscsi`` drivers and
   requires Kubernetes ``1.16`` and above.

For growing an iSCSI PV, you must ensure the following items are taken care of:

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
-----------------------

Starting with ``v18.10``, Trident supports volume expansion for NFS PVs. More
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
