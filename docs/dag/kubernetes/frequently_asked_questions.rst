.. _frequently_asked_questions:

**************************
Frequently Asked Questions
**************************

This section of the Design and Architecture Guide is divided into 3 areas and covers frequently asked questions for each:

#. :ref:`Trident for Kubernetes Installation <Trident for Kubernetes Installation>`
#. :ref:`Trident Backend Configuration and Use <Trident Backend Configuration and Use>`
#. :ref:`Trident Upgrade, Support, Licensing, and Troubleshooting <Trident Upgrade, Support, Licensing, and Troubleshooting>`


Trident for Kubernetes Installation
===================================

This section covers Trident Installation on a Kubernetes cluster.


What are the supported versions of etcd?
----------------------------------------

Trident v19.10 does not require an etcd. It uses CRDs to maintain
state.


Does Trident support an offline install from a private registry?
----------------------------------------------------------------

Yes, Trident can be installed offline.

Refer to the :ref:`Offline install <Trident installation modes>` section
for a step by step procedure.

Can Trident be installed remotely?
----------------------------------

Trident v18.10 and above supports :ref:`remote install capability <Trident installation modes>` from any machine that has kubectl access to the cluster. After kubectl access is verified (e.g. initiate a `kubectl get nodes` command from the remote machine to verify), follow the installation instructions.

Refer to :ref:`Deploying <Deploying>` for more information on how to install Trident.


Can we configure High Availability with Trident?
------------------------------------------------

Since Trident is installed as a Kubernetes Deployment (ReplicaSet) with one instance, it has HA built in. Do not increase the number of replicas in the Trident deployment.

If the node where Trident is installed is lost or the pod is otherwise inaccessible, Kubernetes will automatically
re-deploy the pod to a healthy node in your cluster.

Since Trident is control-plane only, currently mounted pods will not be affected if Trident is re-deployed.


Does Trident need access to kube-system namespace?
--------------------------------------------------

Trident reads from the Kubernetes API Server to determine when applications request new PVCs so it needs access to kube-system.


What are the roles and privileges used by Trident?
--------------------------------------------------

The Trident installer creates a Kubernetes ClusterRole which has specific access to the cluster's PersistentVolume,
PersistentVolumeClaim, StorageClass and Secret resources of the Kubernetes cluster.

Refer to :ref:`Customized Installation <Customized Installation>` for more information.


Can we locally generate the exact manifest files Trident uses to install?
-------------------------------------------------------------------------

You can locally generate and modify the exact manifest files Trident uses to install if needed.

Refer to :ref:`Customized Installation <Customized Installation>` for instructions.


Can we share the same ONTAP backend SVM for two separate Trident instances for two separate Kubernetes clusters?
----------------------------------------------------------------------------------------------------------------

Although it is not advised, you can use the same backend SVM for 2 Trident instances. Specify a unique Trident volume name for
each Trident instance during installation and/or specify a unique StoragePrefix parameter in the setup/backend.json file. This is to ensure the same FlexVol isn't used for both instances.

Refer to: :ref:`Customized Installation <Customized Installation>` for information on specifying a unique Trident volume name.
Refer to: :ref:`Global Configuration <Global Configuration>` for information on creating a unique StoragePrefix.


Is it possible to install Trident under ContainerLinux (formerly CoreOS)?
-------------------------------------------------------------------------

Trident is simply a Kubernetes pod and can be installed wherever Kubernetes is running.

Refer to :ref:`Supported host operating systems <Supported host operating systems>` for more information.


Can we use Trident with NetApp Cloud Volumes ONTAP?
---------------------------------------------------

Yes, it is supported on AWS, Google Cloud and Azure.

Refer to :ref:`Supported backends <Supported backends (storage)>` for more information.


Does Trident work with Cloud Volumes Services?
----------------------------------------------

Yes, Trident supports the Azure NetApp Files service in Azure as well as the Cloud Volumes Service in AWS.

Refer to :ref:`Supported backends <Supported backends (storage)>` for more information.

What versions of Kubernetes support Trident as an enhanced CSI Provisioner?
---------------------------------------------------------------------------

