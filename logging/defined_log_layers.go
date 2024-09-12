package logging

import (
	. "github.com/netapp/trident/config"
)

const (
	// By default, specifying a log layer in addition to a workflow will reduce the the workflow logging to only
	// workflows within that layer. Putting this modifier in front of a layer will log things in that layer in addition
	// to the workflow.
	additiveModifier  = "+"
	LogLayerSeparator = ","

	LogLayerCore                    = LogLayer("core")
	LogLayerCSIFrontend             = LogLayer("csi_frontend")
	LogLayerRESTFrontend            = LogLayer("rest_frontend")
	LogLayerCRDFrontend             = LogLayer("crd_frontend")
	LogLayerDockerFrontend          = LogLayer("docker_frontend")
	LogLayerMetricsFrontend         = LogLayer("metrics_frontend")
	LogLayerPersistentStore         = LogLayer("persistent_store")
	LogLayerANFNASDriver            = LogLayer(AzureNASStorageDriverName)
	LogLayerSolidfireDriver         = LogLayer(SolidfireSANStorageDriverName)
	LogLayerGCPNASDriver            = LogLayer(GCPNFSStorageDriverName)
	LogLayerGCNVNASDriver           = LogLayer(GCNVNASStorageDriverName)
	LogLayerOntapNASDriver          = LogLayer(OntapNASStorageDriverName)
	LogLayerOntapNASFlexgroupDriver = LogLayer(OntapNASFlexGroupStorageDriverName)
	LogLayerOntapNASQtreeDriver     = LogLayer(OntapNASQtreeStorageDriverName)
	LogLayerOntapSANDriver          = LogLayer(OntapSANStorageDriverName)
	LogLayerOntapSANEcoDriver       = LogLayer(OntapSANEconomyStorageDriverName)
	LogLayerFakeDriver              = LogLayer(FakeStorageDriverName)
	LogLayerUtils                   = LogLayer("utils")
	LogLayerAll                     = LogLayer("all")
	LogLayerNone                    = LogLayer("none")
)

var layers = []LogLayer{
	LogLayerCore,
	LogLayerCSIFrontend,
	LogLayerRESTFrontend,
	LogLayerCRDFrontend,
	LogLayerDockerFrontend,
	LogLayerPersistentStore,
	LogLayerANFNASDriver,
	LogLayerSolidfireDriver,
	LogLayerGCPNASDriver,
	LogLayerOntapNASDriver,
	LogLayerOntapNASFlexgroupDriver,
	LogLayerOntapNASQtreeDriver,
	LogLayerOntapSANDriver,
	LogLayerOntapSANEcoDriver,
	LogLayerFakeDriver,
	LogLayerAll,
}
