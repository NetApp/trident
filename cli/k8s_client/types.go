// Copyright 2022 NetApp, Inc. All Rights Reserved.

package k8sclient

//go:generate mockgen -destination=../../mocks/mock_cli/mock_k8s_client/mock_k8s_client.go github.com/netapp/trident/cli/k8s_client KubernetesClient

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v13 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"

	versionutils "github.com/netapp/trident/utils/version"
)

type KubernetesClient interface {
	Version() *version.Info
	ServerVersion() *versionutils.Version
	Namespace() string
	SetNamespace(namespace string)
	SetTimeout(time.Duration)
	Flavor() OrchestratorFlavor
	CLI() string
	Exec(podName, containerName string, commandArgs []string) ([]byte, error)
	GetDeploymentByLabel(label string, allNamespaces bool) (*appsv1.Deployment, error)
	GetDeploymentsByLabel(label string, allNamespaces bool) ([]appsv1.Deployment, error)
	CheckDeploymentExists(name, namespace string) (bool, error)
	CheckDeploymentExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDeploymentByLabel(label string) error
	DeleteDeployment(name, namespace string, foreground bool) error
	PatchDeploymentByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetServiceByLabel(label string, allNamespaces bool) (*v1.Service, error)
	GetServicesByLabel(label string, allNamespaces bool) ([]v1.Service, error)
	CheckServiceExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceByLabel(label string) error
	DeleteService(name, namespace string) error
	PatchServiceByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetDaemonSetByLabel(label string, allNamespaces bool) (*appsv1.DaemonSet, error)
	GetDaemonSetByLabelAndName(label, name string, allNamespaces bool) (*appsv1.DaemonSet, error)
	GetDaemonSetsByLabel(label string, allNamespaces bool) ([]appsv1.DaemonSet, error)
	CheckDaemonSetExists(name, namespace string) (bool, error)
	CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDaemonSetByLabel(label string) error
	DeleteDaemonSetByLabelAndName(label, name string) error
	DeleteDaemonSet(name, namespace string, foreground bool) error
	PatchDaemonSetByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchDaemonSetByLabelAndName(label, daemonSetName string, patchBytes []byte, patchType types.PatchType) error
	GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error)
	GetPodsByLabel(label string, allNamespaces bool) ([]v1.Pod, error)
	CheckPodExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeletePodByLabel(label string) error
	DeletePod(name, namespace string) error
	GetCRD(crdName string) (*apiextensionv1.CustomResourceDefinition, error)
	CheckCRDExists(crdName string) (bool, error)
	PatchCRD(crdName string, patchBytes []byte, patchType types.PatchType) error
	DeleteCRD(crdName string) error
	GetPodSecurityPolicyByLabel(label string) (*v1beta1.PodSecurityPolicy, error)
	GetPodSecurityPoliciesByLabel(label string) ([]v1beta1.PodSecurityPolicy, error)
	CheckPodSecurityPolicyExistsByLabel(label string) (bool, string, error)
	DeletePodSecurityPolicyByLabel(label string) error
	DeletePodSecurityPolicy(pspName string) error
	PatchPodSecurityPolicyByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchPodSecurityPolicyByLabelAndName(label, pspName string, patchBytes []byte, patchType types.PatchType) error
	GetServiceAccountByLabel(label string, allNamespaces bool) (*v1.ServiceAccount, error)
	GetServiceAccountByLabelAndName(label, serviceAccountName string, allNamespaces bool) (*v1.ServiceAccount, error)
	GetServiceAccountsByLabel(label string, allNamespaces bool) ([]v1.ServiceAccount, error)
	CheckServiceAccountExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceAccountByLabel(label string) error
	DeleteServiceAccount(name, namespace string, foreground bool) error
	PatchServiceAccountByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchServiceAccountByLabelAndName(label, serviceAccountName string, patchBytes []byte, patchType types.PatchType) error
	GetClusterRoleByLabelAndName(label, clusterRoleName string) (*v13.ClusterRole, error)
	GetClusterRoleByLabel(label string) (*v13.ClusterRole, error)
	GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error)
	GetRolesByLabel(label string) ([]v13.Role, error)
	CheckClusterRoleExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleByLabel(label string) error
	DeleteClusterRole(name string) error
	DeleteRole(name string) error
	PatchClusterRoleByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchClusterRoleByLabelAndName(label, clusterRoleName string, patchBytes []byte, patchType types.PatchType) error
	PatchRoleByLabelAndName(label, roleName string, patchBytes []byte, patchType types.PatchType) error
	GetClusterRoleBindingByLabelAndName(label, clusterRoleBindingName string) (*v13.ClusterRoleBinding, error)
	GetClusterRoleBindingByLabel(label string) (*v13.ClusterRoleBinding, error)
	GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error)
	GetRoleBindingByLabelAndName(label, roleBindingName string) (*v13.RoleBinding, error)
	GetRoleBindingsByLabel(label string) ([]v13.RoleBinding, error)
	CheckClusterRoleBindingExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleBindingByLabel(label string) error
	DeleteClusterRoleBinding(name string) error
	DeleteRoleBinding(name string) error
	PatchClusterRoleBindingByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchClusterRoleBindingByLabelAndName(label, clusterRoleBindingName string, patchBytes []byte, patchType types.PatchType) error
	PatchRoleBindingByLabelAndName(label, roleBindingName string, patchBytes []byte, patchType types.PatchType) error
	GetCSIDriverByLabel(label string) (*storagev1.CSIDriver, error)
	GetCSIDriversByLabel(label string) ([]storagev1.CSIDriver, error)
	CheckCSIDriverExistsByLabel(label string) (bool, string, error)
	DeleteCSIDriverByLabel(label string) error
	DeleteCSIDriver(name string) error
	PatchCSIDriverByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	CheckNamespaceExists(namespace string) (bool, error)
	PatchNamespaceLabels(namespace string, labels map[string]string) error
	PatchNamespace(namespace string, patchBytes []byte, patchType types.PatchType) error
	GetNamespace(namespace string) (*v1.Namespace, error)
	CreateSecret(secret *v1.Secret) (*v1.Secret, error)
	UpdateSecret(secret *v1.Secret) (*v1.Secret, error)
	GetSecret(secretName string) (*v1.Secret, error)
	GetSecretByLabel(label string, allNamespaces bool) (*v1.Secret, error)
	GetSecretsByLabel(label string, allNamespaces bool) ([]v1.Secret, error)
	CheckSecretExists(secretName string) (bool, error)
	DeleteSecretDefault(secretName string) error
	DeleteSecretByLabel(label string) error
	DeleteSecret(name, namespace string) error
	PatchSecretByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetResourceQuota(label string) (*v1.ResourceQuota, error)
	GetResourceQuotaByLabel(label string) (*v1.ResourceQuota, error)
	GetResourceQuotasByLabel(label string) ([]v1.ResourceQuota, error)
	DeleteResourceQuota(name string) error
	DeleteResourceQuotaByLabel(label string) error
	PatchResourceQuotaByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	CreateObjectByFile(filePath string) error
	CreateObjectByYAML(yaml string) error
	DeleteObjectByFile(filePath string, ignoreNotFound bool) error
	DeleteObjectByYAML(yaml string, ignoreNotFound bool) error
	RemoveTridentUserFromOpenShiftSCC(user, scc string) error
	GetOpenShiftSCCByName(user, scc string) (bool, bool, []byte, error)
	PatchOpenShiftSCC(newJSONData []byte) error
	AddFinalizerToCRD(crdName string) error
	AddFinalizerToCRDs(CRDnames []string) error
	RemoveFinalizerFromCRD(crdName string) error
	IsTopologyInUse() (bool, error)
}