Kubernetes versions ``1.13`` and above support running Trident as a CSI Provisioner. Before installing
Trident, ensure the required :ref:`feature gates <Feature Gates>` are enabled.

Refer to :ref:`Requirements <Supported frontends (orchestrators)>` for a list
of supported orchestrators.

Why should I install Trident to work as a CSI Provisioner?
----------------------------------------------------------

With the 19.07 release, Trident now fully adheres to the latest
CSI 1.1 specification and is production ready. This enables users to
experience all features exposed by the current release, as well as future
releases. Trident can continue to fix issues or add features without touching
the Kubernetes core, while also absorbing any standardized future changes or features efficiently.

How do I install Trident to work as a CSI Provisioner?
------------------------------------------------------

The installation procedure is detailed under the :ref:`Deployment <deploying-in-kubernetes>` section.
Ensure that the :ref:`feature gates <Feature Gates>` are enabled.

How does Trident maintain state if it doesn't use etcd?
-------------------------------------------------------

Trident uses :ref:`Custom Resource Definitions(CRDs) <Kubernetes CustomResourceDefinition objects>`
to maintain its state. This eliminates
the requirement for etcd and a Trident volume on the storage cluster. Trident no longer
needs its separate PV; the information is stored as CRD objects that will be present
in the Kubernetes cluster’s etcd.

How do I uninstall Trident?
---------------------------

The :ref:`Uninstalling Trident <Uninstalling Trident>` section explains how
you can remove Trident.

Trident Backend Configuration and Use
=====================================

This section covers Trident backend definition file configurations and use.

Do we need to define both Management and Data LIFs in an ONTAP backend definition file?
---------------------------------------------------------------------------------------

NetApp recommends having both in the backend definition file. However, the Management LIF is the only one that is
absolutely mandatory.

Refer to :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information on backend definition files.


Can we specify a port in the DataLIF?
-------------------------------------

Trident 19.01 and later supports specifying a port in the DataLIF.

Configure it in the backend.json file as
`"managementLIF": <ip address>:<port>"` For example, if the IP address of your management LIF is 192.0.2.1, and the
port is 1000, configure ``"managementLIF": "192.0.2.1:1000"``,


Is it possible to update the Management LIF on the backend ?
------------------------------------------------------------

Yes, it is possible to update the backend Management LIF using the ``tridentctl update backend`` command.

Refer to :ref:`Backend configuration <Backend configuration>` for more information on updating the backend.


Is it possible to update the Data LIF on the backend ?
------------------------------------------------------

No, it is not possible to update the Data LIF on the backend.


Can we create multiple backends in Trident for Kubernetes?
----------------------------------------------------------

Trident can support many backends simultaneously, either with the same driver or different drivers.

Refer to :ref:`Backend configuration <Backend configuration>` for more information on creating backend definition files.


How does Trident store backend credentials?
-------------------------------------------

Trident stores the backend credentials as Kubernetes Secrets.


How does Trident select a specific backend?
-------------------------------------------

If the backend attributes cannot be used to automatically select the right pools for a class, the `storagePools` and
`additionalStoragePools` parameters are used to select a specific set of pools.

Refer to :ref:`Storage Class design for specific backend utilization <Storage Class design for specific backend utilization>` in the Design and Architecture Guide for more information.


Can we make sure Trident will not provision from a specific backend?
--------------------------------------------------------------------

The `excludeStoragePools` parameter is used to filter the set of pools that Trident will use for provisioning and will
remove any pools that match.

Refer to :ref:`Kubernetes StorageClass Objects <Kubernetes StorageClass objects>`


If there are multiple backends of the same kind, how does Trident select which backend to use?
----------------------------------------------------------------------------------------------

If there are multiple backends configured of the same type, then Trident will select the appropriate backend based on
the parameters present in the StorageClass and the PersistentVolumeClaim. For example, if there are multiple
``ontap-nas`` driver backends, then Trident will try to match parameters in the StorageClass and PersistentVolumeClaim
combined and match a backend which can deliver the requirements listed in the StorageClass and
PersistentVolumeClaim. If there are multiple backends that matches the request, then Trident will choose from one of
them at random.


