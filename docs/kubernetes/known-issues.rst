Known issues
^^^^^^^^^^^^

This page contains a list of known issues that may be observed when using Trident.

* When installing Trident (using ``tridentctl`` or the Trident Operator) and
  using ``tridentctl`` to manage Trident, you must ensure the
  ``KUBECONFIG`` environment variable is set. This is necessary to indicate
  the Kubernetes cluster that ``tridentctl`` must work against. When working
  with multiple Kubernetes environments, care must be taken to ensure the
  KUBECONFIG file is sourced accurately.
* A `bug <https://github.com/NetApp/trident/issues/404>`_ has been identified in
  ``20.04`` with bidirectional CHAP using the
  ``ontap-san`` and ``ontap-san-economy`` drivers. You can still use Trident to
  connect with unidirectional CHAP. This requires defining all 4 CHAP config
  parameters (``chapUsername``, ``chapInitiatorSecret``,
  ``chapTargetInitiatorSecret``, and ``chapTargetUsername``). Trident will only
  use the initiator parameters to authenticate connections over unidirectional
  CHAP. **This bug has been fixed with the 20.07 release of Trident**.
* To perform online space reclamation for iSCSI PVs, the underlying OS on the
  worker node may require mount options to be passed to the volume. This is
  true for RHEL/RedHat CoreOS instances, which require the ``discard``
  `mount option <https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/managing_file_systems/discarding-unused-blocks_managing-file-systems>`_;
  ensure the ``discard`` mountOption is included in your
  `StorageClass <https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options>`_
  to support online block discard.
* Although we provide a deployment for Trident, it should never be scaled
  beyond a single replica.  Similarly, only one instance of Trident should be
  run per Kubernetes cluster. Trident cannot communicate with other instances
  and cannot discover other volumes that they have created, which will lead to
  unexpected and incorrect behavior if more than one instance runs within a
  cluster.
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
* ONTAP cannot concurrently provision more than one FlexGroup at a time
  unless the set of aggregates are unique to each provisioning request.
* When using Trident over IPv6, the ``managementLIF`` and ``dataLIF`` in the backend definition
  must be specified within square brackets, like ``[fd20:8b1e:b258:2000:f816:3eff:feec:0]``.
* If using CoreOS or Ubuntu on Kubernetes nodes, you must ensure ``rpc-statd`` is started
  at boot time.
