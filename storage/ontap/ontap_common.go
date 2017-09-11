// Copyright 2016 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"runtime/debug"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/netapp/netappdvp/apis/ontap"
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
	ontapHDD:    {sa.Media: sa.NewStringOffer(sa.HDD)},
	ontapHybrid: {sa.Media: sa.NewStringOffer(sa.Hybrid)},
	ontapSSD:    {sa.Media: sa.NewStringOffer(sa.SSD)},
}

// getStorageBackendSpecsCommon discovers the aggregates assigned to the configured SVM, and it updates the specified StorageBackend
// object with StoragePools and their associated attributes.
func getStorageBackendSpecsCommon(
	d dvp.OntapStorageDriver, backend *storage.StorageBackend, poolAttributes map[string]sa.Offer,
) (err error) {

	api := d.GetAPI()
	config := d.GetConfig()
	driverName := d.Name()

	// We will examine all assigned aggregates, so warn if one is set in the config.
	if config.Aggregate != "" {
		log.WithFields(log.Fields{
			"driverName": driverName,
			"aggregate":  config.Aggregate,
		}).Warn("aggregate set in backend config.  This will be ignored.")
	}

	// Handle panics from the API layer
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Unable to inspect ONTAP backend:  %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	// Get the aggregates assigned to the SVM.  There must be at least one!
	vserverAggrs, err := api.GetVserverAggregateNames()
	if err != nil {
		return
	}
	if len(vserverAggrs) == 0 {
		err = fmt.Errorf("SVM %s has no assigned aggregates.", config.SVM)
		return
	}

	// Define a storage pool for each of the SVM's aggregates
	storagePools := make(map[string]*storage.StoragePool)
	for _, aggrName := range vserverAggrs {
		storagePools[aggrName] = storage.NewStoragePool(backend, aggrName)
	}

	log.WithFields(log.Fields{
		"svm":   config.SVM,
		"pools": vserverAggrs,
	}).Debug("Read storage pools assigned to SVM.")

	// Update pools with aggregate info (i.e. MediaType) using the best means possible
	var aggr_err error
	if api.SupportsApiFeature(ontap.VSERVER_SHOW_AGGR) {
		aggr_err = getVserverAggregateAttributes(d, &storagePools)
	} else {
		aggr_err = getClusterAggregateAttributes(d, &storagePools)
	}

	if zerr, ok := aggr_err.(ontap.ZapiError); ok && zerr.IsScopeError() {
		log.WithFields(log.Fields{
			"username": config.Username,
		}).Warn("User has insufficient privileges to obtain aggregate info. Storage classes with physical attributes " +
			"such as 'media' will not match pools on this backend.")
	} else if err != nil {
		log.Errorf("Could not obtain aggregate info. Storage classes with physical attributes "+
			"such as 'media' will not match pools on this backend. %v", err)
	}

	// Add attributes common to each pool and register pools with backend
	for _, pool := range storagePools {

		for attrName, offer := range poolAttributes {
			pool.Attributes[attrName] = offer
		}

		backend.AddStoragePool(pool)
	}

	return
}

// getVserverAggregateAttributes gets pool attributes using vserver-show-aggr-get-iter, which will only succeed on Data ONTAP 9 and later.
// If the aggregate attributes are read successfully, the pools passed to this function are updated accordingly.
func getVserverAggregateAttributes(d dvp.OntapStorageDriver, storagePools *map[string]*storage.StoragePool) error {

	result, err := d.GetAPI().VserverShowAggrGetIterRequest()
	if err != nil {
		return err
	}
	if zerr := ontap.NewZapiError(result.Result); !zerr.IsPassed() {
		return zerr
	}

	for _, aggr := range result.Result.AttributesList() {
		aggrName := string(aggr.AggregateName())
		aggrType := aggr.AggregateType()

		// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
		pool, ok := (*storagePools)[aggrName]
		if !ok {
			continue
		}

		// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
		storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
		if !ok {
			log.WithFields(log.Fields{
				"aggregate": aggrName,
				"mediaType": aggrType,
			}).Warn("Aggregate has unknown media type.")

			continue
		}

		log.WithFields(log.Fields{
			"aggregate": aggrName,
			"mediaType": aggrType,
		}).Debug("Read aggregate attributes.")

		// Update the pool with the aggregate storage attributes
		for attrName, attr := range storageAttrs {
			pool.Attributes[attrName] = attr
		}
	}

	return nil
}

