// Copyright 2018 NetApp, Inc. All Rights Reserved.

package solidfire

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	trident "github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/solidfire/api"
	"github.com/netapp/trident/utils"
)

const (
	sfDefaultVolTypeName = "SolidFire-default"
	sfDefaultMinIOPS     = 1000
	sfDefaultMaxIOPS     = 10000
)

const MinimumVolumeSizeBytes = 1000000000 // 1 GB

// SANStorageDriver is for iSCSI storage provisioning
type SANStorageDriver struct {
	initialized      bool
	Config           drivers.SolidfireStorageDriverConfig
	Client           *api.Client
	TenantID         int64
	AccessGroups     []int64
	LegacyNamePrefix string
	InitiatorIFace   string
	Telemetry        *Telemetry
}

type StorageDriverConfigExternal struct {
	*drivers.CommonStorageDriverConfigExternal
	TenantName     string
	EndPoint       string
	SVIP           string
	InitiatorIFace string //iface to use of iSCSI initiator
	Types          *[]api.VolType
	AccessGroups   []int64
	UseCHAP        bool
}

type Telemetry struct {
	trident.Telemetry
	Plugin string `json:"plugin"`
}

func parseQOS(qosOpt string) (qos api.QoS, err error) {
	iops := strings.Split(qosOpt, ",")
	qos.MinIOPS, err = strconv.ParseInt(iops[0], 10, 64)
	qos.MaxIOPS, err = strconv.ParseInt(iops[1], 10, 64)
	qos.BurstIOPS, err = strconv.ParseInt(iops[2], 10, 64)
	return qos, err
}

func parseType(vTypes []api.VolType, typeName string) (qos api.QoS, err error) {
	foundType := false
	for _, t := range vTypes {
		if strings.EqualFold(t.Type, typeName) {
			qos = t.QOS
			log.Infof("Received Type opts in Create and set QoS: %+v", qos)
			foundType = true
			break
		}
	}
	if foundType == false {
		log.Errorf("Specified type label not found: %v", typeName)
		err = errors.New("specified type not found")
	}
	return qos, err
}

// Name is for returning the name of this driver
func (d SANStorageDriver) Name() string {
	return drivers.SolidfireSANStorageDriverName
}

