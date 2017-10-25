************
Getting help
************

Known issues
============
Trident is in an early stage of development, and thus, there are several
outstanding issues to be aware of when using it:

* Due to known issues in Kubernetes 1.5 (or OpenShift 3.5) and earlier, use of
  iSCSI with ONTAP or E-Series in production deployments is not recommended.
  See Kubernetes issues
  `#40941 <https://github.com/kubernetes/kubernetes/issues/40941>`_,
  `#41041 <https://github.com/kubernetes/kubernetes/issues/41041>`_ and
  `#39202 <https://github.com/kubernetes/kubernetes/issues/39202>`_. ONTAP NAS
  and SolidFire are unaffected. These issues are fixed in Kubernetes 1.6 (and
  OpenShift 3.6).
* Although we provide a deployment for Trident, it should never be scaled
  beyond a single replica.  Similarly, only one instance of Trident should be
  run per Kubernetes cluster. Trident cannot communicate with other instances
  and cannot discover other volumes that they have created, which will lead to
  unexpected and incorrect behavior if more than one instance runs within a
  cluster.
* Volumes and storage classes created in the REST API will not have
  corresponding objects (PVCs or ``StorageClasses``) created in Kubernetes;
  however, storage classes created via ``tridentctl`` or the REST API will be
  usable by PVCs created in Kubernetes.
* If Trident-based ``StorageClass`` objects are deleted from Kubernetes while
  Trident is offline, Trident will not remove the corresponding storage classes
  from its database when it comes back online. Any such storage classes must
  be deleted manually using ``tridentctl`` or the REST API.
* If a user deletes a PV provisioned by Trident before deleting the
  corresponding PVC, Trident will not automatically delete the backing volume.
  In this case, the user must remove the volume manually via ``tridentctl`` or
  the REST API.
* Trident will not boot unless it can successfully communicate with an etcd
  instance. If it loses communication with etcd during the bootstrap process,
  it will halt; communication must be restored before Trident will come online.
  Once fully booted, however, Trident is resilient to etcd outages.
* When using a backend across multiple Trident instances, it is recommended
  that each backend configuration file specify a different ``storagePrefix``
  value for ONTAP backends or use a different ``TenantName`` for SolidFire
  backends. Trident cannot detect volumes that other instances of Trident have
  created, and attempting to create an existing volume on either ONTAP or
  SolidFire backends succeeds as Trident treats volume creation as an
  idempotent operation. Thus, if the ``storagePrefix`` or ``TenantName`` does
  not differ, there is a very slim chance to have name collisions for volumes
  created on the same backend.

Troubleshooting
===============
* Run ``tridentctl logs``. Trident incorporates a multi-phase installation
  process that involves multiple pods, and the Trident pod itself includes
  multiple containers, all of which have their own logs. When something goes
  wrong, this is the best way to quickly examine all of the relevant logs.
* If there's not enough information in the logs, you can try enabling debug
  mode for Trident and Trident launcher by passing the ``-d`` flag to the
  install script: ``./install_trident.sh -d -n trident``.
* The :ref:`uninstall script <Uninstalling Trident>` can help with cleaning up
  after a failed run. By default the script does not touch the etcd backing
  store, making it safe to uninstall and install again even in a running
  deployment.
* If service accounts are not available, the logs will report an error that
  ``/var/run/secrets/kubernetes.io/serviceaccount/token`` does not exist.  In
  this case, you will either need to enable service accounts or connect to the
  API server by specifying the insecure address and port on the command line.

Support from NetApp
===================
Trident is an officially supported NetApp project. That means you can reach out
to NetApp using any of the `standard mechanisms`_ and get the enterprise grade
support that you need.

.. _standard mechanisms: http://mysupport.netapp.com/info/web/ECMLP2619434.html

There is also a vibrant public community of container users (including Trident
developers) on the #containers channel in `NetApp's Slack team`_. This is a
great place to ask general questions about the project and discuss related
topics with like-minded peers.

.. _NetApp's Slack team: http://netapp.io/slack
