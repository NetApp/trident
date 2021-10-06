Using Trident with Amazon FSx for NetApp ONTAP
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

`Amazon FSx for NetApp ONTAP <https://docs.aws.amazon.com/fsx/latest/ONTAPGuide/what-is-fsx-ontap.html>`_, is a fully managed AWS service that enables customers to launch and run file systems powered by NetApp’s ONTAP storage operating system. Amazon FSx for NetApp ONTAP enables customers to leverage NetApp features, performance, and administrative capabilities they’re familiar with, while taking advantage of the simplicity, agility, security, and scalability of storing data on AWS. FSx supports many of ONTAP’s file system features and administration APIs.

A file system is the primary resource in Amazon FSx, analogous to an ONTAP cluster on premises. Within each SVM you can create one or multiple volumes, which are data containers that store the files and folders in your file system. With Amazon FSx for NetApp ONTAP, Data ONTAP will be provided as a managed file system in the cloud. The new file system type is called **NetApp ONTAP**.

By using `Trident <https://netapp.io/persistent-storage-provisioner-for-kubernetes/>`_ with Amazon FSx for NetApp ONTAP, customers can ensure that their Kubernetes clusters running in Amazon Elastic Kubernetes Service (EKS) can provision Block and File persistent volumes backed by ONTAP.

Learn about Trident
-------------------

If you are new to Trident, familiarize yourself by using the links provided below:

* `What is Trident? <https://netapp-trident.readthedocs.io/en/latest/introduction.html>`_
* `Requirements for using Trident <https://netapp-trident.readthedocs.io/en/latest/support/requirements.html>`_
* `Deploying Trident <https://netapp-trident.readthedocs.io/en/latest/kubernetes/deploying/index.html>`_
* `Perform post-deployment tasks <https://netapp-trident.readthedocs.io/en/latest/kubernetes/deploying/operator-deploy.html#post-deployment-steps>`_
* `Best practices for configuring ONTAP, Cloud Volumes ONTAP, and Amazon FSx <https://netapp-trident.readthedocs.io/en/latest/dag/kubernetes/storage_configuration_trident.html#best-practices-for-configuring-ontap-cloud-volumes-ontap-and-amazon-fsx-for-ontap>`_
* `Integrating Trident <https://netapp-trident.readthedocs.io/en/latest/dag/kubernetes/integrating_trident.html>`_
* `ONTAP backend configuration <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/backends/ontap/index.html>`_

You can integrate Trident with Amazon FSx for NetApp ONTAP by using the following drivers:

* ``ontap-san``: Each PV provisioned is a LUN within its own Amazon FSx for NetApp ONTAP volume.
* ``ontap-san-economy``: Each PV provisioned is a LUN with a configurable number of LUNs per Amazon FSx for NetApp ONTAP volume.
* ``ontap-nas``: Each PV provisioned is a full Amazon FSx for NetApp ONTAP volume.
* ``ontap-nas-economy``: Each PV provisioned is a qtree, with a configurable number of qtrees per Amazon FSx for NetApp ONTAP volume.
* ``ontap-nas-flexgroup``: Each PV provisioned is a full Amazon FSx for NetApp ONTAP FlexGroup volume.

Learn more about driver capabilities `here <https://netapp-trident.readthedocs.io/en/latest/dag/kubernetes/integrating_trident.html>`_.

Amazon FSx for ONTAP uses `FabricPool <https://docs.netapp.com/ontap-9/topic/com.netapp.doc.dot-mgng-stor-tier-fp/GUID-5A78F93F-7539-4840-AB0B-4A6E3252CF84.html>`_ to manage storage tiers. It enables you to store data in a tier, based on whether the data is frequently accessed.

Trident expects to be run as either an ONTAP or SVM administrator, using the cluster ``fsxadmin`` user or a ``vsadmin`` SVM user, or a user with a different name that has the same role. The ``fsxadmin`` user is a limited replacement for the ``admin`` cluster user. Trident typically uses the ``admin`` cluster user for non-Amazon FSx for ONTAP deployments.