// getClusterAggregateAttributes gets pool attributes using aggr-get-iter, which will only succeed for cluster-scoped users
// with adequate permissions.  If the aggregate attributes are read successfully, the pools passed to this function are updated
// accordingly.
func getClusterAggregateAttributes(d dvp.OntapStorageDriver, storagePools *map[string]*storage.StoragePool) error {

	result, err := d.GetAPI().AggrGetIterRequest()
	if err != nil {
		return err
	}
	if zerr := ontap.NewZapiError(result.Result); !zerr.IsPassed() {
		return zerr
	}

	for _, aggr := range result.Result.AttributesList() {
		aggrName := aggr.AggregateName()
		aggrRaidAttrs := aggr.AggrRaidAttributes()
		aggrType := aggrRaidAttrs.AggregateType()

		// Find matching pool.  There are likely more aggregates in the cluster than those assigned to this backend's SVM.
		pool, ok := (*storagePools)[aggrName]
		if !ok {
			continue
		}

		// Get the storage attributes (i.e. MediaType) corresponding to the aggregate type
		storageAttrs, ok := ontapPerformanceClasses[ontapPerformanceClass(aggrType)]
		if !ok {
			log.WithFields(log.Fields{
				"aggregate": aggrName,
				"mediaType": aggrType,
			}).Warn("Aggregate has unknown media type.")

			continue
		}

		log.WithFields(log.Fields{
			"aggregate": aggrName,
			"mediaType": aggrType,
		}).Debug("Read aggregate attributes.")

		// Update the pool with the aggregate storage attributes
		for attrName, attr := range storageAttrs {
			pool.Attributes[attrName] = attr
		}
	}

	return nil
}

func getVolumeOptsCommon(
	volConfig *storage.VolumeConfig,
	pool *storage.StoragePool,
	requests map[string]sa.Request,
) map[string]string {
	opts := make(map[string]string)
	opts["aggregate"] = pool.Name
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
	if encryptionReq, ok := requests[sa.Encryption]; ok {
		if encryption, ok := encryptionReq.Value().(bool); ok {
			if encryption {
				opts["encryption"] = "true"
			}
		} else {
			log.WithFields(log.Fields{
				"provisioner": "ONTAP",
				"method":      "getVolumeOptsCommon",
				"encryption":  encryptionReq.Value(),
			}).Warnf("Expected bool for %s; ignoring.", sa.Encryption)
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

func getInternalVolumeNameCommon(config *dvp.CommonStorageDriverConfig, name string) string {
	s1 := storage.GetCommonInternalVolumeName(config, name)
	s2 := strings.Replace(s1, "-", "_", -1)
	s3 := strings.Replace(s2, ".", "_", -1)
	return s3
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
		config.CommonStorageDriverConfig)
	return &struct {
		*storage.CommonStorageDriverConfigExternal
		ManagementLIF string `json:"managementLIF"`
		DataLIF       string `json:"dataLIF"`
		IgroupName    string `json:"igroupName"`
		SVM           string `json:"svm"`
	}{
		CommonStorageDriverConfigExternal: storage.GetCommonStorageDriverConfigExternal(
			config.CommonStorageDriverConfig,
		),
		ManagementLIF: config.ManagementLIF,
		DataLIF:       config.DataLIF,
		IgroupName:    config.IgroupName,
		SVM:           config.SVM,
	}
}
