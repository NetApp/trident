##################
Worker preparation
##################

All of the worker nodes in the Kubernetes cluster need to be able to mount the
volumes that users have provisioned for their pods.

If you are using the ``ontap-nas``, ``ontap-nas-economy``, ``ontap-nas-flexgroup`` driver for one of
your backends, your workers will need the :ref:`NFS` tools. Otherwise they
require the :ref:`iSCSI` tools.

.. note::
  Recent versions of RedHat CoreOS have both installed by default. You must ensure
  that the NFS and iSCSI services are started up during boot time.

.. note::
   When using worker nodes that run RHEL/RedHat CoreOS with iSCSI
   PVs, make sure to specify the ``discard`` mountOption in the
   `StorageClass <https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options>`_
   to perform inline space reclamation. Take a look at
   RedHat's documentation `here <https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/managing_file_systems/discarding-unused-blocks_managing-file-systems>`_.

.. warning::
  You should always reboot your worker nodes after installing the NFS or iSCSI
  tools, or attaching volumes to containers may fail.

Automatic Worker Node Prep
==========================

Trident can automatically install the required :ref:`NFS` and :ref:`iSCSI` tools
on nodes present in the Kubernetes cluster. This is a **beta feature** and is
**not meant for** production clusters just yet. Today, the feature is available
for nodes that run **CentOS, RHEL, and Ubuntu**.

For this, Trident includes a new install flag: ``--enable-node-prep``.

.. warning::

  The ``--enable-node-prep`` install option will instruct Trident to install and
  ensure NFS and iSCSI packages and/or services are running when a volume is
  mounted on a worker node. This is a **beta feature** meant to be used in
  Dev/Test environments that is **not qualified** for production use yet.

When the ``--enable-node-prep`` flag is included for Trident installs made with
``tridentctl`` (for the Trident operator, use the Boolean option ``enableNodePrep``):

1. As part of the installation, Trident registers the nodes it runs on.
2. When a PVC request is made, Trident creates a PV from one of the backends it
   manages.
3. Using the PVC in a pod would require Trident to mount the volume on the node
   the pod runs on. Trident attempts to install the required NFS/iSCSI client
   utilities and to ensure the required services are active. This is done before
   mounting the volume.

The preparation of a worker node is done only once: as part of the first attempt
made to mount a volume. All subsequent volume mounts should succeed as long as
no changes outside Trident touch the :ref:`NFS` and :ref:`iSCSI` utilities.

In this manner, Trident can make sure that all nodes in a Kubernetes cluster
have the required utilities needed to mount and attach volumes. For NFS volumes,
the export policy must also permit the volume to be mounted. Trident can
automatically manage export policies per backend; alternatively, users can manage
export policies out-of-band.

NFS
===

Install the following system packages:

**RHEL / CentOS**

  .. code-block:: bash

    sudo yum install -y nfs-utils

**Ubuntu / Debian**

  .. code-block:: bash

    sudo apt-get install -y nfs-common

iSCSI
=====

.. _iscsi-worker-node-prep:

.. warning::

   If using RHCOS >=4.5 or RHEL >=8.2 with the ``solidfire-san`` driver ensure
   that the CHAP authentication algorithm is set to ``MD5`` in ``/etc/iscsi/iscsid.conf``

   .. code-block:: bash

      sudo sed -i 's/^\(node.session.auth.chap_algs\).*/\1 = MD5/' /etc/iscsi/iscsid.conf


**RHEL / CentOS**

  #. Install the following system packages:

     .. code-block:: bash

       sudo yum install -y lsscsi iscsi-initiator-utils sg3_utils device-mapper-multipath

  #. Check that iscsi-initiator-utils version is 6.2.0.874-2.el7 or higher:

     .. code-block:: bash

       rpm -q iscsi-initiator-utils

  #. Set scanning to manual:

     .. code-block:: bash

       sudo sed -i 's/^\(node.session.scan\).*/\1 = manual/' /etc/iscsi/iscsid.conf

  #. Enable multipathing:

     .. code-block:: bash

       sudo mpathconf --enable --with_multipathd y

  #. Ensure that ``iscsid`` and ``multipathd`` are running:

     .. code-block:: bash

       sudo systemctl enable --now iscsid multipathd

  #. Start and enable ``iscsi``:

     .. code-block:: bash

       sudo systemctl enable --now iscsi

**Ubuntu / Debian**

.. note::

   For Ubuntu 18.04 you must discover target ports with ``iscsiadm``
   before starting ``open-iscsi`` for the iSCSI daemon to start. You
   can alternatively modify the ``iscsi`` service to start ``iscsid``
   automatically.

#. Install the following system packages:

   .. code-block:: bash

     sudo apt-get install -y open-iscsi lsscsi sg3-utils multipath-tools scsitools

#. Check that open-iscsi version is 2.0.874-5ubuntu2.10 or higher (for bionic) or 2.0.874-7.1ubuntu6.1 or higher (for focal):

   .. code-block:: bash

     dpkg -l open-iscsi

#. Set scanning to manual:

   .. code-block:: bash

     sudo sed -i 's/^\(node.session.scan\).*/\1 = manual/' /etc/iscsi/iscsid.conf

#. Enable multipathing:

   .. code-block:: bash

     sudo tee /etc/multipath.conf <<-'EOF'
     defaults {
         user_friendly_names yes
         find_multipaths yes
     }
     EOF

     sudo systemctl enable --now multipath-tools.service
     sudo service multipath-tools restart

#. Ensure that ``open-iscsi`` and ``multipath-tools`` are enabled and running:

   .. code-block:: bash

     sudo systemctl status multipath-tools
     sudo systemctl enable --now open-iscsi.service
     sudo systemctl status open-iscsi
