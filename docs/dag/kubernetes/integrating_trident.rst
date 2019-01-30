.. _integrating_trident:

*******************
Integrating Trident
*******************

Trident backend design
======================

ONTAP
-----

**Choosing a backend driver for ONTAP**

Four different backend drivers are available for ONTAP systems. These drivers are differentiated by the protocol being used and how the volumes are provisioned on the storage system. Therefore, give careful consideration regarding which driver to deploy.  

At a higher level, if your application has components which need shared storage (multiple pods accessing the same PVC) NAS based drivers would be the default choice, while the block based iSCSI driver meets the needs of non-shared storage. Choose the protocol based on the requirements of the application and the comfort level of the storage and infrastructure teams. Generally speaking, there is little difference between them for most applications, so often the decision is based upon whether or not shared storage (where more than one pod will need simultaneous access) is needed.

The four :ref:`drivers <Choosing a driver>` for ONTAP backends are listed below:

* ``ontap-nas`` – each PV provisioned is a full ONTAP FlexVolume
* ``ontap-nas-economy`` – each PV provisioned is a qtree, with up to 200 qtrees per FlexVolume
* ``ontap-nas-flexgroup`` - each PV provisioned as a full ONTAP FlexGroup, and all aggregates assigned to a SVM are used.
* ``ontap-san`` – each PV provisioned is a LUN within its own FlexVolume

Choosing between the three NFS drivers has some ramifications to the features which are made available to the application.

Note that, in the tables below, not all of the capabilities are exposed through Trident. Some must be applied by the storage administrator after provisioning if that functionality is desired. The superscript footnotes distinguish the functionality per feature and driver.

.. table:: ONTAP NAS driver capabilities
   :align: left

   +-----------------------------+--------------+--------+--------------+---------------+--------+--------------+
   | ONTAP NFS Drivers           | Snapshots    | Clones | Multi-attach | QoS           | Resize | Replication  |
   +=============================+==============+========+==============+===============+========+==============+
   | ``ontap-nas``               | Yes          | Yes    | Yes          | Yes\ :sup:`2` | Yes    | Yes\ :sup:`2`|
   +-----------------------------+--------------+--------+--------------+---------------+--------+--------------+
   | ``ontap-nas-economy``       | Yes\ :sup:`1`| No     | Yes          | Yes\ :sup:`12`| Yes    | Yes\ :sup:`2`|
   +-----------------------------+--------------+--------+--------------+---------------+--------+--------------+
   | ``ontap-nas-flexgroup``     | Yes          | No     | Yes          | Yes\ :sup:`2` | Yes    | Yes\ :sup:`2`|
   +-----------------------------+--------------+--------+--------------+---------------+--------+--------------+


The SAN driver capabilities are shown below.

.. table:: ONTAP SAN driver capabilities
   :align: left


   +-----------------------------+-----------+--------+--------------+---------------+---------------+---------------+
   | ONTAP SAN Driver            | Snapshots | Clones | Multi-attach | QoS           | Resize        | Replication   |
   +=============================+===========+========+==============+===============+===============+===============+
   | ``ontap-san``               | Yes       | Yes    | No           | Yes\ :sup:`2` | Yes\ :sup:`2` | Yes\ :sup:`2` |
   +-----------------------------+-----------+--------+--------------+---------------+---------------+---------------+

| Footnote for above tables:
| Yes\ :sup:`1`:  Trident managed, but not PV granular
| Yes\ :sup:`2`:  Not Trident managed
| Yes\ :sup:`12`: Not Trident managed and not PV granular 


The features that are not PV granular are applied to the entire FlexVolume and all of the PVs (i.e. qtrees) will share a common schedule for each qtree.

As we can see in the above tables, much of the functionality between the ``ontap-nas`` and ``ontap-nas-economy`` is the same. However, since the ``ontap-nas-economy`` driver limits the ability to control the schedule at per-PV granularity, this may affect your disaster recovery and backup planning in particular. For development teams which desire to leverage PVC clone functionality on ONTAP storage, this is only possible when using the ``ontap-nas`` or ``ontap-san`` drivers (note, the ``solidfire-san`` driver is also capable of cloning PVCs).

