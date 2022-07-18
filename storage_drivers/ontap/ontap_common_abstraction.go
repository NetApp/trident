// Copyright 2022 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
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

type TelemetryAbstraction struct {
	tridentconfig.Telemetry
	Plugin        string                   `json:"plugin"`
	SVM           string                   `json:"svm"`
	StoragePrefix string                   `json:"storagePrefix"`
	Driver        StorageDriverAbstraction `json:"-"`
	done          chan struct{}
	ticker        *time.Ticker
	stopped       bool
}

type StorageDriverAbstraction interface {
	GetConfig() *drivers.OntapStorageDriverConfig
	GetAPI() api.OntapAPI
	GetTelemetry() *TelemetryAbstraction
	Name() string
}

type NASDriverAbstraction interface {
	GetVolumeOpts(context.Context, *storage.VolumeConfig, map[string]sa.Request) (map[string]string, error)
	GetAPI() api.OntapAPI
	GetConfig() *drivers.OntapStorageDriverConfig
}

func NewOntapTelemetryAbstraction(ctx context.Context, d StorageDriverAbstraction) *TelemetryAbstraction {
	t := &TelemetryAbstraction{
		Plugin:        d.Name(),
		SVM:           d.GetAPI().SVMName(),
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
func (t *TelemetryAbstraction) Start(ctx context.Context) {
	go func() {
		time.Sleep(HousekeepingStartupDelaySecs * time.Second)
		EMSHeartbeatAbstraction(ctx, t.Driver)
		for {
			select {
			case tick := <-t.ticker.C:
				Logc(ctx).WithFields(log.Fields{
					"tick":   tick,
					"driver": t.Driver.Name(),
				}).Debug("Sending EMS heartbeat.")
				EMSHeartbeatAbstraction(ctx, t.Driver)
			case <-t.done:
				Logc(ctx).WithFields(log.Fields{
					"driver": t.Driver.Name(),
				}).Debugf("Shut down EMS logs for the driver.")
				return
			}
		}
	}()
}

func (t *TelemetryAbstraction) Stop() {
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
func (t TelemetryAbstraction) String() (out string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in Telemetry#ToString; err: %v", r)
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
func (t TelemetryAbstraction) GoString() string {
	return t.String()
}

func deleteExportPolicyAbstraction(ctx context.Context, policy string, clientAPI api.OntapAPI) error {
	err := clientAPI.ExportPolicyDestroy(ctx, policy)
	if err != nil {
		err = fmt.Errorf("error deleting export policy: %s", err.Error())
	}
	return err
}

func ensureExportPolicyExistsAbstraction(ctx context.Context, policyName string, clientAPI api.OntapAPI) error {
	return clientAPI.ExportPolicyCreate(ctx, policyName)
}

// publishShareAbstraction ensures that the volume has the correct export policy applied.
func publishShareAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
	publishInfo *utils.VolumePublishInfo, volumeName string,
	ModifyVolumeExportPolicy func(ctx context.Context, volumeName, policyName string) error,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "publishFlexVolShare",
			"Type":   "ontap_common",
			"Share":  volumeName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> publishFlexVolShare")
		defer Logc(ctx).WithFields(fields).Debug("<<<< publishFlexVolShare")
	}

	if !config.AutoExportPolicy || publishInfo.Unmanaged {
		// Nothing to do if we're not configuring export policies automatically or volume is not managed
		return nil
	}

	if err := ensureNodeAccessAbstraction(ctx, publishInfo, clientAPI, config); err != nil {
		return err
	}

	// Update volume to use the correct export policy
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	err := ModifyVolumeExportPolicy(ctx, volumeName, policyName)
	return err
}

// ensureNodeAccess check to see if the export policy exists and if not it will create it and force a reconcile.
// This should be used during publish to make sure access is available if the policy has somehow been deleted.
// Otherwise we should not need to reconcile, which could be expensive.
func ensureNodeAccessAbstraction(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig,
) error {
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	if exists, err := clientAPI.ExportPolicyExists(ctx, policyName); err != nil {
		return err
	} else if !exists {
		Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy missing, will create it.")
		return reconcileNASNodeAccessAbstraction(ctx, publishInfo.Nodes, config, clientAPI, policyName)
	}
	Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy exists.")
	return nil
}

func reconcileNASNodeAccessAbstraction(
	ctx context.Context, nodes []*utils.Node, config *drivers.OntapStorageDriverConfig, clientAPI api.OntapAPI,
	policyName string,
) error {
	if !config.AutoExportPolicy {
		return nil
	}
	err := ensureExportPolicyExistsAbstraction(ctx, policyName, clientAPI)
	if err != nil {
		return err
	}
	desiredRules, err := getDesiredExportPolicyRules(ctx, nodes, config)
	if err != nil {
		err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
		Logc(ctx).Error(err)
		return err
	}
	err = reconcileExportPolicyRulesAbstraction(ctx, policyName, desiredRules, clientAPI)
	if err != nil {
		err = fmt.Errorf("unabled to reconcile export policy rules; %v", err)
		Logc(ctx).WithField("ExportPolicy", policyName).Error(err)
		return err
	}
	return nil
}

func reconcileExportPolicyRulesAbstraction(
	ctx context.Context, policyName string, desiredPolicyRules []string, clientAPI api.OntapAPI,
) error {
	fields := log.Fields{
		"Method":             "reconcileExportPolicyRulesAbstraction",
		"Type":               "ontap_common_abstraction",
		"policyName":         policyName,
		"desiredPolicyRules": desiredPolicyRules,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> reconcileExportPolicyRulesAbstraction")
	defer Logc(ctx).WithFields(fields).Debug("<<<< reconcileExportPolicyRulesAbstraction")

	// first grab all existing rules
	rules, err := clientAPI.ExportRuleList(ctx, policyName)

	for _, rule := range desiredPolicyRules {
		if _, ok := rules[rule]; ok {
			// Rule already exists and we want it, so don't create it or delete it
			delete(rules, rule)
		} else {
			// Rule does not exist, so create it
			if err = clientAPI.ExportRuleCreate(ctx, policyName, rule); err != nil {
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

// resizeValidation performs needed validation checks prior to the resize operation.
func resizeValidationAbstraction(
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
		return 0, utils.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size %d is less than existing volume size %d", sizeBytes, volSize))
	}

	return volSizeBytes, nil
}

func reconcileSANNodeAccessAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, igroupName string,
	nodeIQNs []string,
) error {
	err := ensureIGroupExistsAbstraction(ctx, clientAPI, igroupName)
	if err != nil {
		return err
	}

	// Discover mapped initiators
	mappedIQNs, err := clientAPI.IgroupGetByName(ctx, igroupName)
	if err != nil {
		Logc(ctx).WithField("igroup", igroupName)
		return fmt.Errorf("failed to read igroup info; %v", err)
	}

	// Add missing initiators
	for _, iqn := range nodeIQNs {
		if _, ok := mappedIQNs[iqn]; ok {
			// IQN is properly mapped; remove it from the list
			delete(mappedIQNs, iqn)
		} else {
			// IQN isn't mapped and should be; add it
			err = clientAPI.EnsureIgroupAdded(ctx, igroupName, iqn)
			if err != nil {
				return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
			}
		}
	}

	// mappedIQNs is now a list of mapped IQNs that we have no nodes for; remove them
	for iqn := range mappedIQNs {
		err = clientAPI.IgroupRemove(ctx, igroupName, iqn, true)
		if err != nil {
			return fmt.Errorf("error removing IQN %v from igroup %v: %v", iqn, igroupName, err)
		}
	}

	return nil
}

func cleanIgroupsAbstraction(ctx context.Context, client *api.Client, igroupName string) {
	response, err := client.IgroupDestroy(igroupName)
	err = azgo.GetError(ctx, response, err)
	zerr, zerrOK := err.(azgo.ZapiError)
	if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NO_SUCH_INITGROUP) {
		Logc(ctx).WithField("igroup", igroupName).Debug("No such initiator group (igroup).")
	} else if zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_MAPS_EXIST {
		Logc(ctx).WithField("igroup", igroupName).Info("Initiator group (igroup) currently in use.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"igroup": igroupName,
			"error":  err.Error(),
		}).Error("Initiator group (igroup) could not be deleted.")
	}
}

// GetISCSITargetInfoAbstraction returns the iSCSI node name and iSCSI interfaces using the provided client's SVM.
func GetISCSITargetInfoAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
) (iSCSINodeName string, iSCSIInterfaces []string, returnError error) {
	// Get the SVM iSCSI IQN
	iSCSINodeName, err := clientAPI.IscsiNodeGetNameRequest(ctx)
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI node name: %v", err)
		return
	}

	// Get the SVM iSCSI interface with enabled IQNs
	iscsiInterfaces, err := clientAPI.IscsiInterfaceGet(ctx, clientAPI.SVMName())
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI node name: %v", err)
		return
	}
	// Get the IQN
	if iscsiInterfaces == nil {
		returnError = fmt.Errorf("SVM %s has no active iSCSI interfaces", clientAPI.SVMName())
		return
	}

	return
}

