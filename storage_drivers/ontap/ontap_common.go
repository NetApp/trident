// Copyright 2023 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/google/go-cmp/cmp"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

// //////////////////////////////////////////////////////////////////////////////////////////
// /             _____________________
// /            |   <<Interface>>    |
// /            |       ONTAPI       |
// /            |____________________|
// /                ^             ^
// /     Implements |             | Implements
// /   ____________________    ____________________
// /  |  ONTAPAPIREST     |   |  ONTAPAPIZAPI     |
// /  |___________________|   |___________________|
// /  | +API: RestClient  |   | +API: *Client     |
// /  |___________________|   |___________________|
// /
// //////////////////////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////////////////////
// Drivers that offer dual support are to call ONTAP REST or ZAPI's
// via abstraction layer (ONTAPI interface)
// //////////////////////////////////////////////////////////////////////////////////////////

const (
	MinimumVolumeSizeBytes       = 20971520 // 20 MiB
	HousekeepingStartupDelaySecs = 10
	LUNMetadataBufferMultiplier  = 1.1 // 10%
	MaximumIgroupNameLength      = 96  // 96 characters is the maximum character count for ONTAP igroups.

	// Constants for internal pool attributes
	Size                  = "size"
	Region                = "region"
	Zone                  = "zone"
	Media                 = "media"
	SpaceAllocation       = "spaceAllocation"
	SnapshotDir           = "snapshotDir"
	SpaceReserve          = "spaceReserve"
	SnapshotPolicy        = "snapshotPolicy"
	SnapshotReserve       = "snapshotReserve"
	UnixPermissions       = "unixPermissions"
	ExportPolicy          = "exportPolicy"
	SecurityStyle         = "securityStyle"
	BackendType           = "backendType"
	Replication           = "replication"
	Snapshots             = "snapshots"
	Clones                = "clones"
	Encryption            = "encryption"
	LUKSEncryption        = "LUKSEncryption"
	FileSystemType        = "fileSystemType"
	ProvisioningType      = "provisioningType"
	SplitOnClone          = "splitOnClone"
	TieringPolicy         = "tieringPolicy"
	QosPolicy             = "qosPolicy"
	AdaptiveQosPolicy     = "adaptiveQosPolicy"
	maxFlexGroupCloneWait = 120 * time.Second
	maxFlexvolCloneWait   = 30 * time.Second

	VolTypeRW  = "rw"  // read-write
	VolTypeLS  = "ls"  // load-sharing
	VolTypeDP  = "dp"  // data-protection
	VolTypeDC  = "dc"  // data-cache
	VolTypeTMP = "tmp" // temporary
)

// For legacy reasons, these strings mustn't change
const (
	artifactPrefixDocker     = "ndvp"
	artifactPrefixKubernetes = "trident"
	LUNAttributeFSType       = "com.netapp.ndvp.fstype"
)

// StateReason, Change in these strings require change in test automation.
const (
	StateReasonSVMStopped     = "SVM is not in 'running' state"
	StateReasonDataLIFsDown   = "No data LIFs present or all of them are 'down'"
	StateReasonSVMUnreachable = "SVM is not reachable"
)

// CleanBackendName removes brackets and replaces colons with periods to avoid regex parsing errors.
func CleanBackendName(backendName string) string {
	backendName = strings.ReplaceAll(backendName, "[", "")
	backendName = strings.ReplaceAll(backendName, "]", "")
	return strings.ReplaceAll(backendName, ":", ".")
}

func NewOntapTelemetry(ctx context.Context, d StorageDriver) *Telemetry {
	t := &Telemetry{
		Plugin:        d.Name(),
		SVM:           d.GetConfig().SVM,
		StoragePrefix: *d.GetConfig().StoragePrefix,
		Driver:        d,
		done:          make(chan struct{}),
	}

	usageHeartbeat := d.GetConfig().UsageHeartbeat
	heartbeatIntervalInHours := 24.0 // default to 24 hours
	if usageHeartbeat != "" {
		f, err := strconv.ParseFloat(usageHeartbeat, 64)
		if err != nil {
			Logc(ctx).WithField("interval", usageHeartbeat).Warnf("Invalid heartbeat interval. %v", err)
		} else {
			heartbeatIntervalInHours = f
		}
	}
	Logc(ctx).WithField("intervalHours", heartbeatIntervalInHours).Debug("Configured EMS heartbeat.")

	durationInHours := time.Millisecond * time.Duration(MSecPerHour*heartbeatIntervalInHours)
	if durationInHours > 0 {
		t.ticker = time.NewTicker(durationInHours)
	}
	return t
}

// Start starts the flow of ASUP messages for the driver
// These messages can be viewed via filer::> event log show -severity NOTICE.
func (t *Telemetry) Start(ctx context.Context) {
	go func() {
		time.Sleep(HousekeepingStartupDelaySecs * time.Second)
		EMSHeartbeat(ctx, t.Driver)
		for {
			select {
			case tick := <-t.ticker.C:
				Logc(ctx).WithFields(LogFields{
					"tick":   tick,
					"driver": t.Driver.Name(),
				}).Debug("Sending EMS heartbeat.")
				EMSHeartbeat(ctx, t.Driver)
			case <-t.done:
				Logc(ctx).WithFields(LogFields{
					"driver": t.Driver.Name(),
				}).Debugf("Shut down EMS logs for the driver.")
				return
			}
		}
	}()
}

func (t *Telemetry) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
	if !t.stopped {
		// calling close on an already closed channel causes a panic, guard against that
		close(t.done)
		t.stopped = true
	}
}

// String makes Telemetry satisfy the Stringer interface.
func (t Telemetry) String() (out string) {
	defer func() {
		if r := recover(); r != nil {
			Log().Errorf("Panic in Telemetry#ToString; err: %v", r)
			out = "<panic>"
		}
	}()
	elements := reflect.ValueOf(&t).Elem()
	var output strings.Builder
	for i := 0; i < elements.NumField(); i++ {
		fieldName := elements.Type().Field(i).Name
		switch fieldName {
		case "Driver":
			output.WriteString(fmt.Sprintf("%v:%v", "Telemetry.Driver.Name", t.Driver.Name()))
		default:
			output.WriteString(fmt.Sprintf("%v:%v ", fieldName, elements.Field(i)))
		}
	}
	out = output.String()
	return
}

// GoString makes Telemetry satisfy the GoStringer interface.
func (t Telemetry) GoString() string {
	return t.String()
}

func deleteExportPolicy(ctx context.Context, policy string, clientAPI api.OntapAPI) error {
	err := clientAPI.ExportPolicyDestroy(ctx, policy)
	if err != nil {
		err = fmt.Errorf("error deleting export policy: %s", err.Error())
	}
	return err
}

// InitializeOntapConfig parses the ONTAP config, mixing in the specified common config.
func InitializeOntapConfig(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string,
) (*drivers.OntapStorageDriverConfig, error) {
	fields := LogFields{"Method": "InitializeOntapConfig", "Type": "ontap_common"}
	Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> InitializeOntapConfig")
	defer Logd(ctx, commonConfig.StorageDriverName,
		commonConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< InitializeOntapConfig")

	commonConfig.DriverContext = driverContext

	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig

	// decode configJSON into OntapStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode JSON configuration: %v", err)
	}

	// Inject secret if not empty
	if len(backendSecret) != 0 {
		err = config.InjectSecrets(backendSecret)
		if err != nil {
			return nil, fmt.Errorf("could not inject backend secret; err: %v", err)
		}
	}
	// Ensure only one authentication type is specified in the backend config
	if config.ClientPrivateKey != "" && config.Username != "" {
		return nil, fmt.Errorf("more than one authentication method (username/password and clientPrivateKey)" +
			" present in backend config; please ensure only one authentication method is provided")
	}

	// Load default config parameters
	err = PopulateConfigurationDefaults(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	return config, nil
}

func ensureExportPolicyExists(ctx context.Context, policyName string, clientAPI api.OntapAPI) error {
	return clientAPI.ExportPolicyCreate(ctx, policyName)
}

// publishShare ensures that the volume has the correct export policy applied.
func publishShare(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
	publishInfo *utils.VolumePublishInfo, volumeName string,
	ModifyVolumeExportPolicy func(ctx context.Context, volumeName, policyName string) error,
) error {
	fields := LogFields{
		"Method": "publishFlexVolShare",
		"Type":   "ontap_common",
		"Share":  volumeName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> publishFlexVolShare")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< publishFlexVolShare")

	if !config.AutoExportPolicy || publishInfo.Unmanaged {
		// Nothing to do if we're not configuring export policies automatically or volume is not managed
		return nil
	}

	if err := ensureNodeAccess(ctx, publishInfo, clientAPI, config); err != nil {
		return err
	}

	// Update volume to use the correct export policy
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	err := ModifyVolumeExportPolicy(ctx, volumeName, policyName)
	return err
}

func getExportPolicyName(backendUUID string) string {
	return fmt.Sprintf("trident-%s", backendUUID)
}

// ensureNodeAccess check to see if the export policy exists and if not it will create it and force a reconcile.
// This should be used during publish to make sure access is available if the policy has somehow been deleted.
// Otherwise we should not need to reconcile, which could be expensive.
func ensureNodeAccess(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig,
) error {
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	if exists, err := clientAPI.ExportPolicyExists(ctx, policyName); err != nil {
		return err
	} else if !exists {
		Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy missing, will create it.")
		return reconcileNASNodeAccess(ctx, publishInfo.Nodes, config, clientAPI, policyName)
	}
	Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy exists.")
	return nil
}

func reconcileNASNodeAccess(
	ctx context.Context, nodes []*utils.Node, config *drivers.OntapStorageDriverConfig, clientAPI api.OntapAPI,
	policyName string,
) error {
	if !config.AutoExportPolicy {
		return nil
	}
	err := ensureExportPolicyExists(ctx, policyName, clientAPI)
	if err != nil {
		return err
	}
	desiredRules, err := getDesiredExportPolicyRules(ctx, nodes, config)
	if err != nil {
		err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
		Logc(ctx).Error(err)
		return err
	}
	err = reconcileExportPolicyRules(ctx, policyName, desiredRules, clientAPI, config)
	if err != nil {
		err = fmt.Errorf("unabled to reconcile export policy rules; %v", err)
		Logc(ctx).WithField("ExportPolicy", policyName).Error(err)
		return err
	}
	return nil
}

func getDesiredExportPolicyRules(
	ctx context.Context, nodes []*utils.Node, config *drivers.OntapStorageDriverConfig,
) ([]string, error) {
	rules := make([]string, 0)
	for _, node := range nodes {
		// Filter the IPs based on the CIDRs provided by user
		filteredIPs, err := utils.FilterIPs(ctx, node.IPs, config.AutoExportCIDRs)
		if err != nil {
			return nil, err
		}
		if len(filteredIPs) > 0 {
			rules = append(rules, strings.Join(filteredIPs, ","))
		}
	}
	return rules, nil
}

func reconcileExportPolicyRules(
	ctx context.Context, policyName string, desiredPolicyRules []string, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig,
) error {
	fields := LogFields{
		"Method":             "reconcileExportPolicyRule",
		"Type":               "ontap_common",
		"policyName":         policyName,
		"desiredPolicyRules": desiredPolicyRules,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> reconcileExportPolicyRules")
	defer Logc(ctx).WithFields(fields).Debug("<<<< reconcileExportPolicyRules")

	// first grab all existing rules
	rules, err := clientAPI.ExportRuleList(ctx, policyName)
	if err != nil {
		// Could not extract rules, just log it, no action required.
		Logc(ctx).WithField("error", err).Debug("Export policy rules could not be extracted.")
	}

	for _, rule := range desiredPolicyRules {
		if _, ok := rules[rule]; ok {
			// Rule already exists and we want it, so don't create it or delete it
			delete(rules, rule)
		} else {
			// Rule does not exist, so create it
			if err = clientAPI.ExportRuleCreate(ctx, policyName, rule, config.NASType); err != nil {
				return err
			}
		}
	}
	// Now that the desired rules exists, delete the undesired rules
	for _, ruleIndex := range rules {
		if err = clientAPI.ExportRuleDestroy(ctx, policyName, ruleIndex); err != nil {
			return err
		}
	}
	return nil
}

// getSVMState gets the backend SVM state and reason for offline if any.
// Input:
// protocol - to get the data LIFs of similar service from backend.
// pools - list of known pools to compare with the backend aggregate list and determine the change if any.
func getSVMState(
	ctx context.Context, client api.OntapAPI, protocol string, pools []string,
) (string, *roaring.Bitmap) {
	changeMap := roaring.New()
	svmState, err := client.GetSVMState(ctx)
	if err != nil {
		// Could not get the SVM info or SVM is unreachable. Just log it.
		// Set state offline and reason as unreachable.
		Logc(ctx).WithField("error", err).Debug("Error getting SVM information.")
		return StateReasonSVMUnreachable, changeMap
	}

	// Get Aggregates list and verify if there is any change.
	aggrList, err := client.GetSVMAggregateNames(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Debug("Error getting the physical pools from backend.")
	} else {
		sort.Strings(aggrList)
		sort.Strings(pools)
		if !cmp.Equal(pools, aggrList) {
			changeMap.Add(storage.BackendStatePoolsChange)
		}
	}

	if svmState != models.SvmStateRunning {
		return StateReasonSVMStopped, changeMap
	}

	// Get data LIFs.
	upDataLIFs, err := client.NetInterfaceGetDataLIFs(ctx, protocol)
	if err != nil || len(upDataLIFs) == 0 {
		if err != nil {
			// Log error and keep going.
			Logc(ctx).WithField("error", err).Debug("Error getting list of data LIFs from backend.")
		}
		// No data LIFs with state 'up' found.
		return StateReasonDataLIFsDown, changeMap
	}

	return "", changeMap
}

// resizeValidation performs needed validation checks prior to the resize operation.
func resizeValidation(
	ctx context.Context, name string, sizeBytes uint64, volumeExists func(context.Context, string) (bool, error),
	volumeSize func(context.Context, string) (uint64, error),
) (uint64, error) {
	// Check that volume exists
	volExists, err := volumeExists(ctx, name)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Error checking for existing volume.")
		return 0, fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return 0, fmt.Errorf("volume %s does not exist", name)
	}

	// Check that current size is smaller than requested size
	volSize, err := volumeSize(ctx, name)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Error checking volume size.")
		return 0, fmt.Errorf("error occurred when checking volume size")
	}
	volSizeBytes := uint64(volSize)

	if sizeBytes < volSizeBytes {
		return 0, errors.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size %d is less than existing volume size %d", sizeBytes, volSize))
	}

	return volSizeBytes, nil
}

