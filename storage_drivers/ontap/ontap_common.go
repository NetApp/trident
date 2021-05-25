// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
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

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils"
)

const (
	MinimumVolumeSizeBytes       = 20971520 // 20 MiB
	HousekeepingStartupDelaySecs = 10

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
	FileSystemType        = "fileSystemType"
	ProvisioningType      = "provisioningType"
	SplitOnClone          = "splitOnClone"
	TieringPolicy         = "tieringPolicy"
	QosPolicy             = "qosPolicy"
	AdaptiveQosPolicy     = "adaptiveQosPolicy"
	maxFlexGroupCloneWait = 120 * time.Second
)

//For legacy reasons, these strings mustn't change
const (
	artifactPrefixDocker     = "ndvp"
	artifactPrefixKubernetes = "trident"
	LUNAttributeFSType       = "com.netapp.ndvp.fstype"
)

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin        string        `json:"plugin"`
	SVM           string        `json:"svm"`
	StoragePrefix string        `json:"storagePrefix"`
	Driver        StorageDriver `json:"-"`
	done          chan struct{}
	ticker        *time.Ticker
	stopped       bool
}

type StorageDriver interface {
	GetConfig() *drivers.OntapStorageDriverConfig
	GetAPI() *api.Client
	GetTelemetry() *Telemetry
	Name() string
}

type NASDriver interface {
	GetVolumeOpts(context.Context, *storage.VolumeConfig, map[string]sa.Request) (map[string]string, error)
	GetAPI() *api.Client
	GetConfig() *drivers.OntapStorageDriverConfig
}

// CleanBackendName removes brackets and replaces colons with periods to avoid regex parsing errors.
func CleanBackendName(backendName string) string {
	backendName = strings.ReplaceAll(backendName, "[", "")
	backendName = strings.ReplaceAll(backendName, "]", "")
	return strings.ReplaceAll(backendName, ":", ".")
}

func CreateCloneNAS(
	ctx context.Context, d NASDriver, volConfig *storage.VolumeConfig, storagePool *storage.Pool, sourceLabel string,
	labelLimit int, useAsync bool) error {

	// if cloning a FlexGroup, useAsync will be true
	if useAsync && !d.GetAPI().SupportsFeature(ctx, api.NetAppFlexGroupsClone) {
		return errors.New("the ONTAPI version does not support FlexGroup cloning")
	}

	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshot := volConfig.CloneSourceSnapshot

	if d.GetConfig().DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":      "CreateClone",
			"Type":        "NASStorageDriver",
			"name":        name,
			"source":      source,
			"snapshot":    snapshot,
			"storagePool": storagePool,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> CreateClone")
		defer Logc(ctx).WithFields(fields).Debug("<<<< CreateClone")
	}

	opts, err := d.GetVolumeOpts(context.Background(), volConfig, make(map[string]sa.Request))
	if err != nil {
		return err
	}

	// How "splitOnClone" value gets set:
	// In the Core we first check clone's VolumeConfig for splitOnClone value
	// If it is not set then (again in Core) we check source PV's VolumeConfig for splitOnClone value
	// If we still don't have splitOnClone value then HERE we check for value in the source PV's Storage/Virtual Pool
	// If the value for "splitOnClone" is still empty then HERE we set it to backend config's SplitOnClone value

	// Attempt to get splitOnClone value based on storagePool (source Volume's StoragePool)
	var storagePoolSplitOnCloneVal string

	labels := sourceLabel

	if storage.IsStoragePoolUnset(storagePool) {
		// Set the base label
		storagePoolTemp := &storage.Pool{
			Attributes: map[string]sa.Offer{
				sa.Labels: sa.NewLabelOffer(d.GetConfig().Labels),
			},
		}
		labels, err = storagePoolTemp.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit)
		if err != nil {
			return err
		}
	} else {
		storagePoolSplitOnCloneVal = storagePool.InternalAttributes[SplitOnClone]
	}

	// If storagePoolSplitOnCloneVal is still unknown, set it to backend's default value
	if storagePoolSplitOnCloneVal == "" {
		storagePoolSplitOnCloneVal = d.GetConfig().SplitOnClone
	}

	split, err := strconv.ParseBool(utils.GetV(opts, "splitOnClone", storagePoolSplitOnCloneVal))
	if err != nil {
		return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
	}

	qosPolicy := utils.GetV(opts, "qosPolicy", "")
	adaptiveQosPolicy := utils.GetV(opts, "adaptiveQosPolicy", "")
	qosPolicyGroup, err := api.NewQosPolicyGroup(qosPolicy, adaptiveQosPolicy)
	if err != nil {
		return err
	}

	Logc(ctx).WithField("splitOnClone", split).Debug("Creating volume clone.")
	return CreateOntapClone(ctx, name, source, snapshot, labels, split, d.GetConfig(), d.GetAPI(), useAsync,
		qosPolicyGroup)
}

// InitializeOntapConfig parses the ONTAP config, mixing in the specified common config.
func InitializeOntapConfig(
	ctx context.Context, driverContext tridentconfig.DriverContext, configJSON string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string) (*drivers.
		OntapStorageDriverConfig, error) {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapConfig", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeOntapConfig")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeOntapConfig")
	}

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

	// Load default config parameters
	err = PopulateConfigurationDefaults(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not populate configuration defaults: %v", err)
	}

	return config, nil
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
				Logc(ctx).WithFields(log.Fields{
					"tick":   tick,
					"driver": t.Driver.Name(),
				}).Debug("Sending EMS heartbeat.")
				EMSHeartbeat(ctx, t.Driver)
			case <-t.done:
				Logc(ctx).WithFields(log.Fields{
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

// String makes Telemetry satisfy the GoStringer interface.
func (t Telemetry) GoString() string {
	return t.String()
}

func deleteExportPolicy(ctx context.Context, policy string, clientAPI *api.Client) error {
	response, err := clientAPI.ExportPolicyDestroy(policy)
	if err = api.GetError(ctx, response, err); err != nil {
		err = fmt.Errorf("error deleting export policy: %v", err)
	}
	return err
}

func createExportRule(ctx context.Context, desiredPolicyRule, policyName string, clientAPI *api.Client) error {

	ruleResponse, err := clientAPI.ExportRuleCreate(policyName, desiredPolicyRule,
		[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
	if err = api.GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error creating export rule: %v", err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"ClientMatch":  desiredPolicyRule,
		}).Error(err)
	}
	return err
}

func deleteExportRule(ctx context.Context, ruleIndex int, policyName string, clientAPI *api.Client) error {

	ruleDestroyResponse, err := clientAPI.ExportRuleDestroy(policyName, ruleIndex)
	if err = api.GetError(ctx, ruleDestroyResponse, err); err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v",
			policyName, ruleIndex, err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"RuleIndex":    ruleIndex,
		}).Error(err)
	}
	return err
}

func isExportPolicyExists(ctx context.Context, policyName string, clientAPI *api.Client) (bool, error) {

	policyGetResponse, err := clientAPI.ExportPolicyGet(policyName)
	if err != nil {
		err = fmt.Errorf("error getting export policy; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).Error(err)
		return false, err
	}
	if zerr := api.NewZapiError(policyGetResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy not found.")
			return false, nil
		} else {
			err = fmt.Errorf("error getting export policy; %v", zerr)
			Logc(ctx).WithField("exportPolicy", policyName).Error(err)
			return false, err
		}
	}
	return true, nil
}

func ensureExportPolicyExists(ctx context.Context, policyName string, clientAPI *api.Client) error {

	policyCreateResponse, err := clientAPI.ExportPolicyCreate(policyName)
	if err != nil {
		err = fmt.Errorf("error creating export policy %s: %v", policyName, err)
	}
	if zerr := api.NewZapiError(policyCreateResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EDUPLICATEENTRY {
			Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy already exists.")
		} else {
			err = fmt.Errorf("error creating export policy %s: %v", policyName, zerr)
		}
	}
	return err
}

// publishFlexVolShare ensures that the volume has the correct export policy applied.
func publishFlexVolShare(
	ctx context.Context, clientAPI *api.Client, config *drivers.OntapStorageDriverConfig,
	publishInfo *utils.VolumePublishInfo, volumeName string,
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

	if err := ensureNodeAccess(ctx, publishInfo, clientAPI, config); err != nil {
		return err
	}

	// Update volume to use the correct export policy
	policyName := getExportPolicyName(publishInfo.BackendUUID)
	volumeModifyResponse, err := clientAPI.VolumeModifyExportPolicy(volumeName, policyName)
	if err = api.GetError(ctx, volumeModifyResponse, err); err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return err
	}
	return nil
}

