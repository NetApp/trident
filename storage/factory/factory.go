// Copyright 2016 NetApp, Inc. All Rights Reserved.

package factory

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/sfapi"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	fake_driver "github.com/netapp/trident/drivers/fake"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/eseries"
	"github.com/netapp/trident/storage/fake"
	"github.com/netapp/trident/storage/ontap"
	"github.com/netapp/trident/storage/solidfire"
)

// Note:  isPassed is copied verbatim from dvp.ontap_common.
func isPassed(s string) bool {
	const passed = "passed"
	return s == passed
}

func NewStorageBackendForConfig(configJSON string) (
	sb *storage.StorageBackend, err error,
) {
	var storageDriver storage.StorageDriver

	// Some drivers may panic during initialize if given invalid parameters,
	// so catch any panics that might occur and return an error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Unable to instantiate backend:  %v", r)
		}
	}()

	commonConfig, err := dvp.ValidateCommonSettings(configJSON)
	if err != nil {
		err = fmt.Errorf("Input failed validation: %v", err)
		return
	}
	// Pre-driver initialization setup
	switch commonConfig.StorageDriverName {
	case dvp.OntapNASStorageDriverName:
		storageDriver = &ontap.OntapNASStorageDriver{}
	case dvp.OntapSANStorageDriverName:
		storageDriver = &ontap.OntapSANStorageDriver{}
	case dvp.SolidfireSANStorageDriverName:
		storageDriver = &solidfire.SolidfireSANStorageDriver{}
	case dvp.EseriesIscsiStorageDriverName:
		storageDriver = &eseries.EseriesStorageDriver{}
	case fake_driver.FakeStorageDriverName:
		storageDriver = &fake.FakeStorageDriver{}
	default:
		err = fmt.Errorf("Unknown storage driver: %v",
			commonConfig.StorageDriverName)
		return
	}

	// Warn about ignored fields in common config if any are set
	if commonConfig.Debug {
		log.WithFields(log.Fields{
			"driverName": commonConfig.StorageDriverName,
		}).Warn("debug set in backend config.  This will be ignored.")
	}
	if commonConfig.DisableDelete {
		log.WithFields(log.Fields{
			"driverName": commonConfig.StorageDriverName,
		}).Warn("disableDelete set in backend config.  This will be ignored.")
	}

	if initializeErr := storageDriver.Initialize(configJSON, commonConfig); initializeErr != nil {
		err = fmt.Errorf("Problem initializing storage driver: '%v' error: %v",
			commonConfig.StorageDriverName, initializeErr)
		return
	}

	// Post-driver initialization setup
	switch commonConfig.StorageDriverName {
	case dvp.OntapNASStorageDriverName:
	case dvp.OntapSANStorageDriverName:
		driver := storageDriver.(*ontap.OntapSANStorageDriver)
		if driver.Config.IgroupName == "netappdvp" {
			// If 'netappdvp' is intended to be the default igroup,
			// config.DefaultOntapIgroup should be set to 'netappdvp'.
			driver.Config.IgroupName = config.DefaultOntapIgroup
		}

		response, errIgroupList := driver.API.IgroupList()
		if !isPassed(response.Result.ResultStatusAttr) {
			err = fmt.Errorf("Problem listing igroups for SVM %v: %v, %v",
				driver.Config.SVM, errIgroupList, response.Result.ResultErrnoAttr)
			return
		}

		found := false
		initiators := ""
		for _, igroupInfo := range response.Result.AttributesList() {
			if igroupInfo.Vserver() == driver.Config.SVM &&
				igroupInfo.InitiatorGroupName() ==
					driver.Config.IgroupName {
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
			return nil, fmt.Errorf("Initiator group %v doesn't exist for SVM %v "+
				"and needs to be manually created! Please also ensure all "+
				"relevant hosts are added to the igroup.",
				driver.Config.IgroupName, driver.Config.SVM)
		} else {
			log.WithFields(log.Fields{
				"driver":     dvp.OntapSANStorageDriverName,
				"SVM":        driver.Config.SVM,
				"igroup":     driver.Config.IgroupName,
				"initiators": initiators,
			}).Warn("Please ensure all relevant hosts are added to the ",
				"initiator group.")
		}

		/* TODO: DON'T DELETE
		// Create igroup automatically and use a REST endpoint for adding hosts
		// (code from nDVP's ontap_san.go)
		driver := storageDriver.(*ontap.OntapSANStorageDriver)
		response, err := driver.API.IgroupCreate(config.DefaultOntapIgroup, "iscsi", "linux")
		if !isPassed(response.Result.ResultStatusAttr) {
			if response.Result.ResultErrnoAttr != azgo.EVDISK_ERROR_INITGROUP_EXISTS {
				return nil, fmt.Errorf("Problem creating igroup %v: %v, %v",
					config.DefaultOntapIgroup, response.Result, err)
			}
		}

		// Not required for Trident but harmless to add host IQNs to the igroup
		iqns, errIqn := utils.GetInitiatorIqns()
		if errIqn != nil {
			return nil, fmt.Errorf("Problem determining host initiator IQNs: %v", errIqn)
		}
		// Add each IQN we found to the igroup
		for _, iqn := range iqns {
			response2, err2 := driver.API.IgroupAdd(config.DefaultOntapIgroup, iqn)
			if !isPassed(response2.Result.ResultStatusAttr) {
				if response2.Result.ResultErrnoAttr != azgo.EVDISK_ERROR_INITGROUP_HAS_NODE {
					return nil, fmt.Errorf("Problem adding IQN: %v to igroup: %v\n%verror: %v", iqn, config.DefaultOntapIgroup, response2.Result, err2)
				}
			}
		}*/
	case dvp.SolidfireSANStorageDriverName:
		driver := storageDriver.(*solidfire.SolidfireSANStorageDriver)

		// Verify the VAG already exists
		listVAGReq := &sfapi.ListVolumeAccessGroupsRequest{
			StartVAGID: 0,
			Limit:      0,
		}
		vags, vagErr := driver.Client.ListVolumeAccessGroups(listVAGReq)
		if vagErr != nil {
			err = fmt.Errorf("Could not list VAGs for backend %s: %s",
				driver.Config.SVIP, vagErr.Error())
			return
		}

		found := false
		initiators := ""
		for _, vag := range vags {
			//TODO: SolidFire backend config should support taking VAG as an arg
			if vag.Name == config.DefaultSolidFireVAG {
				driver.VagID = vag.VAGID
				found = true
				for _, initiator := range vag.Initiators {
					initiators = initiators + initiator + ","
				}
				initiators = strings.TrimSuffix(initiators, ",")
				break
			}
		}
		if !found {
			err = fmt.Errorf("Volume Access Group %v doesn't exist at %v "+
				"and needs to be manually created! Please also ensure all "+
				"relevant hosts are added to the VAG.",
				config.DefaultSolidFireVAG, driver.Config.SVIP)
			return
		} else {
			log.WithFields(log.Fields{
				"driver":     dvp.SolidfireSANStorageDriverName,
				"SVIP":       driver.Config.SVIP,
				"VAG":        config.DefaultSolidFireVAG,
				"initiators": initiators,
			}).Warn("Please ensure all relevant hosts are added to the ",
				"initiator group.")
		}

		/* TODO: DON'T DELETE
		// Create VAG automatically and use a REST endpoint for adding hosts
		if !found {
			// Add the orchestrator Volume Access Group (VAG)
			// lookp host iqns
			iqns, errIqn := utils.GetInitiatorIqns()
			if errIqn != nil {
				err = fmt.Errorf("Problem determining host initiator IQNs: %v", errIqn)
				return
			}
			createVAGReq := &sfapi.CreateVolumeAccessGroupRequest{
				Name:       config.DefaultSolidFireVAG,
				Initiators: iqns,
			}
			vagID, vagErr := driver.Client.CreateVolumeAccessGroup(createVAGReq)
			if vagErr != nil {
				err = fmt.Errorf("Problem creating Volume Access Group %s: %v",
					config.DefaultSolidFireVAG, vagErr)
				return
			}
			driver.VagID = vagID
		}*/

	case dvp.EseriesIscsiStorageDriverName:
		driver := storageDriver.(*eseries.EseriesStorageDriver)

		// Override default HostGroup name if it is "netappdvp"
		if driver.Config.AccessGroup == "netappdvp" {
			driver.Config.AccessGroup = config.DefaultEseriesHostGroup
			log.Debugf("Set default E-series HostGroup to %s", config.DefaultEseriesHostGroup)
		}

		// Make sure the Trident Host Group exists
		hostGroup, err := driver.API.GetHostGroup(driver.Config.AccessGroup)
		if err != nil {
			return nil, err
		} else if hostGroup.ClusterRef == "" {
			return nil, fmt.Errorf("Host Group %s doesn't exist for E-Series array %s "+
				"and needs to be manually created! Please also ensure all "+
				"relevant Hosts are defined on the array and added to the Host Group.",
				driver.Config.AccessGroup, driver.Config.ControllerA)
		} else {
			log.WithFields(log.Fields{
				"driver":     dvp.EseriesIscsiStorageDriverName,
				"controller": driver.Config.ControllerA,
				"hostGroup":  hostGroup.Label,
			}).Warnf("Please ensure all relevant hosts are added to Host Group %s.", driver.Config.AccessGroup)
		}

	case fake_driver.FakeStorageDriverName:
	default:
		err = fmt.Errorf("Unknown storage driver: %v",
			commonConfig.StorageDriverName)
		return
	}
	sb, err = storage.NewStorageBackend(storageDriver)
	return
}
