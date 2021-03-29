// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

func (d OntapAPIZAPI) ValidateAPIVersion(ctx context.Context) error {

	// Make sure we're using a valid ONTAP version
	ontapVersion, err := d.APIVersion(ctx)
	if err != nil {
		return fmt.Errorf("could not determine Data ONTAP API version: %v", err)
	}

	if !d.SupportsFeature(ctx, MinimumONTAPIVersion) {
		return errors.New("ONTAP 9.3 or later is required")
	}

	Logc(ctx).WithField("Ontapi", ontapVersion).Debug("ONTAP API version.")

	return nil
}

func (d OntapAPIZAPI) VolumeCreate(ctx context.Context, volume Volume) (*APIResponse, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeCreate",
			"Type":   "OntapAPIZAPI",
			"spec":   volume,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeCreate")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeCreate")
	}

	volCreateResponse, err := d.API.VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle, volume.TieringPolicy,
		volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve)
	if err != nil {
		return nil, fmt.Errorf("error creating volume: %v", err)
	}

	if volCreateResponse == nil {
		return nil, fmt.Errorf("missing volume create response")
	}

	response := &APIResponse{
		status: volCreateResponse.Result.ResultStatusAttr,
		reason: volCreateResponse.Result.ResultReasonAttr,
		errno:  volCreateResponse.Result.ResultErrnoAttr,
	}

	if err = GetError(ctx, volCreateResponse, err); err != nil {
		if zerr, ok := err.(ZapiError); ok {
			// Handle case where the Create is passed to every Docker Swarm node
			if zerr.Code() == azgo.EAPIERROR && strings.HasSuffix(strings.TrimSpace(zerr.Reason()), "Job exists") {
				Logc(ctx).WithField("volume", volume.Name).Warn("Volume create job already exists, " +
					"skipping volume create on this node.")
				err = VolumeCreateJobExistsError(fmt.Sprintf("volume create job already exists, %s",
					volume.Name))
			}
		}
	}

	return response, err
}

func (d OntapAPIZAPI) VolumeDestroy(ctx context.Context, name string, force bool) (*APIResponse, error) {

	volDestroyResponse, err := d.API.VolumeDestroy(name, force)
	if err != nil {
		return nil, fmt.Errorf("error destroying volume %v: %v", name, err)
	}

	if zerr := NewZapiError(volDestroyResponse); !zerr.IsPassed() {

		// It's not an error if the volume no longer exists
		if zerr.Code() == azgo.EVOLUMEDOESNOTEXIST {
			Logc(ctx).WithField("volume", name).Warn("Volume already deleted.")
		} else {
			return nil, fmt.Errorf("error destroying volume %v: %v", name, zerr)
		}
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeInfo(ctx context.Context, name string) (*Volume, *APIResponse, error) {

	if d.API.config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "VolumeInfo",
			"Type":   "OntapAPIZAPI",
			"name":   name,
		}
		Logc(ctx).WithFields(fields).Debug(">>>> VolumeInfo")
		defer Logc(ctx).WithFields(fields).Debug("<<<< VolumeInfo")
	}

	// Get Flexvol by name
	volumeGetResponse, err := d.API.VolumeGet(name)

	Logc(ctx).WithFields(log.Fields{
		"volumeGetResponse": volumeGetResponse,
		"name":              name,
		"err":               err,
	}).Debug("d.API.VolumeGet")

	if err != nil {
		Logc(ctx).Errorf("Could not find volume with name: %v; error: %v", name, err.Error())
		return nil, nil, VolumeReadError(fmt.Sprintf("could not find volume with name %s", name))
	}

	volumeInfo, err := VolumeInfoFromZapiAttrsHelper(volumeGetResponse)
	if err != nil {
		return nil, nil, err
	}

	return volumeInfo, APIResponsePassed, nil
}

