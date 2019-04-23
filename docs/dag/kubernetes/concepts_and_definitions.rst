.. _concepts_and_definitions:

************************
Concepts and Definitions
************************

Kubernetes introduces several new concepts that storage, application and platform administrators should take into consideration. It is essential to understand the capability of each within the context of their use case.

Kubernetes storage concepts
===========================

The Kubernetes storage paradigm includes several entities which are important to each stage of requesting, consuming, and managing storage for containerized applications. At a high level, Kubernetes uses three types of objects to describe storage, as described below:

Persistent Volume claim
-----------------------

Persistent volume claims (PVCs) are used by applications to request access to storage resources. At a minimum, this includes two key characteristics:

* Size – The capacity desired by the application component
* Access mode – This describes the rules for accessing the storage volume. In particular, there are three access modes:

  * Read Write Once (RWO) – only one node is allowed to access the storage volume at a time for read and write access
  * Read Only Many (ROX) – many nodes may access the storage volume in read-only mode
  * Read Write Many (RWX) – many nodes may simultaneously read and write to the storage volume 

* Optional: Storage Class - Which storage class to request for this request. See below for storage class information.

More information about PVCs can be found in the `Kubernetes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ or `OpenShift <https://docs.openshift.com/container-platform/latest/architecture/additional_concepts/storage.html#persistent-volume-claims>`_ documentation.

Persistent Volume
-----------------

PVs are objects that describe, to Kubernetes, how to connect to a storage device. Kubernetes supports many different types of storage. However, this document covers only NFS and iSCSI devices since NetApp platforms and services support those protocols.
At a minimum, the PV must contain these parameters:

* The capacity represented by the object, e.g. "5Gi"
* The access mode--same as for PVCs--however, access modes can be dependent on protocol.

  * RWO is supported by all PVs
  * ROX is supported primarily by file and file-like protocols, e.g. NFS and CephFS. However, some block protocols are supported, such as iSCSI
  * RWX is supported by file and file-like protocols only, such as NFS

* The protocol, e.g. "iscsi" or "nfs", and additional information needed to access the storage. For example, an NFS PV will need the NFS server and mount path.
* A reclaim policy that describes the Kubernetes action when the PV is released. There are three options available:

  * Retain, which will mark the volume as waiting for administrator action. The volume cannot be reissued to another PVC.
  * Recycle, where, after being released, Kubernetes will connect the volume to a temporary pod and issue a ``rm -rf`` command to clear the data. For our interests, this is only supported by NFS volumes.
  * A policy of Delete will cause Kubernetes to delete the PV when it is released. Kubernetes does not, however, delete the storage which was referenced by the PV.

More information about PVs can be found in the `Kubernetes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistent-volumes>`_ or `OpenShift <https://docs.openshift.com/container-platform/latest/architecture/additional_concepts/storage.html#persistent-volumes>`_ documentation.

Storage Class
-------------

Kubernetes uses the storage class object to describe storage with specific characteristics. An administrator may define several storage classes that each define different storage properties. They are used by the :ref:`PVC <Persistent Volume Claim>` to provision storage. A storage class may have a provisioner associated with it that will trigger Kubernetes to wait for the volume to be provisioned by the specified provider. In the case of NetApp, the provisioner identifier used is ``netapp.io/trident``.

A storage class object in Kubernetes has only two required fields:

* A name
* The provisioner, a full list of provisioners can be found in `the documentation <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_

The provisioner used may require additional attributes, which will be specific to the provisioner used. Additionally, the storage class may have a reclaim policy and mount options specified which will be applied to all volumes created for that storage class.

More information about storage classes can be found in the `Kubernetes <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ or `OpenShift <https://docs.openshift.com/container-platform/latest/install_config/persistent_storage/dynamically_provisioning_pvs.html>`_ documentation. 

Kubernetes Compute Concepts
===========================

In addition to basic storage concepts, like how to request and consume storage as described above, it's important to understand the compute concepts involved in the consumption of storage resources. Kubernetes is a container orchestrator, which means that it will dynamically assign containerized workloads to cluster members according to the resource requirements they have expressed (or defaults, if no explicit request is made).

For more information about what containers are and why they are different, see the `Docker documentation <https://www.docker.com/what-container>`_.

Pods
----

`A pod <https://kubernetes.io/docs/concepts/workloads/pods/pod-overview/>`_ represents one or more containers which are related to each other. Containers which are members of the same pod are co-scheduled to the same node in the cluster. They typically share network and storage resources, though not every container in the pod may access the storage or be publicly accessible via the network.

