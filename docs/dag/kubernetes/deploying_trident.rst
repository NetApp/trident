.. _deploying_trident:

*****************
Deploying Trident
*****************

The guidelines in this section provide recommendations for Trident installation with various Kubernetes configurations and considerations. As with all the other recommendations in this guide, each of these suggestions should be carefully considered to determine if it's appropriate and will provide benefit to your deployment.

Supported Kubernetes cluster architectures
==========================================

Trident is supported with the following Kubernetes architectures.

   +-----------------------------------------------+-----------+---------------------+
   |         Kubernetes Cluster Architectures      | Supported | Default Install     |
   +===============================================+===========+=====================+
   | Single master, compute                        | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+
   | Multiple master, compute                      | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+
   | Master, etcd, compute                         | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+
   | Master, infrastructure, compute               | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+

Trident installation modes
==========================

Three ways to install Trident are discussed in this chapter.

**Normal install mode**

Normal installation involves running the ``tridentctl install -n trident`` command which deploys the Trident pod on the Kubernetes cluster. Trident installation is quite a straightforward process. For more information on installation and provisioning of volumes, refer to the :ref:`Deploying documentation <Deploying>`.

**Offline install mode**

In many organizations, production and development environments do not have access to public repositories for pulling and posting images as these environments are completely secured and restricted. Such environments only allow pulling images from trusted private repositories.
In such scenarios, make sure that a private registry instance is available. The trident image should be downloaded from a bastion host with internet access and pushed on to the private registry. To install Trident in offline mode, just issue the ``tridentctl install -n trident`` command with the ``--trident-image`` parameter set to the appropriate image name and location.

**Remote install mode**

Trident can be installed on a Kubernetes cluster from a remote machine. To do a remote install, install the appropriate version of ``kubectl`` on the remote machine from where you would be running the ``tridentctl install`` command. Copy the configuration files from the Kubernetes cluster and set the KUBECONFIG environment variable on the remote machine. Initiate a ``kubectl get nodes`` command to verify you can connect to the required Kubernetes cluster. Complete the Trident deployment from the remote machine using the normal installation steps.

Trident Installation on Docker UCP 3.1
======================================

Docker EE Universal Control Plane (UCP) is the cluster management layer that sits on top of the Docker Enterprise Engine. Once deployed, administrators interact with their cluster via UCP instead of each node's individual Docker engines. Since the UCP supports both Kubernetes and Docker via a Web UI or CLI, administrators can use either a Kubernetes YAMLs or Docker Compose files to deploy images from the centralized location. It also provides cluster-wide monitoring of deployed containers, services, and pods.

Installing Trident for Kubernetes on UCP managed nodes is similar to installing Trident on Kubernetes. Refer to the following :ref:`documentation <Deploying>` for instructions on how to install Trident for Kubernetes.

Please note that starting with Docker EE 2.1 UCP and Trident 19.01, it's no longer required to specify the ``--ucp-host`` and ``--ucp-bearer-token`` parameters while installing and uninstalling Trident. Deploy the ``tridentctl install -n <namespace>`` command to start the installation on the UCP managed nodes.


Using Trident with the NetApp Kubernetes Service
=================================================

The `NetApp Kubernetes Service`_ (NKS) is a universal control plane through which production grade Kubernetes clusters
can be provisioned and run on the cloud provider of choice.

.. _NetApp Kubernetes Service: https://cloud.netapp.com/kubernetes-service

Refer to the following :ref:`documentation <Deploying>` for instructions on how to install Trident for NKS.

Deploying Trident as an enhanced CSI Provisioner
================================================

The Trident 19.10 release is built on a production-ready CSI 1.1 provisioner implementation. This allows
Trident to absorb standardized features like snapshots, while still retaining its ability to innovate on the storage model.

To setup Trident as a CSI provisioner, refer to the :ref:`Deployment Guide <deploying-in-kubernetes>`. Ensure
that the required :ref:`Feature Gates <Feature Gates>` are enabled.
After deploying, you should consider :ref:`Upgrading existing PVs to CSI volumes <Upgrading legacy volumes to CSI volumes>`
if you would like to
use new features such as :ref:`On-demand snapshots <Creating Snapshots of Persistent Volumes>`.

.. _installer bundle: https://github.com/NetApp/trident/releases/latest

CRDs for maintaining Trident's state
====================================

