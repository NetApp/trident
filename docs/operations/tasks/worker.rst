##################
Worker preparation
##################

All of the worker nodes in the Kubernetes cluster need to be able to mount the
volumes that users have provisioned for their pods.

If you are using the ``ontap-nas`` or ``ontap-nas-economy`` driver for one of
your backends, your workers will need the :ref:`NFS` tools. Otherwise they
require the :ref:`iSCSI` tools.

.. note::
  Recent versions of CoreOS have both installed by default.

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

       sudo service multipath-tools restart

  #. Ensure that ``open-iscsi`` and ``multipath-tools`` are running:

     .. code-block:: bash

       sudo service open-iscsi start
       sudo service multipath-tools start
