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


**RHEL / CentOS**

  #. Install the following system packages:

     .. code-block:: bash

       sudo yum install -y lsscsi iscsi-initiator-utils sg3_utils device-mapper-multipath

  #. Enable multipathing:

     .. code-block:: bash

       sudo mpathconf --enable --with_multipathd y

  #. Ensure that ``iscsid`` and ``multipathd`` are running:

     .. code-block:: bash

       sudo systemctl enable iscsid multipathd
       sudo systemctl start iscsid multipathd

  #. Start and enable ``iscsi``:

     .. code-block:: bash

       sudo systemctl enable iscsi
       sudo systemctl start iscsi

**Ubuntu / Debian**

  #. Install the following system packages:

     .. code-block:: bash

       sudo apt-get install -y open-iscsi lsscsi sg3-utils multipath-tools scsitools

  #. Enable multipathing:

     .. code-block:: bash

       sudo tee /etc/multipath.conf <<-'EOF'
       defaults {
           user_friendly_names yes
           find_multipaths yes
       }
       EOF
       
       sudo systemctl enable multipath-tools.service
       sudo service multipath-tools restart

  #. Ensure that ``open-iscsi`` and ``multipath-tools`` are enabled and running:

     .. code-block:: bash
       
       sudo systemctl status multipath-tools
       sudo systemctl enable open-iscsi.service
       sudo service open-iscsi start
       sudo systemctl status open-iscsi
