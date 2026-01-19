// Copyright 2025 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"maps"
	"math/rand"
	"net"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	tridentmodels "github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/version"
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
	MinimumVolumeSizeBytes          = 20971520 // 20 MiB
	HousekeepingStartupDelay        = 10 * time.Second
	LUNMetadataBufferMultiplier     = 1.1            // 10%
	MaximumIgroupNameLength         = 96             // 96 characters is the maximum character count for ONTAP igroups.
	ADAdminUserPermission           = "full_control" // AD admin user permission
	DefaultSMBAccessControlUser     = "Everyone"     // Default SMB access control user
	DefaultSMBAccessControlUserType = "windows"      // Default SMB access control user type

	// Constants for internal pool attributes
	Size                  = "size"
	NameTemplate          = "nameTemplate"
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
	FormatOptions         = "formatOptions"
	ProvisioningType      = "provisioningType"
	SplitOnClone          = "splitOnClone"
	TieringPolicy         = "tieringPolicy"
	SkipRecoveryQueue     = "skipRecoveryQueue"
	QosPolicy             = "qosPolicy"
	AdaptiveQosPolicy     = "adaptiveQosPolicy"
	ADAdminUser           = "adAdminUser"
	maxFlexGroupCloneWait = 120 * time.Second
	maxFlexvolCloneWait   = 30 * time.Second

	// maxSnapshotDeleteRetry and maxSnapshotDeleteWait should be used
	// together to balance snapshot deletion wait times and retries.
	maxSnapshotDeleteRetry = 10
	// maxSnapshotDeleteWait and maxSnapshotDeleteRetry should be used
	// together to balance snapshot deletion wait times and retries.
	maxSnapshotDeleteWait = 60 * time.Second

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

	// Default storage pool name when aggregates are managed automatically
	managedStoragePoolName = "managed_storage_pool"
)

// StateReason, Change in these strings require change in test automation.
const (
	StateReasonSVMStopped                 = "SVM is not in 'running' state"
	StateReasonDataLIFsDown               = "No data LIFs present or all of them are 'down'"
	StateReasonSVMUnreachable             = "SVM is not reachable"
	StateReasonNoAggregates               = "SVM does not contain any aggregates."
	StateReasonMissingAggregate           = "Aggregate defined in the aggregate field of the backend config is not present in the SVM"
	StateReasonMissingFlexGroupAggregates = "Some of the aggregates defined in the flexgroupAggregateList field of the backend config are not present in the SVM"
)

var (
	volumeCharRegex          = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	volumeNameRegex          = regexp.MustCompile(`\{+.*\.volume.Name[^{a-z]*\}+`)
	volumeNameStartWithRegex = regexp.MustCompile(`^[A-Za-z_].*`)
	smbShareDeleteACL        = map[string]string{DefaultSMBAccessControlUser: DefaultSMBAccessControlUserType}
	exportPolicyMutex        = locks.NewGCNamedMutex()

	// NOTE: The lock order should be lunMutex first, then igroupMutex
	lunMutex    = locks.NewGCNamedMutex()
	igroupMutex = locks.NewGCNamedMutex()

	// NOTE: The lock order should be namespaceMutex first, then subsystemMutex
	namespaceMutex = locks.NewGCNamedMutex()
	subsystemMutex = locks.NewGCNamedMutex()

	duringVolCloneAfterSnapCreation1 = fiji.Register("duringVolCloneAfterSnapCreation1", "ontap_common")
	duringVolCloneAfterSnapCreation2 = fiji.Register("duringVolCloneAfterSnapCreation2", "ontap_common")
)

// CleanBackendName removes brackets and replaces colons with periods to avoid regex parsing errors.
func CleanBackendName(backendName string) string {
	backendName = strings.ReplaceAll(backendName, "[", "")
	backendName = strings.ReplaceAll(backendName, "]", "")
	return strings.ReplaceAll(backendName, ":", ".")
}

func NewOntapTelemetry(ctx context.Context, d StorageDriver) *Telemetry {
	config := d.GetOntapConfig()
	t := &Telemetry{
		Plugin:        d.Name(),
		SVM:           config.SVM,
		StoragePrefix: *config.StoragePrefix,
		Driver:        d,
		done:          make(chan struct{}),
	}

	usageHeartbeat := config.UsageHeartbeat
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
		time.Sleep(HousekeepingStartupDelay)
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

	return config, nil
}

func ensureExportPolicyExists(ctx context.Context, policyName string, clientAPI api.OntapAPI) error {
	exportPolicyMutex.Lock(policyName)
	defer exportPolicyMutex.Unlock(policyName)

	return clientAPI.ExportPolicyCreate(ctx, policyName)
}

func destroyExportPolicy(ctx context.Context, policyName string, clientAPI api.OntapAPI) error {
	exportPolicyMutex.Lock(policyName)
	defer exportPolicyMutex.Unlock(policyName)

	return clientAPI.ExportPolicyDestroy(ctx, policyName)
}

func getExportPolicyName(backendUUID string) string {
	return fmt.Sprintf("trident-%s", backendUUID)
}

func getEmptyExportPolicyName(storagePrefix string) string {
	return fmt.Sprintf("%sempty", storagePrefix)
}

// ensureNodeAccess check to see if the export policy exists and if not it will create it and force a reconcile.
// This should be used during publish to make sure access is available if the policy has somehow been deleted.
// Otherwise we should not need to reconcile, which could be expensive.
func ensureNodeAccess(
	ctx context.Context, publishInfo *tridentmodels.VolumePublishInfo, clientAPI api.OntapAPI,
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
	ctx context.Context, nodes []*tridentmodels.Node, config *drivers.OntapStorageDriverConfig, clientAPI api.OntapAPI,
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
		err = fmt.Errorf("unable to reconcile export policy rules; %v", err)
		Logc(ctx).WithField("ExportPolicy", policyName).Error(err)
		return err
	}

	return nil
}

// ensureNodeAccessForPolicy check to see if export policy exists and if not it will create it.
// Add the desired rule(s) to the policy.
func ensureNodeAccessForPolicy(
	ctx context.Context, targetNode *tridentmodels.Node, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig, policyName string,
) error {
	fields := LogFields{
		"Method":        "ensureNodeAccessForPolicy",
		"Type":          "ontap_common",
		"policyName":    policyName,
		"targetNodeIPs": targetNode.IPs,
	}

	Logc(ctx).WithFields(fields).Debug(">>>> ensureNodeAccessForPolicy")
	defer Logc(ctx).WithFields(fields).Debug("<<<< ensureNodeAccessForPolicy")

	exportPolicyMutex.Lock(policyName)
	defer exportPolicyMutex.Unlock(policyName)

	if exists, err := clientAPI.ExportPolicyExists(ctx, policyName); err != nil {
		return err
	} else if !exists {
		Logc(ctx).WithField("exportPolicy", policyName).Debug("Export policy missing, will create it.")

		if err = clientAPI.ExportPolicyCreate(ctx, policyName); err != nil {
			return err
		}
	}

	desiredRules, err := network.FilterIPs(ctx, targetNode.IPs, config.AutoExportCIDRs)
	if err != nil {
		err = fmt.Errorf("unable to determine desired export policy rules; %v", err)
		Logc(ctx).Error(err)
		return err
	}
	Logc(ctx).WithField("desiredRules", desiredRules).Debug("Desired export policy rules.")

	// first grab all existing rules
	existingRules, err := clientAPI.ExportRuleList(ctx, policyName)
	if err != nil {
		// Could not list rules, just log it, no action required.
		Logc(ctx).WithField("error", err).Debug("Export policy rules could not be listed.")
	}
	Logc(ctx).WithField("existingRules", existingRules).Debug("Existing export policy rules.")

	for _, desiredRule := range desiredRules {
		desiredRule = strings.TrimSpace(desiredRule)

		desiredIP := net.ParseIP(desiredRule)
		if desiredIP == nil {
			Logc(ctx).WithField("desiredRule", desiredRule).Debug("Invalid desired rule IP")
			continue
		}

		// Loop through the existing rules one by one and compare to make sure we cover the scenario where the
		// existing rule is of format "1.1.1.1, 2.2.2.2" and the desired rule is format "1.1.1.1".
		// This can happen because of the difference in how ONTAP ZAPI and ONTAP REST creates export rule.

		ruleFound := false
		for _, existingRule := range existingRules {
			existingIPs := strings.Split(existingRule, ",")

			for _, ip := range existingIPs {
				ip = strings.TrimSpace(ip)

				existingIP := net.ParseIP(ip)
				if existingIP == nil {
					Logc(ctx).WithField("existingRule", existingRule).Debug("Invalid existing rule IP")
					continue
				}

				if existingIP.Equal(desiredIP) {
					ruleFound = true
					break
				}
			}

			if ruleFound {
				break
			}
		}

		// Rule does not exist, so create it
		if !ruleFound {
			if err = clientAPI.ExportRuleCreate(ctx, policyName, desiredRule, config.NASType); err != nil {
				// Check if error is that the export policy rule already exist error
				if errors.IsAlreadyExistsError(err) {
					Logc(ctx).WithField("desiredRule", desiredRule).WithError(err).Debug(
						"Export policy rule already exists")
					continue
				}
				return err
			}
		}
	}

	return nil
}

func getDesiredExportPolicyRules(
	ctx context.Context, nodes []*tridentmodels.Node, config *drivers.OntapStorageDriverConfig,
) ([]string, error) {
	uniqueRules := make(map[string]struct{})
	for _, node := range nodes {
		// Filter the IPs based on the CIDRs provided by user
		filteredIPs, err := network.FilterIPs(ctx, node.IPs, config.AutoExportCIDRs)
		if err != nil {
			return nil, err
		}
		for _, ip := range filteredIPs {
			uniqueRules[ip] = struct{}{}
		}
	}

	rules := make([]string, 0, len(uniqueRules))
	for ip := range uniqueRules {
		rules = append(rules, ip)
	}

	return rules, nil
}