// reconcileSANNodeAccess ensures unused igroups are removed. Unused igroups are the legacy per-backend igroup and
// per-node igroups without publications. Igroups are not removed if any LUNs are mapped; multiple backends may use
// the same vserver, and existing volumes may still use the per-backend igroup.
func reconcileSANNodeAccess(
	ctx context.Context, clientAPI api.OntapAPI, nodes []string, backendUUID, tridentUUID string,
) error {
	// List all igroups in backend
	igroups, err := clientAPI.IgroupList(ctx)
	if err != nil {
		return err
	}

	// Attempt to delete unused igroups
	igroups = filterUnusedTridentIgroups(igroups, nodes, backendUUID, tridentUUID)
	Logc(ctx).WithFields(LogFields{
		"unusedIgroups": igroups,
		"backendUUID":   backendUUID,
	}).Debug("Attempting to delete unused igroups")
	for _, igroup := range igroups {
		if err := DestroyUnmappedIgroup(ctx, clientAPI, igroup); err != nil {
			return err
		}
	}

	return nil
}

// filterUnusedTridentIgroups returns Trident-created igroups not in use for backend. Includes per-backend
// igroup and any of the form <node name>-<trident uuid>.
func filterUnusedTridentIgroups(igroups, nodes []string, backendUUID, tridentUUID string) []string {
	unusedIgroups := make([]string, 0, len(igroups))
	nodeMap := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeMap[node] = struct{}{}
	}
	backendIgroup := getDefaultIgroupName(tridentconfig.ContextCSI, backendUUID)

	for _, igroup := range igroups {
		if igroup == backendIgroup {
			// Always include deprecated backend igroup
			unusedIgroups = append(unusedIgroups, igroup)
		} else if strings.HasSuffix(igroup, tridentUUID) {
			// Skip igroups without trident uuid
			igroupNodeName := strings.TrimSuffix(igroup, "-"+tridentUUID)
			if _, ok := nodeMap[igroupNodeName]; !ok {
				// Include igroup that is not part of node map
				unusedIgroups = append(unusedIgroups, igroup)
			}
		}
	}

	return unusedIgroups
}

// GetISCSITargetInfo returns the iSCSI node name and iSCSI interfaces using the provided client's SVM.
func GetISCSITargetInfo(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
) (iSCSINodeName string, iSCSIInterfaces []string, returnError error) {
	// Get the SVM iSCSI IQN
	iSCSINodeName, err := clientAPI.IscsiNodeGetNameRequest(ctx)
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI node name: %v", err)
		return
	}

	// Get the SVM iSCSI interface with enabled IQNs
	iscsiInterfaces, err := clientAPI.IscsiInterfaceGet(ctx, config.SVM)
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI node name: %v", err)
		return
	}
	// Get the IQN
	if iscsiInterfaces == nil {
		returnError = fmt.Errorf("SVM %s has no active iSCSI interfaces", config.SVM)
		return
	}

	return
}

var ontapDriverRedactList = [...]string{"API"}

func GetOntapDriverRedactList() []string {
	clone := ontapDriverRedactList
	return clone[:]
}

// getNodeSpecificIgroupName generates a distinct igroup name for node name.
// Igroup names may collide if node names are over 59 characters.
func getNodeSpecificIgroupName(nodeName, tridentUUID string) string {
	igroupName := fmt.Sprintf("%s-%s", nodeName, tridentUUID)

	if len(igroupName) > MaximumIgroupNameLength {
		// If the new igroup name is over the igroup character limit, it means the host name is too long.
		igroupPrefixLength := MaximumIgroupNameLength - len(tridentUUID) - 1
		igroupName = fmt.Sprintf("%s-%s", nodeName[:igroupPrefixLength], tridentUUID)
	}
	return igroupName
}

// PublishLUN publishes the volume to the host specified in publishInfo from ontap-san or
// ontap-san-economy. This method may or may not be running on the host where the volume will be
// mounted, so it should limit itself to updating access rules, initiator groups, etc. that require
// some host identity (but not locality) as well as storage controller API access.
// This function assumes that the list of data LIF IP addresses does not change between driver initialization
// and publish
func PublishLUN(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig, ips []string,
	publishInfo *utils.VolumePublishInfo, lunPath, igroupName, iSCSINodeName string,
) error {
	fields := LogFields{
		"Method":        "PublishLUN",
		"Type":          "ontap_common",
		"lunPath":       lunPath,
		"igroup":        igroupName,
		"iSCSINodeName": iSCSINodeName,
		"publishInfo":   publishInfo,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> PublishLUN")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< PublishLUN")

	var iqn string
	var err error

	if publishInfo.Localhost {

		// Lookup local host IQNs
		iqns, err := utils.GetInitiatorIqns(ctx)
		if err != nil {
			return fmt.Errorf("error determining host initiator IQN: %v", err)
		} else if len(iqns) == 0 {
			return errors.New("could not determine host initiator IQN")
		}
		iqn = iqns[0]

	} else {

		// Host IQN must have been passed in
		if len(publishInfo.HostIQN) == 0 {
			return errors.New("host initiator IQN not specified")
		}
		iqn = publishInfo.HostIQN[0]
	}

	// Get the fstype
	fstype := drivers.DefaultFileSystemType
	lunFSType, err := clientAPI.LunGetFSType(ctx, lunPath)
	if err != nil || lunFSType == "" {
		if err != nil {
			Logc(ctx).Warnf("failed to get fstype for LUN: %v", err)
		}
		Logc(ctx).WithFields(LogFields{
			"LUN":    lunPath,
			"fstype": fstype,
		}).Warn("LUN attribute fstype not found, using default.")
	} else {
		fstype = lunFSType
	}

	// Get LUN Serial Number
	lunResponse, err := clientAPI.LunGetByName(ctx, lunPath)
	if err != nil || lunResponse == nil {
		return fmt.Errorf("problem retrieving LUN info: %v", err)
	}
	serial := lunResponse.SerialNumber

	if serial == "" {
		return fmt.Errorf("LUN '%v' serial number not found", lunPath)
	}

	if config.DriverContext == tridentconfig.ContextCSI {
		// Get the info about the targeted node
		var targetNode *utils.Node
		for _, node := range publishInfo.Nodes {
			if node.Name == publishInfo.HostName {
				targetNode = node
				break
			}
		}
		if targetNode == nil {
			err = fmt.Errorf("node %s has not registered with Trident", publishInfo.HostName)
			Logc(ctx).Error(err)
			return err
		}
	}

	if iqn != "" {
		// Add IQN to igroup
		err = clientAPI.EnsureIgroupAdded(ctx, igroupName, iqn)
		if err != nil {
			return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
		}
	}

	// Map LUN (it may already be mapped)
	lunID, err := clientAPI.EnsureLunMapped(ctx, igroupName, lunPath)
	if err != nil {
		return err
	}

	filteredIPs, err := getISCSIDataLIFsForReportingNodes(ctx, clientAPI, ips, lunPath, igroupName,
		publishInfo.Unmanaged)
	if err != nil {
		return err
	}

	if len(filteredIPs) == 0 {
		Logc(ctx).Warn("Unable to find reporting ONTAP nodes for discovered dataLIFs.")
		filteredIPs = ips
	}

	// xfs volumes are always mounted with '-o nouuid' to allow clones to be mounted to the same node as the source
	if fstype == tridentconfig.FsXfs {
		publishInfo.MountOptions = drivers.EnsureMountOption(publishInfo.MountOptions, drivers.MountOptionNoUUID)
	}

	// Add fields needed by Attach
	publishInfo.IscsiLunNumber = int32(lunID)
	publishInfo.IscsiLunSerial = serial
	publishInfo.IscsiTargetPortal = filteredIPs[0]
	publishInfo.IscsiPortals = filteredIPs[1:]
	publishInfo.IscsiTargetIQN = iSCSINodeName
	publishInfo.SANType = sa.ISCSI

	if igroupName != "" {
		addUniqueIscsiIGroupName(publishInfo, igroupName)
	}

	publishInfo.FilesystemType = fstype
	publishInfo.UseCHAP = config.UseCHAP

	if publishInfo.UseCHAP {
		publishInfo.IscsiUsername = config.ChapUsername
		publishInfo.IscsiInitiatorSecret = config.ChapInitiatorSecret
		publishInfo.IscsiTargetUsername = config.ChapTargetUsername
		publishInfo.IscsiTargetSecret = config.ChapTargetInitiatorSecret
		publishInfo.IscsiInterface = "default"
	}
	publishInfo.SharedTarget = true

	return nil
}

// addUniqueIscsiIGroupName added iscsiIgroup name in the IscsiIgroup name string if it is not present.
func addUniqueIscsiIGroupName(publishInfo *utils.VolumePublishInfo, igroupName string) {
	if publishInfo.IscsiIgroup == "" {
		publishInfo.IscsiIgroup = igroupName
	} else {
		// Validate the iscsiGroupName present in the volume publish info. If not present, add in a string.
		if !strings.Contains(publishInfo.IscsiIgroup, igroupName) {
			publishInfo.IscsiIgroup += "," + igroupName
		}
	}
}

// removeIgroupFromIscsiIgroupList removes iscsiIgroup name in the IscsiIgroup list
func removeIgroupFromIscsiIgroupList(iscsiIgroupList, igroup string) string {
	if iscsiIgroupList != "" {
		newIgroupList := make([]string, 0)
		igroups := strings.Split(iscsiIgroupList, ",")

		for _, value := range igroups {
			if value != igroup {
				newIgroupList = append(newIgroupList, value)
			}
		}
		return strings.Join(newIgroupList, ",")
	}

	return iscsiIgroupList
}

