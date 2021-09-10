.. _netapp_products_integrations:

************************************************
NetApp Products and Integrations with Kubernetes
************************************************

The NetApp portfolio of storage products integrates with many different aspects of a Kubernetes cluster, providing advanced data management capabilities which enhance the functionality, capability, performance, and availability of the Kubernetes deployment.


Trident
-------

NetApp Trident is a dynamic storage provisioner for the containers ecosystem. It provides the ability to create storage volumes for containerized applications managed by Docker and Kubernetes. Trident is a fully supported, open source project hosted on `GitHub <https://github.com/netapp/trident>`_.
Trident works with the portfolio of NetApp storage platforms to deliver storage on-demand to applications according to policies defined by the administrator. When used with Kubernetes, Trident is deployed using native paradigms and provides persistent storage to all namespaces in the cluster.
For more information about Trident, visit `ThePub <https://netapp.io/persistent-storage-provisioner-for-kubernetes/>`_.


ONTAP
-----

ONTAP is NetApp’s multiprotocol, unified storage operating system that provides advanced data management capabilities for any application. ONTAP systems may have all-flash, hybrid, or all-HDD configurations and offer many different deployment models, including engineered hardware (FAS and AFF), white-box (ONTAP Select), cloud-only (Cloud Volumes ONTAP) and running as a file system in the cloud (Amazon FSx for ONTAP). Trident supports all the above mentioned ONTAP deployment models.

Cloud Volumes ONTAP
===================

`Cloud Volumes ONTAP <http://cloud.netapp.com/ontap-cloud?utm_source=GitHub&utm_campaign=Trident>`_ is a software-only storage appliance that runs the ONTAP data management software in the cloud. You can use Cloud Volumes ONTAP for production workloads, disaster recovery, DevOps, file shares, and database management.It extends enterprise storage to the cloud by offering storage efficiencies, high availability, data replication, data tiering and application consistency.

Amazon FSx for ONTAP
====================

`Amazon FSx for ONTAP <https://www.docs.aws.amazon.com/fsx/latest/ONTAPGuide/what-is-fsx-ontap.html>`_ is a fully managed AWS service that enables customers to launch and run file systems powered by NetApp’s ONTAP storage operating system. FSx for ONTAP enables customers to leverage NetApp features, performance, and administrative capabilities they’re familiar with, while taking advantage of the simplicity, agility, security, and scalability of storing data on AWS. FSx for ONTAP supports many of ONTAP’s file system features and administration APIs.

Element Software
----------------

Element Software enables the storage administrator to consolidate workloads by guaranteeing performance and enabling a simplified and streamlined storage footprint. Coupled with an API to enable automation of all aspects of storage management, Element enables storage administrators to do more with less effort.

More information can be found `here <https://www.netapp.com/data-management/element-software/>`_.

NetApp HCI
==========

NetApp HCI simplifies the management and scale of the datacenter by automating routine tasks and enabling infrastructure administrators to focus on more important functions.

NetApp HCI is fully supported by Trident, it can provision and manage storage devices for containerized applications directly against the underlying HCI storage platform. For more information about NetApp HCI visit `NetApp HCI <https://www.netapp.com/us/products/converged-systems/hyper-converged-infrastructure.aspx>`_.

Azure NetApp Files
------------------

`Azure NetApp Files`_ is an enterprise-grade Azure file share service, powered by NetApp. Run your most demanding
file-based workloads in Azure natively, with the performance and rich data management you expect from NetApp.

.. _Azure NetApp Files: https://azure.microsoft.com/en-us/services/netapp/

Cloud Volumes Service for AWS
-----------------------------

`NetApp Cloud Volumes Service for AWS <https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=GitHub&utm_campaign=Trident>`_ is a cloud native file service that provides NAS volumes over NFS and SMB with all-flash performance. This service enables any workload, including legacy applications, to run in the AWS cloud. It provides a fully managed service which offers consistent high performance, instant cloning, data protection and secure access to Elastic Container Service (ECS) instances.

Cloud Volumes Service for GCP
-----------------------------

`NetApp Cloud Volumes Service for CGP <https://cloud.netapp.com/cloud-volumes-service-for-gcp?utm_source=GitHub&utm_campaign=Trident>`_ is a cloud native file service that provides NAS volumes over NFS and SMB with all-flash performance. This service enables any workload, including legacy applications, to run in the GCP cloud. It provides a fully managed service which offers consistent high performance, instant cloning, data protection and secure access to Google Compute Engine (GCE) instances.
