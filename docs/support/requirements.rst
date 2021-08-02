************
Requirements
************

Supported frontends (orchestrators)
===================================

Trident supports multiple container engines and orchestrators, including:

* Mirantis Kubernetes Engine 3.4
* Kubernetes 1.17 or later (latest: 1.21)
* OpenShift 4.4, 4.5, 4.6 [4.6.8+], 4.7, 4.8 (latest 4.8)

The Trident Operator is supported with these releases:

* Kubernetes 1.17 or later (latest: 1.21)
* OpenShift 4.4, 4.5, 4.6 [4.6.8+], 4.7, 4.8 (latest 4.8)

.. important::

  Red Hat OpenShift Container Platform users might observe their ``initiatorname.iscsi`` file to be blank if using any version below 4.6.8. This is a bug that has been identified by RedHat to be fixed with OpenShift 4.6.8. See `this bug fix announcement <https://access.redhat.com/errata/RHSA-2020:5259/>`_. NetApp recommends that you use Trident on OpenShift 4.6.8 and later.

Trident also works with a host of other fully managed and self-managed Kubernetes offerings, including Google Cloud’s Google Kubernetes Engine (GKE), AWS’s Elastic Kubernetes Services (EKS), Azure’s Azure Kubernetes Service (AKS), and Rancher.

Supported backends (storage)
============================

To use Trident, you need one or more of the following supported backends:

* Azure NetApp Files
* Cloud Volumes ONTAP
* Cloud Volumes Service for AWS
* Cloud Volumes Service for GCP
* FAS/AFF/Select 9.3 or later
* NetApp All SAN Array (ASA)
* NetApp HCI/Element Software 8 or later

Feature requirements
====================

The table below summarizes the features available with this release of Trident and the versions of Kubernetes it supports.

================================ ========================== ===============================
         Feature                   Kubernetes version         Feature gates required?
================================ ========================== ===============================
CSI Trident                      1.17 and later                    No
Volume Snapshots                 1.17 and later                    No
PVC from Volume Snapshots        1.17 and later                    No
iSCSI PV resize                  1.17 and later                    No
ONTAP Bidirectional CHAP         1.17 and later                    No
Dynamic Export Policies          1.17 and later                    No
Trident Operator                 1.17 and later                    No
Auto Worker Node Prep (beta)     1.17 and later                    No
CSI Topology                     1.17 and later                    No
================================ ========================== ===============================

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

Container images and corresponding Kubernetes versions
======================================================

For air-gapped installations, see the following table for what container images are needed to install
Trident:

+------------------------+-------------------------------------------------------------+
| KUBERNETES VERSION     | CONTAINER IMAGE                                             |
+========================+=============================================================+
| v1.17.0                | netapp/trident:21.07.0                                      |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-operator:21.07.0                             |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-autosupport:21.01                            |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-provisioner:v2.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-attacher:v3.1.0                  |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-resizer:v1.1.0                   |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-snapshotter:v3.0.3               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.1.0     |
+------------------------+-------------------------------------------------------------+
| v1.18.0                | netapp/trident:21.07.0                                      |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-operator:21.07.0                             |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-autosupport:21.01                            |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-provisioner:v2.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-attacher:v3.1.0                  |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-resizer:v1.1.0                   |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-snapshotter:v3.0.3               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.1.0     |
+------------------------+-------------------------------------------------------------+
| v1.19.0                | netapp/trident:21.07.0                                      |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-operator:21.07.0                             |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-autosupport:21.01                            |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-provisioner:v2.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-attacher:v3.1.0                  |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-resizer:v1.1.0                   |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-snapshotter:v3.0.3               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.1.0     |
+------------------------+-------------------------------------------------------------+
| v1.20.0                | netapp/trident:21.07.0                                      |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-operator:21.07.0                             |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-autosupport:21.01                            |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-provisioner:v2.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-attacher:v3.1.0                  |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-resizer:v1.1.0                   |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-snapshotter:v4.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.1.0     |
+------------------------+-------------------------------------------------------------+
| v1.21.0                | netapp/trident:21.07.0                                      |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-operator:21.07.0                             |
+------------------------+-------------------------------------------------------------+
|                        | netapp/trident-autosupport:21.01                            |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-provisioner:v2.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-attacher:v3.1.0                  |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-resizer:v1.1.0                   |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-snapshotter:v4.1.1               |
+------------------------+-------------------------------------------------------------+
|                        | k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.1.0     |
+------------------------+-------------------------------------------------------------+

.. Note::

  On Kubernetes version 1.20 and later, use the validated ``k8s.gcr.io/sig-storage/csi-snapshotter:v4.x``
  image if only ``v1`` version is serving ``volumesnapshots.snapshot.storage.k8s.io`` CRD. If the
  ``v1beta1`` version is serving the CRD with/without the ``v1`` version, use the validated
  ``k8s.gcr.io/sig-storage/csi-snapshotter:v3.x`` image.