// getISCSIDataLIFsForReportingNodes finds the data LIFs for the reporting nodes for the LUN.
func getISCSIDataLIFsForReportingNodes(
	ctx context.Context, clientAPI api.OntapAPI, ips []string, lunPath, igroupName string, unmanagedImport bool,
) ([]string, error) {
	fields := LogFields{
		"ips":     ips,
		"lunPath": lunPath,
		"igroup":  igroupName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> getISCSIDataLIFsForReportingNodes")
	defer Logc(ctx).WithFields(fields).Debug("<<<< getISCSIDataLIFsForReportingNodes")

	if len(ips) < 1 {
		return nil, fmt.Errorf("missing data LIF information")
	}

	reportingNodes, err := clientAPI.LunMapGetReportingNodes(ctx, igroupName, lunPath)
	if err != nil {
		return nil, fmt.Errorf("could not get iSCSI reported nodes: %v", err)
	}

	// TODO(arorar): Since, unmanaged imports do not adhere to Publish Enforcement yet they are not re-assigned to
	//               a Trident managed iGroup, thus it is very much possible to get zero reporting nodes and/or
	//               zero SLM dataLIFs. Thus adding this ugly temporary ugly condition to handle that scenario.
	if !unmanagedImport && len(reportingNodes) < 1 {
		return nil, fmt.Errorf("no reporting nodes found")
	}

	reportedDataLIFs, err := clientAPI.GetSLMDataLifs(ctx, ips, reportingNodes)
	if err != nil {
		return nil, err
	} else if !unmanagedImport && len(reportedDataLIFs) < 1 {
		return nil, fmt.Errorf("no reporting data LIFs found")
	}

	Logc(ctx).WithField("reportedDataLIFs", reportedDataLIFs).Debug("Data LIFs with reporting nodes.")
	return reportedDataLIFs, nil
}

// ValidateBidirectionalChapCredentials validates the bidirectional CHAP settings
func ValidateBidirectionalChapCredentials(
	defaultAuth api.IscsiInitiatorAuth,
	config *drivers.OntapStorageDriverConfig,
) (*ChapCredentials, error) {
	isDefaultAuthTypeNone := IsDefaultAuthTypeNone(defaultAuth)

	isDefaultAuthTypeCHAP := IsDefaultAuthTypeCHAP(defaultAuth)

	isDefaultAuthTypeDeny := IsDefaultAuthTypeDeny(defaultAuth)

	// make sure it's one of the 3 types we understand
	if !isDefaultAuthTypeNone && !isDefaultAuthTypeCHAP && !isDefaultAuthTypeDeny {
		return nil, fmt.Errorf("default initiator's auth type is unsupported")
	}

	// make sure access is allowed
	if isDefaultAuthTypeDeny {
		return nil, fmt.Errorf("default initiator's auth type is deny")
	}

	// make sure all 4 fields are set
	var l []string
	if config.ChapUsername == "" {
		l = append(l, "ChapUsername")
	}
	if config.ChapInitiatorSecret == "" {
		l = append(l, "ChapInitiatorSecret")
	}
	if config.ChapTargetUsername == "" {
		l = append(l, "ChapTargetUsername")
	}
	if config.ChapTargetInitiatorSecret == "" {
		l = append(l, "ChapTargetInitiatorSecret")
	}
	if len(l) > 0 {
		return nil, fmt.Errorf("missing value for required field(s) %v", l)
	}

	// if CHAP is already enabled, make sure the usernames match
	if isDefaultAuthTypeCHAP {
		if defaultAuth.ChapUser == "" || defaultAuth.ChapOutboundUser == "" {
			return nil, fmt.Errorf("error checking default initiator's credentials")
		}

		if config.ChapUsername != defaultAuth.ChapUser ||
			config.ChapTargetUsername != defaultAuth.ChapOutboundUser {
			return nil, fmt.Errorf("provided CHAP usernames do not match default initiator's usernames")
		}
	}

	result := &ChapCredentials{
		ChapUsername:              config.ChapUsername,
		ChapInitiatorSecret:       config.ChapInitiatorSecret,
		ChapTargetUsername:        config.ChapTargetUsername,
		ChapTargetInitiatorSecret: config.ChapTargetInitiatorSecret,
	}

	return result, nil
}

// isDefaultAuthTypeOfType returns true if the default initiator's auth-type field is set to the provided authType value
func isDefaultAuthTypeOfType(
	response api.IscsiInitiatorAuth, authType string,
) bool {
	// case insensitive compare
	return strings.EqualFold(response.AuthType, authType)
}

// IsDefaultAuthTypeNone returns true if the default initiator's auth-type field is set to the value "none"
func IsDefaultAuthTypeNone(response api.IscsiInitiatorAuth) bool {
	return isDefaultAuthTypeOfType(response, "none")
}

// IsDefaultAuthTypeCHAP returns true if the default initiator's auth-type field is set to the value "CHAP"
func IsDefaultAuthTypeCHAP(response api.IscsiInitiatorAuth) bool {
	return isDefaultAuthTypeOfType(response, "CHAP")
}

// IsDefaultAuthTypeDeny returns true if the default initiator's auth-type field is set to the value "deny"
func IsDefaultAuthTypeDeny(response api.IscsiInitiatorAuth) bool {
	return isDefaultAuthTypeOfType(response, "deny")
}

// InitializeSANDriver performs common ONTAP SAN driver initialization.
func InitializeSANDriver(
	ctx context.Context, driverContext tridentconfig.DriverContext, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig, validate func(context.Context) error, backendUUID string,
) error {
	fields := LogFields{"Method": "InitializeSANDriver", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> InitializeSANDriver")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< InitializeSANDriver")

	// Defer validation to the driver's validate method
	if err := validate(ctx); err != nil {
		return err
	}

	if config.DriverContext != tridentconfig.ContextCSI {
		if config.IgroupName == "" {
			config.IgroupName = getDefaultIgroupName(driverContext, backendUUID)
		}
		err := ensureIGroupExists(ctx, clientAPI, config.IgroupName)
		if err != nil {
			return err
		}
	}

	getDefaultAuthResponse, err := clientAPI.IscsiInitiatorGetDefaultAuth(ctx)
	Logc(ctx).WithFields(LogFields{
		"getDefaultAuthResponse": getDefaultAuthResponse,
		"err":                    err,
	}).Debug("IscsiInitiatorGetDefaultAuth result")

	if err != nil {
		return fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeNone := IsDefaultAuthTypeNone(getDefaultAuthResponse)

	if config.UseCHAP {

		authType := "CHAP"
		chapCredentials, err := ValidateBidirectionalChapCredentials(getDefaultAuthResponse, config)
		if err != nil {
			return fmt.Errorf("error with CHAP credentials: %v", err)
		}
		Logc(ctx).Debug("Using CHAP credentials")

		if isDefaultAuthTypeNone {
			lunsResponse, lunsResponseErr := clientAPI.LunList(ctx, "*")
			if lunsResponseErr != nil {
				return fmt.Errorf("error enumerating LUNs for SVM %v: %v", config.SVM, lunsResponseErr)
			}

			if len(lunsResponse) > 0 {
				return fmt.Errorf(
					"will not enable CHAP for SVM %v; %v exisiting LUNs would lose access",
					config.SVM, len(lunsResponse))
			}
		}

		err = clientAPI.IscsiInitiatorSetDefaultAuth(ctx, authType, chapCredentials.ChapUsername,
			chapCredentials.ChapInitiatorSecret, chapCredentials.ChapTargetUsername,
			chapCredentials.ChapTargetInitiatorSecret)
		if err != nil {
			return fmt.Errorf("error setting CHAP credentials: %v", err)
		}
		config.ChapUsername = chapCredentials.ChapUsername
		config.ChapInitiatorSecret = chapCredentials.ChapInitiatorSecret
		config.ChapTargetUsername = chapCredentials.ChapTargetUsername
		config.ChapTargetInitiatorSecret = chapCredentials.ChapTargetInitiatorSecret

	} else {
		if !isDefaultAuthTypeNone {
			return fmt.Errorf("default initiator's auth type is not 'none'")
		}
	}

	return nil
}

func getDefaultIgroupName(driverContext tridentconfig.DriverContext, backendUUID string) string {
	if driverContext == tridentconfig.ContextCSI {
		return drivers.GetDefaultIgroupName(driverContext) + "-" + backendUUID
	} else {
		return drivers.GetDefaultIgroupName(driverContext)
	}
}

func ensureIGroupExists(ctx context.Context, clientAPI api.OntapAPI, igroupName string) error {
	err := clientAPI.IgroupCreate(ctx, igroupName, "iscsi", "linux")
	if err != nil {
		return fmt.Errorf("error creating igroup: %v", err)
	}
	return nil
}

// InitializeOntapDriver sets up the API client and performs all other initialization tasks
// that are common to all the ONTAP drivers.
func InitializeOntapDriver(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api.OntapAPI, error) {
	fields := LogFields{"Method": "InitializeOntapDriver", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> InitializeOntapDriver")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< InitializeOntapDriver")

	// Splitting config.ManagementLIF with colon allows to provide managementLIF value as address:port format
	mgmtLIF := ""
	if utils.IPv6Check(config.ManagementLIF) {
		// This is an IPv6 address

		mgmtLIF = strings.Split(config.ManagementLIF, "[")[1]
		mgmtLIF = strings.Split(mgmtLIF, "]")[0]
	} else {
		mgmtLIF = strings.Split(config.ManagementLIF, ":")[0]
	}

	addressesFromHostname, err := net.LookupHost(mgmtLIF)
	if err != nil {
		Logc(ctx).WithField("ManagementLIF", mgmtLIF).Error("Host lookup failed for ManagementLIF. ", err)
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"hostname":  mgmtLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from ManagementLIF lookup.")

	// Get the API client
	client, err := InitializeOntapAPI(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not create Data ONTAP API client: %v", err)
	}

	err = client.ValidateAPIVersion(ctx)
	if err != nil {
		return nil, err
	}

	// Log cluster node serial numbers if we can get them
	config.SerialNumbers, err = client.NodeListSerialNumbers(ctx)
	if err != nil {
		Logc(ctx).Warnf("Could not determine controller serial numbers. %v", err)
	} else {
		Logc(ctx).WithFields(LogFields{
			"serialNumbers": strings.Join(config.SerialNumbers, ","),
		}).Info("Controller serial numbers.")
	}

	return client, nil
}

// InitializeOntapAPI returns an ontap.Client ZAPI or REST client.  If the SVM isn't specified in the config
// file, this method attempts to derive the one to use for ZAPI.
func InitializeOntapAPI(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api.OntapAPI, error) {
	var ontapAPI api.OntapAPI
	var err error

	fields := LogFields{
		"Method": "InitializeOntapAPI", "Type": "ontap_common",
		"useREST": config.UseREST,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> InitializeOntapAPI")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< InitializeOntapAPI")

	// When running in Docker context we want to request MAX number of records from ZAPI for Volume, LUNs and Qtrees
	numRecords := api.DefaultZapiRecords
	if config.DriverContext == tridentconfig.ContextDocker {
		numRecords = api.MaxZapiRecords
	}

	if config.UseREST == true {
		ontapAPI, err = api.NewRestClientFromOntapConfig(ctx, config)
	} else {
		ontapAPI, err = api.NewZAPIClientFromOntapConfig(ctx, config, numRecords)
	}

	if err != nil {
		return nil, fmt.Errorf("error creating ONTAP API client: %v", err)
	}

	Logc(ctx).WithField("SVM", ontapAPI.SVMName()).Debug("Using SVM.")
	return ontapAPI, nil
}

// ValidateSANDriver contains the validation logic shared between ontap-san and ontap-san-economy.
func ValidateSANDriver(
	ctx context.Context, config *drivers.OntapStorageDriverConfig, ips []string,
) error {
	fields := LogFields{"Method": "ValidateSANDriver", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ValidateSANDriver")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ValidateSANDriver")

	// Specifying single DataLIF is no longer supported for iSCSI attachments. Please note multipathing should
	// be enabled by default.
	if config.DataLIF != "" {
		Logc(ctx).WithField("dataLIF", config.DataLIF).
			Warning("Specifying data LIF is no longer supported for SAN backends.")
	}

	switch config.DriverContext {
	case tridentconfig.ContextDocker:
		// Make sure this host is logged into the ONTAP iSCSI target
		err := utils.EnsureISCSISessionsWithPortalDiscovery(ctx, ips)
		if err != nil {
			return fmt.Errorf("error establishing iSCSI session: %v", err)
		}
	case tridentconfig.ContextCSI:
		// ontap-san-* drivers should all support publish enforcement with CSI; if the igroup is set
		// in the backend config, log a warning because it will not be used.
		if config.IgroupName != "" {
			Logc(ctx).WithField("igroup", config.IgroupName).
				Warning("Specifying an igroup is no longer supported for SAN backends in a CSI environment.")
		}
	}

	return nil
}

// ValidateNASDriver contains the validation logic shared between ontap-nas and ontap-nas-economy.
func ValidateNASDriver(
	ctx context.Context, api api.OntapAPI, config *drivers.OntapStorageDriverConfig,
) error {
	var dataLIFs []string
	var protocol string

	fields := LogFields{"Method": "ValidateNASDriver", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ValidateNASDriver")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ValidateNASDriver")

	isLuks, err := strconv.ParseBool(config.LUKSEncryption)
	if err != nil {
		return fmt.Errorf("could not parse LUKSEncryption from volume config into a boolean, got %v",
			config.LUKSEncryption)
	}
	if isLuks {
		return fmt.Errorf("ONTAP NAS drivers do not support LUKS encrypted volumes")
	}

	if config.NASType == sa.SMB {
		protocol = "cifs"
	} else {
		protocol = "nfs"
	}

	dataLIFs, err = api.NetInterfaceGetDataLIFs(ctx, protocol)
	if err != nil {
		return err
	}

	if len(dataLIFs) == 0 {
		return fmt.Errorf("no NAS data LIFs found on SVM %s", api.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Found NAS LIFs.")
	}

	// If they didn't set a LIF to use in the config, we'll set it to the first NFS/SMB LIF we happen to find
	if config.DataLIF == "" {
		if utils.IPv6Check(dataLIFs[0]) {
			config.DataLIF = "[" + dataLIFs[0] + "]"
		} else {
			config.DataLIF = dataLIFs[0]
		}
	} else {
		cleanDataLIF := sanitizeDataLIF(config.DataLIF)
		_, err := ValidateDataLIF(ctx, cleanDataLIF, dataLIFs)
		if err != nil {
			return fmt.Errorf("data LIF validation failed: %v", err)
		}
	}

	// Ensure config has a set of valid autoExportCIDRs
	if err := utils.ValidateCIDRs(ctx, config.AutoExportCIDRs); err != nil {
		return fmt.Errorf("failed to validate auto-export CIDR(s): %w", err)
	}

	return nil
}

func ValidateStoragePrefix(storagePrefix string) error {
	// Ensure storage prefix is compatible with ONTAP
	matched, err := regexp.MatchString(`^$|^[a-zA-Z_.-][a-zA-Z0-9_.-]*$`, storagePrefix)
	if err != nil {
		err = fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		err = fmt.Errorf(
			"storage prefix may only contain letters/digits/underscore/dash and must begin with letter/underscore/dash")
	}

	return err
}

// Enable space-allocation by default. If not enabled, Data ONTAP takes the LUNs offline
// when they're seen as full.
// see: https://github.com/NetApp/trident/issues/135
const DefaultSpaceAllocation = "true"

const (
	DefaultSpaceReserve              = "none"
	DefaultSnapshotPolicy            = "none"
	DefaultSnapshotReserve           = "5"
	DefaultUnixPermissions           = "---rwxrwxrwx"
	DefaultSnapshotDir               = "false"
	DefaultExportPolicy              = "default"
	DefaultSecurityStyleNFS          = "unix"
	DefaultSecurityStyleSMB          = "ntfs"
	DefaultNfsMountOptionsDocker     = "-o nfsvers=3"
	DefaultNfsMountOptionsKubernetes = ""
	DefaultSplitOnClone              = "false"
	DefaultLuksEncryption            = "false"
	DefaultMirroring                 = "false"
	DefaultLimitAggregateUsage       = ""
	DefaultLimitVolumeSize           = ""
	DefaultTieringPolicy             = ""
)

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func PopulateConfigurationDefaults(ctx context.Context, config *drivers.OntapStorageDriverConfig) error {
	fields := LogFields{"Method": "PopulateConfigurationDefaults", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> PopulateConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< PopulateConfigurationDefaults")

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	} else {
		_, err := utils.ConvertSizeToBytes(config.Size)
		if err != nil {
			return fmt.Errorf("invalid config value for default volume size: %v", err)
		}
	}

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
	}

	if config.SpaceAllocation == "" {
		config.SpaceAllocation = DefaultSpaceAllocation
	}

	if config.SpaceReserve == "" {
		config.SpaceReserve = DefaultSpaceReserve
	}

	if config.SnapshotPolicy == "" {
		config.SnapshotPolicy = DefaultSnapshotPolicy
	}

	if config.SnapshotReserve == "" {
		if config.SnapshotPolicy == "none" {
			config.SnapshotReserve = ""
		} else {
			config.SnapshotReserve = DefaultSnapshotReserve
		}
	}

	if config.SnapshotDir == "" {
		config.SnapshotDir = DefaultSnapshotDir
	}

	if config.DriverContext != tridentconfig.ContextCSI {
		config.AutoExportPolicy = false
	}

	if config.AutoExportPolicy {
		config.ExportPolicy = "<automatic>"
	} else if config.ExportPolicy == "" {
		config.ExportPolicy = DefaultExportPolicy
	}

	if config.NfsMountOptions == "" {
		switch config.DriverContext {
		case tridentconfig.ContextDocker:
			config.NfsMountOptions = DefaultNfsMountOptionsDocker
		default:
			config.NfsMountOptions = DefaultNfsMountOptionsKubernetes
		}
	}

	if config.SplitOnClone == "" {
		config.SplitOnClone = DefaultSplitOnClone
	} else {
		_, err := strconv.ParseBool(config.SplitOnClone)
		if err != nil {
			return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
		}
	}

	if config.FileSystemType == "" {
		config.FileSystemType = drivers.DefaultFileSystemType
	}

	if config.LUKSEncryption == "" {
		config.LUKSEncryption = DefaultLuksEncryption
	}

	if config.Mirroring == "" {
		config.Mirroring = DefaultMirroring
	}

	if config.LimitAggregateUsage == "" {
		config.LimitAggregateUsage = DefaultLimitAggregateUsage
	}

	if config.LimitVolumeSize == "" {
		config.LimitVolumeSize = DefaultLimitVolumeSize
	}

	if config.TieringPolicy == "" {
		config.TieringPolicy = DefaultTieringPolicy
	}

	if len(config.AutoExportCIDRs) == 0 {
		config.AutoExportCIDRs = []string{"0.0.0.0/0", "::/0"}
	}

	if len(config.FlexGroupAggregateList) == 0 {
		config.FlexGroupAggregateList = []string{}
	}

	if config.SANType == "" {
		config.SANType = sa.ISCSI
	}

	// If NASType is not provided in the backend config, default to NFS
	if config.NASType == "" {
		config.NASType = sa.NFS
	}

	switch config.NASType {
	case sa.SMB:
		if config.SecurityStyle == "" {
			config.SecurityStyle = DefaultSecurityStyleSMB
		}
		// SMB supports "mixed" and "ntfs" security styles.
		// ONTAP supports unix permissions only with security style "mixed" on SMB volume.
		if config.SecurityStyle == "mixed" {
			if config.UnixPermissions == "" {
				config.UnixPermissions = DefaultUnixPermissions
			}
		} else {
			config.UnixPermissions = ""
		}
	case sa.NFS:
		if config.UnixPermissions == "" {
			config.UnixPermissions = DefaultUnixPermissions
		}
		if config.SecurityStyle == "" {
			config.SecurityStyle = DefaultSecurityStyleNFS
		}
	}

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":          *config.StoragePrefix,
		"SpaceAllocation":        config.SpaceAllocation,
		"SpaceReserve":           config.SpaceReserve,
		"SnapshotPolicy":         config.SnapshotPolicy,
		"SnapshotReserve":        config.SnapshotReserve,
		"UnixPermissions":        config.UnixPermissions,
		"SnapshotDir":            config.SnapshotDir,
		"ExportPolicy":           config.ExportPolicy,
		"SecurityStyle":          config.SecurityStyle,
		"NfsMountOptions":        config.NfsMountOptions,
		"SplitOnClone":           config.SplitOnClone,
		"FileSystemType":         config.FileSystemType,
		"Encryption":             config.Encryption,
		"LUKSEncryption":         config.LUKSEncryption,
		"Mirroring":              config.Mirroring,
		"LimitAggregateUsage":    config.LimitAggregateUsage,
		"LimitVolumeSize":        config.LimitVolumeSize,
		"Size":                   config.Size,
		"TieringPolicy":          config.TieringPolicy,
		"AutoExportPolicy":       config.AutoExportPolicy,
		"AutoExportCIDRs":        config.AutoExportCIDRs,
		"FlexgroupAggregateList": config.FlexGroupAggregateList,
	}).Debugf("Configuration defaults")

	return nil
}

func checkAggregateLimitsForFlexvol(
	ctx context.Context, flexvol string, requestedSizeInt uint64, config drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) error {
	volInfo, err := client.VolumeInfo(ctx, flexvol)
	if err != nil {
		return err
	}

	if len(volInfo.Aggregates) < 1 {
		return fmt.Errorf("aggregate info not available from Flexvol %s", flexvol)
	}

	return checkAggregateLimits(ctx, volInfo.Aggregates[0], volInfo.SpaceReserve, requestedSizeInt, config,
		client)
}

func checkAggregateLimits(
	ctx context.Context, aggregate, spaceReserve string, requestedSizeInt uint64,
	config drivers.OntapStorageDriverConfig, client api.OntapAPI,
) error {
	requestedSize := float64(requestedSizeInt)

	limitAggregateUsage := config.LimitAggregateUsage
	limitAggregateUsage = strings.Replace(limitAggregateUsage, "%", "", -1) // strip off any %

	Logc(ctx).WithFields(LogFields{
		"aggregate":           aggregate,
		"requestedSize":       requestedSize,
		"limitAggregateUsage": limitAggregateUsage,
	}).Debugf("Checking aggregate limits")

	if limitAggregateUsage == "" {
		Logc(ctx).Debugf("No limits specified")
		return nil
	}

	if aggregate == "" {
		return errors.New("aggregate not provided, cannot check aggregate provisioning limits")
	}

	// lookup aggregate
	SVMAggregateSpaceList, aggrSpaceErr := client.GetSVMAggregateSpace(ctx, aggregate)
	if aggrSpaceErr != nil {
		return aggrSpaceErr
	}

	for _, aggrSpace := range SVMAggregateSpaceList {

		if limitAggregateUsage != "" {
			percentLimit, parseErr := strconv.ParseFloat(limitAggregateUsage, 64)
			if parseErr != nil {
				return parseErr
			}

			usedIncludingSnapshotReserve := float64(aggrSpace.Used())
			aggregateSize := float64(aggrSpace.Size())

			spaceReserveIsThick := false
			if spaceReserve == "volume" {
				spaceReserveIsThick = true
			}

			if spaceReserveIsThick {
				// we SHOULD include the requestedSize in our computation
				percentUsedWithRequest := ((usedIncludingSnapshotReserve + requestedSize) / aggregateSize) * 100.0
				Logc(ctx).WithFields(LogFields{
					"percentUsedWithRequest": percentUsedWithRequest,
					"percentLimit":           percentLimit,
					"spaceReserve":           spaceReserve,
				}).Debugf("Checking usage percentage limits")

				if percentUsedWithRequest >= percentLimit {
					errorMessage := fmt.Sprintf("aggregate usage of %.2f %% would exceed the limit of %.2f %%",
						percentUsedWithRequest, percentLimit)
					return errors.New(errorMessage)
				}
			} else {
				// we should NOT include the requestedSize in our computation
				percentUsedWithoutRequest := ((usedIncludingSnapshotReserve) / aggregateSize) * 100.0
				Logc(ctx).WithFields(LogFields{
					"percentUsedWithoutRequest": percentUsedWithoutRequest,
					"percentLimit":              percentLimit,
					"spaceReserve":              spaceReserve,
				}).Debugf("Checking usage percentage limits")

				if percentUsedWithoutRequest >= percentLimit {
					errorMessage := fmt.Sprintf("aggregate usage of %.2f %% exceeds the limit of %.2f %%",
						percentUsedWithoutRequest, percentLimit)
					return errors.New(errorMessage)
				}
			}
		}

		Logc(ctx).Debugf("Request within specicifed limits, going to create.")
		return nil
	}

	return errors.New("could not find aggregate, cannot check aggregate provisioning limits for " + aggregate)
}

func GetVolumeSize(sizeBytes uint64, poolDefaultSizeBytes string) (uint64, error) {
	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(poolDefaultSizeBytes)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return 0, errors.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes))
	}
	return sizeBytes, nil
}