Does Trident support bi-directional CHAP with Element/SolidFire?
----------------------------------------------------------------

Bi-directional CHAP is supported with Element.

Refer to :ref:`CHAP authentication <CHAP authentication>` in the Design and Architecture Guide for additional information.


How does Trident deploy Qtrees on an ONTAP volume? How many Qtrees can be deployed on a single volume through Trident?
----------------------------------------------------------------------------------------------------------------------

The ``ontap-nas-economy`` driver will create up to 200 Qtrees in the same FlexVol, 100,000 Qtrees per cluster node, and
2.4M per cluster. When you enter a new PersistentVolumeClaim that is serviced by the economy driver, the driver looks
to see if a FlexVol already exists that can service the new Qtree. If the FlexVol does not exist that can service the
Qtree, a new FlexVol will be created.

Refer to :ref:`Choosing a driver <Choosing a driver>` for more information.


How can we set Unix permissions for volumes provisioned on ONTAP NAS?
---------------------------------------------------------------------

Unix Permissions can be set on the volume provisioned by Trident by setting a parameter in the backend definition file.

Refer to :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


How can we configure an explicit set of ONTAP NFS mount options while provisioning a volume?
--------------------------------------------------------------------------------------------

By default, Trident does not set mount options to any value with Kubernetes.

To specify the mount options in the Kubernetes Storage Class, please follow the example
given `here <https://github.com/NetApp/trident/blob/master/trident-installer/sample-input/storage-class-ontapnas-k8s1.8-mountoptions.yaml#L6.>`_.


How do I set the provisioned volumes to a specific export policy?
-----------------------------------------------------------------

To allow the appropriate hosts access to a volume, use the `exportPolicy` parameter configured in the backend definition file.

Refer to :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


How do I set volume encryption through Trident with ONTAP?
----------------------------------------------------------

Encryption can be set on the volume provisioned by Trident by using the `encryption` parameter in the backend definition file.

Refer to :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


What is the best way to implement QoS for ONTAP through Trident?
----------------------------------------------------------------

Use StorageClasses to implement QoS for ONTAP.

Refer to :ref:`Storage Class design to emulate QoS policies <Storage Class design to emulate QoS policies>` for more information.


How do we specify thin or thick provisioning through Trident?
-------------------------------------------------------------

The ONTAP drivers support either thin or thick provisioning. E-series only support thick provisioning. Element OS backends only support thin provisioning.

The ONTAP drivers default to thin provisioning. If thick provisioning is desired, you may configure either the backend definition file or the `StorageClass`. If both are configured, the StorageClass takes precedence. Configure the following for ONTAP:

  * On the StorageClass, set the ``provisioningType`` attribute as `thick`.
  * On the backend definition file, enable thick volumes by setting backend ``spaceReserve`` parameter as  `volume`.

Refer to :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


How do I make sure that the volumes being used are not deleted even if I accidentally delete the PVC?
-----------------------------------------------------------------------------------------------------

PVC protection is automatically enabled on Kubernetes starting from version 1.10.

Refer to `Storage Object in Use Protection <https://v1-14.docs.kubernetes.io/docs/tasks/administer-cluster/storage-object-in-use-protection/>`_ for additional information.


Can we use PVC resize functionality with NFS, Trident, and ONTAP?
-----------------------------------------------------------------

PVC resize is supported with Trident. Note that `volume autogrow` is an ONTAP feature that is not applicable to
Trident.

Refer to :ref:`Expanding Volumes <Expanding an NFS volume>` for more information.


If I have a volume that was created outside Trident can I import it into Trident?
---------------------------------------------------------------------------------

Starting in Trident v19.04, you can use the volume import feature to bring volumes in to Kubernetes.

Refer to :ref:`Importing a volume <Importing a volume>` for more information.


Can I import a volume while it is in Snapmirror Data Protection (DP) or offline mode?
-------------------------------------------------------------------------------------

The volume import will fail if the external volume is in DP mode or offline. You will receive an error message.

.. code-block:: console

   Error: could not import volume: volume import failed to get size of volume: volume <name> was not found (400 Bad Request) command terminated with exit code 1.

Make sure to remove the DP mode or put the volume online before importing the volume.

