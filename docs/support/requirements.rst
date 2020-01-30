************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident supports multiple container engines and orchestrators, including:

* `NetApp Kubernetes Service <https://cloud.netapp.com/kubernetes-service>`_
* Kubernetes 1.11 or later (latest: 1.17)
* OpenShift 3.11 or 4.2
* Docker Enterprise 2.1 or 3.0

In addition, Trident should work with any distribution of Docker or Kubernetes
that uses one of the supported versions as a base, such as Rancher or Tectonic.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* FAS/AFF/Select 9.1 or later
* HCI/SolidFire Element OS 8 or later
* E/EF-Series SANtricity
* Azure NetApp Files
* Cloud Volumes ONTAP
* Cloud Volumes Service for AWS
* Cloud Volumes Service for GCP

Feature Gates
=============

Trident requires some feature gates to be enabled for certain features
to work. Refer to the table shown below to determine if you need to
enable feature gates, based on your version of Trident and Kubernetes.

================================ =============== ==========================
         Feature                 Trident version    Kubernetes version
================================ =============== ==========================
CSI Trident                      19.07 and above   1.13\ :sup:`1` and above
Volume Snapshots (beta)          20.01 and above       1.17 and above
PVC from Volume Snapshots (beta) 20.01 and above       1.17 and above
iSCSI PV resize                  19.10 and above       1.16 and above
================================ =============== ==========================

| Footnote:
| `1`: Requires enabling ``CSIDriverRegistry`` and ``CSINodeInfo``
       for Kubernetes 1.13. Install CSI Trident on Kubernetes 1.13 using
       the ``--csi`` switch when invoking ``tridentctl install``.

.. note::
   All features mentioned in the table above require CSI Trident.

Check with your Kubernetes vendor to determine the appropriate procedure
for enabling feature gates.

Supported host operating systems
================================

By default Trident itself runs in a container, therefore it will run on any
Linux worker.

However, those workers do need to be able to mount the volumes that Trident
provides using the standard NFS client or iSCSI initiator, depending on the
backend(s) you're using.

These are the Linux distributions that are known to work:

* Debian 8 or later
* Ubuntu 16.04 or later
* CentOS 7.0 or later
* RHEL 7.0 or later
* CoreOS 1353.8.0 or later

The ``tridentctl`` utility also runs on any of these distributions of Linux.

Host configuration
==================

Depending on the backend(s) in use, NFS and/or iSCSI utilities must be
installed on all of the workers in the cluster. See the
:ref:`worker preparation <Worker preparation>` guide for details.

Storage system configuration
============================

Trident may require some changes to a storage system before a backend
configuration can use it. See the
:ref:`backend configuration <Backend configuration>` guide for details.