The smallest granularity of management for Kubernetes compute resources is the pod. It is the atomic unit (smallest unit) of scale and is the consumer of other resources, such as storage.

Services
--------

A Kubernetes `service <https://kubernetes.io/docs/concepts/services-networking/service/>`_ acts as an internal load balancer for replicated pods. It enables the scaling of pods while maintaining a consistent service IP address. There are several types of services, which may be reachable only within the cluster with a ClusterIP, or may be exposed to the outside world with a NodePort, LoadBalancer, or ExternalName.  


Deployments
-----------

A `deployment <https://kubernetes.io/docs/concepts/workloads/controllers/deployment/>`_ is one or more pods which are related to each other and often represent a "service" to a larger application being deployed. The application administrator uses deployments to declare the state of their application component and request that Kubernetes ensure that the state is implemented at all times. This can include several options:

* Pods which should be deployed, including versions, storage, network, and other resource requests
* Number of replicas of each pod instance

The application administrator then uses the deployment as the interface for managing the application. For example, by increasing or decreasing the number of replicas desired the application can be horizontally scaled in or out. Updating the deployment with a new version of the application pod(s) will trigger Kubernetes to remove existing instances one at a time and redeploy using the new version. Conversely, rolling back to a previous version of the deployment will cause Kubernetes to revert the pods to the previously specified version and configuration.

StatefulSets
------------

Deployments specify how to scale application components, but it's limited to just the pods. When a webserver (which is managed as a Kubernetes deployment) is scaled up, Kubernetes will add more instances of that pod to reach the desired count. It is possible to add PVCs to deployments but then the PVC is shared by all pod replicas. What if each pod needs unique persistent storage?

`StatefulSets <https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/>`_ are a special type of deployment where persistent storage is requested along with each replica of the pod(s). The StatefulSet definition includes a template PVC, which is used to request additional storage resources as the application is scaled out. In this case, each replica receives its own volume of storage. This is generally used for stateful applications such as databases. 

In order to accomplish the above, StatefulSets provide unique pod names and network identifiers that are persistent across pod restarts. They also allow ordered operations, including startup, scale-up, upgrades, and deletion.  

As the number of pod replicas increase, the number of PVCs do also. However, scaling down the application will not result in the PVCs being destroyed, as Kubernetes relies on the application administrator to clean up the PVCs in order to prevent inadvertent data loss.

Connecting containers to storage
================================

When the application submits a PVC requesting storage, the Kubernetes engine will assign a PV which matches, or closely matches, the requirement. If no PV exists which can meet the request expressed in the PVC, then it will wait until a PV has been created which matches the request before making the assignment. If no storage class was assigned, then the Kubernetes administrator would be expected to request a storage resource and introduce a PV. However, the provisioner handles that process automatically when using storage classes.

.. _figDynamicStorageProvisioningProcess:

.. figure:: images/DynamicStorageProvisioningProcess.*

   Kubernetes dynamic storage provisioning process
 
The storage is not connected to a Kubernetes node within a cluster until the pod has been scheduled. At that time, ``kubelet``, the `agent <https://kubernetes.io/docs/concepts/overview/components/#node-components>`_  running on each node that is responsible for managing container instances, mounts the storage to the host according to the information in the PV.  When the container(s) in the pod are instantiated on the host, ``kubelet`` mounts the storage devices into the container.

Destroying and creating pods 
============================

It's important to understand that Kubernetes destroys and creates pods (workloads), it does not "move" them the same as live VM migration used by hypervisors. When Kubernetes scales down or needs to re-deploy a workload on a different host, the pod and the container(s) on the original host are stopped, destroyed, and the resources unmounted. The standard mount and instantiate process is then followed wherever in the cluster the same workload is re-deployed as a different pod with a different name, IP address, etc. 
When the application being deployed relies on persistent storage, that storage must be accessible from any Kubernetes node deploying the workload within the cluster. Without a shared storage system available for persistence, the data would be abandoned, and usually deleted, on the source system when the workload is re-deployed elsewhere in the cluster.

To maintain a persistent pod that will always be deployed on the same node with the same name and characteristics, a Stateful Set must be used as described above.

Container Storage Interface
===========================

The Cloud Native Computing Foundation (CNCF) is actively working on a standardized Container Storage Interface (CSI). NetApp is active in the CSI Special Interest Group (SIG). The CSI is meant to be a standard mechanism used by various container orchestrators to expose storage systems to containers. Trident v19.04 with CSI is currently in alpha stage and runs with Kubernetes version >= 1.13. However, today CSI does not provide the features NetApp's interface provides for Kubernetes. Therefore, NetApp recommends deploying Trident without CSI at this time and waiting until CSI is more mature.

