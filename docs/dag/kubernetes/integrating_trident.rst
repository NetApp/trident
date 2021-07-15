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

At a higher level, if your application has components which need shared storage (multiple pods accessing the same PVC) NAS based drivers would be the default choice, while the block-based iSCSI drivers meets the needs of non-shared storage. Choose the protocol based on the requirements of the application and the comfort level of the storage and infrastructure teams. Generally speaking, there is little difference between them for most applications, so often the decision is based upon whether or not shared storage (where more than one pod will need simultaneous access) is needed.

The five drivers for ONTAP backends are listed below:

* ``ontap-nas`` – Each PV provisioned is a full ONTAP FlexVolume.
* ``ontap-nas-economy`` – Each PV provisioned is a qtree, with a configurable number of qtrees per FlexVolume (default is 200).
* ``ontap-nas-flexgroup`` - Each PV provisioned as a full ONTAP FlexGroup, and all aggregates assigned to a SVM are used.
* ``ontap-san`` – Each PV provisioned is a LUN within its own FlexVolume.
* ``ontap-san-economy`` - Each PV provisioned is a LUN, with a configurable number of LUNs per FlexVolume (default is 100).

Choosing between the three NFS drivers has some ramifications to the features which are made available to the application.

Note that, in the tables below, not all of the capabilities are exposed through Trident. Some must be applied by the storage administrator after provisioning if that functionality is desired. The superscript footnotes distinguish the functionality per feature and driver.

.. table:: ONTAP NAS driver capabilities

   +-----------------------------+---------------+-----------------+-------------------------+--------------+---------------+--------+---------------+
   | ONTAP NFS Drivers           | Snapshots     |      Clones     | Dynamic Export Policies | Multi-attach | QoS           | Resize |  Replication  |
   +=============================+===============+=================+=========================+==============+===============+========+===============+
   | ``ontap-nas``               | Yes           |        Yes      |      Yes\ :sup:`4`      | Yes          | Yes\ :sup:`1` | Yes    | Yes\ :sup:`1` |
   +-----------------------------+---------------+-----------------+-------------------------+--------------+---------------+--------+---------------+
   | ``ontap-nas-economy``       | Yes\ :sup:`12`|  Yes\ :sup:`12` |      Yes\ :sup:`4`      | Yes          | Yes\ :sup:`12`| Yes    | Yes\ :sup:`12`|
   +-----------------------------+---------------+-----------------+-------------------------+--------------+---------------+--------+---------------+
   | ``ontap-nas-flexgroup``     | Yes\ :sup:`1` |         No      |      Yes\ :sup:`4`      | Yes          | Yes\ :sup:`1` | Yes    | Yes\ :sup:`1` |
   +-----------------------------+---------------+-----------------+-------------------------+--------------+---------------+--------+---------------+


Trident offers 2 SAN drivers for ONTAP, whose capabilities are shown below.

.. table:: ONTAP SAN driver capabilities

   +-----------------------------+-----------+--------+--------------+--------------------+---------------+---------------+---------------+
   | ONTAP SAN Driver            | Snapshots | Clones | Multi-attach | Bidirectional CHAP | QoS           | Resize        | Replication   |
   +=============================+===========+========+==============+====================+===============+===============+===============+
   | ``ontap-san``               | Yes       | Yes    | Yes\ :sup:`3`|        Yes         | Yes\ :sup:`1` |      Yes      | Yes\ :sup:`1` |
   +-----------------------------+-----------+--------+--------------+--------------------+---------------+---------------+---------------+
   | ``ontap-san-economy``       | Yes       | Yes    | Yes\ :sup:`3`|        Yes         | Yes\ :sup:`12`| Yes\ :sup:`1` | Yes\ :sup:`12`|
   +-----------------------------+-----------+--------+--------------+--------------------+---------------+---------------+---------------+

| Footnote for the above tables:
| Yes\ :sup:`1` :  Not Trident managed
| Yes\ :sup:`2` :  Trident managed, but not PV granular
| Yes\ :sup:`12`:  Not Trident managed and not PV granular
| Yes\ :sup:`3` :  Supported for raw-block volumes
| Yes\ :sup:`4` :  Supported by CSI Trident


