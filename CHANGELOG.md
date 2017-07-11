# Change Log

[Releases](https://github.com/NetApp/trident/releases)

## Changes since v17.04.0

**Fixes:**
- Trident and Trident launcher no longer fail if they cannot validate the
container orchestrator version.
- When running in a pod, the Trident REST interface is no longer accessible
by default from outside the pod.
- Trident correctly handles updating backends that have volumes provisioned
using storage classes that no longer exist
(Issue [#29](https://github.com/NetApp/trident/issues/29)).
- Installer script correctly creates a new namespace
(Issue [#39](https://github.com/NetApp/trident/issues/39)).

**Enhancements:**
- Added support for `storage.k8s.io/v1` storage classes and the default storage
class introduced in Kubernetes v1.6.0.
- Changed the installer script to support both Kubernetes and OpenShift
deployments in a uniform manner and to leverage Role-Based Access Control
(RBAC) authorization for better security.
- Added scripts for uninstalling and updating Trident.
- Added tridentctl CLI tool for managing Trident.
- SolidFire backend configuration file accepts up to four Volume Access Group
IDs (Issue [#24](https://github.com/NetApp/trident/issues/24)).
- Improved efficiency of ONTAP LUN ID selection.
- Added PVC annotation `trident.netapp.io/blockSize` to specify block/sector
size for SolidFire backends (Issues [#33](https://github.com/NetApp/trident/issues/33)
and [#37](https://github.com/NetApp/trident/issues/37)).
- Added PVC annotation `trident.netapp.io/fileSystem` to specify the file
system type for iSCSI volumes (Issue [#37](https://github.com/NetApp/trident/issues/37)).


## v17.04.0

**Fixes:**

- Trident now rejects ONTAP backends with no aggregates assigned to the SVM.
- Trident now allows ONTAP backends even if it cannot read the aggregate media type,
or if the media type is unknown. However, such backends will be ignored for storage
classes that require a specific media type.
- Trident launcher supports creating the ConfigMap in a non-default namespace.

**Enhancements:**

- The improved Trident launcher has a better support for failure recovery, error
reporting, user arguments, and unit testing.
- Enabled SVM-scoped users for ONTAP backends.
- Switched to using vserver-show-aggr-get-iter API for ONTAP 9.0 and later to get
aggregate media type.
- Added support for E-Series.
- Upgraded the etcd version to v3.1.3.
- Added release notes (CHANGELOG.md).

## v1.0

- Trident v1.0 provides storage orchestration for Kubernetes, acting as an
external provisioner for NetApp ONTAP and SolidFire systems.
- Through its REST interface, Trident can provide storage orchestration for
non-Kubernetes deployments.