// PopulateOntapLunMappingAbstraction helper function to fill in volConfig with its LUN mapping values.
// This function assumes that the list of data LIFs has not changed since driver initialization and volume creation
func PopulateOntapLunMappingAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
	ips []string, volConfig *storage.VolumeConfig, lunID int, lunPath, igroupName string,
) error {
	var targetIQN string
	targetIQN, err := clientAPI.IscsiNodeGetNameRequest(ctx)
	if err != nil {
		return fmt.Errorf("problem retrieving iSCSI services: %v", err)
	}

	lunResponse, err := clientAPI.LunGetByName(ctx, lunPath)
	if err != nil || lunResponse == nil {
		return fmt.Errorf("problem retrieving LUN info: %v", err)
	}
	serial := lunResponse.SerialNumber

	filteredIPs, err := getISCSIDataLIFsForReportingNodesAbstraction(ctx, clientAPI, ips, lunPath, igroupName)
	if err != nil {
		return err
	}

	if len(filteredIPs) == 0 {
		Logc(ctx).Warn("Unable to find reporting ONTAP nodes for discovered dataLIFs.")
		filteredIPs = ips
	}

	volConfig.AccessInfo.IscsiTargetPortal = filteredIPs[0]
	volConfig.AccessInfo.IscsiPortals = filteredIPs[1:]
	volConfig.AccessInfo.IscsiTargetIQN = targetIQN
	volConfig.AccessInfo.IscsiLunNumber = int32(lunID)
	volConfig.AccessInfo.IscsiIgroup = igroupName
	volConfig.AccessInfo.IscsiLunSerial = serial
	Logc(ctx).WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"igroup":          volConfig.AccessInfo.IscsiIgroup,
	}).Debug("Mapped ONTAP LUN.")

	return nil
}