// Initialize from the provided config
func (d *SANStorageDriver) Initialize(
	context trident.DriverContext, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
) error {

	if commonConfig.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Initialize", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Initialize")
		defer log.WithFields(fields).Debug("<<<< Initialize")
	}

	commonConfig.DriverContext = context

	c := &drivers.SolidfireStorageDriverConfig{}
	c.CommonStorageDriverConfig = commonConfig

	// decode supplied configJSON string into SolidfireStorageDriverConfig object
	err := json.Unmarshal([]byte(configJSON), &c)
	if err != nil {
		return fmt.Errorf("could not decode JSON configuration: %v", err)
	}

	log.WithFields(log.Fields{
		"Version":           c.Version,
		"StorageDriverName": c.StorageDriverName,
		"DisableDelete":     c.DisableDelete,
	}).Debugf("Parsed into solidfireConfig")

	// SF prefix is always empty
	prefix := ""
	c.StoragePrefix = &prefix

	if context == trident.ContextDocker {
		if !c.UseCHAP {
			log.Info("Enabling CHAP for Docker volumes.")
			c.UseCHAP = true
		}
	}

	log.Debugf("Decoded to %+v", c)
	d.Config = *c

	var tenantID int64
	var defaultBlockSize int64
	defaultBlockSize = 512
	if c.DefaultBlockSize == 4096 {
		defaultBlockSize = 4096
	}
	log.WithField("defaultBlockSize", defaultBlockSize).Info("Set default block size for SolidFire volumes.")

	// create a new api.Config object from the read in json config file
	endpoint := c.EndPoint
	svip := c.SVIP
	cfg := api.Config{
		TenantName:       c.TenantName,
		EndPoint:         c.EndPoint,
		SVIP:             c.SVIP,
		InitiatorIFace:   c.InitiatorIFace,
		Types:            c.Types,
		LegacyNamePrefix: c.LegacyNamePrefix,
		AccessGroups:     c.AccessGroups,
		DefaultBlockSize: defaultBlockSize,
		DebugTraceFlags:  c.DebugTraceFlags,
	}
	defaultTenantName := c.TenantName

	log.WithFields(log.Fields{
		"endpoint":          endpoint,
		"svip":              svip,
		"cfg":               cfg,
		"defaultTenantName": defaultTenantName,
	}).Debug("About to call NewFromParameters")

	// create a new api.Client object for interacting with the SolidFire storage system
	client, _ := api.NewFromParameters(endpoint, svip, cfg, defaultTenantName)
	req := api.GetAccountByNameRequest{
		Name: c.TenantName,
	}

	// lookup the specified account; if not found, dynamically create it
	account, err := client.GetAccountByName(&req)
	if err != nil {
		log.WithFields(log.Fields{
			"tenantName": c.TenantName,
			"error":      err,
		}).Debug("Account not found, creating.")
		req := api.AddAccountRequest{
			Username: c.TenantName,
		}
		tenantID, err = client.AddAccount(&req)
		if err != nil {
			log.WithField("error", err).Error("Failed to initialize SolidFire driver while creating tenant.")
			return err
		}
	} else {
		log.WithFields(log.Fields{
			"tenantName": c.TenantName,
			"tenantID":   account.AccountID,
		}).Debug("Using existing account.")
		tenantID = account.AccountID
	}

	legacyNamePrefix := "netappdvp-"
	if c.LegacyNamePrefix != "" {
		legacyNamePrefix = c.LegacyNamePrefix
	}

	iscsiInterface := "default"
	if c.InitiatorIFace != "" {
		iscsiInterface = c.InitiatorIFace
	}

	if c.Types != nil {
		client.VolumeTypes = c.Types
	}

	if c.AccessGroups != nil {
		client.AccessGroups = c.AccessGroups
	}

	d.TenantID = tenantID
	d.Client = client
	d.InitiatorIFace = iscsiInterface
	d.LegacyNamePrefix = legacyNamePrefix
	log.WithFields(log.Fields{
		"TenantID":       tenantID,
		"InitiatorIFace": iscsiInterface,
	}).Debug("Driver initialized with the following settings")

	validationErr := d.validate()
	if validationErr != nil {
		log.Errorf("Problem validating SANStorageDriver error: %+v", validationErr)
		return errors.New("error encountered validating SolidFire driver on init")
	}

	// log cluster node serial numbers asynchronously since the API can take a long time
	go d.getNodeSerialNumbers(c.CommonStorageDriverConfig)

	d.Telemetry = &Telemetry{
		Telemetry: trident.OrchestratorTelemetry,
		Plugin:    d.Name(),
	}

	d.initialized = true
	return nil
}

func (d *SANStorageDriver) Initialized() bool {
	return d.initialized
}

func (d *SANStorageDriver) Terminate() {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Terminate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Terminate")
		defer log.WithFields(fields).Debug("<<<< Terminate")
	}

	d.initialized = false
}

func (d *SANStorageDriver) getNodeSerialNumbers(c *drivers.CommonStorageDriverConfig) {
	c.SerialNumbers = make([]string, 0, 0)
	hwInfo, err := d.Client.GetClusterHardwareInfo()
	if err != nil {
		log.Errorf("Unable to determine controller serial numbers: %+v ", err)
	} else {
		if nodes, ok := hwInfo.Nodes.(map[string]interface{}); ok {
			for _, node := range nodes {
				serialNumber, ok := node.(map[string]interface{})["serial"].(string)
				if ok && serialNumber != "" {
					c.SerialNumbers = append(c.SerialNumbers, serialNumber)
				}
			}
		}
	}
	log.WithFields(log.Fields{
		"serialNumbers": strings.Join(c.SerialNumbers, ","),
	}).Info("Controller serial numbers.")
}

// Validate the driver configuration and execution environment
func (d *SANStorageDriver) validate() error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "validate", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> validate")
		defer log.WithFields(fields).Debug("<<<< validate")
	}

	// We want to verify we have everything we need to run the Docker driver
	if d.Config.TenantName == "" {
		return errors.New("missing required TenantName in config")
	}
	if d.Config.EndPoint == "" {
		return errors.New("missing required EndPoint in config")
	}
	if d.Config.SVIP == "" {
		return errors.New("missing required SVIP in config")
	}

	if d.Config.DriverContext == trident.ContextDocker {
		// Validate the environment
		isIscsiSupported := utils.IscsiSupported()
		if !isIscsiSupported {
			log.Errorf("Host doesn't appear to support iSCSI.")
			return errors.New("no iSCSI support on this host")
		}
	}

	return nil
}

