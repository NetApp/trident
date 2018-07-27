Known issues
^^^^^^^^^^^^

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
* Due to a known issue in the containerized version of OpenShift, where some
  iSCSI initiator utilities are missing, Trident will fail to install of configure
  iSCSI storage systems. This only affects OpenShift Origin.
  See OpenShift issues
  `#18880 <https://github.com/openshift/origin/issues/18880>`_.
* ONTAP cannot concurrently provision more than one FlexGroup at a time unless the set of aggregates are
  unique to each provisioning request.