func GetSnapshotReserve(snapshotPolicy, snapshotReserve string) (int, error) {
	if snapshotReserve != "" {
		// snapshotReserve defaults to "", so if it is explicitly set
		// (either in config or create options), honor the value.
		snapshotReserveInt64, err := strconv.ParseInt(snapshotReserve, 10, 64)
		if err != nil {
			return api.NumericalValueNotSet, err
		}
		return int(snapshotReserveInt64), nil
	} else {
		// If snapshotReserve isn't set, then look at snapshotPolicy.  If the policy is "none",
		// return 0.  Otherwise return -1, indicating that ONTAP should use its own default value.
		if snapshotPolicy == "none" || snapshotPolicy == "" {
			return 0, nil
		} else {
			snapshotReserveInt64, err := strconv.ParseInt(DefaultSnapshotReserve, 10, 64)
			if err != nil {
				return api.NumericalValueNotSet, err
			}
			return int(snapshotReserveInt64), nil
		}
	}
}

const MSecPerHour = 1000 * 60 * 60 // millis * seconds * minutes

// EMSHeartbeat logs an ASUP message on a timer
// view them via filer::> event log show -severity NOTICE
func EMSHeartbeat(ctx context.Context, driver StorageDriver) {
	// log an informational message on a timer
	hostname, err := os.Hostname()
	if err != nil {
		Logc(ctx).Warnf("Could not determine hostname. %v", err)
		hostname = "unknown"
	}

	message, _ := json.Marshal(driver.GetTelemetry())

	driver.GetAPI().EmsAutosupportLog(ctx, driver.Name(), strconv.Itoa(drivers.ConfigVersion), false, "heartbeat",
		hostname, string(message), 1, tridentconfig.OrchestratorName, 5)
}

