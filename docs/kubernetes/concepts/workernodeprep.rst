#################################
Automatic worker node preparation
#################################

Trident can automatically install the required :ref:`NFS` and :ref:`iSCSI` tools
on nodes present in the Kubernetes cluster. This is a **beta feature** and is
**not meant for** production clusters just yet. Today, the feature is available
for nodes that run **CentOS, RHEL, and Ubuntu**.

For this, Trident includes a new install flag: ``--enable-node-prep``.

.. warning::

  The ``--enable-node-prep`` installation option instructs Trident to install and
  ensure that NFS and iSCSI packages and/or services are running when a volume is
  mounted on a worker node. This is a **beta feature** meant to be used in
  Dev/Test environments that is **not qualified** for production use.

When the ``--enable-node-prep`` flag is included for Trident installations made with
``tridentctl`` (for the Trident operator, use the Boolean option ``enableNodePrep``), the following happens:

1. As part of the installation, Trident registers the nodes it runs on.
2. When a PVC request is made, Trident creates a PV from one of the backends it
   manages.
3. Using the PVC in a pod would require Trident to mount the volume on the node
   the pod runs on. Trident attempts to install the required NFS/iSCSI client
   utilities and to ensure that the required services are active. This is done before
   mounting the volume.

The preparation of a worker node is done only once as part of the first attempt
made to mount a volume. All subsequent volume mounts should succeed as long as
no changes outside Trident touch the :ref:`NFS` and :ref:`iSCSI` utilities.

In this manner, Trident can make sure that all the nodes in a Kubernetes cluster
have the required utilities needed to mount and attach volumes. For NFS volumes,
the export policy should also permit the volume to be mounted. Trident can
automatically manage export policies per backend; alternatively, users can manage
export policies out-of-band.