func getExportPolicyName(backendUUID string) string {
	return fmt.Sprintf("trident-%s", backendUUID)
}

// ensureNodeAccess check to see if the export policy exists and if not it will create it and force a reconcile.
// This should be used during publish to make sure access is available if the policy has somehow been deleted.
// Otherwise we should not need to reconcile, which could be expensive.
func ensureNodeAccess(
	ctx context.Context, publishInfo *utils.VolumePublishInfo, clientAPI *api.Client,
	config *drivers.OntapStorageDriverConfig,
) error {

	policyName := getExportPolicyName(publishInfo.BackendUUID)
	if exists, err := isExportPolicyExists(ctx, policyName, clientAPI); err != nil {
		return err
	} else if !exists {
		Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy missing, will create it.")
		return reconcileNASNodeAccess(ctx, publishInfo.Nodes, config, clientAPI, policyName)
	}
	Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy exists.")
	return nil
}

func reconcileNASNodeAccess(
	ctx context.Context, nodes []*utils.Node, config *drivers.OntapStorageDriverConfig, clientAPI *api.Client,
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
	err = reconcileExportPolicyRules(ctx, policyName, desiredRules, clientAPI)
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
	ctx context.Context, policyName string, desiredPolicyRules []string, clientAPI *api.Client,
) error {

	ruleListResponse, err := clientAPI.ExportRuleGetIterRequest(policyName)
	if err = api.GetError(ctx, ruleListResponse, err); err != nil {
		return fmt.Errorf("error listing export policy rules: %v", err)
	}
	rulesToRemove := make(map[string]int)
	if ruleListResponse.Result.NumRecords() > 0 {
		rulesAttrList := ruleListResponse.Result.AttributesList()
		rules := rulesAttrList.ExportRuleInfo()
		for _, rule := range rules {
			rulesToRemove[rule.ClientMatch()] = rule.RuleIndex()
		}
	}
	for _, rule := range desiredPolicyRules {
		if _, ok := rulesToRemove[rule]; ok {
			// Rule already exists and we want it, so don't create it or delete it
			delete(rulesToRemove, rule)
		} else {
			// Rule does not exist, so create it
			err = createExportRule(ctx, rule, policyName, clientAPI)
			if err != nil {
				return err
			}
		}
	}
	// Now that the desired rules exists, delete the undesired rules
	for _, ruleIndex := range rulesToRemove {
		err = deleteExportRule(ctx, ruleIndex, policyName, clientAPI)
		if err != nil {
			return err
		}
	}
	return nil
}

func reconcileSANNodeAccess(ctx context.Context, clientAPI *api.Client, igroupName string, nodeIQNs []string) error {

	err := ensureIGroupExists(clientAPI, igroupName)
	if err != nil {
		return err
	}

	// Discover mapped initiators
	var initiators []azgo.InitiatorInfoType
	iGroup, err := clientAPI.IgroupGet(igroupName)
	if err != nil {
		Logc(ctx).WithField("igroup", igroupName).Errorf("failed to read igroup info; %v", err)
		return fmt.Errorf("failed to read igroup info; err")
	}
	if iGroup.InitiatorsPtr != nil {
		initiators = iGroup.InitiatorsPtr.InitiatorInfo()
	} else {
		initiators = make([]azgo.InitiatorInfoType, 0)
	}
	mappedIQNs := make(map[string]bool)
	for _, initiator := range initiators {
		mappedIQNs[initiator.InitiatorName()] = true
	}

	// Add missing initiators
	for _, iqn := range nodeIQNs {
		if _, ok := mappedIQNs[iqn]; ok {
			// IQN is properly mapped; remove it from the list
			delete(mappedIQNs, iqn)
		} else {
			// IQN isn't mapped and should be; add it
			response, err := clientAPI.IgroupAdd(igroupName, iqn)
			err = api.GetError(ctx, response, err)
			zerr, zerrOK := err.(api.ZapiError)
			if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_HAS_NODE) {
				Logc(ctx).WithFields(log.Fields{
					"IQN":    iqn,
					"igroup": igroupName,
				}).Debug("Host IQN already in igroup.")
			} else {
				return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
			}
		}
	}

	// mappedIQNs is now a list of mapped IQNs that we have no nodes for; remove them
	for iqn := range mappedIQNs {
		response, err := clientAPI.IgroupRemove(igroupName, iqn, true)
		err = api.GetError(ctx, response, err)
		zerr, zerrOK := err.(api.ZapiError)
		if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_NODE_NOT_IN_INITGROUP) {
			Logc(ctx).WithFields(log.Fields{
				"IQN":    iqn,
				"igroup": igroupName,
			}).Debug("Host IQN not in igroup.")
		} else {
			return fmt.Errorf("error removing IQN %v from igroup %v: %v", iqn, igroupName, err)
		}
	}

	return nil
}

func cleanIgroups(ctx context.Context, client *api.Client, igroupName string) {

	response, err := client.IgroupDestroy(igroupName)
	err = api.GetError(ctx, response, err)
	zerr, zerrOK := err.(api.ZapiError)
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

// GetISCSITargetInfo returns the iSCSI node name and iSCSI interfaces using the provided client's SVM.
func GetISCSITargetInfo(
	clientAPI *api.Client, config *drivers.OntapStorageDriverConfig,
) (iSCSINodeName string, iSCSIInterfaces []string, returnError error) {

	// Get the SVM iSCSI IQN
	nodeNameResponse, err := clientAPI.IscsiNodeGetNameRequest()
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI node name: %v", err)
		return
	}
	iSCSINodeName = nodeNameResponse.Result.NodeName()

	// Get the SVM iSCSI interfaces
	interfaceResponse, err := clientAPI.IscsiInterfaceGetIterRequest()
	if err != nil {
		returnError = fmt.Errorf("could not get SVM iSCSI interfaces: %v", err)
		return
	}
	if interfaceResponse.Result.AttributesListPtr != nil {
		for _, iscsiAttrs := range interfaceResponse.Result.AttributesListPtr.IscsiInterfaceListEntryInfoPtr {
			if !iscsiAttrs.IsInterfaceEnabled() {
				continue
			}
			iSCSIInterface := fmt.Sprintf("%s:%d", iscsiAttrs.IpAddress(), iscsiAttrs.IpPort())
			iSCSIInterfaces = append(iSCSIInterfaces, iSCSIInterface)
		}
	}
	if len(iSCSIInterfaces) == 0 {
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

// PopulateOntapLunMapping helper function to fill in volConfig with its LUN mapping values.
// This function assumes that the list of data LIFs has not changed since driver initialization and volume creation
func PopulateOntapLunMapping(
	ctx context.Context, clientAPI *api.Client, config *drivers.OntapStorageDriverConfig,
	ips []string, volConfig *storage.VolumeConfig, lunID int, lunPath, igroupName string,
) error {

	var (
		targetIQN string
	)
	response, err := clientAPI.IscsiServiceGetIterRequest()
	if err != nil || response.Result.ResultStatusAttr != "passed" {
		return fmt.Errorf("problem retrieving iSCSI services: %v, %v",
			err, response.Result.ResultErrnoAttr)
	}
	if response.Result.AttributesListPtr != nil {
		for _, serviceInfo := range response.Result.AttributesListPtr.IscsiServiceInfoPtr {
			if serviceInfo.Vserver() == config.SVM {
				targetIQN = serviceInfo.NodeName()
				Logc(ctx).WithFields(log.Fields{
					"volume":    volConfig.Name,
					"targetIQN": targetIQN,
				}).Debug("Discovered target IQN for volume.")
				break
			}
		}
	}

	lunSerialResponse, err := clientAPI.LunGetSerialNumber(lunPath)
	if err != nil || "passed" != lunSerialResponse.Result.ResultStatusAttr {
		return fmt.Errorf("problem retrieving LUN info: %v, %v", err,
			lunSerialResponse.Result.ResultErrnoAttr)
	}
	serial := lunSerialResponse.Result.SerialNumber()

	filteredIPs, err := getISCSIDataLIFsForReportingNodes(ctx, clientAPI, ips, lunPath, igroupName)
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
	volConfig.AccessInfo.IscsiIgroup = config.IgroupName
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

// PublishLUN publishes the volume to the host specified in publishInfo from ontap-san or
// ontap-san-economy. This method may or may not be running on the host where the volume will be
// mounted, so it should limit itself to updating access rules, initiator groups, etc. that require
// some host identity (but not locality) as well as storage controller API access.
// This function assumes that the list of data LIF IP addresses does not change between driver initialization
// and publish
func PublishLUN(
	ctx context.Context, clientAPI *api.Client, config *drivers.OntapStorageDriverConfig, ips []string,
	publishInfo *utils.VolumePublishInfo, lunPath, igroupName string, iSCSINodeName string,
) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":  "PublishLUN",
			"Type":    "ontap_common",
			"lunPath": lunPath,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> PublishLUN")
		defer Logc(ctx).WithFields(fields).Debug("<<<< PublishLUN")
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
	attrResponse, err := clientAPI.LunGetAttribute(lunPath, LUNAttributeFSType)
	if err = api.GetError(ctx, attrResponse, err); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"LUN":    lunPath,
			"fstype": fstype,
		}).Warn("LUN attribute fstype not found, using default.")
	} else {
		fstype = attrResponse.Result.Value()
		Logc(ctx).WithFields(log.Fields{"LUN": lunPath, "fstype": fstype}).Debug("Found LUN attribute fstype.")
	}

	allowEmptyIQN := false
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

		// The IQN may be blank if we're in CSI mode and node prep is enabled and hasn't run yet.
		if targetNode.NodePrep.Enabled {
			switch targetNode.NodePrep.ISCSI {
			case "", utils.PrepPending, utils.PrepOutdated, utils.PrepRunning:
				allowEmptyIQN = true
			}
		}
	}

	if !publishInfo.Unmanaged {
		if iqn != "" {
			// Add IQN to igroup
			igroupAddResponse, err := clientAPI.IgroupAdd(igroupName, iqn)
			err = api.GetError(ctx, igroupAddResponse, err)
			zerr, zerrOK := err.(api.ZapiError)
			if err == nil || (zerrOK && zerr.Code() == azgo.EVDISK_ERROR_INITGROUP_HAS_NODE) {
				Logc(ctx).WithFields(
					log.Fields{
						"IQN":    iqn,
						"igroup": igroupName,
					}).Debug("Host IQN already in igroup.")
			} else {
				return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
			}
		} else if !allowEmptyIQN {
			err = fmt.Errorf("unknown initiator for node %s", publishInfo.HostName)
			Logc(ctx).Error(err)
			return err
		}
	}

	// Map LUN (it may already be mapped)
	lunID, err := clientAPI.LunMapIfNotMapped(ctx, igroupName, lunPath, publishInfo.Unmanaged)
	if err != nil {
		return err
	}

	filteredIPs, err := getISCSIDataLIFsForReportingNodes(ctx, clientAPI, ips, lunPath, igroupName)
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

