# Change Log

[Releases](https://github.com/NetApp/trident/releases)

## Changes since v20.01.0

**IMPORTANT**: If you are upgrading from Trident 19.07 or 19.10 please carefully read [this](https://netapp-trident.readthedocs.io/en/stable-v20.01/kubernetes/upgrading.html).

**Fixes:**
- **Kubernetes** Change Trident node server to use downward API instead of relying on Kubernetes DNS to find Trident service. (Issue [#328](https://github.com/NetApp/trident/issues/328))
- Fixes FlexGroup volume deletion against ONTAP version 9.7 (Issue [#326](https://github.com/NetApp/trident/issues/326))
- Refer to backend storage for current volume size during resize

**Enhancements**
- Updated to GoLang 1.14.

## v20.01.0

**IMPORTANT**: If you are upgrading from Trident 19.07 or 19.10 please carefully read [this](https://netapp-trident.readthedocs.io/en/stable-v20.01/kubernetes/upgrading.html).

**Fixes:**
- **Kubernetes:** Updated CSI sidecars to address CVE-2019-11255.
- Set default SVM-DR tiering to `snapshot-only` for ONTAP cluster version 9.4 or less. (Issue [#318](https://github.com/NetApp/trident/issues/318))

**Enhancements:**
- **Kubernetes:** Added support for Kubernetes 1.17. (Issue [#327](https://github.com/NetApp/trident/issues/327))
- **Kubernetes:** Added support for IPv6. (Issue [#122](https://github.com/NetApp/trident/issues/122))
- **Kubernetes:** Added support for Prometheus metrics. (Issue [#121](https://github.com/NetApp/trident/issues/121))
- Switched from glide to go modules for dependency management.
- ONTAP drivers now support virtual pools.
- **Kubernetes:** Added `--image-registry` switch to installer. (Issue [#311](https://github.com/NetApp/trident/issues/311))
- **Kubernetes:** Added `--kubelet-dir` switch to installer to simplify installation on some Kubernetes distributions. (Issue [#314](https://github.com/NetApp/trident/issues/314))
- Added support for ONTAP tiering policy in backend config file (Issue [#199](https://github.com/NetApp/trident/issues/199))
- **Kubernetes:** Added support for `v1beta1` Kubernetes snapshots in Kubernetes 1.17.

**Deprecations:**
- **Kubernetes:** Removed support for `v1alpha1` Kubernetes snapshots.

**Known Issues:**
- Flexgroup driver does not work properly with ONTAP 9.7

## v19.10.0

**Fixes:**
- **Kubernetes:** Added fix to ensure Trident pods only run on amd64/linux nodes. (Issue [#264](https://github.com/NetApp/trident/issues/264))
- **Kubernetes:** Reduced log verbosity in CSI sidecars. (Issue [#275](https://github.com/NetApp/trident/issues/275))
- **Kubernetes:** Added fix for volume names longer than 64 characters in solidfire and ontap-nas-economy drivers.  (Issue [#260](https://github.com/NetApp/trident/issues/260), Issue [#273](https://github.com/NetApp/trident/issues/273))
- **Kubernetes:** Node now retries registration with controller indefinitely (Issue [#283](https://github.com/NetApp/trident/issues/283))
- **Kubernetes:** Fixed a panic when adding a storage backend fails.
- **Kubernetes:** Fixed Azure NetApp Files to work with non-CSI deployments. (Issue [#274](https://github.com/NetApp/trident/issues/274))
- Worked around a breaking API change in NetApp Cloud Volumes Service in AWS. (Issue [#288](https://github.com/NetApp/trident/issues/288))
- Fixed NFS 4.1 access denied issue in ontap-nas-economy driver (Issue [#256](https://github.com/NetApp/trident/issues/256))
- Disabled FabricPool tiering for ONTAP volumes created by Trident. (Issue [#199](https://github.com/NetApp/trident/issues/199))
- Fixed bug when IFace is not set in the Element backend config. (Issue [#272](https://github.com/NetApp/trident/issues/272))

**Enhancements:**
- **Kubernetes:** Added support to CSI Trident for volume expansion for iSCSI PVs.
- **Kubernetes:** Added unsupported tridentctl for MacOS. (Issue [#167](https://github.com/NetApp/trident/issues/167))
- **Kubernetes:** Added support to CSI Trident for raw block volumes with multi-attach for iSCSI PVs.
- **Kubernetes:** Added support for Kubernetes 1.16 and OpenShift 4.2.
- **Kubernetes:** Made installer setup directory optional and relative to working directory. (Issue [#230](https://github.com/NetApp/trident/issues/230))
- **Kubernetes:** Support volume cloning using a PVC as the source.
- **Kubernetes:** Added enhancements to 'tridentctl logs' command for CSI mode.
- Added HTTP proxy support for NetApp Cloud Volumes Service in AWS driver. (Issue [#246](https://github.com/NetApp/trident/issues/246))
- Added snapshotDir option to NetApp Cloud Volumes Service in AWS driver.
- Added driver for NetApp Cloud Volumes Service in Google Cloud Platform.
- Added option for JSON-formatted logging. (Issue [#286](https://github.com/NetApp/trident/issues/286))

**Deprecations:**
- **Kubernetes:** Removed 'dry-run' switch from the installer. (Issue [#192](https://github.com/NetApp/trident/issues/192))
- Changed minimum supported ONTAP version to 9.1.
- Removed support for running Trident with an external etcd instance.
  
## v19.07.0

**Fixes:**
- **Kubernetes:** Improved volume import transaction cleanup during failure scenarios.
- **Kubernetes:** Fix unknown backend states after Trident upgrade.
- **Kubernetes:** Prevent operations on failed backends. 
- **Kubernetes:** Removed size requirement for volume import PVC file.

**Enhancements:**
- Trident driver for Azure NetApp Files.
- **Kubernetes:** Implemented CSI Trident (optional for Kubernetes 1.13, exclusive for Kubernetes 1.14+).
- **Kubernetes:** Added support to CSI Trident for volume snapshots.
- **Kubernetes:** Converted Trident to use custom resource definitions instead of etcd.
- **Kubernetes:** Added support for Kubernetes 1.15.
- **Kubernetes:** CSI Trident only supports CHAP authentication for Element backends.
- Trident now allows Solidfire backends without `Types` defined. However, such backends
  will have one default storage pool with the default backend QoS values.
- Added CONTRIBUTING.md file to describe the process for contributing changes to Trident.
- **Behavioral change:** Enabled space-allocation feature for ONTAP SAN LUNs by default. Setting `spaceAllocation`
  parameter to `false` in ONTAP SAN backend's default config section would disable the space-allocation feature for
  those LUNs.
- **Kubernetes:** Fix failure to set snapshot directory access during FlexGroup creation.

## v19.04.0

**Fixes:**
- Fixed panic if no aggregates are assigned to an ONTAP SVM.
- **Kubernetes:** Updated CSI driver for 1.0 spec and Kubernetes 1.13. (Alpha release - unsupported)
- **Kubernetes:** Allow Trident to start if one or more backend drivers fail to initialize.
- **Kubernetes:** Fixed Trident to install on Kubectl 1.14. (Issue [#241](https://github.com/NetApp/trident/issues/241))

**Enhancements:**
- Trident driver for NetApp Cloud Volumes Service in AWS.
- **Kubernetes:** Import pre-existing volumes using the `ontap-nas`, `ontap-nas-flexgroup`, `solidfire-san`, and `aws-cvs` drivers.  (Issue [#74](https://github.com/NetApp/trident/issues/74))
- **Kubernetes:** Added support for Kubernetes 1.14.
- **Kubernetes:** Updated etcd to v3.3.12.

## v19.01.0

**Fixes:**
- Fixed an issue where Trident did not allow specifying a port in the management LIF config (Issue [#195](https://github.com/NetApp/trident/issues/195)). Thank you, [@vnandha!](https://github.com/vnandha)
- Only strip prefix on volume name if volume name starts with prefix.
- **Kubernetes:** Refactored BackendExternal which fixed the output of "tridentctl get backend -o json" where details like "limitAggregateUsage" and "limitVolumeSize" were not found. (Issue [#188](https://github.com/NetApp/trident/issues/188))

**Enhancements:**
- Updated Trident's 3rd-party dependencies for 19.01 release.
- Added support for Docker 18.09, Docker Enterprise Edition 2.1, OpenShift 3.11 and Kubernetes 1.13.
- Removed support for Docker Enterprise Edition 2.0 and Kubernetes 1.8.
- **Kubernetes:** Added support for raw block volumes for iSCSI PVs.
- **Kubernetes:** Added retry logic to installer for Kubernetes object creation.
- **Kubernetes:** Updated etcd to v3.3.10 and client-go to v10.0.0.
- **Kubernetes:** Trident now honors the nfsMountOptions parameter in ONTAP NAS backend config files.
- **Behavioural change:** The Trident installer now automatically adds the backend used to provision the Trident volume in new installations.

**Deprecations:**
- **Kubernetes:** Deprecated PVC annotation `trident.netapp.io/reclaimPolicy` as the reclaim policy can be set in the
  storage class since Kubernetes v1.8.

## v18.10.0

**Fixes:**
- Modified log messages about ONTAP media types not associated with performance classes (Issue [#158](https://github.com/NetApp/trident/issues/158)).
- **Docker:** Resolved issue where containers might not restart after restarting Docker (Issue [#160](https://github.com/NetApp/trident/issues/160)).

**Enhancements:**
- Added ability to set snapshotReserve in backend config files, volume creation options, and PVC annotations (Issue [#43](https://github.com/NetApp/trident/issues/43)).
- Added ability to limit the size of requested volumes.
- Added ability to limit the ONTAP Aggregate usage percentage (Issue [#64](https://github.com/NetApp/trident/issues/64)).
- Added ability to limit the ONTAP Flexvol size for the ontap-nas-economy driver (Issue [#141](https://github.com/NetApp/trident/issues/141)).
- **Kubernetes:** Added support for expanding NFS Persistent Volumes (Issue [#21](https://github.com/NetApp/trident/issues/21)).
- **Kubernetes:** Modified Trident installer to do most of its work in a pod.
- **Kubernetes:** Updated etcd to v3.3.9 and client-go to v9.0.0.

**Deprecations:**
- **Docker:** Trident's support for Docker EE 2.0's UCP access control will be removed in the
  v19.01 release, replaced by the native Kubernetes access control support in
  Docker EE 2.1 and beyond. The `--ucp-host` and `--ucp-bearer-token` parameters
  will be deprecated and will not be needed in order to install or uninstall Trident.

## v18.07.0

**Fixes:**
- Fixed cleanup of the transaction object upon failed deletion (Issue [#126](https://github.com/NetApp/trident/issues/126)).
- **Kubernetes:** Fixed an issue where Trident would provision a ReadWriteMany PVC to an iSCSI backend.
- **Kubernetes:** Fixed an installer issue where the Trident PV could be bound to the wrong PVC (Issue [#120](https://github.com/NetApp/trident/issues/120)).
- **Kubernetes:** Fixed an issue where the Trident installer could fail if a default storage class is set (Issue [#120](https://github.com/NetApp/trident/issues/120)).
- **Kubernetes:** Fixed an issue where the Trident installer could fail to delete objects on some Kubernetes versions (Issue [#132](https://github.com/NetApp/trident/issues/132)).
- **Docker:** Fixed an issue where deleted qtrees could appear in Docker volume list.
- **Docker:** Fixed plugin restart issue for newer versions of Docker (Issue [#129](https://github.com/NetApp/trident/issues/129)).
- **Docker:** Trident no longer crashes if ONTAP volumes are offline (Issue [#151](https://github.com/NetApp/trident/issues/151)).

**Enhancements:**
- Added ontap-nas-flexgroup driver to support ONTAP FlexGroup Volumes.
- Changed ONTAP drivers so that the snapshot reserve is set to zero when snapshotPolicy is "none".
- Added support for Docker EE UCP.
- **Kubernetes:** Added regex matching to storage pool selection as well as a new excludeStoragePools option for Kubernetes storage classes (Issue [#118](https://github.com/NetApp/trident/issues/118)).
- **Kubernetes:** Added support for iSCSI multipath (Issue [#69](https://github.com/NetApp/trident/issues/69)).
- **Kubernetes:** Updated etcd to v3.2.19 and client-go to v7.0.0.
- **Kubernetes:** Added --previous switch to 'tridentctl logs' command.
- **Kubernetes:** Added liveness probes to the trident-main and etcd containers.
- **Kubernetes:** Added --trident-image and --etcd-image switches to 'tridentctl install' command.
- **Kubernetes:** Added prototype CSI implementation to Trident.

## v18.04.0

**Fixes:**
- Clone operations are more resilient to busy storage controllers.
- Fixed cleanup of goroutines.
- Extended timeouts for storage controller API invocations.
- Prevented exit if a startup error occurs so Docker and Kubernetes don't restart Trident continuously.
- Fixed an issue where SolidFire cloned volumes were not updated with new QoS values.
- **Kubernetes:** Trident no longer emits SCSI bus rescan errors into log.
- **Kubernetes:** Fixed incorrect association of volumes with backends after backend update (Issue [#111](https://github.com/NetApp/trident/issues/111)).
- **Docker:** iSCSI device discovery and removal is faster, more granular, and more reliable.
- **Docker:** Fixed default size handling (Issue [#102](https://github.com/NetApp/trident/issues/102)).
- **Docker:** Client interfaces start immediately to avoid Docker plugin timeouts.

**Enhancements:**
- Added FQDN support for the management and data LIF of ONTAP backends.
- For Kubernetes 1.9+, CHAP secrets will be created in Trident's namespace
  instead of the PVC's namespace.
- Return new HTTP codes from REST interface to indicate Trident startup status.
- Set the minimum supported SolidFire Element version to 8.0.
- **Kubernetes:** Simplified installing Trident with a new installer.
- **Kubernetes:** Added the ability to define a custom name for a storage
  backend. This enhancement enables adding multiple instances of the same
  backend with different policies (e.g., different snapshot policies), which
  obviates extending the Trident storage class API to support new parameters
  (Issue [#93](https://github.com/NetApp/trident/issues/93)).
- **Kubernetes:** Added the ability to rename an existing backend.
- **Kubernetes:** SolidFire defaults to use CHAP if Kubernetes version is >= 1.7
  and a `trident` access group doesn't exist. Setting AccessGroup or UseCHAP in
  config overrides this behavior.
- **Docker:** The aggregate attribute in ONTAP config files is now optional.

## v18.01.0

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