SolidFire Backend Driver
-----------------------
The ``solidfire-san`` driver, used with the SolidFire platform, helps the admin configure a SolidFire backend for Trident on the basis of QoS limits. If you would like to design your backend to set the specific QoS limits on the volumes provisioned by Trident, use the `Type` parameter in the backend file. The admin also can restrict the volume size that could be created on the storage using the `limitVolumeSize` parameter. Currently SolidFire storage features like volume resize and volume replication are not supported through the ``solidfire-san`` driver. These operation should be done manually through Element OS Web UI. 

.. table:: SolidFire SAN driver capabilities
   :align: left

   +-------------------+-----------+--------+--------------+------+-------------------+---------------+
   | SolidFire Driver  | Snapshots | Clones | Multi-attach | QoS  | Resize            | Replication   |
   +===================+===========+========+==============+======+===================+===============+
   | ``solidfire-san`` | Yes       | Yes    | No           | Yes  | Yes\ :sup:`1`     | Yes\ :sup:`1` |
   +-------------------+-----------+--------+--------------+------+-------------------+---------------+
  

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed

Storage Class design
====================

Individual Storage Classes need to be configured and applied to create a Kubernetes storage class object. This section discusses how to design a storage class for your application.

Storage Class design For specific backend utilization
-----------------------------------------------------

Filtering can be used within a specific storage class object to determine which storage pool or set of pools are to be used with that specific storage class. Three sets of filters can be set in the Storage Class:  `storagePools`, `additionalStoragePools`, and/or `excludeStoragePools`. 

The `storagePools` parameter helps restrict storage to the set of pools that match any specified attributes. The `additionalStoragePools` parameter is used to extend the set of pools that Trident will use for provisioning along with the set of pools selected by the attributes and `storagePools` parameters. You can use either parameter alone or both together to make sure that appropriate set of storage pools are selected.

The `excludeStoragePools` parameter is used to specifically exclude the listed set of pools that match the attributes.

Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>`  on how these parameters are used.

Storage Class design To emulate QoS policies
-----------------------------------------------

If you would like to design Storage Classes to emulate Quality of Service policies, create a Storage Class with the `media` attribute as `hdd` or `ssd`. Based on the `media` attribute mentioned in the storage class, Trident will select the appropriate backend that serves `hdd` or `ssd` aggregates to match the media attribute and then direct the provisioning of the volumes on to the specific aggregate. Therefore we can create a storage class PREMIUM which would have `media` attribute set as `ssd` which could be classified as the PREMIUM QoS policy. We can create another storage class STANDARD which would would have the media attribute set as 'hdd' which could be classified as the STANDARD QoS policy. We could also use the “IOPS” attribute in the storage class to redirect provisioning to a SolidFire appliance which can be defined as a QoS Policy.


Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>` on how these parameters can be used.

Storage Class Design To utilize backend based on specific features
---------------------------------------------------------------------

Storage Classes can be designed to direct volume provisioning on a specific backend where features such as thin and thick provisioning, snapshots, clones and encryption are enabled. To specify which storage to use, create Storage Classes that specify the appropriate backend with the required feature enabled.

Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>` on how these parameters can be used.


PVC characteristics which affect storage provisioning
=====================================================

Some parameters beyond the requested storage class may affect Trident's provisioning decision process when creating a PVC.

Access mode
-----------

When requesting storage via a PVC, one of the mandatory fields is the access mode. The mode desired may affect the backend selected to host the storage request.

Trident will attempt to match the storage protocol used with the access method specified according to the following matrix. This is independent of the underlying storage platform.

.. table:: Protocols used by access modes
   :align: left
   
   +-------+---------------+--------------+---------------+
   |       | ReadWriteOnce | ReadOnlyMany | ReadWriteMany |
   +=======+===============+==============+===============+
   | iSCSI | Yes           | Yes          | No            |
   +-------+---------------+--------------+---------------+
   | NFS   | Yes           | Yes          | Yes           |
   +-------+---------------+--------------+---------------+
   
A request for a ReadWriteMany PVC submitted to a Trident deployment without an NFS backend configured will result in no volume being provisioned.  For this reason, the requestor should use the access mode which is appropriate for their application.

Modifying persistent volumes
============================

Persistent volumes are, with two exceptions, immutable objects in Kubernetes. Once created, the reclaim policy and the size can be modified. However, this doesn't prevent some aspects of the volume from being modified outside of Kubernetes. This may be desirable in order to customize the volume for specific applications, to ensure that capacity is not accidentally consumed, or simply to move the volume to a different storage controller for any reason.

.. note::
   Kubernetes in-tree provisioners do not support volume resize operations for NFS or iSCSI PVs at this time. Trident supports expanding NFS volumes. For a list of PV types which support volume resizing refer to the `Kubernetes documentation <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#expanding-persistent-volumes-claims>`_.