// getISCSIDataLIFsForReportingNodes finds the data LIFs for the reporting nodes for the LUN.
func getISCSIDataLIFsForReportingNodes(
	ctx context.Context, clientAPI *api.Client, ips []string, lunPath string, igroupName string,
) ([]string, error) {

	lunMapGetResponse, err := clientAPI.LunMapGet(igroupName, lunPath)
	if err != nil {
		return nil, fmt.Errorf("could not get iSCSI reported nodes: %v", err)
	}

	reportingNodeNames := make(map[string]struct{})
	if lunMapGetResponse.Result.AttributesListPtr != nil {
		for _, lunMapInfo := range lunMapGetResponse.Result.AttributesListPtr {
			for _, reportingNode := range lunMapInfo.ReportingNodes() {
				Logc(ctx).WithField("reportingNode", reportingNode).Debug("Reporting node found.")
				reportingNodeNames[reportingNode] = struct{}{}
			}
		}
	}

	var reportedDataLIFs []string
	for _, ip := range ips {
		currentNodeName, err := clientAPI.NetInterfaceGetDataLIFsNode(ctx, ip)
		if err != nil {
			return nil, err
		}
		if _, ok := reportingNodeNames[currentNodeName]; ok {
			reportedDataLIFs = append(reportedDataLIFs, ip)
		}
	}

	Logc(ctx).WithField("reportedDataLIFs", reportedDataLIFs).Debug("Data LIFs with reporting nodes")
	return reportedDataLIFs, nil
}

