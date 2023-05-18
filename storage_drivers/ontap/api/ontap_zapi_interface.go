// Copyright 2023 NetApp, Inc. All Rights Reserved.

// DO NOT EDIT: Auto generated using 'ifacemaker -f ontap_zapi.go -s Client -i ZapiClientInterface -p api > ontap_zapi_interface.go'

package api

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_ontap/mock_ontap_zapi_interface.go github.com/netapp/trident/storage_drivers/ontap/api ZapiClientInterface

import (
	"context"
	"time"

	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

// ZapiClientInterface ...
type ZapiClientInterface interface {
	ClientConfig() ClientConfig
	SetSVMUUID(svmUUID string)
	SVMUUID() string
	SetSVMMCC(mcc bool)
	SVMMCC() bool
	SVMName() string
	// GetSVMState gets the latest state of the associated vserver
	GetSVMState(ctx context.Context) (string, error)
	// GetClonedZapiRunner returns a clone of the ZapiRunner configured on this driver.
	GetClonedZapiRunner() *azgo.ZapiRunner
	// GetNontunneledZapiRunner returns a clone of the ZapiRunner configured on this driver with the SVM field cleared so ZAPI calls
	// made with the resulting runner aren't tunneled.  Note that the calls could still go directly to either a cluster or
	// vserver management LIF.
	GetNontunneledZapiRunner() *azgo.ZapiRunner
	// SupportsFeature returns true if the Ontapi version supports the supplied feature
	SupportsFeature(ctx context.Context, feature Feature) bool
	// IgroupCreate creates the specified initiator group
	// equivalent to filer::> igroup create docker -vserver iscsi_vs -protocol iscsi -ostype linux
	IgroupCreate(initiatorGroupName, initiatorGroupType, osType string) (*azgo.IgroupCreateResponse, error)
	// IgroupAdd adds an initiator to an initiator group
	// equivalent to filer::> igroup add -vserver iscsi_vs -igroup docker -initiator iqn.1993-08.org.debian:01:9031309bbebd
	IgroupAdd(initiatorGroupName, initiator string) (*azgo.IgroupAddResponse, error)
	// IgroupRemove removes an initiator from an initiator group
	IgroupRemove(initiatorGroupName, initiator string, force bool) (*azgo.IgroupRemoveResponse, error)
	// IgroupDestroy destroys an initiator group
	IgroupDestroy(initiatorGroupName string) (*azgo.IgroupDestroyResponse, error)
	// IgroupList lists initiator groups
	IgroupList() (*azgo.IgroupGetIterResponse, error)
	// IgroupGet gets a specified initiator group
	IgroupGet(initiatorGroupName string) (*azgo.InitiatorGroupInfoType, error)
	// LunCreate creates a lun with the specified attributes
	// equivalent to filer::> lun create -vserver iscsi_vs -path /vol/v/lun1 -size 1g -ostype linux -space-reserve disabled -space-allocation enabled
	LunCreate(
		lunPath string, sizeInBytes int, osType string, qosPolicyGroup QosPolicyGroup,
		spaceReserved, spaceAllocated bool,
	) (*azgo.LunCreateBySizeResponse, error)
	// LunCloneCreate clones a LUN from a snapshot
	LunCloneCreate(
		volumeName, sourceLun, destinationLun string, qosPolicyGroup QosPolicyGroup,
	) (*azgo.CloneCreateResponse, error)
	// LunSetQosPolicyGroup sets the qos policy group or adaptive qos policy group on a lun; does not unset policy groups
	LunSetQosPolicyGroup(lunPath string, qosPolicyGroup QosPolicyGroup) (*azgo.LunSetQosPolicyGroupResponse, error)
	// LunGetSerialNumber returns the serial# for a lun
	LunGetSerialNumber(lunPath string) (*azgo.LunGetSerialNumberResponse, error)
	// LunMapsGetByLun returns a list of LUN map details for a given LUN path
	// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0
	LunMapsGetByLun(lunPath string) (*azgo.LunMapGetIterResponse, error)
	// LunMapsGetByIgroup returns a list of LUN map details for a given igroup
	// equivalent to filer::> lun mapping show -vserver iscsi_vs -igroup trident
	LunMapsGetByIgroup(initiatorGroupName string) (*azgo.LunMapGetIterResponse, error)
	// LunMapGet returns a list of LUN map details
	// equivalent to filer::> lun mapping show -vserver iscsi_vs -path /vol/v/lun0 -igroup trident
	LunMapGet(initiatorGroupName, lunPath string) (*azgo.LunMapGetIterResponse, error)
	// LunMap maps a lun to an id in an initiator group
	// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker -lun-id 0
	LunMap(initiatorGroupName, lunPath string, lunID int) (*azgo.LunMapResponse, error)
	// LunMapAutoID maps a LUN in an initiator group, allowing ONTAP to choose an available LUN ID
	// equivalent to filer::> lun map -vserver iscsi_vs -path /vol/v/lun1 -igroup docker
	LunMapAutoID(initiatorGroupName, lunPath string) (*azgo.LunMapResponse, error)
	LunMapIfNotMapped(ctx context.Context, initiatorGroupName, lunPath string) (int, error)
	// LunMapListInfo returns lun mapping information for the specified lun
	// equivalent to filer::> lun mapped show -vserver iscsi_vs -path /vol/v/lun0
	LunMapListInfo(lunPath string) (*azgo.LunMapListInfoResponse, error)
	// LunOffline offlines a lun
	// equivalent to filer::> lun offline -vserver iscsi_vs -path /vol/v/lun0
	LunOffline(lunPath string) (*azgo.LunOfflineResponse, error)
	// LunOnline onlines a lun
	// equivalent to filer::> lun online -vserver iscsi_vs -path /vol/v/lun0
	LunOnline(lunPath string) (*azgo.LunOnlineResponse, error)
	// LunDestroy destroys a LUN
	// equivalent to filer::> lun destroy -vserver iscsi_vs -path /vol/v/lun0
	LunDestroy(lunPath string) (*azgo.LunDestroyResponse, error)
	// LunSetAttribute sets a named attribute for a given LUN.
	LunSetAttribute(lunPath, name, value string) (*azgo.LunSetAttributeResponse, error)
	// LunGetAttribute gets a named attribute for a given LUN.
	LunGetAttribute(ctx context.Context, lunPath, name string) (string, error)
	// LunGetComment gets the comment for a given LUN.
	LunGetComment(ctx context.Context, lunPath string) (string, error)
	// LunGet returns all relevant details for a single LUN
	// equivalent to filer::> lun show
	LunGet(path string) (*azgo.LunInfoType, error)
	LunGetGeometry(path string) (*azgo.LunGetGeometryResponse, error)
	LunResize(path string, sizeBytes int) (uint64, error)
	// LunGetAll returns all relevant details for all LUNs whose paths match the supplied pattern
	// equivalent to filer::> lun show -path /vol/trident_*/*
	LunGetAll(pathPattern string) (*azgo.LunGetIterResponse, error)
	// LunGetAllForVolume returns all relevant details for all LUNs in the supplied Volume
	// equivalent to filer::> lun show -volume trident_CEwDWXQRPz
	LunGetAllForVolume(volumeName string) (*azgo.LunGetIterResponse, error)
	// LunGetAllForVserver returns all relevant details for all LUNs in the supplied SVM
	// equivalent to filer::> lun show -vserver trident_CEwDWXQRPz
	LunGetAllForVserver(vserverName string) (*azgo.LunGetIterResponse, error)
	// LunCount returns the number of LUNs that exist in a given volume
	LunCount(ctx context.Context, volume string) (int, error)
	// LunRename changes the name of a LUN
	LunRename(path, newPath string) (*azgo.LunMoveResponse, error)
	// LunUnmap deletes the lun mapping for the given LUN path and igroup
	// equivalent to filer::> lun mapping delete -vserver iscsi_vs -path /vol/v/lun0 -igroup group
	LunUnmap(initiatorGroupName, lunPath string) (*azgo.LunUnmapResponse, error)
	// LunSize retrieves the size of the specified volume, does not work with economy driver
	LunSize(flexvolName string) (int, error)
	// FlexGroupCreate creates a FlexGroup with the specified options
	// equivalent to filer::> volume create -vserver svm_name -volume fg_vol_name –auto-provision-as flexgroup -size fg_size  -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
	FlexGroupCreate(
		ctx context.Context, name string, size int, aggrs []azgo.AggrNameType,
		spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
		qosPolicyGroup QosPolicyGroup, encrypt *bool, snapshotReserve int,
	) (*azgo.VolumeCreateAsyncResponse, error)
	// FlexGroupDestroy destroys a FlexGroup
	FlexGroupDestroy(ctx context.Context, name string, force bool) (*azgo.VolumeDestroyAsyncResponse, error)
	// FlexGroupExists tests for the existence of a FlexGroup
	FlexGroupExists(ctx context.Context, name string) (bool, error)
	// FlexGroupUsedSize retrieves the used space of the specified volume
	FlexGroupUsedSize(name string) (int, error)
	// FlexGroupSize retrieves the size of the specified volume
	FlexGroupSize(name string) (int, error)
	// FlexGroupSetSize sets the size of the specified FlexGroup
	FlexGroupSetSize(ctx context.Context, name, newSize string) (*azgo.VolumeSizeAsyncResponse, error)
	// FlexGroupVolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	FlexGroupVolumeDisableSnapshotDirectoryAccess(
		ctx context.Context, name string,
	) (*azgo.VolumeModifyIterAsyncResponse, error)
	FlexGroupModifyUnixPermissions(
		ctx context.Context, volumeName, unixPermissions string,
	) (*azgo.VolumeModifyIterAsyncResponse, error)
	// FlexGroupSetComment sets a flexgroup's comment to the supplied value
	FlexGroupSetComment(ctx context.Context, volumeName, newVolumeComment string) (*azgo.VolumeModifyIterAsyncResponse,
		error)
	// FlexGroupGet returns all relevant details for a single FlexGroup
	FlexGroupGet(name string) (*azgo.VolumeAttributesType, error)
	// FlexGroupGetAll returns all relevant details for all FlexGroups whose names match the supplied prefix
	FlexGroupGetAll(prefix string) (*azgo.VolumeGetIterResponse, error)
	// WaitForAsyncResponse handles waiting for an AsyncResponse to return successfully or return an error.
	WaitForAsyncResponse(ctx context.Context, zapiResult interface{}, maxWaitTime time.Duration) error
	// JobGetIterStatus returns the current job status for Async requests.
	JobGetIterStatus(jobId int) (*azgo.JobGetIterResponse, error)
	// VolumeCreate creates a volume with the specified options
	// equivalent to filer::> volume create -vserver iscsi_vs -volume v -aggregate aggr1 -size 1g -state online -type RW -policy default -unix-permissions ---rwxr-xr-x -space-guarantee none -snapshot-policy none -security-style unix -encrypt false
	VolumeCreate(
		ctx context.Context,
		name, aggregateName, size, spaceReserve, snapshotPolicy, unixPermissions, exportPolicy, securityStyle, tieringPolicy, comment string,
		qosPolicyGroup QosPolicyGroup, encrypt *bool, snapshotReserve int, dpVolume bool,
	) (*azgo.VolumeCreateResponse, error)
	VolumeModifyExportPolicy(volumeName, exportPolicyName string) (*azgo.VolumeModifyIterResponse, error)
	VolumeModifyUnixPermissions(volumeName, unixPermissions string) (*azgo.VolumeModifyIterResponse, error)
	// VolumeCloneCreate clones a volume from a snapshot
	VolumeCloneCreate(name, source, snapshot string) (*azgo.VolumeCloneCreateResponse, error)
	// VolumeCloneCreateAsync clones a volume from a snapshot
	VolumeCloneCreateAsync(name, source, snapshot string) (*azgo.VolumeCloneCreateAsyncResponse, error)
	// VolumeCloneSplitStart splits a cloned volume from its parent
	VolumeCloneSplitStart(name string) (*azgo.VolumeCloneSplitStartResponse, error)
	// VolumeDisableSnapshotDirectoryAccess disables access to the ".snapshot" directory
	// Disable '.snapshot' to allow official mysql container's chmod-in-init to work
	VolumeDisableSnapshotDirectoryAccess(name string) (*azgo.VolumeModifyIterResponse, error)
	// Use this to set the QoS Policy Group for volume clones since
	// we can't set adaptive policy groups directly during volume clone creation.
	VolumeSetQosPolicyGroupName(name string, qosPolicyGroup QosPolicyGroup) (*azgo.VolumeModifyIterResponse, error)
	// VolumeExists tests for the existence of a Flexvol
	VolumeExists(ctx context.Context, name string) (bool, error)
	// VolumeUsedSize retrieves the used bytes of the specified volume
	VolumeUsedSize(name string) (int, error)
	// VolumeSize retrieves the size of the specified volume
	VolumeSize(name string) (int, error)
	// VolumeSetSize sets the size of the specified volume
	VolumeSetSize(name, newSize string) (*azgo.VolumeSizeResponse, error)
	// VolumeMount mounts a volume at the specified junction
	VolumeMount(name, junctionPath string) (*azgo.VolumeMountResponse, error)
	// VolumeUnmount unmounts a volume from the specified junction
	VolumeUnmount(name string, force bool) (*azgo.VolumeUnmountResponse, error)
	// VolumeOffline offlines a volume
	VolumeOffline(name string) (*azgo.VolumeOfflineResponse, error)
	// VolumeDestroy destroys a volume
	VolumeDestroy(name string, force bool) (*azgo.VolumeDestroyResponse, error)
	// VolumeGet returns all relevant details for a single Flexvol
	// equivalent to filer::> volume show
	VolumeGet(name string) (*azgo.VolumeAttributesType, error)
	// VolumeGetType returns the volume type such as RW or DP
	VolumeGetType(name string) (string, error)
	// VolumeGetAll returns all relevant details for all FlexVols whose names match the supplied prefix
	// equivalent to filer::> volume show
	VolumeGetAll(prefix string) (response *azgo.VolumeGetIterResponse, err error)
	// VolumeList returns the names of all Flexvols whose names match the supplied prefix
	VolumeList(prefix string) (*azgo.VolumeGetIterResponse, error)
	// VolumeListByAttrs returns the names of all Flexvols matching the specified attributes
	VolumeListByAttrs(
		prefix, aggregate, spaceReserve, snapshotPolicy, tieringPolicy string, snapshotDir bool, encrypt *bool,
		snapReserve int,
	) (*azgo.VolumeGetIterResponse, error)
	// VolumeListAllBackedBySnapshot returns the names of all FlexVols backed by the specified snapshot
	VolumeListAllBackedBySnapshot(ctx context.Context, volumeName, snapshotName string) ([]string, error)
	// VolumeRename changes the name of a FlexVol (but not a FlexGroup!)
	VolumeRename(volumeName, newVolumeName string) (*azgo.VolumeRenameResponse, error)
	// VolumeSetComment sets a volume's comment to the supplied value
	// equivalent to filer::> volume modify -vserver iscsi_vs -volume v -comment newVolumeComment
	VolumeSetComment(ctx context.Context, volumeName, newVolumeComment string) (*azgo.VolumeModifyIterResponse, error)
	// QtreeCreate creates a qtree with the specified options
	// equivalent to filer::> qtree create -vserver ndvp_vs -volume v -qtree q -export-policy default -unix-permissions ---rwxr-xr-x -security-style unix
	QtreeCreate(name, volumeName, unixPermissions, exportPolicy, securityStyle, qosPolicy string) (*azgo.QtreeCreateResponse,
		error)
	// QtreeRename renames a qtree
	// equivalent to filer::> volume qtree rename
	QtreeRename(path, newPath string) (*azgo.QtreeRenameResponse, error)
	// QtreeDestroyAsync destroys a qtree in the background
	// equivalent to filer::> volume qtree delete -foreground false
	QtreeDestroyAsync(path string, force bool) (*azgo.QtreeDeleteAsyncResponse, error)
	// QtreeList returns the names of all Qtrees whose names match the supplied prefix
	// equivalent to filer::> volume qtree show
	QtreeList(prefix, volumePrefix string) (*azgo.QtreeListIterResponse, error)
	// QtreeCount returns the number of Qtrees in the specified Flexvol, not including the Flexvol itself
	QtreeCount(ctx context.Context, volume string) (int, error)
	// QtreeExists returns true if the named Qtree exists (and is unique in the matching Flexvols)
	QtreeExists(ctx context.Context, name, volumePattern string) (bool, string, error)
	// QtreeGet returns all relevant details for a single qtree
	// equivalent to filer::> volume qtree show
	QtreeGet(name, volumePrefix string) (*azgo.QtreeInfoType, error)
	// QtreeGetAll returns all relevant details for all qtrees whose Flexvol names match the supplied prefix
	// equivalent to filer::> volume qtree show
	QtreeGetAll(volumePrefix string) (*azgo.QtreeListIterResponse, error)
	QtreeModifyExportPolicy(name, volumeName, exportPolicy string) (*azgo.QtreeModifyResponse, error)
	// QuotaOn enables quotas on a Flexvol
	// equivalent to filer::> volume quota on
	QuotaOn(volume string) (*azgo.QuotaOnResponse, error)
	// QuotaOff disables quotas on a Flexvol
	// equivalent to filer::> volume quota off
	QuotaOff(volume string) (*azgo.QuotaOffResponse, error)
	// QuotaResize resizes quotas on a Flexvol
	// equivalent to filer::> volume quota resize
	QuotaResize(volume string) (*azgo.QuotaResizeResponse, error)
	// QuotaStatus returns the quota status for a Flexvol
	// equivalent to filer::> volume quota show
	QuotaStatus(volume string) (*azgo.QuotaStatusResponse, error)
	// QuotaSetEntry creates a new quota rule with an optional hard disk limit
	// equivalent to filer::> volume quota policy rule create
	QuotaSetEntry(qtreeName, volumeName, quotaTarget, quotaType, diskLimit string) (*azgo.QuotaSetEntryResponse, error)
	// QuotaEntryGet returns the disk limit for a single qtree
	// equivalent to filer::> volume quota policy rule show
	QuotaGetEntry(target, quotaType string) (*azgo.QuotaEntryType, error)
	// QuotaEntryList returns the disk limit quotas for a Flexvol
	// equivalent to filer::> volume quota policy rule show
	QuotaEntryList(volume string) (*azgo.QuotaListEntriesIterResponse, error)
	// ExportPolicyCreate creates an export policy
	// equivalent to filer::> vserver export-policy create
	ExportPolicyCreate(policy string) (*azgo.ExportPolicyCreateResponse, error)
	ExportPolicyGet(policy string) (*azgo.ExportPolicyGetResponse, error)
	ExportPolicyDestroy(policy string) (*azgo.ExportPolicyDestroyResponse, error)
	// ExportRuleCreate creates a rule in an export policy
	// equivalent to filer::> vserver export-policy rule create
	ExportRuleCreate(
		policy, clientMatch string, protocols, roSecFlavors, rwSecFlavors, suSecFlavors []string,
	) (*azgo.ExportRuleCreateResponse, error)
	// ExportRuleGetIterRequest returns the export rules in an export policy
	// equivalent to filer::> vserver export-policy rule show
	ExportRuleGetIterRequest(policy string) (*azgo.ExportRuleGetIterResponse, error)
	// ExportRuleDestroy deletes the rule at the given index in the given policy
	ExportRuleDestroy(policy string, ruleIndex int) (*azgo.ExportRuleDestroyResponse, error)
	// SnapshotCreate creates a snapshot of a volume
	SnapshotCreate(snapshotName, volumeName string) (*azgo.SnapshotCreateResponse, error)
	// SnapshotList returns the list of snapshots associated with a volume
	SnapshotList(volumeName string) (*azgo.SnapshotGetIterResponse, error)
	// SnapshotRestoreVolume restores a volume to a snapshot as a non-blocking operation
	SnapshotRestoreVolume(snapshotName, volumeName string) (*azgo.SnapshotRestoreVolumeResponse, error)
	// DeleteSnapshot deletes a snapshot of a volume
	SnapshotDelete(snapshotName, volumeName string) (*azgo.SnapshotDeleteResponse, error)
	// IscsiServiceGetIterRequest returns information about an iSCSI target
	IscsiServiceGetIterRequest() (*azgo.IscsiServiceGetIterResponse, error)
	// IscsiNodeGetNameRequest gets the IQN of the vserver
	IscsiNodeGetNameRequest() (*azgo.IscsiNodeGetNameResponse, error)
	// IscsiInterfaceGetIterRequest returns information about the vserver's iSCSI interfaces
	IscsiInterfaceGetIterRequest() (*azgo.IscsiInterfaceGetIterResponse, error)
	// VserverGetIterRequest returns the vservers on the system
	// equivalent to filer::> vserver show
	VserverGetIterRequest() (*azgo.VserverGetIterResponse, error)
	// VserverGetIterAdminRequest returns vservers of type "admin" on the system.
	// equivalent to filer::> vserver show -type admin
	VserverGetIterAdminRequest() (*azgo.VserverGetIterResponse, error)
	// VserverGetRequest returns vserver to which it is sent
	// equivalent to filer::> vserver show
	VserverGetRequest() (*azgo.VserverGetResponse, error)
	// SVMGetAggregateNames returns an array of names of the aggregates assigned to the configured vserver.
	// The vserver-get-iter API works with either cluster or vserver scope, so the ZAPI runner may or may not
	// be configured for tunneling; using the query parameter ensures we address only the configured vserver.
	SVMGetAggregateNames() ([]string, error)
	// VserverShowAggrGetIterRequest returns the aggregates on the vserver.  Requires ONTAP 9 or later.
	// equivalent to filer::> vserver show-aggregates
	VserverShowAggrGetIterRequest() (*azgo.VserverShowAggrGetIterResponse, error)
	// AggrSpaceGetIterRequest returns the aggregates on the system
	// equivalent to filer::> storage aggregate show-space -aggregate-name aggregate
	AggrSpaceGetIterRequest(aggregateName string) (*azgo.AggrSpaceGetIterResponse, error)
	// AggregateCommitmentPercentage returns the allocated capacity percentage for an aggregate
	// See also;  https://practical-admin.com/blog/netapp-powershell-toolkit-aggregate-overcommitment-report/
	AggregateCommitment(ctx context.Context, aggregate string) (*AggregateCommitment, error)
	// SnapmirrorGetIterRequest returns the snapmirror operations on the destination cluster
	// equivalent to filer::> snapmirror show
	SnapmirrorGetIterRequest(relGroupType string) (*azgo.SnapmirrorGetIterResponse, error)
	// SnapmirrorGetDestinationIterRequest returns the snapmirror operations on the source cluster
	// equivalent to filer::> snapmirror list-destinations
	SnapmirrorGetDestinationIterRequest(relGroupType string) (*azgo.SnapmirrorGetDestinationIterResponse, error)
	// GetPeeredVservers returns a list of vservers peered with the vserver for this backend
	GetPeeredVservers(ctx context.Context) ([]string, error)
	// IsVserverDRDestination identifies if the Vserver is a destination vserver of Snapmirror relationship (SVM-DR) or not
	IsVserverDRDestination(ctx context.Context) (bool, error)
	// IsVserverDRSource identifies if the Vserver is a source vserver of Snapmirror relationship (SVM-DR) or not
	IsVserverDRSource(ctx context.Context) (bool, error)
	SnapmirrorGet(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorGetResponse,
		error)
	SnapmirrorCreate(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName, repPolicy, repSchedule string) (*azgo.SnapmirrorCreateResponse,
		error)
	SnapmirrorInitialize(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorInitializeResponse,
		error)
	SnapmirrorResync(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorResyncResponse,
		error)
	SnapmirrorBreak(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName, snapshotName string) (*azgo.SnapmirrorBreakResponse,
		error)
	SnapmirrorQuiesce(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorQuiesceResponse,
		error)
	SnapmirrorAbort(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorAbortResponse,
		error)
	// SnapmirrorRelease removes all local snapmirror relationship metadata from the source vserver
	// Intended to be used on the source vserver
	SnapmirrorRelease(sourceFlexvolName, sourceSVMName string) error
	// Intended to be from the destination vserver
	SnapmirrorDeleteViaDestination(localInternalVolumeName, localSVMName string) (*azgo.SnapmirrorDestroyResponse,
		error)
	// Intended to be from the destination vserver
	SnapmirrorDelete(localInternalVolumeName, localSVMName, remoteFlexvolName, remoteSVMName string) (*azgo.SnapmirrorDestroyResponse,
		error)
	SnapmirrorUpdate(localInternalVolumeName, snapshotName string) (*azgo.SnapmirrorUpdateResponse, error)
	IsVserverDRCapable(ctx context.Context) (bool, error)
	SnapmirrorPolicyExists(ctx context.Context, policyName string) (bool, error)
	SnapmirrorPolicyGet(ctx context.Context, policyName string) (*azgo.SnapmirrorPolicyInfoType, error)
	JobScheduleExists(ctx context.Context, jobName string) (bool, error)
	// NetInterfaceGet returns the list of network interfaces with associated metadata
	// equivalent to filer::> net interface list, but only those LIFs that are operational
	NetInterfaceGet() (*azgo.NetInterfaceGetIterResponse, error)
	NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error)
	// SystemGetVersion returns the system version
	// equivalent to filer::> version
	SystemGetVersion() (*azgo.SystemGetVersionResponse, error)
	// SystemGetOntapiVersion gets the ONTAPI version using the credentials, and caches & returns the result.
	SystemGetOntapiVersion(ctx context.Context) (string, error)
	NodeListSerialNumbers(ctx context.Context) ([]string, error)
	// EmsAutosupportLog generates an auto support message with the supplied parameters
	EmsAutosupportLog(
		appVersion string, autoSupport bool, category, computerName, eventDescription string, eventID int,
		eventSource string, logLevel int,
	) (*azgo.EmsAutosupportLogResponse, error)
	TieringPolicyValue(ctx context.Context) string
	// IscsiInitiatorAddAuth creates and sets the authorization details for a single initiator
	//
	//	equivalent to filer::> vserver iscsi security create -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
	//	                         -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
	IscsiInitiatorAddAuth(initiator, authType, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorAddAuthResponse,
		error)
	// IscsiInitiatorAuthGetIter returns the authorization details for all non-default initiators for the Client's SVM
	// equivalent to filer::> vserver iscsi security show -vserver SVM
	IscsiInitiatorAuthGetIter() ([]azgo.IscsiSecurityEntryInfoType, error)
	// IscsiInitiatorDeleteAuth deletes the authorization details for a single initiator
	// equivalent to filer::> vserver iscsi security delete -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd
	IscsiInitiatorDeleteAuth(initiator string) (*azgo.IscsiInitiatorDeleteAuthResponse, error)
	// IscsiInitiatorGetAuth returns the authorization details for a single initiator
	// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd
	//
	//	or filer::> vserver iscsi security show -vserver SVM -initiator-name default
	IscsiInitiatorGetAuth(initiator string) (*azgo.IscsiInitiatorGetAuthResponse, error)
	// IscsiInitiatorGetDefaultAuth returns the authorization details for the default initiator
	// equivalent to filer::> vserver iscsi security show -vserver SVM -initiator-name default
	IscsiInitiatorGetDefaultAuth() (*azgo.IscsiInitiatorGetDefaultAuthResponse, error)
	// IscsiInitiatorGetIter returns the initiator details for all non-default initiators for the Client's SVM
	// equivalent to filer::> vserver iscsi initiator show -vserver SVM
	IscsiInitiatorGetIter() ([]azgo.IscsiInitiatorListEntryInfoType, error)
	// IscsiInitiatorModifyCHAPParams modifies the authorization details for a single initiator
	//
	//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name iqn.1993-08.org.debian:01:9031309bbebd \
	//	                         -user-name outboundUserName -outbound-user-name outboundPassphrase
	IscsiInitiatorModifyCHAPParams(initiator, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorModifyChapParamsResponse,
		error)
	// IscsiInitiatorSetDefaultAuth sets the authorization details for the default initiator
	//
	//	equivalent to filer::> vserver iscsi security modify -vserver SVM -initiator-name default \
	//	                          -auth-type CHAP -user-name outboundUserName -outbound-user-name outboundPassphrase
	IscsiInitiatorSetDefaultAuth(authType, userName, passphrase, outboundUserName, outboundPassphrase string) (*azgo.IscsiInitiatorSetDefaultAuthResponse,
		error)
	// SMBShareCreate creates an SMB share with the specified name and path.
	SMBShareCreate(shareName, path string) (*azgo.CifsShareCreateResponse, error)
	// SMBShareExists checks for the existence of an SMB share with the given name.
	SMBShareExists(shareName string) (bool, error)
	// SMBShareDestroy deletes an SMBShare
	SMBShareDestroy(shareName string) (*azgo.CifsShareDeleteResponse, error)
}