type DeploymentYAMLArguments struct {
	DeploymentName          string                `json:"deploymentName"`
	TridentImage            string                `json:"tridentImage"`
	AutosupportImage        string                `json:"autosupportImage"`
	AutosupportProxy        string                `json:"autosupportProxy"`
	AutosupportCustomURL    string                `json:"autosupportCustomURL"`
	AutosupportSerialNumber string                `json:"autosupportSerialNumber"`
	AutosupportHostname     string                `json:"autosupportHostname"`
	ImageRegistry           string                `json:"imageRegistry"`
	LogFormat               string                `json:"logFormat"`
	LogLevel                string                `json:"logLevel"`
	LogWorkflows            string                `json:"logWorkflows"`
	LogLayers               string                `json:"logLayers"`
	ImagePullSecrets        []string              `json:"imagePullSecrets"`
	Labels                  map[string]string     `json:"labels"`
	ControllingCRDetails    map[string]string     `json:"controllingCRDetails"`
	Debug                   bool                  `json:"debug"`
	UseIPv6                 bool                  `json:"useIPv6"`
	SilenceAutosupport      bool                  `json:"silenceAutosupport"`
	Version                 *versionutils.Version `json:"version"`
	TopologyEnabled         bool                  `json:"topologyEnabled"`
	DisableAuditLog         bool                  `json:"disableAuditLog"`
	HTTPRequestTimeout      string                `json:"httpRequestTimeout"`
	NodeSelector            map[string]string     `json:"nodeSelector"`
	Tolerations             []map[string]string   `json:"tolerations"`
	ServiceAccountName      string                `json:"serviceAccountName"`
	ImagePullPolicy         string                `json:"imagePullPolicy"`
	EnableForceDetach       bool                  `json:"enableForceDetach"`
	CloudProvider           string                `json:"cloudProvider"`
}

type DaemonsetYAMLArguments struct {
	DaemonsetName        string                `json:"daemonsetName"`
	TridentImage         string                `json:"tridentImage"`
	ImageRegistry        string                `json:"imageRegistry"`
	KubeletDir           string                `json:"kubeletDir"`
	LogFormat            string                `json:"logFormat"`
	LogLevel             string                `json:"logLevel"`
	LogWorkflows         string                `json:"logWorkflows"`
	LogLayers            string                `json:"logLayers"`
	ProbePort            string                `json:"probePort"`
	ImagePullSecrets     []string              `json:"imagePullSecrets"`
	Labels               map[string]string     `json:"labels"`
	ControllingCRDetails map[string]string     `json:"controllingCRDetails"`
	EnableForceDetach    bool                  `json:"enableForceDetach"`
	DisableAuditLog      bool                  `json:"disableAuditLog"`
	Debug                bool                  `json:"debug"`
	Version              *versionutils.Version `json:"version"`
	HTTPRequestTimeout   string                `json:"httpRequestTimeout"`
	NodeSelector         map[string]string     `json:"nodeSelector"`
	Tolerations          []map[string]string   `json:"tolerations"`
	ServiceAccountName   string                `json:"serviceAccountName"`
	ImagePullPolicy      string                `json:"imagePullPolicy"`
}

type TridentVersionPodYAMLArguments struct {
	TridentVersionPodName string              `json:"tridentVersionPodName"`
	TridentImage          string              `json:"tridentImage"`
	ImagePullSecrets      []string            `json:"imagePullSecrets"`
	Labels                map[string]string   `json:"labels"`
	ControllingCRDetails  map[string]string   `json:"controllingCRDetails"`
	Tolerations           []map[string]string `json:"tolerations"`
	ServiceAccountName    string              `json:"serviceAccountName"`
	ImagePullPolicy       string              `json:"imagePullPolicy"`
}
