.. _storage_kubernetes_infrastructure_services:

**********************************************
Storage for Kubernetes Infrastructure Services
**********************************************

Trident focuses on providing persistence to Kubernetes applications, but before you can host those applications, you need to run a healthy, protected Kubernetes cluster. Those clusters are made up of a number of services with their own persistence requirements that need to be considered.

**Node-local container storage, a.k.a. graph driver storage**

One of the often overlooked components of a Kubernetes deployment is the storage which the container instances consume on the Kubernetes cluster nodes, usually referred to as `graph driver storage <https://success.docker.com/article/an-introduction-to-storage-solutions-for-docker-caas#graphdriverstorage>`_.  When a container is instantiated on a node it consumes capacity and IOPS to do many of it's operations as only data which is read from or written to a persistent volume is offloaded to the external storage.  If the Kubernetes nodes are expected to host dozens, hundreds, or more containers this may be a significant amount of temporary capacity and IOPS which are expected of the node-local storage.  

Even if you don't have a requirement to keep the data, the containers still need enough performance and capacity to execute their application. The Kubernetes administrator and storage administrator should work closely together to determine the requirements for graph storage and ensure adequate performance and capacity is available.

The typical method of augmenting the graph driver storage is to use a block device mounted at the location where container instance storage is located, e.g. ``/var/lib/docker``.  The host operating system being used to underpin the Kubernetes deployment will each have different methods for how to replace the graph storage with something more robust than a simple directory on the node.  Refer to the documentation from Red Hat, Ubuntu, SUSE, etc. for those instructions.

.. note::
   Block protocol is specifically recommended for graph storage due to the nature of how the graph drivers work. In particular, they create thin clones, using a variety of methods depending on the driver, of the container image. NFS does not support this functionality and results in a full copy of the container image file system for each instance, resulting in significant performance and capacity implications.

If the Kubernetes nodes are virtualized, this could also be addressed by ensuring that the datastore they reside on meets the performance and capacity needs, however the flexibility of having a separate device, even an additional virtual disk, should be carefully considered. Using a separate device gives the ability to independently control capacity, performance, and data protection to tailor the policies according to needs.  Often the capacity and performance needed for graph storage can fluctuate dramatically, however data protection is not necessary.

Persistent storage for cluster services
=======================================

There are several critical services hosted on the master servers and other cluster critical services which may be hosted on other node types.

Using capacity provisioned from enterprise storage adds several benefits for each service as delineated below:

* Performance – leveraging enterprise storage means being able to provide latency in-line with application needs and controlling performance using QoS policies.  This can be used to limit performance for `bullies <https://library.netapp.com/ecmdocs/ECMP1364220/html/GUID-71BD6939-9E02-451E-A222-9086B68B52A2.html>`_ or ensure performance for applications as needed.
* Resiliency – in the event of node failure, being able to quickly recover the data and present it to a replacement ensures that the application is minimally affected.
* Data protection – putting application data onto dedicated volumes allows the data to have its own snapshot, replication, and retention policies.
* Data security – ensuring that data is encrypted and protected all the way to the disk level is important for meeting many compliance requirements.
* Scale – using enterprise storage enables deploying fewer instances, with the instance count depending on compute resources, rather than having to increase the total count for resiliency and performance purposes.  Data which is protected automatically, and provides adequate performance, means that horizontal scale out doesn't need to compensate for limited performance.

There are some best practices which apply across all of the cluster services, or any application, which should be addressed as well.

* Determining the amount of acceptable data loss, i.e. the `Recovery Point Objective <https://en.wikipedia.org/wiki/Recovery_point_objective>`_ (RPO), is a critical part of deploying a production level system.  Having an understanding of this will greatly simplify the decisions around other items described in this section.
* Cluster service volumes should have a snapshot policy which enables the rapid recovery of data according to your requirements, as defined by the RPO.  This enables quick and easy restoration of a service to a point in time by reverting the volume or the ability to recover individual files if needed.
* Replication can provide both backup and disaster recovery capabilities, each service should be evaluated and have the recovery plan considered carefully.  Using storage replication may provide rapid recovery with a higher RPO, or can provide a starting baseline which expedites restore operations without having to recover all data from other locations.

etcd considerations
-------------------

You should carefully consider the high availability and data protection policy for the etcd instance used by the Kubernetes master(s).  This service is arguably the most critical to the overall functionality, recoverability, and serviceability of the cluster as a whole, so it's imperative that its deployment meets your goals.  

The most common method of providing high availability and resiliency to the etcd instance is to horizontally scale the application by having multiple instances across multiple nodes. A minimum of three nodes is recommended.

Kubernetes etcd will, by default, use local storage for its persistence.  This holds true whether etcd is co-located with other master services or is hosted on dedicated nodes.  To use enterprise storage for etcd a volume must be provisioned and mounted to the node at the correct location prior to deployment.  This storage cannot be dynamically provisioned by Trident or any other storage class provisioner as it is needed prior to the Kubernetes cluster being operational.

