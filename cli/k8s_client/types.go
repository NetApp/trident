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
	v1beta12 "k8s.io/api/storage/v1beta1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"

	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

type KubernetesClient interface {
	Version() *version.Info
	ServerVersion() *utils.Version
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
	GetStatefulSetByLabel(label string, allNamespaces bool) (*appsv1.StatefulSet, error)
	GetStatefulSetsByLabel(label string, allNamespaces bool) ([]appsv1.StatefulSet, error)
	CheckStatefulSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteStatefulSetByLabel(label string) error
	DeleteStatefulSet(name, namespace string) error
	GetDaemonSetByLabel(label string, allNamespaces bool) (*appsv1.DaemonSet, error)
	GetDaemonSetsByLabel(label string, allNamespaces bool) ([]appsv1.DaemonSet, error)
	CheckDaemonSetExists(name, namespace string) (bool, error)
	CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDaemonSetByLabel(label string) error
	DeleteDaemonSet(name, namespace string, foreground bool) error
	PatchDaemonSetByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetConfigMapByLabel(label string, allNamespaces bool) (*v1.ConfigMap, error)
	GetConfigMapsByLabel(label string, allNamespaces bool) ([]v1.ConfigMap, error)
	CheckConfigMapExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteConfigMapByLabel(label string) error
	CreateConfigMapFromDirectory(path, name, label string) error
	GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error)
	GetPodsByLabel(label string, allNamespaces bool) ([]v1.Pod, error)
	CheckPodExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeletePodByLabel(label string) error
	DeletePod(name, namespace string) error
	GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error)
	GetPVCByLabel(label string, allNamespaces bool) (*v1.PersistentVolumeClaim, error)
	CheckPVCExists(pvcName string) (bool, error)
	CheckPVCBound(pvcName string) (bool, error)
	DeletePVCByLabel(label string) error
	GetPV(pvName string) (*v1.PersistentVolume, error)
	GetPVByLabel(label string) (*v1.PersistentVolume, error)
	CheckPVExists(pvName string) (bool, error)
	DeletePVByLabel(label string) error
	GetCRD(crdName string) (*apiextensionv1.CustomResourceDefinition, error)
	CheckCRDExists(crdName string) (bool, error)
	DeleteCRD(crdName string) error
	GetPodSecurityPolicyByLabel(label string) (*v1beta1.PodSecurityPolicy, error)
	GetPodSecurityPoliciesByLabel(label string) ([]v1beta1.PodSecurityPolicy, error)
	CheckPodSecurityPolicyExistsByLabel(label string) (bool, string, error)
	DeletePodSecurityPolicyByLabel(label string) error
	DeletePodSecurityPolicy(pspName string) error
	PatchPodSecurityPolicyByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetServiceAccountByLabel(label string, allNamespaces bool) (*v1.ServiceAccount, error)
	GetServiceAccountsByLabel(label string, allNamespaces bool) ([]v1.ServiceAccount, error)
	CheckServiceAccountExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceAccountByLabel(label string) error
	DeleteServiceAccount(name, namespace string, foreground bool) error
	PatchServiceAccountByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetClusterRoleByLabel(label string) (*v13.ClusterRole, error)
	GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error)
	CheckClusterRoleExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleByLabel(label string) error
	DeleteClusterRole(name string) error
	PatchClusterRoleByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetClusterRoleBindingByLabel(label string) (*v13.ClusterRoleBinding, error)
	GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error)
	CheckClusterRoleBindingExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleBindingByLabel(label string) error
	DeleteClusterRoleBinding(name string) error
	PatchClusterRoleBindingByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	GetBetaCSIDriverByLabel(label string) (*v1beta12.CSIDriver, error)
	GetCSIDriverByLabel(label string) (*storagev1.CSIDriver, error)
	GetBetaCSIDriversByLabel(label string) ([]v1beta12.CSIDriver, error)
	GetCSIDriversByLabel(label string) ([]storagev1.CSIDriver, error)
	CheckBetaCSIDriverExistsByLabel(label string) (bool, string, error)
	CheckCSIDriverExistsByLabel(label string) (bool, string, error)
	DeleteBetaCSIDriverByLabel(label string) error
	DeleteCSIDriverByLabel(label string) error
	DeleteBetaCSIDriver(name string) error
	DeleteCSIDriver(name string) error
	PatchBetaCSIDriverByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	PatchCSIDriverByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	CheckNamespaceExists(namespace string) (bool, error)
	PatchNamespaceLabels(namespace string, labels map[string]string) error
	PatchNamespace(namespace string, patchBytes []byte, patchType types.PatchType) error
	GetNamespace(namespace string) (*v1.Namespace, error)
	CreateSecret(secret *v1.Secret) (*v1.Secret, error)
	UpdateSecret(secret *v1.Secret) (*v1.Secret, error)
	CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string) (*v1.Secret, error)
	GetSecret(secretName string) (*v1.Secret, error)
	GetSecretByLabel(label string, allNamespaces bool) (*v1.Secret, error)
	GetSecretsByLabel(label string, allNamespaces bool) ([]v1.Secret, error)
	CheckSecretExists(secretName string) (bool, error)
	DeleteSecretDefault(secretName string) error
	DeleteSecretByLabel(label string) error
	DeleteSecret(name, namespace string) error
	PatchSecretByLabel(label string, patchBytes []byte, patchType types.PatchType) error
	CreateObjectByFile(filePath string) error
	CreateObjectByYAML(yaml string) error
	DeleteObjectByFile(filePath string, ignoreNotFound bool) error
	DeleteObjectByYAML(yaml string, ignoreNotFound bool) error
	RemoveTridentUserFromOpenShiftSCC(user, scc string) error
	GetOpenShiftSCCByName(user, scc string) (bool, bool, []byte, error)
	PatchOpenShiftSCC(newJSONData []byte) error
	FollowPodLogs(pod, container, namespace string, logLineCallback LogLineCallback)
	AddFinalizerToCRD(crdName string) error
	AddFinalizerToCRDs(CRDnames []string) error
	RemoveFinalizerFromCRD(crdName string) error
	GetCRDClient() (*crdclient.Clientset, error)
	IsTopologyInUse() (bool, error)
	GetSnapshotterCRDVersion() string
}