func reconcileExportPolicyRules(
	ctx context.Context, policyName string, desiredPolicyRules []string, clientAPI api.OntapAPI,
	config *drivers.OntapStorageDriverConfig,
) error {
	fields := LogFields{
		"Method":             "reconcileExportPolicyRules",
		"Type":               "ontap_common",
		"policyName":         policyName,
		"desiredPolicyRules": desiredPolicyRules,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> reconcileExportPolicyRules")
	defer Logc(ctx).WithFields(fields).Debug("<<<< reconcileExportPolicyRules")

	exportPolicyMutex.Lock(policyName)
	defer exportPolicyMutex.Unlock(policyName)

	// first grab all existing rules
	existingRules, err := clientAPI.ExportRuleList(ctx, policyName)
	if err != nil {
		// Could not extract rules, just log it, no action required.
		Logc(ctx).WithField("error", err).Debug("Export policy rules could not be extracted.")
	}
	Logc(ctx).WithField("existingRules", existingRules).Debug("Existing export policy rules.")

	undesiredRules := maps.Clone(existingRules)

	for _, desiredRule := range desiredPolicyRules {
		desiredRule = strings.TrimSpace(desiredRule)

		desiredIP := net.ParseIP(desiredRule)
		if desiredIP == nil {
			Logc(ctx).WithField("desiredRule", desiredRule).Debug("Invalid desired rule IP")
			continue
		}

		// Loop through the existing rules one by one and compare to make sure we cover the scenario where the
		// existing rule is of format "1.1.1.1, 2.2.2.2" and the desired rule is format "1.1.1.1".
		// This can happen because of the difference in how ONTAP ZAPI and ONTAP REST creates export rule.

		existingRuleIndex := -1
		for ruleIndex, rule := range existingRules {
			existingIPs := strings.Split(rule, ",")

			for _, ip := range existingIPs {
				ip = strings.TrimSpace(ip)

				existingIP := net.ParseIP(ip)
				if existingIP == nil {
					Logc(ctx).WithField("existingRule", rule).Debug("Invalid existing rule IP")
					continue
				}

				if existingIP.Equal(desiredIP) {
					existingRuleIndex = ruleIndex
					break
				}
			}

			if existingRuleIndex != -1 {
				break
			}
		}

		if existingRuleIndex != -1 {
			// Rule already exists and we want it, so don't create it or delete it
			delete(undesiredRules, existingRuleIndex)
		} else {
			// Rule does not exist, so create it
			if err = clientAPI.ExportRuleCreate(ctx, policyName, desiredRule, config.NASType); err != nil {
				// Check if error is that the export policy rule already exist error
				if errors.IsAlreadyExistsError(err) {
					Logc(ctx).WithField("desiredRule", desiredRule).WithError(err).Debug(
						"Export policy rule already exists")
					continue
				}
				return err
			}
		}
	}

	Logc(ctx).WithField("undesiredRules", undesiredRules).Debug("Undesired export policy rules.")
	// Now that the desired rules exists, delete the undesired rules.
	// NOTE: With each rule deletion, ONTAP clears and rebuilds the export-policy cache. All I/O operations using the
	// outdated cache are paused until it is fully reconstructed. Therefore, an upper limit is set on how many rules
	// can be deleted during one reconciliation.
	maxDeleteCount := 10
	deleted := 0
	for ruleIndex := range undesiredRules {
		if deleted >= maxDeleteCount {
			Logc(ctx).WithField("maxDeleteCount", maxDeleteCount).Info("Maximum export rule delete count reached.")
			break
		}
		if err = clientAPI.ExportRuleDestroy(ctx, policyName, ruleIndex); err != nil {
			return err
		}
		deleted++
	}
	return nil
}

// getSVMState gets the backend SVM state and reason for offline if any.
// Input:
// protocol - to get the data LIFs of similar service from backend.
// pools - list of known pools to compare with the backend aggregate list and determine the change if any.
func getSVMState(
	ctx context.Context, client api.OntapAPI, protocol string, pools []string, configAggrs ...string,
) (string, *roaring.Bitmap) {
	changeMap := roaring.New()
	svmState, err := client.GetSVMState(ctx)
	if err != nil {
		// Could not get the SVM info or SVM is unreachable. Just log it.
		// Set state offline and reason as unreachable.
		Logc(ctx).WithField("error", err).Debug("Error getting SVM information.")
		return StateReasonSVMUnreachable, changeMap
	}

	if svmState != models.SvmStateRunning {
		return StateReasonSVMStopped, changeMap
	}

	// Get Aggregates list and verify if there is any change.
	if !client.IsSANOptimized() && client.IsDisaggregated() {
		Logc(ctx).Debug("Disaggregated system detected, skipping aggregate checks.")
	} else {
		aggrList, err := client.GetSVMAggregateNames(ctx)
		if err != nil {
			Logc(ctx).WithField("error", err).Debug("Error getting the physical pools from backend.")
		} else {
			if len(aggrList) == 0 {
				changeMap.Add(storage.BackendStatePoolsChange)
				return StateReasonNoAggregates, changeMap
			}
			sort.Strings(aggrList)
			sort.Strings(pools)
			if !cmp.Equal(pools, aggrList) {
				changeMap.Add(storage.BackendStatePoolsChange)
				// For the case where config.Aggregate is "", but due to configAggrs being a variadic parameter,
				// it will be passed as []string{""}.
				if strings.Join(configAggrs, "") != "" {
					if containsAll, _ := collection.ContainsElements(aggrList, configAggrs); !containsAll {
						return StateReasonMissingAggregate, changeMap
					}
				}
			}
		}
	}

	upDataLIFs := make([]string, 0)
	if protocol == sa.FCP {
		// Check dataLIF for FC protocol
		upDataLIFs, err = client.NetFcpInterfaceGetDataLIFs(ctx, protocol)
	} else {
		// Check dataLIF for iSCSI and NVMe protocols
		upDataLIFs, err = client.NetInterfaceGetDataLIFs(ctx, protocol)
	}

	if err != nil || len(upDataLIFs) == 0 {
		if err != nil {
			// Log error and keep going.
			fields := LogFields{"error": err, "protocol": protocol}
			Logc(ctx).WithFields(fields).Warn("Error getting list of data LIFs from backend.")
		}
		// No data LIFs with state 'up' found.
		return StateReasonDataLIFsDown, changeMap
	}

	// Get ONTAP version
	ontapVerCached, err := client.APIVersion(ctx, true)
	if err != nil {
		// Could not get the ONTAP version. Just log it.
		Logc(ctx).WithError(err).Debug("Error getting cached ONTAP version.")
		return "", changeMap
	}

	ontapVerCurrent, err := client.APIVersion(ctx, false)
	if err != nil {
		// Could not get the ONTAP version. Just log it.
		Logc(ctx).WithError(err).Debug("Error getting ONTAP version.")
		return "", changeMap
	}

	var parsedOntapVerCurrent, parsedOntapVerCached *version.Version

	switch client.(type) {
	// versioning is different for ZAPI, for ex: 1.251 which is equivalent to 9.15.1
	case api.OntapAPIZAPI:
		parsedOntapVerCached, err = version.ParseMajorMinorVersion(ontapVerCached)
		if err != nil {
			Logc(ctx).WithField("error", err).Debug("Error parsing cached ONTAP version.")
			return "", changeMap
		}
		parsedOntapVerCurrent, err = version.ParseMajorMinorVersion(ontapVerCurrent)
		if err != nil {
			Logc(ctx).WithField("error", err).Debug("Error parsing ONTAP version.")
			return "", changeMap
		}

	default:
		parsedOntapVerCached, err = version.ParseSemantic(ontapVerCached)
		if err != nil {
			Logc(ctx).WithField("error", err).Debug("Error parsing cached ONTAP version.")
			return "", changeMap
		}
		parsedOntapVerCurrent, err = version.ParseSemantic(ontapVerCurrent)
		if err != nil {
			Logc(ctx).WithField("error", err).Debug("Error parsing ONTAP version.")
			return "", changeMap
		}
	}

	// Comparing the retrieved ONTAP version with the cached version.
	if parsedOntapVerCurrent.GreaterThan(parsedOntapVerCached) || parsedOntapVerCurrent.LessThan(parsedOntapVerCached) {
		changeMap.Add(storage.BackendStateAPIVersionChange)
	}

	return "", changeMap
}

// resizeValidation performs needed validation checks prior to the resize operation.
func resizeValidation(
	ctx context.Context,
	volConfig *storage.VolumeConfig,
	requestedSizeBytes uint64,
	volumeExists func(context.Context, string) (bool, error),
	volumeSize func(context.Context, string) (uint64, error),
	volumeInfo func(context.Context, string) (*api.Volume, error),
) (uint64, error) {
	name := volConfig.InternalName

	// Ensure the volume exists
	volExists, err := volumeExists(ctx, name)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Error checking for existing volume.")
		return 0, fmt.Errorf("error occurred checking for existing volume")
	}
	if !volExists {
		return 0, fmt.Errorf("volume %s does not exist", name)
	}

	// Lookup the volume's current size on the storage system
	volSize, err := volumeSize(ctx, name)
	if err != nil {
		Logc(ctx).WithField("error", err).Errorf("Error checking volume size.")
		return 0, fmt.Errorf("error occurred when checking volume size")
	}
	volSizeBytes := uint64(volSize)

	// Determine original volume size in bytes
	volConfigSize, err := capacity.ToBytes(volConfig.Size)
	if err != nil {
		return 0, fmt.Errorf("could not convert volume size %s: %v", volConfig.Size, err)
	}
	volConfigSizeBytes, err := strconv.ParseUint(volConfigSize, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%v is an invalid volume size: %v", volConfig.Size, err)
	}

	// Ensure the requested size is greater than the original size that we stored in the vol config
	if requestedSizeBytes < volConfigSizeBytes {
		return 0, errors.UnsupportedCapacityRangeError(fmt.Errorf("requested size %d is less than previous volume size %d",
			requestedSizeBytes, volConfigSizeBytes))
	}

	if requestedSizeBytes == volConfigSizeBytes {
		Logc(ctx).Debugf("Requested volume size %s is the same as current volume size %s.", requestedSizeBytes,
			volSizeBytes)
		// nothing to do
		return 0, nil
	}

	snapshotReserveInt, err := getSnapshotReserveFromOntap(ctx, name, volumeInfo)
	if err != nil {
		Logc(ctx).WithField("name", name).Errorf("Could not get the snapshot reserve percentage for volume")
	}

	// Ensure the final effective volume size is larger than the current volume size
	newFlexvolSize := drivers.CalculateVolumeSizeBytes(ctx, name, requestedSizeBytes, snapshotReserveInt)
	if newFlexvolSize < volSizeBytes {
		return 0, errors.UnsupportedCapacityRangeError(fmt.Errorf("effective volume size %d including any "+
			"snapshot reserve is less than the existing volume size %d", newFlexvolSize, volSizeBytes))
	}

	return newFlexvolSize, nil
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
		igroupMutex.Lock(igroup)
		defer igroupMutex.Unlock(igroup)
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

func GetFCPTargetInfo(
	ctx context.Context, clientAPI api.OntapAPI, config *drivers.OntapStorageDriverConfig,
) (FCPNodeName string, FCPInterfaces []string, returnError error) {
	// Get the SVM FCP WWPN
	FCPNodeName, err := clientAPI.FcpNodeGetNameRequest(ctx)
	if err != nil {
		returnError = fmt.Errorf("could not get SVM FCP node name: %v", err)
		return
	}

	// Get the SVM FCP interface with enabled WWPNs
	FCPInterfaces, err = clientAPI.FcpInterfaceGet(ctx, config.SVM)
	if err != nil {
		returnError = fmt.Errorf("could not get SVM FCP node name: %v", err)
		return
	}
	// Get the WWPN
	if FCPInterfaces == nil {
		returnError = fmt.Errorf("SVM %s has no active FCP interfaces", config.SVM)
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

// getNodeSpecificFCPIgroupName generates a distinct igroup name for node name.
// Igroup names may collide if node names are over 59 characters.
func getNodeSpecificFCPIgroupName(nodeName, tridentUUID string) string {
	igroupName := fmt.Sprintf("%s-fcp-%s", nodeName, tridentUUID)

	if len(igroupName) > MaximumIgroupNameLength {
		// If the new igroup name is over the igroup character limit, it means the host name is too long.
		igroupPrefixLength := MaximumIgroupNameLength - len(tridentUUID) - 5
		igroupName = fmt.Sprintf("%s-fcp-%s", nodeName[:igroupPrefixLength], tridentUUID)
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
	publishInfo *tridentmodels.VolumePublishInfo, lunPath, igroupName, nodeName string,
) error {
	lunMutex.Lock(lunPath)
	defer lunMutex.Unlock(lunPath)
	igroupMutex.Lock(igroupName)
	defer igroupMutex.Unlock(igroupName)
	fields := LogFields{
		"Method":      "PublishLUN",
		"Type":        "ontap_common",
		"lunPath":     lunPath,
		"igroup":      igroupName,
		"nodeName":    nodeName,
		"publishInfo": publishInfo,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> PublishLUN")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< PublishLUN")

	var iqn string
	var err error

	if config.SANType == sa.ISCSI {
		if publishInfo.Localhost {

			// Lookup local host IQNs
			iqns, err := iscsi.GetInitiatorIqns(ctx)
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

	// Get the format options
	// An example of how formatOption may look like:
	// "-E stride=256,stripe_width=16 -F -b 2435965"
	formatOptions, err := clientAPI.LunGetAttribute(ctx, lunPath, "formatOptions")
	if err != nil {
		Logc(ctx).Warnf("Failed to get format options for LUN: %v", err)
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
		var targetNode *tridentmodels.Node
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

	if config.SANType == sa.ISCSI {
		if iqn != "" {
			// Add IQN to igroup
			err = clientAPI.EnsureIgroupAdded(ctx, igroupName, iqn)
			if err != nil {
				return fmt.Errorf("error adding IQN %v to igroup %v: %v", iqn, igroupName, err)
			}
		}
	} else if config.SANType == sa.FCP {
		// Add wwpns to igroup

		var wwpn string
		// Get the WWPNs and WWNNs from the host and propagate only the ones which are mapped to SVM
		for initiatorPortName, targetPortNames := range publishInfo.HostWWPNMap {
			// Format the WWNNs to match the SVM WWNNs
			for _, targetPortName := range targetPortNames {
				portNameFormatted := fcp.ConvertStrToWWNFormat(strings.TrimPrefix(targetPortName, "0x"))
				// Add initiator port name to igroup, if the target port name is mapped to SVM
				if nodeName == portNameFormatted {
					portName := strings.TrimPrefix(initiatorPortName, "0x")
					wwpn = fcp.ConvertStrToWWNFormat(portName)

					err = clientAPI.EnsureIgroupAdded(ctx, igroupName, wwpn)
					if err != nil {
						return fmt.Errorf("error adding WWPN %v to igroup %v: %v", portName, igroupName, err)
					}
				}
			}
		}

		if wwpn == "" {
			err = fmt.Errorf("no matching WWPN found for node %v", nodeName)
			Logc(ctx).Error(err)
			return err
		}
	}

	// Map LUN (it may already be mapped)
	lunID, err := clientAPI.EnsureLunMapped(ctx, igroupName, lunPath)
	if err != nil {
		return err
	}

	var filteredIPs []string
	if config.SANType == sa.ISCSI {
		filteredIPs, err = getISCSIDataLIFsForReportingNodes(ctx, clientAPI, ips, lunPath, igroupName,
			publishInfo.Unmanaged)
		if err != nil {
			return err
		}

		if len(filteredIPs) == 0 {
			Logc(ctx).Warn("Unable to find reporting ONTAP nodes for discovered dataLIFs.")
			filteredIPs = ips
		}
	}

	// xfs volumes are always mounted with '-o nouuid' to allow clones to be mounted to the same node as the source
	if fstype == filesystem.Xfs {
		publishInfo.MountOptions = drivers.EnsureMountOption(publishInfo.MountOptions, drivers.MountOptionNoUUID)
	}

	if config.SANType == sa.ISCSI {
		// Add fields needed by Attach
		publishInfo.IscsiLunNumber = int32(lunID)
		publishInfo.IscsiLunSerial = serial
		publishInfo.IscsiTargetPortal = filteredIPs[0]
		publishInfo.IscsiPortals = filteredIPs[1:]
		publishInfo.IscsiTargetIQN = nodeName
		publishInfo.SANType = config.SANType

		if igroupName != "" {
			addUniqueIscsiIGroupName(publishInfo, igroupName)
		}

		publishInfo.FilesystemType = fstype
		publishInfo.FormatOptions = formatOptions
		publishInfo.UseCHAP = config.UseCHAP

		if publishInfo.UseCHAP {
			publishInfo.IscsiUsername = config.ChapUsername
			publishInfo.IscsiInitiatorSecret = config.ChapInitiatorSecret
			publishInfo.IscsiTargetUsername = config.ChapTargetUsername
			publishInfo.IscsiTargetSecret = config.ChapTargetInitiatorSecret
			publishInfo.IscsiInterface = "default"
		}
		publishInfo.SharedTarget = true
	} else if config.SANType == sa.FCP {
		// Add fields needed by Attach
		publishInfo.FCPLunNumber = int32(lunID)
		publishInfo.FCPLunSerial = serial
		publishInfo.FCTargetWWNN = nodeName
		publishInfo.SANType = config.SANType

		if igroupName != "" {
			addUniqueFCPIGroupName(publishInfo, igroupName)
		}

		publishInfo.FilesystemType = fstype
		publishInfo.FormatOptions = formatOptions
		publishInfo.SharedTarget = true
	}

	return nil
}

// addUniqueIscsiIGroupName added iscsiIgroup name in the IscsiIgroup name string if it is not present.
func addUniqueIscsiIGroupName(publishInfo *tridentmodels.VolumePublishInfo, igroupName string) {
	if publishInfo.IscsiIgroup == "" {
		publishInfo.IscsiIgroup = igroupName
	} else {
		// Validate the iscsiGroupName present in the volume publish info. If not present, add in a string.
		if !strings.Contains(publishInfo.IscsiIgroup, igroupName) {
			publishInfo.IscsiIgroup += "," + igroupName
		}
	}
}

// addUniqueFCPGroupName added FCPIgroup name in the FCPIgroup name string if it is not present.
func addUniqueFCPIGroupName(publishInfo *tridentmodels.VolumePublishInfo, igroupName string) {
	if publishInfo.FCPIgroup == "" {
		publishInfo.FCPIgroup = igroupName
	} else {
		// Validate the FCPGroupName present in the volume publish info. If not present, add in a string.
		if !strings.Contains(publishInfo.FCPIgroup, igroupName) {
			publishInfo.FCPIgroup += "," + igroupName
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

// removeIgroupFromFCPIgroupList removes FCPIgroup name in the FCPIgroup list
func removeIgroupFromFCPIgroupList(fcpIgroupList, igroup string) string {
	if fcpIgroupList != "" {
		newIgroupList := make([]string, 0)
		igroups := strings.Split(fcpIgroupList, ",")

		for _, value := range igroups {
			if value != igroup {
				newIgroupList = append(newIgroupList, value)
			}
		}
		return strings.Join(newIgroupList, ",")
	}

	return fcpIgroupList
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
		return nil, errors.New("no reporting nodes found")
	}

	reportedDataLIFs, err := clientAPI.GetSLMDataLifs(ctx, ips, reportingNodes)
	if err != nil {
		return nil, err
	} else if !unmanagedImport && len(reportedDataLIFs) < 1 {
		return nil, errors.New("no reporting data LIFs found")
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
		return nil, errors.New("default initiator's auth type is unsupported")
	}

	// make sure access is allowed
	if isDefaultAuthTypeDeny {
		return nil, errors.New("default initiator's auth type is deny")
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
			return nil, errors.New("error checking default initiator's credentials")
		}

		if config.ChapUsername != defaultAuth.ChapUser ||
			config.ChapTargetUsername != defaultAuth.ChapOutboundUser {
			return nil, errors.New("provided CHAP usernames do not match default initiator's usernames")
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
		err := ensureIGroupExists(ctx, clientAPI, config.IgroupName, config.SANType)
		if err != nil {
			return err
		}
	}

	if config.SANType == sa.ISCSI {
		getDefaultAuthResponse, err := clientAPI.IscsiInitiatorGetDefaultAuth(ctx)
		Logc(ctx).WithFields(LogFields{
			"getDefaultAuthResponse": getDefaultAuthResponse,
		}).WithError(err).Debug("IscsiInitiatorGetDefaultAuth result")
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
						"will not enable CHAP for SVM %v; %v existing LUNs would lose access",
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
				return errors.New("default initiator's auth type is not 'none'")
			}
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

func ensureIGroupExists(ctx context.Context, clientAPI api.OntapAPI, igroupName, sanType string) error {
	err := clientAPI.IgroupCreate(ctx, igroupName, sanType, "linux")
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

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
	}

	// Splitting config.ManagementLIF with colon allows to provide managementLIF value as address:port format
	mgmtLIF := ""
	if network.IPv6Check(config.ManagementLIF) {
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
// file, this method attempts to derive the one to use.
func InitializeOntapAPI(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (api.OntapAPI, error) {
	var ontapAPI api.OntapAPI
	var err error

	useRESTValue := "<nil>"
	if config.UseREST != nil {
		useRESTValue = strconv.FormatBool(*config.UseREST)
	}
	fields := LogFields{
		"Method":  "InitializeOntapAPI",
		"Type":    "ontap_common",
		"useREST": useRESTValue,
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

	// Initially, the 'ontapAPI' variable is set with a REST client.
	// Based on user-configured options, we either keep the REST client or override it with a ZAPI client.
	ontapAPI, err = api.NewRestClientFromOntapConfig(ctx, config)
	if err != nil {
		// The creation of a REST client may fail due to various reasons.
		// One of the primary reasons could be the lack of authorization for the REST client.
		// In such cases, we attempt to fall back to ZAPI.

		msg := "Error creating ONTAP REST API client for initial call. Falling back to ZAPI."
		Logc(ctx).WithError(err).Error(msg)

		// If the user has set the useREST flag to true, return an error.
		if config.UseREST != nil && *config.UseREST == true {
			Logc(ctx).Error("useREST is set to true. Returning error, instead of falling back to ZAPI.")
			return nil, fmt.Errorf("error creating ONTAP REST API client: %v", err)
		}

		ontapAPI, err = api.NewZAPIClientFromOntapConfig(ctx, config, numRecords)
		if err != nil {
			return nil, fmt.Errorf("error creating ONTAP API client: %v", err)
		}

		Logc(ctx).WithField("Backend", config.BackendName).Info("Using ZAPI client")
		Logc(ctx).WithField("SVM", ontapAPI.SVMName()).Debug("Using SVM.")
		return ontapAPI, nil
	}

	ontapVer, err := ontapAPI.APIVersion(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("error getting ONTAP version: %v", err)
	}
	Logc(ctx).WithField("ontapVersion", ontapVer).Debug("ONTAP version.")

	// Is the ONTAP version greater than or equal to "9.12.1"?
	IsRESTSupported, err := api.IsRESTSupported(ontapVer)
	if err != nil {
		return nil, err
	}

	// Is the ONTAP version greater than or equal to "9.15.1"?
	IsRESTSupportedDefault, err := api.IsRESTSupportedDefault(ontapVer)
	if err != nil {
		return nil, err
	}

	// Is the ONTAP version lesser than or equal to "9.99.x"?
	IsZAPISupported, err := api.IsZAPISupported(ontapVer)
	if err != nil {
		return nil, err
	}

	/*
		The following if-else code block is responsible for initializing the ONTAP API client
			based on the ONTAP version and user configuration (numbering here is analogous to the if-else block below):
		1. If the ONTAP version is below "9.12.1":
				A. REST calls are not supported by Trident. In this case, if the user has set the useREST flag to true,
					an error is returned.
				B. Otherwise, a ZAPI client is created by default, overriding the `ontapAPI` var,
					thereby discarding the previously created REST client above.
		2. If the ONTAP version lies between "9.12.1" and "9.15.1" (this section of code is mainly for backward compatibility):
				A. Existing or new users, who haven't set the useREST flag or set it to false, will use the ZAPI client by default,
					overriding the `ontapAPI` var, thereby discarding the previously created REST client above.
				B. If the user has set the useREST flag to true, the REST client created earlier is used.
		3. If the ONTAP version lies between "9.15.1" and "9.99.x":
				A. Both REST and ZAPI are supported. If the user has not set the useREST flag or set it to true,
					the REST client created earlier is used.
				B. If the user has set the useREST flag to false, a ZAPI client is created,
					overriding the `ontapAPI` var, thereby discarding the previously created REST client above.
		4. If the ONTAP version is greater than "9.99.x":
				A. ZAPI calls are not supported by ONTAP. In this case, if the user has set the useREST flag to false,
					an error is returned.
				B. Otherwise, the REST client created earlier is used by default.
	*/
	if !IsRESTSupported {
		if config.UseREST != nil && *config.UseREST == true {
			return nil, fmt.Errorf("ONTAP version is %s, trident does not support REST calls, please remove `useRest=true` from the backend config",
				ontapVer)
		} else {
			if ontapAPI, err = api.NewZAPIClientFromOntapConfig(ctx, config, numRecords); err != nil {
				return nil, fmt.Errorf("error creating ONTAP API client: %v", err)
			}
		}
	} else if !IsRESTSupportedDefault {
		if config.UseREST == nil || (config.UseREST != nil && *config.UseREST == false) {
			if ontapAPI, err = api.NewZAPIClientFromOntapConfig(ctx, config, numRecords); err != nil {
				return nil, fmt.Errorf("error creating ONTAP API client: %v", err)
			}
		}
	} else if IsZAPISupported {
		if config.UseREST != nil && *config.UseREST == false {
			ontapAPI, err = api.NewZAPIClientFromOntapConfig(ctx, config, numRecords)
			if err != nil {
				return nil, fmt.Errorf("error creating ONTAP API client: %v", err)
			}
		}
	} else {
		if config.UseREST != nil && *config.UseREST == false {
			return nil, fmt.Errorf("ONTAP version %s does not support ZAPI calls, please remove `useRest=false` from the backend config",
				ontapVer)
		}
	}

	fields = LogFields{
		"Backend": config.BackendName,
	}
	switch ontapAPI.(type) {
	case api.OntapAPIREST:
		Logc(ctx).WithFields(fields).Info("Using REST client")
	case api.OntapAPIZAPI:
		Logc(ctx).WithFields(fields).Info("Using ZAPI client")
	default:
		// We should never reach this point, but putting this here just in case.
		Logc(ctx).WithFields(fields).Warn("Unknown ONTAP API client type.")
	}

	Logc(ctx).WithField("SVM", ontapAPI.SVMName()).Debug("Using SVM.")
	return ontapAPI, nil
}

// ValidateSANDriver contains the validation logic shared between ontap-san and ontap-san-economy.
func ValidateSANDriver(
	ctx context.Context, config *drivers.OntapStorageDriverConfig, ips []string, iscsi iscsi.ISCSI,
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
		if config.SANType == sa.ISCSI {
			err := iscsi.EnsureSessionsWithPortalDiscovery(ctx, ips)
			if err != nil {
				return fmt.Errorf("error establishing iSCSI session: %v", err)
			}
		} else if config.SANType == sa.FCP {
			return errors.New("trident does not support FCP in Docker plugin mode")
		}
	case tridentconfig.ContextCSI:
		// ontap-san-* drivers should all support publish enforcement with CSI; if the igroup is set
		// in the backend config, log a warning because it will not be used.
		if config.IgroupName != "" {
			Logc(ctx).WithField("igroup", config.IgroupName).
				Warning("Specifying an igroup is no longer supported for SAN backends in a CSI environment.")
		}
	}

	if config.SANType == sa.FCP && config.UseCHAP {
		return errors.New("CHAP is not supported with FCP protocol")
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
		return errors.New("ONTAP NAS drivers do not support LUKS encrypted volumes")
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
		if network.IPv6Check(dataLIFs[0]) {
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
	if err := network.ValidateCIDRs(ctx, config.AutoExportCIDRs); err != nil {
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
		err = errors.New(
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
	DefaultCloneSplitDelay           = 10
	DefaultLuksEncryption            = "false"
	DefaultMirroring                 = "false"
	DefaultLimitAggregateUsage       = ""
	DefaultLimitVolumeSize           = ""
	DefaultLimitVolumePoolSize       = ""
	DefaultDenyNewVolumePools        = "false"
	DefaultTieringPolicy             = ""
	DefaultSkipRecoveryQueue         = "false"
	DefaultExt3FormatOptions         = ""
	DefaultExt4FormatOptions         = ""
	DefaultXfsFormatOptions          = ""
	DefaultASAEncryption             = "true"
	DefaultADAdminUser               = ""
)

// PopulateConfigurationDefaults fills in default values for configuration settings if not supplied in the config file
func PopulateConfigurationDefaults(ctx context.Context, config *drivers.OntapStorageDriverConfig) error {
	fields := LogFields{"Method": "PopulateConfigurationDefaults", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> PopulateConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< PopulateConfigurationDefaults")

	var err error

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	} else {
		_, err := capacity.ToBytes(config.Size)
		if err != nil {
			return fmt.Errorf("invalid config value for default volume size: %v", err)
		}
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

	// If snapshotDir is provided, ensure it is lower case
	snapDir := DefaultSnapshotDir
	if config.SnapshotDir != "" {
		if snapDir, err = convert.ToFormattedBool(config.SnapshotDir); err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir: %v.", config.SnapshotDir)
			return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
		}
	}
	config.SnapshotDir = snapDir

	if config.DenyNewVolumePools == "" {
		config.DenyNewVolumePools = DefaultDenyNewVolumePools
	} else {
		if _, err = strconv.ParseBool(config.DenyNewVolumePools); err != nil {
			return fmt.Errorf("invalid boolean value for denyNewVolumePools: %v", err)
		}
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

	if config.CloneSplitDelay == "" {
		config.CloneSplitDelay = strconv.FormatInt(DefaultCloneSplitDelay, 10)
	} else if v, err := strconv.ParseInt(config.CloneSplitDelay, 10, 0); err != nil || v <= 0 {
		return fmt.Errorf("invalid value for cloneSplitDelay: %v", config.CloneSplitDelay)
	}

	if config.FileSystemType == "" {
		config.FileSystemType = drivers.DefaultFileSystemType
	}

	if config.FormatOptions == "" {
		switch config.FileSystemType {
		case "ext3":
			config.FormatOptions = DefaultExt3FormatOptions
		case "ext4":
			config.FormatOptions = DefaultExt4FormatOptions
		case "xfs":
			config.FormatOptions = DefaultXfsFormatOptions
		}
	}

	if config.LUKSEncryption == "" {
		config.LUKSEncryption = DefaultLuksEncryption
	}

	if config.Mirroring == "" {
		config.Mirroring = DefaultMirroring
	}

	if config.TieringPolicy == "" {
		config.TieringPolicy = DefaultTieringPolicy
	}

	if config.SkipRecoveryQueue == "" {
		config.SkipRecoveryQueue = DefaultSkipRecoveryQueue
	}

	if len(config.AutoExportCIDRs) == 0 {
		config.AutoExportCIDRs = []string{"0.0.0.0/0", "::/0"}
	}

	if len(config.FlexGroupAggregateList) == 0 {
		config.FlexGroupAggregateList = []string{}
	}

	if config.SANType == "" {
		config.SANType = sa.ISCSI
	} else {
		config.SANType = strings.ToLower(config.SANType)
	}

	// If NASType is not provided in the backend config, default to NFS
	if config.NASType == "" {
		config.NASType = sa.NFS
	}

	if config.ADAdminUser == "" {
		config.ADAdminUser = DefaultADAdminUser
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

	if config.NameTemplate != "" {
		config.NameTemplate = ensureUniquenessInNameTemplate(config.NameTemplate)
	}

	logFields := LogFields{
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
		"CloneSplitDelay":        config.CloneSplitDelay,
		"FileSystemType":         config.FileSystemType,
		"Encryption":             config.Encryption,
		"LUKSEncryption":         config.LUKSEncryption,
		"Mirroring":              config.Mirroring,
		"LimitAggregateUsage":    config.LimitAggregateUsage,
		"LimitVolumeSize":        config.LimitVolumeSize,
		"LimitVolumePoolSize":    config.LimitVolumePoolSize,
		"DenyNewVolumePools":     config.DenyNewVolumePools,
		"Size":                   config.Size,
		"TieringPolicy":          config.TieringPolicy,
		"AutoExportPolicy":       config.AutoExportPolicy,
		"AutoExportCIDRs":        config.AutoExportCIDRs,
		"FlexgroupAggregateList": config.FlexGroupAggregateList,
		"ADAdminUser":            config.ADAdminUser,
		"NameTemplate":           config.NameTemplate,
	}

	if config.StoragePrefix != nil {
		logFields["StoragePrefix"] = *config.StoragePrefix
	}

	Logc(ctx).WithFields(logFields).Debugf("Configuration defaults")

	return nil
}

// PopulateASAConfigurationDefaults fills in default values for configuration settings if not supplied in the config
// file.  This function uses defaults appropriate for ONTAP's All-SAN Array personality.
func PopulateASAConfigurationDefaults(ctx context.Context, config *drivers.OntapStorageDriverConfig) error {
	fields := LogFields{"Method": "PopulateASAConfigurationDefaults", "Type": "ontap_common"}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> PopulateASAConfigurationDefaults")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< PopulateASAConfigurationDefaults")

	var err error

	// Ensure the default volume size is valid, using a "default default" of 1G if not set
	if config.Size == "" {
		config.Size = drivers.DefaultVolumeSize
	} else {
		if _, err = capacity.ToBytes(config.Size); err != nil {
			return fmt.Errorf("invalid value for default volume size: %v", err)
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

	if config.Encryption == "" {
		config.Encryption = DefaultASAEncryption
	}

	if config.SplitOnClone == "" {
		config.SplitOnClone = DefaultSplitOnClone
	} else {
		if _, err = strconv.ParseBool(config.SplitOnClone); err != nil {
			return fmt.Errorf("invalid boolean value for splitOnClone: %v", err)
		}
	}

	if config.CloneSplitDelay == "" {
		config.CloneSplitDelay = strconv.FormatInt(DefaultCloneSplitDelay, 10)
	} else if v, err := strconv.ParseInt(config.CloneSplitDelay, 10, 0); err != nil || v <= 0 {
		return fmt.Errorf("invalid value for cloneSplitDelay: %v", config.CloneSplitDelay)
	}

	if config.FileSystemType == "" {
		config.FileSystemType = drivers.DefaultFileSystemType
	}

	if config.FormatOptions == "" {
		switch config.FileSystemType {
		case "ext3":
			config.FormatOptions = DefaultExt3FormatOptions
		case "ext4":
			config.FormatOptions = DefaultExt4FormatOptions
		case "xfs":
			config.FormatOptions = DefaultXfsFormatOptions
		}
	}

	if config.LUKSEncryption == "" {
		config.LUKSEncryption = DefaultLuksEncryption
	}

	if config.Mirroring == "" {
		config.Mirroring = DefaultMirroring
	}

	if config.TieringPolicy == "" {
		config.TieringPolicy = DefaultTieringPolicy
	}

	if config.SkipRecoveryQueue == "" {
		config.SkipRecoveryQueue = DefaultSkipRecoveryQueue
	}

	if config.SANType == "" {
		config.SANType = sa.ISCSI
	} else {
		config.SANType = strings.ToLower(config.SANType)
	}

	if config.NameTemplate != "" {
		config.NameTemplate = ensureUniquenessInNameTemplate(config.NameTemplate)
	}

	Logc(ctx).WithFields(LogFields{
		"StoragePrefix":       *config.StoragePrefix,
		"SpaceAllocation":     config.SpaceAllocation,
		"SpaceReserve":        config.SpaceReserve,
		"SnapshotPolicy":      config.SnapshotPolicy,
		"SnapshotReserve":     config.SnapshotReserve,
		"SplitOnClone":        config.SplitOnClone,
		"CloneSplitDelay":     config.CloneSplitDelay,
		"FileSystemType":      config.FileSystemType,
		"Encryption":          config.Encryption,
		"LUKSEncryption":      config.LUKSEncryption,
		"Mirroring":           config.Mirroring,
		"LimitAggregateUsage": config.LimitAggregateUsage,
		"LimitVolumeSize":     config.LimitVolumeSize,
		"Size":                config.Size,
		"TieringPolicy":       config.TieringPolicy,
		"NameTemplate":        config.NameTemplate,
		"SkipRecoveryQueue":   config.SkipRecoveryQueue,
		"SANType":             config.SANType,
		"FormatOptions":       config.FormatOptions,
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
	if aggregate == managedStoragePoolName {
		Logc(ctx).Debug("Skipping aggregate limit checks for disaggregated storage")
		return nil // No-op for dummy storage pool aggregates
	}

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

func GetVolumeSize(sizeBytes uint64, poolDefaultSizeBytes string) uint64 {
	if sizeBytes == 0 {
		defaultSize, _ := capacity.ToBytes(poolDefaultSizeBytes)
		sizeBytes, _ = strconv.ParseUint(defaultSize, 10, 64)
	}
	if sizeBytes < MinimumVolumeSizeBytes {
		Log().Infof("Requested size %v is too small. Setting volume size to the minimum allowable %v.", sizeBytes,
			MinimumVolumeSizeBytes)
		sizeBytes = MinimumVolumeSizeBytes
	}

	return sizeBytes
}

// CheckVolumePoolSizeLimits checks if a volume pool size limit has been set.
func CheckVolumePoolSizeLimits(
	ctx context.Context, requestedSize uint64, config *drivers.OntapStorageDriverConfig,
) (bool, uint64, error) {
	// If the user specified a limit for volume pool size, parse and enforce it
	limitVolumePoolSize := config.LimitVolumePoolSize
	Logc(ctx).WithFields(LogFields{
		"limitVolumePoolSize": limitVolumePoolSize,
	}).Debugf("Limits")

	if limitVolumePoolSize == "" {
		Logc(ctx).Debugf("No limits specified, not limiting volume pool size")
		return false, 0, nil
	}

	var volumePoolSizeLimit uint64
	volumePoolSizeLimitStr, parseErr := capacity.ToBytes(limitVolumePoolSize)
	if parseErr != nil {
		return false, 0, fmt.Errorf("error parsing limitVolumePoolSize: %v", parseErr)
	}
	volumePoolSizeLimit, _ = strconv.ParseUint(volumePoolSizeLimitStr, 10, 64)

	Logc(ctx).WithFields(LogFields{
		"limitVolumePoolSize": limitVolumePoolSize,
		"volumePoolSizeLimit": volumePoolSizeLimit,
		"requestedSizeBytes":  requestedSize,
	}).Debugf("Comparing pool limits")

	// Check whether pool size limit would prevent *any* Flexvol from working
	if requestedSize > volumePoolSizeLimit {
		return true, volumePoolSizeLimit, errors.UnsupportedCapacityRangeError(fmt.Errorf(
			"requested size: %d > the pool size limit: %d", requestedSize, volumePoolSizeLimit))
	}

	return true, volumePoolSizeLimit, nil
}

func GetSnapshotReserve(snapshotPolicy, snapshotReserve string) (int, error) {
	if snapshotReserve != "" {
		// snapshotReserve defaults to "", so if it is explicitly set
		// (either in config or create options), honor the value.
		snapshotReserve, err := convert.ToPositiveInt(snapshotReserve)
		if err != nil {
			return api.NumericalValueNotSet, err
		}
		return snapshotReserve, nil
	} else {
		// If snapshotReserve isn't set, then look at snapshotPolicy.  If the policy is "none",
		// return 0.  Otherwise return -1, indicating that ONTAP should use its own default value.
		if snapshotPolicy == "none" || snapshotPolicy == "" {
			return 0, nil
		} else {
			snapshotReserve, err := convert.ToPositiveInt(DefaultSnapshotReserve)
			if err != nil {
				return api.NumericalValueNotSet, err
			}
			return snapshotReserve, nil
		}
	}
}

const MSecPerHour = 1000 * 60 * 60 // millis * seconds * minutes

// refreshDynamicTelemetry updates dynamic telemetry fields before EMS heartbeat transmission
func refreshDynamicTelemetry(ctx context.Context, driver StorageDriver) {
	// Get the current telemetry object from the driver
	telemetry := driver.GetTelemetry()
	if telemetry == nil {
		Logc(ctx).Debug("No telemetry object found for dynamic refresh.")
		return
	}

	// Update dynamic fields using registered updaters (from CSI helper)
	tridentconfig.UpdateDynamicTelemetry(ctx, &telemetry.Telemetry)

	Logc(ctx).WithFields(LogFields{
		"driver":                driver.Name(),
		"svm":                   telemetry.SVM,
		"platformUID":           telemetry.PlatformUID,
		"platformNodeCount":     telemetry.PlatformNodeCount,
		"platformVersion":       telemetry.PlatformVersion,
		"tridentProtectVersion": telemetry.TridentProtectVersion,
	}).Debug("Dynamic telemetry refresh completed.")
}

// EMSHeartbeat logs an ASUP message on a timer
// view them via filer::> event log show -severity NOTICE
func EMSHeartbeat(ctx context.Context, driver StorageDriver) {
	// Refresh dynamic telemetry fields before sending heartbeat
	refreshDynamicTelemetry(ctx, driver)

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

func SplitASAVolumeFromBusySnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, cloneSplitStart func(ctx context.Context, cloneName string) error,
) error {
	fields := LogFields{
		"Method":       "SplitASAVolumeFromBusySnapshot",
		"Type":         "ontap_common",
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> SplitASAVolumeFromBusySnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< SplitASAVolumeFromBusySnapshot")

	childVolumes, err := client.StorageUnitListBySnapshotParent(ctx, snapConfig.InternalName,
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
		}).Error("Could not begin splitting ASA clone from snapshot.")
		return fmt.Errorf("error splitting ASA clone: %v", err)
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName":     snapConfig.InternalName,
		"parentVolumeName": snapConfig.VolumeInternalName,
		"cloneVolumeName":  childVolumes[0],
	}).Info("Began splitting ASA clone from snapshot.")

	return nil
}

func SplitVolumeFromBusySnapshotWithDelay(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, cloneSplitStart func(ctx context.Context, cloneName string) error,
	cloneSplitTimers *sync.Map,
) {
	snapshotID := snapConfig.ID()
	cloneSplitDelay, expired := hasCloneSplitTimerExpired(ctx, snapConfig, config, cloneSplitTimers)
	if !expired {
		// Clone split timer has not expired, so do nothing.
		return
	}

	// The delay has expired, so start the split
	splitErr := SplitVolumeFromBusySnapshot(ctx, snapConfig, config, client, cloneSplitStart)
	if splitErr != nil {

		// The split start failed, so reset the timer so we start again after another brief delay.
		cloneSplitTimers.Store(snapshotID, time.Now())

		Logc(ctx).WithFields(LogFields{
			"snapshot":           snapshotID,
			"secondsBeforeSplit": fmt.Sprintf("%3.2f", cloneSplitDelay.Seconds()),
		}).Warning("Retried locked snapshot delete, clone split failed, restarted timer.")

	} else {

		// The split start succeeded, so add enough time to the timer that we don't try to start it again.
		cloneSplitTimers.Store(snapshotID, time.Now().Add(1*time.Hour))

		Logc(ctx).WithField("snapshot", snapshotID).Warning("Retried locked snapshot delete, clone split started.")
	}
}

// hasCloneSplitTimerExpired Checks whether clone split timer has expired. If yes, returns the clone split delay time.
func hasCloneSplitTimerExpired(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
	config *drivers.OntapStorageDriverConfig, cloneSplitTimers *sync.Map,
) (time.Duration, bool) {
	fields := LogFields{
		"Method":       "hasCloneSplitTimerExpired",
		"Type":         "ontap_common",
		"snapshotID":   snapConfig.ID(),
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}

	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> hasCloneSplitTimerExpired")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< hasCloneSplitTimerExpired")

	snapshotID := snapConfig.ID()

	// Find the configured value for cloneSplitDelay
	delay, err := strconv.ParseInt(config.CloneSplitDelay, 10, 0)
	if err != nil || delay < 0 {
		// Should not come here, but just in case.
		delay = DefaultCloneSplitDelay
	}
	cloneSplitDelay := time.Duration(delay) * time.Second

	// If this is the first delete, just log the time and return.
	firstDeleteTime, ok := cloneSplitTimers.Load(snapshotID)
	if !ok {
		cloneSplitTimers.Store(snapshotID, time.Now())

		Logc(ctx).WithFields(LogFields{
			"snapshot":           snapshotID,
			"secondsBeforeSplit": fmt.Sprintf("%3.2f", cloneSplitDelay.Seconds()),
		}).Warning("Initial locked snapshot delete, starting clone split timer.")

		return 0, false
	}

	t, ok := firstDeleteTime.(time.Time)
	if !ok {
		Logc(ctx).WithFields(LogFields{
			"snapshot":           snapshotID,
			"secondsBeforeSplit": fmt.Sprintf("%3.2f", cloneSplitDelay.Seconds()),
		}).Warning("Time type conversion failed, restarting clone split timer.")
		cloneSplitTimers.Store(snapshotID, time.Now())
		return 0, false
	}

	// This isn't the first delete, and the split is still running, so there is nothing to do.
	if time.Now().Sub(t) < 0 {

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapshotID,
		}).Warning("Retried locked snapshot delete, clone split still running.")

		return 0, false
	}

	// This isn't the first delete, and the delay has not expired, so there is nothing to do.
	if time.Now().Sub(t) < cloneSplitDelay {

		Logc(ctx).WithFields(LogFields{
			"snapshot":           snapshotID,
			"secondsBeforeSplit": fmt.Sprintf("%3.2f", time.Now().Sub(t).Seconds()),
		}).Warning("Retried locked snapshot delete, clone split timer not yet expired.")

		return 0, false
	}

	return cloneSplitDelay, true
}

func SplitASAVolumeFromBusySnapshotWithDelay(
	ctx context.Context, snapConfig *storage.SnapshotConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI, cloneSplitStart func(ctx context.Context, cloneName string) error,
	cloneSplitTimers *sync.Map,
) {
	fields := LogFields{
		"Method":       "SplitASAVolumeFromBusySnapshotWithDelay",
		"Type":         "ontap_common",
		"snapshotID":   snapConfig.ID(),
		"snapshotName": snapConfig.InternalName,
		"volumeName":   snapConfig.VolumeInternalName,
	}

	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> SplitASAVolumeFromBusySnapshotWithDelay")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< SplitASAVolumeFromBusySnapshotWithDelay")

	snapshotID := snapConfig.ID()
	cloneSplitDelay, expired := hasCloneSplitTimerExpired(ctx, snapConfig, config, cloneSplitTimers)
	if !expired {
		// Clone split timer has not expired, so do nothing.
		return
	}

	// The delay has expired, so start the split
	splitErr := SplitASAVolumeFromBusySnapshot(ctx, snapConfig, config, client, cloneSplitStart)
	if splitErr != nil {

		// The split start failed, so reset the timer so we start again after another brief delay.
		cloneSplitTimers.Store(snapshotID, time.Now())

		Logc(ctx).WithFields(LogFields{
			"snapshot":           snapshotID,
			"secondsBeforeSplit": fmt.Sprintf("%3.2f", cloneSplitDelay.Seconds()),
		}).Warning("Retried locked snapshot delete, clone split failed, restarted timer.")

	} else {

		// The split start succeeded, so add enough time to the timer that we don't try to start it again.
		cloneSplitTimers.Store(snapshotID, time.Now().Add(1*time.Hour))

		Logc(ctx).WithField("snapshot", snapshotID).Warning("Retried locked snapshot delete, clone split started.")
	}
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

	snapshotDir := false
	if volume.SnapshotDir != nil {
		snapshotDir = *volume.SnapshotDir
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
		SnapshotDir:     strconv.FormatBool(snapshotDir),
		UnixPermissions: volume.UnixPermissions,
		StorageClass:    "",
		AccessMode:      tridentconfig.ReadWriteMany,
		AccessInfo:      tridentmodels.VolumeAccessInfo{},
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
	config := d.GetOntapConfig()
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

// ensureUniquenessInNameTemplate ensure the volume name should present in the name template.
// Trident wants to provide unique custom volume name from template. The volume name generates with the random UUID.
// That is the reason Trident is making volume name mandatory in the name template.
func ensureUniquenessInNameTemplate(nameTemplate string) string {
	match := volumeNameRegex.MatchString(nameTemplate)

	if match || nameTemplate == "" {
		return nameTemplate
	} else {
		// Suffix Nametemplate with unique ID derived from volume name.
		return nameTemplate + "_{{slice .volume.Name 4 9}}"
	}
}

// validateFormatOptions validates the formatOptions provided by the user.
// Currently, it returns an error if the formatOption is empty.
func validateFormatOptions(formatOptions string) error {
	formatOptions = strings.TrimSpace(formatOptions)
	if formatOptions == "" {
		return errors.New("empty formatOptions is provided")
	}
	return nil
}

func InitializeStoragePoolsCommon(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	if d.GetAPI().IsDisaggregated() {
		// For Disaggregated systems, override media type to reflect all-flash nature
		poolAttributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		return InitializeManagedStoragePoolsCommon(ctx, d, poolAttributes, backendName)
	}
	return InitializeAggregatedStoragePoolsCommon(ctx, d, poolAttributes, backendName)
}

func InitializeAggregatedStoragePoolsCommon(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	config := d.GetOntapConfig()
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

		err = setStoragePoolAttributes(ctx, pool, config, poolAttributes, d.Name())
		if err != nil {
			return nil, nil, err
		}
		physicalPools[pool.Name()] = pool
	}

	virtualPools, err = initializeVirtualPools(ctx, d, poolAttributes, backendName, mediaOffers)

	return physicalPools, virtualPools, err
}

func setStoragePoolAttributes(
	ctx context.Context, pool storage.Pool, config *drivers.OntapStorageDriverConfig,
	poolAttributes map[string]sa.Offer, driverName string,
) error {
	for attrName, offer := range poolAttributes {
		pool.Attributes()[attrName] = offer
	}

	if config.Region != "" {
		pool.Attributes()[sa.Region] = sa.NewStringOffer(config.Region)
	}
	if config.Zone != "" {
		pool.Attributes()[sa.Zone] = sa.NewStringOffer(config.Zone)
	}
	var err error
	if config.SnapshotDir != "" {
		config.SnapshotDir, err = convert.ToFormattedBool(config.SnapshotDir)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir: %v.", config.SnapshotDir)
			return fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
		}
	}

	pool.Attributes()[sa.Labels] = sa.NewLabelOffer(config.Labels)
	pool.Attributes()[sa.NASType] = sa.NewStringOffer(config.NASType)
	pool.Attributes()[sa.SANType] = sa.NewStringOffer(config.SANType)

	pool.InternalAttributes()[Size] = config.Size
	pool.InternalAttributes()[NameTemplate] = config.NameTemplate
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
	pool.InternalAttributes()[SkipRecoveryQueue] = config.SkipRecoveryQueue
	pool.InternalAttributes()[QosPolicy] = config.QosPolicy
	pool.InternalAttributes()[AdaptiveQosPolicy] = config.AdaptiveQosPolicy
	pool.InternalAttributes()[ADAdminUser] = config.ADAdminUser

	pool.SetSupportedTopologies(config.SupportedTopologies)

	if driverName == tridentconfig.OntapSANStorageDriverName || driverName == tridentconfig.OntapSANEconomyStorageDriverName {
		pool.InternalAttributes()[SpaceAllocation] = config.SpaceAllocation
		pool.InternalAttributes()[FileSystemType] = config.FileSystemType
		if config.FormatOptions != "" {
			if err = validateFormatOptions(config.FormatOptions); err != nil {
				return err
			}
		}
		pool.InternalAttributes()[FormatOptions] = strings.TrimSpace(config.FormatOptions)
	}
	return nil
}

// InitializeManagedStoragePoolsCommon initializes storage pools for systems where aggregate
// selection and placement is managed automatically by the storage system rather than explicitly
// by Trident. This includes disaggregated ONTAP systems and other unified storage
// architectures where Trident doesn't need to be aware of individual aggregates.
func InitializeManagedStoragePoolsCommon(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
) (map[string]storage.Pool, map[string]storage.Pool, error) {
	var err error
	config := d.GetOntapConfig()

	mediaOffers := make([]sa.Offer, 0)
	if mediaOffer, ok := poolAttributes[sa.Media]; ok {
		mediaOffers = append(mediaOffers, mediaOffer)
	}

	pool := storage.NewStoragePool(nil, managedStoragePoolName)

	// Update pool with attributes set by default for this backend
	// We do not set internal attributes with these values as this
	// merely means that pools supports these capabilities like
	// encryption, cloning, thick/thin provisioning
	for attrName, offer := range poolAttributes {
		pool.Attributes()[attrName] = offer
	}

	err = setStoragePoolAttributes(ctx, pool, config, poolAttributes, d.Name())
	if err != nil {
		return nil, nil, err
	}

	physicalPools := map[string]storage.Pool{pool.Name(): pool}

	virtualPools, err := initializeVirtualPools(ctx, d, poolAttributes, backendName, mediaOffers)

	return physicalPools, virtualPools, err
}

func initializeVirtualPools(
	ctx context.Context, d StorageDriver, poolAttributes map[string]sa.Offer, backendName string,
	mediaOffers []sa.Offer,
) (map[string]storage.Pool, error) {
	config := d.GetOntapConfig()
	virtualPools := make(map[string]storage.Pool)

	var err error

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

		nameTemplate := config.NameTemplate
		if vpool.NameTemplate != "" {
			nameTemplate = vpool.NameTemplate
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
			snapshotDir, err = convert.ToFormattedBool(vpool.SnapshotDir)
			if err != nil {
				Logc(ctx).WithError(err).Errorf("Invalid boolean value for vpool's snapshotDir: %v.", vpool.SnapshotDir)
				return nil, fmt.Errorf("invalid boolean value for snapshotDir: %v", err)
			}
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

		skipRecoveryQueue := config.SkipRecoveryQueue
		if vpool.SkipRecoveryQueue != "" {
			skipRecoveryQueue = vpool.SkipRecoveryQueue
		}

		qosPolicy := config.QosPolicy
		if vpool.QosPolicy != "" {
			qosPolicy = vpool.QosPolicy
		}

		adaptiveQosPolicy := config.AdaptiveQosPolicy
		if vpool.AdaptiveQosPolicy != "" {
			adaptiveQosPolicy = vpool.AdaptiveQosPolicy
		}

		adAdminUser := config.ADAdminUser
		if vpool.ADAdminUser != "" {
			adAdminUser = vpool.ADAdminUser
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
			sanType = strings.ToLower(vpool.SANType)
			if config.SANType != sanType {
				return nil, fmt.Errorf(
					"trident does not support mixing of %s and %s SAN types", sanType, config.SANType,
				)
			}
		}

		formatOptions := config.FormatOptions
		if vpool.FormatOptions != "" {
			if err = validateFormatOptions(vpool.FormatOptions); err != nil {
				return nil, fmt.Errorf("invalid formatOptions: %w, in pool: %v", err, pool.Name())
			}
			formatOptions = strings.TrimSpace(vpool.FormatOptions)
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
				return nil, fmt.Errorf("invalid boolean value for encryption: %v in virtual pool: %s", err, pool.Name())
			}
			pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(enableEncryption)
			pool.InternalAttributes()[Encryption] = encryption
		}

		pool.InternalAttributes()[Size] = size
		pool.InternalAttributes()[NameTemplate] = ensureUniquenessInNameTemplate(nameTemplate)
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
		pool.InternalAttributes()[SkipRecoveryQueue] = skipRecoveryQueue
		pool.InternalAttributes()[QosPolicy] = qosPolicy
		pool.InternalAttributes()[LUKSEncryption] = luksEncryption
		pool.InternalAttributes()[AdaptiveQosPolicy] = adaptiveQosPolicy
		pool.InternalAttributes()[ADAdminUser] = adAdminUser
		pool.SetSupportedTopologies(supportedTopologies)

		if d.Name() == tridentconfig.OntapSANStorageDriverName || d.Name() == tridentconfig.OntapSANEconomyStorageDriverName {
			pool.InternalAttributes()[SpaceAllocation] = spaceAllocation
			pool.InternalAttributes()[FileSystemType] = fileSystemType
			pool.InternalAttributes()[FormatOptions] = formatOptions
		}

		virtualPools[pool.Name()] = pool
	}

	return virtualPools, nil
}

// ValidateStoragePools checks that pool attribute values are valid for a traditional ONTAP storage cluster.
// Any fields not explicitly specified would have been set to default values during pool initialization.
func ValidateStoragePools(
	ctx context.Context, physicalPools, virtualPools map[string]storage.Pool, d StorageDriver, labelLimit int,
) error {
	config := d.GetOntapConfig()

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

		// Validate SnapshotDir
		if pool.InternalAttributes()[SnapshotDir] == "" {
			return fmt.Errorf("snapshotDir cannot be empty in pool %s", poolName)
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

		// Validate SkipRecoveryQueue
		if pool.InternalAttributes()[SkipRecoveryQueue] == "" {
			return fmt.Errorf("skipRecoveryQueue cannot by empty in pool %s", poolName)
		} else {
			if _, err = strconv.ParseBool(pool.InternalAttributes()[SkipRecoveryQueue]); err != nil {
				return fmt.Errorf("invalid value for skipRecoveryQueue in pool %s: %v", poolName, err)
			}
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
		if defaultSize, err := capacity.ToBytes(pool.InternalAttributes()[Size]); err != nil {
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

		// Validate name template
		if pool.InternalAttributes()[NameTemplate] != "" {
			if _, err = template.New(poolName).Parse(pool.InternalAttributes()[NameTemplate]); err != nil {
				return fmt.Errorf("invalid value for volume name template in pool %s; %v", pool.Name(), err)
			}
		}

		if pool.Attributes()[sa.Labels] != nil {
			// make an array of map of string to string
			labelOffer, ok := pool.Attributes()[sa.Labels].(sa.LabelOffer)
			if !ok {
				return fmt.Errorf("invalid value for labels in pool %s; %v", poolName, err)
			}

			labelOfferMap := labelOffer.Labels()
			if len(labelOfferMap) != 0 {
				for _, v := range labelOfferMap {
					if _, err = template.New(poolName).Parse(v); err != nil {
						return fmt.Errorf("invalid labels template in pool %s; %v", poolName, err)
					}
				}
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

// ValidateASAStoragePools checks that pool attribute values are valid for an All-SAN Array storage cluster.
// Any fields not explicitly specified would have been set to default values during pool initialization.
func ValidateASAStoragePools(
	ctx context.Context, physicalPools, virtualPools map[string]storage.Pool, d StorageDriver, labelLimit int,
) error {
	config := d.GetOntapConfig()

	if config.NASType != "" {
		return errors.New("nasType must not be set for a SAN backend")
	}

	if len(config.AutoExportCIDRs) > 0 {
		return errors.New("invalid value for autoExportCIDRs")
	}

	switch config.SANType {
	case sa.ISCSI:
		break
	case sa.NVMe:
		break
	case sa.FCP:
		break
	default:
		return errors.New("invalid value for sanType")
	}

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

		if pool.InternalAttributes()[SpaceReserve] != "none" {
			return errors.New("spaceReserve must be set to none")
		}

		if pool.InternalAttributes()[SnapshotReserve] != "" {
			return errors.New("snapshotReserve must not be set")
		}

		if pool.InternalAttributes()[SnapshotPolicy] != "none" {
			return errors.New("snapshotPolicy must be set to none")
		}

		if pool.InternalAttributes()[Encryption] != "" {
			if encryption, err := strconv.ParseBool(pool.InternalAttributes()[Encryption]); err != nil {
				return fmt.Errorf("invalid value for encryption in pool %s: %v", poolName, err)
			} else if !encryption {
				return errors.New("encryption must be set to true")
			}
		}

		if pool.InternalAttributes()[SnapshotDir] != "" {
			return errors.New("snapshotDir must not be set")
		}

		if pool.InternalAttributes()[SecurityStyle] != "" {
			return errors.New("invalid value for securityStyle")
		}

		if pool.InternalAttributes()[ExportPolicy] != "" {
			return errors.New("invalid value for exportPolicy")
		}

		if pool.InternalAttributes()[UnixPermissions] != "" {
			return errors.New("invalid value for unixPermissions")
		}

		if pool.InternalAttributes()[TieringPolicy] != "" {
			return errors.New("tieringPolicy must not be set")
		}

		// Validate QoS policy or adaptive QoS policy
		if pool.InternalAttributes()[QosPolicy] != "" || pool.InternalAttributes()[AdaptiveQosPolicy] != "" {
			if _, err := api.NewQosPolicyGroup(
				pool.InternalAttributes()[QosPolicy], pool.InternalAttributes()[AdaptiveQosPolicy],
			); err != nil {
				return err
			}
		}

		// Validate media type
		if pool.InternalAttributes()[Media] != "" {
			for _, mediaType := range strings.Split(pool.InternalAttributes()[Media], ",") {
				if mediaType != sa.SSD {
					Logc(ctx).Errorf("invalid media type %s in pool %s", mediaType, pool.Name())
				}
			}
		}

		// Validate default size
		if defaultSize, err := capacity.ToBytes(pool.InternalAttributes()[Size]); err != nil {
			return fmt.Errorf("invalid value for default volume size in pool %s: %v", poolName, err)
		} else {
			sizeBytes, _ := strconv.ParseUint(defaultSize, 10, 64)
			if sizeBytes < MinimumVolumeSizeBytes {
				return fmt.Errorf("invalid value for size in pool %s. Requested volume size ("+
					"%d bytes) is too small; the minimum volume size is %d bytes", poolName, sizeBytes,
					MinimumVolumeSizeBytes)
			}
		}

		// Validate name template
		if pool.InternalAttributes()[NameTemplate] != "" {
			if _, err := template.New(poolName).Parse(pool.InternalAttributes()[NameTemplate]); err != nil {
				return fmt.Errorf("invalid value for volume name template in pool %s; %v", pool.Name(), err)
			}
		}

		if pool.Attributes()[sa.Labels] != nil {
			// make an array of map of string to string
			labelOffer, ok := pool.Attributes()[sa.Labels].(sa.LabelOffer)
			if !ok {
				return fmt.Errorf("invalid value for labels in pool %s", poolName)
			}

			labelOfferMap := labelOffer.Labels()
			if len(labelOfferMap) != 0 {
				for _, v := range labelOfferMap {
					if _, err := template.New(poolName).Parse(v); err != nil {
						return fmt.Errorf("invalid labels template in pool %s; %v", poolName, err)
					}
				}
			}

			if _, err := pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit); err != nil {
				return fmt.Errorf("invalid value for label in pool %s: %v", poolName, err)
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

		// Validate LUKS configuration, only boolean value and only supported by non-ASA SAN backends
		if _, ok := pool.InternalAttributes()[LUKSEncryption]; ok {
			if isLuks, err := strconv.ParseBool(pool.InternalAttributes()[LUKSEncryption]); err != nil {
				return fmt.Errorf("could not parse LUKSEncryption from volume config into a boolean, got %v",
					pool.InternalAttributes()[LUKSEncryption])
			} else if isLuks {
				return fmt.Errorf("LUKS encrypted volumes are not supported by All-SAN Array backends")
			}
		}

		// Validate SpaceAllocation
		if pool.InternalAttributes()[SpaceAllocation] == "" {
			return fmt.Errorf("spaceAllocation cannot by empty in pool %s", poolName)
		} else {
			spaceAllocation, err := strconv.ParseBool(pool.InternalAttributes()[SpaceAllocation])
			if !spaceAllocation || err != nil {
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

		// Validate formatOptions
		if pool.InternalAttributes()[FormatOptions] != "" {
			if err := validateFormatOptions(pool.InternalAttributes()[FormatOptions]); err != nil {
				return fmt.Errorf("invalid value for formatOptions in pool %s: %w", poolName, err)
			}
		}

		// Validate skipRecoveryQueue
		if pool.InternalAttributes()[SkipRecoveryQueue] != "" {
			skipRecoveryQueueValue, err := strconv.ParseBool(pool.InternalAttributes()[SkipRecoveryQueue])
			if err != nil {
				return fmt.Errorf("invalid value for skipRecoveryQueue in pool %s: %w", poolName, err)
			}

			if skipRecoveryQueueValue {
				// skipRecoveryQueue is not supported in ONTAP ASAr2, so we log a warning and continue.
				Logc(ctx).WithField("skipRecoveryQueue", skipRecoveryQueueValue).Warn(
					"skipRecoveryQueue is not supported. It will be ignored during delete operation.")
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

	// If snapshotDir is provided, ensure it is lower case
	if volConfig.SnapshotDir != "" {
		snapshotDirFormatted, err := convert.ToFormattedBool(volConfig.SnapshotDir)
		if err != nil {
			Logc(ctx).WithError(err).Errorf(
				"Invalid boolean value for volume '%v' snapshotDir: %v.", volConfig.Name, volConfig.SnapshotDir)
		}
		opts["snapshotDir"] = snapshotDirFormatted
	}

	// If skipRecoveryQueue is provided, ensure it is lower case
	if volConfig.SkipRecoveryQueue != "" {
		skipRecoveryQueueFormatted, err := convert.ToFormattedBool(volConfig.SkipRecoveryQueue)
		if err != nil {
			Logc(ctx).WithError(err).Errorf(
				"Invalid boolean value for volume '%v' skipRecoveryQueue: %v.",
				volConfig.Name, volConfig.SkipRecoveryQueue,
			)
		}
		opts["skipRecoveryQueue"] = skipRecoveryQueueFormatted
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
		err := errors.New("backend has no physical pools that can satisfy request")
		return nil, drivers.NewBackendIneligibleError(volConfig.InternalName, []error{err}, []string{})
	}

	// Shuffle physical pools
	rand.Shuffle(len(candidatePools), func(i, j int) {
		candidatePools[i], candidatePools[j] = candidatePools[j], candidatePools[i]
	})

	return candidatePools, nil
}

func getInternalVolumeNameCommon(
	ctx context.Context, config *drivers.OntapStorageDriverConfig, volConfig *storage.VolumeConfig,
	pool storage.Pool,
) string {
	if tridentconfig.UsingPassthroughStore {
		// With a passthrough store, the name mapping must remain reversible
		return *config.StoragePrefix + volConfig.Name
	}

	if pool != nil && pool.InternalAttributes()[NameTemplate] != "" {
		internal, err := GetVolumeNameFromTemplate(ctx, config, volConfig, pool)
		if err == nil {
			return internal
		}
	}
	// With an external store, any transformation of the name is fine
	internal := drivers.GetCommonInternalVolumeName(config.CommonStorageDriverConfig, volConfig.Name)
	internal = strings.Replace(internal, "-", "_", -1) // ONTAP disallows hyphens
	internal = strings.Replace(internal, ".", "_", -1) // ONTAP disallows periods
	for strings.Contains(internal, "__") {
		internal = strings.Replace(internal, "__", "_", -1)
	}
	return internal
}

// GetVolumeNameFromTemplate function generates a volume name from a template. The function parses the name template
// from the pool's internal attributes and creates a map of template data. If the template execution is successful,
// it processes the resulting string to replace any non-alphanumeric characters with underscores,
// remove multiple underscores, and remove any underscores at the beginning or end of the string.
// The function returns the processed string as the generated volume name.
func GetVolumeNameFromTemplate(
	ctx context.Context, config *drivers.OntapStorageDriverConfig, volConfig *storage.VolumeConfig,
	pool storage.Pool,
) (string, error) {
	t, err := template.New("templatizedVolumeName").Parse(pool.InternalAttributes()[NameTemplate])
	if err != nil {
		Logc(ctx).WithError(err).Error("invalid name template in pool %s: %v", pool.Name(), err)
		return "", errors.New("invalid name template in pool")
	} else {
		templateData := make(map[string]interface{})
		templateData["volume"] = volConfig
		// Redacted the sensitive data from the backend config
		templateData["config"] = getExternalConfig(ctx, *config).(drivers.OntapStorageDriverConfig)
		templateData["labels"] = pool.GetLabelMapFromTemplate(ctx, templateData)

		var tBuffer bytes.Buffer
		if err := t.Execute(&tBuffer, templateData); err != nil {
			Logc(ctx).WithError(err).Error("Volume name template execution failed, using default name rules.")
			return "", errors.New("Volume name template execution failed")
		} else {
			internal := tBuffer.String()
			internal = volumeCharRegex.ReplaceAllString(internal, "_")

			volumeNameRegex := regexp.MustCompile("__*")
			internal = volumeNameRegex.ReplaceAllString(internal, "_") // Remove any double underscores
			internal = strings.TrimPrefix(internal, "_")               // Remove any underscores at the beginning
			internal = strings.TrimSuffix(internal, "_")               // Remove any underscores at the end

			// ONTAP flexvol volume name must begin with an alphabet or an underscore. If the name generated from the
			// user-defined name template is start other than an alphabet or an underscore Trident will fall back to the
			// old naming method.
			matched := volumeNameStartWithRegex.MatchString(internal)

			if !matched {
				err = fmt.Errorf("invalid volume name: %s is generated from template, it must begin with "+
					"letter/underscore", internal)
				Logc(ctx).Error(err)
				return "", err
			}

			return internal, nil
		}
	}
}

func createPrepareCommon(ctx context.Context, d storage.Driver, volConfig *storage.VolumeConfig, pool storage.Pool) {
	volConfig.InternalName = d.GetInternalVolumeName(ctx, volConfig, pool)
}

func getExternalConfig(ctx context.Context, config drivers.OntapStorageDriverConfig) interface{} {
	// Clone the config so we don't risk altering the original
	var cloneConfig drivers.OntapStorageDriverConfig
	drivers.Clone(ctx, config, &cloneConfig)

	drivers.SanitizeCommonStorageDriverConfig(cloneConfig.CommonStorageDriverConfig)

	cloneConfig.Username = tridentconfig.REDACTED         // redact the username
	cloneConfig.Password = tridentconfig.REDACTED         // redact the password
	cloneConfig.ClientPrivateKey = tridentconfig.REDACTED // redact the client private key
	cloneConfig.ChapInitiatorSecret = tridentconfig.REDACTED
	cloneConfig.ChapTargetInitiatorSecret = tridentconfig.REDACTED
	cloneConfig.ChapTargetUsername = tridentconfig.REDACTED
	cloneConfig.ChapUsername = tridentconfig.REDACTED
	cloneConfig.Credentials = map[string]string{
		drivers.KeyName: tridentconfig.REDACTED,
		drivers.KeyType: tridentconfig.REDACTED,
	} // redact the credentials

	// https://github.com/golang/go/issues/4609
	// It's how gob encoding-decoding works, it flattens the pointer while encoding,
	// and during the decoding phase, if the default value is encountered, it is assigned as nil.
	if config.UseREST != nil {
		cloneConfig.UseREST = convert.ToPtr(*config.UseREST)
	}

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
		if errors.IsNotFoundError(err) || errors.IsUnsupportedError(err) {
			return nil, err
		} else {
			return nil, fmt.Errorf("error reading volume size: %v", err)
		}
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

// GetGroupSnapshotTarget returns a set of information about the target of a group snapshot.
// This information is used to gather information in a consistent way across storage drivers.
func GetGroupSnapshotTarget(
	ctx context.Context, volConfigs []*storage.VolumeConfig, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) (*storage.GroupSnapshotTargetInfo, error) {
	fields := LogFields{
		"Method": "GetGroupSnapshotTarget",
		"Type":   "ontap_common",
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetGroupSnapshotTarget")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetGroupSnapshotTarget")

	targetType := PersonalityUnified
	if config.Flags != nil && config.Flags[FlagPersonality] != "" {
		targetType = config.Flags[FlagPersonality]
	}

	targetUUID := client.GetSVMUUID()

	// Construct a set of unique source volume IDs to volume names to configs for the group snapshot.
	targetVolumes := make(storage.GroupSnapshotTargetVolumes, 0)
	for _, volumeConfig := range volConfigs {
		volumeName := volumeConfig.Name
		internalVolName := volumeConfig.InternalName

		// If the specified volume doesn't exist, return error
		if volExists, err := client.VolumeExists(ctx, internalVolName); err != nil {
			return nil, fmt.Errorf("error checking for existing volume: %v", err)
		} else if !volExists {
			return nil, errors.NotFoundError("volume %s does not exist", internalVolName)
		}

		if targetVolumes[internalVolName] == nil {
			targetVolumes[internalVolName] = make(map[string]*storage.VolumeConfig)
		}

		// For other drivers such as the SAN-Eco driver, this will be a flexvol name -> pv name -> volume config.
		targetVolumes[internalVolName][volumeName] = volumeConfig
	}

	return storage.NewGroupSnapshotTargetInfo(targetType, targetUUID, targetVolumes), nil
}

func CreateGroupSnapshot(
	ctx context.Context, groupSnapshotConfig *storage.GroupSnapshotConfig, target *storage.GroupSnapshotTargetInfo,
	driverConfig *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) error {
	fields := LogFields{
		"Method": "CreateGroupSnapshot",
		"Type":   "ontap_common",
	}
	Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> CreateGroupSnapshot")
	defer Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< CreateGroupSnapshot")

	// Assign an internal name at the driver level.
	groupSnapshotConfig.InternalName = groupSnapshotConfig.ID()
	groupName := groupSnapshotConfig.ID()
	snapName, err := storage.ConvertGroupSnapshotID(groupName)
	if err != nil {
		return err
	}

	// Look at all target source internalVolumes and build a list of snapshots.
	// The sourceVolumeID here correlates to a parent FlexVolume.
	// The list of constituentVolumes is a set of internalVolumes that exist under the source internalVolumeName ID.
	internalVolumes := make([]string, 0, len(target.GetVolumes()))
	for vol := range target.GetVolumes() {
		internalVolumes = append(internalVolumes, vol)
	}

	// Group snapshots for volumes that reside in a single internal volume do not require a CG snapshot.
	if len(internalVolumes) == 1 {
		internalVolume := internalVolumes[0] // flexVol name.
		Logc(ctx).WithFields(LogFields{
			"targetVolume": internalVolume,
		}).Debug("Single volume in group snapshot target; creating a snapshot.")
		if err = client.VolumeSnapshotCreate(ctx, snapName, internalVolume); err != nil {
			return err
		}
		return nil
	}

	// There are multiple volumes to snapshot; this requires a consistency group snapshot.
	Logc(ctx).WithField(
		"targetVolumes", internalVolumes,
	).Debug("Multiple volumes in group snapshot; creating a consistency group snapshot.")
	if err = client.ConsistencyGroupSnapshot(ctx, snapName, internalVolumes); err != nil {
		return err
	}

	return nil
}

// ProcessGroupSnapshot may only be used by the SAN and NAS drivers for processing a group snapshot.
func ProcessGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, volConfigs []*storage.VolumeConfig,
	driverConfig *drivers.OntapStorageDriverConfig, client api.OntapAPI,
	sizeGetter func(context.Context, string) (int, error),
) ([]*storage.Snapshot, error) {
	fields := LogFields{
		"Method": "ProcessGroupSnapshot",
		"Type":   "ontap_common",
	}
	Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ProcessGroupSnapshot")
	defer Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ProcessGroupSnapshot")

	// Construct the snapshot name from the group snapshot ID.
	groupName := config.ID()
	snapName, err := storage.ConvertGroupSnapshotID(groupName)
	if err != nil {
		return nil, err
	}

	var errs error
	snapshots := make([]*storage.Snapshot, 0)
	for _, volumeConfig := range volConfigs {
		volumeName := volumeConfig.Name
		internalVolumeName := volumeConfig.InternalName
		snap, err := client.VolumeSnapshotInfo(ctx, snapName, internalVolumeName)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		size, err := sizeGetter(ctx, internalVolumeName)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("error reading volume size: %w", err))
			continue
		}

		// Create a snapshot config and object for each constituent snapshot for the group.
		snapConfig := &storage.SnapshotConfig{
			Name:               snapName,
			InternalName:       snapName,
			VolumeInternalName: internalVolumeName,
			VolumeName:         volumeName,
			ImportNotManaged:   false,
			GroupSnapshotName:  groupName,
		}

		snapshot := &storage.Snapshot{
			Config:    snapConfig,
			Created:   snap.CreateTime,
			SizeBytes: int64(size),
			State:     storage.SnapshotStateOnline,
		}

		// Build the sets of snapshots and IDs.
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, errs
}

// ConstructGroupSnapshot accepts a group snapshot config, a list of snapshots and
// constructs the group snapshot storage model.
func ConstructGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig, snapshots []*storage.Snapshot,
	driverConfig *drivers.OntapStorageDriverConfig,
) (*storage.GroupSnapshot, error) {
	fields := LogFields{
		"Method": "ConstructGroupSnapshot",
		"Type":   "ontap_common",
	}
	Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> ConstructGroupSnapshot")
	defer Logd(ctx, driverConfig.StorageDriverName,
		driverConfig.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< ConstructGroupSnapshot")

	if config == nil {
		return nil, fmt.Errorf("empty group snapshot config")
	} else if len(snapshots) == 0 {
		return nil, fmt.Errorf("no grouped snapshots provided for: '%s'", config.ID())
	}

	// Construct the group snapshot.
	dateCreated := ""
	snapshotIDs := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		// Check the creation timestamps.
		if dateCreated == "" {
			dateCreated = snapshot.Created
		} else if dateCreated != snapshot.Created {
			Logc(ctx).Debugf("Snapshots in group '%s' created at different times: '%s' vs '%s'",
				config.ID(), dateCreated, snapshot.Created)
		}

		snapshotIDs = append(snapshotIDs, snapshot.ID())
	}

	return storage.NewGroupSnapshot(config, snapshotIDs, dateCreated), nil
}

func cleanupFailedCloneFlexVol(ctx context.Context, client api.OntapAPI, err error, clonedVolName, sourceVol,
	createdSnapName string,
) {
	// Return if err is nil or if the error is a volume exists error.
	if err == nil || drivers.IsVolumeExistsError(err) {
		return
	}

	Logc(ctx).WithFields(LogFields{
		"clonedVolume": clonedVolName,
		"sourceVol":    sourceVol,
		"snapshot":     createdSnapName,
	}).Debug("Cleaning up after failed flexvol clone.")

	if clonedVolName != "" {
		Logc(ctx).WithFields(LogFields{
			"volume": clonedVolName,
		}).Debug("Deleting volume after failed flexvol clone.")

		if destroyErr := client.VolumeDestroy(ctx, clonedVolName, true, true); destroyErr != nil {
			Logc(ctx).WithError(destroyErr).Warn("Unable to delete volume after failed volume clone.")
		}
	}

	if createdSnapName != "" {
		Logc(ctx).WithFields(LogFields{
			"snapshot": createdSnapName,
		}).Debug("Deleting snapshot after failed flexvol clone.")

		if snapDeleteErr := client.VolumeSnapshotDelete(ctx, createdSnapName, sourceVol); snapDeleteErr != nil {
			Logc(ctx).WithError(snapDeleteErr).Warn("Unable to delete snapshot after failed volume clone.")
		}
	}
}

// cloneFlexvol creates a volume clone
func cloneFlexvol(
	ctx context.Context, cloneVolConfig *storage.VolumeConfig, labels string, split bool,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI, qosPolicyGroup api.QosPolicyGroup,
) error {
	// Vars used in failed clone cleanup
	var err error
	var clonedVolName string   // Holds the cloned volume name
	var createdSnapName string // Holds the snapshot name of a Trident created snapshot

	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	// Cleanup cloned volume and snapshots we created if we error
	defer func() {
		cleanupFailedCloneFlexVol(ctx, client, err, clonedVolName, source, createdSnapName)
	}()

	fields := LogFields{
		"Method":   "cloneFlexvol",
		"Type":     "ontap_common",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
		"split":    split,
		"labels":   labels,
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
		return drivers.NewVolumeExistsError(name)
	}

	// If no specific snapshot was requested, create one
	if snapshot == "" {
		snapshot = time.Now().UTC().Format(storage.SnapshotNameFormat)
		if err = client.VolumeSnapshotCreate(ctx, snapshot, source); err != nil {
			return err
		}
		createdSnapName = snapshot
		cloneVolConfig.CloneSourceSnapshotInternal = snapshot
	}

	if err = duringVolCloneAfterSnapCreation1.Inject(); err != nil {
		return err
	}

	// Create the clone based on a snapshot
	if err = client.VolumeCloneCreate(ctx, name, source, snapshot, false); err != nil {
		return err
	}
	clonedVolName = name

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
		if err = client.VolumeSetQosPolicyGroupName(ctx, name, qosPolicyGroup); err != nil {
			return err
		}
	}

	if err = duringVolCloneAfterSnapCreation2.Inject(); err != nil {
		return err
	}

	// Split the clone if requested
	if split {
		if err = client.VolumeCloneSplitStart(ctx, name); err != nil {
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
		return errors.New(msg)
	}

	errored := false
	for _, igroup := range igroups {
		igroupMutex.Lock(igroup)
		defer igroupMutex.Unlock(igroup)
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
		return errors.New(msg)
	}

	if lunID >= 0 {
		err := clientAPI.LunUnmap(ctx, igroup, lunPath)
		if err != nil {
			msg := "error unmapping LUN"
			Logc(ctx).WithError(err).Error(msg)
			return errors.New(msg)
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
		return errors.New(msg)
	}

	if len(luns) == 0 {
		Logc(ctx).WithField(
			"igroup", igroup,
		).Debugf("No LUNs mapped to this igroup; deleting igroup.")
		if err := clientAPI.IgroupDestroy(ctx, igroup); err != nil {
			msg := fmt.Sprintf("error deleting igroup %s", igroup)
			Logc(ctx).WithError(err).Error(msg)
			return errors.New(msg)
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
	lunMutex.Lock(lunPath)
	defer lunMutex.Unlock(lunPath)
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
		return errors.New(msg)
	}

	volumeConfig.AccessInfo.PublishEnforcement = true
	volumeConfig.AccessInfo.FCPLunNumber = -1
	volumeConfig.AccessInfo.FCPIgroup = ""
	volumeConfig.AccessInfo.IscsiLunNumber = -1
	volumeConfig.AccessInfo.IscsiIgroup = ""

	return nil
}

// HealSANPublishEnforcement is a no-op for ONTAP-SAN volumes because ONTAP-SAN already properly sets
// the LUN mappings during publish/unpublish,
// operations. This function is implemented to satisfy interface assertions on the drivers.
func HealSANPublishEnforcement(_ context.Context, _ storage.Driver, _ *storage.Volume) bool {
	return false
}

// HealNASPublishEnforcement checks if publish enforcement should be enabled on the given NAS volume
// and updates the volume config accordingly. It returns true if the volume config was updated.
func HealNASPublishEnforcement(ctx context.Context, driver storage.Driver, volume *storage.Volume) bool {
	var updated bool
	// Check if publish enforcement is already set.
	if volume.Config.AccessInfo.PublishEnforcement {
		// If publish enforcement is already enabled on the volume, nothing to do.
		return updated
	}

	policy := volume.Config.ExportPolicy
	driverConfig := driver.GetCommonConfig(ctx)
	if policy == getEmptyExportPolicyName(*driverConfig.StoragePrefix) ||
		policy == volume.Config.InternalName {
		volume.Config.AccessInfo.PublishEnforcement = true
		updated = true
	}
	return updated
}

func ValidateStoragePrefixEconomy(storagePrefix string) error {
	// Ensure storage prefix is compatible with ONTAP
	matched, err := regexp.MatchString(`^$|^[a-zA-Z0-9_.-]*$`, storagePrefix)
	if err != nil {
		err = fmt.Errorf("could not check storage prefix; %v", err)
	} else if !matched {
		err = errors.New("storage prefix may only contain letters/digits/underscore/dash")
	}

	return err
}

func parseVolumeHandle(volumeHandle string) (svm, flexvol string, err error) {
	tokens := strings.SplitN(volumeHandle, ":", 2)
	if len(tokens) != 2 {
		return "", "", errors.New("invalid volume handle")
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
			return fmt.Sprintf("/%s/%s/%s", volConfig.CloneSourceVolumeInternal, ".snapshot",
				volConfig.CloneSourceSnapshot)
		} else if volumeName != tridentconfig.UnixPathSeparator+volConfig.InternalName && strings.HasPrefix(volumeName,
			tridentconfig.UnixPathSeparator) {
			// For managed import, return the original junction path
			return volumeName
		}
		return fmt.Sprintf("/%s", volConfig.InternalName)
	case sa.SMB:
		if smbShare != "" && !volConfig.SecureSMBEnabled {
			smbSharePath = fmt.Sprintf("\\%s", smbShare)
		} else {
			// Set share path as empty, volume name contains the path prefix.
			smbSharePath = ""
		}

		if volConfig.ReadOnlyClone {
			if volConfig.SecureSMBEnabled {
				completeVolumePath = fmt.Sprintf("%s\\%s\\%s\\%s", smbSharePath, volConfig.InternalName, "~snapshot",
					volConfig.CloneSourceSnapshot)
			} else {
				completeVolumePath = fmt.Sprintf("%s\\%s\\%s\\%s", smbSharePath, volConfig.CloneSourceVolumeInternal,
					"~snapshot", volConfig.CloneSourceSnapshot)
			}
		} else {
			if volConfig.SecureSMBEnabled && volConfig.ImportOriginalName != "" {
				// For Secure SMB, Trident creates new share with the internalName of the volume.
				completeVolumePath = smbSharePath + "/" + volConfig.InternalName
			} else {
				// If the user does not specify an SMB Share, Trident creates it with the same name as the flexvol volume name.
				completeVolumePath = smbSharePath + volumeName
			}
		}
	}
	// Replace unix styled path separator, if exists
	return strings.Replace(completeVolumePath, tridentconfig.UnixPathSeparator, tridentconfig.WindowsPathSeparator, -1)
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
		completeVolumePath = tridentconfig.WindowsPathSeparator + smbShare + volumeName
	} else {
		// If the user does not specify an SMB Share, Trident creates it with the same name as the flexGroup volume name.
		completeVolumePath = volumeName
	}

	// Replace unix styled path separator, if exists
	return strings.Replace(completeVolumePath, tridentconfig.UnixPathSeparator, tridentconfig.WindowsPathSeparator, -1)
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
		if smbShare != "" && !volConfig.SecureSMBEnabled {
			smbSharePath = smbShare + tridentconfig.WindowsPathSeparator
		}
		// In Secure SMB mode, Trident creates a unique SMB share for each qtree. Therefore, we use internalName (the qtree name) to construct the SMB path, instead of flexvol.
		if volConfig.ReadOnlyClone {
			if volConfig.SecureSMBEnabled {
				// Secure SMB, read-only clone:
				// Use for SMB client access to a snapshot of a qtree.
				// Example: \qtree1\~snapshot\snap1
				completeVolumePath = fmt.Sprintf("\\%s\\%s\\%s", volConfig.InternalName, "~snapshot", volConfig.CloneSourceSnapshot)
			} else {
				// Non-secure SMB, read-only clone:
				// Use for SMB client access to a snapshot of a qtree via a shared SMB path.
				// Example: \share1\flexvol1\sourceQtree\~snapshot\snap1
				completeVolumePath = fmt.Sprintf("\\%s%s\\%s\\%s\\%s", smbSharePath, flexvol, volConfig.CloneSourceVolumeInternal, "~snapshot", volConfig.CloneSourceSnapshot)
			}
		} else {
			if volConfig.SecureSMBEnabled {
				// Secure SMB :
				// Use for SMB client access to a qtree.
				// Example: \qtree1
				completeVolumePath = fmt.Sprintf("\\%s", volConfig.InternalName)
			} else {
				// Non-secure SMB,:
				// Use for SMB client access to a qtree via a shared SMB path.
				// Example: \share1\flexvol1\qtree1
				completeVolumePath = fmt.Sprintf("\\%s%s\\%s", smbSharePath, flexvol, volConfig.InternalName)
			}
		}
		// Replace unix styled path separator, if exists
		completeVolumePath = strings.Replace(completeVolumePath, tridentconfig.UnixPathSeparator,
			tridentconfig.WindowsPathSeparator,
			-1)
	}

	return
}

// ConstructLabelsFromConfigs  constructs labels from the name templates.
// If there is an error in constructing labels from templates, fall back to earlier method of constructing labels
// Function accepts parameters in following way:
// 1. pool: storage pool from which the template is read.
// 2. volConfig: volume config from which the values are read for .volume options in template.
// 3. driverConfig: driver config from which values are read for .config option in template.
func ConstructLabelsFromConfigs(
	ctx context.Context, pool storage.Pool, volConfig *storage.VolumeConfig,
	driverConfig *drivers.CommonStorageDriverConfig, labelLimit int,
) (string, error) {
	Logc(ctx).Debug(">>>> ConstructLabelsFromConfigs")
	defer Logc(ctx).Debug("<<<< ConstructLabelsFromConfigs")

	templateData := make(map[string]interface{})
	templateData["volume"] = volConfig
	templateData["config"] = driverConfig
	labels, labelErr := pool.GetTemplatizedLabelsJSON(
		ctx, storage.ProvisioningLabelTag, labelLimit, templateData)
	if labelErr != nil {
		labels, labelErr = pool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, labelLimit)
		if labelErr != nil {
			return "", labelErr
		}
	}

	return labels, labelErr
}

// ConstructPoolForLabels creates a new storage pool with a given name template and labels. It sets the internal
// attributes of the pool with the provided name template and sets the pool's attributes with the provided labels.
// The function returns the newly created storage pool.
func ConstructPoolForLabels(nameTemplate string, labels map[string]string) *storage.StoragePool {
	pool := &storage.StoragePool{}
	pool.SetInternalAttributes(map[string]string{
		NameTemplate: nameTemplate,
	},
	)
	pool.SetAttributes(map[string]sa.Offer{
		sa.Labels: sa.NewLabelOffer(labels),
	})

	return pool
}

func subtractUintFromSizeString(size string, val uint64) (string, error) {
	sizeBytesString, _ := capacity.ToBytes(size)
	sizeBytes, err := strconv.ParseUint(sizeBytesString, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid size string: %v", err)
	}
	if val > sizeBytes {
		return "", errors.New("right-hand term too large")
	}
	return strconv.FormatUint(sizeBytes-val, 10), nil
}

// adds LUKS overhead iff luksEncryption is a true boolean string. Logs an error if the luksEncryption string is not a boolean.
func incrementWithLUKSMetadataIfLUKSEnabled(ctx context.Context, size uint64, luksEncryption string) uint64 {
	isLUKS, err := strconv.ParseBool(luksEncryption)
	if err != nil && luksEncryption != "" {
		Logc(ctx).WithError(err).Debug("Could not parse luksEncryption string.")
	}

	if isLUKS {
		return size + luks.MetadataSize
	}

	return size
}

// removes LUKS overhead iff luksEncryption is a true boolean string. Logs an error if the luksEncryption string is not a boolean or size is too small.
func decrementWithLUKSMetadataIfLUKSEnabled(ctx context.Context, size uint64, luksEncryption string) uint64 {
	isLUKS, err := strconv.ParseBool(luksEncryption)
	if err != nil && luksEncryption != "" {
		Logc(ctx).WithError(err).Debug("Could not parse luksEncryption string.")
	}

	if luks.MetadataSize > size {
		Logc(ctx).WithError(err).WithField("size", size).Error("Size too small to subtract LUKS metadata.")
		return 0
	}

	if isLUKS {
		return size - luks.MetadataSize
	}

	return size
}

// deleteAutomaticSnapshot deletes a snapshot that was created automatically during volume clone creation.
// An automatic snapshot is detected by the presence of CloneSourceSnapshotInternal in the volume config
// while CloneSourceSnapshot is not set.  This method is called after the volume has been deleted, and it
// will only attempt snapshot deletion if the clone volume deletion completed without error.  This is a
// best-effort method, and any errors encountered will be logged but not returned.
func deleteAutomaticSnapshot(
	ctx context.Context, driver storage.Driver, volDeleteError error, volConfig *storage.VolumeConfig,
	snapshotDelete func(context.Context, string, string) error,
) {
	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshotInternal := volConfig.CloneSourceSnapshotInternal
	snapshot := volConfig.CloneSourceSnapshot

	fields := LogFields{
		"Method":       "DeleteAutomaticSnapshot",
		"Type":         "ontap_common",
		"snapshotName": snapshotInternal,
		"volumeName":   name,
	}

	methodDebugTraceFlags := driver.GetCommonConfig(ctx).DebugTraceFlags["method"]
	Logd(ctx, driver.Name(), methodDebugTraceFlags).WithFields(fields).Trace(">>>> deleteAutomaticSnapshot")
	defer Logd(ctx, driver.Name(), methodDebugTraceFlags).WithFields(fields).Trace("<<<< deleteAutomaticSnapshot")

	logFields := LogFields{
		"snapshotName":    snapshotInternal,
		"cloneSourceName": source,
		"cloneName":       name,
	}

	// Check if there is anything to do
	if !(snapshot == "" && snapshotInternal != "") {
		Logc(ctx).WithFields(logFields).Debug("No automatic clone source snapshot exists, skipping cleanup.")
		return
	}

	// If the clone volume couldn't be deleted, don't attempt to delete any automatic snapshot.
	if volDeleteError != nil {
		Logc(ctx).WithFields(logFields).Debug("Error deleting volume, skipping automatic snapshot cleanup.")
		return
	}

	if err := snapshotDelete(ctx, volConfig.CloneSourceSnapshotInternal, volConfig.CloneSourceVolumeInternal); err != nil {
		if api.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).Debug("Automatic snapshot not found, skipping cleanup.")
		} else {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting automatic snapshot. " +
				"Any automatic snapshot must be manually deleted.")
		}
	}
}

// removeExportPolicyRules takes an export policy name,
// retrieves its rules and matches the rules that exist to the IP addresses from the node.
// Any matched IP addresses will be removed from the export policy.
func removeExportPolicyRules(
	ctx context.Context, exportPolicy string, publishInfo *tridentmodels.VolumePublishInfo, clientAPI api.OntapAPI,
) error {
	fields := LogFields{
		"Method":     "removeExportPolicyRules",
		"policyName": exportPolicy,
		"nodeIPs":    publishInfo.HostIP,
	}

	Logc(ctx).WithFields(fields).Debug(">>>> removeExportPolicyRules")
	defer Logc(ctx).WithFields(fields).Debug("<<<< removeExportPolicyRules")

	exportPolicyMutex.Lock(exportPolicy)
	defer exportPolicyMutex.Unlock(exportPolicy)

	// CNVA-AWARE EXPORT POLICY MANAGEMENT:
	// Instead of blindly removing rules for the departing node, we implement "keep only active nodes" logic.
	// This is critical for CNVA where multiple PVCs (primary + subordinates) share the same export policy.
	// We must preserve rules for all remaining active nodes that still need access to the shared volume.

	// When all nodes have been unpublished
	if publishInfo.Nodes == nil || len(publishInfo.Nodes) == 0 {
		Logc(ctx).WithFields(fields).Debug("No active nodes remaining, removing ALL export policy rules.")

		// Get all existing rules and remove them
		existingRules, err := clientAPI.ExportRuleList(ctx, exportPolicy)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"policyName": exportPolicy,
				"error":      err,
			}).WithError(err).Error("Failed to list export policy rules for cleanup.")
			return err
		}

		// Remove all existing rules
		for ruleIndex := range existingRules {
			if err := clientAPI.ExportRuleDestroy(ctx, exportPolicy, ruleIndex); err != nil {
				Logc(ctx).WithFields(LogFields{
					"policyName": exportPolicy,
					"ruleIndex":  ruleIndex,
				}).WithError(err).Error("Failed to remove export policy rule during full cleanup.")
				// Continue removing other rules even if one fails
			} else {
				Logc(ctx).WithFields(LogFields{
					"policyName": exportPolicy,
					"ruleIndex":  ruleIndex,
				}).Debug("Removed export policy rule during full cleanup.")
			}
		}

		Logc(ctx).WithFields(LogFields{
			"policyName":        exportPolicy,
			"totalRulesRemoved": len(existingRules),
		}).Debug("All export policy rules removed for empty volume.")
		return nil
	}

	// Build set of IPs that should be KEPT (all active nodes)
	activeNodeIPs := make(map[string]struct{})
	for _, node := range publishInfo.Nodes {
		for _, ip := range node.IPs {
			ip = strings.TrimSpace(ip)
			activeNodeIPs[ip] = struct{}{}
		}
	}

	// Get existing export policy rules
	existingExportRules, err := clientAPI.ExportRuleList(ctx, exportPolicy)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"policyName": exportPolicy,
			"error":      err,
		}).WithError(err).Error("Failed to list existing export policy rules.")
		return err
	}

	var removeRuleIndexes []int

	// Analyze each existing rule to determine if it should be kept or removed
	for ruleIndex, clientMatch := range existingExportRules {
		clientIPs := strings.Split(clientMatch, ",")
		shouldKeepRule := false

		// Check if ANY IP in this rule should be preserved (belongs to active nodes)
		for _, singleClientIP := range clientIPs {
			singleClientIP = strings.TrimSpace(singleClientIP)
			if _, shouldKeep := activeNodeIPs[singleClientIP]; shouldKeep {
				shouldKeepRule = true
				break
			}
		}

		if !shouldKeepRule {
			// This rule contains ONLY IPs that are no longer active - safe to remove
			removeRuleIndexes = append(removeRuleIndexes, ruleIndex)
		}
	}

	// Remove rules that contain only inactive node IPs
	for _, ruleIndex := range removeRuleIndexes {
		ruleToRemove := existingExportRules[ruleIndex]

		if err = clientAPI.ExportRuleDestroy(ctx, exportPolicy, ruleIndex); err != nil {
			Logc(ctx).WithFields(LogFields{
				"policyName":   exportPolicy,
				"ruleIndex":    ruleIndex,
				"ruleToRemove": ruleToRemove,
				"error":        err,
			}).WithError(err).Error("Failed to remove export policy rule.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"policyName":  exportPolicy,
				"ruleIndex":   ruleIndex,
				"removedRule": ruleToRemove,
			}).Debug("Removed export policy rule.")
		}
	}

	// Get final rules after cleanup
	finalExportRules, err := clientAPI.ExportRuleList(ctx, exportPolicy)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"policyName": exportPolicy,
			"error":      err,
		}).WithError(err).Error("Could not list final export policy rules.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"policyName":        exportPolicy,
			"finalExportRules":  finalExportRules,
			"finalRuleCount":    len(finalExportRules),
			"originalRuleCount": len(existingExportRules),
			"removedRuleCount":  len(removeRuleIndexes),
		}).Debug("Export policy cleanup completed.")
	}

	return nil
}

func deleteAutomaticASASnapshot(
	ctx context.Context,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
	volConfig *storage.VolumeConfig,
) {
	name := volConfig.InternalName
	source := volConfig.CloneSourceVolumeInternal
	snapshotInternal := volConfig.CloneSourceSnapshotInternal
	snapshot := volConfig.CloneSourceSnapshot

	fields := LogFields{
		"Method":       "DeleteAutomaticASASnapshot",
		"Type":         "ontap_common",
		"snapshotName": snapshotInternal,
		"volumeName":   name,
	}

	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> deleteAutomaticASASnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< deleteAutomaticASASnapshot")

	logFields := LogFields{
		"snapshotName":    snapshotInternal,
		"cloneSourceName": source,
		"cloneName":       name,
	}

	// Automatic snapshot when created will set snapshotInternal field and leave snapshot field empty. Check if such exists.
	if !(snapshot == "" && snapshotInternal != "") {
		Logc(ctx).WithFields(logFields).Debug("No automatic ASA clone source snapshot exists, skipping cleanup.")
		return
	}

	// Delete automatic ASA snapshot with backoff retry to handle busy snapshots case
	deleteSnapshot := func() error {
		if err := client.StorageUnitSnapshotDelete(ctx, snapshotInternal, source); err != nil {
			if api.IsNotFoundError(err) {
				// Snapshot is already deleted. Nothing to clean up.
				Logc(ctx).WithFields(logFields).Debug("Automatic ASA snapshot not found, skipping cleanup.")
				return nil
			} else if api.IsSnapshotBusyError(err) {
				// Snapshot is busy. Retry after some time.
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting automatic ASA snapshot. " +
					"Snapshot is busy. Retrying after some time.")
				return err
			} else {
				// Some other error occurred. Log the error and ask to do cleanup manually.
				Logc(ctx).WithFields(logFields).WithError(err).Error("Error deleting automatic ASA snapshot. " +
					"Any automatic snapshot must be manually deleted.")
				return nil
			}
		}

		return nil
	}

	statusNotify := func(err error, duration time.Duration) {
		logFields["increment"] = duration
		Logc(ctx).WithFields(logFields).Debug("Automatic ASA Snapshot is not deleted, waiting.")
	}

	statusBackoff := backoff.NewExponentialBackOff()
	statusBackoff.InitialInterval = 2 * time.Second
	statusBackoff.Multiplier = 2
	statusBackoff.RandomizationFactor = 0.1
	statusBackoff.MaxElapsedTime = 10 * time.Second

	// Delete automatic ASA snapshot using an exponential backoff
	if err := backoff.RetryNotify(deleteSnapshot, statusBackoff, statusNotify); err != nil {
		Logc(ctx).WithFields(logFields).Warnf("Automatic ASA Snapshot not deleted even after %3.2f seconds."+
			"Any automatic snapshot must be manually deleted.",
			statusBackoff.MaxElapsedTime.Seconds())
		return
	}

	return
}

// createASASnapshot Creates a snapshot for ASA unit.
func createASASnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "createASASnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> createASASnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< createASASnapshot")

	// Create the snapshot for ASA unit
	if err := client.StorageUnitSnapshotCreate(ctx, internalSnapName, internalVolName); err != nil {
		return nil, err
	}

	snap, err := client.StorageUnitSnapshotInfo(ctx, internalSnapName, internalVolName)
	if err != nil {
		return nil, err
	}

	if snap == nil {
		// unlikely case
		return nil, fmt.Errorf("no snapshot with name %v could be created for volume %v", internalSnapName, internalVolName)
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
		"created":      snap.CreateTime,
	}).Debug("Found snapshot.")

	return &storage.Snapshot{
		Config:  snapConfig,
		Created: snap.CreateTime,
		State:   storage.SnapshotStateOnline,
	}, nil
}

// getASASnapshot gets an ASA snapshot.  To distinguish between an API error reading the snapshot
// and a non-existent snapshot, this method may return (nil, nil).
func getASASnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) (*storage.Snapshot, error) {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "getASASnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> GetSnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< GetSnapshot")

	snap, err := client.StorageUnitSnapshotInfo(ctx, internalSnapName, internalVolName)
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
		Config:  snapConfig,
		Created: snap.CreateTime,
		State:   storage.SnapshotStateOnline,
	}, nil
}

// getVolumeSnapshotList returns the list of snapshots associated with the named volume.
func getASASnapshotList(
	ctx context.Context, volConfig *storage.VolumeConfig,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) ([]*storage.Snapshot, error) {
	internalVolName := volConfig.InternalName

	fields := LogFields{
		"Method":     "getASASnapshotList",
		"Type":       "ontap_common",
		"volumeName": internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> getASASnapshotList")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< getASASnapshotList")

	snapshots, err := client.StorageUnitSnapshotList(ctx, internalVolName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	if snapshots == nil {
		return nil, fmt.Errorf("no snapshots found for volume %v", internalVolName)
	}

	result := make([]*storage.Snapshot, 0)

	for _, snap := range *snapshots {

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
			Created: snap.CreateTime,
			State:   storage.SnapshotStateOnline,
		}

		result = append(result, snapshot)
	}

	return result, nil
}

// restoreASASnapshot restores an ASA volume (in place) from a snapshot.
func restoreASASnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "restoreASASnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> RestoreASASnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< RestoreASASnapshot")

	if err := client.StorageUnitSnapshotRestore(ctx, internalSnapName, internalVolName); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Debug("Restored snapshot.")

	return nil
}