The features that are not PV granular are applied to the entire FlexVolume and all of the PVs (i.e. qtrees or LUNs in shared FlexVols) will share a common schedule.

As we can see in the above tables, much of the functionality between the ``ontap-nas`` and ``ontap-nas-economy`` is the same. However, since the ``ontap-nas-economy`` driver limits the ability to control the schedule at per-PV granularity, this may affect your disaster recovery and backup planning in particular. For development teams which desire to leverage PVC clone functionality on ONTAP storage, this is only possible when using the ``ontap-nas``, ``ontap-san`` or ``ontap-san-economy`` drivers (note, the ``solidfire-san`` driver is also capable of cloning PVCs).


**Choosing a backend driver for Cloud Volumes ONTAP**

Cloud Volumes ONTAP provides data control along with enterprise-class storage features for various use cases, including file shares and block-level storage serving NAS and SAN protocols (NFS, SMB / CIFS, and iSCSI). The compatible drivers for Cloud Volume ONTAP are ``ontap-nas``, ``ontap-nas-economy``, ``ontap-san`` and
``ontap-san-economy``. These are applicable for Cloud Volume ONTAP for AWS, Cloud Volume ONTAP for Azure, Cloud Volume ONTAP for GCP.


Element software (NetApp HCI/SolidFire)
---------------------------------------
The ``solidfire-san`` driver used with the NetApp HCI/SolidFire platforms, helps the admin configure an Element backend for Trident on the basis of QoS limits. If you would like to design your backend to set the specific QoS limits on the volumes provisioned by Trident, use the ``type`` parameter in the backend file. The admin also can restrict the volume size that could be created on the storage using the `limitVolumeSize` parameter. Currently, Element storage features like volume resize and volume replication are not supported through the ``solidfire-san`` driver. These operations should be done manually through Element software web UI.

.. table:: SolidFire SAN driver capabilities

   +-------------------+----------------+--------+--------------+------+------+--------+---------------+
   | SolidFire Driver  | Snapshots      | Clones | Multi-attach | CHAP | QoS  | Resize | Replication   |
   +===================+================+========+==============+======+======+========+===============+
   | ``solidfire-san`` | Yes            | Yes    | Yes\ :sup:`2`| Yes  | Yes  |   Yes  | Yes\ :sup:`1` |
   +-------------------+----------------+--------+--------------+------+------+--------+---------------+

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed
| Yes\ :sup:`2`: Supported for raw-block volumes

SANtricity (E-Series)
------------------------
To configure an E-Series backend for Trident, set the ``storageDriverName`` parameter to ``eseries-iscsi`` driver in the backend configuration. Once the E-Series backend has been configured, any requests to provision volume from the E-Series will be handled by Trident based on the host groups. Trident uses host groups to gain access to the LUNs that it provisions and by default, it looks for a host group named ``trident`` unless a different host group name is specified using the ``accessGroupName`` parameter in the backend configuration. The admin also can restrict the volume size that could be created on the storage using the `limitVolumeSize` parameter. Currently, E-Series storage features like volume resize and volume replication are not supported through the ``eseries-iscsi`` driver. These operations should be done manually through SANtricity System Manager.

.. table:: E-Series driver capabilities

   +-------------------+---------------+---------------+--------------+------+--------+---------------+
   | E-Series Driver   | Snapshots     | Clones        | Multi-attach | QoS  | Resize | Replication   |
   +===================+===============+===============+==============+======+========+===============+
   | ``eseries-iscsi`` | Yes\ :sup:`1` | Yes\ :sup:`1` | Yes\ :sup:`2`| No   |   Yes  | Yes\ :sup:`1` |
   +-------------------+---------------+---------------+--------------+------+--------+---------------+

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed
| Yes\ :sup:`2`:  Supported for raw-block volumes

Azure NetApp Files Backend Driver
---------------------------------

Trident uses the ``azure-netapp-files`` driver to manage the `Azure NetApp Files`_ service.

