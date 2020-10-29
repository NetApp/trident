############
CSI Topology
############

Trident can selectively create and attach volumes to nodes present in a
Kubernetes cluster by making use of the `CSI Topology Feature <https://kubernetes-csi.github.io/docs/topology.html>`_.
Using the CSI Topology Feature, access to volumes can be limited to a subset of
nodes, based on regions and availability zones. Cloud providers today enable
Kubernetes administrators to spawn nodes that are zone-based. Nodes can be
located in different regions within an availability zone, or across various
availability zones. To facilitate the provisioning of volumes for workloads in
a multi-zone architecture, Trident's CSI Topology support is used.

In addition, volumes can also support the ``WaitForFirstConsumer`` binding mode.
The volume binding mode dictates when the volume should be created and bound to
the PVC request. Kubernetes provides two unique volume binding modes:

.. note::

  The ``WaitForFirstConsumer`` binding mode **does not require** topology labels.
  This can be used independent of the CSI Topology feature.

1. With **VolumeBindingMode** set to ``Immediate``, Trident creates the volume
   without any topology awareness. Volume binding and dynamic provisioning are
   handled when the PVC is created. This is the default VolumeBindingMode and is
   suited for clusters that do not enforce topology constraints. Persistent
   Volumes are created without having any dependency on the requesting pod's
   scheduling requirements.
2. Using the ``WaitForFirstConsumer`` **VolumeBindingMode**, the creation and
   binding of a Persistent Volume for a PVC is delayed until a pod that uses the
   PVC is scheduled and created. This way, volumes are created to meet the
   scheduling constraints that are enforced by topology requirements.
   `Resource requirements <https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/>`_,
   `node selectors <https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector>`_,
   `pod affinity and anti-affinity <https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity>`_,
   and `taints and tolerations <https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration>`_.

You can learn more about the CSI Topology Feature from this Kubernetes
`blog <https://kubernetes.io/blog/2018/10/11/topology-aware-volume-provisioning-in-kubernetes/>`_.

Prerequisites
-------------

To make use of CSI Topology, you will need:

1. A Kubernetes cluster running ``1.17`` or above.
2. Nodes in the cluster to have labels that introduce topology
   awareness [``topology.kubernetes.io/region`` and ``topology.kubernetes.io/zone``].
   These labels **must be present on nodes in the cluster** before Trident is
   installed for Trident to be topology aware.

.. code-block:: bash

    $ kubectl get nodes -o=jsonpath='{range .items[*]}[{.metadata.name}, {.metadata.labels}]{"\n"}{end}' | grep --color "topology.kubernetes.io"
    [node1, {"beta.kubernetes.io/arch":"amd64","beta.kubernetes.io/os":"linux","kubernetes.io/arch":"amd64","kubernetes.io/hostname":"node1","kubernetes.io/os":"linux","node-role.kubernetes.io/master":"","topology.kubernetes.io/region":"us-east1","topology.kubernetes.io/zone":"us-east1-a"}]
    [node2, {"beta.kubernetes.io/arch":"amd64","beta.kubernetes.io/os":"linux","kubernetes.io/arch":"amd64","kubernetes.io/hostname":"node2","kubernetes.io/os":"linux","node-role.kubernetes.io/worker":"","topology.kubernetes.io/region":"us-east1","topology.kubernetes.io/zone":"us-east1-b"}]
    [node3, {"beta.kubernetes.io/arch":"amd64","beta.kubernetes.io/os":"linux","kubernetes.io/arch":"amd64","kubernetes.io/hostname":"node3","kubernetes.io/os":"linux","node-role.kubernetes.io/worker":"","topology.kubernetes.io/region":"us-east1","topology.kubernetes.io/zone":"us-east1-c"}]

    $ kubectl version
    Client Version: version.Info{Major:"1", Minor:"19", GitVersion:"v1.19.3", GitCommit:"1e11e4a2108024935ecfcb2912226cedeafd99df", GitTreeState:"clean", BuildDate:"2020-10-14T12:50:19Z", GoVersion:"go1.15.2", Compiler:"gc", Platform:"linux/amd64"}
    Server Version: version.Info{Major:"1", Minor:"19", GitVersion:"v1.19.3", GitCommit:"1e11e4a2108024935ecfcb2912226cedeafd99df", GitTreeState:"clean", BuildDate:"2020-10-14T12:41:49Z", GoVersion:"go1.15.2", Compiler:"gc", Platform:"linux/amd64"}