// randomString returns a string of the specified length.
func randomChapString(strSize int) (string, error) {
	b := make([]byte, strSize)
	_, err := cryptorand.Read(b)
	if err != nil {
		log.Error(err)
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	return encoded, nil
}

// randomString returns a string of length 16 (128 bits)
func randomChapString16() (string, error) {
	s, err := randomChapString(256)
	if err != nil {
		return "", err
	}
	if s == "" || len(s) < 256 {
		return "", fmt.Errorf("invalid random string created '%s'", s)
	}

	result := ""
	for i := 0; len(result) < 16; i++ {
		if s[i] == '+' || s[i] == '/' || s[i] == '=' {
			continue
		} else {
			result += string(s[i])
		}
	}

	return result[0:16], nil
}

// ChapCredentials holds the bidrectional chap settings
type ChapCredentials struct {
	ChapUsername              string
	ChapInitiatorSecret       string
	ChapTargetUsername        string
	ChapTargetInitiatorSecret string
}

// ValidateBidrectionalChapCredentials validates the bidirectional CHAP settings
func ValidateBidrectionalChapCredentials(getDefaultAuthResponse *azgo.IscsiInitiatorGetDefaultAuthResponse, config *drivers.OntapStorageDriverConfig) (*ChapCredentials, error) {

	isDefaultAuthTypeNone, err := IsDefaultAuthTypeNone(getDefaultAuthResponse)
	if err != nil {
		return nil, fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeCHAP, err := IsDefaultAuthTypeCHAP(getDefaultAuthResponse)
	if err != nil {
		return nil, fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	isDefaultAuthTypeDeny, err := IsDefaultAuthTypeDeny(getDefaultAuthResponse)
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
		if getDefaultAuthResponse.Result.UserNamePtr == nil ||
			getDefaultAuthResponse.Result.OutboundUserNamePtr == nil {
			return nil, fmt.Errorf("error checking default initiator's credentials")
		}

		if config.ChapUsername != getDefaultAuthResponse.Result.UserName() ||
			config.ChapTargetUsername != getDefaultAuthResponse.Result.OutboundUserName() {
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
func isDefaultAuthTypeOfType(response *azgo.IscsiInitiatorGetDefaultAuthResponse, authType string) (bool, error) {
	if response == nil {
		return false, fmt.Errorf("response is nil")
	}

	if response.Result.AuthTypePtr == nil {
		return false, fmt.Errorf("response's auth-type is nil")
	}

	// case insensitive compare
	return strings.EqualFold(response.Result.AuthType(), authType), nil
}

// IsDefaultAuthTypeNone returns true if the default initiator's auth-type field is set to the value "none"
func IsDefaultAuthTypeNone(response *azgo.IscsiInitiatorGetDefaultAuthResponse) (bool, error) {
	return isDefaultAuthTypeOfType(response, "none")
}

// IsDefaultAuthTypeCHAP returns true if the default initiator's auth-type field is set to the value "CHAP"
func IsDefaultAuthTypeCHAP(response *azgo.IscsiInitiatorGetDefaultAuthResponse) (bool, error) {
	return isDefaultAuthTypeOfType(response, "CHAP")
}

// IsDefaultAuthTypeDeny returns true if the default initiator's auth-type field is set to the value "deny"
func IsDefaultAuthTypeDeny(response *azgo.IscsiInitiatorGetDefaultAuthResponse) (bool, error) {
	return isDefaultAuthTypeOfType(response, "deny")
}

// InitializeSANDriver performs common ONTAP SAN driver initialization.
func InitializeSANDriver(
	ctx context.Context, driverContext tridentconfig.DriverContext, clientAPI *api.Client,
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
	err := ensureIGroupExists(clientAPI, config.IgroupName)
	if err != nil {
		return err
	}
	if driverContext == tridentconfig.ContextKubernetes {
		Logc(ctx).WithFields(log.Fields{
			"driver": drivers.OntapSANStorageDriverName,
			"SVM":    config.SVM,
			"igroup": config.IgroupName,
		}).Warn("Please ensure all relevant hosts are added to the initiator group.")
	}

	getDefaultAuthResponse, err := clientAPI.IscsiInitiatorGetDefaultAuth()
	Logc(ctx).WithFields(log.Fields{
		"getDefaultAuthResponse": getDefaultAuthResponse,
		"err":                    err,
	}).Debug("IscsiInitiatorGetDefaultAuth result")

	if zerr := api.NewZapiError(getDefaultAuthResponse); !zerr.IsPassed() {
		return fmt.Errorf("error checking default initiator's auth type: %v", zerr)
	}

	isDefaultAuthTypeNone, err := IsDefaultAuthTypeNone(getDefaultAuthResponse)
	if err != nil {
		return fmt.Errorf("error checking default initiator's auth type: %v", err)
	}

	if config.UseCHAP {

		authType := "CHAP"
		chapCredentials, err := ValidateBidrectionalChapCredentials(getDefaultAuthResponse, config)
		if err != nil {
			return fmt.Errorf("error with CHAP credentials: %v", err)
		}
		Logc(ctx).Debug("Using CHAP credentials")

		if isDefaultAuthTypeNone {
			lunsResponse, lunsResponseErr := clientAPI.LunGetAllForVserver(config.SVM)
			if lunsResponseErr != nil {
				return lunsResponseErr
			}
			if lunsResponseErr = api.GetError(ctx, lunsResponse, lunsResponseErr); lunsResponseErr != nil {
				return fmt.Errorf("error enumerating LUNs for SVM %v: %v", config.SVM, lunsResponseErr)
			}

			if lunsResponse.Result.AttributesListPtr != nil &&
				lunsResponse.Result.AttributesListPtr.LunInfoPtr != nil {
				if len(lunsResponse.Result.AttributesListPtr.LunInfoPtr) > 0 {
					return fmt.Errorf(
						"will not enable CHAP for SVM %v; %v exisiting LUNs would lose access",
						config.SVM,
						len(lunsResponse.Result.AttributesListPtr.LunInfoPtr))
				}
			}
		}

		setDefaultAuthResponse, err := clientAPI.IscsiInitiatorSetDefaultAuth(
			authType,
			chapCredentials.ChapUsername, chapCredentials.ChapInitiatorSecret,
			chapCredentials.ChapTargetUsername, chapCredentials.ChapTargetInitiatorSecret)
		if err != nil {
			return fmt.Errorf("error setting CHAP credentials: %v", err)
		}
		if zerr := api.NewZapiError(setDefaultAuthResponse); !zerr.IsPassed() {
			return fmt.Errorf("error setting CHAP credentials: %v", zerr)
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

func ensureIGroupExists(clientAPI *api.Client, igroupName string) error {
	igroupResponse, err := clientAPI.IgroupCreate(igroupName, "iscsi", "linux")
	if err != nil {
		return fmt.Errorf("error creating igroup: %v", err)
	}
	if zerr := api.NewZapiError(igroupResponse); !zerr.IsPassed() {
		// Handle case where the igroup already exists
		if zerr.Code() != azgo.EVDISK_ERROR_INITGROUP_EXISTS {
			return fmt.Errorf("error creating igroup %v: %v", igroupName, zerr)
		}
	}
	return nil
}

// InitializeOntapDriver sets up the API client and performs all other initialization tasks
// that are common to all the ONTAP drivers.
func InitializeOntapDriver(ctx context.Context, config *drivers.OntapStorageDriverConfig) (*api.Client, error) {

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
	client, err := InitializeOntapAPI(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not create Data ONTAP API client: %v", err)
	}

	// Make sure we're using a valid ONTAP version
	ontapi, err := client.SystemGetOntapiVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not determine Data ONTAP API version: %v", err)
	}
	if !client.SupportsFeature(ctx, api.MinimumONTAPIVersion) {
		return nil, errors.New("ONTAP 9.3 or later is required")
	}
	Logc(ctx).WithField("Ontapi", ontapi).Debug("ONTAP API version.")

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

// InitializeOntapAPI returns an ontap.Client ZAPI client.  If the SVM isn't specified in the config
// file, this method attempts to derive the one to use.
func InitializeOntapAPI(ctx context.Context, config *drivers.OntapStorageDriverConfig) (*api.Client, error) {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "InitializeOntapAPI", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> InitializeOntapAPI")
		defer Logc(ctx).WithFields(fields).Debug("<<<< InitializeOntapAPI")
	}

	client := api.NewClient(api.ClientConfig{
		ManagementLIF:        config.ManagementLIF,
		SVM:                  config.SVM,
		Username:             config.Username,
		Password:             config.Password,
		ClientCertificate:    config.ClientCertificate,
		ClientPrivateKey:     config.ClientPrivateKey,
		TrustedCACertificate: config.TrustedCACertificate,
		DriverContext:        config.DriverContext,
		DebugTraceFlags:      config.DebugTraceFlags,
	})
	if config.SVM != "" {

		vserverResponse, err := client.VserverGetRequest()
		if err = api.GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error reading SVM details: %v", err)
		}

		client.SVMUUID = string(vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid())

		Logc(ctx).WithField("SVM", config.SVM).Debug("Using specified SVM.")
		return client, nil
	}

	// Use VserverGetIterRequest to populate config.SVM if it wasn't specified and we can derive it
	vserverResponse, err := client.VserverGetIterRequest()
	if err = api.GetError(ctx, vserverResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating SVMs: %v", err)
	}

	if vserverResponse.Result.NumRecords() != 1 {
		return nil, errors.New("cannot derive SVM to use; please specify SVM in config file")
	}

	// Update everything to use our derived SVM
	config.SVM = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverName()
	svmUUID := string(vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].Uuid())

	client = api.NewClient(api.ClientConfig{
		ManagementLIF:        config.ManagementLIF,
		SVM:                  config.SVM,
		Username:             config.Username,
		Password:             config.Password,
		ClientCertificate:    config.ClientCertificate,
		ClientPrivateKey:     config.ClientPrivateKey,
		TrustedCACertificate: config.TrustedCACertificate,
		DriverContext:        config.DriverContext,
		DebugTraceFlags:      config.DebugTraceFlags,
	})
	client.SVMUUID = svmUUID

	Logc(ctx).WithField("SVM", config.SVM).Debug("Using derived SVM.")
	return client, nil
}

// ValidateSANDriver contains the validation logic shared between ontap-san and ontap-san-economy.
func ValidateSANDriver(ctx context.Context, _ *api.Client, config *drivers.OntapStorageDriverConfig,
	ips []string) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ValidateSANDriver", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> ValidateSANDriver")
		defer Logc(ctx).WithFields(fields).Debug("<<<< ValidateSANDriver")
	}

	// If the user sets the LIF to use in the config, disable multipathing and use just the one IP address
	if config.DataLIF != "" {
		// Make sure it's actually a valid address
		if ip := net.ParseIP(config.DataLIF); nil == ip {
			return fmt.Errorf("data LIF is not a valid IP: %s", config.DataLIF)
		}
		// Make sure the IP matches one of the LIFs
		found := false
		for _, ip := range ips {
			if config.DataLIF == ip {
				found = true
				break
			}
		}
		if found {
			Logc(ctx).WithField("ip", config.DataLIF).Debug("Found matching Data LIF.")
		} else {
			Logc(ctx).WithField("ip", config.DataLIF).Debug("Could not find matching Data LIF.")
			return fmt.Errorf("could not find Data LIF for %s", config.DataLIF)
		}
		// Replace the IPs with a singleton list
		ips = []string{config.DataLIF}
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

// ValidateNASDriver contains the validation logic shared between ontap-nas and ontap-nas-economy.
func ValidateNASDriver(ctx context.Context, api *api.Client, config *drivers.OntapStorageDriverConfig) error {

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
		return fmt.Errorf("no NAS data LIFs found on SVM %s", config.SVM)
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
		cleanDataLIF := strings.Replace(config.DataLIF, "[", "", 1)
		cleanDataLIF = strings.Replace(cleanDataLIF, "]", "", 1)
		_, err := ValidateDataLIF(ctx, cleanDataLIF, dataLIFs)
		if err != nil {
			return fmt.Errorf("data LIF validation failed: %v", err)
		}
	}

	return nil
}

func ValidateStoragePrefix(storagePrefix string) error {

	// Ensure storage prefix is compatible with ONTAP
	matched, err := regexp.MatchString(`^$|^[a-zA-Z_.-][a-zA-Z0-9_.-]*$`, storagePrefix)
	if err != nil {
		err = fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		err = fmt.Errorf("storage prefix may only contain letters/digits/underscore/dash and must begin with letter/underscore/dash")
	}

	return err
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

func ValidateDataLIF(ctx context.Context, dataLIF string, dataLIFs []string) ([]string, error) {

	addressesFromHostname, err := net.LookupHost(dataLIF)
	if err != nil {
		Logc(ctx).Error("Host lookup failed. ", err)
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
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

// Enable space-allocation by default. If not enabled, Data ONTAP takes the LUNs offline
// when they're seen as full.
// see: https://github.com/NetApp/trident/issues/135
const DefaultSpaceAllocation = "true"
const DefaultSpaceReserve = "none"
const DefaultSnapshotPolicy = "none"
const DefaultSnapshotReserve = ""
const DefaultUnixPermissions = "---rwxrwxrwx"
const DefaultSnapshotDir = "false"
const DefaultExportPolicy = "default"
const DefaultSecurityStyle = "unix"
const DefaultNfsMountOptionsDocker = "-o nfsvers=3"
const DefaultNfsMountOptionsKubernetes = ""
const DefaultSplitOnClone = "false"
const DefaultEncryption = "false"
const DefaultMirroring = "false"
const DefaultLimitAggregateUsage = ""
const DefaultLimitVolumeSize = ""
const DefaultTieringPolicy = ""

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func PopulateConfigurationDefaults(ctx context.Context, config *drivers.OntapStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "PopulateConfigurationDefaults", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> PopulateConfigurationDefaults")
		defer Logc(ctx).WithFields(fields).Debug("<<<< PopulateConfigurationDefaults")
	}

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
		config.SnapshotReserve = DefaultSnapshotReserve
	}

	if config.UnixPermissions == "" {
		config.UnixPermissions = DefaultUnixPermissions
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

	if config.SecurityStyle == "" {
		config.SecurityStyle = DefaultSecurityStyle
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

	if config.Encryption == "" {
		config.Encryption = DefaultEncryption
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

	Logc(ctx).WithFields(log.Fields{
		"StoragePrefix":       *config.StoragePrefix,
		"SpaceAllocation":     config.SpaceAllocation,
		"SpaceReserve":        config.SpaceReserve,
		"SnapshotPolicy":      config.SnapshotPolicy,
		"SnapshotReserve":     config.SnapshotReserve,
		"UnixPermissions":     config.UnixPermissions,
		"SnapshotDir":         config.SnapshotDir,
		"ExportPolicy":        config.ExportPolicy,
		"SecurityStyle":       config.SecurityStyle,
		"NfsMountOptions":     config.NfsMountOptions,
		"SplitOnClone":        config.SplitOnClone,
		"FileSystemType":      config.FileSystemType,
		"Encryption":          config.Encryption,
		"Mirroring":           config.Mirroring,
		"LimitAggregateUsage": config.LimitAggregateUsage,
		"LimitVolumeSize":     config.LimitVolumeSize,
		"Size":                config.Size,
		"TieringPolicy":       config.TieringPolicy,
		"AutoExportPolicy":    config.AutoExportPolicy,
		"AutoExportCIDRs":     config.AutoExportCIDRs,
	}).Debugf("Configuration defaults")

	return nil
}

func checkAggregateLimitsForFlexvol(
	ctx context.Context, flexvol string, requestedSizeInt uint64, config drivers.OntapStorageDriverConfig,
	client *api.Client,
) error {

	var aggregate, spaceReserve string

	volInfo, err := client.VolumeGet(flexvol)
	if err != nil {
		return err
	}
	if volInfo.VolumeIdAttributesPtr != nil {
		aggregate = volInfo.VolumeIdAttributesPtr.ContainingAggregateName()
	} else {
		return fmt.Errorf("aggregate info not available from Flexvol %s", flexvol)
	}
	if volInfo.VolumeSpaceAttributesPtr != nil {
		spaceReserve = volInfo.VolumeSpaceAttributesPtr.SpaceGuarantee()
	} else {
		return fmt.Errorf("spaceReserve info not available from Flexvol %s", flexvol)
	}

	return checkAggregateLimits(ctx, aggregate, spaceReserve, requestedSizeInt, config, client)
}

func checkAggregateLimits(
	ctx context.Context, aggregate, spaceReserve string, requestedSizeInt uint64,
	config drivers.OntapStorageDriverConfig, client *api.Client,
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
	aggrSpaceResponse, aggrSpaceErr := client.AggrSpaceGetIterRequest(aggregate)
	if aggrSpaceErr != nil {
		return aggrSpaceErr
	}

	// iterate over results
	if aggrSpaceResponse.Result.AttributesListPtr != nil {
		for _, aggrSpace := range aggrSpaceResponse.Result.AttributesListPtr.SpaceInformationPtr {
			aggrName := aggrSpace.Aggregate()
			if aggregate != aggrName {
				Logc(ctx).Debugf("Skipping " + aggrName)
				continue
			}

			Logc(ctx).WithFields(log.Fields{
				"aggrName":                            aggrName,
				"size":                                aggrSpace.AggregateSize(),
				"volumeFootprints":                    aggrSpace.VolumeFootprints(),
				"volumeFootprintsPercent":             aggrSpace.VolumeFootprintsPercent(),
				"usedIncludingSnapshotReserve":        aggrSpace.UsedIncludingSnapshotReserve(),
				"usedIncludingSnapshotReservePercent": aggrSpace.UsedIncludingSnapshotReservePercent(),
			}).Info("Dumping aggregate space")

			if limitAggregateUsage != "" {
				percentLimit, parseErr := strconv.ParseFloat(limitAggregateUsage, 64)
				if parseErr != nil {
					return parseErr
				}

				usedIncludingSnapshotReserve := float64(aggrSpace.UsedIncludingSnapshotReserve())
				aggregateSize := float64(aggrSpace.AggregateSize())

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
	}

	return errors.New("could not find aggregate, cannot check aggregate provisioning limits for " + aggregate)
}

func GetVolumeSize(sizeBytes uint64, poolDefaultSizeBytes string) (uint64, error) {

	if sizeBytes == 0 {
		defaultSize, _ := utils.ConvertSizeToBytes(poolDefaultSizeBytes)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		return 0, fmt.Errorf("requested volume size (%d bytes) is too small; "+
			"the minimum volume size is %d bytes", sizeBytes, MinimumVolumeSizeBytes)
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
		if snapshotPolicy == "none" {
			return 0, nil
		} else {
			return api.NumericalValueNotSet, nil
		}
	}
}

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

	emsResponse, err := driver.GetAPI().EmsAutosupportLog(
		strconv.Itoa(drivers.ConfigVersion), false, "heartbeat", hostname,
		string(message), 1, tridentconfig.OrchestratorName, 5)

	if err = api.GetError(ctx, emsResponse, err); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"driver": driver.Name(),
			"error":  err,
		}).Error("Error logging EMS message.")
	} else {
		Logc(ctx).WithField("driver", driver.Name()).Debug("Logged EMS message.")
	}
}

const MSecPerHour = 1000 * 60 * 60 // millis * seconds * minutes

// probeForVolume polls for the ONTAP volume to appear, with backoff retry logic
func probeForVolume(ctx context.Context, name string, client *api.Client) error {

	checkVolumeExists := func() error {
		volExists, err := client.VolumeExists(ctx, name)
		if err != nil {
			return err
		}
		if !volExists {
			return fmt.Errorf("volume %v does not yet exist", name)
		}
		return nil
	}
	volumeExistsNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Volume not yet present, waiting.")
	}
	volumeBackoff := backoff.NewExponentialBackOff()
	volumeBackoff.InitialInterval = 1 * time.Second
	volumeBackoff.Multiplier = 2
	volumeBackoff.RandomizationFactor = 0.1
	volumeBackoff.MaxElapsedTime = 30 * time.Second

	// Run the volume check using an exponential backoff
	if err := backoff.RetryNotify(checkVolumeExists, volumeBackoff, volumeExistsNotify); err != nil {
		Logc(ctx).WithField("volume", name).Warnf("Could not find volume after %3.2f seconds.", volumeBackoff.MaxElapsedTime.Seconds())
		return fmt.Errorf("volume %v does not exist", name)
	} else {
		Logc(ctx).WithField("volume", name).Debug("Volume found.")
		return nil
	}
}

// Create a volume clone
func CreateOntapClone(
	ctx context.Context, name, source, snapshot, labels string, split bool, config *drivers.OntapStorageDriverConfig,
	client *api.Client, useAsync bool, qosPolicyGroup api.QosPolicyGroup,
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
		snapResponse, err := client.SnapshotCreate(snapshot, source)
		if err = api.GetError(ctx, snapResponse, err); err != nil {
			return fmt.Errorf("error creating snapshot: %v", err)
		}
	}

	// Create the clone based on a snapshot
	if useAsync {
		cloneResponse, err := client.VolumeCloneCreateAsync(name, source, snapshot)
		err = client.WaitForAsyncResponse(ctx, cloneResponse, maxFlexGroupCloneWait)
		if err != nil {
			return errors.New("waiting for async response failed")
		}
	} else {
		cloneResponse, err := client.VolumeCloneCreate(name, source, snapshot)
		if err != nil {
			return fmt.Errorf("error creating clone: %v", err)
		}
		if zerr := api.NewZapiError(cloneResponse); !zerr.IsPassed() {
			return handleCreateOntapCloneErr(ctx, zerr, client, snapshot, source, name)
		}
	}

	modifyCommentResponse, err := client.VolumeSetComment(ctx, name, labels)
	if err = api.GetError(ctx, modifyCommentResponse, err); err != nil {
		Logc(ctx).WithField("name", name).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", name, err)
	}

	if config.StorageDriverName == drivers.OntapNASStorageDriverName {
		// Mount the new volume
		mountResponse, err := client.VolumeMount(name, "/"+name)
		if err = api.GetError(ctx, mountResponse, err); err != nil {
			return fmt.Errorf("error mounting volume to junction: %v", err)
		}
	}

	// Set the QoS Policy if necessary
	if qosPolicyGroup.Kind != api.InvalidQosPolicyGroupKind {
		qosResponse, err := client.VolumeSetQosPolicyGroupName(name, qosPolicyGroup)
		if err = api.GetError(ctx, qosResponse, err); err != nil {
			return fmt.Errorf("error setting QoS policy: %v", err)
		}
	}

	// Split the clone if requested
	if split {
		splitResponse, err := client.VolumeCloneSplitStart(name)
		if err = api.GetError(ctx, splitResponse, err); err != nil {
			return fmt.Errorf("error splitting clone: %v", err)
		}
	}

	return nil
}

func handleCreateOntapCloneErr(
	ctx context.Context, zerr api.ZapiError, client *api.Client, snapshot, source, name string,
) error {

	if zerr.Code() == azgo.EOBJECTNOTFOUND {
		return fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, source)
	} else if zerr.IsFailedToLoadJobError() {
		fields := log.Fields{
			"zerr": zerr,
		}
		Logc(ctx).WithFields(fields).Warn(
			"Problem encountered during the clone create operation, " +
				"attempting to verify the clone was actually created")
		if volumeLookupError := probeForVolume(ctx, name, client); volumeLookupError != nil {
			return volumeLookupError
		}
	} else {
		return fmt.Errorf("error creating clone: %v", zerr)
	}

	return nil
}

// GetSnapshot gets a snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func GetSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.Client, sizeGetter func(string) (int, error),
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

	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapListResponse, err := client.SnapshotList(internalVolName)
	if err = api.GetError(ctx, snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {
			if snap.Name() == internalSnapName {

				Logc(ctx).WithFields(log.Fields{
					"snapshotName": internalSnapName,
					"volumeName":   internalVolName,
					"created":      snap.AccessTime(),
				}).Debug("Found snapshot.")

				return &storage.Snapshot{
					Config:    snapConfig,
					Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
					SizeBytes: int64(size),
					State:     storage.SnapshotStateOnline,
				}, nil
			}
		}
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Warning("Snapshot not found.")

	return nil, nil
}

// GetSnapshots returns the list of snapshots associated with the named volume.
func GetSnapshots(
	ctx context.Context, volConfig *storage.VolumeConfig, config *drivers.OntapStorageDriverConfig, client *api.Client,
	sizeGetter func(string) (int, error),
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

	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapListResponse, err := client.SnapshotList(internalVolName)
	if err = api.GetError(ctx, snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	Logc(ctx).Debugf("Returned %v snapshots.", snapListResponse.Result.NumRecords())
	snapshots := make([]*storage.Snapshot, 0)

	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {

			Logc(ctx).WithFields(log.Fields{
				"name":       snap.Name(),
				"accessTime": snap.AccessTime(),
			}).Debug("Snapshot")

			snapshot := &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Version:            tridentconfig.OrchestratorAPIVersion,
					Name:               snap.Name(),
					InternalName:       snap.Name(),
					VolumeName:         volConfig.Name,
					VolumeInternalName: volConfig.InternalName,
				},
				Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
				SizeBytes: int64(size),
				State:     storage.SnapshotStateOnline,
			}

			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots, nil
}

// CreateSnapshot creates a snapshot for the given volume.
func CreateSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.Client, sizeGetter func(string) (int, error),
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

	size, err := sizeGetter(internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error reading volume size: %v", err)
	}

	snapResponse, err := client.SnapshotCreate(internalSnapName, internalVolName)
	if err = api.GetError(ctx, snapResponse, err); err != nil {
		return nil, fmt.Errorf("could not create snapshot: %v", err)
	}

	// Fetching list of snapshots to get snapshot access time
	snapListResponse, err := client.SnapshotList(internalVolName)
	if err = api.GetError(ctx, snapListResponse, err); err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}
	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {
			if snap.Name() == internalSnapName {
				return &storage.Snapshot{
					Config:    snapConfig,
					Created:   time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
					SizeBytes: int64(size),
					State:     storage.SnapshotStateOnline,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find snapshot %s for souce volume %s", internalSnapName, internalVolName)
}

// Restore a volume (in place) from a snapshot.
func RestoreSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.Client,
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

	snapResponse, err := client.SnapshotRestoreVolume(internalSnapName, internalVolName)

	if err = api.GetError(ctx, snapResponse, err); err != nil {
		return fmt.Errorf("error restoring snapshot: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Debug("Restored snapshot.")

	return nil
}

// DeleteSnapshot deletes a single snapshot.
func DeleteSnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.Client,
) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "DeleteSnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> DeleteSnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< DeleteSnapshot")
	}

	snapResponse, err := client.SnapshotDelete(internalSnapName, internalVolName)

	if err != nil {
		return fmt.Errorf("error deleting snapshot: %v", err)
	}
	if zerr := api.NewZapiError(snapResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.ESNAPSHOTBUSY {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			_ = SplitVolumeFromBusySnapshot(ctx, snapConfig, config, client)
		}
		return fmt.Errorf("error deleting snapshot: %v", zerr)
	}

	Logc(ctx).WithField("snapshotName", internalSnapName).Debug("Deleted snapshot.")
	return nil
}

