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
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AstraDSClusterSpec defines the desired state of AstraDSCluster
type AstraDSClusterSpec struct {
	// Namespace is the namespace of the AstraDS control plane
	Namespace string `json:"namespace,omitempty"`
	// ADSNodeConfig defines the settings for resource use for ADS node in the cluster
	ADSNodeConfig ADSNodeConfig `json:"adsNodeConfig"`
	// ADSNodeSelector is the filter for selecting eligible nodes for use in the ADS cluster
	ADSNodeSelector *metav1.LabelSelector `json:"adsNodeSelector,omitempty" protobuf:"bytes,1,opt,name=selector"`
	// ADSNodeCount optionally allows the user to specify the cluster size in number of nodes
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Maximum:=16
	// +kubebuilder:validation:Minimum:=3
	ADSNodeCount *int `json:"adsNodeCount,omitempty"`
	// IP address of the MVIP for the cluster
	MVIP string `json:"mvip"`
	// List of data networks (__currently only a single data network is supported__)
	ADSDataNetworks []ADSDataNetwork `json:"adsDataNetworks"`
	// +kubebuilder:validation:Optional
	// ADSNetworkInterfaces defines settings for configuration of physical network interfaces used in the cluster
	ADSNetworkInterfaces ADSNetworkInterfaces `json:"adsNetworkInterfaces,omitempty"`
	// ADSProtectionDomainKey defines settings for data protection domain in the cluster
	ADSProtectionDomainKey string `json:"adsProtectionDomainKey,omitempty"`
	// AutoSupportConfig defines the configuration of autosupport settings
	AutoSupportConfig AutoSupportConfigSpec `json:"autoSupportConfig"`
	// MonitoringConfig defines the configuration of the monitoring operator
	MonitoringConfig *MonitoringConfig `json:"monitoringConfig,omitempty"`
	// +kubebuilder:validation:Optional
	// Configure SEAR
	SoftwareEncryptionAtRest SoftwareEncryptionAtRest `json:"softwareEncryptionAtRest,omitempty"`
	// DryRun is the flag to specify whether a dry run has to be performed
	// for this cluster before committing to it
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	DryRun bool `json:"dryRun"`
}

// SoftwareEncryptionAtRest defines software encryption at rest configuration
type SoftwareEncryptionAtRest struct {
	// Default 90 days
	// +kubebuilder:default:=`2160h`
	KeyRotationPeriod *metav1.Duration `json:"keyRotationPeriod,omitempty"`
	// +kubebuilder:validation:Optional
	// If no key provider is configured, then the internal keymanager is used.
	ADSKeyProvider string `json:"adsKeyProvider,omitempty"`
}

// ADSNodeConfig defines the per node settings of the Astra Spec
type ADSNodeConfig struct {
	// CPU is the maximum number of CPUs(cores) per ADS node that will be reserved <br> (___default:___ `9`)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=9;23
	// +kubebuilder:default:=9
	// +optional
	CPU int64 `json:"cpu"`
	// Memory is the amount of RAM in GiB that ADS is allowed to consume on a node.
	// <br> (___default:___ `38`)
	// <br> Must be at least 2 + (4 * CPU)
	// <br> Set Memory to 38 for a 9 CPU deployment
	// <br> Set Memory to 94 for a 23 CPU deployment
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=38
	// +optional
	Memory int64 `json:"memory"`
	// Capacity is the maximum drive capacity (in GiB) to utilize on each ADS cluster node.
	// If no capacity is specified, the license capacity is used.
	// The capacity must be large enough to accommodate the minimum disk
	// count required.
	// +kubebuilder:validation:Minimum:=300
	Capacity *int64 `json:"capacity,omitempty"`
	// CacheDevice is the cache device drive name
	// +kubebuilder:validation:Optional
	// +optional
	CacheDevice *string `json:"cacheDevice,omitempty"`
	// DrivesFilter is a regular expression matching the drive names to be selected as data disks on cluster creation or node addition
	// +kubebuilder:validation:Optional
	// +optional
	DrivesFilter *string `json:"drivesFilter,omitempty"`
}

type MonitoringConfig struct {
	// Namespace is where the monitoring agent is present.  <br> (___default:___ `netapp-monitoring`)
	// +kubebuilder:default:=`netapp-monitoring`
	Namespace string `json:"namespace"`
	// Repo is where the monitoring operator would find required images such as fluent, telegraf etc.
	Repo string `json:"repo"`
}

