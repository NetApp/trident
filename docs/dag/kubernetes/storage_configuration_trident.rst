.. _storage_configuration_trident:

*********************************
Storage Configuration for Trident
*********************************

Each storage platform in NetApp's portfolio has unique capabilities that benefit applications, containerized or not. Trident works with each of the major platforms: ONTAP, Element, and E-Series.  There is not one platform which is better suited for all applications and scenarios than another, however the needs of the application and the team administering the device should be taken into account when choosing a platform.

The storage administrator(s), Kubernetes administrator(s), and the application team(s) should work with their NetApp team to ensure that the most appropriate platform is selected.

Regardless of which NetApp storage platform you will be connecting to your Kubernetes environment, you should follow the baseline best practices for the host operating system with the protocol that you will be leveraging. Optionally, you may want to consider incorporating application best practices, when available, with backend, storage class, and PVC settings to optimize storage for specific applications.

Some of the best practices guides are listed below, however refer to the `NetApp Library <https://www.netapp.com/us/search/index.aspx?i=1&q1=Documents&x1=t1>`_ for the most current versions.

* ONTAP

  * `NFS Best Practice and Implementation Guide <https://www.netapp.com/us/media/tr-4067.pdf>`_
  * `SAN Administration Guide <http://docs.netapp.com/ontap-9/index.jsp?topic=%2Fcom.netapp.doc.dot-cm-sanag%2Fhome.html>`_ (for iSCSI)
  * `iSCSI Express Configuration for RHEL <http://docs.netapp.com/ontap-9/index.jsp?topic=%2Fcom.netapp.doc.exp-iscsi-rhel-cg%2Fhome.html>`_
  
* SolidFire

  * `Configuring SolidFire for Linux <http://www.netapp.com/us/media/tr-4639.pdf>`_

* E-Series

  * `Installing and Configuring for Linux Express Guide <https://library.netapp.com/ecm/ecm_download_file/ECMLP2601376>`_

Some example application best practices guides:

* `ONTAP Best Practice Guidelines for MySQL <https://www.netapp.com/us/media/tr-4722.pdf>`_
* `MySQL Best Practices for NetApp SolidFire <http://www.netapp.com/us/media/tr-4605.pdf>`_
* `NetApp SolidFire and Cassandra <http://www.netapp.com/us/media/tr-4635.pdf>`_
* `Oracle Best Practices on NetApp SolidFire <http://www.netapp.com/us/media/tr-4606.pdf>`_
* `PostgreSQL Best Practices on NetApp SolidFire <http://www.netapp.com/us/media/tr-4610.pdf>`_

Not all applications will have specific guidelines, it's important to work with your NetApp team and to refer to the `library <https://www.netapp.com/us/search/index.aspx?i=1&q1=Documents&x1=t1>`_ for the most up-to-date recommendations and guides.

Best practices for configuring ONTAP
====================================

The following recommendations are guidelines for configuring ONTAP for containerized workloads which consume volumes dynamically provisioned by Trident.  Each should be considered and evaluated for appropriateness in your environment.

Use SVM(s) which are dedicated to Trident
-----------------------------------------

Storage Virtual Machines (SVMs) provide isolation and administrative separation between tenants on an ONTAP system.  Dedicating an SVM to applications enables the delegation of privileges and enables applying best practices for limiting resource consumption.

There are several options available for the management of the SVM:

* Provide the cluster management interface in the backend configuration, along with appropriate credentials, and specify the SVM name
* Create a dedicated management interface for the SVM
* Share the management role with an NFS data interface 

In each case, the interface should be in DNS, and the DNS name should be used when configuring Trident.  This helps to facilicate some DR scenarios, for example, SVM-DR without the use of network identity retention.

There is no preference between having a dedicated or shared management LIF for the SVM, however you should ensure that your network security policies align with the approach you choose.  Regardless, the management LIF should be accessible via DNS to facilitate maximum flexibility should `SVM-DR <https://library.netapp.com/ecm/ecm_download_file/ECMLP2496252>`_ be used in conjunction with Trident.