Refer to your Kubernetes provider's documentation on where to mount the volume and/or customize the etcd configuration to use non-default storage.

logging
-------

Centralized logging for applications deployed to the Kubernetes cluster is an optional service. Using enterprise storage for logging has the same benefits as with etcd: performance, resiliency, protection, security, and scale.

Depending on how and when the service is deployed, the storage class may define how to dynamically provision storage for the service. Refer to your vendor's documentation on how to customize the storage for logging services. Additionally, this document discusses Red Hat's OpenShift logging service best practices in a later chapter.

metrics
-------

There are many third-party metrics and analytics services available for monitoring and reporting of the status and health of every aspect of the cluster and application. Many of these require persistent storage, often with specific performance characteristics, for the service in order for it to function as intended.

Architecturally, many of these function similarly where an agent exists on each node which forwards data to a centralized collector to aggregate, analyze, and display the data.  Similar to the logging service, using entprise storage allows the aggregation service to move across nodes in the cluster seamlessly and ensures the data is protected in the event of node failure.

Each vendor has different recommendations and deployment methodology.  Work with your vendor to identify requirements and, if needed, provision storage from an enterprise array to meet the requirements.  This document will discuss the Red Hat OpenShift metrics service in a later chapter.

registry
--------

The registry is the service with which users and applications will have the most direct interaction. It can also have a dramatic affect on the perceived performance of the Kubernetes cluster as a whole, as slow image push and pull operations can result in lengthy times for tasks which directly affect the developer and application.

Fortunately, the registry is flexible with regard to storage protocol. Keep in mind different protocols have different implications.  

* Object storage is the default recommendation and is the simplest to use for Kubernetes deployments which expect to have significant scale or where the images need to be accessed across geographic regions.
* NFS is a a good choice for many deployments as it allows a single repository for the container images while allowing many registry endpoints to front the capacity.
* Block protocols, such as iSCSI, can be used for registry storage, but they introduce a single point of failure. The block device can only be attached to a single registry node due the single-writer limitation of the supported filesystems.

Protecting the images stored in the registry will have different priorities for each organization and each application. Registry images are, generally, either cached from upstream registries or have images pushed to them during the application build process. The RTO is important to the desired protection scheme because it will affect the recovery process.  If RTO is not an issue, then the applications may be able to simply rebuild the container images and push them into a new instance of the registry.  If faster RTO is desired, then a replication policy should be used which adheres to the desired recovery goal.

Design choices and guidelines when using ONTAP
==============================================

When using ONTAP as the backend storage for containerized applications, with storage dynamically provisioned by Trident, there are several design and implementation considerations which should be addressed prior to deployment.

Storage Virtual Machines
------------------------

Storage virtual machines (SVMs) are used for administrative delegation within ONTAP.  They give the storage administrator the ability to isolate a particular user, group, or application to only having access to resources which they have been specifically granted.  When Trident accesses the storage system via an SVM, it is prevented from doing many system level management tasks, providing additional isolation of capabilities for storage provisioning and management tasks.

There are several different ways which SVMs can be leveraged with Trident. Each is explained below. It's important to understand that having multiple Trident deployments, i.e. multiple Kubernetes clusters, does not change the below statements. When an SVM is shared with multiple Trident instances they simply need distinct prefixes defined in the backend configuration files.

**SVM shared with non Trident-managed workloads**

This configuration uses a single, or small number of, SVMs to host all of the workloads on the cluster and results in the containerized applications being hosted by the same SVM as other, non-containerized, workloads.  The shared SVM model is common in organizations where there exists multiple network segments which are isolated and adding additional IP addresses is difficult or impossible. 

There is nothing inherently wrong with this configuration, however it is more challenging to apply policies which affect only the container workloads.

**Dedicated SVM for Trident-managed workloads**

Creating an SVM which is used solely by Trident for provisioning and deprovisioning volumes for containerized workloads is the default recommendation from NetApp.  This enables the storage administrator to put controls in place to limit the amount of resources which Trident is able to consume.

As was noted above, having multiple Kubernetes clusters connect to and consume storage from the same SVM is acceptable, the only change to the Trident configuration should be to :ref:`provide a different prefix <Backend configuration options>`.

When creating backends which connect to the same underlying SVM resources, but have differing features applied, e.g. snapshot policies, using different prefixes is recommended to aid the storage administrator with identifying volumes and ensuring that no confusion ensues as a result.

**Multiple SVMs dedicated to Trident-managed workloads**

You may consider using multiple SVMs with Trident for many different reasons, including isolating applications and resource domains, strict control over resources, and to facilitate multitenacy.  It's also worth considering using at least two SVMs with any Kubernetes cluster to isolate persistent storage for cluster services from application storage.

When using multiple SVMs, with one dedicated to cluster services, the goal is to isolate and control the workload in a more flexible way.  This is possible because the expectation is that the Kubernetes cluster services SVM will not have dynamic provisioning happening against it in the same manner that the application SVM will.  Many of the persistent storage resources needed by the Kubernetes cluster must exist prior to Trident deployment and consequentially must be manually provisioned by the storage administrator.