// ADS Data Network includes netmask/gateways and ip address pool of storage ips
type ADSDataNetwork struct {
	// Addresses is a string of comma separated list of data ip addresses, one per node in the cluster
	Addresses string `json:"addresses"`
	// Netmask is the netmask for a data network
	Netmask string `json:"netmask"`
	// Gateway is the default gateway of the data network <br> (___optional:___ if it is already configured for the data interface on each node)
	Gateway string `json:"gateway,omitempty"`
}

// ADS Network Interfaces defines the network interfaces used in the cluster
type ADSNetworkInterfaces struct {
	// ManagementInterface is the interface name to use for management traffic
	ManagementInterface string `json:"managementInterface,omitempty"`
	// ClusterInterface is the interface name to use for cluster traffic
	ClusterInterface string `json:"clusterInterface,omitempty"`
	// StorageInterface is the interface name to use for nfs traffic
	StorageInterface string `json:"storageInterface,omitempty"`
}

// ADS Data Address to be exposed in cluster CR status
type ADSDataAddress struct {
	// Address is the IP address of a data address resource
	Address string `json:"address,omitempty"`
	// UUID is the internal UUID of a data address resource
	UUID string `json:"uuid,omitempty"`
	// CurrentNode is the node id that a data address is currently on
	CurrentNode uint32 `json:"currentNode,omitempty"`
}

// AutoSupportConfigSpec defines the spec of the AutoSupport configuration
type AutoSupportConfigSpec struct {
	// AutoUpload is the flag to enable or disable AutoSupport upload in the cluster
	AutoUpload bool `json:"autoUpload"`

	// DestinationURL is the endpoint to transfer the AutoSupport bundle collection ( In most cases this shouldn't be changed ) <br> (___default:___ `https://support.netapp.com/put/AsupPut`)
	// +kubebuilder:validation:Pattern=`^https?:\/\/.+$`
	// +kubebuilder:default:=`https://support.netapp.com/put/AsupPut`
	// +optional
	DestinationURL string `json:"destinationURL,omitempty"`

	// Enabled defines the flag to enable or disable automatic AutoSupport collection.
	// When disabled, periodic and event driven AutoSupport collection would be disabled.
	// It is still possible to trigger an AutoSupport manually while AutoSupport is disabled
	Enabled bool `json:"enabled"`

	// CoredumpUpload defines the flag to enable or disable the upload of coredumps for this ADS Cluster
	CoredumpUpload bool `json:"coredumpUpload"`

	// HistoryRetentionCount is the number of local (not uploaded) AutoSupport Custom Resources to retain in the cluster before deletion
	HistoryRetentionCount int `json:"historyRetentionCount,omitempty"`

	// ProxyURL is the URL of the proxy with port to be used for AutoSupport bundle transfer (Example `https://proxy.mycorp.org`)
	// +kubebuilder:validation:Pattern=`^(https?:\/\/.+|)$`
	ProxyURL string `json:"proxyURL,omitempty"`

	// Periodic defines the config for periodic/scheduled AutoSupport objects
	Periodic []AutoSupportScheduleConfiguration `json:"periodic,omitempty"`
}

type AutoSupportScheduleConfiguration struct {
	// Schedule defines the Kubernetes Cronjob schedule
	Schedule string `json:"schedule"`

	// PeriodicConfig defines the fields needed to create the Periodic AutoSupports
	PeriodicConfig []AutoSupportPeriodicConfig `json:"periodicconfig,omitempty"`
}

type AutoSupportPeriodicConfig struct {
	// Nodes define the nodes to trigger/collect AutoSupport(s) from. This needs to be specified only for Storage Components and can be ignored otherwise.
	Nodes string `json:"nodes,omitempty"`

	// AutoSupportSetting defines the fields needed to create the AutoSupport CR
	AutoSupportSetting `json:",inline"`
}

type ADSProtectionSchemeResilience struct {
	// Protection scheme type eg: DoubleHelix
	ProtectionScheme string `json:"protectionScheme,omitempty"`
	// Predicted number of simultaneous failures which may occur without losing the ability to automatically heal
	// to node tolerance.
	SustainableFailures int64 `json:"sustainableFailures"`
}