func deleteASASnapshot(
	ctx context.Context, snapConfig *storage.SnapshotConfig,
	config *drivers.OntapStorageDriverConfig, client api.OntapAPI,
	cloneSplitTimers *sync.Map,
) error {
	internalSnapName := snapConfig.InternalName
	internalVolName := snapConfig.VolumeInternalName

	fields := LogFields{
		"Method":       "deleteASASnapshot",
		"Type":         "ontap_common",
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}
	Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> DeleteASASnapshot")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< DeleteASASnapshot")

	err := client.StorageUnitSnapshotDelete(ctx, snapConfig.InternalName, internalVolName)
	if err != nil {
		if api.IsSnapshotBusyError(err) {
			Logc(ctx).WithFields(LogFields{
				"snapshotName": internalSnapName,
				"volumeName":   internalVolName,
			}).Debug("Snapshot busy.")

			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			SplitASAVolumeFromBusySnapshotWithDelay(ctx, snapConfig, config, client,
				client.StorageUnitCloneSplitStart, cloneSplitTimers)
		}

		// We must return the error, even if we started a split, so the snapshot delete is retried.
		return err
	}

	// Clean up any split timer
	cloneSplitTimers.Delete(snapConfig.ID())

	Logc(ctx).WithFields(LogFields{
		"snapshotName": internalSnapName,
		"volumeName":   internalVolName,
	}).Debug("Deleted ASA snapshot.")

	return nil
}