func VolumeInfoFromZapiAttrsHelper(volumeGetResponse *azgo.VolumeAttributesType) (*Volume, error) {
	var responseAccessType string
	var responseAggregates []string
	var responseComment string
	var responseExportPolicy string
	var responseJunctionPath string
	var responseName string
	var responseSize string
	var responseSnapdirAccessEnabled bool
	var responseSnapshotPolicy string
	var responseSnapshotReserveInt int
	var responseSpaceReserve string
	var responseUnixPermissions string

	if volumeGetResponse.VolumeIdAttributesPtr != nil {
		volumeIdAttributes := volumeGetResponse.VolumeIdAttributes()

		responseAggregates = []string{
			volumeIdAttributes.ContainingAggregateName(),
		}

		if volumeGetResponse.VolumeIdAttributesPtr.TypePtr != nil {
			responseAccessType = volumeIdAttributes.Type()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.CommentPtr != nil {
			responseComment = volumeIdAttributes.Comment()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.NamePtr != nil {
			responseName = volumeIdAttributes.Name()
		}

		if volumeGetResponse.VolumeIdAttributesPtr.JunctionPathPtr != nil {
			responseJunctionPath = volumeIdAttributes.JunctionPath()
		}
	} else {
		return nil, VolumeIdAttributesReadError(fmt.Sprintf("error reading id attributes for volume %s",
			responseName))
	}

	if volumeGetResponse.VolumeSpaceAttributesPtr != nil {
		volumeSpaceAttrs := volumeGetResponse.VolumeSpaceAttributes()
		if volumeSpaceAttrs.PercentageSnapshotReservePtr != nil {
			responseSnapshotReserveInt = volumeSpaceAttrs.PercentageSnapshotReserve()
		}

		if volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuaranteePtr != nil {
			responseSpaceReserve = volumeGetResponse.VolumeSpaceAttributesPtr.SpaceGuarantee()
		}
		if volumeGetResponse.VolumeSpaceAttributesPtr.SizePtr != nil {
			responseSize = strconv.FormatInt(int64(volumeGetResponse.VolumeSpaceAttributesPtr.Size()), 10)
		} else {
			return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("volume %s size not available", responseName))
		}
	} else {
		return nil, VolumeSpaceAttributesReadError(fmt.Sprintf("error reading space attributes for volume %s",
			responseName))
	}

	if volumeGetResponse.VolumeExportAttributesPtr != nil {
		responseExportAttrs := volumeGetResponse.VolumeExportAttributes()
		if responseExportAttrs.PolicyPtr != nil {
			responseExportPolicy = responseExportAttrs.Policy()
		}
	}

	if volumeGetResponse.VolumeSecurityAttributesPtr != nil {
		responseSecurityAttrs := volumeGetResponse.VolumeSecurityAttributesPtr
		if responseSecurityAttrs.VolumeSecurityUnixAttributesPtr != nil {
			responseSecurityUnixAttrs := responseSecurityAttrs.VolumeSecurityUnixAttributes()
			if responseSecurityUnixAttrs.PermissionsPtr != nil {
				responseUnixPermissions = responseSecurityUnixAttrs.Permissions()
			}
		}
	}

	if volumeGetResponse.VolumeSnapshotAttributesPtr != nil {
		if volumeGetResponse.VolumeSnapshotAttributesPtr.SnapshotPolicyPtr != nil {
			responseSnapshotPolicy = volumeGetResponse.VolumeSnapshotAttributesPtr.SnapshotPolicy()
		}

		if volumeGetResponse.VolumeSnapshotAttributesPtr.SnapdirAccessEnabledPtr != nil {
			responseSnapdirAccessEnabled = volumeGetResponse.VolumeSnapshotAttributesPtr.SnapdirAccessEnabled()
		}
	}

	volumeInfo := &Volume{
		AccessType:      responseAccessType,
		Aggregates:      responseAggregates,
		Comment:         responseComment,
		ExportPolicy:    responseExportPolicy,
		JunctionPath:    responseJunctionPath,
		Name:            responseName,
		Size:            responseSize,
		SnapshotDir:     responseSnapdirAccessEnabled,
		SnapshotPolicy:  responseSnapshotPolicy,
		SnapshotReserve: responseSnapshotReserveInt,
		SpaceReserve:    responseSpaceReserve,
		UnixPermissions: responseUnixPermissions,
	}
	return volumeInfo, nil
}

func (d OntapAPIZAPI) APIVersion(ctx context.Context) (string, error) {
	return d.API.SystemGetOntapiVersion(ctx)
}

func (d OntapAPIZAPI) SupportsFeature(ctx context.Context, feature feature) bool {
	return d.API.SupportsFeature(ctx, feature)
}

func (d OntapAPIZAPI) NodeListSerialNumbers(ctx context.Context) ([]string, error) {
	return d.API.NodeListSerialNumbers(ctx)
}

type OntapAPIZAPI struct {
	API *Client
}

func NewOntapAPIZAPI(zapiClient *Client) (OntapAPIZAPI, error) {
	result := OntapAPIZAPI{
		API: zapiClient,
	}
	return result, nil
}

// NewZAPIClient is a factory method for creating a new instance
func NewZAPIClient(config ClientConfig) *Client {
	d := &Client{
		config: config,
		zr: &azgo.ZapiRunner{
			ManagementLIF:        config.ManagementLIF,
			SVM:                  config.SVM,
			Username:             config.Username,
			Password:             config.Password,
			ClientPrivateKey:     config.ClientPrivateKey,
			ClientCertificate:    config.ClientCertificate,
			TrustedCACertificate: config.TrustedCACertificate,
			Secure:               true,
			DebugTraceFlags:      config.DebugTraceFlags,
		},
		m: &sync.Mutex{},
	}
	return d
}

func NewZAPIClientFromOntapConfig(ctx context.Context, ontapConfig *drivers.OntapStorageDriverConfig,
	numRecords int) (OntapAPI, error) {

	client := NewZAPIClient(ClientConfig{
		ManagementLIF:           ontapConfig.ManagementLIF,
		SVM:                     ontapConfig.SVM,
		Username:                ontapConfig.Username,
		Password:                ontapConfig.Password,
		ClientCertificate:       ontapConfig.ClientCertificate,
		ClientPrivateKey:        ontapConfig.ClientPrivateKey,
		ContextBasedZapiRecords: numRecords,
		TrustedCACertificate:    ontapConfig.TrustedCACertificate,
		DebugTraceFlags:         ontapConfig.DebugTraceFlags,
	})
	if ontapConfig.SVM != "" {

		vserverResponse, err := client.VserverGetRequest()
		if err = GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error reading SVM details: %v", err)
		}

		client.SVMUUID = vserverResponse.Result.AttributesPtr.VserverInfoPtr.Uuid()

		Logc(ctx).WithField("SVM", ontapConfig.SVM).Debug("Using specified SVM.")

	} else {

		// Use VserverGetIterRequest to populate config.SVM if it wasn't specified and we can derive it
		vserverResponse, err := client.VserverGetIterRequest()
		if err = GetError(ctx, vserverResponse, err); err != nil {
			return nil, fmt.Errorf("error enumerating SVMs: %v", err)
		}

		if vserverResponse.Result.NumRecords() != 1 {
			return nil, errors.New("cannot derive SVM to use; please specify SVM in config file")
		}

		// Update everything to use our derived SVM
		ontapConfig.SVM = vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].VserverName()
		svmUUID := vserverResponse.Result.AttributesListPtr.VserverInfoPtr[0].Uuid()

		client = NewZAPIClient(ClientConfig{
			ManagementLIF:        ontapConfig.ManagementLIF,
			SVM:                  ontapConfig.SVM,
			Username:             ontapConfig.Username,
			Password:             ontapConfig.Password,
			ClientCertificate:    ontapConfig.ClientCertificate,
			ClientPrivateKey:     ontapConfig.ClientPrivateKey,
			TrustedCACertificate: ontapConfig.TrustedCACertificate,
			DebugTraceFlags:      ontapConfig.DebugTraceFlags,
		})
		client.SVMUUID = svmUUID
	}

	apiZAPI, err := NewOntapAPIZAPI(client)
	if err != nil {
		return nil, fmt.Errorf("unable to get zapi client for ontap: %v", err)
	}

	return apiZAPI, nil
}