Creating a Topology Aware Backend
---------------------------------

Trident storage backends can be designed to selectively provision volumes based
on availability zones. Each backend can carry an optional ``supportedTopologies``
block that represents a list of zones and regions that must be supported. For
StorageClasses that make use of such a backend, a volume would only be created
if requested by an application that is scheduled in a supported region/zone.

Here is what an example backend definition looks like:

.. _storage-pool-topology:

.. code-block:: json

    {
     "version": 1,
     "storageDriverName": "ontap-san",
     "backendName": "san-backend-us-east1",
     "managementLIF": "192.168.27.5",
     "svm": "iscsi_svm",
     "username": "admin",
     "password": "xxxxxxxxxxxx",
     "supportedTopologies": [
    {"topology.kubernetes.io/region": "us-east1", "topology.kubernetes.io/zone": "us-east1-a"},
    {"topology.kubernetes.io/region": "us-east1", "topology.kubernetes.io/zone": "us-east1-b"}
    ]
    }

.. note::

  ``supportedTopologies`` is used to provide a list of regions and zones per
  backend. These regions and zones represent the list of permissible values that
  can be provided in a StorageClass. For StorageClasses that contain a subset of
  the regions and zones provided in a backend, Trident will create a volume on
  the backend.

Users may ask: How do I use this along with
:ref:`Virtual Storage Pools <Virtual Storage Pools>`? You can define
``supportedTopologies`` per storage pool as well.

.. code-block:: json

    {"version": 1,
    "storageDriverName": "ontap-nas",
    "backendName": "nas-backend-us-central1",
    "managementLIF": "172.16.238.5",
    "svm": "nfs_svm",
    "username": "admin",
    "password": "Netapp123",
    "supportedTopologies": [
          {"topology.kubernetes.io/region": "us-central1", "topology.kubernetes.io/zone": "us-central1-a"},
          {"topology.kubernetes.io/region": "us-central1", "topology.kubernetes.io/zone": "us-central1-b"}
        ]
    "storage": [
       {
           "labels": {"workload":"production"},
            "region": "Iowa-DC",
            "zone": "Iowa-DC-A",
            "supportedTopologies": [
                {"topology.kubernetes.io/region": "us-central1", "topology.kubernetes.io/zone": "us-central1-a"}
            ]
        },
        {
            "labels": {"workload":"dev"},
             "region": "Iowa-DC",
             "zone": "Iowa-DC-B",
             "supportedTopologies": [
                 {"topology.kubernetes.io/region": "us-central1", "topology.kubernetes.io/zone": "us-central1-b"}
             ]
         }
    ]
    }

In this example, the ``region`` and ``zone`` labels stand for the location of
the storage pool. ``topology.kubernetes.io/region`` and ``topology.kubernetes.io/zone``
dictate where the storage pools can be consumed from.

Defining StorageClasses that are Topology Aware
-----------------------------------------------

Based on the topology labels that are provided to the nodes in the cluster,
StorageClasses can be defined to contain topology information. This will
determine the storage pools that serve as candidates for PVC requests made, and
the subset of nodes that can make use of the volumes provisioned by Trident.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
    name: netapp-san-us-east1
    provisioner: csi.trident.netapp.io
    volumeBindingMode: WaitForFirstConsumer
    allowedTopologies:
    - matchLabelExpressions:
    - key: topology.kubernetes.io/zone
      values:
      - us-east1-a
      - us-east1-b
    - key: topology.kubernetes.io/region
      values:
      - us-east1
    parameters:
      fsType: "ext4"