Limit the maximum volume count
------------------------------

ONTAP storage systems have a maximum volume count which varies based on the software version and hardware platform, see the `Hardware Universe <https://hwu.netapp.com/>`_ for your specific platform and ONTAP version to determine exact limits.  When the volume count is exhausted, provisioning operations will fail not only for Trident, but for all storage requests.

Trident's ``ontap-nas`` and ``ontap-san`` drivers provision a FlexVolume for each Kubernetes persistent volume (PV) which is created, and the ``ontap-nas-economy`` driver will create approximately one FlexVolume for every 200 PVs.  To prevent Trident from consuming all available volumes on the storage system, it is recommended that a limit be placed on the SVM.  This can be done from the command line:

.. code-block:: console

   vserver modify -vserver <svm_name> -max-volumes <num_of_volumes>

The value for ``max-volumes`` will vary based on several criteria specific to your environment:

* The number of existing volumes in the ONTAP cluster
* The number of volumes you expect to provision outside of Trident for other applications
* The number of persistent volumes expected to be consumed by Kubernetes applications

The ``max-volumes`` value is total volumes provisioned across all nodes in the cluster, not to an individual node.  As a result, some conditions may be encountered where a cluster node may have far more, or less, Trident provisioned volumes than another.

For example, a 2-node ONTAP cluster has the ability to host a maximum of 2000 FlexVolumes.  Having the maximum volume count set to 1250 appears very reasonable.  However, if only aggregates from one node are assigned to the SVM, or the aggregates assigned from one node are unable to be provisioned against (e.g. due to capacity), then the other node will be the target for all Trident provisioned volumes.  This means that the volume limit may be reached for that node before the ``max-volumes`` value is reached, resulting in impacting both Trident and other volume operations using that node.  Avoid this situation by ensuring that aggregates from each node in the cluster are assigned to the SVM used by Trident in equal numbers.

In addition to controlling the volume count at the storage array, leveraging Kubernetes capabilities should also be used as explained in the next chapter.

Limit the maximum size of volumes created by the Trident user
-------------------------------------------------------------

ONTAP can prevent a user from creating a volume above a maximum size, as defined by the administrator.  This is implemented using the permissions system and should be applied to the user which Trident uses to authenticate, e.g. ``vsadmin``.

.. code-block:: console

   security login role modify -vserver <svm_name> -role <username> -access all -cmddirname "volume create" -query "-size <=50g"

The above example command will prevent the user from creating volume larger than 50GiB in size.  The value should be modified to what is appropriate for your applications and the expected size of volumes desired.

.. note::
   This does not apply when using the ``ontap-nas-economy`` driver.  The economy driver will create the FlexVolume with a size equal to the first PVC provisioned to that FlexVolume.  Subsequent PVCs provisioned to that FlexVolume will result in the volume being resized, which is not subject to the limitation described above.

In addition to controlling the volume size at the storage array, leveraging Kubernetes capabilities should also be used as explained in the next chapter.

Create and use an SVM QoS policy
--------------------------------

Leveraging an ONTAP QoS policy, applied to the SVM, limits the number of IOPS consumable by the Trident provisioned volumes.  This helps to `prevent a bully <http://docs.netapp.com/ontap-9/topic/com.netapp.doc.pow-perf-mon/GUID-77DF9BAF-4ED7-43F6-AECE-95DFB0680D2F.html?cp=7_1_2_1_2>`_ or out-of-control container from affecting workloads outside of the Trident SVM.

Creating a QoS policy for the SVM can be done in a few steps.  Refer to the documentation for your version of ONTAP for the most accurate information.  The example below creates a QoS policy which limits the total IOPS available to the SVM to 5000.

.. code-block:: console

   # create the policy group for the SVM
   qos policy-group create -policy-group <policy_name> -vserver <svm_name> -max-throughput 5000iops
   
   # assign the policy group to the SVM, note this will not work 
   # if volumes or files in the SVM have existing QoS policies
   vserver modify -vserver <svm_name> -qos-policy-group <policy_name>