Refer to: :ref:`Behavior of Drivers for Volume Import <Behavior of Drivers for Volume Import>` for additional information.


Can we use PVC resize functionality with iSCSI, Trident, and ONTAP?
-------------------------------------------------------------------

PVC resize functionality with iSCSI is not supported with Trident.


How is resource quota translated to a NetApp cluster?
-----------------------------------------------------

Kubernetes Storage Resource Quota should work as long as NetApp Storage has capacity. When the NetApp storage cannot
honor the Kubernetes quota settings due to lack of capacity, Trident will try to provision but will error out.

Can you create Volume Snapshots using Trident?
----------------------------------------------

The creation of Kubernetes Volume Snapshots is possible with the ``20.01``
release and above. This requires Kubernetes ``1.17`` and above.

Refer to the 20.01 Trident documentation: `On-Demand Volume Snapshots <https://netapp-trident.readthedocs.io/en/stable-v20.01/kubernetes/operations/tasks/volumes.html#on-demand-volume-snapshots>`_
for more information.

How do we take a snapshot backup of a volume provisioned by Trident with ONTAP?
-------------------------------------------------------------------------------
This is available on ``ontap-nas``, ``ontap-san``, and ``ontap-nas-flexgroup`` drivers.

This is also available on the ``ontap-nas-economy`` drivers but on the FlexVol level granularity and not on the qtree level granularity.

To enable the ability to snapshot volumes provisioned by Trident, set the backend parameter option `snapshotPolicy`
to the desired snapshot policy as defined on the ONTAP backend. Any snapshots taken by the storage controller will not be known by Trident.


Can we set a snapshot reserve percentage for a volume provisioned through Trident?
----------------------------------------------------------------------------------

Yes, we can reserve a specific percentage of disk space for storing the snapshot copies through Trident by setting the
`snapshotReserve` attribute in the backend definition file. If you have configured the snapshotPolicy and the
snapshotReserve option in the backend definition file, then snapshot reserve percentage will be set according to the
snapshotReserve percentage mentioned in the backend file. If the snapshotReserve percentage number is not mentioned,
then ONTAP by default will take the snapshot reserve percentage as 5. In the case where the snapshotPolicy option is
set to none, then the snapshot reserve percentage is set to 0.

Refer to: :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


Can we directly access the volume snapshot directory and copy files?
--------------------------------------------------------------------

Yes, It is possible to access the snapshot directory on the volume provisioned by Trident by setting the `snapshotDir`
parameter in the backend definition file.

Refer to: :ref:`ONTAP (AFF/FAS/Select/Cloud)` for more information.


Can we set up SnapMirror for Trident volumes through Trident?
-------------------------------------------------------------

Currently, SnapMirror has be set externally using ONTAP CLI or OnCommand System Manager.


How do I restore Persistent Volumes to a specific ONTAP snapshot?
-----------------------------------------------------------------

To restore a volume to an ONTAP snapshot, follow the following steps:

  * Quiesce the application pod which is using the Persistent volume .
  * Revert to the required snapshot through ONTAP CLI or OnCommand System Manager.
  * Restart the application pod.


How can I obtain complete Trident configuration details?
--------------------------------------------------------

``tridentctl get`` command provides more information on the Trident Configuration.

Refer to :ref:`tridentctl get <get>` for more information on this command.


How can we separate out storage class usage for each customer/tenant?
---------------------------------------------------------------------

Kubernetes does not allow storage classes in namespaces. However, we can use Kubernetes to limit usage of a specific
storage class per namespace by using
`Storage Resource Quotas <https://kubernetes.io/docs/concepts/policy/resource-quotas/#storage-resource-quota>`_  which
are per namespace. To deny a specific namespace access to specific storage, set the resource quota to 0 for that storage class.


Does Trident provide insight into the capacity of the storage?
--------------------------------------------------------------

This is out of scope for Trident. NetApp offers `Cloud Insights <https://cloud.netapp.com/cloud-insights>`_ for
monitoring and analysis.


Does the user experience change when using Trident as a CSI Provisioner?
------------------------------------------------------------------------