type ADSProtectionDomainResilience struct {
	// The maximum number of bytes that can be stored on the cluster before losing the ability to automatically heal
	// to where the data has node tolerance.
	SingleFailureThresholdBytes int64 `json:"singleFailureThresholdBytes"`
	// The number of simultaneous failures which may occur without losing the ability to automatically heal to where
	// the ensemble has node tolerance.
	SustainableFailuresForEnsemble int64 `json:"sustainableFailuresForEnsemble"`
	// The number of simultaneous failures which may occur without losing the ability to automatically heal to where
	// volume access has node tolerance.
	SustainableFailuresForVolumes int64 `json:"sustainableFailuresForVolumes"`
	// Describes the ability for the cluster to automatically heal back to node tolerance in response to a protection domain
	// failure.
	ProtectionSchemeResilience []ADSProtectionSchemeResilience `json:"protectionSchemeResilience,omitempty"`
}

type ADSProtectionSchemeTolerance struct {
	// Protection scheme type eg: DoubleHelix
	ProtectionScheme string `json:"protectionScheme,omitempty"`
	// The number of simultaneous failures which can occur without losing availability for the Protection Scheme.
	SustainableFailures int64 `json:"sustainableFailures"`
}

type ADSProtectionDomainTolerance struct {
	// The number of simultaneous failures which can occur without losing the ensemble quorum.
	SustainableFailuresForEnsemble int64 `json:"sustainableFailuresForEnsemble"`
	// The number of simultaneous failures which can occur without losing volume access.
	SustainableFailuresForVolumes int64 `json:"sustainableFailuresForVolumes"`
	// Describes how many simultaneous failures can be sustained through which the cluster can continue to read and
	// write data.
	ProtectionSchemeTolerance []ADSProtectionSchemeTolerance `json:"protectionSchemeTolerance,omitempty"`
}

type ADSProtectionDomain struct {
	// UUID of the protection domain
	UUID string `json:"uuid,omitempty"`
	// Name of the protection domain
	Name string `json:"name,omitempty"`
	// Describes the ability for the cluster to automatically heal back in response to a protection domain
	// failure.
	Resilience ADSProtectionDomainResilience `json:"resilience,omitempty"`
	// Describes the ability of the cluster to continue reading and writing data through one or more protection domain
	// failures.
	Tolerance ADSProtectionDomainTolerance `json:"tolerance,omitempty"`
}

// AstraDSClusterStatus defines the observed state of AstraDSCluster
type AstraDSClusterStatus struct {
	// DryRunSummary shows the results of a dry run for the cluster. This summary
	// will be deleted once the DryRun in Spec is set to false i.e. when the user
	// proceeds to creating the cluster post dry run
	DryRunSummary DryRunSummary `json:"dryRunSummary,omitempty"`
	// NodeStatuses contains information about the current state of nodes in the cluster
	NodeStatuses []AstraDSNodeStatus `json:"nodeStatuses,omitempty"`
	// LicenseSerialNumber is the current license serial number
	LicenseSerialNumber string `json:"licenseSerialNumber,omitempty"`
	// FTNodeCount is the current firetap cluster node count
	FTNodeCount int `json:"ftNodeCount,omitempty"`
	// MasterNodeID is the ID of the current cluster master node
	MasterNodeID int64 `json:"masterNodeID,omitempty"`
	// AstraDSClusterResources is the current amount of physical resources deployed in the cluster
	Resources AstraDSClusterResources `json:"resources,omitempty"`
	// Firetap cluster UUID
	ClusterUUID string `json:"clusterUUID,omitempty"`
	// ClusterStatus tells us what pre-defined stage the cluster is in at the moment
	ClusterStatus string `json:"clusterStatus,omitempty"`
	// StorageClusterCreationTimestamp is used to set the firetap cluster creation timestamp
	StorageClusterCreationTimestamp metav1.Time `json:"storageClusterCreationTimestamp,omitempty"`
	// FTClusterHealth tells us the state of the firetap cluster
	FTClusterHealth  FiretapClusterHealth      `json:"ftClusterHealth,omitempty"`
	AutoSupport      AutoSupportConfigStatus   `json:"autosupport,omitempty"`
	Conditions       []AstraDSClusterCondition `json:"conditions,omitempty"`
	ADSDataAddresses []ADSDataAddress          `json:"adsDataAddresses,omitempty"`
	// The version of ADS that the operator is attempting to upgrade to
	DesiredVersions Versions `json:"desiredVersions,omitempty"`
	// The current version of the ADS cluster
	Versions Versions `json:"versions,omitempty"`
	// SoftwareEncryptionAtRest status
	SoftwareEncryptionAtRest SoftwareEncryptionAtRestStatus `json:"softwareEncryptionAtRestStatus,omitempty"`
	CertificateUpdate        *CertificateUpdateStatus       `json:"certificateUpdate,omitempty"`
	// Protection domain status
	ProtectionDomains []ADSProtectionDomain `json:"protectionDomains,omitempty"`
	// Qos Priority Bands
	QosPriorityBands []AstraDSClusterQosPriorityBand `json:"qosPriorityBands,omitempty"`
}