// RestoreSnapshot restores a volume (in place) from a snapshot.
func RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "RestoreSnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreSnapshot")

	if err := client.SnapshotRestoreVolume(ctx, internalSnapName, internalVolName); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Debug("Restored snapshot.")

	return nil
}

// SplitVolumeFromBusySnapshot gets the list of volumes backed by a busy snapshot and starts
// a split operation on the first one (sorted by volume name).
func SplitVolumeFromBusySnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, cloneSplitStart func(ctx context.Context, cloneName string) error,
) error {
	fields := LogFields{
		"Method":       "SplitVolumeFromBusySnapshot",
		"Type":         "ontap_common",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> SplitVolumeFromBusySnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< SplitVolumeFromBusySnapshot")

	childVolumes, err := client.VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName)
	if err != nil {
		return err
	} else if childVolumes == nil || len(childVolumes) == 0 {
		return nil
	}

	if err := cloneSplitStart(ctx, childVolumes[0]); err != nil {
		Logc(ctx).WithFields(LogFields{
			"snapshotName":     snapConfig.InternalName,
			"parentVolumeName": snapConfig.VolumeInternalName,
			"cloneVolumeName":  childVolumes[0],
			"error":            err,
		}).Error("Could not begin splitting clone from snapshot.")
		return fmt.Errorf("error splitting clone: %v", err)
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName":     snapConfig.InternalName,
		"parentVolumeName": snapConfig.VolumeInternalName,
		"cloneVolumeName":  childVolumes[0],
	}).Info("Began splitting clone from snapshot.")

	return nil
}

// getVolumeExternal is a method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func getVolumeExternalCommon(
	volume api.Volume, storagePrefix, svmName string,
) *storage.VolumeExternal {
	internalName := volume.Name
	name := internalName
	if strings.HasPrefix(internalName, storagePrefix) {
		name = internalName[len(storagePrefix):]
	}

	volumeConfig := &storage.VolumeConfig{
		Version:         tridentconfig.OrchestratorAPIVersion,
		Name:            name,
		InternalName:    internalName,
		Size:            volume.Size,
		Protocol:        tridentconfig.File,
		SnapshotPolicy:  volume.SnapshotPolicy,
		SnapshotReserve: strconv.Itoa(volume.SnapshotReserve),
		ExportPolicy:    volume.ExportPolicy,
		SnapshotDir:     strconv.FormatBool(volume.SnapshotDir),
		UnixPermissions: volume.UnixPermissions,
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      utils.VolumeAccessInfo{},
		BlockSize:       "",
		FileSystem:      "",
	}

	pool := drivers.UnsetPool
	if len(volume.Aggregates) > 0 {
		if len(volume.Aggregates) == 1 {
			pool = volume.Aggregates[0]
		} else {
			pool = svmName
		}
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   pool,
	}
}

const (
	ontapHDD    ontapPerformanceClass = "hdd"
	ontapHybrid ontapPerformanceClass = "hybrid"
	ontapSSD    ontapPerformanceClass = "ssd"
)

var ontapPerformanceClasses = map[ontapPerformanceClass]map[string]sa.Offer{
	ontapHDD:    {sa.Media: sa.NewStringOffer(sa.HDD)},
	ontapHybrid: {sa.Media: sa.NewStringOffer(sa.Hybrid)},
	ontapSSD:    {sa.Media: sa.NewStringOffer(sa.SSD)},
}

// discoverBackendAggrNamesCommon discovers names of the aggregates assigned to the configured SVM
func discoverBackendAggrNamesCommon(ctx context.Context, d StorageDriver) ([]string, error) {
	client := d.GetAPI()
	config := d.GetConfig()
	driverName := d.Name()
	var err error

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := client.GetSVMAggregateNames(ctx)
	if err != nil {
		return nil, err
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", client.SVMName())
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"svm":   client.SVMName(),
		"pools": vserverAggrs,
	}).Debug("Read storage pools assigned to SVM.")

	var aggrNames []string
	for _, aggrName := range vserverAggrs {
		if config.Aggregate != "" {
			if aggrName != config.Aggregate {
				continue
			}

			Logc(ctx).WithFields(LogFields{
				"driverName": driverName,
				"aggregate":  config.Aggregate,
			}).Debug("Provisioning will be restricted to the aggregate set in the backend config.")
		}

		aggrNames = append(aggrNames, aggrName)
	}

	// Make sure the configured aggregate is available to the SVM
	if config.Aggregate != "" && (len(aggrNames) == 0) {
		err = fmt.Errorf("the assigned aggregates for SVM %s do not include the configured aggregate %s",
			client.SVMName(), config.Aggregate)
		return nil, err
	}

	return aggrNames, nil
}

// getVserverAggrAttributes gets pool attributes using vserver-show-aggr-get-iter,
// which will only succeed on Data ONTAP 9 and later.
// If the aggregate attributes are read successfully, the pools passed to this function are updated accordingly.
func getVserverAggrAttributes(
	ctx context.Context, d StorageDriver, poolsAttributeMap *map[string]map[string]sa.Offer,
) (err error) {
	aggrList, err := d.GetAPI().GetSVMAggregateAttributes(ctx)
	if err != nil {
		return err
	}

	if aggrList != nil {
		for aggrName, aggrType := range aggrList {
			// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
			_, ok := (*poolsAttributeMap)[aggrName]
			if !ok {
				continue
			}

			fields := LogFields{"aggregate": aggrName, "mediaType": aggrType}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				Logc(ctx).WithFields(fields).Debug("Aggregate has unknown performance characteristics.")
				continue
			}

			Logc(ctx).WithFields(fields).Debug("Read aggregate attributes.")

			// Update the pool with the aggregate storage attributes
			for attrName, attr := range storageAttrs {
				(*poolsAttributeMap)[aggrName][attrName] = attr
			}
		}
	}

	return
}

// poolName constructs the name of the pool reported by this driver instance
func poolName(name, backendName string) string {
	return fmt.Sprintf("%s_%s", backendName, strings.Replace(name, "-", "", -1))
}

func InitializeStoragePoolsCommon(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	config := d.GetConfig()
	physicalPools := make(map[string]storage.Pool)
	virtualPools := make(map[string]storage.Pool)

	// To identify list of media types supported by physical pools
	mediaOffers := make([]sa.Offer, 0)

	// Get name of the physical storage pools which in case of ONTAP is list of aggregates
	physicalStoragePoolNames, err := discoverBackendAggrNamesCommon(ctx, d)
	if err != nil || len(physicalStoragePoolNames) == 0 {
		return physicalPools, virtualPools, fmt.Errorf("could not get storage pools from array: %v", err)
	}

	// Create a map of Physical storage pool name to their attributes map
	physicalStoragePoolAttributes := make(map[string]map[string]sa.Offer)
	for _, physicalStoragePoolName := range physicalStoragePoolNames {
		physicalStoragePoolAttributes[physicalStoragePoolName] = make(map[string]sa.Offer)
	}

	// Update physical pool attributes map with aggregate info (i.e. MediaType)
	aggrErr := getVserverAggrAttributes(ctx, d, &physicalStoragePoolAttributes)

	if zerr, ok := aggrErr.(azgo.ZapiError); ok && zerr.IsScopeError() {
		Logc(ctx).WithFields(LogFields{
			"username": config.Username,
		}).Warn("User has insufficient privileges to obtain aggregate info. " +
			"Storage classes with physical attributes such as 'media' will not match pools on this backend.")
	} else if aggrErr != nil {
		Logc(ctx).Errorf("Could not obtain aggregate info; storage classes with physical attributes such as 'media'"+
			" will not match pools on this backend: %v.", aggrErr)
	}

	// Define physical pools
	for _, physicalStoragePoolName := range physicalStoragePoolNames {

		pool := storage.NewStoragePool(nil, physicalStoragePoolName)

		// Update pool with attributes set by default for this backend
		// We do not set internal attributes with these values as this
		// merely means that pools supports these capabilities like
		// encryption, cloning, thick/thin provisioning
		for attrName, offer := range poolAttributes {
			pool.Attributes()[attrName] = offer
		}

		attrMap := physicalStoragePoolAttributes[physicalStoragePoolName]

		// Update pool with attributes based on aggregate attributes discovered on the backend
		for attrName, attrValue := range attrMap {
			pool.Attributes()[attrName] = attrValue
			pool.InternalAttributes()[attrName] = attrValue.ToString()

			if attrName == sa.Media {
				mediaOffers = append(mediaOffers, attrValue)
			}
		}

		if config.Region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(config.Region)
		}
		if config.Zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(config.Zone)
		}

		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels)
		pool.Attributes()[sa.NASType] = sa.NewStringOffer(config.NASType)
		pool.Attributes()[sa.SANType] = sa.NewStringOffer(config.SANType)

		pool.InternalAttributes()[Size] = config.Size
		pool.InternalAttributes()[Region] = config.Region
		pool.InternalAttributes()[Zone] = config.Zone
		pool.InternalAttributes()[SpaceReserve] = config.SpaceReserve
		pool.InternalAttributes()[SnapshotPolicy] = config.SnapshotPolicy
		pool.InternalAttributes()[SnapshotReserve] = config.SnapshotReserve
		pool.InternalAttributes()[SplitOnClone] = config.SplitOnClone
		pool.InternalAttributes()[Encryption] = config.Encryption
		pool.InternalAttributes()[LUKSEncryption] = config.LUKSEncryption
		pool.InternalAttributes()[UnixPermissions] = config.UnixPermissions
		pool.InternalAttributes()[SnapshotDir] = config.SnapshotDir
		pool.InternalAttributes()[ExportPolicy] = config.ExportPolicy
		pool.InternalAttributes()[SecurityStyle] = config.SecurityStyle
		pool.InternalAttributes()[TieringPolicy] = config.TieringPolicy
		pool.InternalAttributes()[QosPolicy] = config.QosPolicy
		pool.InternalAttributes()[AdaptiveQosPolicy] = config.AdaptiveQosPolicy

		pool.SetSupportedTopologies(config.SupportedTopologies)

		if d.Name() == tridentconfig.OntapSANStorageDriverName || d.Name() == tridentconfig.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes()[SpaceAllocation] = config.SpaceAllocation
			pool.InternalAttributes()[FileSystemType] = config.FileSystemType
		}

		physicalPools[pool.Name()] = pool
	}

	// Define virtual pools
	for index, vpool := range config.Storage {

		region := config.Region
		if vpool.Region != "" {
			region = vpool.Region
		}

		zone := config.Zone
		if vpool.Zone != "" {
			zone = vpool.Zone
		}

		size := config.Size
		if vpool.Size != "" {
			size = vpool.Size
		}
		supportedTopologies := config.SupportedTopologies
		if vpool.SupportedTopologies != nil {
			supportedTopologies = vpool.SupportedTopologies
		}

		spaceAllocation := config.SpaceAllocation
		if vpool.SpaceAllocation != "" {
			spaceAllocation = vpool.SpaceAllocation
		}

		spaceReserve := config.SpaceReserve
		if vpool.SpaceReserve != "" {
			spaceReserve = vpool.SpaceReserve
		}

		snapshotPolicy := config.SnapshotPolicy
		if vpool.SnapshotPolicy != "" {
			snapshotPolicy = vpool.SnapshotPolicy
		}

		snapshotReserve := config.SnapshotReserve
		if vpool.SnapshotReserve != "" {
			snapshotReserve = vpool.SnapshotReserve
		}

		splitOnClone := config.SplitOnClone
		if vpool.SplitOnClone != "" {
			splitOnClone = vpool.SplitOnClone
		}

		unixPermissions := config.UnixPermissions
		if vpool.UnixPermissions != "" {
			unixPermissions = vpool.UnixPermissions
		}

		snapshotDir := config.SnapshotDir
		if vpool.SnapshotDir != "" {
			snapshotDir = vpool.SnapshotDir
		}

		exportPolicy := config.ExportPolicy
		if vpool.ExportPolicy != "" {
			exportPolicy = vpool.ExportPolicy
		}

		securityStyle := config.SecurityStyle
		if vpool.SecurityStyle != "" {
			securityStyle = vpool.SecurityStyle
		}

		fileSystemType := config.FileSystemType
		if vpool.FileSystemType != "" {
			fileSystemType = vpool.FileSystemType
		}

		encryption := config.Encryption
		if vpool.Encryption != "" {
			encryption = vpool.Encryption
		}

		luksEncryption := config.LUKSEncryption
		if vpool.LUKSEncryption != "" {
			luksEncryption = vpool.LUKSEncryption
		}

		tieringPolicy := config.TieringPolicy
		if vpool.TieringPolicy != "" {
			tieringPolicy = vpool.TieringPolicy
		}

		qosPolicy := config.QosPolicy
		if vpool.QosPolicy != "" {
			qosPolicy = vpool.QosPolicy
		}

		adaptiveQosPolicy := config.AdaptiveQosPolicy
		if vpool.AdaptiveQosPolicy != "" {
			adaptiveQosPolicy = vpool.AdaptiveQosPolicy
		}

		pool := storage.NewStoragePool(nil, poolName(fmt.Sprintf("pool_%d", index), backendName))

		// Update pool with attributes set by default for this backend
		// We do not set internal attributes with these values as this
		// merely means that pools supports these capabilities like
		// encryption, cloning, thick/thin provisioning
		for attrName, offer := range poolAttributes {
			pool.Attributes()[attrName] = offer
		}

		nasType := config.NASType
		if vpool.NASType != "" {
			nasType = vpool.NASType
		}

		sanType := config.SANType
		if vpool.SANType != "" {
			sanType = vpool.SANType
		}

		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels, vpool.Labels)
		pool.Attributes()[sa.NASType] = sa.NewStringOffer(nasType)
		pool.Attributes()[sa.SANType] = sa.NewStringOffer(sanType)

		if region != "" {
			pool.Attributes()[sa.Region] = sa.NewStringOffer(region)
		}
		if zone != "" {
			pool.Attributes()[sa.Zone] = sa.NewStringOffer(zone)
		}
		if len(mediaOffers) > 0 {
			pool.Attributes()[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
			pool.InternalAttributes()[Media] = pool.Attributes()[sa.Media].ToString()
		}
		if encryption != "" {
			enableEncryption, err := strconv.ParseBool(encryption)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err,
					pool.Name())
			}
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(enableEncryption)
			pool.InternalAttributes()[Encryption] = encryption
		}

		pool.InternalAttributes()[Size] = size
		pool.InternalAttributes()[Region] = region
		pool.InternalAttributes()[Zone] = zone
		pool.InternalAttributes()[SpaceReserve] = spaceReserve
		pool.InternalAttributes()[SnapshotPolicy] = snapshotPolicy
		pool.InternalAttributes()[SnapshotReserve] = snapshotReserve
		pool.InternalAttributes()[SplitOnClone] = splitOnClone
		pool.InternalAttributes()[UnixPermissions] = unixPermissions
		pool.InternalAttributes()[SnapshotDir] = snapshotDir
		pool.InternalAttributes()[ExportPolicy] = exportPolicy
		pool.InternalAttributes()[SecurityStyle] = securityStyle
		pool.InternalAttributes()[TieringPolicy] = tieringPolicy
		pool.InternalAttributes()[QosPolicy] = qosPolicy
		pool.InternalAttributes()[LUKSEncryption] = luksEncryption
		pool.InternalAttributes()[AdaptiveQosPolicy] = adaptiveQosPolicy
		pool.SetSupportedTopologies(supportedTopologies)

		if d.Name() == tridentconfig.OntapSANStorageDriverName || d.Name() == tridentconfig.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes()[SpaceAllocation] = spaceAllocation
			pool.InternalAttributes()[FileSystemType] = fileSystemType
		}

		virtualPools[pool.Name()] = pool
	}

	return physicalPools, virtualPools, nil
}