// Make SolidFire name
func MakeSolidFireName(name string) string {
	return strings.Replace(name, "_", "-", -1)
}

// Create a SolidFire volume
func (d *SANStorageDriver) Create(name string, sizeBytes uint64, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":    "Create",
			"Type":      "SANStorageDriver",
			"name":      name,
			"sizeBytes": sizeBytes,
			"opts":      opts,
		}
		log.WithFields(fields).Debug(">>>> Create")
		defer log.WithFields(fields).Debug("<<<< Create")
	}

	var req api.CreateVolumeRequest
	var qos api.QoS
	telemetry, _ := json.Marshal(d.Telemetry)
	var meta = map[string]string{
		"trident":     string(telemetry),
		"docker-name": name,
	}

	v, err := d.GetVolume(name)
	if err == nil && v.VolumeID != 0 {
		log.Warningf("found existing Volume by name: %s", name)
		return errors.New("volume with requested name already exists")
	}

	if sizeBytes < MinimumVolumeSizeBytes {
		return fmt.Errorf("requested volume size (%d bytes) is too small; the minimum volume size is %d bytes",
			sizeBytes, MinimumVolumeSizeBytes)
	}

	qosOpt := utils.GetV(opts, "qos", "")
	if qosOpt != "" {
		qos, err = parseQOS(qosOpt)
		if err != nil {
			return err
		}
	}

	typeOpt := utils.GetV(opts, "type", "")
	if typeOpt != "" {
		if qos.MinIOPS != 0 {
			log.Warningf("QoS values appear to have been set using -o qos, but " +
				"type is set as well, overriding with type option.")
		}
		qos, err = parseType(*d.Client.VolumeTypes, typeOpt)
		if err != nil {
			return err
		}
	}

	// Use whatever is set in the config as default
	if d.Client.DefaultBlockSize == 4096 {
		req.Enable512e = false
	} else {
		req.Enable512e = true
	}

	// Now check if they specified a block size and use it if they did
	blockSizeOpt := utils.GetV(opts, "blocksize", "")
	if blockSizeOpt != "" {
		if blockSizeOpt == "4096" {
			req.Enable512e = false
		} else {
			req.Enable512e = true
		}
	}
	log.WithFields(log.Fields{
		"blocksize":  blockSizeOpt,
		"enable512e": req.Enable512e,
	}).Debug("Parsed blocksize option.")

	// Check for a supported file system type
	fstype := strings.ToLower(utils.GetV(opts, "fstype|fileSystemType", "ext4"))
	switch fstype {
	case "xfs", "ext3", "ext4":
		log.WithFields(log.Fields{"fileSystemType": fstype, "name": name}).Debug("Filesystem format.")
		meta["fstype"] = fstype
	default:
		return fmt.Errorf("unsupported fileSystemType option: %s", fstype)
	}

	req.Qos = qos
	req.TotalSize = int64(sizeBytes)
	req.AccountID = d.TenantID
	req.Name = MakeSolidFireName(name)
	req.Attributes = meta
	_, err = d.Client.CreateVolume(&req)
	if err != nil {
		return err
	}
	return nil
}

