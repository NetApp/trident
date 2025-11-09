// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/config"
)

const (
	TridentFinalizerName = "trident.netapp.io"
)

func GetTridentFinalizers() []string {
	return []string{
		TridentFinalizerName,
	}
}

/************************
* Trident Orchestrator
************************/

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentOrchestrator is the Schema for the tridentorchestrators API
type TridentOrchestrator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TridentOrchestratorSpec   `json:"spec,omitempty"`
	Status TridentOrchestratorStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TridentOrchestratorList contains a list of TridentOrchestrator
type TridentOrchestratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TridentOrchestrator `json:"items"`
}

// TridentOrchestratorSpec defines the desired state of TridentOrchestrator
type TridentOrchestratorSpec struct {
	EnableForceDetach            bool              `json:"enableForceDetach"`
	DisableAuditLog              *bool             `json:"disableAuditLog"`
	Namespace                    string            `json:"namespace"`
	IPv6                         bool              `json:"IPv6,omitempty"`
	K8sTimeout                   int               `json:"k8sTimeout,omitempty"`
	HTTPRequestTimeout           string            `json:"httpRequestTimeout,omitempty"`
	SilenceAutosupport           bool              `json:"silenceAutosupport,omitempty"`
	ExcludeAutosupport           *bool             `json:"excludeAutosupport,omitempty"`
	AutosupportImage             string            `json:"autosupportImage,omitempty"`
	AutosupportProxy             string            `json:"autosupportProxy,omitempty"`
	AutosupportInsecure          bool              `json:"autosupportInsecure,omitempty"`
	AutosupportSerialNumber      string            `json:"autosupportSerialNumber,omitempty"`
	AutosupportHostname          string            `json:"autosupportHostname,omitempty"`
	Uninstall                    bool              `json:"uninstall,omitempty"`
	LogFormat                    string            `json:"logFormat,omitempty"`
	LogLevel                     string            `json:"logLevel,omitempty"`
	Debug                        bool              `json:"debug,omitempty"`
	LogWorkflows                 string            `json:"logWorkflows,omitempty"`
	LogLayers                    string            `json:"logLayers,omitempty"`
	ProbePort                    *int64            `json:"probePort,omitempty"`
	TridentImage                 string            `json:"tridentImage,omitempty"`
	ImageRegistry                string            `json:"imageRegistry,omitempty"`
	KubeletDir                   string            `json:"kubeletDir,omitempty"`
	Wipeout                      []string          `json:"wipeout,omitempty"`
	ImagePullSecrets             []string          `json:"imagePullSecrets,omitempty"`
	ControllerPluginNodeSelector map[string]string `json:"controllerPluginNodeSelector,omitempty"`
	ControllerPluginTolerations  []Toleration      `json:"controllerPluginTolerations,omitempty"`
	NodePluginNodeSelector       map[string]string `json:"nodePluginNodeSelector,omitempty"`
	NodePluginTolerations        []Toleration      `json:"nodePluginTolerations,omitempty"`
	Windows                      bool              `json:"windows,omitempty"`
	ImagePullPolicy              string            `json:"imagePullPolicy,omitempty"`
	CloudProvider                string            `json:"cloudProvider,omitempty"`
	CloudIdentity                string            `json:"cloudIdentity,omitempty"`
	EnableACP                    bool              `json:"enableACP,omitempty"`
	ACPImage                     string            `json:"acpImage,omitempty"`
	EnableAutoBackendConfig      bool              `json:"enableAutoBackendConfig,omitempty"`
	ISCSISelfHealingInterval     string            `json:"iscsiSelfHealingInterval,omitempty"`
	ISCSISelfHealingWaitTime     string            `json:"iscsiSelfHealingWaitTime,omitempty"`
	K8sAPIQPS                    int               `json:"k8sAPIQPS,omitempty"`
	FSGroupPolicy                string            `json:"fsGroupPolicy,omitempty"`
	NodePrep                     []string          `json:"nodePrep"`
	SkipCRDsToObliviate          []string          `json:"skipCRDsToObliviate,omitempty"`
	EnableConcurrency            bool              `json:"enableConcurrency,omitempty"`
	Resources                    *Resources        `json:"resources,omitempty"`
	HTTPSMetrics                 bool              `json:"httpsMetrics,omitempty"`
	HostNetwork                  bool              `json:"hostNetwork,omitempty"`
}

// Toleration
type Toleration struct {
	Effect            string `json:"effect,omitempty"`
	Key               string `json:"key,omitempty"`
	Value             string `json:"value,omitempty"`
	Operator          string `json:"operator,omitempty"`
	TolerationSeconds int    `json:"tolerationSeconds,omitempty"`
}