Additionally, if your version of ONTAP supports it, you may consider using a QoS minimum in order to guarantee an amount of throughput to containerized workloads.  Adaptive QoS is not compatible with an SVM level policy.

The number of IOPS dedicated to the containerized workloads depends on many aspects.  Among other things, these include:

* Other workloads using the storage array.  If there are other workloads, not related to the Kubernetes deployment, utilizing the storage resources, then care should be taken to ensure that those workloads are not accidentally adversely impacted.
* Expected workloads running in containers.  If workloads which have high IOPS requirements will be running in containers, then a low QoS policy will result in a bad experience.

It's important to remember that a QoS policy assigned at the SVM level will result in all volumes provisioned to the SVM sharing the same IOPS pool.  If one, or a small number, of the containerized applications has a high IOPS requirement it could become a bully to the other containerized workloads.  If this is the case, you may want to consider using external automation to assign per-volume QoS policies.

Limit storage resource access to Kubernetes cluster members
-----------------------------------------------------------

Limiting access to the NFS volumes and iSCSI LUNs created by Trident is a critical component of the security posture for your Kubernetes deployment.  Doing so prevents hosts which are not a part of the Kubernetes cluster from accessing the volumes and potentially modifying data unexpectedly.

It's important to understand that namespaces are the logical boundary for resources in Kubernetes.  The assumption is that resources in the same namespace are able to be shared, however, importantly, there is no cross-namespace capability.  This means that even though PVs are global objects, when bound to a PVC they are only accessible by pods which are in that same namespace.  It's critical to ensure that namespaces are used to provide separation when appropriate.

The primary concern for most organizations, with regard to data security in a Kubernetes context, is that a process in a container can access storage mounted to the host, but which is not intended for the container.  Simply put, this is not possible.  The underlying technology for containers, `namespaces <https://en.wikipedia.org/wiki/Linux_namespaces>`_, are designed to prevent this type of compromise.  However, there is one exception: privileged containers. 

A privileged container is one that is run with substantially more host-level permissions than normal.  These are not denied by default, so disabling the capability using `pod security policies <https://kubernetes.io/docs/concepts/policy/pod-security-policy/>`_ is very important for preventing this accidental exposure.

For volumes where access is desired from both Kubernetes and external hosts, the storage should be managed in a traditional manner, with the PV introduced by the administrator and not managed by Trident.  This ensures that the storage volume is destroyed only when both the Kubernetes and external hosts have disconnected and are no longer using the volume.  Additionally, a custom export policy can be applied which enables access from the Kubernetes cluster nodes and targeted servers outside of the Kubernetes cluster.

For deployments which have dedicated infrastructure nodes (e.g. OpenShift), or other nodes which are not schedulable for user applications, separate export policies should be used to further limit access to storage resources.  This includes creating an export policy for services which are deployed to those infrastructure nodes, for example the OpenShift Metrics and Logging services, and standard applications which are deployed to non-infrastructure nodes.

Create export policy 
--------------------

Create appropriate export policies for the Storage Virtual Machines. Allow only Kubernetes nodes access to the NFS volumes.

Export policies contain one or more export rules that process each node access request. Use the ``vserver export-policy create`` ONTAP CLI to create the export policy. Add rules to the export policy using the ``vserver export-policy rule create`` ONTAP CLI command. Performing the above commands enables you to restrict which nodes have access to data.

Disable ``showmount`` for the application SVM
---------------------------------------------

The showmount feature enables an NFS client to query the SVM for a list of available NFS exports.  A pod deployed to the Kubernetes cluster could issue the showmount -e command against the data LIF and receive a list of available mounts, including those which it does not have access to.  While this isn't, by itself, dangerous or a security compromise, it does provide unnecessary information potentially aiding an unauthorized user with connecting to an NFS export.

Disabling showmount is an SVM level command:

.. code-block:: console

   vserver nfs modify -vserver <svm_name> -showmount disabled