In the StorageClass definition provided above:

1. ``volumeBindingMode`` is set to ``WaitForFirstConsumer``. PVCs that are
   requested with this StorageClass will not be acted upon until they are
   referenced in a pod.
2. ``allowedTopologies`` provides the zones and region to be used. The
   ``netapp-san-us-east1`` StorageClass will create PVCs on the ``san-backend-us-east1``
   backend defined :ref:`above <storage-pool-topology>`.

Creating and using a PVC
------------------------

With the StorageClass created and mapped to a backend, PVCs can now be created.

.. code-block:: yaml

    ---
    kind: PersistentVolumeClaim
    apiVersion: v1
    metadata:
    name: pvc-san
    spec:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 300Mi
    storageClassName: netapp-san-us-east1

Creating a PVC using this manifest would result in this:

.. code-block:: bash

    $ kubectl create -f pvc.yaml
    persistentvolumeclaim/pvc-san created
    $ kubectl get pvc
    NAME      STATUS    VOLUME   CAPACITY   ACCESS MODES   STORAGECLASS          AGE
    pvc-san   Pending                                      netapp-san-us-east1   2s
    $ kubectl describe pvc
    Name:          pvc-san
    Namespace:     default
    StorageClass:  netapp-san-us-east1
    Status:        Pending
    Volume:
    Labels:        <none>
    Annotations:   <none>
    Finalizers:    [kubernetes.io/pvc-protection]
    Capacity:
    Access Modes:
    VolumeMode:    Filesystem
    Mounted By:    <none>
    Events:
      Type    Reason                Age   From                         Message
      ----    ------                ----  ----                         -------
      Normal  WaitForFirstConsumer  6s    persistentvolume-controller  waiting for first consumer to be created before binding

For Trident to create a volume and bind it to the PVC, you will need to use the
PVC in a pod.

.. code-block:: yaml

    apiVersion: v1
    kind: Pod
    metadata:
      name: app-pod-1
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: topology.kubernetes.io/region
                operator: In
                values:
                - us-east1
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 1
            preference:
              matchExpressions:
              - key: topology.kubernetes.io/zone
                operator: In
                values:
                - us-east1-a
                - us-east1-b
      securityContext:
        runAsUser: 1000
        runAsGroup: 3000
        fsGroup: 2000
      volumes:
      - name: vol1
        persistentVolumeClaim:
          claimName: pvc-san
      containers:
      - name: sec-ctx-demo
        image: busybox
        command: [ "sh", "-c", "sleep 1h" ]
        volumeMounts:
        - name: vol1
          mountPath: /data/demo
        securityContext:
          allowPrivilegeEscalation: false

This podSpec instructs Kubernetes to schedule the pod on nodes that are present
in the ``us-east1`` region, and choose from any node that is present in the
``us-east1-a`` or ``us-east1-b`` zones.

.. code-block:: bash

    $ kubectl get pods -o wide
    NAME        READY   STATUS    RESTARTS   AGE   IP               NODE              NOMINATED NODE   READINESS GATES
    app-pod-1   1/1     Running   0          19s   192.168.25.131   node2             <none>           <none>
    $ kubectl get pvc -o wide
    NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS          AGE   VOLUMEMODE
    pvc-san   Bound    pvc-ecb1e1a0-840c-463b-8b65-b3d033e2e62b   300Mi      RWO            netapp-san-us-east1   48s   Filesystem

Updating Backends to include supportedTopologies
------------------------------------------------

Pre-existing backends can be updated to include a list of ``supportedTopologies``
using ``tridentctl backend update``. This will not affect volumes that have
already been provisioned, and will only be used for subsequent PVCs.
