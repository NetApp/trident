################
Managing volumes
################

Resizing an NFS volume
----------------------

Starting with v18.10, Trident supports volume resize for NFS PVs. More 
specifically, PVs provisioned on ``ontap-nas``, ``ontap-nas-economy``,
``ontap-nas-flexgroup``, and ``aws-cvs`` backends can be expanded.

`Resizing Persistent Volumes using Kubernetes`_ blog post describes the
workflows involved in resizing a PV. Volume resize was introduced in
Kubernetes v1.8 as an alpha feature and was promoted to beta in v1.11,
which means this feature is enabled by default starting with Kubernetes
v1.11.

.. _Resizing Persistent Volumes using Kubernetes: https://kubernetes.io/blog/2018/07/12/resizing-persistent-volumes-using-kubernetes/

Because NFS PV resize is not supported by Kubernetes, and is implemented by the
Trident orchestrator externally, Kubernetes admission controller may reject PVC
size updates for in-tree volume plugins that don't support resize (e.g., NFS).
The Trident team has changed Kubernetes to allow such changes starting
with Kubernetes 1.12. Therefore, we recommend using this feature with Kubernetes
1.12 or later as it would just work.

While we recommend using Kubernetes 1.12 or later, it is still possible to
resize NFS PVs with earlier versions of Kubernetes that support resize.
This is done by disabling the ``PersistentVolumeClaimResize`` admission plugin
when the Kubernetes API server is started:

.. code-block:: bash
  
  kube-apiserver --disable-admission-plugins=PersistentVolumeClaimResize


With Kubernetes 1.8-1.10 that offer this feature as alpha, the
``ExpandPersistentVolumes`` `Feature Gate`_ should also be turned on:

.. _Feature Gate: https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/

.. code-block:: bash
  
  kube-apiserver --feature-gates=ExpandPersistentVolumes=true --disable-admission-plugins=PersistentVolumeClaimResize


To resize an NFS PV, the admin first needs to configure the storage class to
allow volume expansion by setting the ``allowVolumeExpansion`` field to ``true``:

.. code-block:: bash
  
  $ cat storageclass-ontapnas.yaml 
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: ontapnas
  provisioner: netapp.io/trident
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
    NAME                    STATUS    VOLUME                                CAPACITY   ACCESS MODES   STORAGECLASS        AGE
    ontapnas20mb            Bound     default-ontapnas20mb-c1bd7            20Mi       RWO            ontapnas            14s
    
    $ kubectl get pv default-ontapnas20mb-c1bd7
    NAME                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM                  STORAGECLASS       REASON    AGE
    default-ontapnas20mb-c1bd7   20Mi       RWO            Delete           Bound     default/ontapnas20mb   ontapnas                     1m

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
        volume.beta.kubernetes.io/storage-provisioner: netapp.io/trident
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
    NAME           STATUS    VOLUME                       CAPACITY   ACCESS MODES   STORAGECLASS       AGE
    ontapnas20mb   Bound     default-ontapnas20mb-c1bd7   1Gi        RWO            ontapnas           6m
    
    $ kubectl get pv default-ontapnas20mb-c1bd7
    NAME                         CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM                  STORAGECLASS       REASON    AGE
    default-ontapnas20mb-c1bd7   1Gi        RWO            Delete           Bound     default/ontapnas20mb   ontapnas             6m
    
    $ tridentctl get volume default-ontapnas20mb-c1bd7 -n trident
    +----------------------------+---------+------------------+----------+------------------------+--------------+
    |            NAME            |  SIZE   |  STORAGE CLASS   | PROTOCOL |        BACKEND         |     POOL     |
    +----------------------------+---------+------------------+----------+------------------------+--------------+
    | default-ontapnas20mb-c1bd7 | 1.0 GiB | ontapnas         | file     | ontapnas_10.63.171.111 | VICE08_aggr1 |
    +----------------------------+---------+------------------+----------+------------------------+--------------+


Importing a volume
------------------------

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

As Trident doesn't currently perform operations in the data path, the volume import process does not verify if the
volume can be mounted. If a mistake is made with volume import (e.g. the StorageClass is incorrect), you can recover by
changing the reclaim policy on the PV to "Retain", deleting the PVC and PV, and retrying the volume import command.

You can use the ``--no-manage`` argument to verify that the volume import process will work as expected. Once you verify
the volume can be mounted by Kubernetes, you can safely delete the PVC & PV and then repeat the volume import without
the ``--no-manage`` argument.

.. note::
    SolidFire supports duplicate volume names. If there are duplicate volume names Trident's volume import process
    will return an error. As a workaround, clone the SolidFire volume and provide a unique volume name. Then import
    the cloned volume.

For example, to import a volume named ``test_volume`` on a backend named ``nas_blog`` use the following command:

.. code-block:: bash

   $ tridentctl import volume nas_blog test_volume -f <path-to-pvc-file> -n blog

+------------------------------------+---------+------------------------+----------+----------+------+
|                NAME                |  SIZE   |     STORAGE CLASS      | PROTOCOL | BACKEND  | POOL |
+------------------------------------+---------+------------------------+----------+----------+------+
| blog-blog-content-deployment-5deb1 | 1.0 GiB | storage-class-nas-blog | file     | nas_blog |      |
+------------------------------------+---------+------------------------+----------+----------+------+

To import a volume named "test_volume2" on the backend called `nas_blog`, which Trident will not manage, use the
following command:

.. code-block:: bash

   $ tridentctl import volume nas_blog test-volume2 -f <path-to-pvc-file> --no-manage -n blog

+------------------------------------+---------+------------------------+----------+----------+------+
|                NAME                |  SIZE   |     STORAGE CLASS      | PROTOCOL | BACKEND  | POOL |
+------------------------------------+---------+------------------------+----------+----------+------+
| test-volume2                       | 1.0 GiB | storage-class-nas-blog | file     | nas_blog |      |
+------------------------------------+---------+------------------------+----------+----------+------+

.. note::
  The name of the volume does not change since no-manage is specified.

To import an ``aws-cvs`` volume on the backend called `awscvs_YEppr` with the volume path of `adroit-jolly-swift`
use the following command:

.. code-block:: bash
    $ tridentctl import volume awscvs_YEppr adroit-jolly-swift -f <path-to-pvc-file> -n trident

+---------------------------+---------+-------------------+----------+--------------+------+
|           NAME            |  SIZE   | STORAGE CLASS     | PROTOCOL |   BACKEND    | POOL |
+---------------------------+---------+-------------------+----------+--------------+------+
| trident-aws-claim01-41970 | 1.0 GiB | storage-class-aws | file     | awscvs_YEppr |      |
+---------------------------+---------+-------------------+----------+--------------+------+

.. note::
  The AWS volume path is the portion of the volume's export path after the `:/`. For example, if the export path is
  ``10.0.0.1:/adroit-jolly-swift`` then the volume path is ``adroit-jolly-swift``.

Behavior of Drivers for Volume Import
--------------------------------------

  * The ``ontap-nas`` and ``ontap-nas-flexgroup`` drivers do not allow duplicate volume names.
  * The ``ontap-nas`` driver renames the storage volume unless the ``--no-manage`` argument is used.
  * To import a volume backed by the NetApp Cloud Volumes Service in AWS, identify the volume by its volume path instead
    of its name. An example is provided in the previous section.
  * An ONTAP volume must be of type `rw` to be imported by Trident. If a volume is of type `dp` it is a SnapMirror
    destination volume; you must break the mirror relationship before importing the volume into Trident.