Use NFSv4 for Trident's etcd when possible
------------------------------------------

NFSv3 locks are handled by Network Lock Manager (NLM), which is a sideband mechanism not using the NFS protocol. Therefore, during a failure scenario and a server hosting the Trident pod ungracefully leaves the network (either by a hard reboot or all access being abruptly severed), the NFS lock is held indefinitely. This results in Trident failure because etcd's volume cannot be mounted from another node.  

NFSv4 has session management and locking built into the protocol and the locks are released automatically when the session times out. In a recovery situation, the trident pod will be redeployed on another node, mount, and come back up after the v4 locks are automatically released.


Best practices for configuring SolidFire
=========================================

**Solidfire Account**

Create a SolidFire account. Each SolidFire account represents a unique volume owner and receives its own set of Challenge-Handshake Authentication Protocol (CHAP) credentials. You can access volumes assigned to an account either by using the account name and the relative CHAP credentials or through a volume access group. An account can have up to two-thousand volumes assigned to it, but a volume can belong to only one account.

**SolidFire QoS**

Use QoS policy if you would like to create and save a standardized quality of service setting that can be applied to many volumes.

Quality of Service parameters can be set on a per-volume basis. Performance for each volume can be assured by setting three configurable parameters that define the QoS: Min IOPS, Max IOPS, and Burst IOPS.

The following table shows the possible minimum, maximum, and Burst IOPS values for 4Kb block Size.

 +-------------------+----------------------------------------------------+-----------+---------------+----------------+
 |   IOPS Parameter  |                        Definition                  | Min value | Default Value | Max Value(4Kb) |
 +===================+====================================================+===========+===============+================+
 |     Min IOPS      |   The guaranteed level of performance for a volume.| 50        |       50      |   15000        |
 +-------------------+----------------------------------------------------+-----------+---------------+----------------+
 |     Max IOPS      |   Performance will not exceed this limit.          | 50        |     15000     |   200,000      |
 +-------------------+----------------------------------------------------+-----------+---------------+----------------+
 |     Burst IOPS    |   Maximum IOPS allowed in a short burst scenario.  | 50        |     15000     |   200,000      |
 +-------------------+----------------------------------------------------+-----------+---------------+----------------+
  
Note: Although the Max IOPS and Burst IOPS can be set as high as 200,000, real-world maximum performance of a volume is limited by cluster usage and per-node performance.
  
Block size and bandwidth have a direct influence on the number of IOPS. As block sizes increase, the system increases bandwidth to a level necessary to process the larger block sizes. As bandwidth increases the number of IOPS the system is able to attain decreases. For more information on QoS and performance, refer to the `NetApp SolidFire Quality of Service (QoS) <https://www.netapp.com/us/media/tr-4644.pdf>`_ Guide.
 
   
**SolidFire authentication**

Element supports two methods for authentication: CHAP and Volume Access Groups (VAG). CHAP uses the CHAP protocol to authenticate the host to the backend. Volume Access groups controls access to the volumes it provisions. NetApp recommends using CHAP for authentication as it's simplier and has no scaling limits.

CHAP authentication (verification that the initiator is the intended volume user) is supported only with account-based access control. If you are using CHAP for authentication, 2 options are available: unidirectional CHAP and bidirectional CHAP. Unidirectional CHAP authenticates volume access by using the SolidFire account name and initiator secret. The bidirectional CHAP option provides the most secure way of authenticating the volume since the volume authenticates the host through the account name and the initiator secret, and then the host authenticates the volume through the account name and the target secret.

However, if CHAP is unable to be enabled and VAGs are required, create the access group and add the host initiators and volumes to the access group. Each IQN that you add to an access group can access each volume in the group with or without CHAP authentication. If the iSCSI initiator is configured to use CHAP authentication, account-based access control is used. If the iSCSI initiator is not configured to use CHAP authentication, then volume access group access control is used.

For more information on how to setup Volume Access Groups and CHAP authentication, please refer the NetApp HCI Installation and setup guide.