func (d OntapAPIZAPI) NetInterfaceGetDataLIFs(ctx context.Context, protocol string) ([]string, error) {
	return d.API.NetInterfaceGetDataLIFs(ctx, protocol)
}

func (d OntapAPIZAPI) GetSVMAggregateNames(_ context.Context) ([]string, error) {
	return d.API.SVMGetAggregateNames()
}

// EmsAutosupportLog generates an auto support message with the supplied parameters
func (d OntapAPIZAPI) EmsAutosupportLog(
	ctx context.Context,
	driverName string,
	appVersion string,
	autoSupport bool,
	category string,
	computerName string,
	eventDescription string,
	eventID int,
	eventSource string,
	logLevel int) {

	emsResponse, err := d.API.EmsAutosupportLog(appVersion, autoSupport, category, computerName, eventDescription, eventID,
		eventSource, logLevel)

	if err = GetError(ctx, emsResponse, err); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"driver": driverName,
			"error":  err,
		}).Error("Error logging EMS message.")
	} else {
		Logc(ctx).WithField("driver", driverName).Debug("Logged EMS message.")
	}
}

func (d OntapAPIZAPI) GetSVMAggregateAttributes(_ context.Context) (aggrList map[string]string, err error) {
	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to inspect ONTAP backend: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	result, err := d.API.VserverShowAggrGetIterRequest()
	if err != nil {
		return
	}

	if zerr := NewZapiError(result.Result); !zerr.IsPassed() {
		err = zerr
		return
	}

	aggrList = make(map[string]string)

	if result.Result.AttributesListPtr != nil {
		for _, aggr := range result.Result.AttributesListPtr.ShowAggregatesPtr {
			aggrList[aggr.AggregateName()] = aggr.AggregateType()
		}
	}

	return aggrList, nil
}