// ValidateStoragePools makes sure that values are set for the fields, if value(s) were not specified
// for a field then a default should have been set in for that field in the initialize storage pools
func ValidateStoragePools(
	ctx context.Context, physicalPools, virtualPools map[string]storage.Pool, d StorageDriver, labelLimit int,
) error {
	config := d.GetConfig()

	// Validate pool-level attributes
	allPools := make([]storage.Pool, 0, len(physicalPools)+len(virtualPools))

	for _, pool := range physicalPools {
		allPools = append(allPools, pool)
	}
	for _, pool := range virtualPools {
		allPools = append(allPools, pool)
	}

	for _, pool := range allPools {

		poolName := pool.Name()

		// Validate SpaceReserve
		switch pool.InternalAttributes()[SpaceReserve] {
		case "none", "volume":
			break
		default:
			return fmt.Errorf("invalid spaceReserve %s in pool %s", pool.InternalAttributes()[SpaceReserve], poolName)
		}

		// Validate SnapshotPolicy
		if pool.InternalAttributes()[SnapshotPolicy] == "" {
			return fmt.Errorf("snapshot policy cannot by empty in pool %s", poolName)
		}

		// Validate Encryption
		if pool.InternalAttributes()[Encryption] != "" {
			_, err := strconv.ParseBool(pool.InternalAttributes()[Encryption])
			if err != nil {
				return fmt.Errorf("invalid value for encryption in pool %s: %v", poolName, err)
			}
		}
		// Validate snapshot dir
		if pool.InternalAttributes()[SnapshotDir] == "" {
			return fmt.Errorf("snapshotDir cannot by empty in pool %s", poolName)
		} else {
			_, err := strconv.ParseBool(pool.InternalAttributes()[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s: %v", poolName, err)
			}
		}

		_, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit)
		if err != nil {
			return fmt.Errorf("invalid value for label in pool %s: %v", poolName, err)
		}

		// Validate SecurityStyles
		// SMB supports "mixed" and "ntfs" security styles.
		// NFS supports "mixed" and "unix" security styles.
		if config.NASType == sa.SMB {
			switch pool.InternalAttributes()[SecurityStyle] {
			case "mixed", "ntfs":
				break
			default:
				return fmt.Errorf("invalid securityStyle %s, for NASType %s in pool %s",
					pool.InternalAttributes()[SecurityStyle], config.NASType, poolName)
			}
		} else {
			switch pool.InternalAttributes()[SecurityStyle] {
			case "mixed", "unix":
				break
			default:
				return fmt.Errorf("invalid securityStyle %s, for NASType %s in pool %s",
					pool.InternalAttributes()[SecurityStyle], config.NASType, poolName)
			}
		}

		// Validate ExportPolicy
		if pool.InternalAttributes()[ExportPolicy] == "" {
			return fmt.Errorf("export policy cannot by empty in pool %s", poolName)
		}

		// Validate UnixPermissions
		if config.NASType == sa.NFS {
			if pool.InternalAttributes()[UnixPermissions] == "" {
				return fmt.Errorf("UNIX permissions cannot by empty in pool %s", poolName)
			}
		}

		// Validate TieringPolicy
		switch pool.InternalAttributes()[TieringPolicy] {
		case "snapshot-only", "auto", "none", "backup", "all", "":
			break
		default:
			return fmt.Errorf("invalid tieringPolicy %s in pool %s", pool.InternalAttributes()[TieringPolicy], poolName)
		}

		// Validate QoS policy or adaptive QoS policy
		if pool.InternalAttributes()[QosPolicy] != "" || pool.InternalAttributes()[AdaptiveQosPolicy] != "" {
			if !d.GetAPI().SupportsFeature(ctx, api.QosPolicies) {
				return fmt.Errorf("trident does not support QoS policies for ONTAP version")
			}

			if _, err := api.NewQosPolicyGroup(
				pool.InternalAttributes()[QosPolicy], pool.InternalAttributes()[AdaptiveQosPolicy],
			); err != nil {
				return err
			}

			if d.Name() == tridentconfig.OntapNASQtreeStorageDriverName && pool.InternalAttributes()[AdaptiveQosPolicy] != "" {
				return fmt.Errorf("qtrees do not support adaptive QoS policies")
			}
		}

		// Validate media type
		if pool.InternalAttributes()[Media] != "" {
			for _, mediaType := range strings.Split(pool.InternalAttributes()[Media], ",") {
				switch mediaType {
				case sa.HDD, sa.SSD, sa.Hybrid:
					break
				default:
					Logc(ctx).Errorf("invalid media type in pool %s: %s", pool.Name(), mediaType)
				}
			}
		}

		// Validate default size
		if defaultSize, err := utils.ConvertSizeToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		} else {
			sizeBytes, _ := strconv.ParseUint(defaultSize, 10, 64)
			if sizeBytes < MinimumVolumeSizeBytes {
				return fmt.Errorf("invalid value for size in pool %s. "+
					"Requested volume size ("+
					"%d bytes) is too small; the minimum volume size is %d bytes", poolName, sizeBytes,
					MinimumVolumeSizeBytes)
			}
		}

		// Cloning is not supported on ONTAP FlexGroups driver
		if d.Name() != tridentconfig.OntapNASFlexGroupStorageDriverName {
			// Validate splitOnClone
			if pool.InternalAttributes()[SplitOnClone] == "" {
				return fmt.Errorf("splitOnClone cannot by empty in pool %s", poolName)
			} else {
				_, err := strconv.ParseBool(pool.InternalAttributes()[SplitOnClone])
				if err != nil {
					return fmt.Errorf("invalid value for splitOnClone in pool %s: %v", poolName, err)
				}
			}
		}

		// Validate LUKS configuration, only boolean value and only supported by ONTAP NAS backends
		if _, ok := pool.InternalAttributes()[LUKSEncryption]; ok {
			isLuks, err := strconv.ParseBool(pool.InternalAttributes()[LUKSEncryption])
			if err != nil {
				return fmt.Errorf("could not parse LUKSEncryption from volume config into a boolean, got %v",
					pool.InternalAttributes()[LUKSEncryption])
			}
			if isLuks && !(d.Name() == tridentconfig.OntapSANStorageDriverName || d.Name() == tridentconfig.OntapSANEconomyStorageDriverName) {
				return fmt.Errorf("LUKS encrypted volumes are only supported by the following drivers: %s, %s",
					tridentconfig.OntapSANStorageDriverName, tridentconfig.OntapSANEconomyStorageDriverName)
			}
		}

		if d.Name() == tridentconfig.OntapSANStorageDriverName || d.Name() == tridentconfig.OntapSANEconomyStorageDriverName {

			// Validate SpaceAllocation
			if pool.InternalAttributes()[SpaceAllocation] == "" {
				return fmt.Errorf("spaceAllocation cannot by empty in pool %s", poolName)
			} else {
				_, err := strconv.ParseBool(pool.InternalAttributes()[SpaceAllocation])
				if err != nil {
					return fmt.Errorf("invalid value for SpaceAllocation in pool %s: %v", poolName, err)
				}
			}

			// Validate FileSystemType
			if pool.InternalAttributes()[FileSystemType] == "" {
				return fmt.Errorf("fileSystemType cannot by empty in pool %s", poolName)
			} else {
				_, err := drivers.CheckSupportedFilesystem(ctx, pool.InternalAttributes()[FileSystemType], "")
				if err != nil {
					return fmt.Errorf("invalid value for fileSystemType in pool %s: %v", poolName, err)
				}
			}
		}
	}

	return nil
}

