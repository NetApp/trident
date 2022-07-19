/*
Copyright 2020 NetApp Inc..

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSLicenseInfo information describing the state of the license
type AstraDSLicenseInfo struct {
	// Product is the user readable name for this license tier. Taken from "product" field of license file
	Product string `json:"product"`
	// Protocol is the internal name for this license tier. Taken from licenses->licenseProtocol field in license file
	Protocol string `json:"protocol"`
	// Evaluation is the license Evaluation status
	Evaluation bool `json:"evaluation"`
	// EndDate is the date at which currently applied license expires
	EndDate string `json:"endDate"`
	// EntitlementLastUpdated is the time at which license was generated
	EntitlementLastUpdated string `json:"entitlementLastUpdated"`
	// HostID is the ID of the Kubernetes cluster
	HostID string `json:"hostID"`
	// LicenseSerialNumber is the serial number of the license file
	LicenseSerialNumber string `json:"licenseSerialNumber"`
}

// AstraDSLicenseConfiguration information defining FireTap Configuration
type AstraDSLicenseConfiguration struct {
	// ClusterCoreCountTotal is the total core count allowed from license file
	ClusterCoreCountTotal uint64 `json:"clusterCoreCountTotal"`
	// ClusterStorageMaxCapacityTiB is the maximum cluster storage capacity from license file
	ClusterStorageMaxCapacityTiB uint64 `json:"clusterStorageMaxCapacityTiB"`
}

// AstraDSLicenselicensedFeatures information defining true/false status of licensed features
type AstraDSLicenselicensedFeatures struct {
	NFS                                  bool `json:"nfs"`                                  // NLF - "p-nfs"
	StoragePolicyBasedPvs                bool `json:"storagePolicyBasedPvs"`                // NLF - "dm-storage-policy-based-pvs-for-k8s"
	StorageEfficency                     bool `json:"storageEfficency"`                     // NLF - "dm-storage-efficiency"
	DataAtRestEncryption                 bool `json:"dataAtRestEncryption"`                 // NLF - "dm-data-at-rest-encryption"
	OnDemandLocalSnapshots               bool `json:"onDemandLocalSnapshots"`               // NLF - "dm-on-demand-local-snapshots"
	OnDemandClones                       bool `json:"onDemandclones"`                       // NLF - "dm-on-demand-clones"
	OnDemandRemoteSnapshotsToObjectStore bool `json:"onDemandRemoteSnapshotsToObjectStore"` // NLF - "dm-on-demand-remote-snapshots-to-s3-object-store"
	ScheduledSnapshotsRemote             bool `json:"scheduledSnapshotsRemote"`             // NLF - "dm-snapshot-schedules-local-remote"
	RackAwareness                        bool `json:"rackAwareness"`                        // NLF - "dp-rack-awareness"
	NoSpof                               bool `json:"noSpof"`                               // NLF - "dp-no-spof"
	AirGapManagement                     bool `json:"airGapManagement"`                     // NLF - "cvm-sds-management-airgap"
	CIRecommendations                    bool `json:"ciRecommendations"`                    // NLF - "lm-ci-recommendations"
}

// AstraDSLicenseLimits defines any limits associated with the license
type AstraDSLicenseLimits struct {
	InitialNodeCount           int   `json:"initialNodeCount,omitempty"`           // Maximum number of nodes allowed to be included on cluster creation
	MaxNodeCount               int   `json:"maxNodeCount,omitempty"`               // Limit on total number of nodes in a cluster
	MaxPerNodeVolumeCapacityGB int64 `json:"maxPerNodeVolumeCapacityGB,omitempty"` // The sum of the size of all volumes in the cluster is limited to maxPerNodeVolumeCapacityGiB * node count
	MaxPerNodeVolumeCount      int   `json:"maxPerNodeVolumeCount,omitempty"`      // The total cluster volume count is limited to MaxPerNodeVolumeCount * node count
	MaxVolumeSizeGB            int64 `json:"maxVolumeSizeGB,omitempty"`            // Limit on volume size in GiB
	MaxCPUPerNode              int   `json:"maxCPUPerNode,omitempty"`              // Per node CPU limit
	DaysUntilAsupRestriction   int   `json:"daysUntilAsupRestriction,omitempty"`   // If this many days pass without a successful AutoSupport upload, features will be restricted
}

// AstraDSLicenseLicenseStatus defines the license status of the cluster
type AstraDSLicenseLicenseStatus struct {
	// Valid is the cluster license validity
	Valid bool `json:"valid"`
	// Restricted defines if this license is operating in restricted mode (Read only mode for ADS objects)
	// This only applies to preview release, when usage asups have not been uploaded in 15 days
	Restricted bool `json:"restricted"`
	// Message detailing the current state of this license
	Msg string `json:"msg"`
}

// AstraDSLicenseSpec defines the desired state of AstraDSLicense
type AstraDSLicenseSpec struct {
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// NLF defines the current license file, passed in as a json string
	NLF string `json:"netappLicenseFile"`
	// +kubebuilder:validation:MinLength=1
	// ADSCluster defines the name of the ADS cluster that this license is associated with
	ADSCluster string `json:"adsClusterName"`
}

// AstraDSLicenseStatus defines the observed state of AstraDSLicense
type AstraDSLicenseStatus struct {
	// License file information
	LicenseInfo AstraDSLicenseInfo `json:"licenseInfo"`

	// Information defining FireTap Configuration
	Configuration AstraDSLicenseConfiguration `json:"configuration"`

	// Information defining true/false status of licensed features
	LicensedFeatures AstraDSLicenselicensedFeatures `json:"licensedFeatures"`

	// Limits enforced by license
	LicenseLimits AstraDSLicenseLimits `json:"licenseLimits,omitempty"`

	// The status of the license
	LicenseStatus AstraDSLicenseLicenseStatus `json:"licenseStatus"`

	// The last time license file was validated by license controller
	LastLicenseValidation string `json:"lastLicenseValidation"`

	// Name of ADS Cluster the license is associated with
	ADSCluster string `json:"adsClusterName"`

	// NLF the status fields are associated with
	NLF string `json:"netappLicenseFile"`
	// LastAutosupport is the datetime of the last autosupport that has been uploaded
	LastAutosupport *metav1.Time `json:"lastAutosupport,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="adsCluster",type="string",JSONPath=".spec.adsClusterName", description="ADS cluster association"
// +kubebuilder:printcolumn:name="Valid",type="string",JSONPath=".status.licenseStatus.valid", description="Valid status of license"
// +kubebuilder:printcolumn:name="Product",type="string",JSONPath=".status.licenseInfo.product", description="Product and tier of license"
// +kubebuilder:printcolumn:name="Evaluation",type="boolean",JSONPath=".status.licenseInfo.evaluation", description="License evaluation status"
// +kubebuilder:printcolumn:name="EndDate",type="string",JSONPath=".status.licenseInfo.endDate", description="Expiration date of license"
// +kubebuilder:printcolumn:name="Validated",type="string",JSONPath=".status.lastLicenseValidation", description="Last license file validation"
// +kubebuilder:printcolumn:name="Validation Result",type="string",JSONPath=".status.licenseStatus.msg", description="Information about issues seen in validation", priority=1
// +kubebuilder:resource:shortName=adsli,categories={ads,all}

// AstraDSLicense is the Schema for the astradslicenses API
type AstraDSLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSLicenseSpec   `json:"spec,omitempty"`
	Status AstraDSLicenseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSLicenseList contains a list of AstraDSLicense
type AstraDSLicenseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSLicense `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSLicense{}, &AstraDSLicenseList{})
}
