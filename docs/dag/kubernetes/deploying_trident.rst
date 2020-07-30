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

Installing Trident on a Kubernetes cluster will result in the Trident
installer:

1. Fetching the container images over the Internet.

2. Creating a deployment and/or node daemonset which spin up Trident pods
   on all eligible nodes in the Kubernetes cluster.

A standard installation such as this can be performed in two different
ways:

1. Using ``tridentctl install`` to install Trident.

2. Using the Trident Operator.

This mode of installing is the easiest way to install Trident and
works for most environments that do not impose network restrictions. The
:ref:`Deploying <Deploying>` guide will help you get started.

**Offline install mode**

In many organizations, production and development environments do not have access to public repositories for pulling and posting images as these environments are completely secured and restricted. Such environments only allow pulling images from trusted private repositories.

To perform an air-gapped installation of Trident, you can use the ``--image-registry`` flag
when invoking ``tridentctl install`` to point to a private image registry. If installing with
the Trident Operator, you can alternatively specify ``spec.imageRegistry`` in your
TridentProvisioner. This registry must contain the Trident image
(obtained `here <https://hub.docker.com/r/netapp/trident/>`_)
and the CSI sidecar images as required by your Kubernetes version.

To customize your installation further, you can use ``tridentctl`` to generate the manifests
for Trident's resources. This includes the deployment, daemonset, service account and the cluster
role that Trident creates as part of its installation. 
The :ref:`Customized Installation <Customized Installation>` section talks about the options available
for performing a custom Trident install.

**Remote install mode**

Trident can be installed on a Kubernetes cluster from a remote machine.
To do a remote install, install the appropriate version of ``kubectl``
on the remote machine from where you would be installing Trident. Copy
the configuration files from the Kubernetes cluster and set the KUBECONFIG
environment variable on the remote machine. Initiate a ``kubectl get nodes``
command to verify you can connect to the required Kubernetes cluster.
Complete the Trident deployment from the remote machine using the normal
installation steps.

Trident Installation on Docker UCP 3.1
======================================

Docker EE Universal Control Plane (UCP) is the cluster management layer that sits on top of the Docker Enterprise Engine. Once deployed, administrators interact with their cluster via UCP instead of each node's individual Docker engines. Since the UCP supports both Kubernetes and Docker via a Web UI or CLI, administrators can use either a Kubernetes YAMLs or Docker Compose files to deploy images from the centralized location. It also provides cluster-wide monitoring of deployed containers, services, and pods.

Installing Trident for Kubernetes on UCP managed nodes is similar to installing Trident on Kubernetes. Refer to the following :ref:`documentation <Deploying>` for instructions on how to install Trident for Kubernetes.

Please note that starting with Docker EE 2.1 UCP and Trident 19.01, it's no longer required to specify the ``--ucp-host`` and ``--ucp-bearer-token`` parameters while installing and uninstalling Trident. Deploy the ``tridentctl install -n <namespace>`` command to start the installation on the UCP managed nodes.

Deploying Trident as an enhanced CSI Provisioner
================================================

Trident provides a CSI frontend that can be used to install Trident as a CSI provisioner. Available exclusively
for Kubernetes ``1.13`` and above, this allows Trident to absorb standardized features like snapshots
while still retaining its ability to innovate on the storage model.

To setup Trident as a CSI provisioner, refer to the :ref:`Deployment Guide <deploying-in-kubernetes>`. Ensure
that the required :ref:`Feature Gates <Feature Requirements>` are enabled.
After deploying, you should consider :ref:`Upgrading existing PVs to CSI volumes <Upgrading legacy volumes to CSI volumes>`
if you would like to
use new features such as :ref:`On-demand snapshots <On-Demand Volume Snapshots>`.

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

Deploy Trident to infrastructure nodes (OpenShift 3.11)
-------------------------------------------------------

Trident is a core service to the OpenShift cluster, provisioning and managing the volumes used across all projects. Consideration should be given to deploying Trident to the infrastructure nodes in order to provide the same level of care and concern.

To deploy Trident to the infrastructure nodes, the project for Trident must be created by an administrator using the `oc adm` command. This prevents the project from inheriting the default node selector, which forces the pod to execute on compute nodes.

.. code-block:: console

   # create the project which Trident will be deployed to using
   # the non-default node selector
   oc adm new-project <project_name> --node-selector="region=infra"

   # deploy Trident using the project name
   tridentctl install -n <project_name>

The result of the above command is that any pod deployed to the project will be scheduled to nodes which have the tag "``region=infra``".  This also removes the default node selector used by other projects which schedule pods to nodes which have the label "``node-role.kubernetes.io/compute=true``".
