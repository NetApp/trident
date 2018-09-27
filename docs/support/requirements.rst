************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident supports multiple container engines and orchestrators, including:

.. warning::
  Do not use Docker 18.03.0-ce. It contains a `critical bug`_ that impacts
  most volume plugins, including Trident. The fix is available in 18.03.1-ce.

.. _critical bug: https://github.com/moby/moby/issues/36784

* Docker 17.06 (CE or EE) or later (latest: 18.03, but see the warning above)
* Docker Enterprise Edition 17.06 or later (latest: 2.0)
* Kubernetes 1.6 or later (latest: 1.12)
* OpenShift 3.6 or later (latest: 3.10)

In addition, Trident should work with any distribution of Docker or Kubernetes
that uses one of the supported versions as a base, such as Rancher or Tectonic.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* FAS/AFF/Select/Cloud ONTAP 8.3 or later
* SolidFire Element OS 8 or later
* E/EF-Series SANtricity

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

External etcd cluster (Optional)
================================

Trident uses etcd v3.1.3 or later to store its metadata. The standard
installation process includes an etcd container that is managed by Trident and
backed by a volume from a supported storage system, so there is no need to
install it separately.

If you would prefer to use a separate external etcd cluster instead, Trident
can easily be configured to do so. See the :ref:`external etcd guide <etcd>`
for details.