The connection details of the PV cannot be modified after creation.

Volume move operations
----------------------

Storage administrators have the ability to move volumes between aggregates and controllers in the ONTAP cluster non-disruptively to the storage consumer.  This operation does not affect Trident or the Kubernetes cluster, so long as the destination aggregate is one which the SVM Trident is using has access to.  Importantly, if the aggregate has been newly added to the SVM, the backend will need to be "refreshed" by re-adding it to Trident. This will trigger Trident to reinventory the SVM so that the new aggregate is recognized.

However, moving volumes across backends is not supported. This includes between SVMs in the same cluster, between clusters, or onto a different storage platform (even if that storage system is one which is connected to Trident).

Resizing volumes
----------------
To make sure that the Persistent Volumes provisioned by Trident can be resized later, create Persistent Volume based out of a PersistentVolume Claim that utilizes a Storage Class which allow  volume expansion by setting "allowVolumeExpansion” attribute as true. Whenever the Persistent Volume needs to be resized, edit the "spec.resources.requests.storage" annotation in the Persistent Volume Claim to the required volume size and Trident will automatically take care of resizing the volume on ONTAP.
 
.. note::
   1. Currently NFS PV resize is only supported by Trident and not iSCSI PV resize.
   2. Kubernetes, prior to version 1.12, does not support NFS PV resize as the admission controller may reject PVC size updates. The Trident team has changed Kubernetes to allow such changes starting with Kubernetes 1.12. While we recommend using Kubernetes 1.12, it is still possible to resize NFS PVs for earlier versions of Kubernetes that support resize. This is done by disabling the PersistentVolumeClaimResize admission plugin when the Kubernetes API server is started. 

When to manually provision a volume instead of using Trident
============================================================

Trident's goal is to be the provisioning engine for all storage consumed by containers.  However, we understand that there are scenarios which may still need a manually provisioned PV.  Generally speaking, these situations are limited to needing to customize the properties of the underlying storage device in ways which Trident does not support.

There are two ways which the desired settings can be applied:

#.  Use the backend configuration, or PVC attributes, to customize the volume properties at provisioning time
#.  After the volume is provisioned, the storage administrator applies configuration to the volume which is bound to the PVC

Option number 1 is limited by the volume options with Trident supports, which do not encompass all of the options available.  Option 2 may be the only viable solution for fully customizing storage for a particular application.  Finally, you can always provision a volume manually and introduce a matching PV outside of Trident if you do not want Trident to manage it for some reason.

If you have requirements to customize volumes in ways which Trident does not support, please let us know using resources on the :ref:`contact_us` page.

Deploying OpenShift services using Trident
==========================================

The OpenShift value-add cluster services provide important functionality to cluster administrators and the applications being hosted.  The storage which these services use can be provisioned using the node-local resources, however this often limits the capacity, performance, recoverability, and sustainability of the service.  Leveraging an enterprise storage array to provide capacity to these services can enable dramatically improved service, however, as with all applications, the OpenShift and storage administrators should work closely together to determine the best options for each.  The Red Hat documentation should be leveraged heavily to determine the requirements and ensure that sizing and performance needs are met.

Registry service
----------------

Deploying and managing storage for the registry has been documented on `netapp.io <https://netapp.io/>`_ in `this blog post <https://netapp.io/2017/08/24/deploying-the-openshift-registry-using-netapp-storage/>`_.

Logging service
---------------

Like other OpenShift services, the logging service is deployed using Ansible with configuration parameters supplied by the inventory, a.k.a. hosts, file provided to the playbook.  There are two installation methods which will be covered: deploying logging during initial OpenShift install and deploying logging after OpenShift has been installed.