// SplitVolumeFromBusySnapshot gets the list of volumes backed by a busy snapshot and starts
// a split operation on the first one (sorted by volume name).
func SplitVolumeFromBusySnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client *api.Client,
) error {

	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":       "SplitVolumeFromBusySnapshot",
			"Type":         "ontap_common",
			"snapshotName": internalSnapName,
			"volumeName":   internalVolName,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> SplitVolumeFromBusySnapshot")
		defer Logc(ctx).WithFields(fields).Debug("<<<< SplitVolumeFromBusySnapshot")
	}

	childVolumes, err := client.VolumeListAllBackedBySnapshot(ctx, internalVolName, internalSnapName)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     internalSnapName,
			"parentVolumeName": internalVolName,
			"error":            err,
		}).Error("Could not list volumes backed by snapshot.")
		return err
	} else if len(childVolumes) == 0 {
		return nil
	}

	// We're going to start a single split operation, but there could be multiple children, so we
	// sort the volumes by name to not have more than one split operation running at a time.
	sort.Strings(childVolumes)

	splitResponse, err := client.VolumeCloneSplitStart(childVolumes[0])
	if err = api.GetError(ctx, splitResponse, err); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     internalSnapName,
			"parentVolumeName": internalVolName,
			"cloneVolumeName":  childVolumes[0],
			"error":            err,
		}).Error("Could not begin splitting clone from snapshot.")
		return fmt.Errorf("error splitting clone: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
		"snapshotName":     internalSnapName,
		"parentVolumeName": internalVolName,
		"cloneVolumeName":  childVolumes[0],
	}).Info("Began splitting clone from snapshot.")

	return nil
}