// cloneASAvol clones an ASA volume from a snapshot.  The clone name must not already exist.
func cloneASAvol(
	ctx context.Context, cloneVolConfig *storage.VolumeConfig,
	split bool, config *drivers.OntapStorageDriverConfig,
	client api.OntapAPI,
) error {
	name := cloneVolConfig.InternalName
	source := cloneVolConfig.CloneSourceVolumeInternal
	snapshot := cloneVolConfig.CloneSourceSnapshotInternal

	fields := LogFields{
		"Method":   "cloneASAvol",
		"Type":     "ontap_common",
		"name":     name,
		"source":   source,
		"snapshot": snapshot,
		"split":    split,
	}
	Logd(ctx, config.StorageDriverName, config.DebugTraceFlags["method"]).WithFields(fields).Trace(">>>> cloneASAvol")
	defer Logd(ctx, config.StorageDriverName,
		config.DebugTraceFlags["method"]).WithFields(fields).Trace("<<<< cloneASAvol")

	// If the storage unit (could be LUN or NVMe namespace) corresponding to the clone name already exists, return an error
	exists, err := client.StorageUnitExists(ctx, name)
	if err != nil {
		return fmt.Errorf("error checking for existing volume: %v", err)
	}

	if exists {
		return drivers.NewVolumeExistsError(name)
	}

	// If no specific snapshot was requested, create one
	if snapshot == "" {
		snapshot = time.Now().UTC().Format(storage.SnapshotNameFormat)
		if err = client.StorageUnitSnapshotCreate(ctx, snapshot, source); err != nil {
			return err
		}
		cloneVolConfig.CloneSourceSnapshotInternal = snapshot
	}

	// Create the clone based on a snapshot
	if err = client.StorageUnitCloneCreate(ctx, name, source, snapshot); err != nil {
		return err
	}

	// Split the clone if requested
	if split {
		if err := client.StorageUnitCloneSplitStart(ctx, name); err != nil {
			return fmt.Errorf("error splitting clone: %v", err)
		}
	}

	return nil
}

