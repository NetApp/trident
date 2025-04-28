// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

//go:generate mockgen -destination=../../../../mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer/mock_installer.go github.com/netapp/trident/operator/controllers/orchestrator/installer TridentInstaller,ExtendedK8sClient

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	v15 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

// TridentInstaller is responsible for installing, patching, or uninstalling orchestrator controlled instances of Trident.
type TridentInstaller interface {
	CreateOrPatchCRD(crdName, crdYAML string, performOperationOnce bool) error
	InstallOrPatchTrident(
		cr v15.TridentOrchestrator, currentInstallationVersion string, shouldUpdate, crdUpdateNeeded bool,
	) (*v15.TridentOrchestratorSpecValues, string, string, error)
	ObliviateCRDs(skipCRDs []string) error
	TridentDeploymentInformation(deploymentLabel string) (*appsv1.Deployment, []appsv1.Deployment, bool, error)
	TridentDaemonSetInformation() (*appsv1.DaemonSet, []appsv1.DaemonSet, bool, error)
	UninstallTrident() error
	GetACPVersion() string
}

// ExtendedK8sClient extends the vanilla k8s client Interface and is responsible for enabling the TridentInstaller.
type ExtendedK8sClient interface {
	k8sclient.KubernetesClient

	CreateCustomResourceDefinition(crdName, crdYAML string) error
	PutCustomResourceDefinition(
		currentCRD *apiextensionv1.CustomResourceDefinition, crdName string, createCRD bool, newCRDYAML string,
	) error
	WaitForCRDEstablished(crdName string, timeout time.Duration) error
	DeleteCustomResourceDefinition(crdName, crdYAML string) error

	GetCSIDriverInformation(
		csiDriverName, appLabel string, shouldUpdate bool,
	) (*storagev1.CSIDriver, []storagev1.CSIDriver, bool, error)
	PutCSIDriver(currentCSIDriver *storagev1.CSIDriver, createCSIDriver bool, newCSIDriverYAML, appLabel string) error
	DeleteCSIDriverCR(csiDriverName, appLabel string) error
	RemoveMultipleCSIDriverCRs(unwantedCSIDriverCRs []storagev1.CSIDriver) error

	GetClusterRoleInformation(
		clusterRoleName, appLabel string, shouldUpdate bool,
	) (*rbacv1.ClusterRole, []rbacv1.ClusterRole, bool, error)
	PutClusterRole(
		currentClusterRole *rbacv1.ClusterRole, createClusterRole bool, newClusterRoleYAML, appLabel string,
	) error
	DeleteTridentClusterRole(clusterRoleName, appLabel string) error
	RemoveMultipleClusterRoles(unwantedClusterRoles []rbacv1.ClusterRole) error

	GetClusterRoleBindingInformation(
		clusterRoleBindingName, appLabel string, shouldUpdate bool,
	) (*rbacv1.ClusterRoleBinding, []rbacv1.ClusterRoleBinding, bool, error)
	PutClusterRoleBinding(
		currentClusterRoleBinding *rbacv1.ClusterRoleBinding, createClusterRoleBinding bool,
		newClusterRoleBindingYAML, appLabel string,
	) error
	DeleteTridentClusterRoleBinding(clusterRoleBindingName, appLabel string) error
	RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding) error

	GetMultipleRoleInformation(
		roleNames []string, appLabel string, shouldUpdate bool,
	) (map[string]*rbacv1.Role, []rbacv1.Role, map[string]bool, error)
	PutRole(currentRole *rbacv1.Role, reuseRole bool, newRoleYAML, appLabel string) error
	DeleteMultipleTridentRoles(roleNames []string, appLabel string) error
	RemoveMultipleRoles(unwantedRoles []rbacv1.Role) error

	GetMultipleRoleBindingInformation(
		roleBindingNames []string, appLabel string, shouldUpdate bool,
	) (map[string]*rbacv1.RoleBinding, []rbacv1.RoleBinding, map[string]bool, error)
	PutRoleBinding(
		currentRoleBinding *rbacv1.RoleBinding, reuseRoleBinding bool, newRoleBindingYAML, appLabel string,
	) error
	DeleteMultipleTridentRoleBindings(roleBindingNames []string, appLabel string) error
	RemoveMultipleRoleBindings(unwantedRoleBindings []rbacv1.RoleBinding) error

	GetResourceQuotaInformation(
		resourceQuotaName, appLabel, namespace string,
	) (*corev1.ResourceQuota, []corev1.ResourceQuota, bool, error)
	PutResourceQuota(
		currentResourceQuota *corev1.ResourceQuota, createResourceQuota bool, newDeploymentYAML, appLabel string,
	) error
	DeleteTridentResourceQuota(nodeLabel string) error
	RemoveMultipleResourceQuotas(unwantedResourceQuotas []corev1.ResourceQuota) error

	GetDaemonSetInformation(
		nodeLabel, namespace string, isWindows bool,
	) (*appsv1.DaemonSet, []appsv1.DaemonSet, bool, error)
	PutDaemonSet(
		currentDaemonSet *appsv1.DaemonSet, createDaemonSet bool, newDaemonSetYAML, nodeLabel, daemonSetName string,
	) error
	DeleteTridentDaemonSet(nodeLabel string) error
	RemoveMultipleDaemonSets(unwantedDaemonSets []appsv1.DaemonSet) error

	GetDeploymentInformation(
		deploymentName, appLabel, namespace string,
	) (*appsv1.Deployment, []appsv1.Deployment, bool, error)
	PutDeployment(currentDeployment *appsv1.Deployment, createDeployment bool, newDeploymentYAML, appLabel string) error
	DeleteTridentDeployment(appLabel string) error
	RemoveMultipleDeployments(unwantedDeployments []appsv1.Deployment) error

	GetSecretInformation(
		secretName, appLabel, namespace string, shouldUpdate bool,
	) (*corev1.Secret, []corev1.Secret, bool, error)
	PutSecret(createSecret bool, newSecretYAML, secretName string) error
	DeleteTridentSecret(secretName, appLabel, namespace string) error
	RemoveMultipleSecrets(unwantedSecrets []corev1.Secret) error

	GetServiceInformation(
		serviceName, appLabel, namespace string, shouldUpdate bool,
	) (*corev1.Service, []corev1.Service, bool, error)
	PutService(currentService *corev1.Service, createService bool, newServiceYAML, appLabel string) error
	DeleteTridentService(serviceName, appLabel, namespace string) error
	RemoveMultipleServices(unwantedServices []corev1.Service) error

	GetMultipleServiceAccountInformation(
		serviceAccountNames []string, appLabel, namespace string, shouldUpdate bool,
	) (map[string]*corev1.ServiceAccount, []corev1.ServiceAccount, map[string][]string, map[string]bool, error)
	PutServiceAccount(
		currentServiceAccount *corev1.ServiceAccount, reuseServiceAccount bool, newServiceAccountYAML, appLabel string,
	) (bool, error)
	DeleteMultipleTridentServiceAccounts(serviceAccountNames []string, appLabel, namespace string) error
	RemoveMultipleServiceAccounts(unwantedServiceAccounts []corev1.ServiceAccount) error

	GetMultipleTridentOpenShiftSCCInformation(
		openShiftSCCNames, openShiftSCCUserNames []string, shouldUpdate bool,
	) (map[string][]byte, map[string]bool, map[string]bool, error)
	PutOpenShiftSCC(currentOpenShiftSCCJSON []byte, createOpenShiftSCC bool, newOpenShiftSCCYAML string) error
	DeleteMultipleOpenShiftSCC(openShiftSCCUserNames, openShiftSCCNames []string, appLabelValue string) error

	DeleteTransientVersionPod(tridentVersionPodLabel string) error
	RemoveMultiplePods(unwantedPods []corev1.Pod) error

	ExecPodForVersionInformation(podName string, cmd []string, timeout time.Duration) ([]byte, error)
}