.. _Azure NetApp Files: https://azure.microsoft.com/en-us/services/netapp/

More information about this driver and how to configure it can be found in Trident's
:ref:`Azure NetApp Files backend documentation <Azure NetApp Files>`.

.. table:: Azure NetApp Files driver capabilities

   +---------------------------+--------------+--------+--------------+------+-------------------+---------------+
   | Azure NetApp Files Driver | Snapshots    | Clones | Multi-attach | QoS  | Expand            | Replication   |
   +===========================+==============+========+==============+======+===================+===============+
   | ``azure-netapp-files``    | Yes          | Yes    | Yes          | Yes  | Yes               | Yes\ :sup:`1` |
   +---------------------------+--------------+--------+--------------+------+-------------------+---------------+

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed

Cloud Volumes Service with AWS Backend Driver
---------------------------------------------

Trident uses the ``aws-cvs`` driver to link with the Cloud Volumes Service on the AWS backend. To configure the AWS backend on Trident, you are required specify ``apiRegion``, ``apiURL``, ``apiKey``, and the ``secretKey`` in the backend file. These values can be found in the CVS web portal in Account settings/API access. The supported service levels are aligned with CVS and include `standard`, `premium`, and `extreme`. More information on this driver may be found in the :ref:`Cloud Volumes Service for AWS Documentation <Cloud Volumes Service for AWS>`. Currently, 100G is the minimum volume size that will be provisioned. Future releases of CVS may remove this restriction.

.. table:: Cloud Volume Service driver capabilities

   +--------------------+--------------+--------+--------------+------+-------------------+---------------+
   | CVS for AWS Driver | Snapshots    | Clones | Multi-attach | QoS  | Expand            | Replication   |
   +====================+==============+========+==============+======+===================+===============+
   | ``aws-cvs``        | Yes          | Yes    |  Yes         | Yes  | Yes               | Yes\ :sup:`1` |
   +--------------------+--------------+--------+--------------+------+-------------------+---------------+

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed

The ``aws-cvs`` driver uses virtual storage pools. Virtual storage pools abstract the backend, letting Trident decide volume placement. The administrator defines the virtual storage pools in the backend.json file(s). Storage classes identify the virtual storage pools with the use of labels. More information on the virtual storage pools feature can be found in :ref:`Virtual Storage Pools Documentation <Virtual Storage Pools>`.

Cloud Volumes Service with GCP Backend Driver
---------------------------------------------

Trident uses the ``gcp-cvs`` driver to link with the Cloud Volumes Service on the GCP backend. To configure the GCP backend on Trident, you are required specify ``projectNumber``, ``apiRegion``, and ``apiKey`` in the backend file. The project number may be found in the GCP web portal, while the API key must be taken from the service account private key file that you created while setting up API access for Cloud Volumes on GCP.
Trident can create CVS volumes in one of two `service types <https://cloud.google.com/architecture/partners/netapp-cloud-volumes/service-types>`_:

1. **CVS**: The base CVS service type, which provides high zonal availability with
   limited/moderate performance levels.
2. **CVS-Performance**: Performance-optimized service type best suited
   for production workloads that value performance. Choose from three unique service levels
   [`standard`, `premium`, and `extreme`].

More information on this driver may be found in the :ref:`Cloud Volumes Service for GCP Documentation <Cloud Volumes Service for GCP>`.
Currently, 100 GiB is the minimum CVS-Performance volume size that will be provisioned, while CVS volumes must be at
least 300 GiB. Future releases of CVS may remove this restriction.

.. warning::

 When deploying backends using the default CVS service type [``storageClass=software``],
 users **must obtain access** to the sub-1TiB volumes feature on GCP for the Project Number(s)
 and Project ID(s) in question. This is necessary for Trident to provision sub-1TiB volumes.
 If not, volume creations **will fail** for PVCs that are <600 GiB. Obtain access to sub-1TiB
 volumes using `this <https://docs.google.com/forms/d/e/1FAIpQLSc7_euiPtlV8bhsKWvwBl3gm9KUL4kOhD7lnbHC3LlQ7m02Dw/viewform>`_
 form.

