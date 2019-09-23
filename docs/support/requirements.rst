************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident supports multiple container engines and orchestrators, including:

* `NetApp Kubernetes Service <https://cloud.netapp.com/kubernetes-service>`_
* Kubernetes 1.11 to 1.15
* OpenShift 3.11 
* Docker Enterprise 17.06 or later (latest: 3.0)

In addition, Trident should work with any distribution of Docker or Kubernetes
that uses one of the supported versions as a base, such as Rancher or Tectonic.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* FAS/AFF/Select/Cloud ONTAP 8.3 or later
* HCI/SolidFire Element OS 8 or later
* E/EF-Series SANtricity
* Azure NetApp Files
* Cloud Volumes Service for AWS

Feature Gates
=============

Trident requires some feature gates to be enabled for it to work.
These feature gates are detailed below:

======================== ======= ===== ===== =====
Feature Gate             Default Stage Since Until
======================== ======= ===== ===== =====
CSIDriverRegistry        False   Alpha 1.12  1.13
CSIDriverRegistry        True    Beta  1.14    -
CSINodeInfo              False   Alpha 1.12  1.13
CSINodeInfo              True    Beta  1.14    -
VolumeSnapshotDataSource False   Alpha 1.12    -
======================== ======= ===== ===== =====

If using Kubernetes ``1.13``:

- Enable the ``CSIDriverRegistry``, ``CSINodeInfo`` and
  ``VolumeSnapshotDataSource`` flags.

If using Kubernetes ``1.14`` and above:

- Enable the ``VolumeSnapshotDataSource`` flag. The other
  flags are enabled by default.

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