// Create a volume clone
func (d *SANStorageDriver) CreateClone(name, source, snapshot string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":   "CreateClone",
			"Type":     "SANStorageDriver",
			"name":     name,
			"source":   source,
			"snapshot": snapshot,
			"opts":     opts,
		}
		log.WithFields(fields).Debug(">>>> CreateClone")
		defer log.WithFields(fields).Debug("<<<< CreateClone")
	}

	var req api.CloneVolumeRequest
	telemetry, _ := json.Marshal(d.Telemetry)
	var meta = map[string]string{
		"trident":     string(telemetry),
		"docker-name": name,
	}

	// Check to see if the clone already exists
	v, err := d.GetVolume(name)
	if err == nil && v.VolumeID != 0 {
		log.Warningf("found existing Volume by name: %s", name)
		return errors.New("volume with requested name already exists")
	}

	// Get the volume ID for the source volume
	v, err = d.GetVolume(source)
	if err != nil || v.VolumeID == 0 {
		log.Errorf("Unable to locate requested source volume: %+v", err)
		return errors.New("error performing clone operation, source volume not found")
	}

	// If a snapshot was specified, use that
	if snapshot != "" {
		s, err := d.Client.GetSnapshot(0, v.VolumeID, snapshot)
		if err != nil || s.SnapshotID == 0 {
			log.Errorf("Unable to locate requested source snapshot: %+v", err)
			return errors.New("error performing clone operation, source snapshot not found")
		}
		req.SnapshotID = s.SnapshotID
	}

	// Create the clone of the source volume with the name specified
	req.VolumeID = v.VolumeID
	req.Name = MakeSolidFireName(name)
	req.Attributes = meta
	vol, err := d.Client.CloneVolume(&req)
	if err != nil {
		log.Errorf("Failed to create clone: %+v", err)
		return errors.New("error performing clone operation")
	}

	var modifyReq api.ModifyVolumeRequest
	var qos api.QoS
	modifyReq.VolumeID = vol.VolumeID
	doModify := false

	qosOpt := utils.GetV(opts, "qos", "")
	if qosOpt != "" {
		doModify = true
		qos, err = parseQOS(qosOpt)
		if err != nil {
			return err
		}
	}

	typeOpt := utils.GetV(opts, "type", "")
	if typeOpt != "" {
		doModify = true
		if qos.MinIOPS != 0 {
			log.Warningf("qos values appear to have been set using -o qos, but type is set as well, " +
				"overriding with type option")
		}
		qos, err = parseType(*d.Client.VolumeTypes, typeOpt)
		if err != nil {
			return err
		}
	}

	if doModify {
		modifyReq.Qos = qos
		err = d.Client.ModifyVolume(&modifyReq)
		if err != nil {
			log.Errorf("Failed to update QoS on clone: %+v", err)
			return errors.New("error performing clone operation")
		}
	}
	return nil
}

// Destroy the requested docker volume
func (d *SANStorageDriver) Destroy(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "Destroy",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> Destroy")
		defer log.WithFields(fields).Debug("<<<< Destroy")
	}

	v, err := d.GetVolume(name)
	if err != nil && err.Error() != "volume not found" {
		log.Errorf("Unable to locate volume for delete operation: %+v", err)
		return err
	} else if err != nil {
		// Volume wasn't found. No action needs to be taken.
		log.Warnf("volume doesn't exist")
		return nil
	}
	d.Client.DetachVolume(v)
	err = d.Client.DeleteVolume(v.VolumeID)
	if err != nil {
		log.Errorf("Error during delete operation: %+v", err)
		return err
	}

	return nil
}

// Attach the lun
func (d *SANStorageDriver) Attach(name, mountpoint string, opts map[string]string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Attach",
			"Type":       "SANStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
			"opts":       opts,
		}
		log.WithFields(fields).Debug(">>>> Attach")
		defer log.WithFields(fields).Debug("<<<< Attach")
	}

	v, err := d.GetVolume(name)
	if err != nil {
		log.Errorf("Unable to locate volume for mount operation: %+v", err)
		return errors.New("volume not found")
	}
	path, device, err := d.Client.AttachVolume(&v, d.InitiatorIFace)
	if path == "" || device == "" && err == nil {
		log.Errorf("Path not found on attach: (path: %s, device: %s)", path, device)
		return errors.New("path not found")
	}
	if err != nil {
		log.Errorf("Error on iSCSI attach: %+v", err)
		return errors.New("iSCSI attach error")
	}
	log.Debugf("Attached volume: (path: %s, device: %s)", path, device)

	// NOTE(jdg): Check for device including multipath/DM device)
	info, err := utils.GetDeviceInfoForLuns()
	for _, e := range info {
		if e.Device != device {
			continue
		}
		if e.MultipathDevice != "" {
			device = e.MultipathDevice
		}
	}

	// Get the fstype
	attrs, _ := v.Attributes.(map[string]interface{})
	fstype := "ext4"
	if str, ok := attrs["fstype"].(string); ok {
		fstype = str
	}

	// Put a filesystem on it if there isn't one already there
	existingFstype := utils.GetFSType(device)
	if existingFstype == "" {
		log.WithFields(log.Fields{"LUN": path, "fstype": fstype}).Debug("Formatting LUN.")
		err := utils.FormatVolume(device, fstype)
		if err != nil {
			log.Errorf("Error on formatting volume: %+v", err)
			return errors.New("format (mkfs) error")
		}
	} else if existingFstype != fstype {
		log.WithFields(log.Fields{
			"LUN":             path,
			"existingFstype":  existingFstype,
			"requestedFstype": fstype,
		}).Warn("LUN already formatted with a different file system type.")
	} else {
		log.WithFields(log.Fields{"LUN": path, "fstype": existingFstype}).Debug("LUN already formatted.")
	}

	if mountErr := utils.Mount(device, mountpoint); mountErr != nil {
		log.Errorf("Unable to mount device: (device: %s, mountpoint: %s, error: %+v", device, mountpoint, err)
		return errors.New("unable to mount device")
	}

	return nil
}

