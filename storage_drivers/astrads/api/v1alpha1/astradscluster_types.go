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

package v1alpha1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// AstraDSClusterSpec defines the desired state of AstraDSCluster
type AstraDSClusterSpec struct {
	Namespace       string                `json:"namespace,omitempty"`
	ADSNodeConfig   ADSNodeConfig         `json:"adsNodeConfig"`
	ADSNodeSelector *metav1.LabelSelector `json:"adsNodeSelector,omitempty" protobuf:"bytes,1,opt,name=selector"`
	ADSNodeCount    int                   `json:"adsNodeCount,omitempty"`
	MVIP            string                `json:"mvip"`
	// Currently only a single data network is supported
	ADSDataNetworks []ADSDataNetwork `json:"adsDataNetworks,omitempty"`
	// +kubebuilder:validation:Optional
	ADSNetworkInterfaces   ADSNetworkInterfaces  `json:"adsNetworkInterfaces,omitempty"`
	ADSProtectionDomainKey string                `json:"adsProtectionDomainKey,omitempty"`
	AutoSupportConfig      AutoSupportConfigSpec `json:"autoSupportConfig"`
	// Config to set up monitoring operator
	MonitoringConfig *MonitoringConfig `json:"monitoringConfig,omitempty"`
}

// ADSNodeConfig defines the per node settings of the Astra Spec
type ADSNodeConfig struct {
	// CPU defines the maximum CPU per ADS node that can be used
	// +kubebuilder:validation:Minimum=0
	CPU int64 `json:"cpu"`
	// +kubebuilder:validation:Minimum=0
	Memory int64 `json:"memory"`
	// The max drive capacity to utilize on each ADS cluster node.
	// The capacity is not guaranteed nor promised if the hardware capabilities
	// do not meet the capacity specified.
	//
	// If no capacity is specified, the license capacity is used.
	//
	// The capacity must be large enough to accomodate the minimum disk
	// count required.
	Capacity *int64 `json:"capacity,omitempty"`
	// +kubebuilder:Pattern=`\/.*\/.*`
	// +kubebuilder:validation:Optional
	CacheDevice string `json:"cacheDevice,omitempty"`
	// DrivesFilter defines the regex to filter out the drives
	// +kubebuilder:default:=".*"
	// +kubebuilder:Pattern=`\/.*\/.*`
	// +kubebuilder:validation:Optional
	DrivesFilter string `json:"drivesFilter,omitempty"`
}

type MonitoringConfig struct {
	// Namespace where the monitoring agent is present.
	// +kubebuilder:default:=`netapp-monitoring`
	Namespace string `json:"namespace"`
	// Repo where monitoring operator would find images such as fluent, telegraf etc.
	Repo string `json:"repo"`
}

// ADS Data Network includes netmask/gateways and ip address pool of storage ips
type ADSDataNetwork struct {
	// comma separated list of ip addresses
	// must be at least as many addresses as there are nodes in the cluster
	Addresses string `json:"addresses,omitempty"`
	// netmask is optional as long as the data network is the same network
	// as is already configured for the data interface
	Netmask string `json:"netmask,omitempty"`
	// gateway is optional as long as the data network is the same network
	// as is already configured for the data interface
	Gateway string `json:"gateway,omitempty"`
}

// ADS Network Interfaces defines the type of network interfaces to use
type ADSNetworkInterfaces struct {
	// For MVP, you can only specify all or none
	ManagementInterface string `json:"managementInterface,omitempty"`
	ClusterInterface    string `json:"clusterInterface,omitempty"`
	StorageInterface    string `json:"storageInterface,omitempty"`
}

// ADS Data Address to be exposed in cluster CR status
type ADSDataAddress struct {
	// IP address
	Address string `json:"address,omitempty"`
	// UUID of IP address resource
	UUID string `json:"uuid,omitempty"`
	// The node that this IP is currently on.
	CurrentNode uint32 `json:"currentNode,omitempty"`
}

// AutoSupportConfigSpec defines AutoSupport configuration
type AutoSupportConfigSpec struct {
	// AutoUpload defines the flag to enable or disable AutoSupport upload in the cluster
	// +kubebuilder:default:=true
	AutoUpload bool `json:"autoUpload,omitempty"`

	// DestinationURL defines the endpoint to transfer the AutoSupport bundle collection
	// +kubebuilder:validation:Pattern=`^https?:\/\/.+$`
	// +kubebuilder:default:=`https://support.netapp.com/put/AsupPut`
	DestinationURL string `json:"destinationURL,omitempty"`

	// Enabled defines the flag to enable or disable automatic AutoSupport collection.
	// When disabled, periodic and event driven AutoSupport collection would be disabled.
	// It is still possible to trigger an AutoSupport manually while AutoSupport is disabled
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// CoredumpUpload defines the flag to enable or disable the upload of coredumps for this ADS Cluster
	// +kubebuilder:default:=false
	CoredumpUpload bool `json:"coredumpUpload,omitempty"`

	// HistoryRetentionCount defines the number of local (not uploaded) AutoSupport Custom Resources to retain in the cluster before deletion
	HistoryRetentionCount int `json:"historyRetentionCount,omitempty"`

	// ProxyURL defines the URL of the proxy with port to be used for AutoSupport bundle transfer
	// +kubebuilder:validation:Pattern=`^(https?:\/\/.+|)$`
	ProxyURL string `json:"proxyURL,omitempty"`

	// Periodic defines the config for periodic/scheduled AutoSupport objects
	Periodic []AutoSupportScheduleConfiguration `json:"periodic,omitempty"`
}

