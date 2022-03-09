package installer

//go:generate mockgen -destination=../../../../mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer/mock_installer.go github.com/netapp/trident/operator/controllers/orchestrator/installer TridentInstaller,ExtendedK8sClient

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	v15 "github.com/netapp/trident/operator/controllers/orchestrator/apis/netapp/v1"
)

// TridentInstaller is responsible for installing, patching, or uninstalling orchestrator controlled instances of Trident.
type TridentInstaller interface {
	CreateCRD(crdName, crdYAML string) error
	InstallOrPatchTrident(cr v15.TridentOrchestrator, currentInstallationVersion string, shouldUpdate bool) (*v15.TridentOrchestratorSpecValues,
		string, error)
	ObliviateCRDs() error
	TridentDeploymentInformation(deploymentLabel string) (*appsv1.Deployment, []appsv1.Deployment, bool, error)
	TridentDaemonSetInformation() (*appsv1.DaemonSet, []appsv1.DaemonSet, bool, error)
	UninstallTrident() error
	UninstallCSIPreviewTrident() error
	UninstallLegacyTrident() error
}

// ExtendedK8sClient extends the vanilla k8s client Interface and is responsible for enabling the TridentInstaller.
type ExtendedK8sClient interface {
	k8sclient.KubernetesClient

	CreateCustomResourceDefinition(crdName, crdYAML string) error
	WaitForCRDEstablished(crdName string, timeout time.Duration) error
	DeleteCustomResourceDefinition(crdName, crdYAML string) error

	GetBetaCSIDriverInformation(csiDriverName, appLabel string, shouldUpdate bool) (*storagev1beta1.CSIDriver,
		[]storagev1beta1.CSIDriver, bool, error)
	PutBetaCSIDriver(currentCSIDriver *storagev1beta1.CSIDriver, createCSIDriver bool, newCSIDriverYAML, appLabel string) error
	DeleteBetaCSIDriverCR(csiDriverName, appLabel string) error
	RemoveMultipleBetaCSIDriverCRs(unwantedCSIDriverCRs []storagev1beta1.CSIDriver) error

	GetCSIDriverInformation(csiDriverName, appLabel string, shouldUpdate bool) (*storagev1.CSIDriver,
		[]storagev1.CSIDriver, bool, error)
	PutCSIDriver(currentCSIDriver *storagev1.CSIDriver, createCSIDriver bool, newCSIDriverYAML, appLabel string) error
	DeleteCSIDriverCR(csiDriverName, appLabel string) error
	RemoveMultipleCSIDriverCRs(unwantedCSIDriverCRs []storagev1.CSIDriver) error

	GetClusterRoleInformation(clusterRoleName, appLabel string, shouldUpdate bool) (*rbacv1.ClusterRole,
		[]rbacv1.ClusterRole, bool, error)
	PutClusterRole(currentClusterRole *rbacv1.ClusterRole, createClusterRole bool, newClusterRoleYAML, appLabel string) error
	DeleteTridentClusterRole(clusterRoleName, appLabel string) error
	RemoveMultipleClusterRoles(unwantedClusterRoles []rbacv1.ClusterRole) error

	GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel string, shouldUpdate bool) (*rbacv1.ClusterRoleBinding,
		[]rbacv1.ClusterRoleBinding, bool, error)
	PutClusterRoleBinding(currentClusterRoleBinding *rbacv1.ClusterRoleBinding, createClusterRoleBinding bool, newClusterRoleBindingYAML, appLabel string) error
	DeleteTridentClusterRoleBinding(clusterRoleBindingName, appLabel string) error
	RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding) error

	GetDaemonSetInformation(daemonSetName, nodeLabel, namespace string) (*appsv1.DaemonSet, []appsv1.DaemonSet, bool,
		error)
	PutDaemonSet(currentDaemonSet *appsv1.DaemonSet, createDaemonSet bool, newDaemonSetYAML, nodeLabel string) error
	DeleteTridentDaemonSet(nodeLabel string) error
	RemoveMultipleDaemonSets(unwantedDaemonSets []appsv1.DaemonSet) error

	GetDeploymentInformation(deploymentName, appLabel, namespace string) (*appsv1.Deployment, []appsv1.Deployment, bool,
		error)
	PutDeployment(currentDeployment *appsv1.Deployment, createDeployment bool, newDeploymentYAML, appLabel string) error
	DeleteTridentDeployment(appLabel string) error
	RemoveMultipleDeployments(unwantedDeployments []appsv1.Deployment) error

	GetPodSecurityPolicyInformation(pspName, appLabel string, shouldUpdate bool) (*policyv1beta1.PodSecurityPolicy,
		[]policyv1beta1.PodSecurityPolicy, bool, error)
	PutPodSecurityPolicy(currentPSP *policyv1beta1.PodSecurityPolicy, createPSP bool, newPSPYAML, appLabel string) error
	DeleteTridentPodSecurityPolicy(pspName, appLabel string) error
	RemoveMultiplePodSecurityPolicies(unwantedPSPs []policyv1beta1.PodSecurityPolicy) error

	GetSecretInformation(secretName, appLabel, namespace string, shouldUpdate bool) (*corev1.Secret,
		[]corev1.Secret, bool, error)
	PutSecret(createSecret bool, newSecretYAML string, secretName string) error
	DeleteTridentSecret(secretName, appLabel, namespace string) error
	RemoveMultipleSecrets(unwantedSecrets []corev1.Secret) error

	GetServiceInformation(serviceName, appLabel, namespace string, shouldUpdate bool) (*corev1.Service,
		[]corev1.Service, bool, error)
	PutService(currentService *corev1.Service, createService bool, newServiceYAML, appLabel string) error
	DeleteTridentService(serviceName, appLabel, namespace string) error
	RemoveMultipleServices(unwantedServices []corev1.Service) error

	GetServiceAccountInformation(serviceAccountName, appLabel, namespace string, shouldUpdate bool) (*corev1.ServiceAccount,
		[]corev1.ServiceAccount, []string, bool, error)
	PutServiceAccount(currentServiceAccount *corev1.ServiceAccount, createServiceAccount bool, newServiceAccountYAML, appLabel string) (bool,
		error)
	DeleteTridentServiceAccount(serviceAccountName, appLabel, namespace string) error
	RemoveMultipleServiceAccounts(unwantedServiceAccounts []corev1.ServiceAccount) error

	GetTridentOpenShiftSCCInformation(openShiftSCCName, openShiftSCCUserName string, shouldUpdate bool) ([]byte, bool,
		bool, error)
	PutOpenShiftSCC(currentOpenShiftSCCJSON []byte,
		createOpenShiftSCC bool, newOpenShiftSCCYAML string) error
	DeleteOpenShiftSCC(openShiftSCCUserName, openShiftSCCName,
		appLabelValue string) error

	DeleteTridentStatefulSet(appLabel string) error
	RemoveMultipleStatefulSets(unwantedStatefulSets []appsv1.StatefulSet) error

	DeleteTransientVersionPod(tridentVersionPodLabel string) error
	RemoveMultiplePods(unwantedPods []corev1.Pod) error

	ExecPodForVersionInformation(podName string, cmd []string, timeout time.Duration) ([]byte, error)
	GetCSISnapshotterVersion(currentDeployment *appsv1.Deployment) string
}