func (d OntapAPIZAPI) ExportPolicyDestroy(ctx context.Context, policy string) (*APIResponse, error) {
	response, err := d.API.ExportPolicyDestroy(policy)
	if err = GetError(ctx, response, err); err != nil {
		err = fmt.Errorf("error deleting export policy: %v", err)
	}
	return APIResponsePassed, err
}

func (d OntapAPIZAPI) VolumeExists(ctx context.Context, volumeName string) (bool, error) {
	return d.API.VolumeExists(ctx, volumeName)
}

func (d OntapAPIZAPI) TieringPolicyValue(ctx context.Context) string {
	return d.API.TieringPolicyValue(ctx)
}

func (d OntapAPIZAPI) GetSVMAggregateSpace(ctx context.Context, aggregate string) ([]SVMAggregateSpace, error) {
	// lookup aggregate
	aggrSpaceResponse, aggrSpaceErr := d.API.AggrSpaceGetIterRequest(aggregate)
	if aggrSpaceErr != nil {
		return nil, aggrSpaceErr
	}

	var svmAggregateSpaceList []SVMAggregateSpace

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

			svmAggregateSpace := SVMAggregateSpace{
				size:      int64(aggrSpace.AggregateSize()),
				used:      int64(aggrSpace.UsedIncludingSnapshotReserve()),
				footprint: int64(aggrSpace.VolumeFootprints()),
			}

			svmAggregateSpaceList = append(svmAggregateSpaceList, svmAggregateSpace)
		}
	}

	return svmAggregateSpaceList, nil
}

func (d OntapAPIZAPI) VolumeDisableSnapshotDirectoryAccess(ctx context.Context, name string) (*APIResponse, error) {
	snapDirResponse, err := d.API.VolumeDisableSnapshotDirectoryAccess(name)
	if err = GetError(ctx, snapDirResponse, err); err != nil {
		return nil, fmt.Errorf("error disabling snapshot directory access: %v", err)
	}

	response := &APIResponse{
		status: "passed",
		reason: snapDirResponse.Result.ResultReasonAttr,
		errno:  snapDirResponse.Result.ResultErrnoAttr,
	}

	return response, nil
}