// Detach the volume
func (d *SANStorageDriver) Detach(name, mountpoint string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method":     "Detach",
			"Type":       "SANStorageDriver",
			"name":       name,
			"mountpoint": mountpoint,
		}
		log.WithFields(fields).Debug(">>>> Detach")
		defer log.WithFields(fields).Debug("<<<< Detach")
	}

	umountErr := utils.Umount(mountpoint)
	if umountErr != nil {
		log.Errorf("Unable to unmount device: (name: %s, mountpoint: %s, error: %+v", name, mountpoint, umountErr)
		return errors.New("unable to unmount device")
	}

	v, err := d.GetVolume(name)
	if err != nil {
		log.WithField("volume", v).Errorf("Unable to locate volume: %+v", err)
		return errors.New("volume not found")
	}
	// no longer detaching and removing iSCSI session here because it was causing issues with 'docker cp'
	// see also;  https://github.com/moby/moby/issues/34665
	//d.Client.DetachVolume(v)

	return nil
}

// List of volumes according to backend device
func (d *SANStorageDriver) List() (vols []string, err error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "List", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> List")
		defer log.WithFields(fields).Debug("<<<< List")
	}

	var req api.ListVolumesForAccountRequest
	req.AccountID = d.TenantID
	volumes, err := d.Client.ListVolumesForAccount(&req)
	for _, v := range volumes {
		if v.Status != "deleted" {
			attrs, _ := v.Attributes.(map[string]interface{})
			dName := strings.Replace(v.Name, d.LegacyNamePrefix, "", -1)
			if str, ok := attrs["docker-name"].(string); ok {
				dName = strings.Replace(str, d.LegacyNamePrefix, "", -1)
			}
			vols = append(vols, dName)
		}
	}
	return vols, err
}

// SnapshotList returns the list of snapshots associated with the named volume
func (d *SANStorageDriver) SnapshotList(name string) ([]storage.Snapshot, error) {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{
			"Method": "SnapshotList",
			"Type":   "SANStorageDriver",
			"name":   name,
		}
		log.WithFields(fields).Debug(">>>> SnapshotList")
		defer log.WithFields(fields).Debug("<<<< SnapshotList")
	}

	v, err := d.GetVolume(name)
	if err != nil {
		log.Errorf("Unable to locate parent volume in snapshot list: %+v", err)
		return nil, errors.New("volume not found")
	}

	var req api.ListSnapshotsRequest
	req.VolumeID = v.VolumeID

	s, err := d.Client.ListSnapshots(&req)
	if err != nil {
		log.Errorf("Unable to locate snapshot: %+v", err)
		return nil, errors.New("snapshot not found")
	}

	log.Debugf("Returned %d snapshots", len(s))
	var snapshots []storage.Snapshot

	for _, snap := range s {
		log.Debugf("Snapshot name: %s, date: %s", snap.Name, snap.CreateTime)
		snapshots = append(snapshots, storage.Snapshot{Name: snap.Name, Created: snap.CreateTime})
	}

	return snapshots, nil
}

// Get tests for the existence of a volume
func (d *SANStorageDriver) Get(name string) error {

	if d.Config.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "Get", "Type": "SANStorageDriver"}
		log.WithFields(fields).Debug(">>>> Get")
		defer log.WithFields(fields).Debug("<<<< Get")
	}

	_, err := d.GetVolume(name)
	return err
}