.. table:: Cloud Volume Service driver capabilities

   +--------------------+--------------+--------+--------------+------+-------------------+---------------+
   | CVS for GCP Driver | Snapshots    | Clones | Multi-attach | QoS  | Expand            | Replication   |
   +====================+==============+========+==============+======+===================+===============+
   | ``gcp-cvs``        | Yes          | Yes    |  Yes         | Yes  | Yes               | Yes\ :sup:`1` |
   +--------------------+--------------+--------+--------------+------+-------------------+---------------+

| Footnote:
| Yes\ :sup:`1`:  Not Trident managed

The ``gcp-cvs`` driver uses virtual storage pools. Virtual storage pools abstract the backend, letting Trident decide volume placement. The administrator defines the virtual storage pools in the backend.json file(s). Storage classes identify the virtual storage pools with the use of labels. More information on the virtual storage pools feature can be found in :ref:`Virtual Storage Pools Documentation <Virtual Storage Pools>`.


Storage Class design
====================

Individual Storage Classes need to be configured and applied to create a Kubernetes Storage Class object. This section discusses how to design a storage class for your application.

Storage Class design for specific backend utilization
-----------------------------------------------------

Filtering can be used within a specific storage class object to determine which storage pool or set of pools are to be used with that specific storage class. Three sets of filters can be set in the Storage Class:  `storagePools`, `additionalStoragePools`, and/or `excludeStoragePools`.

The `storagePools` parameter helps restrict storage to the set of pools that match any specified attributes. The `additionalStoragePools` parameter is used to extend the set of pools that Trident will use for provisioning along with the set of pools selected by the attributes and `storagePools` parameters. You can use either parameter alone or both together to make sure that the appropriate set of storage pools are selected.

The `excludeStoragePools` parameter is used to specifically exclude the listed set of pools that match the attributes.

Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>`  on how these parameters are used.

Storage Class design to emulate QoS policies
--------------------------------------------

If you would like to design Storage Classes to emulate Quality of Service policies, create a Storage Class with the `media` attribute as `hdd` or `ssd`. Based on the `media` attribute mentioned in the storage class, Trident will select the appropriate backend that serves `hdd` or `ssd` aggregates to match the media attribute and then direct the provisioning of the volumes on to the specific aggregate. Therefore we can create a storage class PREMIUM which would have `media` attribute set as `ssd` which could be classified as the PREMIUM QoS policy. We can create another storage class STANDARD which would have the media attribute set as 'hdd' which could be classified as the STANDARD QoS policy. We could also use the “IOPS” attribute in the storage class to redirect provisioning to an Element appliance which can be defined as a QoS Policy.


Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>` on how these parameters can be used.

Storage Class Design To utilize backend based on specific features
------------------------------------------------------------------

Storage Classes can be designed to direct volume provisioning on a specific backend where features such as thin and thick provisioning, snapshots, clones, and encryption are enabled. To specify which storage to use, create Storage Classes that specify the appropriate backend with the required feature enabled.

Please refer to :ref:`Trident StorageClass Objects <Trident StorageClass objects>` on how these parameters can be used.

Storage Class Design for Virtual Storage Pools
----------------------------------------------
Virtual Storage Pools are available for all Trident backends. You can define Virtual Storage Pools
for any backend, using any driver that Trident provides.

Virtual Storage Pools allow an administrator to create a level of abstraction over backends which can be referenced through Storage Classes, for greater flexibility and efficient placement of volumes on backends. Different backends can be defined with the same class of service. Moreover, multiple Storage Pools can be created on the same backend but with different characteristics. When a Storage Class is configured with a selector with the specific labels , Trident chooses a backend which matches all the selector labels to place the volume. If the Storage Class selector labels matches multiple Storage Pools, Trident will choose one of them to provision the volume from.

Please refer to :ref:`Virtual Storage Pools <Virtual Storage Pools>` for more information and applicable parameters.

Virtual Storage Pool Design
===========================