// GetVolume checks for the existence of a volume.  It returns nil if the volume
// exists and an error if it does not (or the API call fails).
func GetVolume(ctx context.Context, name string, client *api.Client, config *drivers.OntapStorageDriverConfig) error {

	if config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "GetVolume", "Type": "ontap_common"}
		Logc(ctx).WithFields(fields).Debug(">>>> GetVolume")
		defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolume")
	}

	volExists, err := client.VolumeExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}
	if !volExists {
		Logc(ctx).WithField("flexvol", name).Debug("Flexvol not found.")
		return fmt.Errorf("volume %s does not exist", name)
	}

	return nil
}

type ontapPerformanceClass string

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
	vserverAggrs, err := client.VserverGetAggregateNames()
	if err != nil {
		return nil, err
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates", config.SVM)
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"svm":   config.SVM,
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
			config.SVM, config.Aggregate)
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

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.GetAPI().VserverShowAggrGetIterRequest()
	if err != nil {
		return
	}

	if zerr := api.NewZapiError(result.Result); !zerr.IsPassed() {
		err = zerr
		return
	}

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.ShowAggregatesPtr {
			aggrName := string(aggr.AggregateName())
			aggrType := aggr.AggregateType()

			// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
			_, ok := (*poolsAttributeMap)[aggrName]
			if !ok {
				continue
			}

			// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
			storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
			if !ok {
				Logc(ctx).WithFields(log.Fields{
					"aggregate": aggrName,
					"mediaType": aggrType,
				}).Debug("Aggregate has unknown performance characteristics.")

				continue
			}

			Logc(ctx).WithFields(log.Fields{
				"aggregate": aggrName,
				"mediaType": aggrType,
			}).Debug("Read aggregate attributes.")

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

	return fmt.Sprintf("%s_%s",
		backendName,
		strings.Replace(name, "-", "", -1))
}