Trident offers two modes of authentication:

1.	Credential-based: You can use the ``fsxadmin`` user for your file system or the ``vsadmin`` user configured for your SVM. We recommend using the ``vsadmin`` user to configure your backend. Trident will communicate with the FSx file system using this username and password.
2.	Certificate-based: Trident will communicate with the SVM on your FSx file system using a certificate installed on your SVM.

Deployment and Configuration of Trident on EKS with Amazon FSx for NetApp ONTAP
-------------------------------------------------------------------------------

Prerequisites
~~~~~~~~~~~~~

1. An existing `Amazon EKS cluster <https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html>`_ or self-managed Kubernetes cluster with ``kubectl`` installed.
2. An existing Amazon FSx for NetApp ONTAP file system and storage virtual machine (SVM) that is reachable from your cluster's worker nodes.
3. Worker nodes that are prepared for `NFS <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/worker.html#nfs>`_ and/or `iSCSI <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/worker.html#iscsi>`_

Deployment
~~~~~~~~~~

Deploy Trident using one of the `deployment methods <https://netapp-trident.readthedocs.io/en/latest/kubernetes/deploying/index.html>`_.

Configuration
~~~~~~~~~~~~~

1. Collect your SVM's management LIF DNS name. For example, with the AWS CLI, find the "DNSName" entry under "Endpoints" -> "Management" after running the following command:

  .. code-block:: bash

    aws fsx describe-storage-virtual-machines --region <file system region>

2. Create and install certificates for authentication:

  * `Using ontap-san driver <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/backends/ontap/ontap-san/preparing.html#certificated-based-authentication>`_
  * `Using ontap-nas driver <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/backends/ontap/ontap-nas/preparing.html#certificated-based-authentication>`_

  .. note::

    You can log in to your file system (for example to install certificates) using SSH from anywhere that can reach your file system. Use the `fsxadmin` user, the password you configured when you created your file system, and the management DNS name from ``aws fsx describe-file-systems``.

3. Create a backend file using your certificates and the DNS name of your management LIF, as shown in the sample below. For information about creating backends, `see here <https://netapp-trident.readthedocs.io/en/latest/kubernetes/operations/tasks/managing-backends/index.html>`_.

.. code-block:: json

  {
    "version": 1,
    "storageDriverName": "ontap-san",
    "backendName": "customBackendName",
    "managementLIF": "svm-XXXXXXXXXXXXXXXXX.fs-XXXXXXXXXXXXXXXXX.fsx.us-east-2.aws.internal",
    "svm": "svm01",
    "clientCertificate": "ZXR0ZXJwYXB...ICMgJ3BhcGVyc2",
    "clientPrivateKey": "vciwKIyAgZG...0cnksIGRlc2NyaX",
    "trustedCACertificate": "zcyBbaG...b3Igb3duIGNsYXNz",
   }

.. note::

  Ensure that you follow the `node preparation steps <https://netapp-trident.readthedocs.io/en/stable-v21.07/kubernetes/operations/tasks/worker.html>`_ required for Amazon Linux and Ubuntu `Amazon Machine Images (AMIs) <https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/AMIs.html>`_ depending on your EKS AMI type.

.. note::

  We recommend not setting the ``dataLIF`` for the ``ontap-san`` and ``ontap-san-economy`` drivers to allow Trident to use multipath.

.. warning::

  When using Amazon FSx for NetApp ONTAP with Trident, the ``limitAggregateUsage`` parameter will not work with the ``vsadmin`` and ``fsxadmin`` user accounts. The configuration operation will fail if you specify this parameter.

After deployment, perform the steps to `create a storage class, provision a volume, and mount the volume in a pod <https://netapp-trident.readthedocs.io/en/latest/kubernetes/deploying/operator-deploy.html#post-deployment-steps>`_.