// getStorageBackendSpecsCommon updates the specified Backend object with StoragePools.
func getStorageBackendSpecsCommon(
	backend storage.Backend, physicalPools, virtualPools map[string]storage.Pool, backendName string,
) (err error) {
	backend.SetName(backendName)

	virtual := len(virtualPools) > 0

	for _, pool := range physicalPools {
		pool.SetBackend(backend)
		if !virtual {
			backend.AddStoragePool(pool)
		}
	}

	for _, pool := range virtualPools {
		pool.SetBackend(backend)
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

func getVolumeOptsCommon(
	ctx context.Context, volConfig *storage.VolumeConfig, requests map[string]sa.Request,
) map[string]string {
	opts := make(map[string]string)
	if provisioningTypeReq, ok := requests[sa.ProvisioningType]; ok {
		if p, ok := provisioningTypeReq.Value().(string); ok {
			if p == "thin" {
				opts["spaceReserve"] = "none"
			} else if p == "thick" {
				// p will equal "thick" here
				opts["spaceReserve"] = "volume"
			} else {
				Logc(ctx).WithFields(LogFields{
					"provisioner":      "ONTAP",
					"method":           "getVolumeOptsCommon",
					"provisioningType": provisioningTypeReq.Value(),
				}).Warnf("Expected 'thick' or 'thin' for %s; ignoring.",
					sa.ProvisioningType)
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"provisioner":      "ONTAP",
				"method":           "getVolumeOptsCommon",
				"provisioningType": provisioningTypeReq.Value(),
			}).Warnf("Expected string for %s; ignoring.", sa.ProvisioningType)
		}
	}
	if encryptionReq, ok := requests[sa.Encryption]; ok {
		if encryption, ok := encryptionReq.Value().(bool); ok {
			if encryption {
				opts["encryption"] = "true"
			} else {
				opts["encryption"] = "false"
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"provisioner": "ONTAP",
				"method":      "getVolumeOptsCommon",
				"encryption":  encryptionReq.Value(),
			}).Warnf("Expected bool for %s; ignoring.", sa.Encryption)
		}
	}
	if volConfig.SnapshotPolicy != "" {
		opts["snapshotPolicy"] = volConfig.SnapshotPolicy
	}
	if volConfig.SnapshotReserve != "" {
		opts["snapshotReserve"] = volConfig.SnapshotReserve
	}
	if volConfig.UnixPermissions != "" {
		opts["unixPermissions"] = volConfig.UnixPermissions
	}
	if volConfig.SnapshotDir != "" {
		opts["snapshotDir"] = volConfig.SnapshotDir
	}
	if volConfig.ExportPolicy != "" {
		opts["exportPolicy"] = volConfig.ExportPolicy
	}
	if volConfig.SpaceReserve != "" {
		opts["spaceReserve"] = volConfig.SpaceReserve
	}
	if volConfig.SecurityStyle != "" {
		opts["securityStyle"] = volConfig.SecurityStyle
	}
	if volConfig.SplitOnClone != "" {
		opts["splitOnClone"] = volConfig.SplitOnClone
	}
	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}
	if volConfig.Encryption != "" {
		opts["encryption"] = volConfig.Encryption
	}
	if volConfig.QosPolicy != "" {
		opts["qosPolicy"] = volConfig.QosPolicy
	}
	if volConfig.AdaptiveQosPolicy != "" {
		opts["adaptiveQosPolicy"] = volConfig.AdaptiveQosPolicy
	}

	return opts
}

func getStorageBackendPhysicalPoolNamesCommon(physicalPools map[string]storage.Pool) []string {
	physicalPoolNames := make([]string, 0)
	for poolName := range physicalPools {
		physicalPoolNames = append(physicalPoolNames, poolName)
	}
	return physicalPoolNames
}

func getPoolsForCreate(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool storage.Pool,
	volAttributes map[string]sa.Request, physicalPools, virtualPools map[string]storage.Pool,
) ([]storage.Pool, error) {
	// If a physical pool was requested, just use it
	if _, ok := physicalPools[storagePool.Name()]; ok {
		return []storage.Pool{storagePool}, nil
	}

	// If a virtual pool was requested, find a physical pool to satisfy it
	if _, ok := virtualPools[storagePool.Name()]; !ok {
		return nil, fmt.Errorf("could not find pool %s", storagePool.Name())
	}

	// Make a storage class from the volume attributes to simplify pool matching
	attributesCopy := make(map[string]sa.Request)
	for k, v := range volAttributes {
		attributesCopy[k] = v
	}
	delete(attributesCopy, sa.Selector)
	storageClass := sc.NewFromAttributes(attributesCopy)

	// Find matching pools
	candidatePools := make([]storage.Pool, 0)

	for _, pool := range physicalPools {
		if storageClass.Matches(ctx, pool) {
			candidatePools = append(candidatePools, pool)
		}
	}

	if len(candidatePools) == 0 {
		err := fmt.Errorf("backend has no physical pools that can satisfy request")
		return nil, drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}

	// Shuffle physical pools
	rand.Shuffle(len(candidatePools), func(i, j int) {
		candidatePools[i], candidatePools[j] = candidatePools[j], candidatePools[i]
	})

	return candidatePools, nil
}

func getInternalVolumeNameCommon(commonConfig *drivers.CommonStorageDriverConfig, name string) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *commonConfig.StoragePrefix + name
	} else {
		// With an external store, any transformation of the name is fine
		internal := drivers.GetCommonInternalVolumeName(commonConfig, name)
		internal = strings.Replace(internal, "-", "_", -1)  // ONTAP disallows hyphens
		internal = strings.Replace(internal, ".", "_", -1)  // ONTAP disallows periods
		internal = strings.Replace(internal, "__", "_", -1) // Remove any double underscores
		return internal
	}
}

func createPrepareCommon(ctx context.Context, d storage.Driver, volConfig *storage.VolumeConfig) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig.Name)
}

func getExternalConfig(ctx context.Context, config drivers.OntapStorageDriverConfig) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.OntapStorageDriverConfig
	drivers.Clone(ctx, config, &cloneConfig)

	drivers.SanitizeCommonStorageDriverConfig(cloneConfig.CommonStorageDriverConfig)

	cloneConfig.Username = utils.REDACTED         // redact the username
	cloneConfig.Password = utils.REDACTED         // redact the password
	cloneConfig.ClientPrivateKey = utils.REDACTED // redact the client private key
	cloneConfig.ChapInitiatorSecret = utils.REDACTED
	cloneConfig.ChapTargetInitiatorSecret = utils.REDACTED
	cloneConfig.ChapTargetUsername = utils.REDACTED
	cloneConfig.ChapUsername = utils.REDACTED
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: utils.REDACTED,
		drivers.KeyType: utils.REDACTED,
	} // redact the credentials
	return cloneConfig
}

func calculateFlexvolEconomySizeBytes(
	ctx context.Context, flexvol string, volAttrs *api.Volume, newLunOrQtreeSizeBytes, totalDiskLimitBytes uint64,
) uint64 {
	snapReserveDivisor := 1.0 - (float64(volAttrs.SnapshotReserve) / 100.0)
	snapshotSizeBytes := float64(volAttrs.SnapshotSpaceUsed)

	usableSpaceBytes := float64(newLunOrQtreeSizeBytes + totalDiskLimitBytes)
	usableSpaceWithSnapshots := usableSpaceBytes + snapshotSizeBytes
	usableSpaceSnapReserve := float64(usableSpaceBytes / snapReserveDivisor)

	var flexvolSizeBytes uint64
	if usableSpaceSnapReserve < usableSpaceWithSnapshots {
		flexvolSizeBytes = uint64(usableSpaceWithSnapshots)
	} else {
		flexvolSizeBytes = uint64(usableSpaceSnapReserve)
	}

	Logc(ctx).WithFields(LogFields{
		"flexvol":                flexvol,
		"snapshotReserve":        volAttrs.SnapshotReserve,
		"snapReserveDivisor":     snapReserveDivisor,
		"snapshotSizeBytes":      snapshotSizeBytes,
		"totalDiskLimitBytes":    totalDiskLimitBytes,
		"newLunOrQtreeSizeBytes": newLunOrQtreeSizeBytes,
		"spaceWithSnapshots":     usableSpaceWithSnapshots,
		"spaceWithSnapReserve":   usableSpaceSnapReserve,
		"flexvolSizeBytes":       flexvolSizeBytes,
	}).Debug("Calculated optimal size for Flexvol with new LUN or QTree.")

	return flexvolSizeBytes
}

// calculateFlexvolSizeBytes calculates the size of the Flexvol taking into account the snapshot reserve
func calculateFlexvolSizeBytes(
	ctx context.Context, flexvol string, requestedSizeBytes uint64, snapshotReserve int,
) uint64 {
	snapReserveDivisor := 1.0 - (float64(snapshotReserve) / 100.0)

	sizeWithSnapReserve := float64(requestedSizeBytes) / snapReserveDivisor

	flexvolSizeBytes := uint64(sizeWithSnapReserve)

	Logc(ctx).WithFields(LogFields{
		"flexvol":             flexvol,
		"snapReserveDivisor":  snapReserveDivisor,
		"requestedSize":       requestedSizeBytes,
		"sizeWithSnapReserve": sizeWithSnapReserve,
		"flexvolSizeBytes":    flexvolSizeBytes,
	}).Debug("Calculated optimal size for Flexvol with snapshot reserve.")

	return flexvolSizeBytes
}

type GetVolumeInfoFunc func(ctx context.Context, volumeName string) (volume *api.Volume, err error)

// getSnapshotReserveFromOntap takes a volume name and retrieves the snapshot policy and snapshot reserve
func getSnapshotReserveFromOntap(
	ctx context.Context, name string, GetVolumeInfo GetVolumeInfoFunc,
) (int, error) {
	snapshotPolicy := ""
	snapshotReserveInt := 0

	info, err := GetVolumeInfo(ctx, name)
	if err != nil {
		return snapshotReserveInt, fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	snapshotPolicy = info.SnapshotPolicy
	snapshotReserveInt = info.SnapshotReserve

	snapshotReserveInt, err = GetSnapshotReserve(snapshotPolicy, strconv.Itoa(snapshotReserveInt))
	if err != nil {
		return snapshotReserveInt, fmt.Errorf("invalid value for snapshotReserve: %v", err)
	}

	return snapshotReserveInt, nil
}

func isFlexvolRW(ctx context.Context, ontap api.OntapAPI, name string) (bool, error) {
	flexvol, err := ontap.VolumeInfo(ctx, name)
	if err != nil {
		Logc(ctx).Error(err)
		return false, fmt.Errorf("could not get volume %v", name)
	}

	if flexvol.AccessType == VolTypeRW {
		return true, nil
	}
	return false, nil
}

// getVolumeSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func getVolumeSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "getVolumeSnapshot",
		"Type":         "NASStorageDriver",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snap, err := client.VolumeSnapshotInfo(ctx, internalSnapName, internalVolName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, nil
		} else {
			return nil, err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      snap.CreateTime,
	}).Debug("Found snapshot.")
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snap.CreateTime,
		SizeBytes: int64(size),
		State:     storage.SnapshotStateOnline,
	}, nil
}

// getVolumeSnapshotList returns the list of snapshots associated with the named volume.
func getVolumeSnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	fields := LogFields{
		"Method":     "getVolumeSnapshotList",
		"Type":       "NASStorageDriver",
		"volumeName": internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getVolumeSnapshotList")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getVolumeSnapshotList")

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	result := make([]*storage.Snapshot, 0)

	for _, snap := range snapshots {

		Logc(ctx).WithFields(LogFields{
			"name":       snap.Name,
			"accessTime": snap.CreateTime,
		}).Debug("Snapshot")

		snapshot := &storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Version:            tridentconfig.OrchestratorAPIVersion,
				Name:               snap.Name,
				InternalName:       snap.Name,
				VolumeName:         volConfig.Name,
				VolumeInternalName: volConfig.InternalName,
			},
			Created:   snap.CreateTime,
			SizeBytes: int64(size),
			State:     storage.SnapshotStateOnline,
		}

		result = append(result, snapshot)
	}

	return result, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func createFlexvolSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "CreateSnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateSnapshot")

	// If the specified volume doesn't exist, return error
	volExists, err := client.VolumeExists(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		return nil, fmt.Errorf("volume %s does not exist", internalVolName)
	}

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	if err := client.VolumeSnapshotCreate(ctx, internalSnapName, internalVolName); err != nil {
		return nil, err
	}

	snap, err := client.VolumeSnapshotInfo(ctx, internalSnapName, internalVolName)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      snap.CreateTime,
	}).Debug("Found snapshot.")
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   snap.CreateTime,
		SizeBytes: int64(size),
		State:     storage.SnapshotStateOnline,
	}, nil
}

// cloneFlexvol creates a volume clone
func cloneFlexvol(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, qosPolicyGroup api.QosPolicyGroup,
) error {
	fields := LogFields{
		"Method":   "cloneFlexvol",
		"Type":     "ontap_common",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
		"split":    split,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> cloneFlexvol")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< cloneFlexvol")

	// If the specified volume already exists, return an error
	volExists, err := client.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if volExists {
		return fmt.Errorf("volume %s already exists", name)
	}

	// If no specific snapshot was requested, create one
	if snapshot == "" {
		snapshot = time.Now().UTC().Format(storage.SnapshotNameFormat)
		if err = client.VolumeSnapshotCreate(ctx, snapshot, source); err != nil {
			return err
		}
	}

	// Create the clone based on a snapshot
	if err = client.VolumeCloneCreate(ctx, name, source, snapshot, false); err != nil {
		return err
	}

	desiredStates, abortStates := []string{"online"}, []string{"error"}
	volState, err := client.VolumeWaitForStates(ctx, name, desiredStates, abortStates, maxFlexvolCloneWait)
	if err != nil {
		return fmt.Errorf("unable to create flexClone for volume %v, volState:%v", name, volState)
	}

	if err = client.VolumeSetComment(ctx, name, name, labels); err != nil {
		return err
	}

	if config.StorageDriverName == tridentconfig.OntapNASStorageDriverName {
		// Mount the new volume
		if err = client.VolumeMount(ctx, name, "/"+name); err != nil {
			return err
		}
	}

	// Set the QoS Policy if necessary
	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		if err := client.VolumeSetQosPolicyGroupName(ctx, name, qosPolicyGroup); err != nil {
			return err
		}
	}

	// Split the clone if requested
	if split {
		if err := client.VolumeCloneSplitStart(ctx, name); err != nil {
			return fmt.Errorf("error splitting clone: %v", err)
		}
	}

	return nil
}

