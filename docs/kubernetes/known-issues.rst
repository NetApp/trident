Known issues
^^^^^^^^^^^^

This page contains a list of known issues that may be observed when using Trident.

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
* When using a backend across multiple Trident instances, it is recommended
  that each backend configuration file specify a different ``storagePrefix``
  value for ONTAP backends or use a different ``TenantName`` for Element
  backends. Trident cannot detect volumes that other instances of Trident have
  created, and attempting to create an existing volume on either ONTAP or
  Element backends succeeds as Trident treats volume creation as an
  idempotent operation. Thus, if the ``storagePrefix`` or ``TenantName`` does
  not differ, there is a very slim chance to have name collisions for volumes
  created on the same backend.
* ONTAP cannot concurrently provision more than one FlexGroup at a time unless the set of aggregates are
  unique to each provisioning request.