// purgeRecoveryQueueVolume is best effort
func purgeRecoveryQueueVolume(ctx context.Context, api api.OntapAPI, volumeName string) {
	recoveryQueueVolumeName, err := api.VolumeRecoveryQueueGetName(ctx, volumeName)
	if err != nil {
		Logc(ctx).WithField("volume",
			volumeName).Errorf("error getting volume from ONTAP recovery queue: %v", err)
		return
	}

	if err = api.VolumeRecoveryQueuePurge(ctx, recoveryQueueVolumeName); err != nil {
		Logc(ctx).WithField("volume",
			volumeName).Errorf("error purging volume from ONTAP recovery queue: %v", err)
	}
}

// getSMBShareNamePath constructs the SMB share name and path based on the provided parameters.
func getSMBShareNamePath(flexvol, name string, secureSMBEnabled bool) (string, string) {
	if secureSMBEnabled {
		return name, "/" + flexvol + "/" + name
	}
	return flexvol, "/" + flexvol
}

// getUniqueNodeSpecificSubsystemName generates a subsystem name combining node name and Trident UUID along with a prefix.
// If the generated subsystem name exceeds the maximum length, it attempts to shorten the Trident UUID.
// If it still exceeds the maximum length, it generates a hash of the subsystem name and truncates the hash if needed.
func getUniqueNodeSpecificSubsystemName(
	nodeName, tridentUUID, prefix string, maxSubsystemLength int,
) (string, string, error) {
	// Ensure node name is not empty
	if len(nodeName) == 0 {
		return "", "", fmt.Errorf("failed to generate susbsystem name for node %v; node name is empty", nodeName)
	}

	// Ensure Trident UUID is not empty
	if len(tridentUUID) == 0 {
		return "", "", fmt.Errorf("failed to generate susbsystem name for node %v; Trident UUID is empty", nodeName)
	}

	// Construct the subsystem name
	completeSSName := fmt.Sprintf("%s_%s_%s", prefix, nodeName, tridentUUID)
	// Skip any underscores at the beginning
	completeSSName = strings.TrimLeft(completeSSName, "_")
	finalSSName := completeSSName

	// Ensure the final name does not exceed the maximum length
	if len(finalSSName) > maxSubsystemLength {
		// Try reducing character count of Trident UUID.
		// Convert it to raw bytes and then do base64 encoding. This returns 24 character string.
		u, err := uuid.Parse(tridentUUID)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate susbsystem name for node %v; %w", nodeName, err)
		}

		base64Str := base64.StdEncoding.EncodeToString(u[:])
		finalSSName = fmt.Sprintf("%s_%s_%s", prefix, nodeName, base64Str)
		finalSSName = strings.TrimLeft(finalSSName, "_")

		if len(finalSSName) > maxSubsystemLength {
			// If even after Trident UUID reduction, the length is more than max length,
			// then generate a hash and truncate if needed.
			finalSSName = fmt.Sprintf("%x", sha256.Sum256([]byte(completeSSName)))
			if len(finalSSName) > maxSubsystemLength {
				finalSSName = finalSSName[:maxSubsystemLength]
			}
		}
	}

	return completeSSName, finalSSName, nil
}