type AutoSupportScheduleConfiguration struct {
	// Schedule defines the Kubernetes Cronjob schedule
	// **** TODO: Add Regex validation for schedule
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

// AstraDSClusterStatus defines the observed state of AstraDSCluster
type AstraDSClusterStatus struct {
	NodeStatuses []AstraDSNodeStatus `json:"nodeStatuses,omitempty"`
	// License serial number
	LicenseSerialNumber string `json:"licenseSerialNumber,omitempty"`
	// Firetap cluster node count
	FTNodeCount  int                     `json:"ftNodeCount,omitempty"`
	MasterNodeID int64                   `json:"masterNodeID,omitempty"`
	Resources    AstraDSClusterResources `json:"resources,omitempty"`
	// Firetap cluster UUID
	ClusterUUID string `json:"clusterUUID,omitempty"`
	// ClusterStatus tells us what pre-defined stage the cluster is in at the moment
	ClusterStatus string `json:"clusterStatus,omitempty"`
	// FTClusterHealth tells us the state of the firetap cluster
	FTClusterHealth  FiretapClusterHealth      `json:"ftClusterHealth,omitempty"`
	AutoSupport      AutoSupportConfigStatus   `json:"autosupport,omitempty"`
	Conditions       []AstraDSClusterCondition `json:"conditions,omitempty"`
	ADSDataAddresses []ADSDataAddress          `json:"adsDataAddresses,omitempty"`
	// NOTE: This design may need to be revisited. These fields are populated by the AstraDSDeployment
	// associated with the cluster. The customer can set the spec.version.ads and
	// spec.version.firetap on the deployment. Moving these fields to the status to avoid having
	// them set by the customer here works for reconciliation, but these fields are essentially just proxies for
	// the related spec fields in the deployment.
	DesiredVersions Versions `json:"desiredVersions,omitempty"`
	Versions        Versions `json:"versions,omitempty"`
}

// The cluster is considered unhealthy if it is syncing and/or there are
// cluster faults. The cluster usually only goes into syncing when attempting
// to resolve cluster faults.
type FiretapClusterHealth struct {
	// +kubebuilder:default:=false
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
	CPUDeployed      int64 `json:"cpuDeployed,omitempty"`
	CapacityDeployed int64 `json:"capacityDeployed,omitempty"`
}

func (cr AstraDSClusterResources) String() string {
	return fmt.Sprintf("CPUDeployed: %v, CapacityDeployed: %v", cr.CPUDeployed, cr.CapacityDeployed)
}

type AstraDSClusterConditionType string

const (
	ClusterConditionTypeLicenseExceeded AstraDSClusterConditionType = "LicenseExceeded"
	// If cluster is healthy, that means there are no cluster faults and we are
	// not syncing. Syncing is often part of resolving a fault, so it is unlikely
	// that we will be syncing and no cluster faults will be present.
	ClusterConditionTypeFiretapClusterHealth    AstraDSClusterConditionType = "FiretapClusterHealthy"
	ClusterConditionTypeFiretapNodeAPIHealthy   AstraDSClusterConditionType = "FiretapNodeAPIHealthy"
	ClusterConditionTypeFiretapDriveAPIHealthy  AstraDSClusterConditionType = "FiretapDriveAPIHealthy"
	ClusterConditionTypeFiretapVolumeAPIHealthy AstraDSClusterConditionType = "FiretapVolumeAPIHealthy"
)

type AstraDSClusterCondition struct {
	// Type of cluster condition.
	Type AstraDSClusterConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`
	// The last time this condition was updated.
	//
	// TODO: (brendank) consider making this *metav1.Time or upgrade to newest
	// pacakge. We may want to choose to not set this field and this only works
	// though these two methods.
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
	// If nil, we have not recieved a value from firetap.
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
	DriveID     string    `json:"driveID,omitempty"`
	DriveName   DriveName `json:"driveName,omitempty"`
	DriveSerial string    `json:"driveSerial,omitempty"`
	DriveStatus string    `json:"drivesStatus,omitempty"`
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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.clusterStatus",description="The status of the cluster"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.versions.ads",description="The version of the ADS cluster"
// +kubebuilder:printcolumn:name="Serial Number",type="string",JSONPath=".status.licenseSerialNumber",description="The license serial number of the cluster"
// +kubebuilder:printcolumn:name="MVIP",type="string",JSONPath=".spec.mvip",description="Managment Virtual IP"
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