No. From the user's point of view, there are no changes as far as the user experience
and functionalities are concerned. The provisioner name used will be ``csi.trident.netapp.io``.
This method of installing Trident is recommended to use all new features provided by current
and future releases.

How do I design a Disaster Workflow for Trident v19.10?
-------------------------------------------------------

The :ref:`Data replication using ONTAP <Data replication using ONTAP>` section
talks about backup and DR workflows using ONTAP.

Trident Upgrade, Support, Licensing, and Troubleshooting
========================================================

This section covers upgrading Trident, Trident Support, Licensing and Troubleshooting.


How frequently is Trident released?
-----------------------------------

Trident is released every 3 months: January, April, July and October. This is one month after a Kubernetes release.


Does NetApp support Trident?
----------------------------

Although Trident is open source and provided for free, NetApp fully supports Trident provided your NetApp backend
is supported.


How do I raise a support case for Trident?
------------------------------------------

To raise a support case, you could do the following

  *  Customers can reach their Support Account Manager and get help to raise a ticket.
  *  Raise a support case by contacting support at `mysupport.netapp.com <https://mysupport.netapp.com>`_.


How do I generate a support log bundle using Trident?
-----------------------------------------------------

You can create a support bundle by running ``tridentctl logs -a``. In addition to the logs captured in the bundle, capture the kubelet log to diagnose the mount problems on the k8s side. The instructions to get the kubelet log varies based on how k8s is installed.

Refer to: :ref:`Troubleshooting <Troubleshooting>`.


Does Trident support all the features that are released in a particular version of Kubernetes?
----------------------------------------------------------------------------------------------

Trident usually doesn’t support alpha features in Kubernetes. We may support beta features within the following two
Trident releases after the Kubernetes beta release.


What do I do if I need to raise a request for a new feature on Trident?
-----------------------------------------------------------------------

If you would like to raise a request for a new feature, raise an issue at NetApp/Trident
`Github <https://github.com/NetApp/trident>`_  and make sure to mention “RFE” in the subject and description of the issue.


Where do I raise a defect for Trident?
--------------------------------------

If you would like to raise a defect against Trident, raise an issue at NetApp/Trident `Github <https://github.com/NetApp/trident>`_. Make sure to include all the necessary information and logs pertaining to the issue.


What happens if I have quick question on Trident that I need clarification on? Is there a community or a forum for Trident?
---------------------------------------------------------------------------------------------------------------------------

If you have any questions, issues, or requests please reach out to us through our `Slack <https://netapp.io/slack>`_ team
or `GitHub <https://github.com/NetApp/trident>`_.


Does Trident have any dependencies on other NetApp products for its functioning?
--------------------------------------------------------------------------------

Trident doesn’t have any dependencies on other NetApp software products and it works as a standalone application. However,
you must have a NetApp backend storage device.


Can I upgrade from a older version of Trident directly to a newer version (skipping a few versions)?
----------------------------------------------------------------------------------------------------

We support upgrading directly from a version up to one year back. For example, if you are currently on v18.04, v18.07,
or v19.01, we will support directly upgrading to v19.04. We suggest testing upgrading in a lab prior to production deployment.
Information on upgrading Trident can be obtained :ref:`here <Upgrading Trident>`.


How can I upgrade to the most recent version of Trident?
--------------------------------------------------------

Refer to :ref:`Upgrading Trident <Upgrading Trident>` for the steps involved in
Upgrading Trident to the latest release.


Is it possible to downgrade Trident to a previous release?
----------------------------------------------------------

**Downgrading Trident is not recommended** for the :ref:`following reasons <Downgrading Trident>`.

If the Trident pod is destroyed, will we lose the data?
-------------------------------------------------------

No data will be lost if the Trident pod is destroyed. Trident's metadata will be stored in CRD objects.
All PVs that have been provisioned by Trident will function normally.

My storage system's password has changed and Trident no longer works, how do I recover?
---------------------------------------------------------------------------------------

Update the backend's password with a ``tridentctl update backend myBackend -f </path/to_new_backend.json> -n trident``.
Replace `myBackend` in the example with your backend name, and `/path/to_new_backend.json` with the path to the correct
backend.json file.