// lockNamespaceAndSubsystem acquires the namespace lock first and then the subsystem lock.
// It returns a function that should be deferred to release the locks in reverse order.
func lockNamespaceAndSubsystem(nsUUID, ssUUID string) func() {
	namespaceMutex.Lock(nsUUID)
	subsystemMutex.Lock(ssUUID)

	return func() {
		subsystemMutex.Unlock(ssUUID)
		namespaceMutex.Unlock(nsUUID)
	}
}

// getSuperSubsystemName finds or allocates a SuperSubsystem for the given NVMe namespace.
//
// The function uses an approach with the following priority:
//  1. First checks if the namespace is already mapped to any SuperSubsystem
//  2. If not mapped, retrieves all SuperSubsystems (single wildcard call) and finds one with capacity.
//  3. Otherwise, creates a new SuperSubsystem number by filling gaps in the numbering sequence.
//     For example: if subsystems 1 and 3 exist, it returns 2; if 1,2,3 exist and all are full, it returns 4.
func getSuperSubsystemName(ctx context.Context, api api.OntapAPI, nsUUID, tridentUUID string) (string, error) {
	ssPrefix := fmt.Sprintf("%s%s_", superSubsystemNamePrefix, tridentUUID)

	// Get all the subsystems this namespace is mapped to
	mappedSubsystems, err := api.NVMeGetSubsystemsForNamespace(ctx, nsUUID)
	if err != nil {
		return "", fmt.Errorf("failed to get subsystems for namespace: %w", err)
	}

	// Check if any of the mapped subsystems is a SuperSubsystem
	for _, subsys := range mappedSubsystems {
		if strings.HasPrefix(subsys.Name, ssPrefix) {
			// Namespace is already mapped to a SuperSubsystem - return
			Logc(ctx).WithFields(LogFields{
				"namespace":   nsUUID,
				"subsystem":   subsys.Name,
				"subsystemID": subsys.UUID,
			}).Debug("Namespace already mapped to SuperSubsystem.")
			return subsys.Name, nil
		}
	}

	// Namespace is not mapped to any SuperSubsystem yet
	// Get all SuperSubsystems to find one with capacity or create a new one
	Logc(ctx).WithField("tridentUUID", tridentUUID).Debug("Getting all SuperSubsystems.")
	superSubsystems, err := api.NVMeSubsystemList(ctx, fmt.Sprintf("%s%s*", superSubsystemNamePrefix, tridentUUID))
	if err != nil {
		return "", fmt.Errorf("failed to list subsystems: %w", err)
	}

	usedSubsystemNumbers := make(map[int]bool, len(superSubsystems))
	for _, ss := range superSubsystems {
		// Extract number from name (e.g., "trident_subsystem_4321_19" -> 19)
		numStr := strings.TrimPrefix(ss.Name, ssPrefix)
		if num, err := strconv.Atoi(numStr); err == nil {
			usedSubsystemNumbers[num] = true
		}

		// check the capacity of subsystem and return if there is capacity left
		nsCount, err := api.NVMeSubsystemGetNamespaceCount(ctx, ss.UUID)
		if err != nil {
			return "", fmt.Errorf("error getting namespace count for subsystem %s: %w", ss.UUID, err)
		}
		if nsCount < maxNamespacesPerSuperSubsystem {
			Logc(ctx).WithFields(LogFields{
				"subsystem":      ss.Name,
				"namespaceCount": nsCount,
				"maxNamespaces":  maxNamespacesPerSuperSubsystem,
			}).Debug("Found SuperSubsystem with available capacity.")
			return ss.Name, nil
		}
	}

	// Create a new one, fill gaps in numbering (e.g., if 1,3 exist -> return 2)
	nextNumber := 1
	for {
		if !usedSubsystemNumbers[nextNumber] {
			newSubsystemName := fmt.Sprintf("%s%s_%d", superSubsystemNamePrefix, tridentUUID, nextNumber)
			Logc(ctx).WithFields(LogFields{
				"subsystem": newSubsystemName,
				"number":    nextNumber,
			}).Debug("Allocating new SuperSubsystem number.")
			return newSubsystemName, nil
		}
		nextNumber++
	}
}