func InitializeStoragePoolsCommon(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]*storage.Pool, map[string]*storage.Pool, error) {

	config := d.GetConfig()
	physicalPools := make(map[string]*storage.Pool)
	virtualPools := make(map[string]*storage.Pool)

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

	if zerr, ok := aggrErr.(api.ZapiError); ok && zerr.IsScopeError() {
		Logc(ctx).WithFields(log.Fields{
			"username": config.Username,
		}).Warn("User has insufficient privileges to obtain aggregate info. " +
			"Storage classes with physical attributes such as 'media' will not match pools on this backend.")
	} else if aggrErr != nil {
		Logc(ctx).Errorf("Could not obtain aggregate info; storage classes with physical attributes such as 'media' will"+
			" not match pools on this backend: %v.", aggrErr)
	}

	// Define physical pools
	for _, physicalStoragePoolName := range physicalStoragePoolNames {

		pool := storage.NewStoragePool(nil, physicalStoragePoolName)

		// Update pool with attributes set by default for this backend
		// We do not set internal attributes with these values as this
		// merely means that pools supports these capabilities like
		// encryption, cloning, thick/thin provisioning
		for attrName, offer := range poolAttributes {
			pool.Attributes[attrName] = offer
		}

		attrMap := physicalStoragePoolAttributes[physicalStoragePoolName]

		// Update pool with attributes based on aggregate attributes discovered on the backend
		for attrName, attrValue := range attrMap {
			pool.Attributes[attrName] = attrValue
			pool.InternalAttributes[attrName] = attrValue.ToString()

			if attrName == sa.Media {
				mediaOffers = append(mediaOffers, attrValue)
			}
		}

		if config.Region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(config.Region)
		}
		if config.Zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(config.Zone)
		}

		pool.Attributes[sa.Labels] = sa.NewLabelOffer(config.Labels)

		pool.InternalAttributes[Size] = config.Size
		pool.InternalAttributes[Region] = config.Region
		pool.InternalAttributes[Zone] = config.Zone
		pool.InternalAttributes[SpaceReserve] = config.SpaceReserve
		pool.InternalAttributes[SnapshotPolicy] = config.SnapshotPolicy
		pool.InternalAttributes[SnapshotReserve] = config.SnapshotReserve
		pool.InternalAttributes[SplitOnClone] = config.SplitOnClone
		pool.InternalAttributes[Encryption] = config.Encryption
		pool.InternalAttributes[UnixPermissions] = config.UnixPermissions
		pool.InternalAttributes[SnapshotDir] = config.SnapshotDir
		pool.InternalAttributes[ExportPolicy] = config.ExportPolicy
		pool.InternalAttributes[SecurityStyle] = config.SecurityStyle
		pool.InternalAttributes[TieringPolicy] = config.TieringPolicy
		pool.InternalAttributes[QosPolicy] = config.QosPolicy
		pool.InternalAttributes[AdaptiveQosPolicy] = config.AdaptiveQosPolicy

		pool.SupportedTopologies = config.SupportedTopologies

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes[SpaceAllocation] = config.SpaceAllocation
			pool.InternalAttributes[FileSystemType] = config.FileSystemType
		}

		physicalPools[pool.Name] = pool
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
			pool.Attributes[attrName] = offer
		}

		pool.Attributes[sa.Labels] = sa.NewLabelOffer(config.Labels, vpool.Labels)

		if region != "" {
			pool.Attributes[sa.Region] = sa.NewStringOffer(region)
		}
		if zone != "" {
			pool.Attributes[sa.Zone] = sa.NewStringOffer(zone)
		}
		if len(mediaOffers) > 0 {
			pool.Attributes[sa.Media] = sa.NewStringOfferFromOffers(mediaOffers...)
			pool.InternalAttributes[Media] = pool.Attributes[sa.Media].ToString()
		}
		if encryption != "" {
			enableEncryption, err := strconv.ParseBool(encryption)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err,
					pool.Name)
			}
			pool.Attributes[sa.Encryption] = sa.NewBoolOffer(enableEncryption)
			pool.InternalAttributes[Encryption] = encryption
		}

		pool.InternalAttributes[Size] = size
		pool.InternalAttributes[Region] = region
		pool.InternalAttributes[Zone] = zone
		pool.InternalAttributes[SpaceReserve] = spaceReserve
		pool.InternalAttributes[SnapshotPolicy] = snapshotPolicy
		pool.InternalAttributes[SnapshotReserve] = snapshotReserve
		pool.InternalAttributes[SplitOnClone] = splitOnClone
		pool.InternalAttributes[UnixPermissions] = unixPermissions
		pool.InternalAttributes[SnapshotDir] = snapshotDir
		pool.InternalAttributes[ExportPolicy] = exportPolicy
		pool.InternalAttributes[SecurityStyle] = securityStyle
		pool.InternalAttributes[TieringPolicy] = tieringPolicy
		pool.InternalAttributes[QosPolicy] = qosPolicy
		pool.InternalAttributes[AdaptiveQosPolicy] = adaptiveQosPolicy
		pool.SupportedTopologies = supportedTopologies

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes[SpaceAllocation] = spaceAllocation
			pool.InternalAttributes[FileSystemType] = fileSystemType
		}

		virtualPools[pool.Name] = pool
	}

	return physicalPools, virtualPools, nil
}