// getVolumes returns all volumes for the configured tenant.  The
// keys are the volume names as reported to Docker.
func (d *SANStorageDriver) getVolumes() (map[string]api.Volume, error) {
	var req api.ListVolumesForAccountRequest
	req.AccountID = d.TenantID
	volumes, err := d.Client.ListVolumesForAccount(&req)
	if err != nil {
		return nil, err
	}

	volMap := make(map[string]api.Volume)
	for _, volume := range volumes {
		if volume.Status != "deleted" {
			attrs, _ := volume.Attributes.(map[string]interface{})
			dName := strings.Replace(volume.Name, d.LegacyNamePrefix, "", -1)
			if str, ok := attrs["docker-name"].(string); ok {
				dName = strings.Replace(str, d.LegacyNamePrefix, "", -1)
			}
			volMap[dName] = volume
		}
	}
	return volMap, nil
}

func (d *SANStorageDriver) GetVolume(name string) (api.Volume, error) {
	var vols []api.Volume
	var req api.ListVolumesForAccountRequest

	// I know, I know... just use V8 of the API and let the Cluster filter on
	// things like Name; trouble is we completely screwed up Name usage so we
	// can't trust it.  We now have a few possibilities including Name,
	// Name-With-Prefix and Attributes.  It could be any of the 3.  At some
	// point let's fix that and just use something efficient like Name and be
	// done with it. Otherwise, we just get all for the account and iterate
	// which isn't terrible.
	req.AccountID = d.TenantID
	volumes, err := d.Client.ListVolumesForAccount(&req)
	if err != nil {
		log.Errorf("Error encountered requesting volumes in SolidFire:getVolume: %+v", err)
		return api.Volume{}, errors.New("device reported API error")
	}

	legacyName := MakeSolidFireName(d.LegacyNamePrefix + name)
	baseSFName := MakeSolidFireName(name)

	for _, v := range volumes {
		attrs, _ := v.Attributes.(map[string]interface{})
		// We prefer attributes, so check that first, then pick up legacy
		// volumes using Volume Name
		if attrs["docker-name"] == name && v.Status == "active" {
			log.Debugf("Found volume by attributes: %+v", v)
			vols = append(vols, v)
		} else if (v.Name == legacyName || v.Name == baseSFName) && v.Status == "active" {
			log.Warningf("Found volume by name using deprecated Volume.Name mapping: %+v", v)
			vols = append(vols, v)
		}
	}
	if len(vols) == 0 {
		return api.Volume{}, errors.New("volume not found")
	}
	return vols[0], nil
}

// GetStorageBackendSpecs retrieves storage backend capabilities
func (d *SANStorageDriver) GetStorageBackendSpecs(backend *storage.Backend) error {

	backend.Name = "solidfire_" + strings.Split(d.Config.SVIP, ":")[0]

	volTypes := *d.Client.VolumeTypes
	if len(volTypes) == 0 {
		volTypes = []api.VolType{
			{
				Type: sfDefaultVolTypeName,
				QOS: api.QoS{
					MinIOPS: sfDefaultMinIOPS,
					MaxIOPS: sfDefaultMaxIOPS,
					// Leave burst IOPS blank, since we don't do anything with
					// it for storage classes.
				},
			},
		}
	}
	for _, volType := range volTypes {
		pool := storage.NewStoragePool(backend, volType.Type)

		pool.Attributes[sa.Media] = sa.NewStringOffer(sa.SSD)
		pool.Attributes[sa.IOPS] = sa.NewIntOffer(int(volType.QOS.MinIOPS),
			int(volType.QOS.MaxIOPS))
		pool.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Clones] = sa.NewBoolOffer(true)
		pool.Attributes[sa.Encryption] = sa.NewBoolOffer(false)
		pool.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thin")
		pool.Attributes[sa.BackendType] = sa.NewStringOffer(d.Name())
		backend.AddStoragePool(pool)

		log.WithFields(log.Fields{
			"attributes": fmt.Sprintf("%+v", pool.Attributes),
			"pool":       pool.Name,
			"backend":    backend.Name,
		}).Debug("Added pool for SolidFire backend.")
	}

	return nil
}

func (d *SANStorageDriver) GetInternalVolumeName(name string) string {

	if trident.UsingPassthroughStore {
		return strings.Replace(name, "_", "-", -1)
	} else {
		internal := drivers.GetCommonInternalVolumeName(d.Config.CommonStorageDriverConfig, name)
		internal = strings.Replace(internal, "_", "-", -1)
		internal = strings.Replace(internal, ".", "-", -1)
		internal = strings.Replace(internal, "--", "-", -1) // Remove any double hyphens
		return internal
	}
}