.. warning::
   As of Red Hat OpenShift version 3.9, the official documentation recommends against NFS for the logging service due to concerns around data corruption. This is based on Red Hat testing of their products. ONTAP's NFS server does not have these issues, and can easily back a logging deployment. Ultimately, the choice of protocol for the logging service is up to you, just know that both will work great when using NetApp platforms and there is no reason to avoid NFS if that is your preference.
   
   If you choose to use NFS with the logging service, you will need to set the Ansible variable ``openshift_enable_unsupported_configurations`` to ``true`` to prevent the installer from failing.

**Getting started**

The logging service can, optionally, be deployed for both applications as well as for the core operations of the OpenShift cluster itself.  If you choose to deploy operations logging, by specifying the variable ``openshift_logging_use_ops`` as ``true``, two instances of the service will be created.  The variables which control the logging instance for operations contain "ops" in them, whereas the instance for applications do not.

Configuring the Ansible variables according to the deployment method is important in order to ensure that the correct storage is utilized by the underlying services.  Let's look at the options for each of the deployment methods

.. note::
   The tables below only contain the variables which are relevant for storage configuration as it relates to the logging service.  There are many other options found in `the documentation <https://docs.openshift.com/container-platform/latest/install_config/aggregate_logging.html>`_ which should be reviewed, configured, and used according to your deployment.

The variables in the below table will result in the Ansible playbook creating a PV and PVC for the logging service using the details provided.  This method is significantly less flexible than using the component installation playbook after OpenShift installation, however if you have existing volumes available, it is an option.

.. table:: Logging variables when deploying at OpenShift install time
   :align: left
   
   +---------------------------------------------+------------------------------------------------+
   | Variable                                    | Details                                        |
   +=============================================+================================================+
   | ``openshift_logging_storage_kind``          | Set to ``nfs`` to have the installer create an |
   |                                             | NFS PV for the logging service.                |
   +---------------------------------------------+------------------------------------------------+
   | ``openshift_logging_storage_host``          | The hostname or IP address of the NFS host.    |
   |                                             | This should be set to the data LIF for your    |
   |                                             | virtual machine.                               |
   +---------------------------------------------+------------------------------------------------+
   | ``openshift_logging_storage_nfs_directory`` | The mount path for the NFS export.  For        |
   |                                             | example, if the volume is junctioned as        |
   |                                             | ``/openshift_logging``, you would use that     |
   |                                             | path for this variable.                        |
   +---------------------------------------------+------------------------------------------------+
   | ``openshift_logging_storage_volume_name``   | The name, e.g. ``pv_ose_logs``, of the PV to   |
   |                                             | create.                                        |
   +---------------------------------------------+------------------------------------------------+
   | ``openshift_logging_storage_volume_size``   | The size of the NFS export, for example        |
   |                                             | ``100Gi``.                                     |
   +---------------------------------------------+------------------------------------------------+

If your OpenShift cluster is already running, and therefore Trident has been deployed and configured, the installer can use dynamic provisioning to create the volumes.  The following variables will need to be configured.

.. table:: Logging variables when deploying after OpenShift install
   :align: left
   
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | Variable                                            | Details                                                                              |
   +=====================================================+======================================================================================+
   | ``openshift_logging_es_pvc_dynamic``                | Set to true to use dynamically provisioned volumes.                                  |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_pvc_storage_class_name``     | The name of the storage class which will be used in the PVC.                         |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_pvc_size``                   | The size of the volume requested in the PVC.                                         |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_pvc_prefix``                 | A prefix for the PVCs used by the logging service.                                   |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_ops_pvc_dynamic``            | Set to ``true`` to use dynamically provisioned volumes for the ops logging instance. |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_ops_pvc_storage_class_name`` | The name of the storage class for the ops logging instance.                          |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_ops_pvc_size``               | The size of the volume request for the ops instance.                                 |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   | ``openshift_logging_es_ops_pvc_prefix``             | A prefix for the ops instance PVCs.                                                  |
   +-----------------------------------------------------+--------------------------------------------------------------------------------------+
   
.. note::
   A bug exists in OpenShift 3.9 which prevents a storage class from being used when the value for ``openshift_logging_es_ops_pvc_dynamic`` is set to ``true``.  However, this can be worked around by, counterintuitively, setting the variable to ``false``, which will include the storage class in the PVC.

**Deploy the logging stack**