// ValidateStoragePools makes sure that values are set for the fields, if value(s) were not specified
// for a field then a default should have been set in for that field in the initialize storage pools
func ValidateStoragePools(
	ctx context.Context, physicalPools, virtualPools map[string]*storage.Pool, d StorageDriver, labelLimit int) error {

	// Validate pool-level attributes
	allPools := make([]*storage.Pool, 0, len(physicalPools)+len(virtualPools))

	for _, pool := range physicalPools {
		allPools = append(allPools, pool)
	}
	for _, pool := range virtualPools {
		allPools = append(allPools, pool)
	}

	for _, pool := range allPools {

		poolName := pool.Name

		// Validate SpaceReserve
		switch pool.InternalAttributes[SpaceReserve] {
		case "none", "volume":
			break
		default:
			return fmt.Errorf("invalid spaceReserve %s in pool %s", pool.InternalAttributes[SpaceReserve], poolName)
		}

		// Validate SnapshotPolicy
		if pool.InternalAttributes[SnapshotPolicy] == "" {
			return fmt.Errorf("snapshot policy cannot by empty in pool %s", poolName)
		}

		// Validate Encryption
		if pool.InternalAttributes[Encryption] == "" {
			return fmt.Errorf("encryption cannot by empty in pool %s", poolName)
		} else {
			_, err := strconv.ParseBool(pool.InternalAttributes[Encryption])
			if err != nil {
				return fmt.Errorf("invalid value for encryption in pool %s: %v", poolName, err)
			}
		}
		// Validate snapshot dir
		if pool.InternalAttributes[SnapshotDir] == "" {
			return fmt.Errorf("snapshotDir cannot by empty in pool %s", poolName)
		} else {
			_, err := strconv.ParseBool(pool.InternalAttributes[SnapshotDir])
			if err != nil {
				return fmt.Errorf("invalid value for snapshotDir in pool %s: %v", poolName, err)
			}
		}

		_, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit)
		if err != nil {
			return fmt.Errorf("invalid value for label in pool %s: %v", poolName, err)
		}

		// Validate SecurityStyles
		switch pool.InternalAttributes[SecurityStyle] {
		case "unix", "mixed":
			break
		default:
			return fmt.Errorf("invalid securityStyle %s in pool %s", pool.InternalAttributes[SecurityStyle], poolName)
		}

		// Validate ExportPolicy
		if pool.InternalAttributes[ExportPolicy] == "" {
			return fmt.Errorf("export policy cannot by empty in pool %s", poolName)
		}

		// Validate UnixPermissions
		if pool.InternalAttributes[UnixPermissions] == "" {
			return fmt.Errorf("UNIX permissions cannot by empty in pool %s", poolName)
		}

		// Validate TieringPolicy
		switch pool.InternalAttributes[TieringPolicy] {
		case "snapshot-only", "auto", "none", "backup", "all", "":
			break
		default:
			return fmt.Errorf("invalid tieringPolicy %s in pool %s", pool.InternalAttributes[TieringPolicy],
				poolName)
		}

		// Validate QoS policy or adaptive QoS policy
		if pool.InternalAttributes[QosPolicy] != "" || pool.InternalAttributes[AdaptiveQosPolicy] != "" {
			if !d.GetAPI().SupportsFeature(ctx, api.QosPolicies) {
				return fmt.Errorf("trident does not support QoS policies for ONTAP version")
			}

			if _, err := api.NewQosPolicyGroup(pool.InternalAttributes[QosPolicy],
				pool.InternalAttributes[AdaptiveQosPolicy]); err != nil {
				return err
			}

			if d.Name() == drivers.OntapNASQtreeStorageDriverName && pool.InternalAttributes[AdaptiveQosPolicy] != "" {
				return fmt.Errorf("qtrees do not support adaptive QoS policies")
			}
		}

		// Validate media type
		if pool.InternalAttributes[Media] != "" {
			for _, mediaType := range strings.Split(pool.InternalAttributes[Media], ",") {
				switch mediaType {
				case sa.HDD, sa.SSD, sa.Hybrid:
					break
				default:
					Logc(ctx).Errorf("invalid media type in pool %s: %s", pool.Name, mediaType)
				}
			}
		}

		// Validate default size
		if defaultSize, err := utils.ConvertSizeToBytes(pool.InternalAttributes[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		} else {
			sizeBytes, _ := strconv.ParseUint(defaultSize, 10, 64)
			if sizeBytes < MinimumVolumeSizeBytes {
				return fmt.Errorf("invalid value for size in pool %s. Requested volume size ("+
					"%d bytes) is too small; the minimum volume size is %d bytes", poolName, sizeBytes, MinimumVolumeSizeBytes)
			}
		}

		// Cloning is not supported on ONTAP FlexGroups driver
		if d.Name() != drivers.OntapNASFlexGroupStorageDriverName {
			// Validate splitOnClone
			if pool.InternalAttributes[SplitOnClone] == "" {
				return fmt.Errorf("splitOnClone cannot by empty in pool %s", poolName)
			} else {
				_, err := strconv.ParseBool(pool.InternalAttributes[SplitOnClone])
				if err != nil {
					return fmt.Errorf("invalid value for splitOnClone in pool %s: %v", poolName, err)
				}
			}
		}

		if d.Name() == drivers.OntapSANStorageDriverName || d.Name() == drivers.OntapSANEconomyStorageDriverName {

			// Validate SpaceAllocation
			if pool.InternalAttributes[SpaceAllocation] == "" {
				return fmt.Errorf("spaceAllocation cannot by empty in pool %s", poolName)
			} else {
				_, err := strconv.ParseBool(pool.InternalAttributes[SpaceAllocation])
				if err != nil {
					return fmt.Errorf("invalid value for SpaceAllocation in pool %s: %v", poolName, err)
				}
			}

			// Validate FileSystemType
			if pool.InternalAttributes[FileSystemType] == "" {
				return fmt.Errorf("fileSystemType cannot by empty in pool %s", poolName)
			} else {
				_, err := drivers.CheckSupportedFilesystem(ctx, pool.InternalAttributes[FileSystemType], "")
				if err != nil {
					return fmt.Errorf("invalid value for fileSystemType in pool %s: %v", poolName, err)
				}
			}
		}
	}

	return nil
}

// getStorageBackendSpecsCommon updates the specified Backend object with StoragePools.
func getStorageBackendSpecsCommon(backend *storage.Backend, physicalPools,
	virtualPools map[string]*storage.Pool, backendName string) (err error) {

	backend.Name = backendName

	virtual := len(virtualPools) > 0

	for _, pool := range physicalPools {
		pool.Backend = backend
		if !virtual {
			backend.AddStoragePool(pool)
		}
	}

	for _, pool := range virtualPools {
		pool.Backend = backend
		if virtual {
			backend.AddStoragePool(pool)
		}
	}

	return nil
}

func getStorageBackendPhysicalPoolNamesCommon(physicalPools map[string]*storage.Pool) []string {
	physicalPoolNames := make([]string, 0)
	for poolName := range physicalPools {
		physicalPoolNames = append(physicalPoolNames, poolName)
	}
	return physicalPoolNames
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

// getPoolsForCreate returns candidate storage pools for creating volumes
func getPoolsForCreate(
	ctx context.Context, volConfig *storage.VolumeConfig, storagePool *storage.Pool,
	volAttributes map[string]sa.Request, physicalPools map[string]*storage.Pool, virtualPools map[string]*storage.Pool,
) ([]*storage.Pool, error) {

	// If a physical pool was requested, just use it
	if _, ok := physicalPools[storagePool.Name]; ok {
		return []*storage.Pool{storagePool}, nil
	}

	// If a virtual pool was requested, find a physical pool to satisfy it
	if _, ok := virtualPools[storagePool.Name]; !ok {
		return nil, fmt.Errorf("could not find pool %s", storagePool.Name)
	}

	// Make a storage class from the volume attributes to simplify pool matching
	attributesCopy := make(map[string]sa.Request)
	for k, v := range volAttributes {
		attributesCopy[k] = v
	}
	delete(attributesCopy, sa.Selector)
	storageClass := sc.NewFromAttributes(attributesCopy)

	// Find matching pools
	candidatePools := make([]*storage.Pool, 0)

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

	cloneConfig.Username = drivers.REDACTED         // redact the username
	cloneConfig.Password = drivers.REDACTED         // redact the password
	cloneConfig.ClientPrivateKey = drivers.REDACTED // redact the client private key
	cloneConfig.ChapInitiatorSecret = drivers.REDACTED
	cloneConfig.ChapTargetInitiatorSecret = drivers.REDACTED
	cloneConfig.ChapTargetUsername = drivers.REDACTED
	cloneConfig.ChapUsername = drivers.REDACTED
	cloneConfig.Credentials = map[string]string{drivers.KeyName: drivers.REDACTED,
		drivers.KeyType: drivers.REDACTED} // redact the credentials
	return cloneConfig
}

// resizeValidation performs needed validation checks prior to the resize operation.
func resizeValidation(
	ctx context.Context, name string, sizeBytes uint64, volumeExists func(context.Context, string) (bool, error),
	volumeSize func(string) (int, error),
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
	volSize, err := volumeSize(name)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Error checking volume size.")
		return 0, fmt.Errorf("error occurred when checking volume size")
	}
	volSizeBytes := uint64(volSize)

	if sizeBytes < volSizeBytes {
		return 0, fmt.Errorf("requested size %d is less than existing volume size %d", sizeBytes, volSize)
	}

	return volSizeBytes, nil
}

// Unmount a volume and then take it offline. This may need to be done before deleting certain types of volumes.
func UnmountAndOfflineVolume(ctx context.Context, API *api.Client, name string) (bool, error) {

	// This call is sync and idempotent
	umountResp, err := API.VolumeUnmount(name, true)
	if err != nil {
		return true, fmt.Errorf("error unmounting Volume %v: %v", name, err)
	}

	if zerr := api.NewZapiError(umountResp); !zerr.IsPassed() {
		if zerr.Code() == azgo.EOBJECTNOTFOUND {
			Logc(ctx).WithField("volume", name).Warn("Volume does not exist.")
			return false, nil
		} else {
			return true, fmt.Errorf("error unmounting Volume %v: %v", name, zerr)
		}
	}

	// This call is sync, but not idempotent, so we check if it is already offline
	offlineResp, offErr := API.VolumeOffline(name)
	if offErr != nil {
		return true, fmt.Errorf("error taking Volume %v offline: %v", name, offErr)
	}

	if zerr := api.NewZapiError(offlineResp); !zerr.IsPassed() {
		if zerr.Code() == azgo.EVOLUMEOFFLINE {
			Logc(ctx).WithField("volume", name).Warn("Volume already offline.")
		} else if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Debug("Volume already deleted, skipping destroy.")
			return false, nil
		} else {
			return true, fmt.Errorf("error taking Volume %v offline: %v", name, zerr)
		}
	}

	return true, nil
}

func calculateOptimalSizeForFlexvol(
	ctx context.Context, flexvol string, volAttrs *azgo.VolumeAttributesType, newLunOrQtreeSizeBytes,
	totalDiskLimitBytes uint64,
) uint64 {
	volSpaceAttrs := volAttrs.VolumeSpaceAttributes()
	snapReserveDivisor := 1.0 - (float64(volSpaceAttrs.PercentageSnapshotReserve()) / 100.0)
	snapshotSizeBytes := float64(volSpaceAttrs.SizeUsedBySnapshots())

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
		"snapshotReserve":        volSpaceAttrs.PercentageSnapshotReserve(),
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
