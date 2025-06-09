// Copyright 2025 NetApp, Inc. All Rights Reserved.

// DO NOT EDIT: Auto generated using 'ifacemaker -f ontap_rest.go -s RestClient -i RestClientInterface -p api'
package api

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_ontap/mock_ontap_rest_interface.go github.com/netapp/trident/storage_drivers/ontap/api RestClientInterface

import (
	"context"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/application"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/cluster"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	nvme "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_v_me"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	san "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/snapmirror"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/svm"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// RestClientInterface ...
type RestClientInterface interface {
	ClientConfig() ClientConfig
	SetSVMUUID(svmUUID string)
	SVMUUID() string
	SetSVMName(svmName string)
	SVMName() string
	IsSANOptimized() bool
	IsDisaggregated() bool
	// SupportsFeature returns true if the Ontap version supports the supplied feature
	SupportsFeature(ctx context.Context, feature Feature) bool
	// VolumeList returns the names of all Flexvols whose names match the supplied pattern
	VolumeList(ctx context.Context, pattern string, fields []string) (*storage.VolumeCollectionGetOK, error)
	// VolumeListByAttrs is used to find bucket volumes for nas-eco and san-eco
	VolumeListByAttrs(ctx context.Context, volumeAttrs *Volume, fields []string) (*storage.VolumeCollectionGetOK, error)
	// VolumeCreate creates a volume with the specified options
	// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW
	// -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix
	// -encrypt false
	VolumeCreate(
		ctx context.Context, name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions,
		exportPolicy, securityStyle, tieringPolicy, comment string, qosPolicyGroup QosPolicyGroup, encrypt *bool,
		snapshotReserve int, dpVolume bool,
	) error
	// VolumeExists tests for the existence of a flexvol
	VolumeExists(ctx context.Context, volumeName string) (bool, error)
	// VolumeGetByName gets the flexvol with the specified name
	VolumeGetByName(ctx context.Context, volumeName string, fields []string) (*models.Volume, error)
	// VolumeMount mounts a flexvol at the specified junction
	VolumeMount(ctx context.Context, volumeName, junctionPath string) error
	// VolumeRename changes the name of a flexvol
	VolumeRename(ctx context.Context, volumeName, newVolumeName string) error
	VolumeModifyExportPolicy(ctx context.Context, volumeName, exportPolicyName string) error
	// VolumeSize retrieves the size of the specified flexvol
	VolumeSize(ctx context.Context, volumeName string) (uint64, error)
	// VolumeUsedSize retrieves the used bytes of the specified volume
	VolumeUsedSize(ctx context.Context, volumeName string) (int, error)
	// VolumeSetSize sets the size of the specified flexvol
	VolumeSetSize(ctx context.Context, volumeName, newSize string) error
	VolumeModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error
	// VolumeSetComment sets a flexvol's comment to the supplied value
	// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
	VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) error
	// VolumeSetQosPolicyGroupName sets the QoS Policy Group for volume clones since
	// we can't set adaptive policy groups directly during volume clone creation.
	VolumeSetQosPolicyGroupName(ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup) error
	// VolumeCloneSplitStart starts splitting theflexvol clone
	VolumeCloneSplitStart(ctx context.Context, volumeName string) error
	// VolumeDestroy destroys a flexvol
	VolumeDestroy(ctx context.Context, name string, force bool) error
	// VolumeRecoveryQueuePurge purges the specified volume from the recovery queue
	VolumeRecoveryQueuePurge(ctx context.Context, recoveryQueueVolumeName string) error
	// VolumeRecoveryQueueGetName returns the name of the volume in the recovery queue for the specified volume
	VolumeRecoveryQueueGetName(ctx context.Context, name string) (string, error)

	// ConsistencyGroupCreate creates a consistency group
	ConsistencyGroupCreate(ctx context.Context, cgName string,
		flexVols []string) (*application.ConsistencyGroupCreateAccepted, error)
	// ConsistencyGroupCreateAndWait creates a consistency group and waits on the job to complete
	ConsistencyGroupCreateAndWait(ctx context.Context, cgName string, flexVols []string) error
	// ConsistencyGroupGet returns the consistency group info
	ConsistencyGroupGet(ctx context.Context, cgName string) (*models.ConsistencyGroupResponseInlineRecordsInlineArrayItem, error)
	// ConsistencyGroupDelete deletes the consistency group
	ConsistencyGroupDelete(ctx context.Context, cgName string) error
	// ConsistencyGroupSnapshot creates a snapshot for the consistency group
	ConsistencyGroupSnapshot(ctx context.Context, cgName,
		snapName string) (*application.ConsistencyGroupSnapshotCreateCreated, *application.ConsistencyGroupSnapshotCreateAccepted, error)
	// ConsistencyGroupSnapshotAndWait creates a snapshot for the consistency group and waits on the job to complete
	ConsistencyGroupSnapshotAndWait(ctx context.Context, cgName, snapName string) error

	// SnapshotCreate creates a snapshot
	SnapshotCreate(ctx context.Context, volumeUUID, snapshotName string) (*storage.SnapshotCreateAccepted, error)
	// SnapshotCreateAndWait creates a snapshot and waits on the job to complete
	SnapshotCreateAndWait(ctx context.Context, volumeUUID, snapshotName string) error
	// SnapshotList lists snapshots
	SnapshotList(ctx context.Context, volumeUUID string) (*storage.SnapshotCollectionGetOK, error)
	// SnapshotListByName lists snapshots by name
	SnapshotListByName(ctx context.Context, volumeUUID, snapshotName string) (*storage.SnapshotCollectionGetOK, error)
	// SnapshotGet returns info on the snapshot
	SnapshotGet(ctx context.Context, volumeUUID, snapshotUUID string) (*storage.SnapshotGetOK, error)
	// SnapshotGetByName finds the snapshot by name
	SnapshotGetByName(ctx context.Context, volumeUUID, snapshotName string) (*models.Snapshot, error)
	// SnapshotDelete deletes a snapshot
	SnapshotDelete(ctx context.Context, volumeUUID, snapshotUUID string) (*storage.SnapshotDeleteAccepted, error)
	// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
	SnapshotRestoreVolume(ctx context.Context, snapshotName, volumeName string) error
	// SnapshotRestoreFlexgroup restores a volume to a snapshot as a non-blocking operation
	SnapshotRestoreFlexgroup(ctx context.Context, snapshotName, volumeName string) error
	// VolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
	VolumeModifySnapshotDirectoryAccess(ctx context.Context, volumeName string, enable bool) error
	// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
	VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string, error)
	// VolumeCloneCreate creates a clone
	// see also: https://library.netapp.com/ecmdocs/ECMLP2858435/html/resources/volume.html#creating-a-flexclone-and-specifying-its-properties-using-post
	VolumeCloneCreate(ctx context.Context, cloneName, sourceVolumeName, snapshotName string) (*storage.VolumeCreateAccepted, error)
	// VolumeCloneCreateAsync clones a volume from a snapshot
	VolumeCloneCreateAsync(ctx context.Context, cloneName, sourceVolumeName, snapshot string) error
	// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
	// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
	IscsiInitiatorGetDefaultAuth(ctx context.Context, fields []string) (*san.IscsiCredentialsCollectionGetOK, error)
	// IscsiInterfaceGet returns information about the vserver's  iSCSI interfaces
	IscsiInterfaceGet(ctx context.Context, fields []string) (*san.IscsiServiceCollectionGetOK, error)
	// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
	//
	//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
	//	                          -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
	IscsiInitiatorSetDefaultAuth(ctx context.Context, authType, userName, passphrase, outbountUserName, outboundPassphrase string) error
	// IscsiNodeGetName returns information about the vserver's iSCSI node name
	IscsiNodeGetName(ctx context.Context, fields []string) (*san.IscsiServiceGetOK, error)
	// IgroupCreate creates the specified initiator group
	// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
	IgroupCreate(ctx context.Context, initiatorGroupName, initiatorGroupType, osType string) error
	// IgroupAdd adds an initiator to an initiator group
	// equivalent to filer::> lun igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.
	// debian:01:9031309bbebd
	IgroupAdd(ctx context.Context, initiatorGroupName, initiator string) error
	// IgroupRemove removes an initiator from an initiator group
	IgroupRemove(ctx context.Context, initiatorGroupName, initiator string) error
	// IgroupDestroy destroys an initiator group
	IgroupDestroy(ctx context.Context, initiatorGroupName string) error
	// IgroupList lists initiator groups
	IgroupList(ctx context.Context, pattern string, fields []string) (*san.IgroupCollectionGetOK, error)
	// IgroupGet gets the igroup with the specified uuid
	IgroupGet(ctx context.Context, uuid string) (*san.IgroupGetOK, error)
	// IgroupGetByName gets the igroup with the specified name
	IgroupGetByName(ctx context.Context, initiatorGroupName string, fields []string) (*models.Igroup, error)
	// LunOptions gets the LUN options
	LunOptions(ctx context.Context) (*LunOptionsResult, error)
	// LunCloneCreate creates a LUN clone
	LunCloneCreate(ctx context.Context, lunPath, sourcePath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup) error
	// LunCreate creates a LUN
	LunCreate(ctx context.Context, lunPath string, sizeInBytes int64, osType string, qosPolicyGroup QosPolicyGroup, spaceReserved, spaceAllocated *bool) error
	// LunGet gets the LUN with the specified uuid
	LunGet(ctx context.Context, uuid string) (*san.LunGetOK, error)
	// LunGetByName gets the LUN with the specified name
	LunGetByName(ctx context.Context, name string, fields []string) (*models.Lun, error)
	// LunList finds LUNs with the specified pattern
	LunList(ctx context.Context, pattern string, fields []string) (*san.LunCollectionGetOK, error)
	// LunDelete deletes a LUN
	LunDelete(ctx context.Context, lunUUID string) error
	// LunGetComment gets the comment for a given LUN.
	LunGetComment(ctx context.Context, lunPath string) (string, error)
	// LunSetComment sets the comment for a given LUN.
	LunSetComment(ctx context.Context, lunPath, comment string) error
	// LunGetAttribute gets an attribute by name for a given LUN.
	LunGetAttribute(ctx context.Context, lunPath, attributeName string) (string, error)
	// LunSetAttribute sets the attribute to the provided value for a given LUN.
	LunSetAttribute(ctx context.Context, lunPath, attributeName, attributeValue string) error
	// LunSetQosPolicyGroup sets the QoS Policy Group for a given LUN.
	LunSetQosPolicyGroup(ctx context.Context, lunPath, qosPolicyGroup string) error
	// LunRename changes the name of a LUN
	LunRename(ctx context.Context, lunPath, newLunPath string) error
	// LunMapInfo gets the LUN maping information for the specified LUN
	LunMapInfo(ctx context.Context, initiatorGroupName, lunPath string) (*san.LunMapCollectionGetOK, error)
	// LunUnmap deletes the lun mapping for the given LUN path and igroup
	// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
	LunUnmap(ctx context.Context, initiatorGroupName, lunPath string) error
	// LunMap maps a LUN to an id in an initiator group
	// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
	LunMap(ctx context.Context, initiatorGroupName, lunPath string, lunID int) (*san.LunMapCreateCreated, error)
	// LunMapList equivalent to the following
	// filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
	// filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup *
	// filer::> lun mapping show -vserver iscsi_vs -path *           -igroup trident
	LunMapList(ctx context.Context, initiatorGroupName, lunPath string, fields []string) (*san.LunMapCollectionGetOK, error)
	// LunMapGetReportingNodes
	// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
	LunMapGetReportingNodes(ctx context.Context, initiatorGroupName, lunPath string) ([]string, error)
	// LunSize gets the size for a given LUN.
	LunSize(ctx context.Context, lunPath string) (int, error)
	// LunSetSize sets the size for a given LUN.
	LunSetSize(ctx context.Context, lunPath, newSize string) (uint64, error)
	// NetworkIPInterfacesList lists all IP interfaces
	NetworkIPInterfacesList(ctx context.Context) (*networking.NetworkIPInterfacesGetOK, error)
	NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error)
	NetFcpInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error)
	// JobGet returns the job by ID
	JobGet(ctx context.Context, jobUUID string, fields []string) (*cluster.JobGetOK, error)
	// IsJobFinished lookus up the supplied JobLinkResponse's UUID to see if it's reached a terminal state
	IsJobFinished(ctx context.Context, payload *models.JobLinkResponse) (bool, error)
	// PollJobStatus polls for the ONTAP job to complete, with backoff retry logic
	PollJobStatus(ctx context.Context, payload *models.JobLinkResponse) error
	// AggregateList returns the names of all Aggregates whose names match the supplied pattern
	AggregateList(ctx context.Context, pattern string, fields []string) (*storage.AggregateCollectionGetOK, error)
	// SvmGet gets the volume with the specified uuid
	SvmGet(ctx context.Context, uuid string) (*svm.SvmGetOK, error)
	// SvmList returns the names of all SVMs whose names match the supplied pattern
	SvmList(ctx context.Context, pattern string) (*svm.SvmCollectionGetOK, error)
	// SvmGetByName gets the SVM with the specified name
	SvmGetByName(ctx context.Context, svmName string) (*models.Svm, error)
	SVMGetAggregateNames(ctx context.Context) ([]string, error)
	// ClusterInfo returns information about the cluster
	ClusterInfo(ctx context.Context, fields []string, ignoreUnknownFields bool) (*cluster.ClusterGetOK, error)
	// SystemGetOntapVersion gets the ONTAP version using the credentials, and caches & returns the result.
	SystemGetOntapVersion(ctx context.Context, cached bool) (string, error)
	// NodeList returns information about nodes
	NodeList(ctx context.Context, pattern string) (*cluster.NodesGetOK, error)
	NodeListSerialNumbers(ctx context.Context) ([]string, error)
	// EmsAutosupportLog generates an auto support message with the supplied parameters
	EmsAutosupportLog(
		ctx context.Context, appVersion string, autoSupport bool, category, computerName, eventDescription string, eventID int, eventSource string, logLevel int,
	) error
	TieringPolicyValue(ctx context.Context) string
	// ExportPolicyCreate creates an export policy
	// equivalent to filer::> vserver export-policy create
	ExportPolicyCreate(ctx context.Context, policy string) (*nas.ExportPolicyCreateCreated, error)
	// ExportPolicyGet gets the export policy with the specified uuid
	ExportPolicyGet(ctx context.Context, id int64) (*nas.ExportPolicyGetOK, error)
	// ExportPolicyList returns the names of all export polices whose names match the supplied pattern
	ExportPolicyList(ctx context.Context, pattern string) (*nas.ExportPolicyCollectionGetOK, error)
	// ExportPolicyGetByName gets the volume with the specified name
	ExportPolicyGetByName(ctx context.Context, exportPolicyName string) (*models.ExportPolicy, error)
	ExportPolicyDestroy(ctx context.Context, policy string) (*nas.ExportPolicyDeleteOK, error)
	// ExportRuleList returns the export rules in an export policy
	// equivalent to filer::> vserver export-policy rule show
	ExportRuleList(ctx context.Context, policy string) (*nas.ExportRuleCollectionGetOK, error)
	// ExportRuleCreate creates a rule in an export policy
	// equivalent to filer::> vserver export-policy rule create
	ExportRuleCreate(ctx context.Context, policy, clientMatch string, protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string) (*nas.ExportRuleCreateCreated, error)
	// ExportRuleDestroy deletes the rule at the given index in the given policy
	ExportRuleDestroy(ctx context.Context, policy string, ruleIndex int) (*nas.ExportRuleDeleteOK, error)
	// FlexGroupCreate creates a FlexGroup with the specified options
	// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size
	// -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none
	// -security-style unix -encrypt false
	FlexGroupCreate(
		ctx context.Context, name string, size int, aggrs []string, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
		qosPolicyGroup QosPolicyGroup, encrypt *bool, snapshotReserve int,
	) error
	// FlexgroupCloneSplitStart starts splitting the flexgroup clone
	FlexgroupCloneSplitStart(ctx context.Context, volumeName string) error
	// FlexGroupDestroy destroys a FlexGroup
	FlexGroupDestroy(ctx context.Context, name string, force bool) error
	// FlexGroupExists tests for the existence of a FlexGroup
	FlexGroupExists(ctx context.Context, volumeName string) (bool, error)
	// FlexGroupSize retrieves the size of the specified flexgroup
	FlexGroupSize(ctx context.Context, volumeName string) (uint64, error)
	// FlexGroupUsedSize retrieves the used space of the specified volume
	FlexGroupUsedSize(ctx context.Context, volumeName string) (int, error)
	// FlexGroupSetSize sets the size of the specified FlexGroup
	FlexGroupSetSize(ctx context.Context, volumeName, newSize string) error
	// FlexgroupSetQosPolicyGroupName note: we can't set adaptive policy groups directly during volume clone creation.
	FlexgroupSetQosPolicyGroupName(ctx context.Context, volumeName string, qosPolicyGroup QosPolicyGroup) error
	// FlexGroupVolumeModifySnapshotDirectoryAccess modifies access to the ".snapshot" directory
	FlexGroupVolumeModifySnapshotDirectoryAccess(ctx context.Context, flexGroupVolumeName string, enable bool) error
	FlexGroupModifyUnixPermissions(ctx context.Context, volumeName, unixPermissions string) error
	// FlexGroupSetComment sets a flexgroup's comment to the supplied value
	FlexGroupSetComment(ctx context.Context, volumeName, newVolumeComment string) error
	// FlexGroupGetByName gets the flexgroup with the specified name
	FlexGroupGetByName(ctx context.Context, volumeName string, fields []string) (*models.Volume, error)
	// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
	FlexGroupGetAll(ctx context.Context, pattern string, fields []string) (*storage.VolumeCollectionGetOK, error)
	// FlexGroupMount mounts a flexgroup at the specified junction
	FlexGroupMount(ctx context.Context, volumeName, junctionPath string) error
	// FlexgroupUnmount unmounts the flexgroup
	FlexgroupUnmount(ctx context.Context, volumeName string) error
	FlexgroupModifyExportPolicy(ctx context.Context, volumeName, exportPolicyName string) error
	// QtreeCreate creates a qtree with the specified options
	// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
	QtreeCreate(ctx context.Context, name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string) error
	// QtreeRename renames a qtree
	// equivalent to filer::> volume qtree rename
	QtreeRename(ctx context.Context, path, newPath string) error
	// QtreeDestroyAsync destroys a qtree in the background
	// equivalent to filer::> volume qtree delete -foreground false
	QtreeDestroyAsync(ctx context.Context, path string, force bool) error
	// QtreeList returns the names of all Qtrees whose names match the supplied prefix
	// equivalent to filer::> volume qtree show
	QtreeList(ctx context.Context, prefix, volumePrefix string, fields []string) (*storage.QtreeCollectionGetOK, error)
	// QtreeGetByPath gets the qtree with the specified path
	QtreeGetByPath(ctx context.Context, path string, fields []string) (*models.Qtree, error)
	// QtreeGetByName gets the qtree with the specified name in the specified volume
	QtreeGetByName(ctx context.Context, name, volumeName string) (*models.Qtree, error)
	// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
	QtreeCount(ctx context.Context, volumeName string) (int, error)
	// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
	QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error)
	// QtreeGet returns all relevant details for a single qtree
	// equivalent to filer::> volume qtree show
	QtreeGet(ctx context.Context, name, volumePrefix string) (*models.Qtree, error)
	// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
	// equivalent to filer::> volume qtree show
	QtreeGetAll(ctx context.Context, volumePrefix string) (*storage.QtreeCollectionGetOK, error)
	// QtreeModifyExportPolicy modifies the export policy for the qtree
	QtreeModifyExportPolicy(ctx context.Context, name, volumeName, newExportPolicyName string) error
	// QuotaOn enables quotas on a Flexvol
	// equivalent to filer::> volume quota on
	QuotaOn(ctx context.Context, volumeName string) error
	// QuotaOff disables quotas on a Flexvol
	// equivalent to filer::> volume quota off
	QuotaOff(ctx context.Context, volumeName string) error
	// QuotaSetEntry updates (or creates) a quota rule with an optional hard disk limit
	// equivalent to filer::> volume quota policy rule modify
	QuotaSetEntry(ctx context.Context, qtreeName, volumeName, quotaType, diskLimit string) error
	// QuotaAddEntry creates a quota rule with an optional hard disk limit
	// equivalent to filer::> volume quota policy rule create
	QuotaAddEntry(ctx context.Context, volumeName, qtreeName, quotaType, diskLimit string) error
	// QuotaGetEntry returns the disk limit for a single qtree
	// equivalent to filer::> volume quota policy rule show
	QuotaGetEntry(ctx context.Context, volumeName, qtreeName, quotaType string) (*models.QuotaRule, error)
	// QuotaEntryList returns the disk limit quotas for a Flexvol
	// equivalent to filer::> volume quota policy rule show
	QuotaEntryList(ctx context.Context, volumeName string) (*storage.QuotaRuleCollectionGetOK, error)
	// GetPeeredVservers returns a list of vservers peered with the vserver for this backend
	GetPeeredVservers(ctx context.Context) ([]string, error)
	SnapmirrorRelationshipsList(ctx context.Context) (*snapmirror.SnapmirrorRelationshipsGetOK, error)
	// IsVserverDRDestination identifies if the Vserver is a destination vserver of Snapmirror relationship (SVM-DR) or not
	IsVserverDRDestination(ctx context.Context) (bool, error)
	// IsVserverDRSource identifies if the Vserver is a source vserver of Snapmirror relationship (SVM-DR) or not
	IsVserverDRSource(ctx context.Context) (bool, error)
	// IsVserverInSVMDR identifies if the Vserver is in Snapmirror relationship (SVM-DR) or not
	IsVserverInSVMDR(ctx context.Context) bool
	FcpNodeGetName(ctx context.Context, fields []string) (*san.FcpServiceGetOK, error)
	FcpInterfaceGet(ctx context.Context, fields []string) (*san.FcpServiceCollectionGetOK, error)
	SnapmirrorGet(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string, fields []string) (*models.SnapmirrorRelationship, error)
	SnapmirrorListDestinations(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) (*models.SnapmirrorRelationship, error)
	SnapmirrorCreate(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, repPolicy, repSchedule string) error
	SnapmirrorInitialize(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) error
	SnapmirrorResync(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) error
	SnapmirrorBreak(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName, snapshotName string) error
	SnapmirrorQuiesce(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) error
	SnapmirrorAbort(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) error
	// SnapmirrorRelease removes all local snapmirror relationship metadata from the source vserver
	// Intended to be used on the source vserver
	SnapmirrorRelease(ctx context.Context, sourceFlexvolName, sourceSVMName string) error
	// Intended to be from the destination vserver
	SnapmirrorDeleteViaDestination(ctx context.Context, localFlexvolName, localSVMName string) error
	// Intended to be from the destination vserver
	SnapmirrorDelete(ctx context.Context, localFlexvolName, localSVMName, remoteFlexvolName, remoteSVMName string) error
	IsVserverDRCapable(ctx context.Context) (bool, error)
	SnapmirrorPolicyExists(ctx context.Context, policyName string) (bool, error)
	SnapmirrorPolicyGet(ctx context.Context, policyName string) (*snapmirror.SnapmirrorPoliciesGetOK, error)
	JobScheduleExists(ctx context.Context, jobName string) (bool, error)
	// GetSVMState returns the SVM state from the backend storage.
	GetSVMState(ctx context.Context) (string, error)
	SnapmirrorUpdate(ctx context.Context, localInternalVolumeName, snapshotName string) error
	// SMBShareCreate creates an SMB share with the specified name and path.
	// Equivalent to filer::> vserver cifs share create -share-name <shareName> -path <path>
	SMBShareCreate(ctx context.Context, shareName, path string) error
	// SMBShareExists checks for the existence of an SMB share with the given name.
	// Equivalent to filer::> cifs share show <shareName>
	SMBShareExists(ctx context.Context, smbShareName string) (bool, error)
	// SMBShareDestroy destroys an SMB share.
	// Equivalent to filer::> cifs share delete <shareName>
	SMBShareDestroy(ctx context.Context, shareName string) error
	// SMBShareAccessControlCreate creates an SMB share access control entry for the specified share.
	// Equivalent to filer::> cifs share access-control create -share <shareName> -user-or-group <userOrGroup> -access-type <accessType>
	SMBShareAccessControlCreate(ctx context.Context, shareName string, smbShareACL map[string]string) error
	// SMBShareAccessControlDelete deletes an SMB share access control entry for the specified share.
	// Equivalent to filer::> cifs share access-control delete -share <shareName> -user-or-group <userOrGroup>
	SMBShareAccessControlDelete(ctx context.Context, shareName string, smbShareACL map[string]string) error

	// NVMe Namespace operations
	// NVMeNamespaceCreate creates NVMe namespace in the backend's SVM.
	NVMeNamespaceCreate(ctx context.Context, ns NVMeNamespace) error
	// NVMeNamespaceSetSize updates the namespace size to newSize.
	NVMeNamespaceSetSize(ctx context.Context, nsUUID string, newSize int64) error
	// NVMeNamespaceSetComment sets comment on the namespace.
	NVMeNamespaceSetComment(ctx context.Context, nsUUID, comment string) error
	// NVMeNamespaceSetQosPolicyGroup sets QoS Policy Group on the namespace.
	NVMeNamespaceSetQosPolicyGroup(ctx context.Context, nsUUID string, qosPolicyGroup QosPolicyGroup) error
	// NVMeNamespaceRename renames the namespace to newName.
	NVMeNamespaceRename(ctx context.Context, nsUUID, newName string) error
	// NVMeNamespaceList finds Namespaces with the specified pattern.
	NVMeNamespaceList(ctx context.Context, pattern string, fields []string) (*nvme.NvmeNamespaceCollectionGetOK, error)
	// NVMeNamespaceGetByName gets the Namespace with the specified name.
	NVMeNamespaceGetByName(ctx context.Context, name string, fields []string) (*models.NvmeNamespace, error)
	// NVMeNamespaceDelete deletes NVMe namespace in the backend's SVM.
	NVMeNamespaceDelete(ctx context.Context, nsUUID string) error
	// NVMe Subsystem operations
	// NVMeSubsystemAddNamespace adds namespace to subsystem-map
	NVMeSubsystemAddNamespace(ctx context.Context, subsystemUUID, nsUUID string) error
	// NVMeSubsystemRemoveNamespace removes a namespace from subsystem-map
	NVMeSubsystemRemoveNamespace(ctx context.Context, subsysUUID, nsUUID string) error
	// NVMeIsNamespaceMapped retrives a namespace from subsystem-map
	NVMeIsNamespaceMapped(ctx context.Context, subsysUUID, namespaceUUID string) (bool, error)
	// NVMeNamespaceCount gets the number of namespaces mapped to a subsystem
	NVMeNamespaceCount(ctx context.Context, subsysUUID string) (int64, error)
	// Subsystem operations
	// NVMeSubsystemList returns a list of subsystems seen by the host
	NVMeSubsystemList(ctx context.Context, pattern string, fields []string) (*nvme.NvmeSubsystemCollectionGetOK, error)
	// NVMeSubsystemGetByName gets the subsystem with the specified name
	NVMeSubsystemGetByName(ctx context.Context, subsystemName string, fields []string) (*models.NvmeSubsystem, error)
	// NVMeSubsystemCreate creates a new subsystem
	NVMeSubsystemCreate(ctx context.Context, subsystemName string) (*models.NvmeSubsystem, error)
	// NVMeSubsystemDelete deletes a given subsystem
	NVMeSubsystemDelete(ctx context.Context, subsysUUID string) error
	// NVMeAddHostNqnToSubsystem adds the NQN of the host to the subsystem
	NVMeAddHostNqnToSubsystem(ctx context.Context, hostNQN, subsUUID string) error
	// NVMeRemoveHostFromSubsystem remove the NQN of the host from the subsystem
	NVMeRemoveHostFromSubsystem(ctx context.Context, hostNQN, subsUUID string) error
	// NVMeGetHostsOfSubsystem retuns all the hosts connected to a subsystem
	NVMeGetHostsOfSubsystem(ctx context.Context, subsUUID string) ([]*models.NvmeSubsystemHost, error)
	NVMeNamespaceSize(ctx context.Context, namespacePath string) (int, error)

	StorageUnitGetByName(ctx context.Context, suName string) (*models.StorageUnit, error)
	StorageUnitSnapshotCreateAndWait(ctx context.Context, suUUID, snapshotName string) error
	StorageUnitSnapshotListByName(ctx context.Context, suUUID, snapshotName string) (*san.StorageUnitSnapshotCollectionGetOK, error)
	StorageUnitSnapshotList(ctx context.Context, suUUID string) (*san.StorageUnitSnapshotCollectionGetOK, error)
	StorageUnitSnapshotRestore(ctx context.Context, snapshotName, suUUID string) error
	StorageUnitSnapshotGetByName(ctx context.Context, snapshotName, suUUID string) (*models.StorageUnitSnapshot, error)
	StorageUnitSnapshotDelete(ctx context.Context, suUUID, snapshotUUID string) (*models.JobLinkResponse, error)
	StorageUnitCloneCreate(ctx context.Context, suUUID, cloneName, snapshot string) error
	StorageUnitCloneSplitStart(ctx context.Context, suUUID string) error
	StorageUnitListAllBackedBySnapshot(ctx context.Context, suName, snapshotName string) ([]string, error)
}