// RemoveHostFromSubsystem handles the host removal during NVMe volume unpublish operations.
//	This function safely removes a host (NQN) from a subsystem, but only
//	if the host no longer has mounted any other namespaces from that subsystem. This is critical
//	because a single host can have multiple volumes (namespaces) mounted from the same subsystem.

// Algorithm:
//  1. Check if host exists in subsystem, if not no need to remove
//  2. Check if host has mounted any other namespaces from this subsystem
//  3. Remove host only if it has no other namespaces mounted from this subsystem
func RemoveHostFromSubsystem(
	ctx context.Context,
	api api.OntapAPI,
	hostNQN, subsystemUUID string,
	namespacesPublishedToNode []string,
) (bool, error) {
	fields := LogFields{
		"method":        "NVMeRemoveHostFromSubsystem",
		"hostNQN":       hostNQN,
		"subsystemUUID": subsystemUUID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> NVMeRemoveHostFromSubsystem")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NVMeRemoveHostFromSubsystem")

	// Step 1: Verify host exists in subsystem - if not present, nothing to do
	hostExists, err := isHostPresentInSubsystem(ctx, api, subsystemUUID, hostNQN)
	if err != nil {
		return false, fmt.Errorf("error checking if host %s exists in subsystem %s: %w", hostNQN, subsystemUUID, err)
	}
	if !hostExists {
		Logc(ctx).Debugf("Host %s not found in subsystem %s; no removal needed", hostNQN, subsystemUUID)
		return false, nil
	}

	// Step 2: Check if host has mounted any other namespaces from this subsystem
	otherNamespacesExists, err := namespacesFromSubsystemExistsOnHost(ctx, api, subsystemUUID, namespacesPublishedToNode)
	if err != nil {
		return false, fmt.Errorf("error checking host dependencies in subsystem %s: %w", subsystemUUID, err)
	}
	if otherNamespacesExists {
		Logc(ctx).Debugf("Host %s still has other namespaces from subsystem %s; cannot remove host yet",
			hostNQN, subsystemUUID)
		return false, nil
	}

	// Step 3: Safe to remove - host has no other publications from this subsystem
	if err := api.NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID); err != nil {
		return false, fmt.Errorf("failed to remove host %s from subsystem %s: %w", hostNQN, subsystemUUID, err)
	}

	Logc(ctx).Debugf("Successfully removed host %s from subsystem %s", hostNQN, subsystemUUID)
	return true, nil
}