func (t *Toleration) GetMap() map[string]string {
	toleration := map[string]string{}

	if t.Key != "" {
		toleration["key"] = t.Key
	}

	if t.Effect != "" {
		toleration["effect"] = t.Effect
	}

	if t.Value != "" {
		toleration["value"] = t.Value
	}

	if t.Operator != "" {
		toleration["operator"] = t.Operator
	}

	if t.TolerationSeconds != 0 {
		toleration["tolerationSeconds"] = fmt.Sprintf("%d", t.TolerationSeconds)
	}

	return toleration
}

type ContainersResourceRequirements map[string]*ContainerResource

type Resources struct {
	Controller ContainersResourceRequirements `json:"controller,omitempty"`
	Node       *NodeResources                 `json:"node,omitempty"`
}

type NodeResources struct {
	Linux   ContainersResourceRequirements `json:"linux,omitempty"`
	Windows ContainersResourceRequirements `json:"windows,omitempty"`
}

type ContainerResource struct {
	Requests *ResourceRequirements `json:"requests,omitempty"`
	Limits   *ResourceRequirements `json:"limits,omitempty"`
}

type ResourceRequirements struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// TridentOrchestratorStatus defines the observed state of TridentOrchestrator
type TridentOrchestratorStatus struct {
	Message                   string                        `json:"message"`
	Status                    string                        `json:"status"`
	Version                   string                        `json:"version"`
	Namespace                 string                        `json:"namespace"`
	CurrentInstallationParams TridentOrchestratorSpecValues `json:"currentInstallationParams"`
	ACPVersion                string                        `json:"acpVersion,omitempty"`
}

type TridentOrchestratorSpecValues struct {
	EnableForceDetach        string            `json:"enableForceDetach"`
	DisableAuditLog          string            `json:"disableAuditLog"`
	IPv6                     string            `json:"IPv6"`
	SilenceAutosupport       string            `json:"silenceAutosupport"`
	AutosupportImage         string            `json:"autosupportImage"`
	AutosupportProxy         string            `json:"autosupportProxy"`
	AutosupportInsecure      bool              `json:"autosupportInsecure"`
	AutosupportSerialNumber  string            `json:"autosupportSerialNumber"`
	AutosupportHostname      string            `json:"autosupportHostname"`
	K8sTimeout               string            `json:"k8sTimeout"`
	HTTPRequestTimeout       string            `json:"httpRequestTimeout"`
	LogFormat                string            `json:"logFormat"`
	LogLevel                 string            `json:"logLevel"`
	Debug                    string            `json:"debug"`
	LogWorkflows             string            `json:"logWorkflows"`
	LogLayers                string            `json:"logLayers"`
	ProbePort                string            `json:"probePort"`
	TridentImage             string            `json:"tridentImage"`
	ImageRegistry            string            `json:"imageRegistry"`
	KubeletDir               string            `json:"kubeletDir"`
	ImagePullSecrets         []string          `json:"imagePullSecrets"`
	NodePluginNodeSelector   map[string]string `json:"nodePluginNodeSelector,omitempty"`
	NodePluginTolerations    []Toleration      `json:"nodePluginTolerations,omitempty"`
	ImagePullPolicy          string            `json:"imagePullPolicy"`
	EnableACP                string            `json:"enableACP"`
	ACPImage                 string            `json:"acpImage"`
	ISCSISelfHealingInterval string            `json:"iscsiSelfHealingInterval"`
	ISCSISelfHealingWaitTime string            `json:"iscsiSelfHealingWaitTime"`
	K8sAPIQPS                int               `json:"k8sAPIQPS,omitempty"`
	FSGroupPolicy            string            `json:"fsGroupPolicy,omitempty"`
	NodePrep                 []string          `json:"nodePrep"`
	EnableConcurrency        string            `json:"enableConcurrency"`
	Resources                *config.Resources `json:"resources,omitempty"`
	HTTPSMetrics             string            `json:"httpsMetrics"`
	HostNetwork              bool              `json:"hostNetwork"`
}

/************************
* Trident Configurator
************************/

// TridentConfigurator defines a Trident backend.
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentConfigurator struct {
	metav1.TypeMeta `json:",inline"`
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Input spec for the Trident Backend
	Spec   TridentConfiguratorSpec   `json:"spec"`
	Status TridentConfiguratorStatus `json:"status"`
}

// TridentConfiguratorList is a list of TridentBackend objects.
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TridentConfiguratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of TridentConfigurator objects
	Items []*TridentConfigurator `json:"items"`
}

// TridentConfiguratorSpec defines the desired state of TridentConfigurator
type TridentConfiguratorSpec struct {
	runtime.RawExtension
}

// TridentConfiguratorStatus defines the observed state of TridentConfigurator
type TridentConfiguratorStatus struct {
	BackendNames        []string `json:"backendNames"`
	Message             string   `json:"message"`
	DeletionPolicy      string   `json:"deletionPolicy"`
	Phase               string   `json:"phase"`
	LastOperationStatus string   `json:"lastOperationStatus"`
	CloudProvider       string   `json:"cloudProvider"`
}