Kubernetes cluster services
---------------------------

Even for cluster services persistent volumes created by Trident, there should be serious consideration given to using per-volume QoS policies, including QoS minimums when possible, and customizing the volume options for the application.  Below are the default recommendations for the cluster services, however you should evaluate your needs and adjust policies according to your data protection, performance, and availability requirements.  Despite these recommendations, you will still want to evaluate and determine what works best for your Kubernetes cluster and applications.

**etcd**

* The default snapshot policy is often adequate for protecting against data corruption and loss, however snapshots are not a backup strategy.  Some consideration should be given to increasing the frequency, and decreasing the retention period, for etcd volumes.  For example, keeping 24 hourly snapshots or 48 snapshots taken every 30 minutes, but not retaining them for more than one or two days.  Since any data loss for etcd can be problematic, having more frequent snapshots makes this scenario easier to recover from.
* If the disaster recovery plan is to recover the Kubernetes cluster as-is at the destination site, then these volumes should be replicated with SnapMirror or SnapVault.
* etcd does not have significant IOPS or throughput requirements, however latency can play a critical role in the responsiveness of the Kubernetes API server.  Whenever possible the lowest latency storage available should be used.
* A QoS policy should be leveraged to provide a minimum amount of IOPS to the etcd volume(s).  The minimum value will depend on the number of nodes and pods which are deployed to your Kubernetes cluster.  Monitoring should be used to verify that the configured policy is adequate and adjusted over time as the Kubernetes cluster expands.
* The etcd volumes should have their export policy or iGroup limited to only the nodes which are hosting, or could potentially host, etcd instances.

**logging**

* Volumes which are providing storage capacity for aggregated logging services need to be protected, however an average RPO is adequate in many instances since logging data is often not critical to recovery.  If your application has strict compliance requirements, this may be different however.
* Using the default snapshot policy is generally adequate.  Optionally, depending on the needs of the administrators, reducing the snapshot policy to one which keeps as few as seven daily snapshots may be acceptable.
* Logging volumes should be replicated to protect the historical data for use by the application and by administrators, however recovery may be deprioritized for other services.
* Logging services have widely variable IOPS requirements and read/write patterns.  It's important to consider the number of nodes, pods, and other objects in the cluster. Each of these will generate data which needs to be stored, indexed, analyzed, and presented, so a larger cluster may have substantially more data than expected.
* A QoS policy may be leveraged to provide both a minimum and maximum amount of throughput available.  Note that the maximum may need to be adjusted as additional pods are deployed, so close monitoring of the performance should be used to verify that logging is not being adversely affected by storage performance.
* The volumes export policy or iGroup should be limited to nodes which host the logging service.  This will depend on the particular solution used and the chosen configuration. For example OpenShift's logging service is deployed to the infrastructure nodes.

**metrics**

* Kubernetes autoscale feature relies on metrics to provide data for when scale operations need to occur.  Also, metrics data often plays a critical role in show-back and charge-back operations, so ensure that you are working to address the needs of the entire business with the RPO policy.  Ensure that your RPO and RTO meet the needs of these functions.
* As the number of cluster nodes and deployed pods increases, so too does the amount of data which is collected and retained by the metrics service.  It's important to understand the performance and capacity recommendations provided by the vendor for your metrics service as they can vary dramatically, particularly depending on the amount of time for which the data is retained and the number of metrics which are being monitored.
* A QoS policy can be used to limit the amount of IOPS or throughput which the metrics services uses, however it is generally not necessary to use a minimum policy.
* It is recommended to limit the export policy or iGroup to the hosts which the metrics service is executed from.  Note that it's important to understand the architecture of your metrics provider.  Many have agents which run on all hosts in the cluster, however those will report metrics to a centralised repository for storage and reporting.  Only that group of nodes needs access.

**registry**

* Using a snapshot policy for the registry data may be valuable for recovering from data corruption or other issues, however it is not necessary.  A basic snapshot policy is recommended, however individual container images cannot be recovered (they are stored in a hashed manner), only a full volume revert can be used to recover data.
* The workload for the registry can vary widely, however the general rule is that push operations happen infrequently, while pull operations happen frequently.  If a CI/CD pipeline process is used to build, test, and deploy the application(s) this may result in a predictable workload.  Alternatively, and even with a CI/CD system in use, the workload can vary based on application scaling requirements, build requirements, and even Kubernetes node add/remove operations.  Close monitoring of the workload should be implemented to adjust as necessary.
* A QoS policy may be implemented to ensure that application instances are still able to pull and deploy new container images regardless of other workloads on the storage system. In the event of a disaster recovery scenario, the registry may have a heavy read workload while applications are instantiated on the destination site. The configured QoS minimum policy will prevent other disaster recovery operations from slowing application deployment.