// isHostPresentInSubsystem checks whether a host(NQN) exists in a subsystem
func isHostPresentInSubsystem(
	ctx context.Context,
	api api.OntapAPI,
	subsystemUUID, hostNQN string,
) (bool, error) {
	// Query all hosts currently added in this subsystem
	hosts, err := api.NVMeGetHostsOfSubsystem(ctx, subsystemUUID)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve host list for subsystem %s: %w", subsystemUUID, err)
	}

	// Search for the target host in the subsystem's host list
	for _, host := range hosts {
		if host != nil && host.NQN == hostNQN {
			return true, nil
		}
	}

	// Host not found in subsystem
	return false, nil
}

// namespacesFromSubsystemExistsOnHost checks if a host has mounted any other namespaces (other than the one
// being unpublished) from the specified subsystem.
//
//	Before removing a host from a subsystem, we must ensure the host doesn't have OTHER volumes
//	mounted from that same subsystem. This prevents breaking connectivity to other volumes.
//
// Why this matters:
//   - In SuperSubsystem scenarios: A host may have volumes vol1, vol2, vol3 all using the same subsystem.
//     When unpublishing vol1, we cannot remove the host because vol2 and vol3 still need access.
//   - In PerNode scenarios: A node's personal subsystem may contain namespaces from multiple volumes.
//     Removing the host prematurely would break access to all other volumes.
//
// How it works:
//  1. Get all namespaces that exist in this subsystem from ONTAP
//  2. Check if any of the host's currently mounted namespaces belong to this subsystem
//  3. If a match is found, the host still needs access to the subsystem
func namespacesFromSubsystemExistsOnHost(
	ctx context.Context,
	api api.OntapAPI,
	subsystemUUID string,
	namespacesPublishedToNode []string,
) (bool, error) {
	// Step 1: Get all namespaces that belong to this subsystem from ONTAP
	namespacesInSubsystem, err := api.NVMeGetNamespaceUUIDsForSubsystem(ctx, subsystemUUID)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve namespace list for subsystem %s: %w", subsystemUUID, err)
	}

	// Step 2: Convert subsystem's namespace list to map for O(1) lookup performance
	subsystemNamespaceMap := make(map[string]struct{}, len(namespacesInSubsystem))
	for _, nsUUID := range namespacesInSubsystem {
		subsystemNamespaceMap[nsUUID] = struct{}{}
	}

	// Step 3: Check if any of the host's mounted namespaces belong to this subsystem
	for _, nsUUID := range namespacesPublishedToNode {
		if _, exists := subsystemNamespaceMap[nsUUID]; exists {
			// Found a match - host has at least one other namespace from this subsystem
			Logc(ctx).Debugf("Host has another namespace %s from subsystem %s; host must remain",
				nsUUID, subsystemUUID)
			return true, nil
		}
	}

	// No matches found - host has no other namespaces from this subsystem
	return false, nil
}

// UnmapNamespaceFromSubsystem handles the namespace unmapping.
//
//	This function safely unmaps a namespace from a NVMe subsystem, but only if no other hosts
//	present in the subsystem, still need access to that namespace. This is critical for RWX (ReadWriteMany) volumes where
//	multiple hosts may be accessing the same namespace simultaneously.

// Algorithm:
//  1. Check if namespace is mapped, if not mapped -> nothing to be done
//  2. Check if any other hosts in the subsystem, still need this namespace
//  3. Unmap namespace only if no other hosts need it
func UnmapNamespaceFromSubsystem(
	ctx context.Context,
	api api.OntapAPI,
	subsystemUUID, namespaceUUID string,
	publishedNodes []*tridentmodels.Node,
) (bool, error) {
	fields := LogFields{
		"method":        "NVMeEnsureNamespaceUnmapped",
		"subsystemUUID": subsystemUUID,
		"namespaceUUID": namespaceUUID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> NVMeEnsureNamespaceUnmapped")
	defer Logc(ctx).WithFields(fields).Debug("<<<< NVMeEnsureNamespaceUnmapped")

	// Step 1: Verify namespace is mapped to subsystem - if not, nothing to do
	isMapped, err := isNamespaceMappedToSubsystem(ctx, api, subsystemUUID, namespaceUUID)
	if err != nil {
		return false, fmt.Errorf("error checking namespace %s mapping status: %w", namespaceUUID, err)
	}
	if !isMapped {
		Logc(ctx).Infof("Namespace %s is not mapped to subsystem %s; already unmapped",
			namespaceUUID, subsystemUUID)
		return true, nil
	}

	// Step 2: Check if any other hosts in the subsystem still need access to this namespace
	// if yes, then don't unmap namespace from this subsystem
	hostsNeedNS, err := otherHostsNeedNamespace(ctx, api, subsystemUUID, publishedNodes)
	if err != nil {
		return false, fmt.Errorf("error checking if other hosts need namespace %s: %w", namespaceUUID, err)
	}
	if hostsNeedNS {
		Logc(ctx).Debug("Other hosts still need access to this namespace; cannot unmap yet")
		return false, nil
	}

	// Step 3: Safe to unmap - no other hosts need this namespace
	if err := api.NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, namespaceUUID); err != nil {
		return false, fmt.Errorf("failed to unmap namespace %s from subsystem %s: %w",
			namespaceUUID, subsystemUUID, err)
	}
	Logc(ctx).Debugf("Successfully unmapped namespace %s from subsystem %s", namespaceUUID, subsystemUUID)

	return true, nil
}

// isNamespaceMappedToSubsystem checks if a namespace is currently mapped to a subsystem.
func isNamespaceMappedToSubsystem(
	ctx context.Context,
	api api.OntapAPI,
	subsystemUUID, namespaceUUID string,
) (bool, error) {
	isMapped, err := api.NVMeIsNamespaceMapped(ctx, subsystemUUID, namespaceUUID)
	if err != nil {
		return false, fmt.Errorf("failed to check mapping status for namespace %s in subsystem %s: %w",
			namespaceUUID, subsystemUUID, err)
	}
	return isMapped, nil
}

// otherHostsNeedNamespace determines if any hosts (other than the one being unpublished) in the subsystem still
// need access to the specified namespace within the subsystem.
//
// How it works:
//  1. Get all hosts currently registered in this subsystem from ONTAP
//  2. Build a map of host NQNs for O(1) lookup performance
//  3. Check if any node which has published this volume, is there in this subsystem
//  4. If a match is found, at least one other host still needs access to this namespace.
func otherHostsNeedNamespace(
	ctx context.Context,
	api api.OntapAPI,
	subsystemUUID string,
	publishedNodes []*tridentmodels.Node,
) (bool, error) {
	// Step 1: Get all hosts (NQNs) currently registered in this subsystem from ONTAP
	hosts, err := api.NVMeGetHostsOfSubsystem(ctx, subsystemUUID)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve host list for subsystem %s: %w", subsystemUUID, err)
	}

	// Step 2: Build a map of host NQNs present in this subsystem for O(1) lookup performance
	hostsInSubsystem := make(map[string]struct{})
	for _, host := range hosts {
		if host != nil && host.NQN != "" {
			hostsInSubsystem[host.NQN] = struct{}{}
		}
	}

	// Step 3:  Check if any node which has published this volume, is there in this subsystem
	// publishedNodes contains the list of nodes that currently have this volume published to them
	for _, node := range publishedNodes {
		if node != nil && node.NQN != "" {
			if _, exists := hostsInSubsystem[node.NQN]; exists {
				// Found a match - at least one other host in this subsystem still needs this namespace
				Logc(ctx).Debugf("Host %s still needs namespace in subsystem %s; cannot unmap yet",
					node.NQN, subsystemUUID)
				return true, nil
			}
		}
	}

	// No matches found - no other hosts in this subsystem need this namespace, safe to unmap
	return false, nil
}

// deleteSubsystemIfEmpty checks if subsystem has no namespaces and deletes it
func deleteSubsystemIfEmpty(ctx context.Context, api api.OntapAPI, subsystemUUID string) error {
	// Check remaining namespace count
	remainingNS, err := api.NVMeSubsystemGetNamespaceCount(ctx, subsystemUUID)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error getting remaining namespace count, skipping subsystem deletion")
		return nil
	}

	if remainingNS > 0 {
		Logc(ctx).Debugf("Subsystem %s still has %d namespaces, not deleting", subsystemUUID, remainingNS)
		return nil
	}

	// Subsystem has no namespaces, delete it
	if err := api.NVMeSubsystemDelete(ctx, subsystemUUID); err != nil {
		return fmt.Errorf("error deleting empty subsystem %s: %w", subsystemUUID, err)
	}
	Logc(ctx).Debugf("Deleted empty subsystem %s", subsystemUUID)

	return nil
}
