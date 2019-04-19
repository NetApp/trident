##########################
CSI Trident for Kubernetes
##########################

CSI overview
============

The container storage interface (`CSI`_) is a standardized API for container orchestrators to manage storage plugins.
A community-driven effort, CSI promises to let storage vendors write a single plugin to make their products
work with multiple container orchestrators.

Kubernetes 1.13 contains support for CSI-based plugins.  It uses CSI version 1.0, which has very
limited capabilities, including creating/deleting/mounting/unmounting persistent volumes.

CSI Trident
===========

As an independent external volume provisioner, Trident for Kubernetes offers many capabilities beyond those
allowed by CSI, so it is unlikely that Trident will limit itself to CSI in the foreseeable future.

However, NetApp is participating in the community's efforts to see CSI achieve its full potential.  As part of that
work, we have developed a public preview version of Trident that deploys itself as a CSI plugin.  The goals of shipping
CSI Trident are to better inform our contributions to CSI as well as to give our customers a way to kick the tires
on CSI and see where the container storage community is going.

.. warning::
  CSI Trident for Kubernetes is an unsupported, alpha-level preview release for evaluation purposes only, and it
  should not be deployed in support of production workloads.  We recommend you install CSI Trident in a sandbox
  cluster used primarily for testing and evaluation.

Installing CSI Trident
----------------------

To install CSI Trident, follow the the extensive :ref:`deployment <deploying-in-kubernetes>` guide, with just one
difference.

Invoke the install command with the ``--csi`` switch:

.. code-block:: console

   #./tridentctl install -n trident --csi
   WARN CSI Trident for Kubernetes is a technology preview and should not be installed in production environments!
   INFO Starting Trident installation.                namespace=trident
   INFO Created service account.
   INFO Created cluster role.
   INFO Created cluster role binding.
   INFO Created Trident service.
   INFO Created Trident statefulset.
   INFO Created Trident daemonset.
   INFO Waiting for Trident pod to start.
   INFO Trident pod started.                          namespace=trident pod=trident-csi-0
   INFO Waiting for Trident REST interface.
   INFO Trident REST interface is up.                 version=19.04.0
   INFO Trident installation succeeded.

It will look like this when the installer is complete:

.. code-block:: console

   # kubectl get pod -n trident
   NAME                       READY     STATUS    RESTARTS   AGE
   trident-csi-0              4/4       Running   1          2m
   trident-csi-vwhl2          2/2       Running   0          2m

   # ./tridentctl -n trident version
   +----------------+----------------+
   | SERVER VERSION | CLIENT VERSION |
   +----------------+----------------+
   | 19.04.0        | 19.04.0        |
   +----------------+----------------+

Using CSI Trident
-----------------

To provision storage with CSI Trident, define one or more storage classes with the provisioner value
``csi.trident.netapp.io``.  The simplest storage class to start with is one based on the
``sample-input/storage-class-csi.yaml.templ`` file that comes with the installer, replacing ``__BACKEND_TYPE__``
with the storage driver name.:

.. code-block:: console

   apiVersion: storage.k8s.io/v1
   kind: StorageClass
   metadata:
     name: basic-csi
   provisioner: csi.trident.netapp.io
   parameters:
     backendType: "__BACKEND_TYPE__"

.. note::
  ``tridentctl`` will detect and manage either Trident or CSI Trident automatically.  We don't recommend installing
  both on the same cluster, but if both are present, use the ``--csi`` switch to force ``tridentctl`` to manage
  CSI Trident.

Uninstalling CSI Trident
------------------------

Use the ``--csi`` switch to uninstall CSI Trident:

.. code-block:: console

   #./tridentctl uninstall --csi
   INFO Deleted Trident daemonset.
   INFO Deleted Trident statefulset.
   INFO Deleted Trident service.
   INFO Deleted cluster role binding.
   INFO Deleted cluster role.
   INFO Deleted service account.
   INFO The uninstaller did not delete the Trident's namespace, PVC, and PV in case they are going to be reused. Please use the --all option if you need the PVC and PV deleted.
   INFO Trident uninstallation succeeded.


.. _CSI: https://github.com/container-storage-interface/spec
.. _beta: https://kubernetes.io/blog/2018/04/10/container-storage-interface-beta/