While creating a backend, you can generally specify a set of parameters.
It was impossible for the administrator to create another backend with the same
storage credentials and with a different set of parameters. With the
introduction of Virtual Storage Pools, this issue has been alleviated. Virtual
Storage Pools is a level abstraction introduced between the backend and the
Kubernetes Storage Class so that the administrator can define parameters along
with labels which can be referenced through Kubernetes Storage Classes as a
selector, in a backend-agnostic way. Virtual Storage Pools can be defined for
all supported NetApp backends with Trident. That list includes E-Series,
SolidFire/HCI, ONTAP, Cloud Volumes Service on AWS and GCP, as well as Azure
NetApp Files.

.. note::

   When defining Virtual Storage Pools, it is recommended to not attempt to rearrange
   the order of existing virtual pools in a backend definition. It is also advisable
   to not edit/modify attributes for an existing virtual pool and define a new virtual
   pool instead.

Design Virtual Storage Pools for emulating different Service Levels/QoS
-----------------------------------------------------------------------

It is possible to design Virtual Storage Pools for emulating service classes. Using the virtual pool implementation for Cloud Volume Service for AWS, let us examine how we can setup up different service classes. Configure the AWS-CVS backend with multiple labels, representing different performance levels. Set "servicelevel" aspect to the appropriate performance level and add other required aspects under each labels. Now create different Kubernetes Storage Classes that would map to different virtual Storage Pools. Using the ``parameters.selector`` field, each StorageClass calls out which virtual pool(s) may be used to host a volume.

Design Virtual Pools for Assigning Specific Set of Aspects
----------------------------------------------------------

Multiple Virtual Storage pools with a specific set of aspects can be designed from a single storage backend. For doing so, configure the backend with multiple labels and set the required aspects under each label. Now create different Kubernetes Storage Classes using the ``parameters.selector`` field that would map to different Virtual Storage Pools.The volumes that get provisioned on the backend will have the aspects defined in the chosen Virtual Storage Pool.

PVC characteristics which affect storage provisioning
=====================================================

Some parameters beyond the requested storage class may affect Trident's provisioning decision process when creating a PVC.

Access mode
-----------

When requesting storage via a PVC, one of the mandatory fields is the access mode. The mode desired may affect the backend selected to host the storage request.

Trident will attempt to match the storage protocol used with the access method specified according to the following matrix. This is independent of the underlying storage platform.

.. table:: Protocols used by access modes

   +-------+---------------+--------------+---------------+
   |       | ReadWriteOnce | ReadOnlyMany | ReadWriteMany |
   +=======+===============+==============+===============+
   | iSCSI | Yes           | Yes          | Yes(Raw block)|
   +-------+---------------+--------------+---------------+
   | NFS   | Yes           | Yes          | Yes           |
   +-------+---------------+--------------+---------------+

A request for a ReadWriteMany PVC submitted to a Trident deployment without an NFS backend configured will result in no volume being provisioned.  For this reason, the requestor should use the access mode which is appropriate for their application.

Volume Operations
=================

Modifying persistent volumes
----------------------------

Persistent volumes are, with two exceptions, immutable objects in Kubernetes. Once created, the reclaim policy and the size can be modified. However, this doesn't prevent some aspects of the volume from being modified outside of Kubernetes. This may be desirable in order to customize the volume for specific applications, to ensure that capacity is not accidentally consumed, or simply to move the volume to a different storage controller for any reason.

.. note::
   Kubernetes in-tree provisioners do not support volume resize operations for NFS or iSCSI PVs at this time. Trident supports expanding both NFS and iSCSI volumes. For a list of PV types which support volume resizing refer to the `Kubernetes documentation <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#expanding-persistent-volumes-claims>`_.

The connection details of the PV cannot be modified after creation.

On-Demand Volume Snapshots with Trident's Enhanced CSI Provisioner
------------------------------------------------------------------

Trident supports on-demand volume snapshot creation and
the creation of PVCs from snapshots using the CSI framework. Snapshots
provide a convenient method of maintaining point-in-time copies of the data and have
a lifecycle independent of the source PV in Kubernetes. These snapshots can be used
to clone PVCs.