If you are deploying logging as a part of the initial OpenShift install process, then you only need to follow the standard deployment process.  Ansible will configure and deploy the needed services and OpenShift objects so that the service is available as soon as Ansible completes.

However, if you are deploying after the initial installation, the component playbook will need to be used by Ansible. This process may change slightly with different versions of OpenShift, so be sure to read and follow `the documentation <https://docs.openshift.com/container-platform/3.11/welcome/index.html>`_ for your version.

Metrics service
---------------

The metrics service provides valuable information to the administrator regarding the status, resource utilization, and availability of the OpenShift cluster.  It is also necessary for pod autoscale functionality and many organizations use data from the metrics service for their charge back and/or show back applications.

Like with the logging service, and OpenShift as a whole, Ansible is used to deploy the metrics service.  Also, like the logging service, the metrics service can be deployed during initial setup of the cluster or after its operational using the component installation method.  The following tables contain the variables which are important when configuring persistent storage for the metrics service.

.. note::
   The tables below only contain the variables which are relevant for storage configuration as it relates to the metrics service.  There are many other options found in the documentation which should be reviewed, configured, and used according to your deployment.

.. table:: Metrics variables when deploying at OpenShift install time
   :align: left
   
   +---------------------------------------------+-----------------------------------------------------+
   | Variable                                    | Details                                             |
   +=============================================+=====================================================+
   | ``openshift_metrics_storage_kind``          | Set to ``nfs`` to have the installer create an NFS  |
   |                                             | PV for the logging service.                         |
   +---------------------------------------------+-----------------------------------------------------+
   | ``openshift_metrics_storage_host``          | The hostname or IP address of the NFS host. This    |
   |                                             | should be set to the data LIF for your SVM.         |
   +---------------------------------------------+-----------------------------------------------------+
   | ``openshift_metrics_storage_nfs_directory`` | The mount path for the NFS export.  For example, if |
   |                                             | the volume is junctioned as ``/openshift_metrics``, |
   |                                             | you would use that path for this variable.          |
   +---------------------------------------------+-----------------------------------------------------+
   | ``openshift_metrics_storage_volume_name``   | The name, e.g. ``pv_ose_metrics``, of the PV to     |
   |                                             | create.                                             |
   +---------------------------------------------+-----------------------------------------------------+
   | ``openshift_metrics_storage_volume_size``   | The size of the NFS export, for example ``100Gi``.  |
   +---------------------------------------------+-----------------------------------------------------+

If your OpenShift cluster is already running, and therefore Trident has been deployed and configured, the installer can use dynamic provisioning to create the volumes.  The following variables will need to be configured.

.. table:: Metrics variables when deploying after OpenShift install
   :align: left
   
   +-------------------------------------------------------+-------------------------------------------------------------+
   | Variable                                              | Details                                                     |
   +=======================================================+=============================================================+
   | ``openshift_metrics_cassandra_pvc_prefix``            | A prefix to use for the metrics PVCs.                       |
   +-------------------------------------------------------+-------------------------------------------------------------+
   | ``openshift_metrics_cassandra_pvc_size``              | The size of the volumes to request.                         |
   +-------------------------------------------------------+-------------------------------------------------------------+
   | ``openshift_metrics_cassandra_storage_type``          | The type of storage to use for metrics, this must be set to |
   |                                                       | dynamic for Ansible to create PVCs with the appropriate     |
   |                                                       | storage class.                                              |
   +-------------------------------------------------------+-------------------------------------------------------------+
   | ``openshift_metrics_cassanda_pvc_storage_class_name`` | The name of the storage class to use.                       |
   +-------------------------------------------------------+-------------------------------------------------------------+

**Deploying the metrics service**

With the appropriate Ansible variables defined in your hosts/inventory file, deploy the service using Ansible.  If you are deploying at OpenShift install time, then the PV will be created and used automatically.  If you're deploying using the component playbooks, after OpenShift install, then Ansible will create any PVCs which are needed and, after Trident has provisioned storage for them, deploy the service.

The variables above, and the process for deploying, may change with each version of OpenShift.  Ensure you review and follow `the deployment guide <https://docs.openshift.com/container-platform/latest/install_config/cluster_metrics.html>`_ for your version so that it is configured for your environment.