The 19.07 release of Trident introduces a set of :ref:`Custom Resource Definitions(CRDs) <Kubernetes CustomResourceDefinition objects>`
for maintaining
Trident's stateful information. CRDs are a Kubernetes construct used to group a set of similar objects
together and classify them as user-defined resources. This translates to Trident no longer needing a
dedicated etcd and a PV that it needs to use on the backend storage. All stateful objects used by Trident
will be CRD objects that are present in the Kubernetes cluster's etcd.

Things to keep in mind about Trident's CRDs
-------------------------------------------

1. When Trident is installed, a set of CRDs are created and can be used like any other resource type.

2. When :ref:`upgrading from a previous version of Trident <Upgrading Trident>` (one that used etcd to maintain state), the Trident
   installer will migrate data from the etcd key-value data store and create corresponding CRD objects.

3. :ref:`Downgrading <Downgrading Trident>` to a previous Trident version is not recommended.

4. When uninstalling Trident using the ``tridentctl uninstall`` command, Trident pods are deleted but the created CRDs will not be cleaned up. Refer to the :ref:`Uninstalling Guide <Uninstalling Trident>` to understand how Trident can be completely removed and reconfigured from scratch.

5. Since the CRD objects that are used by Trident are stored in the Kubernetes cluster's etcd, :ref:`Trident disaster recovery workflows <Backup and Disaster Recovery>` will be different when compared to previous versions of Trident.

Trident Upgrade/Downgrade Process
=================================

Upgrading Trident
-----------------

If you are looking to upgrade to the latest version of Trident, the :ref:`Upgrade section <Upgrading Trident>`
provides a complete overview of the upgrade process.

Downgrading Trident
-------------------

**Downgrading to a previous release is not recommended**. If you choose to downgrade, ensure that the PV
used by the previous Trident installation is available.

Refer to the :ref:`Troubleshooting <Troubleshooting>` section to understand what happens when a downgrade is
attempted.

Recommendations for all deployments
===================================

Deploy Trident to a dedicated namespace
---------------------------------------

`Namespaces <https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/>`_ provide administrative separation between different applications and are a barrier for resource sharing, for example, a PVC from one namespace cannot be consumed from another.  Trident provides PV resources to all namespaces in the Kubernetes cluster and consequently leverages a service account which has elevated privileges.

Additionally, access to the Trident pod may enable a user to access storage system credentials and other sensitive information.  It is important to ensure that application users and management applications do not have the ability to access the Trident object definitions or the pods themselves.

Use quotas and range limits to control storage consumption
----------------------------------------------------------

Kubernetes has two features which, when combined, provide a powerful mechanism for limiting the resource consumption by applications.  The `storage quota mechanism <https://kubernetes.io/docs/concepts/policy/resource-quotas/#storage-resource-quota>`_ allows the administrator to implement global, and storage class specific, capacity and object count consumption limits on a per-namespace basis.  Further, using a `range limit <https://kubernetes.io/docs/tasks/administer-cluster/limit-storage-consumption/#limitrange-to-limit-requests-for-storage>`_ will ensure that the PVC requests must be within both a minimum and maximum value before the request is forwarded to the provisioner.

These values are defined on a per-namespace basis, which means that each namespace will need to have values defined which fall in line with their resource requirements.  An example of `how to leverage quotas <https://netapp.io/2017/06/09/self-provisioning-storage-kubernetes-without-worry/>`_ can be found on `netapp.io <https://netapp.io>`_.


Deploying Trident to OpenShift
==============================

OpenShift uses Kubernetes for the underlying container orchestrator. Consequently, the same recommendations will apply when using Trident with Kubernetes or OpenShift. However, there are some minor additions when using OpenShift which should be taken into consideration.

Deploy Trident to infrastructure nodes
--------------------------------------

Trident is a core service to the OpenShift cluster, provisioning and managing the volumes used across all projects. Consideration should be given to deploying Trident to the infrastructure nodes in order to provide the same level of care and concern.

To deploy Trident to the infrastructure nodes, the project for Trident must be created by an administrator using the `oc adm` command. This prevents the project from inheriting the default node selector, which forces the pod to execute on compute nodes.

.. code-block:: console

   # create the project which Trident will be deployed to using
   # the non-default node selector
   oc adm new-project <project_name> --node-selector="region=infra"

   # deploy Trident using the project name
   tridentctl install -n <project_name>

The result of the above command is that any pod deployed to the project will be scheduled to nodes which have the tag "``region=infra``".  This also removes the default node selector used by other projects which schedule pods to nodes which have the label "``node-role.kubernetes.io/compute=true``".
