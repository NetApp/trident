// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"os/exec"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/azgo"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

// OntapSANStorageDriver is for iSCSI storage provisioning
type OntapSANStorageDriver struct {
	dvp.OntapSANStorageDriver
}

// Retrieve storage backend capabilities
func (d *OntapSANStorageDriver) GetStorageBackendSpecs(backend *storage.StorageBackend) error {

	backend.Name = "ontapsan_" + d.Config.DataLIF
	return getStorageBackendSpecsCommon(d, backend)
}

func (d *OntapSANStorageDriver) GetVolumeOpts(
	volConfig *storage.VolumeConfig,
	vc *storage.StoragePool,
	requests map[string]sa.Request,
) (map[string]string, error) {
	return getVolumeOptsCommon(volConfig, vc, requests), nil
}

func (d *OntapSANStorageDriver) GetInternalVolumeName(name string) string {
	return getInternalVolumeNameCommon(
		storage.GetCommonInternalVolumeName(&d.Config.CommonStorageDriverConfig,
			name),
	)
}

func (d *OntapSANStorageDriver) CreatePrepare(
	volConfig *storage.VolumeConfig,
) bool {
	// Sanitize the volume name
	volConfig.InternalName = d.GetInternalVolumeName(volConfig.Name)

	// Because the storage prefix specified in the backend config must create
	// a unique set of volume names, we do not need to check whether volumes
	// exist in the backend here.
	return true
}

func (d *OntapSANStorageDriver) CreateFollowup(volConfig *storage.VolumeConfig) error {
	return d.mapOntapSANLun(volConfig)
}

func (d *OntapSANStorageDriver) mapOntapSANLun(volConfig *storage.VolumeConfig) error {
	var (
		targetIQN                 string
		lunID                     int32
		acceptableLunMapResponses = map[string]bool{
			azgo.EVDISK_ERROR_INITGROUP_HAS_VDISK:  true, // LUN is already mapped to the igroup
			azgo.EVDISK_ERROR_INITGROUP_MAPS_EXIST: true, // TODO: bad naming by nDVP?
		}
	)

	response, err := d.API.IscsiServiceGetIterRequest()
	if response.Result.ResultStatusAttr != "passed" || err != nil {
		return fmt.Errorf("Problem retrieving iSCSI services: %v, %v",
			err, response.Result.ResultErrnoAttr)
	}
	for _, serviceInfo := range response.Result.AttributesList() {
		if serviceInfo.Vserver() == d.Config.SVM {
			targetIQN = serviceInfo.NodeName()
			log.WithFields(log.Fields{
				"volume":    volConfig.Name,
				"targetIQN": targetIQN,
			}).Debug("Successfully discovered target IQN for the volume.")
			break
		}
	}

	// Based on netappdvp/storage_drivers/ontap_san.go:
	// Map the newly created volume to the specified or default igroup
	// spin until we get a LUN ID that works
	// TODO: find one directly instead of spinning-and-looking for one:
	// (The signature for netappdvp/apis/ontap/ontap.go:LunMap() needs to change.)
	lunPath := fmt.Sprintf("/vol/%v/lun0", volConfig.InternalName)
	for i := 0; i < 4096; i++ {
		response, err := d.API.LunMap(d.Config.IgroupName, lunPath, i)
		if err != nil {
			return fmt.Errorf("Problem mapping lun: %v error: %v,%v",
				lunPath, err, response.Result.ResultErrnoAttr)
		}
		if response.Result.ResultStatusAttr == "passed" {
			lunID = int32(i)
			break
		}
		if acceptableLunMapResponses[response.Result.ResultErrnoAttr] {
			break
		} else if response.Result.ResultErrnoAttr ==
			azgo.EVDISK_ERROR_INITGROUP_HAS_LUN {
			// another LUN mapped with this LUN number
			continue
		} else {
			return fmt.Errorf("Problem mapping lun: %v error: %v,%v",
				lunPath, err, response.Result.ResultErrnoAttr)
		}
	}

	volConfig.AccessInfo.IscsiTargetPortal = d.Config.DataLIF
	volConfig.AccessInfo.IscsiTargetIQN = targetIQN
	volConfig.AccessInfo.IscsiLunNumber = lunID
	volConfig.AccessInfo.IscsiIgroup = d.Config.IgroupName
	log.WithFields(log.Fields{
		"volume":          volConfig.Name,
		"volume_internal": volConfig.InternalName,
		"targetIQN":       volConfig.AccessInfo.IscsiTargetIQN,
		"lunNumber":       volConfig.AccessInfo.IscsiLunNumber,
		"igroup":          volConfig.AccessInfo.IscsiIgroup,
	}).Debug("Successfully mapped ONTAP LUN.")

	return nil
}

func (d *OntapSANStorageDriver) GetProtocol() config.Protocol {
	return config.Block
}

func (d *OntapSANStorageDriver) GetDriverName() string {
	return d.Config.StorageDriverName
}

func (d *OntapSANStorageDriver) StoreConfig(
	b *storage.PersistentStorageBackendConfig,
) {
	storage.SanitizeCommonStorageDriverConfig(
		&d.Config.CommonStorageDriverConfig)
	b.OntapConfig = &d.Config
}

func (d *OntapSANStorageDriver) GetExternalConfig() interface{} {
	return getExternalConfig(d.Config)
}

func DiscoverIscsiTarget(targetIP string) error {
	cmd := exec.Command("sudo", "iscsiadm", "-m", "discoverydb", "-t", "st", "-p", targetIP, "--discover")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	} else {
		log.WithFields(log.Fields{
			"target":    targetIP,
			"targetIQN": string(out[:]),
		}).Info("Successful iSCSI target discovery.")
	}
	cmd = exec.Command("sudo", "iscsiadm", "-m", "node", "-p", targetIP, "--login")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}