func (d *SANStorageDriver) CreatePrepare(volConfig *storage.VolumeConfig) bool {

	// 1. Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	if volConfig.CloneSourceVolume != "" {
		volConfig.CloneSourceVolumeInternal =
			d.GetInternalVolumeName(volConfig.CloneSourceVolume)
	}

	return true
}

func (d *SANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {
	return d.mapSolidfireLun(volConfig)
}

func (d *SANStorageDriver) mapSolidfireLun(volConfig *storage.VolumeConfig) error {
	// Add the newly created volume to the default VAG
	name := volConfig.InternalName
	v, err := d.GetVolume(name)
	if err != nil {
		return fmt.Errorf("could not find SolidFire volume %s: %s", name, err.Error())
	}

	// start volConfig...
	volConfig.AccessInfo.IscsiTargetPortal = d.Config.SVIP
	volConfig.AccessInfo.IscsiTargetIQN = v.Iqn
	volConfig.AccessInfo.IscsiLunNumber = 0
	volConfig.AccessInfo.IscsiInterface = d.Config.InitiatorIFace

	if d.Config.UseCHAP {
		var req api.GetAccountByIDRequest
		req.AccountID = v.AccountID
		a, err := d.Client.GetAccountByID(&req)
		if err != nil {
			return fmt.Errorf("could not lookup SolidFire account ID %v, error: %+v ", v.AccountID, err)
		}

		// finish volConfig
		volConfig.AccessInfo.IscsiUsername = a.Username
		volConfig.AccessInfo.IscsiInitiatorSecret = a.InitiatorSecret
		volConfig.AccessInfo.IscsiTargetSecret = a.TargetSecret
	} else {

		volumeIDList := []int64{v.VolumeID}
		for _, vagID := range d.Config.AccessGroups {
			req := api.AddVolumesToVolumeAccessGroupRequest{
				VolumeAccessGroupID: vagID,
				Volumes:             volumeIDList,
			}

			err = d.Client.AddVolumesToAccessGroup(&req)
			if err != nil {
				return fmt.Errorf("could not map SolidFire volume %s to the VAG: %s", name, err.Error())
			}
		}

		// finish volConfig
		volConfig.AccessInfo.IscsiVAGs = d.Config.AccessGroups
	}

	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"interface":       volConfig.AccessInfo.IscsiInterface,
		"VAGs":            volConfig.AccessInfo.IscsiVAGs,
		"Username":        volConfig.AccessInfo.IscsiUsername,
		"InitiatorSecret": volConfig.AccessInfo.IscsiInitiatorSecret,
		"TargetSecret":    volConfig.AccessInfo.IscsiTargetSecret,
		"UseCHAP":         d.Config.UseCHAP,
	}).Info("Successfully mapped SolidFire LUN.")

	return nil
}

func (d *SANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	pool *storage.Pool,
	requests map[string]sa.Request,
) (map[string]string, error) {

	opts := make(map[string]string)

	if volConfig.FileSystem != "" {
		opts["fileSystemType"] = volConfig.FileSystem
	}
	if pool != nil {
		opts["type"] = pool.Name
	}
	if volConfig.BlockSize != "" {
		opts["blocksize"] = volConfig.BlockSize
	}
	if volConfig.QoS != "" {
		opts["qos"] = volConfig.QoS
	}

	if volConfig.QoSType != "" {
		opts["type"] = volConfig.QoSType
	} else if volConfig.QoS != "" {
		opts["type"] = ""
	} else {
		// Default to "pool" name only if we aren't cloning
		if volConfig.CloneSourceVolume != "" {
			opts["type"] = ""
		} else {
			opts["type"] = pool.Name
		}
	}

	return opts, nil
}

func (d *SANStorageDriver) GetProtocol() trident.Protocol {
	return trident.Block
}

func (d *SANStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	drivers.SanitizeCommonStorageDriverConfig(d.Config.CommonStorageDriverConfig)
	b.SolidfireConfig = &d.Config
}