type DeploymentYAMLArguments struct {
	DeploymentName          string              `json:"deploymentName"`
	TridentImage            string              `json:"tridentImage"`
	AutosupportImage        string              `json:"autosupportImage"`
	AutosupportProxy        string              `json:"autosupportProxy"`
	AutosupportCustomURL    string              `json:"autosupportCustomURL"`
	AutosupportSerialNumber string              `json:"autosupportSerialNumber"`
	AutosupportHostname     string              `json:"autosupportHostname"`
	ImageRegistry           string              `json:"imageRegistry"`
	LogFormat               string              `json:"logFormat"`
	SnapshotCRDVersion      string              `json:"snapshotCRDVersion"`
	ImagePullSecrets        []string            `json:"imagePullSecrets"`
	Labels                  map[string]string   `json:"labels"`
	ControllingCRDetails    map[string]string   `json:"controllingCRDetails"`
	Debug                   bool                `json:"debug"`
	UseIPv6                 bool                `json:"useIPv6"`
	SilenceAutosupport      bool                `json:"silenceAutosupport"`
	Version                 *utils.Version      `json:"version"`
	TopologyEnabled         bool                `json:"topologyEnabled"`
	HTTPRequestTimeout      string              `json:"httpRequestTimeout"`
	NodeSelector            map[string]string   `json:"nodeSelector"`
	Tolerations             []map[string]string `json:"tolerations"`
}

type DaemonsetYAMLArguments struct {
	DaemonsetName        string              `json:"daemonsetName"`
	TridentImage         string              `json:"tridentImage"`
	ImageRegistry        string              `json:"imageRegistry"`
	KubeletDir           string              `json:"kubeletDir"`
	LogFormat            string              `json:"logFormat"`
	ProbePort            string              `json:"probePort"`
	ImagePullSecrets     []string            `json:"imagePullSecrets"`
	Labels               map[string]string   `json:"labels"`
	ControllingCRDetails map[string]string   `json:"controllingCRDetails"`
	Debug                bool                `json:"debug"`
	Version              *utils.Version      `json:"version"`
	HTTPRequestTimeout   string              `json:"httpRequestTimeout"`
	NodeSelector         map[string]string   `json:"nodeSelector"`
	Tolerations          []map[string]string `json:"tolerations"`
}
