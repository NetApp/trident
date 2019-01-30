.. _deploying_trident:

*****************
Deploying Trident
*****************

The guidelines in this section provide recommendations for Trident installation with various Kubernetes configurations and considerations. As with all the other recommendations in this guide, each of these suggestions should be carefully considered to determine if it's appropriate and will provide benefit to your deployment.

Supported Kubernetes cluster architectures
==========================================

Trident is supported with the following Kubernetes architectures. In each of the Kubernetes architectures below, the installation steps remain relatively the same except for the ones which have an asterick.

   +-----------------------------------------------+-----------+---------------------+
   |         Kubernetes Cluster Architectures      | Supported | Normal Installation |
   +===============================================+===========+=====================+
   | Single master, compute                        | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+
   | Multiple master, compute                      | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+
   | Master, etcd, compute                         | Yes*      |       No            |
   +-----------------------------------------------+-----------+---------------------+
   | Master, infrastructure, compute               | Yes       |       Yes           |
   +-----------------------------------------------+-----------+---------------------+

The cell marked with an asterik above has an external production etcd cluster and requires different installation steps for deploying Trident. The :ref:`Trident etcd documentation <etcd>` discusses in detail how to freshly deploy Trident on an external etcd cluster. It also mentions how to migrate existing Trident deployment to an external etcd cluster as well.  

Trident installation modes 
==========================

Three ways to install Trident are discussed in this chapter.

**Normal install mode**

Normal installation involves running the ``tridentctl install -n trident`` command which deploys the Trident pod on the Kubernetes cluster. Trident installation is quite a straightforward process. For more information on installation and provisioning of volumes, refer to the :ref:`Deploying documentation <Deploying>`.

**Offline install mode**

In many organizations, production and development environments do not have access to public repositories for pulling and posting images as these environments are completely secured and restricted. Such environments only allow pulling images from trusted private repositories. 
In such scenarios, make sure that a private registry instance is available. Then trident and etcd images should be downloaded from a bastion host with internet access and pushed on to the private registry. To install Trident in offline mode, just issue the ``tridentctl install -n trident`` command with the  ``--etcd-image`` and  the ``--trident-image`` parameter with the appropriate image name and location. For more information on how to install Trident in offline mode, please examine the blog on `Installing Trident for Kubernetes from a Private Registry <https://netapp.io/2018/12/19/installing-trident-from-a-private-registry/>`_.


**Remote install mode**

Trident can be installed on a Kubernetes cluster from a remote machine. To do a remote install, install the appropriate version of ``kubectl`` on the remote machine from where you would be running the ``tridentctl install`` command remotely. Copy the configuration files from the Kubernetes cluster and set the KUBECONFIG environment variable on the remote machine. Initiate a ``kubectl get nodes`` command to verify you can connect to the required Kubernetes cluster. Complete the Trident deployment from the remote machine using the normal installation steps. 

Trident CSI installation
========================
The Container Storage Interface (CSI) is a standardized API for container orchestrators to manage storage plugins. CSI still in early stages and has not yet integrated much functionality. However, using the early specs, NetApp developed a Trident CSI for Kubernetes alpha driver for the Kubernetes CSI Beta integration for testing purposes only. As of Trident 19.01, NetApp recommends not deploying CSI in production environments until CSI becomes more mature. More information regarding CSI can be found in the :ref:`Trident documentation <CSI Trident for Kubernetes>`.


Recommendations for all deployments
===================================

Deploy Trident to a dedicated namespace
---------------------------------------

`Namespaces <https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/>`_ provide administrative separation between different applications and are a barrier for resource sharing, for example a PVC from one namespace cannot be consumed from another.  Trident provides PV resources to all namespaces in the Kubernetes cluster and consequently leverages a service account which has elevated privileges.  

Additionally, access to the Trident pod may enable a user to access storage system credentials and other sensitive information.  It is important to ensure that application users and management applications do not have the ability to access the Trident object definitions or the pods themselves.