func (d *SANStorageDriver) GetExternalConfig() interface{} {
	endpointHalves := strings.Split(d.Config.EndPoint, "@")
	return &StorageDriverConfigExternal{
		CommonStorageDriverConfigExternal: drivers.GetCommonStorageDriverConfigExternal(
			d.Config.CommonStorageDriverConfig),
		TenantName:     d.Config.TenantName,
		EndPoint:       fmt.Sprintf("https://%s", endpointHalves[1]),
		SVIP:           d.Config.SVIP,
		InitiatorIFace: d.Config.InitiatorIFace,
		Types:          d.Config.Types,
		AccessGroups:   d.Config.AccessGroups,
		UseCHAP:        d.Config.UseCHAP,
	}
}

// Find/Return items that exist in b but NOT a
func diffSlices(a, b []int64) []int64 {
	var r []int64
	m := make(map[int64]bool)
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			r = append(r, s)
		}
	}
	return r
}

// VerifyVags verifies that the provided list of VAG ID's exist, return list of those that don't
func (d *SANStorageDriver) VerifyVags(vags []int64) ([]int64, error) {
	var vagIDs []int64

	discovered, err := d.Client.ListVolumeAccessGroups(&api.ListVolumeAccessGroupsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve VAGs from SolidFire backend: %+v", err)
	}

	for _, v := range discovered {
		vagIDs = append(vagIDs, v.VAGID)
	}
	return diffSlices(vagIDs, vags), nil
}

// AddMissingVolumesToVag adds volume ID's in the provided list that aren't already a member of the specified VAG
func (d *SANStorageDriver) AddMissingVolumesToVag(vagID int64, vols []int64) error {
	var req api.ListVolumeAccessGroupsRequest
	req.StartVAGID = vagID
	req.Limit = 1

	vags, _ := d.Client.ListVolumeAccessGroups(&req)
	missingVolIDs := diffSlices(vags[0].Volumes, vols)

	var addReq api.AddVolumesToVolumeAccessGroupRequest
	addReq.VolumeAccessGroupID = vagID
	addReq.Volumes = missingVolIDs

	return d.Client.AddVolumesToAccessGroup(&addReq)
}

// GetVolumeExternal queries the storage backend for all relevant info about
// a single container volume managed by this driver and returns a VolumeExternal
// representation of the volume.
func (d *SANStorageDriver) GetVolumeExternal(name string) (*storage.VolumeExternal, error) {

	volume, err := d.GetVolume(name)
	if err != nil {
		return nil, err
	}

	return d.getVolumeExternal(name, &volume), nil
}

// GetVolumeExternalWrappers queries the storage backend for all relevant info about
// container volumes managed by this driver.  It then writes a VolumeExternal
// representation of each volume to the supplied channel, closing the channel
// when finished.
func (d *SANStorageDriver) GetVolumeExternalWrappers(
	channel chan *storage.VolumeExternalWrapper) {

	// Let the caller know we're done by closing the channel
	defer close(channel)

	// Get all volumes
	volumes, err := d.getVolumes()
	if err != nil {
		channel <- &storage.VolumeExternalWrapper{nil, err}
		return
	}

	// Convert all volumes to VolumeExternal and write them to the channel
	for externalName, volume := range volumes {
		channel <- &storage.VolumeExternalWrapper{d.getVolumeExternal(externalName, &volume), nil}
	}
}

// getExternalVolume is a private method that accepts info about a volume
// as returned by the storage backend and formats it as a VolumeExternal
// object.
func (d *SANStorageDriver) getVolumeExternal(
	externalName string, volumeAttrs *api.Volume) *storage.VolumeExternal {

	volumeConfig := &storage.VolumeConfig{
		Version:         trident.OrchestratorAPIVersion,
		Name:            externalName,
		InternalName:    string(volumeAttrs.Name),
		Size:            strconv.FormatInt(volumeAttrs.TotalSize, 10),
		Protocol:        trident.Block,
		SnapshotPolicy:  "",
		ExportPolicy:    "",
		SnapshotDir:     "false",
		UnixPermissions: "",
		StorageClass:    "",
		AccessMode:      trident.ReadWriteOnce,
		AccessInfo:      storage.VolumeAccessInfo{},
		BlockSize:       strconv.FormatInt(volumeAttrs.BlockSize, 10),
		FileSystem:      "",
	}

	return &storage.VolumeExternal{
		Config: volumeConfig,
		Pool:   drivers.UnsetPool,
	}
}