The :ref:`Volume Snapshots <On-Demand Volume Snapshots>` section provides
an example that explains how volume snapshots work.

Creating Volumes from Snapshots with Trident's Enhanced CSI Provisioner
-----------------------------------------------------------------------

Trident also supports the creation of PersistentVolumes from volume snapshots.
To accomplish this, just create a PersistentVolumeClaim and mention the ``datasource``
as the required snapshot from which the volume needs to be created. Trident will handle this
PVC by creating a volume with the data present on the snapshot. With this feature, it is possible
to duplicate data across regions, create test environments, replace a damaged or corrupted production
volume in its entirety, or retrieve specific files and directories and transfer them to another attached volume.

Take a look at :ref:`Creating PVCs from Snapshots <Create PVCs from VolumeSnapshots>`
for more information.


Volume Move Operations
----------------------

Storage administrators have the ability to move volumes between aggregates and controllers in the ONTAP cluster non-disruptively to the storage consumer.  This operation does not affect Trident or the Kubernetes cluster, as long as the destination aggregate is one which the SVM Trident is using has access to.  Importantly, if the aggregate has been newly added to the SVM, the backend will need to be "refreshed" by re-adding it to Trident. This will trigger Trident to reinventory the SVM so that the new aggregate is recognized.

However, moving volumes across backends is not supported automatically by Trident. This includes between SVMs in the same cluster, between clusters, or onto a different storage platform (even if that storage system is one which is connected to Trident).

If a volume is copied to another location, the :ref:`volume import feature <Importing a volume>` may be used to import current volumes into Trident.

Expanding volumes
-----------------

Trident supports resizing NFS and iSCSI PVs, beginning with the ``18.10`` and ``19.10``
releases respectively. This enables users to resize their volumes directly through
the Kubernetes layer. Volume expansion is possible for all major NetApp storage platforms,
including ONTAP, Element/HCI and Cloud Volumes Service backends.
Take a look at the :ref:`Expanding an NFS volume` and
:ref:`Expanding an iSCSI volume` for examples and conditions that must be met.
To allow possible expansion later, set `allowVolumeExpansion` to `true` in your StorageClass associated with the volume. Whenever the Persistent Volume needs to be resized, edit the ``spec.resources.requests.storage`` annotation in the Persistent Volume Claim to the required volume size. Trident will automatically take care of resizing the volume on the storage cluster.

Import an existing volume into Kubernetes
-----------------------------------------

Volume Import provides the ability to import an existing storage volume into a Kubernetes environment. This is currently
supported by the ``ontap-nas``, ``ontap-nas-flexgroup``, ``solidfire-san``, ``azure-netapp-files``, ``aws-cvs``, and
``gcp-cvs`` drivers. This feature is useful when porting an existing application into Kubernetes or during disaster
recovery scenarios.

When using the ONTAP and ``solidfire-san`` drivers, use the command ``tridentctl import volume <backend-name> <volume-name> -f /path/pvc.yaml``
to import an existing volume into Kubernetes to be managed by Trident. The PVC YAML or JSON file used in the import volume
command points to a storage class which identifies Trident as the provisioner. When using a HCI/SolidFire
backend, ensure the volume names are unique. If the volume names are duplicated, clone the volume to a unique name so
the volume import feature can distinguish between them.

If the ``aws-cvs``, ``azure-netapp-files`` or ``gcp-cvs`` driver is used, use the command ``tridentctl import volume <backend-name> <volume path> -f /path/pvc.yaml`` to import the volume into Kubernetes to be managed by Trident. This ensures a unique volume reference.

When the above command is executed, Trident will find the volume on the backend and read its size. It will automatically add (and overwrite if necessary) the configured PVC’s volume size.  Trident then creates the new PV and Kubernetes binds the PVC to the PV.

If a container was deployed such that it required the specific imported PVC, it would remain in a pending state until the PVC/PV pair are bound via the volume import process. After the PVC/PV pair are bound, the container should come up, provided there are no other issues.

For information, please see the :ref:`documentation <Importing a Volume>`.

Deploying OpenShift services using Trident
==========================================

