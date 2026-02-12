// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	. "github.com/netapp/trident/config"
)

type LogLayer string

func (l LogLayer) String() string {
	return string(l)
}

const (
	// By default, specifying a log layer in addition to a workflow will reduce the workflow logging to only
	// workflows within that layer. Putting this modifier in front of a layer will log things in that layer in addition
	// to the workflow.
	additiveModifier  = "+"
	LogLayerSeparator = ","

	LogLayerCore                    = LogLayer("core")
	LogLayerCoreCache               = LogLayer("core_cache")
	LogLayerCSIFrontend             = LogLayer("csi_frontend")
	LogLayerRESTFrontend            = LogLayer("rest_frontend")
	LogLayerCRDFrontend             = LogLayer("crd_frontend")
	LogLayerDockerFrontend          = LogLayer("docker_frontend")
	LogLayerMetricsFrontend         = LogLayer("metrics_frontend")
	LogLayerPersistentStore         = LogLayer("persistent_store")
	LogLayerANFNASDriver            = LogLayer(AzureNASStorageDriverName)
	LogLayerSolidfireDriver         = LogLayer(SolidfireSANStorageDriverName)
	LogLayerGCNVNASDriver           = LogLayer(GCNVNASStorageDriverName)
	LogLayerOntapNASDriver          = LogLayer(OntapNASStorageDriverName)
	LogLayerOntapNASFlexgroupDriver = LogLayer(OntapNASFlexGroupStorageDriverName)
	LogLayerOntapNASQtreeDriver     = LogLayer(OntapNASQtreeStorageDriverName)
	LogLayerOntapSANDriver          = LogLayer(OntapSANStorageDriverName)
	LogLayerOntapSANEcoDriver       = LogLayer(OntapSANEconomyStorageDriverName)
	LogLayerOntapAPI                = LogLayer("ontap_api")
	LogLayerKubernetesAPI           = LogLayer("kubernetes_api")
	LogLayerFakeDriver              = LogLayer(FakeStorageDriverName)
	LogLayerUtils                   = LogLayer("utils")
	LogLayerAutogrow                = LogLayer("autogrow")
	LogLayerAll                     = LogLayer("all")
	LogLayerNone                    = LogLayer("none")
)

var Layers = []LogLayer{
	LogLayerCore,
	LogLayerCSIFrontend,
	LogLayerRESTFrontend,
	LogLayerCRDFrontend,
	LogLayerDockerFrontend,
	LogLayerPersistentStore,
	LogLayerANFNASDriver,
	LogLayerSolidfireDriver,
	LogLayerOntapNASDriver,
	LogLayerOntapNASFlexgroupDriver,
	LogLayerOntapNASQtreeDriver,
	LogLayerOntapSANDriver,
	LogLayerOntapSANEcoDriver,
	LogLayerFakeDriver,
	LogLayerAutogrow,
	LogLayerAll,
}
