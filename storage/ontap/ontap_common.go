// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/azgo"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

type ontapPerformanceClass string

const (
	ontapHDD    ontapPerformanceClass = "hdd"
	ontapHybrid ontapPerformanceClass = "hybrid"
	ontapSSD    ontapPerformanceClass = "ssd"
)

var ontapPerformanceClasses = map[ontapPerformanceClass]map[string]sa.Offer{
	ontapHDD: map[string]sa.Offer{
		sa.Media: sa.NewStringOffer(sa.HDD),
	},
	ontapHybrid: map[string]sa.Offer{
		sa.Media: sa.NewStringOffer(sa.Hybrid),
	},
	ontapSSD: map[string]sa.Offer{
		sa.Media: sa.NewStringOffer(sa.SSD),
	},
}

func getCommonONTAPStoragePoolAttributes(vc *storage.StoragePool) {
	// ONTAP supports snapshots
	vc.Attributes[sa.Snapshots] = sa.NewBoolOffer(true)
	// ONTAP volumes support both thick and thin provisioning.
	vc.Attributes[sa.ProvisioningType] = sa.NewStringOffer("thick", "thin")
}

func getStorageBackendSpecsCommon(
	backend *storage.StorageBackend, name string, svm string,
	zr *azgo.ZapiRunner, aggregate string,
) (err error) {
	if aggregate != "" {
		log.WithFields(log.Fields{
			"driverName": name,
		}).Warn("aggregate specified in backend config.  Trident ignores this.")
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Unable to inspect ONTAP backend:  %v\nStack "+
				"trace:\n%s", r, debug.Stack())
		}
	}()
	// find the aggregates for this backend
	r1, err1 := azgo.NewVserverGetIterRequest().SetMaxRecords(0xffffffff - 1).ExecuteUsing(zr)
	if err1 != nil {
		err = fmt.Errorf("Problem in contacting storage backend to get its " +
			"specs!")
		return
	}

	storagePools := make(map[string]*storage.StoragePool)
	clusterManagementLIF := false
	validSVM := false
	for _, attrs := range r1.Result.AttributesList() {
		if attrs.VserverName() != svm {
			clusterManagementLIF = true
			continue
		}
		// cluster SVM
		validSVM = true
		var aggrInfo []azgo.VserverAggrInfoType = attrs.VserverAggrInfoList()
		for _, v := range aggrInfo {
			aggrName := string(v.AggrName())
			storagePools[aggrName] = storage.NewStoragePool(backend, aggrName)
		}
	}

	// error handling
	if !clusterManagementLIF {
		err = errors.New("Cluster management LIF, username, and password" +
			" must be specified in the backend configuration!")
		return
	}
	if !validSVM {
		err = fmt.Errorf("Couldn't find SVM %s on this cluster!", svm)
		return
	}

	// find storage level classes for aggregates that are backed by this SVM
	r2, err2 := azgo.NewAggrGetIterRequest().SetMaxRecords(0xffffffff - 1).ExecuteUsing(zr)
	if err2 != nil {
		err = fmt.Errorf("Problem in contacting storage backends to get its " +
			"specs!")
		return
	}
	for _, attrs := range r2.Result.AttributesList() {
		attrRaid := attrs.AggrRaidAttributes()
		attrRaidPtr := &attrRaid
		vc, ok := storagePools[attrs.AggregateName()]
		if !ok {
			continue
		}
		performanceAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(
			attrRaidPtr.AggregateType())]
		if !ok {
			err = fmt.Errorf("Aggregate %s has unknown type %s",
				attrs.AggregateName(), attrRaidPtr.AggregateType())
			return
		}
		for attrName, attr := range performanceAttrs {
			vc.Attributes[attrName] = attr
		}
		vc.Attributes[sa.BackendType] = sa.NewStringOffer(name)
		getCommonONTAPStoragePoolAttributes(vc)
		backend.AddStoragePool(vc)
	}

	return
}

func getVolumeOptsCommon(
	volConfig *storage.VolumeConfig,
	vc *storage.StoragePool,
	requests map[string]sa.Request,
) map[string]string {
	opts := make(map[string]string)
	opts["aggregate"] = vc.Name
	if provisioningTypeReq, ok := requests[sa.ProvisioningType]; ok {
		if p, ok := provisioningTypeReq.Value().(string); ok {
			if p == "thin" {
				opts["spaceReserve"] = "none"
			} else if p == "thick" {
				// p will equal "thick" here
				opts["spaceReserve"] = "volume"
			} else {
				log.WithFields(log.Fields{
					"provisioner":      "ONTAP",
					"method":           "getVolumeOptsCommon",
					"provisioningType": provisioningTypeReq.Value(),
				}).Warnf("Expected 'thick' or 'thin' for %s; ignoring.",
					sa.ProvisioningType)
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner":      "ONTAP",
				"method":           "getVolumeOptsCommon",
				"provisioningType": provisioningTypeReq.Value(),
			}).Warnf("Expected string for %s; ignoring.", sa.ProvisioningType)
		}
	}
	if volConfig.SnapshotPolicy != "" {
		opts["snapshotPolicy"] = volConfig.SnapshotPolicy
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
	return opts
}

func getInternalVolumeNameCommon(name string) string {
	return strings.Replace(name, "-", "_", -1)
}

/*func createPrepareCommon(volConfig *storage.VolumeConfig) bool {
	// 1. Sanitize the volume name
	volConfig.InternalName = getInternalVolumeNameCommon(volConfig.Name)

	// Because the storage prefix specified in the backend config must create
	// a unique set of volume names, we do not need to check whether volumes
	// exist in the backend here.
	return true
}*/

func getExternalConfig(config dvp.OntapStorageDriverConfig) interface{} {
	storage.SanitizeCommonStorageDriverConfig(
		&config.CommonStorageDriverConfig)
	return &struct {
		*storage.CommonStorageDriverConfigExternal
		ManagementLIF string `json:"managementLIF"`
		DataLIF       string `json:"dataLIF"`
		IgroupName    string `json:"igroupName"`
		SVM           string `json:"svm"`
	}{
		CommonStorageDriverConfigExternal: storage.GetCommonStorageDriverConfigExternal(
			&config.CommonStorageDriverConfig,
		),
		ManagementLIF: config.ManagementLIF,
		DataLIF:       config.DataLIF,
		IgroupName:    config.IgroupName,
		SVM:           config.SVM,
	}
}