The OpenShift value-add cluster services provide important functionality to cluster administrators and the applications being hosted.  The storage which these services use can be provisioned using the node-local resources, however, this often limits the capacity, performance, recoverability, and sustainability of the service. Leveraging an enterprise storage array to provide the capacity to these services can enable dramatically improved service, however, as with all applications, the OpenShift and storage administrators should work closely together to determine the best options for each.  The Red Hat documentation should be leveraged heavily to determine the requirements and ensure that sizing and performance needs are met.

Registry service
----------------

Deploying and managing storage for the registry has been documented on `netapp.io <https://netapp.io/>`_ in `this blog post <https://netapp.io/2017/08/24/deploying-the-openshift-registry-using-netapp-storage/>`_.

Logging service
---------------

Like other OpenShift services, the logging service is deployed using Ansible with configuration parameters supplied by the inventory file, a.k.a. hosts, provided to the playbook.  There are two installation methods which will be covered: deploying logging during initial OpenShift install and deploying logging after OpenShift has been installed.

.. warning::
   As of Red Hat OpenShift version 3.9, the official documentation recommends against NFS for the logging service due to concerns around data corruption. This is based on Red Hat testing of their products. ONTAP's NFS server does not have these issues, and can easily back a logging deployment. Ultimately, the choice of protocol for the logging service is up to you, just know that both will work great when using NetApp platforms and there is no reason to avoid NFS if that is your preference.

   If you choose to use NFS with the logging service, you will need to set the Ansible variable ``openshift_enable_unsupported_configurations`` to ``true`` to prevent the installer from failing.

**Getting started**

The logging service can, optionally, be deployed for both applications as well as for the core operations of the OpenShift cluster itself.  If you choose to deploy operations logging, by specifying the variable ``openshift_logging_use_ops`` as ``true``, two instances of the service will be created.  The variables which control the logging instance for operations contain "ops" in them, whereas the instance for applications does not.

Configuring the Ansible variables according to the deployment method is important in order to ensure that the correct storage is utilized by the underlying services.  Let's look at the options for each of the deployment methods

.. note::
   The tables below only contain the variables which are relevant for storage configuration as it relates to the logging service.  There are many other options found in the `logging documentation <https://docs.openshift.com/container-platform/3.11/install_config/aggregate_logging.html>`_ which should be reviewed, configured, and used according to your deployment.

The variables in the below table will result in the Ansible playbook creating a PV and PVC for the logging service using the details provided.  This method is significantly less flexible than using the component installation playbook after OpenShift installation, however, if you have existing volumes available, it is an option.

.. table:: Logging variables when deploying at OpenShift install time

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

**Deploy the logging stack**

If you are deploying logging as a part of the initial OpenShift install process, then you only need to follow the standard deployment process.  Ansible will configure and deploy the needed services and OpenShift objects so that the service is available as soon as Ansible completes.

However, if you are deploying after the initial installation, the component playbook will need to be used by Ansible. This process may change slightly with different versions of OpenShift, so be sure to read and follow `the documentation <https://docs.openshift.com/container-platform/3.11/welcome/index.html>`_ for your version.

Metrics service
---------------

The metrics service provides valuable information to the administrator regarding the status, resource utilization, and availability of the OpenShift cluster.  It is also necessary for pod autoscale functionality and many organizations use data from the metrics service for their charge back and/or show back applications.

Like with the logging service, and OpenShift as a whole, Ansible is used to deploy the metrics service.  Also, like the logging service, the metrics service can be deployed during an initial setup of the cluster or after it's operational using the component installation method.  The following tables contain the variables which are important when configuring persistent storage for the metrics service.

.. note::
   The tables below only contain the variables which are relevant for storage configuration as it relates to the metrics service.  There are many other options found in the documentation which should be reviewed, configured, and used according to your deployment.

.. table:: Metrics variables when deploying at OpenShift install time

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

The variables above, and the process for deploying, may change with each version of OpenShift.  Ensure you review and follow the `deployment guide <https://docs.openshift.com/container-platform/3.11/install_config/cluster_metrics.html>`_ for your version so that it is configured for your environment.