// DryRunSummary defines the observed state of DryRunSummary
type DryRunSummary struct {
	Validations          []DryRunValidation  `json:"validations,omitempty"`
	SelectedNodes        []SelectedNode      `json:"selectedNodes,omitempty"`
	RejectedNodes        []RejectedNode      `json:"rejectedNodes,omitempty"`
	ClusterConfiguration DryRunConfiguration `json:"dryRunConfiguration,omitempty"`
}

type DryRunValidation struct {
	// Type specifies the type of validation i.e. Network, Disk, Node, License, etc.
	Type string `json:"type,omitempty"`
	// Message is a string explanation of dry run result
	Message string `json:"message,omitempty"`
	// Status can be one of "OK, ERROR or WARNING"
	Status string `json:"status,omitempty"`
	// Resolution is the corrective/recommended action
	Resolution string `json:"resolution,omitempty"`
}

type NodeDetails struct {
	// Name is the name of the node (k8s FQDN)
	Name string `json:"name"`
	// Capacity is the disk capacity contributed by the node
	Capacity string `json:"capacity,omitempty"`
	// RawCapacityLimit is the maximum disk capacity that can be contributed by this node
	RawCapacityLimit int64 `json:"rawCapacityLimit,omitempty"`
	// ProtectionDomain is the node's protection domain
	ProtectionDomain string `json:"protectionDomain,omitempty"`
	// Disks is a list of selected disks on this node
	Disks []Disk `json:"disks,omitempty"`
}

