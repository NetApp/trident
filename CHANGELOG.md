# Change Log

[Releases](https://github.com/NetApp/trident/releases)

## Changes since v18.01.0

**Fixes:**
- **Kubernetes:** Trident no longer emits SCSI bus rescan errors into log

**Enhancements:**

## Changes since v17.10.0

**Fixes:**
- Volume deletion is an idempotent operation with the ontap-nas-economy driver (Issue [#65](https://github.com/NetApp/trident/issues/65)).
- Enabled Trident installation on an EF-series (all-flash) array.
- Fixed an issue where qtrees with names near the 64-character limit could not
  be deleted.
- Enforced fencing of ONTAP aggregates and E-series pools to match config file values.
- Fixed an issue where deleting volumes using the ONTAP SAN driver could leave
  volumes stuck in a partially deleted state.
- Fixed an issue where the ONTAP SAN driver would create junction paths for cloned iSCSI volumes

**Enhancements:**
- Trident can now serve as a Docker Volume Plugin.
- Enabled cloning volumes via a new PVC annotation.
- Added support for Kubernetes 1.8 Storage Classes to set the mount options and reclaim policy of PVs (Issue [#49](https://github.com/NetApp/trident/issues/49)).
- Introduced the fsType parameter in the Kubernetes Storage Class to set the file system type of iSCSI PVs.
- Added CHAP support for SolidFire (Issue [#42](https://github.com/NetApp/trident/issues/42)).
- Decoupled Trident storage pools from volumes to facilitate "vol move" and SVM-DR with ONTAP backends.
- Added support for the etcdv3 API (Issue [#45](https://github.com/NetApp/trident/issues/45)).
- Added the etcd-copy utility to migrate data between etcd clusters.
- Enabled TLS for Trident's etcd client and the etcd-copy utility.
- Added scripts and instructions for setting up an external etcd cluster using
  etcd operator.
- Significantly reduced Trident and Trident-Launcher Docker image sizes.
- Clarified and enhanced storage class pool selection by introducing storagePools and renaming
  requiredStorage to additionalStoragePools.
- Added hostname support to the dataLIF option in the config file for ontap-nas and ontap-nas-economy drivers.
- Added minimum volume size checks to all plugins.
- **Docker:** Improved iSCSI rescan performance for ONTAP SAN and E-series plugins.

## v17.10.0

**Fixes:**
- tridentctl correctly handles larger payloads using chunked encoding.
- Trident installs correctly in a Kubernetes pod with E-series and ONTAP
  SAN.
- Trident allows periods in PVC names (Issue [#40](https://github.com/NetApp/trident/issues/40)).
- Fixed issue where ONTAP NAS volumes were not mountable immediately
  after creation when using load-sharing mirrors for the SVM root
  volume (Issue [#44](https://github.com/NetApp/trident/issues/44)).
- File system type is not set for NFS volumes in the persistent store
  (Issue [#57](https://github.com/NetApp/trident/issues/57)).
- Deprecated the update script.

**Enhancements:**
- Controller serial numbers are reported by the REST interface and
  tridentctl.
- tridentctl logs can display launcher and ephemeral logs, and it can
  create a support archive.
- Added ontap-nas-economy driver (Issue [#2](https://github.com/NetApp/trident/issues/2)).
- Added support for NetApp Volume Encryption to the ONTAP drivers
  (Issue [#3](https://github.com/NetApp/trident/issues/3)).
- Trident installer now works with Kubernetes 1.8.
- tridentctl can detect and use 'oc' in OpenShift environments.

## v17.07.0

**Fixes:**
- Trident and Trident launcher no longer fail if they cannot validate
  the container orchestrator version.
- When running in a pod, the Trident REST interface is no longer
  accessible by default from outside the pod.
- Trident correctly handles updating backends that have volumes
  provisioned using storage classes that no longer exist (Issue [#29](https://github.com/NetApp/trident/issues/29)).
- Installer script correctly creates a new namespace (Issue [#39](https://github.com/NetApp/trident/issues/39)).

**Enhancements:**
- Added support for `storage.k8s.io/v1` storage classes and the default
  storage class introduced in Kubernetes v1.6.0.
- Changed the installer script to support both Kubernetes and OpenShift
  deployments in a uniform manner and to leverage Role-Based Access
  Control (RBAC) authorization for better security.
- Added scripts for uninstalling and updating Trident.
- Added tridentctl CLI tool for managing Trident.
- SolidFire backend configuration file accepts up to four Volume Access
  Group IDs (Issue [#24](https://github.com/NetApp/trident/issues/24)).
- Improved efficiency of ONTAP LUN ID selection.
- Added PVC annotation `trident.netapp.io/blockSize` to specify
  block/sector size for SolidFire backends (Issues [#33](https://github.com/NetApp/trident/issues/33)
  and [#37](https://github.com/NetApp/trident/issues/37)).
- Added PVC annotation `trident.netapp.io/fileSystem` to specify the
  file system type for iSCSI volumes (Issue [#37](https://github.com/NetApp/trident/issues/37)).


## v17.04.0

**Fixes:**

- Trident now rejects ONTAP backends with no aggregates assigned to the
  SVM.
- Trident now allows ONTAP backends even if it cannot read the aggregate
  media type, or if the media type is unknown. However, such backends
  will be ignored for storage classes that require a specific media
  type.
- Trident launcher supports creating the ConfigMap in a non-default
  namespace.

**Enhancements:**

- The improved Trident launcher has a better support for failure
  recovery, error reporting, user arguments, and unit testing.
- Enabled SVM-scoped users for ONTAP backends.
- Switched to using vserver-show-aggr-get-iter API for ONTAP 9.0 and
  later to get aggregate media type.
- Added support for E-Series.
- Upgraded the etcd version to v3.1.3.
- Added release notes (CHANGELOG.md).

## v1.0

- Trident v1.0 provides storage orchestration for Kubernetes, acting as
  an external provisioner for NetApp ONTAP and SolidFire systems.
- Through its REST interface, Trident can provide storage orchestration
  for non-Kubernetes deployments.