// LunUnmapAllIgroups removes all maps from a given LUN
func LunUnmapAllIgroups(ctx context.Context, clientAPI api.OntapAPI, lunPath string) error {
	Logc(ctx).WithField("LUN", lunPath).Debug("Unmapping LUN from all igroups.")

	igroups, err := clientAPI.LunListIgroupsMapped(ctx, lunPath)
	if err != nil {
		msg := "error listing igroup mappings"
		Logc(ctx).WithError(err).Errorf(msg)
		return fmt.Errorf(msg)
	}

	errored := false
	for _, igroup := range igroups {
		err = clientAPI.LunUnmap(ctx, igroup, lunPath)
		if err != nil {
			errored = true
			Logc(ctx).WithFields(LogFields{
				"LUN":    lunPath,
				"igroup": igroup,
			}).WithError(err).Error("Error unmapping LUN from igroup.")
		}
	}
	if errored {
		return fmt.Errorf("error unmapping one or more LUN mappings")
	}
	return nil
}

// LunUnmapIgroup removes a LUN from an igroup.
func LunUnmapIgroup(ctx context.Context, clientAPI api.OntapAPI, igroup, lunPath string) error {
	Logc(ctx).WithFields(LogFields{
		"LUN":    lunPath,
		"igroup": igroup,
	}).Debugf("Unmapping LUN from igroup.")

	lunID, err := clientAPI.LunMapInfo(ctx, igroup, lunPath)
	if err != nil {
		msg := fmt.Sprintf("error reading LUN maps")
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}

	if lunID >= 0 {
		err := clientAPI.LunUnmap(ctx, igroup, lunPath)
		if err != nil {
			msg := "error unmapping LUN"
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
	}

	return nil
}

// DestroyUnmappedIgroup removes an igroup iff no LUNs are mapped to it.
func DestroyUnmappedIgroup(ctx context.Context, clientAPI api.OntapAPI, igroup string) error {
	luns, err := clientAPI.IgroupListLUNsMapped(ctx, igroup)
	if err != nil {
		msg := fmt.Sprintf("error listing LUNs mapped to igroup %s", igroup)
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}

	if len(luns) == 0 {
		Logc(ctx).WithField(
			"igroup", igroup,
		).Debugf("No LUNs mapped to this igroup; deleting igroup.")
		if err := clientAPI.IgroupDestroy(ctx, igroup); err != nil {
			msg := fmt.Sprintf("error deleting igroup %s", igroup)
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
	} else {
		Logc(ctx).WithField(
			"igroup", igroup,
		).Debugf("Found LUNs mapped to this igroup; igroup deletion is not safe.")
	}

	return nil
}

// EnableSANPublishEnforcement unmaps a LUN from all igroups to allow per-node igroup mappings during publish volume.
func EnableSANPublishEnforcement(
	ctx context.Context, clientAPI api.OntapAPI, volumeConfig *storage.VolumeConfig, lunPath string,
) error {
	fields := LogFields{
		"volume": volumeConfig.Name,
		"LUN":    volumeConfig.InternalName,
	}
	Logc(ctx).WithFields(fields).Debug("Unmapping igroups for publish enforcement.")

	// Do not enable publish enforcement on unmanaged imports.
	if volumeConfig.ImportNotManaged {
		Logc(ctx).WithFields(fields).Debug("Unable to remove igroup mappings; imported LUN is not managed.")
		return nil
	}
	err := LunUnmapAllIgroups(ctx, clientAPI, lunPath)
	if err != nil {
		msg := "error removing all igroup mappings from LUN"
		Logc(ctx).WithFields(fields).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}

	volumeConfig.AccessInfo.IscsiLunNumber = -1
	volumeConfig.AccessInfo.PublishEnforcement = true
	volumeConfig.AccessInfo.IscsiIgroup = ""
	return nil
}

func ValidateStoragePrefixEconomy(storagePrefix string) error {
	// Ensure storage prefix is compatible with ONTAP
	matched, err := regexp.MatchString(`^$|^[a-zA-Z0-9_.-]*$`, storagePrefix)
	if err != nil {
		err = fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		err = fmt.Errorf("storage prefix may only contain letters/digits/underscore/dash")
	}

	return err
}

func parseVolumeHandle(volumeHandle string) (svm, flexvol string, err error) {
	tokens := strings.SplitN(volumeHandle, ":", 2)
	if len(tokens) != 2 {
		return "", "", fmt.Errorf("invalid volume handle")
	}
	return tokens[0], tokens[1], nil
}

func ValidateDataLIF(ctx context.Context, dataLIF string, dataLIFs []string) ([]string, error) {
	addressesFromHostname, err := net.LookupHost(dataLIF)
	if err != nil {
		Logc(ctx).Error("Host lookup failed. ", err)
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"hostname":  dataLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from hostname lookup.")

	for _, hostNameAddress := range addressesFromHostname {
		foundValidLIFAddress := false

	loop:
		for _, lifAddress := range dataLIFs {
			if lifAddress == hostNameAddress {
				foundValidLIFAddress = true
				break loop
			}
		}
		if foundValidLIFAddress {
			Logc(ctx).WithField("hostNameAddress", hostNameAddress).Debug("Found matching Data LIF.")
		} else {
			Logc(ctx).WithField("hostNameAddress", hostNameAddress).Debug("Could not find matching Data LIF.")
			return nil, fmt.Errorf("could not find Data LIF for %s", hostNameAddress)
		}

	}

	return addressesFromHostname, nil
}

// sanitizeDataLIF removes any brackets from the provided data LIF (can be present with IPv6)
func sanitizeDataLIF(dataLIF string) string {
	result := strings.TrimPrefix(dataLIF, "[")
	result = strings.TrimSuffix(result, "]")
	return result
}

// GetEncryptionValue: Returns "true"/"false" if encryption is explicitely mentioned in the
// backend or storage class. Otherwise, it returns "nil" which enables NAE/NVE on the volume
// depending on the aggregate properties.
func GetEncryptionValue(encryption string) (*bool, string, error) {
	if encryption != "" {
		enable, err := strconv.ParseBool(encryption)
		if err != nil {
			return nil, "", err
		}
		return &enable, encryption, err
	}
	return nil, "", nil
}

// ConstructOntapNASVolumeAccessPath returns volume path for ONTAP NAS.
// Function accepts parameters in following way:
// 1.smbShare : This takes the value given in backend config, without path prefix.
// 2.volumeName : This takes the value of volume's internal name, it is always prefixed with unix styled path separator.
// 3.volConfig : This takes value of volume configuration.
// 4.Protocol : This takes the value of NAS protocol (NFS/SMB).
// Example, ConstructOntapNASVolumeAccessPath(ctx, "test_share", "/vol" , volConfig, "nfs")
func ConstructOntapNASVolumeAccessPath(
	ctx context.Context, smbShare, volumeName string,
	volConfig *storage.VolumeConfig, protocol string,
) string {
	Logc(ctx).Debug(">>>> smb.ConstructOntapNASVolumeAccessPath")
	defer Logc(ctx).Debug("<<<< smb.ConstructOntapNASVolumeAccessPath")

	var completeVolumePath string
	var smbSharePath string
	switch protocol {
	case sa.NFS:
		if volConfig.ReadOnlyClone {
			if volConfig.ImportOriginalName != "" {
				// For an imported volume, use junction path for the mount
				return fmt.Sprintf("/%s/%s/%s", volumeName, ".snapshot", volConfig.CloneSourceSnapshot)
			}
			return fmt.Sprintf("/%s/%s/%s", volConfig.CloneSourceVolumeInternal, ".snapshot", volConfig.CloneSourceSnapshot)
		} else if volumeName != utils.UnixPathSeparator+volConfig.InternalName && strings.HasPrefix(volumeName,
			utils.UnixPathSeparator) {
			// For managed import, return the original junction path
			return volumeName
		}
		return fmt.Sprintf("/%s", volConfig.InternalName)
	case sa.SMB:
		if smbShare != "" {
			smbSharePath = fmt.Sprintf("\\%s", smbShare)
		} else {
			// Set share path as empty, volume name contains the path prefix.
			smbSharePath = ""
		}

		if volConfig.ReadOnlyClone {
			completeVolumePath = fmt.Sprintf("%s\\%s\\%s\\%s", smbSharePath, volConfig.CloneSourceVolumeInternal,
				"~snapshot", volConfig.CloneSourceSnapshot)
		} else {
			// If the user does not specify an SMB Share, Trident creates it with the same name as the flexvol volume name.
			completeVolumePath = smbSharePath + volumeName
		}
	}
	// Replace unix styled path separator, if exists
	return strings.Replace(completeVolumePath, utils.UnixPathSeparator, utils.WindowsPathSeparator, -1)
}

// ConstructOntapNASFlexGroupSMBVolumePath returns windows compatible volume path for Ontap NAS FlexGroup
// Function accepts parameters in following way:
// 1.smbShare : This takes the value given in backend config, without path prefix.
// 2.volumeName : This takes the value of volume's internal name, it is always prefixed with unix styled path separator.
// Example, ConstructOntapNASFlexGroupSMBVolumePath(ctx, "test_share", "/vol")
func ConstructOntapNASFlexGroupSMBVolumePath(ctx context.Context, smbShare, volumeName string) string {
	Logc(ctx).Debug(">>>> smb.ConstructOntapNASFlexGroupSMBVolumePath")
	defer Logc(ctx).Debug("<<<< smb.ConstructOntapNASFlexGroupSMBVolumePath")

	var completeVolumePath string
	if smbShare != "" {
		completeVolumePath = utils.WindowsPathSeparator + smbShare + volumeName
	} else {
		// If the user does not specify an SMB Share, Trident creates it with the same name as the flexGroup volume name.
		completeVolumePath = volumeName
	}

	// Replace unix styled path separator, if exists
	return strings.Replace(completeVolumePath, utils.UnixPathSeparator, utils.WindowsPathSeparator, -1)
}

// ConstructOntapNASQTreeVolumePath returns volume path for Ontap NAS QTree
// Function accepts parameters in following way:
// 1.smbShare : This takes the value given in backend config, without path prefix.
// 2.flexVol : This takes the value of the parent volume, without path prefix.
// 3.volConfig : This takes the value of volume configuration.
// 4. protocol: This takes the value of the protocol for which the path needs to be created.
// Example, ConstructOntapNASQTreeVolumePath(ctx, test.smbShare, "flex-vol", volConfig, sa.SMB)
func ConstructOntapNASQTreeVolumePath(
	ctx context.Context, smbShare, flexvol string,
	volConfig *storage.VolumeConfig, protocol string,
) (completeVolumePath string) {
	Logc(ctx).Debug(">>>> smb.ConstructOntapNASQTreeVolumePath")
	defer Logc(ctx).Debug("<<<< smb.ConstructOntapNASQTreeVolumePath")

	switch protocol {
	case sa.NFS:
		if volConfig.ReadOnlyClone {
			completeVolumePath = fmt.Sprintf("/%s/%s/%s/%s", flexvol, volConfig.CloneSourceVolumeInternal,
				".snapshot", volConfig.CloneSourceSnapshot)
		} else {
			completeVolumePath = fmt.Sprintf("/%s/%s", flexvol, volConfig.InternalName)
		}
	case sa.SMB:
		var smbSharePath string
		if smbShare != "" {
			smbSharePath = smbShare + utils.WindowsPathSeparator
		}
		if volConfig.ReadOnlyClone {
			completeVolumePath = fmt.Sprintf("\\%s%s\\%s\\%s\\%s", smbSharePath, flexvol,
				volConfig.CloneSourceVolumeInternal, "~snapshot", volConfig.CloneSourceSnapshot)
		} else {
			completeVolumePath = fmt.Sprintf("\\%s%s\\%s", smbSharePath, flexvol, volConfig.InternalName)
		}

		// Replace unix styled path separator, if exists
		completeVolumePath = strings.Replace(completeVolumePath, utils.UnixPathSeparator, utils.WindowsPathSeparator,
			-1)
	}

	return
}
