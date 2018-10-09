################
Managing volumes
################

Resizing an NFS volume
----------------------

Starting with v18.10, Trident supports volume resize for NFS PVs. More 
specifically, PVs provisioned on ``ontap-nas``, ``ontap-nas-economy``,
and ``ontap-nas-flexgroup`` backends can be expanded.

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

.. _Feature Gate : https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/

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