type Disk struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id"`
	Capacity string `json:"capacity,omitempty"`
	Type     string `json:"type,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

type SelectedNode struct {
	NodeDetails `json:",inline"`
}

type RejectedNode struct {
	NodeDetails `json:",inline"`
	// RejectedReason is a string explanation for rejecting this node
	RejectionReason string `json:"rejectionReason,omitempty"`
}

type DryRunConfiguration struct {
	CPUDeployed            int   `json:"cpuDeployed,omitempty"`
	RawCapacityDeployed    int64 `json:"rawCapacityDeployed,omitempty"`
	LicensedUsableCapacity int64 `json:"licensedUsableCapacity,omitempty"`
	// TODO: Add this field if ACC needs this later
	// LicensedUsableCPU      int   `json:"licensedUsableCpu"`
}

type SoftwareEncryptionAtRestStatus struct {
	KeyProviderUUID string      `json:"keyProviderUUID,omitempty"`
	KeyProviderName string      `json:"keyProviderName,omitempty"`
	KeyUUID         string      `json:"keyUUID,omitempty"`
	KeyActiveTime   metav1.Time `json:"keyActiveTime,omitempty"`
}

// The firetap nodes certificate update status
type CertificateUpdateStatus struct {
	// UpdateCerts tells us which cert update operation to perform ("FullChain" or "NodeCertsOnly")
	UpdateCerts string `json:"updateCerts,omitempty"`
	// Internal job UUID
	UUID string `json:"uuid,omitempty"`
	// StartTime is the timestamp that cert update operation is ordered
	StartTime metav1.Time `json:"startTime,omitempty"`
	// State shows the internal job state
	State string `json:"state,omitempty"`
}

// The cluster is considered unhealthy if it is syncing and/or there are
// cluster faults. The cluster usually only goes into syncing when attempting
// to resolve cluster faults.
type FiretapClusterHealth struct {
	// +kubebuilder:default:=false
	// Healthy is set to true when the cluster has no cluster faults
	Healthy bool `json:"healthy,omitempty"`
	// These details are more specific on why the cluster would be unhealthy.
	// This field will be nil if there are no cluster faults
	// and the cluster is not syncing.
	Details *FiretapClusterStateDetails `json:"details,omitempty"`
}

type FiretapClusterStateDetails struct {
	// +kubebuilder:default:=false
	Syncing       bool           `json:"syncing,omitempty"`
	ClusterFaults []ClusterFault `json:"clusterFaults,omitempty"`
}

func (c *FiretapClusterStateDetails) Healthy() bool {
	return !c.hasFaults() && !c.isSyncing()
}

func (c *FiretapClusterStateDetails) ReasonAndMessage() (string, string) {
	reason := "ClusterHealthy"
	message := "Firetap Cluster is healthy"
	if c.hasFaults() || c.isSyncing() {
		message = "Firetap Cluster is unhealthy"
	}

	if c.hasFaults() && c.isSyncing() {
		reason = "SyncingWithClusterFaults"
	} else {
		if c.hasFaults() {
			reason = "ClusterFaults"
		}
		if c.isSyncing() {
			reason = "Syncing"
		}
	}
	return reason, message
}

func (c *FiretapClusterStateDetails) hasFaults() bool {
	return len(c.ClusterFaults) > 0
}

func (c *FiretapClusterStateDetails) isSyncing() bool {
	return c.Syncing
}

type ClusterFault struct {
	Code      string      `json:"code"`
	Details   string      `json:"details"`
	NodeID    uint32      `json:"nodeId"`
	Timestamp metav1.Time `json:"timestamp"`
}

type AstraDSClusterResources struct {
	// CPUDeployed is the total number of cores used in this cluster
	CPUDeployed int64 `json:"cpuDeployed,omitempty"`
	// CapacityDeployed is the total storage (in GiB) used in this cluster
	CapacityDeployed int64 `json:"capacityDeployed,omitempty"`
	// LicenseCapacity is the total capacity (in GiB) enforced by license.
	LicensedCapacity int64 `json:"licensedCapacity,omitempty"`
	// UsedCapacity is the total raw capacity (in GiB) used.  This is limited by LicensedCapacity
	UsedCapacity int64 `json:"usedCapacity,omitempty"`
}

type AstraDSClusterQosPriorityBand struct {
	// What the band is called. One of 'High', 'Medium', or 'Low'
	Name string `json:"name"`
	// How much more 'priority' this band has relative to the 'Low' band. Low is always 1
	Priority uint32 `json:"priority"`
}

func (cr AstraDSClusterResources) String() string {
	return fmt.Sprintf("CPUDeployed: %v, CapacityDeployed: %v", cr.CPUDeployed, cr.CapacityDeployed)
}

type AstraDSClusterConditionType string

const (
	// LicenseExceeded is set to true when the cluster has gone past maximum limits defined for this cluster
	ClusterConditionTypeLicenseExceeded AstraDSClusterConditionType = "LicenseExceeded"
	// FiretapClusterHealthy indicates the cluster is healthy, meaning there are no cluster faults and we are
	// not syncing.
	ClusterConditionTypeFiretapClusterHealth AstraDSClusterConditionType = "FiretapClusterHealthy"
	// FiretapNodeAPIHealthy is set to false when the node status APIs are failing with errors or unable to connect
	ClusterConditionTypeFiretapNodeAPIHealthy AstraDSClusterConditionType = "FiretapNodeAPIHealthy"
	// FiretapDriveAPIHealthy is set to false when the drive status APIs are failing or unable to connect
	ClusterConditionTypeFiretapDriveAPIHealthy AstraDSClusterConditionType = "FiretapDriveAPIHealthy"
	// FiretapVolumeAPIHealthy is set to false when the volume status APIs are failing or unable to connect
	ClusterConditionTypeFiretapVolumeAPIHealthy AstraDSClusterConditionType = "FiretapVolumeAPIHealthy"
	// ADSVersion condition indicates the state of ADS Cluster version
	// The status can be one of Unknown, True or False
	// Status               Reason                                                  When
	// Unknown              StorageClusterUpgrading                 ADS Cluster Upgrade is started/scheduled due to version mismatch
	//                              StorageClusterUpgraded                  Storage Cluster has been upgraded to requested version
	//                              ADSClusterUpgrading                             Control plane is being upgraded to the requested version
	//                              ADSClusterUpgraded                              Control plane has been upgraded to the requested version
	// True                 ADSVersionAppliedSuccessfully   ADS Cluster is at requested version
	// False                StorageClusterUpgradeHalted             Storage cluster upgrade fails or is stuck at some node
	//                              ADSClusterUpgradeFailed                 Control plane upgrade fails
	ClusterConditionTypeADSVersion AstraDSClusterConditionType = "ADSVersion"
)

type AstraDSClusterCondition struct {
	// Type of cluster condition.
	Type AstraDSClusterConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// AstraDSNodeStatus defines the observed state of firetap nodes
type AstraDSNodeStatus struct {
	NodeName          string                       `json:"nodeName,omitempty"`
	NodeUUID          string                       `json:"nodeUUID,omitempty"`
	NodeID            int64                        `json:"nodeID,omitempty"`
	NodeManagementIP  string                       `json:"nodeManagementIP,omitempty"`
	NodeRole          string                       `json:"nodeRole,omitempty"`
	NodeVersion       string                       `json:"nodeVersion,omitempty"`
	ProtectionDomain  string                       `json:"protectionDomain,omitempty"`
	MaintenanceStatus AstraDSNodeMaintenanceStatus `json:"maintenanceStatus,omitempty"`
	NodeStatus        string                       `json:"status,omitempty"`
	// If true, node is reachable. If false node is not reachable.
	// If nil, we have not received a value from firetap.
	NodeIsReachable *bool `json:"nodeIsReachable,omitempty"`
	// NodeHA implies if this node is needed for high availability or not,
	// also when NodeHA is false, then this node could be removed
	NodeHA              *bool                `json:"nodeHA,omitempty"`
	DriveFilterValidity string               `json:"driveFilterValidity,omitempty"`
	DriveStatuses       []AstraDSDriveStatus `json:"driveStatuses,omitempty"`
}

// IsRemovedNode contains the logic to tell if a node has been removed
func (ns AstraDSNodeStatus) IsRemovedNode() bool {
	return !*ns.NodeHA && ns.NodeStatus == "Pending"
}

// CanRemoveNode contains the logic to tell if a node is in a removeable state
func (ns AstraDSNodeStatus) CanRemoveNode() bool {
	return !*ns.NodeHA && ns.NodeStatus == "Added"
}

// AstraDSNode's maintenance state
type AstraDSNodeMaintenanceStatus struct {
	State   string `json:"state,omitempty"`
	Variant string `json:"variant,omitempty"`
}

// AstraDSDriveStatus is the status for an Astra SDS Drive
type AstraDSDriveStatus struct {
	// DriveID is the internal ID of a drive
	DriveID string `json:"driveID,omitempty"`
	// DriveName is the device name of a drive
	DriveName DriveName `json:"driveName,omitempty"`
	// DriveSerial is the serail number of a drive
	DriveSerial string `json:"driveSerial,omitempty"`
	// DriveStatus is the current cluster state of a drive
	DriveStatus string `json:"drivesStatus,omitempty"`
}

type DriveName string

// MatchDriveFilters returns whether a drive matches a node or cluster's filter conditions
func (s DriveName) MatchDriveFilters(nodeFilter, clusterFilter string) bool {
	return true
}

// AutoSupportConfigStatus defines the condition of the AutoSupport configuration in the cluster
type AutoSupportConfigStatus struct {
	// Details defines an optional string containing any additional information about the state
	Details string `json:"details,omitempty"`

	// PeriodicMap defines the config for periodic/scheduled AutoSupport objects
	PeriodicMap map[string]AutoSupportScheduleConfiguration `json:"periodicmap,omitempty"`
}

// AutoSupportConfigState represents the overall possible state of the AutoSupport subsystem in the deployment
type AutoSupportConfigState string

// ready: All autosupport components are functional/ healthy
// partial: Some autosupport components are functional/ healthy
// error: No autosupport components are functional/ healthy
const (
	AutoSupportConfigReady   AutoSupportConfigState = "ready"
	AutoSupportConfigPartial AutoSupportConfigState = "partial"
	AutoSupportConfigError   AutoSupportConfigState = "error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.clusterStatus",description="The status of the cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.versions.ads",description="The version of the ADS cluster"
// +kubebuilder:printcolumn:name="Serial Number",type="string",JSONPath=".status.licenseSerialNumber",description="The license serial number of the cluster"
// +kubebuilder:printcolumn:name="MVIP",type="string",JSONPath=".spec.mvip",description="Management Virtual IP"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:shortName=adscl,categories={ads,all}

// AstraDSCluster is the Schema for the astradsclusters API
type AstraDSCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AstraDSClusterSpec   `json:"spec,omitempty"`
	Status AstraDSClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AstraDSClusterList contains a list of AstraDSCluster
type AstraDSClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AstraDSCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AstraDSCluster{}, &AstraDSClusterList{})
}