// PublishLUNAbstraction publishes the volume to the host specified in publishInfo from ontap-san or
// ontap-san-economy. This method may or may not be running on the host where the volume will be
// mounted, so it should limit itself to updating access rules, initiator groups, etc. that require
// some host identity (but not locality) as well as storage controller API access.
// This function assumes that the list of data LIF IP addresses does not change between driver initialization
// and publish
func PublishLUNAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig, ips []string,
	publishInfo *utils.VolumePublishInfo, lunPath, igroupName, iSCSINodeName string,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":        "PublishLUN",
			"Type":          "ontap_common",
			"lunPath":       lunPath,
			"iSCSINodeName": iSCSINodeName,
			"publishInfo":   publishInfo,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> PublishLUNAbstraction")
		defer Logc(ctx).WithFields(fields).Debug("<<<< PublishLUNAbstraction")
	}

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
	lunComment, parse, err := clientAPI.LunGetComment(ctx, lunPath)
	if err != nil || lunComment == "" {
		Logc(ctx).WithFields(log.Fields{
			"LUN":    lunPath,
			"fstype": fstype,
		}).Warn("LUN attribute fstype not found, using default.")
		parse = false
	}
	if parse {
		lunAttrs, err := clientAPI.ParseLunComment(ctx, lunComment)
		if err == nil && lunAttrs != nil {
			fstype = lunAttrs["fstype"]
		}
		Logc(ctx).WithFields(log.Fields{"LUN": lunPath, "fstype": fstype}).Debug("Found LUN attribute fstype.")
	} else if lunComment != "" {
		fstype = lunComment
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

	if !publishInfo.Unmanaged {
		if iqn != "" {
			// Add IQN to igroup
			err = clientAPI.EnsureIgroupAdded(ctx, igroupName, iqn)
			if err != nil {
				return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
			}
		}
	}

	// Map LUN (it may already be mapped)
	lunID, err := clientAPI.EnsureLunMapped(ctx, igroupName, lunPath, publishInfo.Unmanaged)
	if err != nil {
		return err
	}

	filteredIPs, err := getISCSIDataLIFsForReportingNodesAbstraction(ctx, clientAPI, ips, lunPath, igroupName)
	if err != nil {
		return err
	}

	if len(filteredIPs) == 0 {
		Logc(ctx).Warn("Unable to find reporting ONTAP nodes for discovered dataLIFs.")
		filteredIPs = ips
	}

	// Add fields needed by Attach
	publishInfo.IscsiLunNumber = int32(lunID)
	publishInfo.IscsiTargetPortal = filteredIPs[0]
	publishInfo.IscsiPortals = filteredIPs[1:]
	publishInfo.IscsiTargetIQN = iSCSINodeName
	publishInfo.IscsiIgroup = igroupName
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

// LunUnmapAllIgroups removes all maps from a given LUN
func LunUnmapAllIgroups(ctx context.Context, clientAPI api.OntapAPI, lunPath string) error {
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
			Logc(ctx).WithError(err).WithFields(log.Fields{
				"LUN":    lunPath,
				"igroup": igroup,
			}).Warn("error unmapping LUN from igroup")
		}
	}
	if errored {
		return fmt.Errorf("error unmapping one or more LUN mappings")
	}
	return nil
}