Use quotas and range limits to control storage consumption
----------------------------------------------------------

Kubernetes has two features which, when combined, provide a powerful mechanism for limiting the resource consumption by applications.  The `storage quota mechanism <https://kubernetes.io/docs/concepts/policy/resource-quotas/#storage-resource-quota>`_ allows the administrator to implement global, and storage class specific, capacity and object count consumption limits on a per-namespace basis.  Further, using a `range limit <https://kubernetes.io/docs/tasks/administer-cluster/limit-storage-consumption/#limitrange-to-limit-requests-for-storage>`_ will ensure that the PVC requests must be within both a minimum and maximum value before the request is forwarded to the provisioner.

These values are defined on a per-namespace basis, which means that each namespace will need to have values defined which fall in line with their resource requirements.  An example of `how to leverage quotas <https://netapp.io/2017/06/09/self-provisioning-storage-kubernetes-without-worry/>`_ can be found on `netapp.io <https://netapp.io>`_.

Use PVC protection to protect in-use resources
----------------------------------------------

This feature is on by default if running Kubernetes 1.10 or greater. Follow this process only if running Kubernetes 1.09 or less.

`Storage object in use protection <https://kubernetes.io/docs/tasks/administer-cluster/storage-object-in-use-protection/>`_, or simply PVC protection, is a Kubernetes feature which prevents the deletion of PVCs which are in use by a pod and PVs which are bound to a PVC.  This is important as it prevents a volume from being destroyed while it's actively being used by an application, which can result in data loss.

Implementing PVC protection is slightly different depending on your host operating system and Kubernetes distribution.  Generically, the following process can be followed when using CentOS with "vanilla" Kubernetes, however be sure to follow the documentation for your particular operating system and Kubernetes version.

.. code-block:: console

   # from each master server in the cluster, edit the api server config
   vi /etc/kubernetes/manifests/kube-apiserver.yaml
   
   # search for the line under the "kube-apiserver" command stanza which 
   # starts with "- --admission-control" and append the value 
   # "StorageObjectInUseProtection"
   
   # alternatively, backup the file and use the following sed command 
   cp /etc/kubernetes/manifests/kube-apiserver.yaml \
      /etc/kubernetes/manifests/kube-apiserver.yaml.orig
   
   sed -i '/admission-control/ s/$/,StorageObjectInUseProtection/' \
     /etc/kubernetes/manifests/kube-apiserver.yaml
   
   # after editing, restart the kube-apiserver service
   systemctl restart kube-apiserver.service

After adding the admission controller for storage object protection you can verify it's functioning by viewing the details of a PVC and verifying the presence of the finalizer.

.. code-block:: console
   :emphasize-lines: 11

   [root@kubemaster ~]# kubectl describe pvc pvc-protection
   Name:          pvc-protection
   Namespace:     default
   StorageClass:  performance
   Status:        Bound
   Volume:        default-pvc-protection-3bcc8
   Labels:        <none>
   Annotations:   pv.kubernetes.io/bind-completed=yes
                  pv.kubernetes.io/bound-by-controller=yes
                  volume.beta.kubernetes.io/storage-provisioner=netapp.io/trident
   Finalizers:    [kubernetes.io/pvc-protection]
   Capacity:      3972844748800m
   Access Modes:  RWO
   Events:
     Type    Reason                Age                From                         Message
     ----    ------                ----               ----                         -------
     Normal  ExternalProvisioning  23s (x2 over 23s)  persistentvolume-controller  waiting for a volume to be created, either by external provisioner "netapp.io/trident" or manually created by system administrator
     Normal  ProvisioningSuccess   21s                netapp.io/trident            Kubernetes frontend provisioned a volume and a PV for the PVC

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

The result of the above command is that any pod deployed to the project will be scheduled to nodes which have the tag "``region=infra``".  This also removes the default node selector used by other projects which schedules pods to nodes which have the label "``node-role.kubernetes.io/compute=true``".
