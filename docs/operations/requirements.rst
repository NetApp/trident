************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident is designed to operate in and integrate with Kubernetes, and it
currently supports:

* Kubernetes 1.5, 1.6, 1.7 and 1.8
* OpenShift 1.5/3.5 and 3.6

In addition, Trident should work with any distribution of Kubernetes that uses
one of the supported Kubernetes versions as a base, such as Tectonic.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* FAS/AFF/Select/Cloud ONTAP 8.3 or later
* SolidFire Element OS 7 or later
* E/EF-Series SANtricity

Supported host operating systems
================================

By default Trident itself runs in a container, in a pod, as part of a Kubernetes
deployment, therefore it will run on any Linux worker.

However, Kubernetes itself does need to be able to mount the volumes that
Trident provides using the standard NFS client or iSCSI initiator, depending on
the backend(s) you're using.

These are the distributions that are known to work:

* Debian 8 and above
* Ubuntu 14.04 and above, 15.10 and above if using iSCSI multipathing
* CentOS 7.0 and above
* RHEL 7.0 and above
* CoreOS 1353.8.0 and above

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
