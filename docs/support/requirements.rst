************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident supports multiple container engines and orchestrators, including:

* Anthos GKE On-Prem v1.5
* Docker Enterprise 2.1, 3.0, and 3.1
* Kubernetes 1.11 or later (latest: 1.20)
* OpenShift 3.11, 4.2, 4.3, 4.4, 4.5, and 4.6.8

The Trident Operator is supported with these releases:

* Anthos GKE On-Prem v1.5
* Kubernetes 1.14 or later (latest: 1.20)
* OpenShift 4.2, 4.3, 4.4, 4.5, and 4.6.8

.. important::

  Red Hat OpenShift Container Platform users might observe their ``initiatorname.iscsi`` file to be blank if using any version below 4.6.8. This is a bug that has been identified by RedHat to be fixed with OpenShift 4.6.8. See `this bug fix announcement <https://access.redhat.com/errata/RHSA-2020:5259/>`_. NetApp recommends that you use Trident 20.10 or later on OpenShift 4.6.8.

Trident also works with a host of other fully managed and self-managed Kubernetes offerings, including Google Cloud’s Google Kubernetes Engine (GKE), AWS’s Elastic Kubernetes Services (EKS), Azure’s Azure Kubernetes Service (AKS), and Rancher.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* Azure NetApp Files
* Cloud Volumes ONTAP
* Cloud Volumes Service for AWS
* Cloud Volumes Service for GCP
* E/EF-Series SANtricity
* FAS/AFF/Select 9.1 or later
* NetApp All SAN Array (ASA)
* NetApp HCI/Element software 8 or later

Feature requirements
====================

Trident requires some feature gates to be enabled for certain features
to work. See the table below to determine if you need to
enable feature gates, based on your version of Trident and Kubernetes.

================================ =============== ========================== ===============================
         Feature                 Trident version    Kubernetes version         Feature gates required?
================================ =============== ========================== ===============================
CSI Trident                      19.07 and above   1.13\ :sup:`1` and above   Yes for ``1.13``\ :sup:`1`
Volume Snapshots (beta)          20.01 and above       1.17 and above                    No
PVC from Volume Snapshots (beta) 20.01 and above       1.17 and above                    No
iSCSI PV resize                  19.10 and above       1.16 and above                    No
ONTAP Bidirectional CHAP         20.04 and above       1.11 and above                    No
Dynamic Export Policies          20.04 and above  1.13\ :sup:`1` and above   Requires CSI Trident\ :sup:`1`
Trident Operator                 20.04 and above       1.14 and above                    No
Auto Worker Node Prep (beta)     20.10 and above  1.13\ :sup:`1` and above   Requires CSI Trident\ :sup:`1`
CSI Topology                     20.10 and above       1.17 and above                    No
================================ =============== ========================== ===============================

| Footnote:
| `1`: Requires enabling ``CSIDriverRegistry`` and ``CSINodeInfo``
       for Kubernetes 1.13. Install CSI Trident on Kubernetes 1.13 using
       the ``--csi`` switch when invoking ``tridentctl install``.

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
* RedHat CoreOS 4.2 and 4.3
* RHEL or CentOS 7.4 or later
* Ubuntu 18.04 or later

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
