/*
 * Copyright 2018 NetApp, Inc. All Rights Reserved.
 */

package factory

import (
	"fmt"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/eseries"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/ontap"
	ontapi "github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/solidfire"
	sfapi "github.com/netapp/trident/storage_drivers/solidfire/api"
)

func NewStorageBackendForConfig(configJSON string) (sb *storage.Backend, err error) {

	var storageDriver storage.Driver

	// Some drivers may panic during initialize if given invalid parameters,
	// so catch any panics that might occur and return an error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("unable to instantiate backend: %v", r)
		}
	}()

	// Convert config (JSON or YAML) to JSON
	configJSONBytes, err := yaml.YAMLToJSON([]byte(configJSON))
	if err != nil {
		err = fmt.Errorf("invalid config format: %v", err)
		return
	}
	configJSON = string(configJSONBytes)

	// Parse the common config struct from JSON
	commonConfig, err := drivers.ValidateCommonSettings(configJSON)
	if err != nil {
		err = fmt.Errorf("input failed validation: %v", err)
		return
	}

	// Pre-driver initialization setup
	switch commonConfig.StorageDriverName {
	case drivers.OntapNASStorageDriverName:
		storageDriver = &ontap.NASStorageDriver{}
	case drivers.OntapNASQtreeStorageDriverName:
		storageDriver = &ontap.NASQtreeStorageDriver{}
	case drivers.OntapSANStorageDriverName:
		storageDriver = &ontap.SANStorageDriver{}
	case drivers.SolidfireSANStorageDriverName:
		storageDriver = &solidfire.SANStorageDriver{}
	case drivers.EseriesIscsiStorageDriverName:
		storageDriver = &eseries.SANStorageDriver{}
	case drivers.FakeStorageDriverName:
		storageDriver = &fake.StorageDriver{}
	default:
		err = fmt.Errorf("unknown storage driver: %v",
			commonConfig.StorageDriverName)
		return
	}

	if initializeErr := storageDriver.Initialize(
		config.CurrentDriverContext, configJSON, commonConfig); initializeErr != nil {
		err = fmt.Errorf("problem initializing storage driver: '%v' error: %v",
			commonConfig.StorageDriverName, initializeErr)
		return
	}

	// Post-driver initialization setup
	switch commonConfig.StorageDriverName {
	case drivers.OntapNASStorageDriverName:
		break

	case drivers.OntapNASQtreeStorageDriverName:
		break

	case drivers.OntapSANStorageDriverName:
		driver := storageDriver.(*ontap.SANStorageDriver)

		iGroupResponse, err := driver.API.IgroupList()
		if err = ontapi.GetError(iGroupResponse, err); err != nil {
			return nil, err
		}

		found := false
		initiators := ""
		for _, igroupInfo := range iGroupResponse.Result.AttributesList() {
			if igroupInfo.Vserver() == driver.Config.SVM &&
				igroupInfo.InitiatorGroupName() == driver.Config.IgroupName {
				found = true
				initiatorList := igroupInfo.Initiators()
				for _, initiator := range initiatorList {
					initiators = initiators + initiator.InitiatorName() + ","
				}
				initiators = strings.TrimSuffix(initiators, ",")
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("initiator group %v doesn't exist for SVM %v and needs to be manually created"+
				"; please also ensure all relevant hosts are added to the igroup", driver.Config.IgroupName, driver.Config.SVM)
		} else {
			log.WithFields(log.Fields{
				"driver":     drivers.OntapSANStorageDriverName,
				"SVM":        driver.Config.SVM,
				"igroup":     driver.Config.IgroupName,
				"initiators": initiators,
			}).Warn("Please ensure all relevant hosts are added to the initiator group.")
		}
	case drivers.SolidfireSANStorageDriverName:
		driver := storageDriver.(*solidfire.SANStorageDriver)

		if !driver.Config.UseCHAP {
			// VolumeAccessGroup logic

			// If zero AccessGroups are specified it could be that this is an upgrade where we
			// just utilize the default 'trident' group automatically.  Or, perhaps the deployment
			// doesn't need more than one set of 64 initiators, so we'll just use the old way of
			// doing it here, and look for/set the default group.
			if len(driver.Config.AccessGroups) == 0 {
				// We're going to do some hacky stuff here and make sure that if this is an upgrade
				// that we verify that one of the AccessGroups in the list is the default Trident VAG ID
				listVAGReq := &sfapi.ListVolumeAccessGroupsRequest{
					StartVAGID: 0,
					Limit:      0,
				}
				vags, vagErr := driver.Client.ListVolumeAccessGroups(listVAGReq)
				if vagErr != nil {
					err = fmt.Errorf("could not list VAGs for backend %s: %s",
						driver.Config.SVIP, vagErr.Error())
					return
				}

				found := false
				initiators := ""
				for _, vag := range vags {
					//TODO: SolidFire backend config should support taking VAG as an arg
					if vag.Name == config.DefaultSolidFireVAG {
						driver.Config.AccessGroups = append(driver.Config.AccessGroups, vag.VAGID)
						found = true
						for _, initiator := range vag.Initiators {
							initiators = initiators + initiator + ","
						}
						initiators = strings.TrimSuffix(initiators, ",")
						log.Infof("no AccessGroup ID's configured, using the default group: %v, "+
							"with initiators: %+v", vag.Name, initiators)
						break
					}
				}
				if !found {
					err = fmt.Errorf("volume Access Group %v doesn't exist at %v and needs to be manually created"+
						"; please also ensure all relevant hosts are added to the VAG", config.DefaultSolidFireVAG, driver.Config.SVIP)
					return
				}
			} else if len(driver.Config.AccessGroups) > 4 {
				err = fmt.Errorf("the maximum number of allowed Volume Access Groups per config is 4 but your config"+
					" has specified %v", len(driver.Config.AccessGroups))
				return
			} else {
				// We only need this in the case that AccessGroups were specified, if it was zero and we
				// used the default we already verified it in that step so we're good here.
				var missingVags []int64
				missingVags, err = driver.VerifyVags(driver.Config.AccessGroups)
				if err != nil {
					return
				}
				if len(missingVags) != 0 {
					err = fmt.Errorf("failed to discover the following specified VAG ID's: %+v", missingVags)
					return
				}
			}

			log.WithFields(log.Fields{
				"driver":       drivers.SolidfireSANStorageDriverName,
				"SVIP":         driver.Config.SVIP,
				"AccessGroups": driver.Config.AccessGroups,
				"UseCHAP":      driver.Config.UseCHAP,
			}).Warn("Please ensure all relevant hosts are added to one of ",
				"the specified Volume Access Groups.")

			// Deal with upgrades for versions prior to handling multiple VAG ID's
			var vIDs []int64
			var req sfapi.ListVolumesForAccountRequest
			req.AccountID = driver.TenantID
			volumes, _ := driver.Client.ListVolumesForAccount(&req)
			for _, v := range volumes {
				if v.Status != "deleted" {
					vIDs = append(vIDs, v.VolumeID)
				}
			}
			for _, vag := range driver.Config.AccessGroups {
				addAGErr := driver.AddMissingVolumesToVag(vag, vIDs)
				if addAGErr != nil {
					err = fmt.Errorf("failed to update AccessGroup membership of volume %+v", addAGErr)
					return
				}
			}
		} else {
			// CHAP logic
			log.WithFields(log.Fields{
				"driver":  drivers.SolidfireSANStorageDriverName,
				"SVIP":    driver.Config.SVIP,
				"UseCHAP": driver.Config.UseCHAP,
			}).Warn("Using CHAP, skipping Volume Access Group logic")
		}

	case drivers.EseriesIscsiStorageDriverName:
		driver := storageDriver.(*eseries.SANStorageDriver)

		// Make sure the Trident Host Group exists
		hostGroup, err := driver.API.GetHostGroup(driver.Config.AccessGroup)
		if err != nil {
			return nil, err
		} else if hostGroup.ClusterRef == "" {
			return nil, fmt.Errorf("host Group %s doesn't exist for E-Series array %s and needs to be manually"+
				" created; please also ensure all relevant Hosts are defined on the array and added to the Host Group",
				driver.Config.AccessGroup, driver.Config.ControllerA)
		} else {
			log.WithFields(log.Fields{
				"driver":     drivers.EseriesIscsiStorageDriverName,
				"controller": driver.Config.ControllerA,
				"hostGroup":  hostGroup.Label,
			}).Warnf("Please ensure all relevant hosts are added to Host Group %s.", driver.Config.AccessGroup)
		}

	case drivers.FakeStorageDriverName:
		break

	default:
		err = fmt.Errorf("unknown storage driver: %v", commonConfig.StorageDriverName)
		return
	}

	sb, err = storage.NewStorageBackend(storageDriver)
	return
}