// getISCSIDataLIFsForReportingNodes finds the data LIFs for the reporting nodes for the LUN.
func getISCSIDataLIFsForReportingNodesAbstraction(
	ctx context.Context, clientAPI api.OntapAPI, ips []string, lunPath, igroupName string,
) ([]string, error) {
	reportingNodes, err := clientAPI.LunMapGetReportingNodes(ctx, igroupName, lunPath)
	if err != nil {
		return nil, fmt.Errorf("could not get iSCSI reported nodes: %v", err)
	}

	reportingNodeNames := make(map[string]struct{})
	if reportingNodes != nil {
		for _, reportingNode := range reportingNodes {
			Logc(ctx).WithField("reportingNode", reportingNode).Debug("Reporting node found.")
			reportingNodeNames[reportingNode] = struct{}{}
		}
	}

	currentNode, reportedDataLIFs, err := clientAPI.GetReportedDataLifs(ctx)
	if err != nil {
		return nil, err
	}
	var dataLIFs []string
	for _, ip := range ips {
		if _, ok := reportingNodeNames[currentNode]; ok {
			for _, lif := range reportedDataLIFs {
				if lif == ip {
					dataLIFs = append(dataLIFs, ip)
				}
			}
		}
	}

	return dataLIFs, nil
}

// ValidateBidrectionalChapCredentialsAbstraction validates the bidirectional CHAP settings
func ValidateBidrectionalChapCredentialsAbstraction(
	defaultAuth api.IscsiInitiatorAuth,
	config *drivers.OntapStorageDriverConfig,
) (*ChapCredentials, error) {
	isDefaultAuthTypeNone, err := IsDefaultAuthTypeNoneAbstraction(defaultAuth)
	if err != nil {
		return nil, fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeCHAP, err := IsDefaultAuthTypeCHAPAbstraction(defaultAuth)
	if err != nil {
		return nil, fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeDeny, err := IsDefaultAuthTypeDenyAbstraction(defaultAuth)
	if err != nil {
		return nil, fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

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
func isDefaultAuthTypeOfTypeAbstraction(
	response api.IscsiInitiatorAuth, authType string,
) (bool, error) {
	// case insensitive compare
	return strings.EqualFold(response.AuthType, authType), nil
}

// IsDefaultAuthTypeNoneAbstraction returns true if the default initiator's auth-type field is set to the value "none"
func IsDefaultAuthTypeNoneAbstraction(response api.IscsiInitiatorAuth) (bool, error) {
	return isDefaultAuthTypeOfTypeAbstraction(response, "none")
}

// IsDefaultAuthTypeCHAPAbstraction returns true if the default initiator's auth-type field is set to the value "CHAP"
func IsDefaultAuthTypeCHAPAbstraction(response api.IscsiInitiatorAuth) (bool, error) {
	return isDefaultAuthTypeOfTypeAbstraction(response, "CHAP")
}

// IsDefaultAuthTypeDenyAbstraction returns true if the default initiator's auth-type field is set to the value "deny"
func IsDefaultAuthTypeDenyAbstraction(response api.IscsiInitiatorAuth) (bool, error) {
	return isDefaultAuthTypeOfTypeAbstraction(response, "deny")
}

// InitializeSANDriverAbstraction performs common ONTAP SAN driver initialization.
func InitializeSANDriverAbstraction(
	ctx context.Context, driverContext tridentconfig.DriverContext, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig, validate func(context.Context) error, backendUUID string,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeSANDriver", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeSANDriver")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeSANDriver")
	}

	if config.IgroupName == "" {
		config.IgroupName = getDefaultIgroupName(driverContext, backendUUID)
	}

	// Defer validation to the driver's validate method
	if err := validate(ctx); err != nil {
		return err
	}

	// Create igroup
	err := ensureIGroupExistsAbstraction(ctx, clientAPI, config.IgroupName)
	if err != nil {
		return err
	}

	getDefaultAuthResponse, err := clientAPI.IscsiInitiatorGetDefaultAuth(ctx)
	Logc(ctx).WithFields(log.Fields{
		"getDefaultAuthResponse": getDefaultAuthResponse,
		"err":                    err,
	}).Debug("IscsiInitiatorGetDefaultAuth result")

	if err != nil {
		return fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeNone, err := IsDefaultAuthTypeNoneAbstraction(getDefaultAuthResponse)
	if err != nil {
		return fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	if config.UseCHAP {

		authType := "CHAP"
		chapCredentials, err := ValidateBidrectionalChapCredentialsAbstraction(getDefaultAuthResponse, config)
		if err != nil {
			return fmt.Errorf("error with CHAP credentials: %v", err)
		}
		Logc(ctx).Debug("Using CHAP credentials")

		if isDefaultAuthTypeNone {
			lunsResponse, lunsResponseErr := clientAPI.LunList(ctx, fmt.Sprintf("svm=%s", clientAPI.SVMName()))
			if lunsResponseErr != nil {
				return fmt.Errorf("error enumerating LUNs for SVM %v: %v", clientAPI.SVMName(), lunsResponseErr)
			}

			if len(lunsResponse) > 0 {
				return fmt.Errorf(
					"will not enable CHAP for SVM %v; %v exisiting LUNs would lose access",
					clientAPI.SVMName(), len(lunsResponse))
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

func ensureIGroupExistsAbstraction(ctx context.Context, clientAPI api.OntapAPI, igroupName string) error {
	err := clientAPI.IgroupCreate(ctx, igroupName, "iscsi", "linux")
	if err != nil {
		return fmt.Errorf("error creating igroup: %v", err)
	}
	return nil
}

// InitializeOntapDriverAbstraction sets up the API client and performs all other initialization tasks
// that are common to all the ONTAP drivers.
func InitializeOntapDriverAbstraction(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api.OntapAPI, error) {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapDriver", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeOntapDriver")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeOntapDriver")
	}

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

	Logc(ctx).WithFields(log.Fields{
		"hostname":  mgmtLIF,
		"addresses": addressesFromHostname,
	}).Debug("Addresses found from ManagementLIF lookup.")

	// Get the API client
	client, err := InitializeOntapAPIAbstraction(ctx, config)
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
		Logc(ctx).WithFields(log.Fields{
			"serialNumbers": strings.Join(config.SerialNumbers, ","),
		}).Info("Controller serial numbers.")
	}

	return client, nil
}

// InitializeOntapAPIAbstraction returns an ontap.Client ZAPI or REST client.  If the SVM isn't specified in the config
// file, this method attempts to derive the one to use for ZAPI.
func InitializeOntapAPIAbstraction(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api.OntapAPI, error) {
	var ontapAPI api.OntapAPI
	var err error

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "InitializeOntapAPIAbstraction", "Type": "ontap_common",
			"useREST": config.UseREST,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeOntapAPIAbstraction")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeOntapAPIAbstraction")
	}

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

// ValidateSANDriverAbstraction contains the validation logic shared between ontap-san and ontap-san-economy.
func ValidateSANDriverAbstraction(
	ctx context.Context, config *drivers.OntapStorageDriverConfig, ips []string,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ValidateSANDriver", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> ValidateSANDriver")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ValidateSANDriver")
	}

	// If the user sets the LIF to use in the config, disable multipathing and use just the one IP address
	if config.DataLIF != "" {
		cleanDataLIF := sanitizeDataLIF(config.DataLIF)

		// Make sure it's actually a valid address
		if ip := net.ParseIP(cleanDataLIF); nil == ip {
			return fmt.Errorf("data LIF is not a valid IP: %s", cleanDataLIF)
		}
		// Make sure the IP matches one of the LIFs
		found := false
		for _, ip := range ips {
			if cleanDataLIF == ip {
				found = true
				break
			}
		}
		if found {
			Logc(ctx).WithField("ip", cleanDataLIF).Debug("Found matching Data LIF.")
		} else {
			Logc(ctx).WithField("ip", cleanDataLIF).Debug("Could not find matching Data LIF.")
			return fmt.Errorf("could not find Data LIF for %s", cleanDataLIF)
		}

		// Replace the IPs with a singleton list
		ips = []string{cleanDataLIF}
	}

	if config.DriverContext == tridentconfig.ContextDocker {
		// Make sure this host is logged into the ONTAP iSCSI target
		err := utils.EnsureISCSISessionsWithPortalDiscovery(ctx, ips)
		if err != nil {
			return fmt.Errorf("error establishing iSCSI session: %v", err)
		}
	}

	return nil
}

// ValidateNASDriverAbstraction contains the validation logic shared between ontap-nas and ontap-nas-economy.
func ValidateNASDriverAbstraction(
	ctx context.Context, api api.OntapAPI, config *drivers.OntapStorageDriverConfig,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ValidateNASDriver", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> ValidateNASDriver")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ValidateNASDriver")
	}

	dataLIFs, err := api.NetInterfaceGetDataLIFs(ctx, "nfs")
	if err != nil {
		return err
	}

	if len(dataLIFs) == 0 {
		return fmt.Errorf("no NAS data LIFs found on SVM %s", api.SVMName())
	} else {
		Logc(ctx).WithField("dataLIFs", dataLIFs).Debug("Found NAS LIFs.")
	}

	// If they didn't set a LIF to use in the config, we'll set it to the first nfs LIF we happen to find
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

func checkAggregateLimitsForFlexvolAbstraction(
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

	return checkAggregateLimitsAbstraction(ctx, volInfo.Aggregates[0], volInfo.SpaceReserve, requestedSizeInt, config,
		client)
}

func checkAggregateLimitsAbstraction(
	ctx context.Context, aggregate, spaceReserve string, requestedSizeInt uint64,
	config drivers.OntapStorageDriverConfig, client api.OntapAPI,
) error {
	requestedSize := float64(requestedSizeInt)

	limitAggregateUsage := config.LimitAggregateUsage
	limitAggregateUsage = strings.Replace(limitAggregateUsage, "%", "", -1) // strip off any %

	Logc(ctx).WithFields(log.Fields{
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
				Logc(ctx).WithFields(log.Fields{
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
				Logc(ctx).WithFields(log.Fields{
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

// EMSHeartbeatAbstraction logs an ASUP message on a timer
// view them via filer::> event log show -severity NOTICE
func EMSHeartbeatAbstraction(ctx context.Context, driver StorageDriverAbstraction) {
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

// CreateOntapCloneAbstraction creates a volume clone
func CreateOntapCloneAbstraction(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, useAsync bool, qosPolicyGroup api.QosPolicyGroup,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateOntapClone",
			"Type":     "ontap_common",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"split":    split,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateOntapClone")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateOntapClone")
	}

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
	if err = client.VolumeCloneCreate(ctx, name, source, snapshot, useAsync); err != nil {
		return err
	}

	if err = client.VolumeSetComment(ctx, name, name, labels); err != nil {
		return err
	}

	if config.StorageDriverName == drivers.OntapNASStorageDriverName {
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
		err := client.VolumeCloneSplitStart(ctx, name)
		if err != nil {
			return fmt.Errorf("error splitting clone: %s", err.Error())
		}
	}

	return nil
}

// GetSnapshotAbstraction gets a snapshot. To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func GetSnapshotAbstraction(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "GetSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
			"created":      snap.CreateTime,
		}).Debug("Found snapshot.")
		if snap.Name == internalSnapName {
			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	return nil, nil
}

// GetSnapshotsAbstraction returns the list of snapshots associated with the named volume.
func GetSnapshotsAbstraction(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
	sizeGetter func(context.Context, string) (int, error),
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "GetSnapshotList",
			"Type":       "ontap_common",
			"volumeName": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshotList")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshotList")
	}

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %s", err.Error())
	}

	result := make([]*storage.Snapshot, 0)

	for _, snap := range snapshots {

		Logc(ctx).WithFields(log.Fields{
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

// CreateSnapshotAbstraction creates a snapshot for the given volume.
func CreateSnapshotAbstraction(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

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

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		if snap.Name == internalSnapName {
			Logc(ctx).WithFields(log.Fields{
				"snapshotName": snapConfig.InternalName,
				"volumeName":   snapConfig.VolumeInternalName,
			}).Info("Snapshot created.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find snapshot %s for souce volume %s", internalSnapName, internalVolName)
}

// RestoreSnapshotAbstraction restores a volume (in place) from a snapshot.
func RestoreSnapshotAbstraction(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "RestoreSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> RestoreSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< RestoreSnapshot")
	}

	if err := client.SnapshotRestoreVolume(ctx, internalSnapName, internalVolName); err != nil {
		return err
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Debug("Restored snapshot.")

	return nil
}

// SplitVolumeFromBusySnapshotAbstraction gets the list of volumes backed by a busy snapshot and starts
// a split operation on the first one (sorted by volume name).
func SplitVolumeFromBusySnapshotAbstraction(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, cloneSplitStart func(ctx context.Context, cloneName string) error,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "SplitVolumeFromBusySnapshot",
			"Type":         "ontap_common",
			"snapshotName": snapConfig.InternalName,
			"volumeName":   snapConfig.VolumeInternalName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> SplitVolumeFromBusySnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< SplitVolumeFromBusySnapshot")
	}

	childVolumes, err := client.VolumeListBySnapshotParent(ctx, snapConfig.InternalName,
		snapConfig.VolumeInternalName)
	if err != nil {
		return err
	} else if childVolumes == nil || len(childVolumes) == 0 {
		return nil
	}

	if err := cloneSplitStart(ctx, childVolumes[0]); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     snapConfig.InternalName,
			"parentVolumeName": snapConfig.VolumeInternalName,
			"cloneVolumeName":  childVolumes[0],
			"error":            err,
		}).Error("Could not begin splitting clone from snapshot.")
		return fmt.Errorf("error splitting clone: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
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

// discoverBackendAggrNamesCommon discovers names of the aggregates assigned to the configured SVM
func discoverBackendAggrNamesCommonAbstraction(ctx context.Context, d StorageDriverAbstraction) ([]string, error) {
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

	Logc(ctx).WithFields(log.Fields{
		"svm":   client.SVMName(),
		"pools": vserverAggrs,
	}).Debug("Read storage pools assigned to SVM.")

	var aggrNames []string
	for _, aggrName := range vserverAggrs {
		if config.Aggregate != "" {
			if aggrName != config.Aggregate {
				continue
			}

			Logc(ctx).WithFields(log.Fields{
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
func getVserverAggrAttributesAbstraction(
	ctx context.Context, d StorageDriverAbstraction, poolsAttributeMap *map[string]map[string]sa.Offer,
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

			fields := log.Fields{"aggregate": aggrName, "mediaType": aggrType}

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

func InitializeStoragePoolsCommonAbstraction(
	ctx context.Context, d StorageDriverAbstraction, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	config := d.GetConfig()
	physicalPools := make(map[string]storage.Pool)
	virtualPools := make(map[string]storage.Pool)

	// To identify list of media types supported by physical pools
	mediaOffers := make([]sa.Offer, 0)

	// Get name of the physical storage pools which in case of ONTAP is list of aggregates
	physicalStoragePoolNames, err := discoverBackendAggrNamesCommonAbstraction(ctx, d)
	if err != nil || len(physicalStoragePoolNames) == 0 {
		return physicalPools, virtualPools, fmt.Errorf("could not get storage pools from array: %v", err)
	}

	// Create a map of Physical storage pool name to their attributes map
	physicalStoragePoolAttributes := make(map[string]map[string]sa.Offer)
	for _, physicalStoragePoolName := range physicalStoragePoolNames {
		physicalStoragePoolAttributes[physicalStoragePoolName] = make(map[string]sa.Offer)
	}

	// Update physical pool attributes map with aggregate info (i.e. MediaType)
	aggrErr := getVserverAggrAttributesAbstraction(ctx, d, &physicalStoragePoolAttributes)

	if zerr, ok := aggrErr.(azgo.ZapiError); ok && zerr.IsScopeError() {
		Logc(ctx).WithFields(log.Fields{
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

		pool.InternalAttributes()[Size] = config.Size
		pool.InternalAttributes()[Region] = config.Region
		pool.InternalAttributes()[Zone] = config.Zone
		pool.InternalAttributes()[SpaceReserve] = config.SpaceReserve
		pool.InternalAttributes()[SnapshotPolicy] = config.SnapshotPolicy
		pool.InternalAttributes()[SnapshotReserve] = config.SnapshotReserve
		pool.InternalAttributes()[SplitOnClone] = config.SplitOnClone
		pool.InternalAttributes()[Encryption] = config.Encryption
		pool.InternalAttributes()[UnixPermissions] = config.UnixPermissions
		pool.InternalAttributes()[SnapshotDir] = config.SnapshotDir
		pool.InternalAttributes()[ExportPolicy] = config.ExportPolicy
		pool.InternalAttributes()[SecurityStyle] = config.SecurityStyle
		pool.InternalAttributes()[TieringPolicy] = config.TieringPolicy
		pool.InternalAttributes()[QosPolicy] = config.QosPolicy
		pool.InternalAttributes()[AdaptiveQosPolicy] = config.AdaptiveQosPolicy

		pool.SetSupportedTopologies(config.SupportedTopologies)

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
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

		pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels, vpool.Labels)

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
		pool.InternalAttributes()[AdaptiveQosPolicy] = adaptiveQosPolicy
		pool.SetSupportedTopologies(supportedTopologies)

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes()[SpaceAllocation] = spaceAllocation
			pool.InternalAttributes()[FileSystemType] = fileSystemType
		}

		virtualPools[pool.Name()] = pool
	}

	return physicalPools, virtualPools, nil
}

// ValidateStoragePoolsAbstraction makes sure that values are set for the fields, if value(s) were not specified
// for a field then a default should have been set in for that field in the initialize storage pools
func ValidateStoragePoolsAbstraction(
	ctx context.Context, physicalPools, virtualPools map[string]storage.Pool, d StorageDriverAbstraction,
	labelLimit int,
) error {
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
		if pool.InternalAttributes()[Encryption] == "" {
			return fmt.Errorf("encryption cannot by empty in pool %s", poolName)
		} else {
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
		switch pool.InternalAttributes()[SecurityStyle] {
		case "unix", "mixed":
			break
		default:
			return fmt.Errorf("invalid securityStyle %s in pool %s", pool.InternalAttributes()[SecurityStyle], poolName)
		}

		// Validate ExportPolicy
		if pool.InternalAttributes()[ExportPolicy] == "" {
			return fmt.Errorf("export policy cannot by empty in pool %s", poolName)
		}

		// Validate UnixPermissions
		if pool.InternalAttributes()[UnixPermissions] == "" {
			return fmt.Errorf("UNIX permissions cannot by empty in pool %s", poolName)
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

			if d.Name() == drivers.OntapNASQtreeStorageDriverName && pool.InternalAttributes()[AdaptiveQosPolicy] != "" {
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
				return fmt.Errorf("invalid value for size in pool %s. Requested volume size (%d bytes) is too small;"+
					" the minimum volume size is %d bytes", poolName, sizeBytes, MinimumVolumeSizeBytes)
			}
		}

		// Validate splitOnClone
		if pool.InternalAttributes()[SplitOnClone] == "" {
			return fmt.Errorf("splitOnClone cannot by empty in pool %s", poolName)
		} else {
			_, err := strconv.ParseBool(pool.InternalAttributes()[SplitOnClone])
			if err != nil {
				return fmt.Errorf("invalid value for splitOnClone in pool %s: %v", poolName, err)
			}
		}

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {

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

func getVolumeOptsCommonAbstraction(
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
				Logc(ctx).WithFields(log.Fields{
					"provisioner":      "ONTAP",
					"method":           "getVolumeOptsCommon",
					"provisioningType": provisioningTypeReq.Value(),
				}).Warnf("Expected 'thick' or 'thin' for %s; ignoring.",
					sa.ProvisioningType)
			}
		} else {
			Logc(ctx).WithFields(log.Fields{
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
			}
		} else {
			Logc(ctx).WithFields(log.Fields{
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

func calculateOptimalSizeForFlexvolAbstraction(
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

	Logc(ctx).WithFields(log.Fields{
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

// getSnapshotReserveFromOntapAbstraction takes a volume name and retrieves the snapshot policy and snapshot reserve
func getSnapshotReserveFromOntapAbstraction(
	ctx context.Context, name string, GetVolumeInfo func(
		ctx context.Context, volumeName string,
	) (volume *api.Volume, err error),
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

func isFlexvolRWAbstraction(ctx context.Context, ontap api.OntapAPI, name string) (bool, error) {
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

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "getVolumeSnapshot",
			"Type":         "NASStorageDriver",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> GetSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetSnapshot")
	}

	size, err := sizeGetter(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
			"created":      snap.CreateTime,
		}).Debug("Found snapshot.")
		if snap.Name == internalSnapName {
			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}

	return nil, nil
}

// getVolumeSnapshotList returns the list of snapshots associated with the named volume.
func getVolumeSnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, sizeGetter func(context.Context, string) (int, error),
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "getVolumeSnapshotList",
			"Type":       "NASStorageDriver",
			"volumeName": internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> getVolumeSnapshotList")
		defer Logc(ctx).WithFields(fields).Debug("<<<< getVolumeSnapshotList")
	}

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

		Logc(ctx).WithFields(log.Fields{
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

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "CreateSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateSnapshot")
	}

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

	snapshots, err := client.VolumeSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		if snap.Name == internalSnapName {
			Logc(ctx).WithFields(log.Fields{
				"snapshotName": snapConfig.InternalName,
				"volumeName":   snapConfig.VolumeInternalName,
			}).Info("Snapshot created.")

			return &storage.Snapshot{
				Config:    snapConfig,
				Created:   snap.CreateTime,
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find snapshot %s for souce volume %s", internalSnapName, internalVolName)
}

// cloneFlexvol creates a volume clone
func cloneFlexvol(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, qosPolicyGroup api.QosPolicyGroup,
) error {
	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "cloneFlexvol",
			"Type":     "ontap_common",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"split":    split,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> cloneFlexvol")
		defer Logc(ctx).WithFields(fields).Debug("<<<< cloneFlexvol")
	}

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

	if err = client.VolumeSetComment(ctx, name, name, labels); err != nil {
		return err
	}

	if config.StorageDriverName == drivers.OntapNASStorageDriverName {
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