func (d OntapAPIZAPI) VolumeMount(ctx context.Context, name, junctionPath string) (*APIResponse, error) {

	mountResponse, err := d.API.VolumeMount(name, junctionPath)
	if err = GetError(ctx, mountResponse, err); err != nil {
		return nil, fmt.Errorf("error mounting volume to junction: %v", err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeRename(ctx context.Context, originalName, newName string) error {
	renameResponse, err := d.API.VolumeRename(originalName, newName)
	if err = GetError(ctx, renameResponse, err); err != nil {
		Logc(ctx).WithField("originalName", originalName).Errorf("renaming volume failed: %v", err)
		return fmt.Errorf("volume %s rename failed: %v", originalName, err)
	}

	return nil
}

func (d OntapAPIZAPI) VolumeSetComment(ctx context.Context, volumeNameInternal, volumeNameExternal, comment string) error {
	modifyCommentResponse, err := d.API.VolumeSetComment(ctx, volumeNameInternal, comment)
	if err = GetError(ctx, modifyCommentResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Modifying comment failed: %v", err)
		return fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return nil
}

func (d OntapAPIZAPI) ExportPolicyCreate(ctx context.Context, policy string) error {
	policyCreateResponse, err := d.API.ExportPolicyCreate(policy)
	if err != nil {
		err = fmt.Errorf("error creating export policy %s: %v", policy, err)
	}
	if zerr := NewZapiError(policyCreateResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.EDUPLICATEENTRY {
			Logc(ctx).WithField("exportPolicy", policy).Debug("Export policy already exists.")
		} else {
			err = fmt.Errorf("error creating export policy %s: %v", policy, zerr)
		}
	}
	return err
}

func (d OntapAPIZAPI) VolumeSize(_ context.Context, volumeName string) (int, error) {
	return d.API.VolumeSize(volumeName)
}

func (d OntapAPIZAPI) VolumeSetSize(ctx context.Context, name, newSize string) (*APIResponse, error) {
	volumeSetSizeResponse, err := d.API.VolumeSetSize(name, newSize)

	if volumeSetSizeResponse == nil {
		if err != nil {
			err = fmt.Errorf("volume resize failed for volume %s: %v", name, err)
		} else {
			err = fmt.Errorf("volume resize failed for volume %s: unexpected error", name)
		}
		return nil, err
	}

	if err = GetError(ctx, volumeSetSizeResponse.Result, err); err != nil {
		Logc(ctx).WithField("error", err).Error("Volume resize failed.")
		return nil, fmt.Errorf("volume resize failed")
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeModifyUnixPermissions(ctx context.Context, volumeNameInternal, volumeNameExternal,
	unixPermissions string) (*APIResponse, error) {
	modifyUnixPermResponse, err := d.API.VolumeModifyUnixPermissions(volumeNameInternal, unixPermissions)
	if err = GetError(ctx, modifyUnixPermResponse, err); err != nil {
		Logc(ctx).WithField("originalName", volumeNameExternal).Errorf("Could not import volume, "+
			"modifying unix permissions failed: %v", err)
		return nil, fmt.Errorf("volume %s modify failed: %v", volumeNameExternal, err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeListByPrefix(ctx context.Context, prefix string) (Volumes, *APIResponse, error) {
	// Get all volumes matching the storage prefix
	volumesResponse, err := d.API.VolumeGetAll(prefix)
	if err = GetError(ctx, volumesResponse, err); err != nil {
		return nil, nil, err
	}

	volumes := Volumes{}

	// Convert all volumes to VolumeExternal and write them to the channel
	if volumesResponse.Result.AttributesListPtr != nil {
		for _, volume := range volumesResponse.Result.AttributesListPtr.VolumeAttributesPtr {
			volumeInfo, err := VolumeInfoFromZapiAttrsHelper(&volume)
			if err != nil {
				return nil, nil, err
			}
			volumes = append(volumes, *volumeInfo)
		}
	}

	return volumes, APIResponsePassed, nil
}

func (d OntapAPIZAPI) ExportRuleCreate(ctx context.Context, policyName, desiredPolicyRule string) (*APIResponse, error) {
	ruleResponse, err := d.API.ExportRuleCreate(policyName, desiredPolicyRule,
		[]string{"nfs"}, []string{"any"}, []string{"any"}, []string{"any"})
	if err = GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error creating export rule: %v", err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"ClientMatch":  desiredPolicyRule,
		}).Error(err)
	}

	if ruleResponse == nil {
		return nil, fmt.Errorf("unexpected response")
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) ExportRuleDestroy(ctx context.Context, policyName string, ruleIndex int) (*APIResponse, error) {
	ruleResponse, err := d.API.ExportRuleDestroy(policyName, ruleIndex)
	if err = GetError(ctx, ruleResponse, err); err != nil {
		err = fmt.Errorf("error deleting export rule on policy %s at index %d; %v",
			policyName, ruleIndex, err)
		Logc(ctx).WithFields(log.Fields{
			"ExportPolicy": policyName,
			"RuleIndex":    ruleIndex,
		}).Error(err)
	}

	if ruleResponse == nil {
		return nil, fmt.Errorf("unexpected response")
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeModifyExportPolicy(ctx context.Context, volumeName, policyName string) (*APIResponse, error) {
	volumeModifyResponse, err := d.API.VolumeModifyExportPolicy(volumeName, policyName)
	if err = GetError(ctx, volumeModifyResponse, err); err != nil {
		err = fmt.Errorf("error updating export policy on volume %s: %v", volumeName, err)
		Logc(ctx).Error(err)
		return nil, err
	}

	if volumeModifyResponse == nil {
		return nil, fmt.Errorf("unexpected response")
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) ExportPolicyExists(ctx context.Context, policyName string) (bool, error) {
	policyGetResponse, err := d.API.ExportPolicyGet(policyName)
	if err != nil {
		err = fmt.Errorf("error getting export policy; %v", err)
		Logc(ctx).WithField("exportPolicy", policyName).Error(err)
		return false, err
	}
	if zerr := NewZapiError(policyGetResponse); !zerr.IsPassed() {
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

func (d OntapAPIZAPI) ExportRuleList(ctx context.Context, policyName string) (map[string]int, error) {

	ruleListResponse, err := d.API.ExportRuleGetIterRequest(policyName)
	if err = GetError(ctx, ruleListResponse, err); err != nil {
		return nil, fmt.Errorf("error listing export policy rules: %v", err)
	}
	rules := make(map[string]int)

	if ruleListResponse.Result.NumRecords() > 0 {
		rulesAttrList := ruleListResponse.Result.AttributesList()
		exportRuleList := rulesAttrList.ExportRuleInfo()
		for _, rule := range exportRuleList {
			rules[rule.ClientMatch()] = rule.RuleIndex()
		}
	}

	return rules, nil
}

func (d OntapAPIZAPI) SnapshotCreate(ctx context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {
	snapResponse, err := d.API.SnapshotCreate(snapshotName, sourceVolume)
	if err = GetError(ctx, snapResponse, err); err != nil {
		return nil, fmt.Errorf("error creating snapshot: %v", err)
	}
	return APIResponsePassed, nil
}

// probeForVolume polls for the ONTAP volume to appear, with backoff retry logic
func (d OntapAPIZAPI) probeForVolume(ctx context.Context, name string) error {

	checkVolumeExists := func() error {
		volExists, err := d.VolumeExists(ctx, name)
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

func (d OntapAPIZAPI) VolumeCloneCreate(ctx context.Context, cloneName, sourceName, snapshot string, async bool) (
	*APIResponse, error) {

	if async {
		cloneResponse, err := d.API.VolumeCloneCreateAsync(cloneName, sourceName, snapshot)
		err = d.API.WaitForAsyncResponse(ctx, cloneResponse, 120*time.Second)
		if err != nil {
			return nil, errors.New("waiting for async response failed")
		}
	} else {
		cloneResponse, err := d.API.VolumeCloneCreate(cloneName, sourceName, snapshot)
		if err != nil {
			return nil, fmt.Errorf("error creating clone: %v", err)
		}
		if zerr := NewZapiError(cloneResponse); !zerr.IsPassed() {

			if zerr.Code() == azgo.EOBJECTNOTFOUND {
				return nil, fmt.Errorf("snapshot %s does not exist in volume %s", snapshot, sourceName)
			} else if zerr.IsFailedToLoadJobError() {
				fields := log.Fields{
					"zerr": zerr,
				}
				Logc(ctx).WithFields(fields).Warn(
					"Problem encountered during the clone create operation, " +
						"attempting to verify the clone was actually created")
				if volumeLookupError := d.probeForVolume(ctx, sourceName); volumeLookupError != nil {
					return nil, volumeLookupError
				}
			} else {
				return nil, fmt.Errorf("error creating clone: %v", zerr)
			}
		}
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) SnapshotList(ctx context.Context, sourceVolume string) (Snapshots, *APIResponse, error) {
	snapListResponse, err := d.API.SnapshotList(sourceVolume)
	if err = GetError(ctx, snapListResponse, err); err != nil {
		return nil, nil, fmt.Errorf("error enumerating snapshots: %v", err)
	}

	snapshots := Snapshots{}

	if snapListResponse.Result.AttributesListPtr != nil {
		for _, snap := range snapListResponse.Result.AttributesListPtr.SnapshotInfoPtr {
			snapshots = append(snapshots, Snapshot{
				CreateTime: time.Unix(int64(snap.AccessTime()), 0).UTC().Format(storage.SnapshotTimestampFormat),
				Name:       snap.Name(),
			})
		}
	}

	Logc(ctx).Debugf("Returned %v snapshots.", len(snapshots))

	return snapshots, APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeSetQosPolicyGroupName(ctx context.Context, name string, qos QosPolicyGroup) (*APIResponse,
	error) {
	qosResponse, err := d.API.VolumeSetQosPolicyGroupName(name, qos)
	if err = GetError(ctx, qosResponse, err); err != nil {
		return nil, fmt.Errorf("error setting quality of service policy: %v", err)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeCloneSplitStart(ctx context.Context, cloneName string) (*APIResponse, error) {
	splitResponse, err := d.API.VolumeCloneSplitStart(cloneName)
	if err = GetError(ctx, splitResponse, err); err != nil {
		return nil, fmt.Errorf("error splitting clone: %v", err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) SnapshotRestoreVolume(ctx context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {

	snapResponse, err := d.API.SnapshotRestoreVolume(snapshotName, sourceVolume)

	if err = GetError(ctx, snapResponse, err); err != nil {
		return nil, fmt.Errorf("error restoring snapshot: %v", err)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) SnapshotDelete(_ context.Context, snapshotName, sourceVolume string) (*APIResponse, error) {
	snapResponse, err := d.API.SnapshotDelete(snapshotName, sourceVolume)

	if err != nil {
		return nil, fmt.Errorf("error deleting snapshot: %v", err)
	}
	if zerr := NewZapiError(snapResponse); !zerr.IsPassed() {
		if zerr.Code() == azgo.ESNAPSHOTBUSY {
			// Start a split here before returning the error so a subsequent delete attempt may succeed.
			return nil, SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName,
				sourceVolume))
		}
		return nil, fmt.Errorf("error deleting snapshot: %v", zerr)
	}
	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) VolumeListBySnapshotParent(ctx context.Context, snapshotName, sourceVolume string) (
	VolumeNameList, *APIResponse, error) {
	childVolumes, err := d.API.VolumeListAllBackedBySnapshot(ctx, sourceVolume, snapshotName)

	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshotName":     snapshotName,
			"parentVolumeName": sourceVolume,
			"error":            err,
		}).Error("Could not list volumes backed by snapshot.")
		return nil, nil, err
	} else if len(childVolumes) == 0 {
		return nil, nil, nil
	}

	// We're going to start a single split operation, but there could be multiple children, so we
	// sort the volumes by name to not have more than one split operation running at a time.
	sort.Strings(childVolumes)

	return childVolumes, APIResponsePassed, nil
}

func (d OntapAPIZAPI) SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName string) (*APIResponse, error) {
	snapDeleteResponse, err := d.API.SnapmirrorDeleteViaDestination(localFlexvolName, localSVMName)
	if err != nil && snapDeleteResponse != nil && snapDeleteResponse.Result.ResultErrnoAttr != azgo.EOBJECTNOTFOUND {
		return nil, fmt.Errorf("error deleting snapmirror info for volume %v: %v", localFlexvolName, err)
	}

	// Ensure no leftover snapmirror metadata
	err = d.API.SnapmirrorRelease(localFlexvolName, localSVMName)
	if err != nil {
		return nil, fmt.Errorf("error releasing snapmirror info for volume %v: %v", localFlexvolName, err)
	}

	return APIResponsePassed, nil
}

func (d OntapAPIZAPI) IsSVMDRCapable(ctx context.Context) (bool, error) {
	return d.API.IsVserverDRCapable(ctx)
}
