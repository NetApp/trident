// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	storagev1 "k8s.io/api/storage/v1"

	commonconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/yaml"
)

const (
	TridentAppLabelKey       = "app"
	DefaultContainerLabelKey = "kubectl.kubernetes.io/default-container"
)

func GetNamespaceYAML(namespace string) string {
	return strings.ReplaceAll(namespaceYAMLTemplate, "{NAMESPACE}", namespace)
}

func isControllerRBACResource(labels map[string]string) bool {
	return strings.HasPrefix(labels[TridentAppLabelKey], "controller")
}

const namespaceYAMLTemplate = `---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    pod-security.kubernetes.io/enforce: privileged
  name: {NAMESPACE}
`

func GetServiceAccountYAML(
	serviceAccountName string, secrets []string, labels, controllingCRDetails map[string]string, cloudIdentity string,
) string {
	var saYAML string
	Log().WithFields(LogFields{
		"ServiceAccountName":   serviceAccountName,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetServiceAccountYAML")
	defer func() { Log().Trace("<<<< GetServiceAccountYAML") }()

	if len(secrets) > 0 {
		saYAML = serviceAccountWithSecretYAML
	} else {
		saYAML = serviceAccountYAML
	}

	if cloudIdentity != "" {
		saYAML = strings.ReplaceAll(saYAML, "{CLOUD_IDENTITY}", constructServiceAccountAnnotation(cloudIdentity))
	} else {
		saYAML = strings.ReplaceAll(saYAML, "{CLOUD_IDENTITY}", "")
	}

	saYAML = strings.ReplaceAll(saYAML, "{NAME}", serviceAccountName)
	saYAML = yaml.ReplaceMultilineTag(saYAML, "LABELS", constructLabels(labels))
	saYAML = yaml.ReplaceMultilineTag(saYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	// Log().service account YAML before the secrets are added.
	Log().WithField("yaml", saYAML).Trace("Service account YAML.")
	saYAML = strings.Replace(saYAML, "{SECRETS}", constructServiceAccountSecrets(secrets), 1)
	return saYAML
}

const serviceAccountYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {NAME}
  {CLOUD_IDENTITY}
  {LABELS}
  {OWNER_REF}
`

const serviceAccountWithSecretYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {NAME}
  {CLOUD_IDENTITY}
  {LABELS}
  {OWNER_REF}
{SECRETS}
`

func GetClusterRoleYAML(clusterRoleName string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"ClusterRoleName":      clusterRoleName,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetClusterRoleYAML")
	defer func() { Log().Trace("<<<< GetClusterRoleYAML") }()

	clusterRoleYAML := controllerClusterRoleCSIYAMLTemplate
	clusterRoleYAML = strings.ReplaceAll(clusterRoleYAML, "{CLUSTER_ROLE_NAME}", clusterRoleName)
	clusterRoleYAML = yaml.ReplaceMultilineTag(clusterRoleYAML, "LABELS", constructLabels(labels))
	clusterRoleYAML = yaml.ReplaceMultilineTag(clusterRoleYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))
	Log().WithField("yaml", clusterRoleYAML).Trace("Cluster role YAML.")
	return clusterRoleYAML
}

// Specific permissions for sidecars
// csi-resizer needs 'list' for pods
// trident-autosupport needs 'get' for namespace resource cluster-wide
const controllerClusterRoleCSIYAMLTemplate = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {CLUSTER_ROLE_NAME}
  {LABELS}
  {OWNER_REF}
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["resourcequotas"]
    verbs: ["get", "list", "delete", "patch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots", "volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots/status", "volumesnapshotcontents/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["groupsnapshot.storage.k8s.io"]
    resources: ["volumegroupsnapshots"]
    verbs: ["list"]
  - apiGroups: ["groupsnapshot.storage.k8s.io"]
    resources: ["volumegroupsnapshotclasses"]
    verbs: ["list", "watch"]
  - apiGroups: ["groupsnapshot.storage.k8s.io"]
    resources: ["volumegroupsnapshotcontents/status"]
    verbs: ["update"]
  - apiGroups: ["groupsnapshot.storage.k8s.io"]
    resources: ["volumegroupsnapshotcontents"]
    verbs: ["get", "list", "watch", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["csinodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["trident.netapp.io"]
    resources: ["tridentversions", "tridentbackends", "tridentstorageclasses", "tridentvolumes","tridentnodes",
"tridenttransactions", "tridentsnapshots", "tridentbackendconfigs", "tridentbackendconfigs/status",
"tridentmirrorrelationships", "tridentmirrorrelationships/status", "tridentsnapshotinfos",
"tridentsnapshotinfos/status", "tridentvolumepublications", "tridentvolumereferences",
"tridentactionmirrorupdates", "tridentactionmirrorupdates/status",
"tridentactionsnapshotrestores", "tridentactionsnapshotrestores/status",
"tridentgroupsnapshots", "tridentgroupsnapshots/status"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    verbs: ["use"]
    resourceNames:
      - {CLUSTER_ROLE_NAME}
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["get"]
`

func GetRoleYAML(namespace, roleName string, labels, controllingCRDetails map[string]string) string {
	var roleYAML string

	if isControllerRBACResource(labels) {
		roleYAML = controllerRoleCSIYAMLTemplate
	} else {
		roleYAML = nodeRoleCSIYAMLTemplate
	}

	roleYAML = strings.ReplaceAll(roleYAML, "{ROLE_NAME}", roleName)
	roleYAML = strings.ReplaceAll(roleYAML, "{NAMESPACE}", namespace)
	roleYAML = yaml.ReplaceMultilineTag(roleYAML, "LABELS", constructLabels(labels))
	roleYAML = yaml.ReplaceMultilineTag(roleYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	return roleYAML
}

// trident-autosupport needs 'get' for pod/log resources in the 'trident' namespace
const controllerRoleCSIYAMLTemplate = `---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: {NAMESPACE}
  name: {ROLE_NAME}
  {LABELS}
  {OWNER_REF}
rules:
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
`

const nodeRoleCSIYAMLTemplate = `---
kind: Role
apiVersion: "rbac.authorization.k8s.io/v1"
metadata:
  namespace: {NAMESPACE}
  name: {ROLE_NAME}
  {LABELS}
  {OWNER_REF}
rules:
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    verbs: ["use"]
    resourceNames:
      - {ROLE_NAME}
`

func GetRoleBindingYAML(namespace, name string, labels, controllingCRDetails map[string]string) string {
	rbYAML := roleBindingKubernetesV1YAMLTemplate
	rbYAML = strings.ReplaceAll(rbYAML, "{NAMESPACE}", namespace)
	rbYAML = strings.ReplaceAll(rbYAML, "{NAME}", name)
	rbYAML = yaml.ReplaceMultilineTag(rbYAML, "LABELS", constructLabels(labels))
	rbYAML = yaml.ReplaceMultilineTag(rbYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))
	return rbYAML
}

const roleBindingKubernetesV1YAMLTemplate = `---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {NAME}
  namespace: {NAMESPACE}
  {LABELS}
  {OWNER_REF}
subjects:
  - kind: ServiceAccount
    name: {NAME}
    apiGroup: ""
roleRef:
  kind: Role
  name: {NAME}
  apiGroup: rbac.authorization.k8s.io
`

func GetClusterRoleBindingYAML(
	namespace, name string, flavor OrchestratorFlavor, labels, controllingCRDetails map[string]string,
) string {
	Log().WithFields(LogFields{
		"Namespace":            namespace,
		"Name":                 name,
		"Flavor":               flavor,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetClusterRoleBindingYAML")
	defer func() { Log().Trace("<<<< GetClusterRoleBindingYAML") }()

	crbYAML := clusterRoleBindingKubernetesV1YAMLTemplate

	crbYAML = strings.ReplaceAll(crbYAML, "{NAMESPACE}", namespace)
	crbYAML = strings.ReplaceAll(crbYAML, "{NAME}", name)
	crbYAML = yaml.ReplaceMultilineTag(crbYAML, "LABELS", constructLabels(labels))
	crbYAML = yaml.ReplaceMultilineTag(crbYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", crbYAML).Trace("Cluster role binding YAML.")
	return crbYAML
}

const clusterRoleBindingKubernetesV1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {NAME}
  {LABELS}
  {OWNER_REF}
subjects:
  - kind: ServiceAccount
    name: {NAME}
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: {NAME}
  apiGroup: rbac.authorization.k8s.io
`

func GetCSIServiceYAML(serviceName string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"ServiceName":          serviceName,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetCSIServiceYAML")
	defer func() { Log().Trace("<<<< GetCSIServiceYAML") }()

	serviceYAML := strings.ReplaceAll(serviceYAMLTemplate, "{LABEL_APP}", labels[TridentAppLabelKey])
	serviceYAML = strings.ReplaceAll(serviceYAML, "{SERVICE_NAME}", serviceName)
	serviceYAML = yaml.ReplaceMultilineTag(serviceYAML, "LABELS", constructLabels(labels))
	serviceYAML = yaml.ReplaceMultilineTag(serviceYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", serviceYAML).Trace("CSI Service YAML.")
	return serviceYAML
}

const serviceYAMLTemplate = `---
apiVersion: v1
kind: Service
metadata:
  name: {SERVICE_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  selector:
    app: {LABEL_APP}
  ports:
  - name: https
    protocol: TCP
    port: 34571
    targetPort: 8443
  - name: metrics
    protocol: TCP
    port: 9220
    targetPort: 8001
`

func GetResourceQuotaYAML(resourceQuotaName, namespace string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"ResourceQuotaName":    resourceQuotaName,
		"Namespace":            namespace,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetResourceQuotaYAML")
	defer func() { Log().Trace("<<<< GetResourceQuotaYAML") }()

	resourceQuotaYAML := strings.ReplaceAll(resourceQuotaYAMLTemplate, "{RESOURCEQUOTA_NAME}", resourceQuotaName)
	resourceQuotaYAML = strings.ReplaceAll(resourceQuotaYAML, "{NAMESPACE}", namespace)
	resourceQuotaYAML = yaml.ReplaceMultilineTag(resourceQuotaYAML, "LABELS", constructLabels(labels))
	resourceQuotaYAML = yaml.ReplaceMultilineTag(resourceQuotaYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", resourceQuotaYAML).Trace("Resource Quota YAML.")
	return resourceQuotaYAML
}

const resourceQuotaYAMLTemplate = `---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: {RESOURCEQUOTA_NAME}
  namespace: {NAMESPACE}
  {LABELS}
  {OWNER_REF}
spec:
  scopeSelector:
    matchExpressions:
    - operator : In
      scopeName: PriorityClass
      values: ["system-node-critical"]
`

const deploymentAutosupportYAMLTemplate = `
      - name: {AUTOSUPPORT_CONTAINER_NAME}
        image: {AUTOSUPPORT_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        command:
        - /usr/local/bin/trident-autosupport
        args:
        - "--k8s-pod"
        - "--log-format={LOG_FORMAT}"
        - "--trident-silence-collector={AUTOSUPPORT_SILENCE}"
        {ENABLE_ACP}
        {AUTOSUPPORT_PROXY}
        {AUTOSUPPORT_CUSTOM_URL}
        {AUTOSUPPORT_SERIAL_NUMBER}
        {AUTOSUPPORT_HOSTNAME}
        {AUTOSUPPORT_DEBUG}
        {AUTOSUPPORT_INSECURE}
        resources:
          limits:
            memory: 1Gi
        volumeMounts:
        - name: asup-dir
          mountPath: /asup
`

// getCSIDeploymentAutosupportYAML will return either an empty string (excludeAutosupport) or an ASUP YAML template
// that will be inserted into the deployment YAML.
// There may be some variables that are still not replaced, such as {LOG_FORMAT} or {IMAGE_PULL_POLICY}.
// It is expected that the caller will complete the YAML replacements.
func getCSIDeploymentAutosupportYAML(args *DeploymentYAMLArguments) string {
	if args.ExcludeAutosupport {
		return ""
	}

	sidecarImages := []struct {
		arg *string
		tag string
	}{
		{&args.CSISidecarProvisionerImage, commonconfig.CSISidecarProvisionerImageTag},
		{&args.CSISidecarAttacherImage, commonconfig.CSISidecarAttacherImageTag},
		{&args.CSISidecarResizerImage, commonconfig.CSISidecarResizerImageTag},
		{&args.CSISidecarSnapshotterImage, commonconfig.CSISidecarSnapshotterImageTag},
	}
	for _, image := range sidecarImages {
		if *image.arg == "" {
			*image.arg = args.ImageRegistry + "/" + image.tag
		}
	}

	if args.AutosupportImage == "" {
		args.AutosupportImage = commonconfig.DefaultAutosupportImage
	}

	autosupportProxyLine := ""
	if args.AutosupportProxy != "" {
		autosupportProxyLine = fmt.Sprint("- -proxy-url=", args.AutosupportProxy)
	}

	autosupportCustomURLLine := ""
	if args.AutosupportCustomURL != "" {
		autosupportCustomURLLine = fmt.Sprint("- -custom-url=", args.AutosupportCustomURL)
	}

	autosupportSerialNumberLine := ""
	if args.AutosupportSerialNumber != "" {
		autosupportSerialNumberLine = fmt.Sprint("- -serial-number=", args.AutosupportSerialNumber)
	}

	autosupportHostnameLine := ""
	if args.AutosupportHostname != "" {
		autosupportHostnameLine = fmt.Sprint("- -hostname=", args.AutosupportHostname)
	}

	if args.Labels == nil {
		args.Labels = make(map[string]string)
	}
	args.Labels[DefaultContainerLabelKey] = "trident-main"

	autosupportDebugLine := "- -debug"
	if !IsLogLevelDebugOrHigher(args.LogLevel) {
		autosupportDebugLine = "#" + autosupportDebugLine
	}

	autosupportInsecureLine := ""
	if args.AutosupportInsecure {
		autosupportInsecureLine = "- -insecure"
	}

	asupYAMLSnippet := deploymentAutosupportYAMLTemplate
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_CONTAINER_NAME}", commonconfig.DefaultAutosupportName)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_IMAGE}", args.AutosupportImage)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_PROXY}", autosupportProxyLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_INSECURE}", autosupportInsecureLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_CUSTOM_URL}", autosupportCustomURLLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_SERIAL_NUMBER}", autosupportSerialNumberLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_HOSTNAME}", autosupportHostnameLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_DEBUG}", autosupportDebugLine)
	asupYAMLSnippet = strings.ReplaceAll(asupYAMLSnippet, "{AUTOSUPPORT_SILENCE}", strconv.FormatBool(args.SilenceAutosupport))

	return asupYAMLSnippet
}

var deploymentAutosupportVolumeYAML = `
      - name: asup-dir
        emptyDir:
          medium: ""
          sizeLimit: 1Gi
`

func getCSIDeploymentAutosupportVolumeYAML(args *DeploymentYAMLArguments) string {
	if args.ExcludeAutosupport {
		return ""
	}
	return deploymentAutosupportVolumeYAML
}

func GetCSIDeploymentYAML(args *DeploymentYAMLArguments) string {
	var debugLine, sideCarLogLevel, ipLocalhost, enableACP, K8sAPISidecarThrottle, K8sAPITridentThrottle string
	Log().WithFields(LogFields{
		"Args": args,
	}).Trace(">>>> GetCSIDeploymentYAML")
	defer func() { Log().Trace("<<<< GetCSIDeploymentYAML") }()

	if args.LogLevel == "" && !args.Debug {
		args.LogLevel = "info"
	} else if args.LogLevel == "" {
		args.LogLevel = "debug"
	}

	if args.Debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	if IsLogLevelDebugOrHigher(args.LogLevel) || args.Debug {
		sideCarLogLevel = "8"
	} else {
		sideCarLogLevel = "2"
	}

	if args.UseIPv6 {
		ipLocalhost = "[::1]"
	} else {
		ipLocalhost = "127.0.0.1"
	}

	deploymentYAML := csiDeployment120YAMLTemplate

	if args.ImageRegistry == "" {
		args.ImageRegistry = commonconfig.KubernetesCSISidecarRegistry
	}

	sidecarImages := []struct {
		arg *string
		tag string
	}{
		{&args.CSISidecarProvisionerImage, commonconfig.CSISidecarProvisionerImageTag},
		{&args.CSISidecarAttacherImage, commonconfig.CSISidecarAttacherImageTag},
		{&args.CSISidecarResizerImage, commonconfig.CSISidecarResizerImageTag},
		{&args.CSISidecarSnapshotterImage, commonconfig.CSISidecarSnapshotterImageTag},
	}
	for _, image := range sidecarImages {
		if *image.arg == "" {
			*image.arg = args.ImageRegistry + "/" + image.tag
		}
	}

	if args.IdentityLabel {
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LABEL_IDENTITY}", AzureWorkloadIdentityLabel)
	} else {
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LABEL_IDENTITY}", "")
	}

	if args.EnableACP {
		enableACP = "- \"-enable_acp\""
	}

	if strings.EqualFold(args.CloudProvider, CloudProviderAzure) {
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_ENV}", "- name: AZURE_CREDENTIAL_FILE\n          value: /etc/kubernetes/azure.json")
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_VOLUME}",
			"- name: azure-cred\n        hostPath:\n          path: /etc/kubernetes\n          type: DirectoryOrCreate")
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_VOLUME_MOUNT}", "- name: azure-cred\n          mountPath: /etc/kubernetes")
	} else {
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_ENV}", "")
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_VOLUME}", "")
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AZURE_CREDENTIAL_FILE_VOLUME_MOUNT}", "")
	}

	if args.K8sAPIQPS != 0 {
		queriesPerSecond := args.K8sAPIQPS
		burst := getBurstValueForQPS(queriesPerSecond)
		K8sAPITridentThrottle = fmt.Sprintf("- --k8s_api_qps=%d\n        - --k8s_api_burst=%d", queriesPerSecond, burst)
		K8sAPISidecarThrottle = fmt.Sprintf("- --kube-api-qps=%d\n        - --kube-api-burst=%d", queriesPerSecond, burst)
	}

	// Fill in the CSI feature gates from the YAML.
	// CSIFeatureGates should already be deduplicated.
	// If multiple feature gates are specified for the same placeholder,
	// gates will be a comma-delimited string.
	for placeholder, gates := range args.CSIFeatureGates {
		featureGates := fmt.Sprintf("- \"--feature-gates=%s\"", gates)
		deploymentYAML = strings.ReplaceAll(deploymentYAML, placeholder, featureGates)
	}

	// If there are no feature gates, remove the placeholder(s).
	if len(args.CSIFeatureGates) == 0 {
		deploymentYAML = strings.ReplaceAll(deploymentYAML, "{FEATURE_GATES_CSI_SNAPSHOTTER}", "")
	}

	// Get the autosupport YAML snippet and insert it into the deployment YAML.
	// This YAML snippet will have some "{}" variables that need to be replaced.
	autosupportYAML := getCSIDeploymentAutosupportYAML(args)
	autosupportVolumeYAML := getCSIDeploymentAutosupportVolumeYAML(args)

	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DEPLOYMENT_NAME}", args.DeploymentName)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{CSI_SIDECAR_PROVISIONER_IMAGE}", args.CSISidecarProvisionerImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{CSI_SIDECAR_ATTACHER_IMAGE}", args.CSISidecarAttacherImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{CSI_SIDECAR_RESIZER_IMAGE}", args.CSISidecarResizerImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{CSI_SIDECAR_SNAPSHOTTER_IMAGE}", args.CSISidecarSnapshotterImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LABEL_APP}", args.Labels[TridentAppLabelKey])
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{SIDECAR_LOG_LEVEL}", sideCarLogLevel)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{IP_LOCALHOST}", ipLocalhost)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_YAML}", autosupportYAML)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_VOLUME_YAML}", autosupportVolumeYAML)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_FORMAT}", args.LogFormat)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DEBUG}", debugLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_LEVEL}", args.LogLevel)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_WORKFLOWS}", args.LogWorkflows)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_LAYERS}", args.LogLayers)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DISABLE_AUDIT_LOG}", strconv.FormatBool(args.DisableAuditLog))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{HTTP_REQUEST_TIMEOUT}", args.HTTPRequestTimeout)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{IMAGE_PULL_POLICY}", args.ImagePullPolicy)
	deploymentYAML = yaml.ReplaceMultilineTag(deploymentYAML, "LABELS", constructLabels(args.Labels))
	deploymentYAML = yaml.ReplaceMultilineTag(deploymentYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	deploymentYAML = yaml.ReplaceMultilineTag(deploymentYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	deploymentYAML = yaml.ReplaceMultilineTag(deploymentYAML, "NODE_TOLERATIONS", constructTolerations(args.Tolerations))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{ENABLE_FORCE_DETACH}", strconv.FormatBool(args.EnableForceDetach))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{ENABLE_ACP}", enableACP)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{K8S_API_CLIENT_TRIDENT_THROTTLE}", K8sAPITridentThrottle)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{K8S_API_CLIENT_SIDECAR_THROTTLE}", K8sAPISidecarThrottle)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{ENABLE_CONCURRENCY}", strconv.FormatBool(args.EnableConcurrency))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", deploymentYAML).Trace("CSI Deployment YAML.")
	deploymentYAML = yaml.ReplaceMultilineTag(deploymentYAML, "IMAGE_PULL_SECRETS",
		constructImagePullSecrets(args.ImagePullSecrets))

	return deploymentYAML
}

const csiDeployment120YAMLTemplate = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {DEPLOYMENT_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: {LABEL_APP}
  template:
    metadata:
      labels:
        app: {LABEL_APP}
        {LABEL_IDENTITY}
      annotations:
        openshift.io/required-scc: {SERVICE_ACCOUNT}
    spec:
      serviceAccount: {SERVICE_ACCOUNT}
      containers:
      - name: trident-main
        image: {TRIDENT_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          runAsNonRoot: false
          capabilities:
            drop:
            - all
        ports:
        - containerPort: 8443
        - containerPort: 8001
        command:
        - /trident_orchestrator
        args:
        - "--crd_persistence"
        - "--k8s_pod"
        - "--https_rest"
        - "--https_port=8443"
        - "--csi_node_name=$(KUBE_NODE_NAME)"
        - "--csi_endpoint=$(CSI_ENDPOINT)"
        - "--csi_role=controller"
        - "--log_format={LOG_FORMAT}"
        - "--log_level={LOG_LEVEL}"
        - "--log_workflows={LOG_WORKFLOWS}"
        - "--log_layers={LOG_LAYERS}"
        - "--disable_audit_log={DISABLE_AUDIT_LOG}"
        - "--address={IP_LOCALHOST}"
        - "--http_request_timeout={HTTP_REQUEST_TIMEOUT}"
        - "--enable_force_detach={ENABLE_FORCE_DETACH}"
        - "--enable_concurrency={ENABLE_CONCURRENCY}"
        - "--metrics"
        {ENABLE_ACP}
        {DEBUG}
        {K8S_API_CLIENT_TRIDENT_THROTTLE}
        livenessProbe:
          exec:
            command:
            - tridentctl
            - -s
            - "{IP_LOCALHOST}:8000"
            - version
          failureThreshold: 2
          initialDelaySeconds: 120
          periodSeconds: 120
          timeoutSeconds: 90
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: CSI_ENDPOINT
          value: unix://plugin/csi.sock
        - name: TRIDENT_SERVER
          value: "{IP_LOCALHOST}:8000"
        {AZURE_CREDENTIAL_FILE_ENV}
        volumeMounts:
        - name: socket-dir
          mountPath: /plugin
        - name: certs
          mountPath: /certs
          readOnly: true
        {AZURE_CREDENTIAL_FILE_VOLUME_MOUNT}
      {AUTOSUPPORT_YAML}
      - name: csi-provisioner
        image: {CSI_SIDECAR_PROVISIONER_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=600s"
        - "--csi-address=$(ADDRESS)"
        - "--retry-interval-start=8s"
        - "--retry-interval-max=30s"
        - "--worker-threads=5"
        {K8S_API_CLIENT_SIDECAR_THROTTLE}
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-attacher
        image: {CSI_SIDECAR_ATTACHER_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=60s"
        - "--retry-interval-start=10s"
        - "--worker-threads=10"
        - "--csi-address=$(ADDRESS)"
        {K8S_API_CLIENT_SIDECAR_THROTTLE}
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-resizer
        image: {CSI_SIDECAR_RESIZER_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=300s"
        - "--workers=10"
        - "--csi-address=$(ADDRESS)"
        {K8S_API_CLIENT_SIDECAR_THROTTLE}
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-snapshotter
        image: {CSI_SIDECAR_SNAPSHOTTER_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=300s"
        - "--worker-threads=10"
        - "--csi-address=$(ADDRESS)"
        {FEATURE_GATES_CSI_SNAPSHOTTER}
        {K8S_API_CLIENT_SIDECAR_THROTTLE}
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      {IMAGE_PULL_SECRETS}
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
                  {NODE_SELECTOR}
      {NODE_TOLERATIONS}
      volumes:
      - name: socket-dir
        emptyDir:
      - name: certs
        projected:
          sources:
          - secret:
              name: trident-csi
          - secret:
              name: trident-encryption-keys
      {AUTOSUPPORT_VOLUME_YAML}
      {AZURE_CREDENTIAL_FILE_VOLUME}
`

func GetCSIDaemonSetYAMLWindows(args *DaemonsetYAMLArguments) string {
	var debugLine, sidecarLogLevel, daemonSetYAML string
	var version int
	Log().WithFields(LogFields{
		"Args": args,
	}).Trace(">>>> GetCSIDaemonSetYAMLWindows")
	defer func() { Log().Trace("<<<< GetCSIDaemonSetYAMLWindows") }()

	if args.LogLevel == "" && !args.Debug {
		args.LogLevel = "info"
	} else if args.LogLevel == "" {
		args.LogLevel = "debug"
	}

	if args.Debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	if IsLogLevelDebugOrHigher(args.LogLevel) || args.Debug {
		sidecarLogLevel = "8"
	} else {
		sidecarLogLevel = "2"
	}

	if args.Version != nil {
		version, _ = strconv.Atoi(args.Version.MinorVersionString())
	}

	if version >= 20 {
		daemonSetYAML = daemonSet120YAMLTemplateWindows
	}

	if args.Labels == nil {
		args.Labels = map[string]string{}
	}
	args.Labels[DefaultContainerLabelKey] = "trident-main"
	tolerations := args.Tolerations
	if args.Tolerations == nil {
		// Default to tolerating everything
		tolerations = []map[string]string{
			{"effect": "NoExecute", "operator": "Exists"},
			{"effect": "NoSchedule", "operator": "Exists"},
		}
	}

	if args.ImageRegistry == "" {
		args.ImageRegistry = commonconfig.KubernetesCSISidecarRegistry
	}

	sidecarImages := []struct {
		arg *string
		tag string
	}{
		{&args.CSISidecarNodeDriverRegistrarImage, commonconfig.CSISidecarNodeDriverRegistrarImageTag},
		{&args.CSISidecarLivenessProbeImage, commonconfig.CSISidecarLivenessProbeImageTag},
	}
	for _, image := range sidecarImages {
		if *image.arg == "" {
			*image.arg = args.ImageRegistry + "/" + image.tag
		}
	}

	kubeletDir := strings.TrimRight(args.KubeletDir, "/")
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DAEMONSET_NAME}", args.DaemonsetName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{CSI_SIDECAR_NODE_DRIVER_REGISTRAR_IMAGE}", args.CSISidecarNodeDriverRegistrarImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{CSI_SIDECAR_LIVENESS_PROBE_IMAGE}", args.CSISidecarLivenessProbeImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{KUBELET_DIR}", kubeletDir)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LABEL_APP}", args.Labels[TridentAppLabelKey])
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{SIDECAR_LOG_LEVEL}", sidecarLogLevel)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_FORMAT}", args.LogFormat)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DEBUG}", debugLine)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_LEVEL}", args.LogLevel)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_WORKFLOWS}", args.LogWorkflows)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_LAYERS}", args.LogLayers)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DISABLE_AUDIT_LOG}", strconv.FormatBool(args.DisableAuditLog))
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{PROBE_PORT}", args.ProbePort)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{HTTP_REQUEST_TIMEOUT}", args.HTTPRequestTimeout)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{IMAGE_PULL_POLICY}", args.ImagePullPolicy)
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "NODE_TOLERATIONS", constructTolerations(tolerations))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "LABELS", constructLabels(args.Labels))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", daemonSetYAML).Trace("CSI Daemonset Windows YAML.")
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "IMAGE_PULL_SECRETS",
		constructImagePullSecrets(args.ImagePullSecrets))

	return daemonSetYAML
}

func GetCSIDaemonSetYAMLLinux(args *DaemonsetYAMLArguments) string {
	var debugLine, sidecarLogLevel string
	Log().WithFields(LogFields{
		"Args": args,
	}).Trace(">>>> GetCSIDaemonSetYAMLLinux")
	defer func() { Log().Trace("<<<< GetCSIDaemonSetYAMLLinux") }()

	if args.LogLevel == "" && !args.Debug {
		args.LogLevel = "info"
	} else if args.LogLevel == "" {
		args.LogLevel = "debug"
	}

	if args.Debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	if IsLogLevelDebugOrHigher(args.LogLevel) || args.Debug {
		sidecarLogLevel = "8"
	} else {
		sidecarLogLevel = "2"
	}

	daemonSetYAML := daemonSet120YAMLTemplateLinux

	if args.Labels == nil {
		args.Labels = map[string]string{}
	}
	args.Labels[DefaultContainerLabelKey] = "trident-main"
	tolerations := args.Tolerations
	if args.Tolerations == nil {
		// Default to tolerating everything
		tolerations = []map[string]string{
			{"effect": "NoExecute", "operator": "Exists"},
			{"effect": "NoSchedule", "operator": "Exists"},
		}
	}

	if args.ImageRegistry == "" {
		args.ImageRegistry = commonconfig.KubernetesCSISidecarRegistry
	}

	if args.CSISidecarNodeDriverRegistrarImage == "" {
		args.CSISidecarNodeDriverRegistrarImage = args.ImageRegistry + "/" + commonconfig.CSISidecarNodeDriverRegistrarImageTag
	}

	kubeletDir := strings.TrimRight(args.KubeletDir, "/")
	// NodePrep this must come first because it adds a section that has tags in it
	daemonSetYAML = replaceNodePrepTag(daemonSetYAML, args.NodePrep)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DAEMONSET_NAME}", args.DaemonsetName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{CSI_SIDECAR_NODE_DRIVER_REGISTRAR_IMAGE}",
		args.CSISidecarNodeDriverRegistrarImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{KUBELET_DIR}", kubeletDir)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LABEL_APP}", args.Labels[TridentAppLabelKey])
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{FORCE_DETACH_BOOL}", strconv.FormatBool(args.EnableForceDetach))
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{SIDECAR_LOG_LEVEL}", sidecarLogLevel)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_FORMAT}", args.LogFormat)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DISABLE_AUDIT_LOG}", strconv.FormatBool(args.DisableAuditLog))
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DEBUG}", debugLine)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_LEVEL}", args.LogLevel)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_WORKFLOWS}", args.LogWorkflows)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{LOG_LAYERS}", args.LogLayers)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{PROBE_PORT}", args.ProbePort)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{HTTP_REQUEST_TIMEOUT}", args.HTTPRequestTimeout)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{IMAGE_PULL_POLICY}", args.ImagePullPolicy)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{ISCSI_SELF_HEALING_INTERVAL}", args.ISCSISelfHealingInterval)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{ISCSI_SELF_HEALING_WAIT_TIME}", args.ISCSISelfHealingWaitTime)
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "NODE_TOLERATIONS", constructTolerations(tolerations))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "LABELS", constructLabels(args.Labels))
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", daemonSetYAML).Trace("CSI Daemonset Linux YAML.")
	daemonSetYAML = yaml.ReplaceMultilineTag(daemonSetYAML, "IMAGE_PULL_SECRETS",
		constructImagePullSecrets(args.ImagePullSecrets))

	return daemonSetYAML
}

func replaceNodePrepTag(daemonSetYAML string, nodePrep []string) string {
	if len(nodePrep) > 0 {
		prepareNodeLinuxFormatted := indentSection(daemonSetYAML, prepareNodeLinux, "{PREPARE_NODE}")
		daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{PREPARE_NODE}", prepareNodeLinuxFormatted)
		daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{NODE_PREP}", strings.Join(nodePrep, ","))
	} else {
		daemonSetYAML = removeSectionTag(daemonSetYAML, "{PREPARE_NODE}")
	}
	return daemonSetYAML
}

func removeSectionTag(daemonSetYAML, tag string) string {
	r := regexp.MustCompile("\n +" + tag + "\n")
	return string(r.ReplaceAll([]byte(daemonSetYAML), []byte("\n")))
}

func indentSection(daemonSetYAML, section, tag string) (indentedSection string) {
	indent := getSectionIndentation(daemonSetYAML, tag)
	indentedSection = strings.ReplaceAll(section, "{INDENT}", indent)
	return
}

func getSectionIndentation(daemonSetYAML, tag string) (indent string) {
	tagIdx := strings.Index(daemonSetYAML, tag)
	lineStartIdx := strings.LastIndex(daemonSetYAML[0:tagIdx], "\n") + 1
	return daemonSetYAML[lineStartIdx:tagIdx]
}

const prepareNodeLinux = `initContainers:
{INDENT}- name: trident-prepare-node
{INDENT}  securityContext:
{INDENT}    privileged: true
{INDENT}    allowPrivilegeEscalation: true
{INDENT}    capabilities:
{INDENT}      drop:
{INDENT}      - all
{INDENT}      add:
{INDENT}      - SYS_ADMIN
{INDENT}  image: {TRIDENT_IMAGE}
{INDENT}  imagePullPolicy: {IMAGE_PULL_POLICY}
{INDENT}  command:
{INDENT}  - /node_prep
{INDENT}  args:
{INDENT}  - "--node-prep={NODE_PREP}"
{INDENT}  - "--log-format={LOG_FORMAT}"
{INDENT}  - "--log-level={LOG_LEVEL}"
{INDENT}  {DEBUG}
{INDENT}  env:
{INDENT}  - name: PATH
{INDENT}    value: /netapp:/bin
{INDENT}  volumeMounts:
{INDENT}  - name: host-dir
{INDENT}    mountPath: /host
{INDENT}    mountPropagation: "Bidirectional"`

const daemonSet120YAMLTemplateLinux = `---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {DAEMONSET_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  selector:
    matchLabels:
      app: {LABEL_APP}
  template:
    metadata:
      labels:
        app: {LABEL_APP}
      annotations:
        openshift.io/required-scc: {SERVICE_ACCOUNT}
    spec:
      serviceAccount: {SERVICE_ACCOUNT}
      hostNetwork: true
      hostIPC: true
      hostPID: true
      dnsPolicy: ClusterFirstWithHostNet
      priorityClassName: system-node-critical
      {PREPARE_NODE}
      containers:
      - name: trident-main
        securityContext:
          privileged: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - all
            add:
            - SYS_ADMIN
        image: {TRIDENT_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        command:
        - /trident_orchestrator
        args:
        - "--no_persistence"
        - "--k8s_pod"
        - "--rest=false"
        - "--csi_node_name=$(KUBE_NODE_NAME)"
        - "--csi_endpoint=$(CSI_ENDPOINT)"
        - "--csi_role=node"
        - "--log_format={LOG_FORMAT}"
        - "--log_level={LOG_LEVEL}"
        - "--log_workflows={LOG_WORKFLOWS}"
        - "--log_layers={LOG_LAYERS}"
        - "--disable_audit_log={DISABLE_AUDIT_LOG}"
        - "--http_request_timeout={HTTP_REQUEST_TIMEOUT}"
        - "--https_rest"
        - "--https_port={PROBE_PORT}"
        - "--enable_force_detach={FORCE_DETACH_BOOL}"
        - "--iscsi_self_healing_interval={ISCSI_SELF_HEALING_INTERVAL}"
        - "--iscsi_self_healing_wait_time={ISCSI_SELF_HEALING_WAIT_TIME}"
        {DEBUG}
        startupProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readiness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
          initialDelaySeconds: 15
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: KUBELET_DIR
          value: {KUBELET_DIR}
        - name: CSI_ENDPOINT
          value: unix://plugin/csi.sock
        - name: PATH
          value: /netapp:/bin
        volumeMounts:
        - name: plugin-dir
          mountPath: /plugin
        - name: plugins-mount-dir
          mountPath: {KUBELET_DIR}/plugins
          mountPropagation: "Bidirectional"
        - name: pods-mount-dir
          mountPath: {KUBELET_DIR}/pods
          mountPropagation: "Bidirectional"
        - name: dev-dir
          mountPath: /dev
        - name: sys-dir
          mountPath: /sys
        - name: host-dir
          mountPath: /host
          mountPropagation: "Bidirectional"
        - name: trident-tracking-dir
          mountPath: /var/lib/trident/tracking
          mountPropagation: "Bidirectional"
        - name: certs
          mountPath: /certs
          readOnly: true
      - name: driver-registrar
        image: {CSI_SIDECAR_NODE_DRIVER_REGISTRAR_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--csi-address=$(ADDRESS)"
        - "--kubelet-registration-path=$(REGISTRATION_PATH)"
        env:
        - name: ADDRESS
          value: /plugin/csi.sock
        - name: REGISTRATION_PATH
          value: "{KUBELET_DIR}/plugins/csi.trident.netapp.io/csi.sock"
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: plugin-dir
          mountPath: /plugin
        - name: registration-dir
          mountPath: /registration
      {IMAGE_PULL_SECRETS}
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
                  {NODE_SELECTOR}
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - node.csi.trident.netapp.io
              topologyKey: kubernetes.io/hostname
      {NODE_TOLERATIONS}
      volumes:
      - name: plugin-dir
        hostPath:
          path: {KUBELET_DIR}/plugins/csi.trident.netapp.io/
          type: DirectoryOrCreate
      - name: registration-dir
        hostPath:
          path: {KUBELET_DIR}/plugins_registry/
          type: Directory
      - name: plugins-mount-dir
        hostPath:
          path: {KUBELET_DIR}/plugins
          type: DirectoryOrCreate
      - name: pods-mount-dir
        hostPath:
          path: {KUBELET_DIR}/pods
          type: DirectoryOrCreate
      - name: dev-dir
        hostPath:
          path: /dev
          type: Directory
      - name: sys-dir
        hostPath:
          path: /sys
          type: Directory
      - name: host-dir
        hostPath:
          path: /
          type: Directory
      - name: trident-tracking-dir
        hostPath:
          path: /var/lib/trident/tracking
          type: DirectoryOrCreate
      - name: certs
        projected:
          sources:
          - secret:
              name: trident-csi
          - secret:
              name: trident-encryption-keys
`

const daemonSet120YAMLTemplateWindows = `---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {DAEMONSET_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  selector:
    matchLabels:
      app: {LABEL_APP}
  template:
    metadata:
      labels:
        app: {LABEL_APP}
      annotations:
        openshift.io/required-scc: {SERVICE_ACCOUNT}
    spec:
      securityContext:
        windowsOptions:
          hostProcess: false
          runAsUserName: "ContainerAdministrator"
      priorityClassName: system-node-critical
      serviceAccount: {SERVICE_ACCOUNT}
      containers:
      - name: trident-main
        imagePullPolicy: {IMAGE_PULL_POLICY}
        image: {TRIDENT_IMAGE}
        command:
        - trident_orchestrator.exe
        args:
        - "--no_persistence"
        - "--k8s_pod"
        - "--rest=false"
        - "--csi_node_name=$(KUBE_NODE_NAME)"
        - "--csi_endpoint=$(CSI_ENDPOINT)"
        - "--csi_role=node"
        - "--log_format={LOG_FORMAT}"
        - "--log_level={LOG_LEVEL}"
        - "--log_workflows={LOG_WORKFLOWS}"
        - "--log_layers={LOG_LAYERS}"
        - "--disable_audit_log={DISABLE_AUDIT_LOG}"
        - "--http_request_timeout={HTTP_REQUEST_TIMEOUT}"
        - "--https_rest"
        - "--https_port={PROBE_PORT}"
        {DEBUG}
        # Windows requires named ports for it to actually bind
        ports:
          - containerPort: {PROBE_PORT}
            name: healthz
            protocol: TCP
        startupProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: healthz
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: healthz
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readiness
            scheme: HTTPS
            port: healthz
          failureThreshold: 5
          timeoutSeconds: 5
          periodSeconds: 10
          initialDelaySeconds: 15
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: CSI_ENDPOINT
          value: unix:///csi/csi.sock
        - name: KUBELET_DIR
          value: {KUBELET_DIR}
        volumeMounts:
        - name: trident-tracking-dir
          mountPath: C:\var\lib\trident\tracking
        - name: certs
          mountPath: /certs
          readOnly: true
        - name: kubelet-dir
          mountPath: {KUBELET_DIR}
        - name: plugin-dir
          mountPath: C:\csi
        - name: csi-proxy-fs-pipe-v1
          mountPath: \\.\pipe\csi-proxy-filesystem-v1
        # This is needed for NodeGetVolumeStats
        - name: csi-proxy-volume-pipe
          mountPath: \\.\pipe\csi-proxy-volume-v1
        - name: csi-proxy-smb-pipe-v1
          mountPath: \\.\pipe\csi-proxy-smb-v1
        # these paths are still included for compatibility, they're used
        # only if the node has still the beta version of the CSI proxy
        - name: csi-proxy-fs-pipe-v1beta1
          mountPath: \\.\pipe\csi-proxy-filesystem-v1beta1
        - name: csi-proxy-smb-pipe-v1beta1
          mountPath: \\.\pipe\csi-proxy-smb-v1beta1
        resources:
          limits:
            memory: 400Mi
          requests:
            cpu: 10m
            memory: 20Mi
      - name: node-driver-registrar
        image: {CSI_SIDECAR_NODE_DRIVER_REGISTRAR_IMAGE}
        imagePullPolicy: {IMAGE_PULL_POLICY}
        args:
        - --v=2
        - --csi-address=$(CSI_ENDPOINT)
        - --kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)
        livenessProbe:
          exec:
            command:
            - /csi-node-driver-registrar.exe
            - --kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)
            - --mode=kubelet-registration-probe
          initialDelaySeconds: 60
          timeoutSeconds: 30
        env:
        - name: CSI_ENDPOINT
          value: unix:///csi/csi.sock
        - name: DRIVER_REG_SOCK_PATH
          value: {KUBELET_DIR}\plugins\csi.trident.netapp.io\csi.sock
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: kubelet-dir
          mountPath: {KUBELET_DIR}
        - name: plugin-dir
          mountPath: C:\csi
        - name: registration-dir
          mountPath: C:\registration
        resources:
          limits:
            memory: 200Mi
          requests:
            cpu: 10m
            memory: 20Mi
      - name: liveness-probe
        volumeMounts:
          - mountPath: C:\csi
            name: plugin-dir
        image: {CSI_SIDECAR_LIVENESS_PROBE_IMAGE}
        args:
          - --csi-address=$(CSI_ENDPOINT)
          - --probe-timeout=3s
          - --health-port=29643
          - --v=2
        env:
          - name: CSI_ENDPOINT
            value: unix:///csi/csi.sock
        resources:
          limits:
            memory: 100Mi
          requests:
            cpu: 10m
            memory: 40Mi
      {IMAGE_PULL_SECRETS}
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - windows
                  {NODE_SELECTOR}
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - node.csi.trident.netapp.io
              topologyKey: kubernetes.io/hostname
      {NODE_TOLERATIONS}
      volumes:
        - name: trident-tracking-dir
          hostPath:
            path: C:\var\lib\trident\tracking
            type: DirectoryOrCreate
        - name: certs
          projected:
            sources:
            - secret:
                name: trident-csi
            - secret:
                name: trident-encryption-keys
        - name: csi-proxy-volume-pipe
          hostPath:
            path: \\.\pipe\csi-proxy-volume-v1
            type: ""
        - name: csi-proxy-fs-pipe-v1
          hostPath:
            path: \\.\pipe\csi-proxy-filesystem-v1
        - name: csi-proxy-smb-pipe-v1
          hostPath:
            path: \\.\pipe\csi-proxy-smb-v1
        # these paths are still included for compatibility, they're used
        # only if the node has still the beta version of the CSI proxy
        - name: csi-proxy-fs-pipe-v1beta1
          hostPath:
            path: \\.\pipe\csi-proxy-filesystem-v1beta1
        - name: csi-proxy-smb-pipe-v1beta1
          hostPath:
            path: \\.\pipe\csi-proxy-smb-v1beta1
        - name: registration-dir
          hostPath:
            path: {KUBELET_DIR}\plugins_registry\
            type: Directory
        - name: kubelet-dir
          hostPath:
            path: {KUBELET_DIR}\
            type: Directory
        - name: plugin-dir
          hostPath:
            path: {KUBELET_DIR}\plugins\csi.trident.netapp.io\
            type: DirectoryOrCreate
`

func GetTridentVersionPodYAML(args *TridentVersionPodYAMLArguments) string {
	Log().WithFields(LogFields{
		"Name":                 args.TridentVersionPodName,
		"TridentImage":         args.TridentImage,
		"ServiceAccountName":   args.ServiceAccountName,
		"Labels":               args.Labels,
		"ControllingCRDetails": args.ControllingCRDetails,
	}).Trace(">>>> GetTridentVersionPodYAML")
	defer func() { Log().Trace("<<<< GetTridentVersionPodYAML") }()

	versionPodYAML := strings.ReplaceAll(tridentVersionPodYAML, "{NAME}", args.TridentVersionPodName)
	versionPodYAML = strings.ReplaceAll(versionPodYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	versionPodYAML = strings.ReplaceAll(versionPodYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	versionPodYAML = yaml.ReplaceMultilineTag(versionPodYAML, "LABELS", constructLabels(args.Labels))
	versionPodYAML = yaml.ReplaceMultilineTag(versionPodYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	versionPodYAML = yaml.ReplaceMultilineTag(versionPodYAML, "NODE_TOLERATIONS", constructTolerations(args.Tolerations))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", versionPodYAML).Trace("Trident Version Pod YAML.")
	versionPodYAML = yaml.ReplaceMultilineTag(versionPodYAML, "IMAGE_PULL_SECRETS",
		constructImagePullSecrets(args.ImagePullSecrets))
	versionPodYAML = strings.ReplaceAll(versionPodYAML, "{IMAGE_PULL_POLICY}", args.ImagePullPolicy)

	return versionPodYAML
}

const tridentVersionPodYAML = `---
apiVersion: v1
kind: Pod
metadata:
  name: {NAME}
  {LABELS}
  {OWNER_REF}
spec:
  serviceAccount: {SERVICE_ACCOUNT}
  restartPolicy: Never
  containers:
  - name: trident-main
    imagePullPolicy: {IMAGE_PULL_POLICY}
    image: {TRIDENT_IMAGE}
    command: ["tridentctl"]
    args: ["pause"]
    securityContext:
      capabilities:
        drop:
        - all
  {IMAGE_PULL_SECRETS}
  {NODE_TOLERATIONS}
  affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator : In
                    values:
                    - linux
`

func GetOpenShiftSCCYAML(
	sccName, user, namespace string, labels, controllingCRDetails map[string]string, privileged bool,
) string {
	sccYAML := openShiftUnprivilegedSCCYAML
	// Only linux node pod needs an privileged SCC (i.e. privileged set to true)
	if privileged {
		sccYAML = openShiftPrivilegedSCCYAML
	}
	sccYAML = strings.ReplaceAll(sccYAML, "{SCC}", sccName)
	sccYAML = strings.ReplaceAll(sccYAML, "{NAMESPACE}", namespace)
	sccYAML = strings.ReplaceAll(sccYAML, "{USER}", user)
	sccYAML = yaml.ReplaceMultilineTag(sccYAML, "LABELS", constructLabels(labels))
	sccYAML = yaml.ReplaceMultilineTag(sccYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", sccYAML).Trace("OpenShift SCC YAML.")
	return sccYAML
}

const openShiftPrivilegedSCCYAML = `
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  annotations:
    kubernetes.io/description: '{SCC} is a clone of the privileged built-in, and is meant just for use with trident.'
  name: {SCC}
  {LABELS}
  {OWNER_REF}
allowHostDirVolumePlugin: true
allowHostIPC: true
allowHostNetwork: true
allowHostPID: true
allowHostPorts: false
allowPrivilegeEscalation: true
allowPrivilegedContainer: true
allowedCapabilities:
- SYS_ADMIN
allowedUnsafeSysctls: null
defaultAddCapabilities: null
fsGroup:
  type: RunAsAny
groups: []
priority: null
readOnlyRootFilesystem: false
requiredDropCapabilities: null
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: RunAsAny
supplementalGroups:
  type: RunAsAny
users:
- system:serviceaccount:{NAMESPACE}:{USER}
volumes:
- hostPath
- downwardAPI
- projected
- emptyDir
`

const openShiftUnprivilegedSCCYAML = `
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  annotations:
    kubernetes.io/description: '{SCC} is a clone of the anyuid built-in, and is meant just for use with trident.'
  name: {SCC}
  {LABELS}
  {OWNER_REF}
allowHostDirVolumePlugin: false
allowHostIPC: false
allowHostNetwork: false
allowHostPID: false
allowHostPorts: false
allowPrivilegeEscalation: true
allowPrivilegedContainer: false
allowedCapabilities: null
apiVersion: security.openshift.io/v1
defaultAddCapabilities: null
fsGroup:
  type: RunAsAny
groups: []
priority: null
readOnlyRootFilesystem: false
requiredDropCapabilities:
- MKNOD
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
supplementalGroups:
  type: RunAsAny
users:
- system:serviceaccount:{NAMESPACE}:{USER}
volumes:
  - hostPath
  - downwardAPI
  - projected
  - emptyDir
`

func GetOpenShiftSCCQueryYAML(scc string) string {
	Log().WithFields(LogFields{
		"SCC": scc,
	}).Trace(">>>> GetOpenShiftSCCQueryYAML")
	defer func() { Log().Trace("<<<< GetOpenShiftSCCQueryYAML") }()

	yaml := strings.ReplaceAll(openShiftSCCQueryYAMLTemplate, "{SCC}", scc)

	Log().WithField("yaml", yaml).Trace("OpenShift SCC Query YAML.")
	return yaml
}

const openShiftSCCQueryYAMLTemplate = `
kind: SecurityContextConstraints
apiVersion: security.openshift.io/v1
metadata:
  name: {SCC}
`

func GetSecretYAML(
	secretName, namespace string, labels, controllingCRDetails, data, stringData map[string]string,
) string {
	Log().WithFields(LogFields{
		"SecretName":           secretName,
		"Namespace":            namespace,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetSecretYAML")
	defer func() { Log().Trace("<<<< GetSecretYAML") }()

	secretYAML := strings.ReplaceAll(secretYAMLTemplate, "{SECRET_NAME}", secretName)
	secretYAML = strings.ReplaceAll(secretYAML, "{NAMESPACE}", namespace)
	secretYAML = yaml.ReplaceMultilineTag(secretYAML, "LABELS", constructLabels(labels))
	secretYAML = yaml.ReplaceMultilineTag(secretYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	// Log before actual secrets are inserted into YAML.
	Log().WithField("yaml", secretYAML).Trace("(Redacted) Secret YAML.")

	if data != nil {
		secretYAML += "data:\n"
		for key, value := range data {
			secretYAML += fmt.Sprintf("  %s: %s\n", key, value)
		}
	}

	if stringData != nil {
		secretYAML += "stringData:\n"
		for key, value := range stringData {
			secretYAML += fmt.Sprintf("  %s: %s\n", key, value)
		}
	}

	return secretYAML
}

//nolint:gosec
const secretYAMLTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: {SECRET_NAME}
  namespace: {NAMESPACE}
  {LABELS}
  {OWNER_REF}
`

func GetCRDsYAML() string {
	Log().Trace(">>>> GetCRDsYAML")
	defer func() { Log().Trace("<<<< GetCRDsYAML") }()
	return customResourceDefinitionYAMLv1
}

func GetVersionCRDYAML() string {
	Log().Trace(">>>> GetVersionCRDYAML")
	defer func() { Log().Trace("<<<< GetVersionCRDYAML") }()
	return tridentVersionCRDYAMLv1
}

func GetBackendCRDYAML() string {
	Log().Trace(">>>> GetBackendCRDYAML")
	defer func() { Log().Trace("<<<< GetBackendCRDYAML") }()
	return tridentBackendCRDYAMLv1
}

func GetBackendConfigCRDYAML() string {
	Log().Trace(">>>> GetBackendConfigCRDYAML")
	defer func() { Log().Trace("<<<< GetBackendConfigCRDYAML") }()
	return tridentBackendConfigCRDYAMLv1
}

func GetConfiguratorCRDYAML() string {
	Log().Trace(">>>> GetConfiguratorCRDYAML")
	defer func() { Log().Trace("<<<< GetConfiguratorCRDYAML") }()
	return tridentConfiguratorCRDYAMLv1
}

func GetMirrorRelationshipCRDYAML() string {
	Log().Trace(">>>> GetMirrorRelationshipCRDYAML")
	defer func() { Log().Trace("<<<< GetMirrorRelationshipCRDYAML") }()
	return tridentMirrorRelationshipCRDYAMLv1
}

func GetActionMirrorUpdateCRDYAML() string {
	Log().Trace(">>>> GetActionMirrorUpdateCRDYAML")
	defer func() { Log().Trace("<<<< GetActionMirrorUpdateCRDYAML") }()
	return tridentActionMirrorUpdateCRDYAMLv1
}

func GetSnapshotInfoCRDYAML() string {
	Log().Trace(">>>> GetSnapshotInfoCRDYAML")
	defer func() { Log().Trace("<<<< GetSnapshotInfoCRDYAML") }()
	return tridentSnapshotInfoCRDYAMLv1
}

func GetStorageClassCRDYAML() string {
	Log().Trace(">>>> GetStorageClassCRDYAML")
	defer func() { Log().Trace("<<<< GetStorageClassCRDYAML") }()
	return tridentStorageClassCRDYAMLv1
}

func GetVolumeCRDYAML() string {
	Log().Trace(">>>> GetVolumeCRDYAML")
	defer func() { Log().Trace("<<<< GetVolumeCRDYAML") }()
	return tridentVolumeCRDYAMLv1
}

func GetVolumePublicationCRDYAML() string {
	Log().Trace(">>>> GetVolumePublicationCRDYAML")
	defer func() { Log().Trace("<<<< GetVolumePublicationCRDYAML") }()
	return tridentVolumePublicationCRDYAMLv1
}

func GetNodeCRDYAML() string {
	Log().Trace(">>>> GetNodeCRDYAML")
	defer func() { Log().Trace("<<<< GetNodeCRDYAML") }()
	return tridentNodeCRDYAMLv1
}

func GetTransactionCRDYAML() string {
	Log().Trace(">>>> GetTransactionCRDYAML")
	defer func() { Log().Trace("<<<< GetTransactionCRDYAML") }()
	return tridentTransactionCRDYAMLv1
}

func GetSnapshotCRDYAML() string {
	Log().Trace(">>>> GetSnapshotCRDYAML")
	defer func() { Log().Trace("<<<< GetSnapshotCRDYAML") }()
	return tridentSnapshotCRDYAMLv1
}

func GetGroupSnapshotCRDYAML() string {
	Log().Trace(">>>> GetGroupSnapshotCRDYAML")
	defer func() { Log().Trace("<<<< GetGroupSnapshotCRDYAML") }()
	return tridentGroupSnapshotCRDYAMLv1
}

func GetVolumeReferenceCRDYAML() string {
	Log().Trace(">>>> GetVolumeReferenceCRDYAML")
	defer func() { Log().Trace("<<<< GetVolumeReferenceCRDYAML") }()
	return tridentVolumeReferenceCRDYAMLv1
}

func GetActionSnapshotRestoreCRDYAML() string {
	Log().Trace(">>>> GetActionSnapshotRestoreCRDYAML")
	defer func() { Log().Trace("<<<< GetActionSnapshotRestoreCRDYAML") }()
	return tridentActionSnapshotRestoreCRDYAMLv1
}

func GetOrchestratorCRDYAML() string {
	Log().Trace(">>>> GetOrchestratorCRDYAML")
	defer func() { Log().Trace("<<<< GetOrchestratorCRDYAML") }()
	return tridentOrchestratorCRDYAMLv1
}

/*
kubectl delete crd tridentversions.trident.netapp.io --wait=false
kubectl delete crd tridentbackends.trident.netapp.io --wait=false
kubectl delete crd tridentmirrorrelationships.trident.netapp.io --wait=false
kubectl delete crd tridentactionmirrorupdates.trident.netapp.io --wait=false
kubectl delete crd tridentbackendconfigs.trident.netapp.io --wait=false
kubectl delete crd tridentstorageclasses.trident.netapp.io --wait=false
kubectl delete crd tridentvolumes.trident.netapp.io --wait=false
kubectl delete crd tridentvolumepublications.trident.netapp.io --wait=false
kubectl delete crd tridentnodes.trident.netapp.io --wait=false
kubectl delete crd tridenttransactions.trident.netapp.io --wait=false
kubectl delete crd tridentsnapshots.trident.netapp.io --wait=false
kubectl delete crd tridentgroupsnapshots.trident.netapp.io --wait=false
kubectl delete crd tridentvolumereferences.trident.netapp.io --wait=false
kubectl delete crd tridentactionsnapshotrestores.trident.netapp.io --wait=false

kubectl patch crd tridentversions.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentbackends.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentmirrorrelationships.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentactionmirrorupdates.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentbackendconfigs.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentstorageclasses.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentvolumes.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentvolumepublications.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentnodes.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridenttransactions.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentsnapshots.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentgroupsnapshots.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentvolumereferences.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge
kubectl patch crd tridentactionsnapshotrestores.trident.netapp.io -p '{"metadata":{"finalizers": []}}' --type=merge

kubectl delete crd tridentversions.trident.netapp.io
kubectl delete crd tridentbackends.trident.netapp.io
kubectl delete crd tridentmirrorrelationships.trident.netapp.io
kubectl delete crd tridentactionmirrorupdates.trident.netapp.io
kubectl delete crd tridentbackendconfigs.trident.netapp.io
kubectl delete crd tridentstorageclasses.trident.netapp.io
kubectl delete crd tridentvolumes.trident.netapp.io
kubectl delete crd tridentvolumepublications.trident.netapp.io
kubectl delete crd tridentnodes.trident.netapp.io
kubectl delete crd tridenttransactions.trident.netapp.io
kubectl delete crd tridentsnapshots.trident.netapp.io
kubectl delete crd tridentgroupsnapshots.trident.netapp.io
kubectl delete crd tridentvolumereferences.trident.netapp.io
kubectl delete crd tridentactionsnapshotrestores.trident.netapp.io
*/

const tridentVersionCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentversions.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: Version
        type: string
        description: The Trident version
        priority: 0
        jsonPath: .trident_version
  scope: Namespaced
  names:
    plural: tridentversions
    singular: tridentversion
    kind: TridentVersion
    shortNames:
    - tver
    - tversion
    categories:
    - trident
    - trident-internal`

const tridentBackendCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentbackends.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: Backend
        type: string
        description: The backend name
        priority: 0
        jsonPath: .backendName
      - name: Backend UUID
        type: string
        description: The backend UUID
        priority: 0
        jsonPath: .backendUUID
  scope: Namespaced
  names:
    plural: tridentbackends
    singular: tridentbackend
    kind: TridentBackend
    shortNames:
    - tbe
    - tbackend
    categories:
    - trident
    - trident-internal`

const tridentMirrorRelationshipCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentmirrorrelationships.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                state:
                  type: string
                  enum:
                  - ""
                  - promoted
                  - established
                  - reestablished
                replicationPolicy:
                  type: string
                replicationSchedule:
                  type: string
                volumeMappings:
                  items:
                    type: object
                    properties:
                      promotedSnapshotHandle:
                        type: string
                      localPVCName:
                        type: string
                      remoteVolumeHandle:
                        type: string
                    required:
                    - localPVCName
                  minItems: 1
                  maxItems: 1
                  type: array
              required:
              - volumeMappings
            status:
              type: object
              properties:
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      lastTransitionTime:
                        type: string
                      localPVCName:
                        type: string
                      localVolumeHandle:
                        type: string
                      remoteVolumeHandle:
                        type: string
                      message:
                        type: string
                      observedGeneration:
                        type: integer
                      state:
                        type: string
                      replicationPolicy:
                        type: string
                      replicationSchedule:
                        type: string
      subresources:
        status: {}
      additionalPrinterColumns:
      - description: The desired mirror state
        jsonPath: .spec.state
        name: Desired State
        type: string
      - description: Local PVCs for the mirror
        jsonPath: .spec.volumeMappings[*].localPVCName
        name: Local PVC
        type: string
      - description: Status
        jsonPath: .status.conditions[*].state
        name: Actual state
        type: string
      - description: Status message
        jsonPath: .status.conditions[*].message
        name: Message
        type: string
  scope: Namespaced
  names:
    plural: tridentmirrorrelationships
    singular: tridentmirrorrelationship
    kind: TridentMirrorRelationship
    shortNames:
    - tmr
    - tmrelationship
    - tmirrorrelationship
    categories:
    - trident
    - trident-internal
    - trident-external
`

const tridentActionMirrorUpdateCRDYAMLv1 = `
 apiVersion: apiextensions.k8s.io/v1
 kind: CustomResourceDefinition
 metadata:
   name: tridentactionmirrorupdates.trident.netapp.io
 spec:
   group: trident.netapp.io
   versions:
     - name: v1
       served: true
       storage: true
       schema:
         openAPIV3Schema:
           type: object
           x-kubernetes-preserve-unknown-fields: true
       additionalPrinterColumns:
         - description: Namespace
           jsonPath: .metadata.namespace
           name: Namespace
           type: string
           priority: 0
         - description: State
           jsonPath: .status.state
           name: State
           type: string
           priority: 0
         - description: CompletionTime
           jsonPath: .status.completionTime
           name: CompletionTime
           type: date
           priority: 0
         - description: Message
           jsonPath: .status.message
           name: Message
           type: string
           priority: 1
         - description: LocalVolumeHandle
           jsonPath: .status.localVolumeHandle
           name: LocalVolumeHandle
           type: string
           priority: 1
         - description: RemoteVolumeHandle
           jsonPath: .status.remoteVolumeHandle
           name: RemoteVolumeHandle
           type: string
           priority: 1
   scope: Namespaced
   names:
     plural: tridentactionmirrorupdates
     singular: tridentactionmirrorupdate
     kind: TridentActionMirrorUpdate
     shortNames:
     - tamu
     - tamupdate
     - tamirrorupdate
     categories:
     - trident
     - trident-external
 `

const tridentSnapshotInfoCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentsnapshotinfos.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                snapshotName:
                  type: string
              required:
              - snapshotName
            status:
              type: object
              properties:
                lastTransitionTime:
                  type: string
                observedGeneration:
                  type: integer
                snapshotHandle:
                  type: string
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Snapshot Handle
          type: string
          description: VolumeSnapshotContent Handle
          priority: 0
          jsonPath: .status.snapshotHandle
  scope: Namespaced
  names:
    plural: tridentsnapshotinfos
    singular: tridentsnapshotinfo
    kind: TridentSnapshotInfo
    shortNames:
    - tsi
    - tsinfo
    - tsnapshotinfo
    categories:
    - trident
    - trident-internal
    - trident-external
`

const tridentBackendConfigCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentbackendconfigs.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
      additionalPrinterColumns:
      - name: Backend Name
        type: string
        description: The backend name
        priority: 0
        jsonPath: .status.backendInfo.backendName
      - name: Backend UUID
        type: string
        description: The backend UUID
        priority: 0
        jsonPath: .status.backendInfo.backendUUID
      - name: Phase
        type: string
        description: The backend config phase
        priority: 0
        jsonPath: .status.phase
      - name: Status
        type: string
        description: The result of the last operation
        priority: 0
        jsonPath: .status.lastOperationStatus
      - name: Storage Driver
        type: string
        description: The storage driver type
        priority: 1
        jsonPath: .spec.storageDriverName
      - name: Deletion Policy
        type: string
        description: The deletion policy
        priority: 1
        jsonPath: .status.deletionPolicy
  scope: Namespaced
  names:
    plural: tridentbackendconfigs
    singular: tridentbackendconfig
    kind: TridentBackendConfig
    shortNames:
    - tbc
    - tbconfig
    - tbackendconfig
    categories:
    - trident
    - trident-internal
    - trident-external`

const tridentConfiguratorCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentconfigurators.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
      additionalPrinterColumns:
      - name: Phase
        type: string
        description: The backend config phase
        priority: 0
        jsonPath: .status.phase
      - name: Status
        type: string
        description: The result of the last operation
        priority: 0
        jsonPath: .status.lastOperationStatus
      - name: Cloud Provider
        type: string
        description: The name of cloud provider
        priority: 0
        jsonPath: .status.cloudProvider
      - name: Storage Driver
        type: string
        description: The storage driver type
        priority: 1
        jsonPath: .spec.storageDriverName
      - name: Deletion Policy
        type: string
        description: The deletion policy
        priority: 1
        jsonPath: .status.deletionPolicy
  scope: Cluster
  names:
    plural: tridentconfigurators
    singular: tridentconfigurator
    kind: TridentConfigurator
    shortNames:
    - tconf
    - tconfigurator
    categories:
    - trident
    - trident-internal
    - trident-external`

const tridentStorageClassCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentstorageclasses.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: tridentstorageclasses
    singular: tridentstorageclass
    kind: TridentStorageClass
    shortNames:
    - tsc
    - tstorageclass
    categories:
    - trident
    - trident-internal`

const tridentVolumeCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentvolumes.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: Age
        type: date
        priority: 0
        jsonPath: .metadata.creationTimestamp
      - name: Size
        type: string
        description: The volume's size
        priority: 1
        jsonPath: .config.size
      - name: Storage Class
        type: string
        description: The volume's storage class
        priority: 1
        jsonPath: .config.storageClass
      - name: State
        type: string
        description: The volume's state
        priority: 1
        jsonPath: .state
      - name: Protocol
        type: string
        description: The volume's protocol
        priority: 1
        jsonPath: .config.protocol
      - name: Backend UUID
        type: string
        description: The volume's backend UUID
        priority: 1
        jsonPath: .backendUUID
      - name: Pool
        type: string
        description: The volume's pool
        priority: 1
        jsonPath: .pool
  scope: Namespaced
  names:
    plural: tridentvolumes
    singular: tridentvolume
    kind: TridentVolume
    shortNames:
    - tvol
    - tvolume
    categories:
    - trident
    - trident-internal`

const tridentVolumePublicationCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentvolumepublications.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            volumeID:
              type: string
            nodeID:
              type: string
            readOnly:
              type: boolean
            accessMode:
              type: integer
              format: int32
          required:
              - volumeID
              - nodeID
              - readOnly
      additionalPrinterColumns:
        - name: Volume
          type: string
          description: Volume ID
          priority: 0
          jsonPath: .volumeID
        - name: Node
          type: string
          description: Node ID
          priority: 0
          jsonPath: .nodeID
  scope: Namespaced
  names:
    plural: tridentvolumepublications
    singular: tridentvolumepublication
    kind: TridentVolumePublication
    shortNames:
    - tvp
    - tvpub
    - tvpublication
    - tvolpub
    - tvolumepub
    - tvolpublication
    - tvolumepublication
    categories:
    - trident
    - trident-internal`

const tridentNodeCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentnodes.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: tridentnodes
    singular: tridentnode
    kind: TridentNode
    shortNames:
    - tnode
    categories:
    - trident
    - trident-internal`

const tridentTransactionCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridenttransactions.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: tridenttransactions
    singular: tridenttransaction
    kind: TridentTransaction
    shortNames:
    - ttx
    - ttransaction
    categories:
    - trident-internal`

const tridentSnapshotCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentsnapshots.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
          openAPIV3Schema:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: State
        type: string
        description: The snapshot's state
        priority: 1
        jsonPath: .state
  scope: Namespaced
  names:
    plural: tridentsnapshots
    singular: tridentsnapshot
    kind: TridentSnapshot
    shortNames:
    - tss
    - tsnap
    - tsnapshot
    categories:
    - trident
    - trident-internal`

const tridentGroupSnapshotCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentgroupsnapshots.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - name: Created
        type: date
        description: Creation time of the group snapshot
        jsonPath: .dateCreated
  scope: Namespaced
  names:
    plural: tridentgroupsnapshots
    singular: tridentgroupsnapshot
    kind: TridentGroupSnapshot
    shortNames:
      - tgroupsnapshot
      - tgroupsnap
      - tgsnapshot
      - tgsnap
      - tgss
    categories:
      - trident
      - trident-internal`

const tridentOrchestratorCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentorchestrators.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
  names:
    kind: TridentOrchestrator
    listKind: TridentOrchestratorList
    plural: tridentorchestrators
    singular: tridentorchestrator
    shortNames:
    - torc
    - torchestrator
  scope: Cluster`

const tridentVolumeReferenceCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentvolumereferences.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                pvcName:
                  type: string
                pvcNamespace:
                  type: string
              required:
              - pvcName
              - pvcNamespace
      additionalPrinterColumns:
  scope: Namespaced
  names:
    plural: tridentvolumereferences
    singular: tridentvolumereference
    kind: TridentVolumeReference
    shortNames:
    - tvr
    - tvref
    categories:
    - trident
    - trident-external
    - trident-internal`

const tridentActionSnapshotRestoreCRDYAMLv1 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tridentactionsnapshotrestores.trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
      - description: Namespace
        jsonPath: .metadata.namespace
        name: Namespace
        type: string
        priority: 0
      - description: PVC
        jsonPath: .spec.pvcName
        name: PVC
        type: string
        priority: 0
      - description: Snapshot
        jsonPath: .spec.volumeSnapshotName
        name: Snapshot
        type: string
        priority: 0
      - description: State
        jsonPath: .status.state
        name: State
        type: string
        priority: 0
      - description: CompletionTime
        jsonPath: .status.completionTime
        name: CompletionTime
        type: date
        priority: 0
      - description: Message
        jsonPath: .status.message
        name: Message
        type: string
        priority: 1
  scope: Namespaced
  names:
    plural: tridentactionsnapshotrestores
    singular: tridentactionsnapshotrestore
    kind: TridentActionSnapshotRestore
    shortNames:
    - tasr
    categories:
    - trident
    - trident-external`

const customResourceDefinitionYAMLv1 = tridentVersionCRDYAMLv1 +
	"\n---" + tridentBackendCRDYAMLv1 +
	"\n---" + tridentBackendConfigCRDYAMLv1 +
	"\n---" + tridentMirrorRelationshipCRDYAMLv1 +
	"\n---" + tridentActionMirrorUpdateCRDYAMLv1 +
	"\n---" + tridentSnapshotInfoCRDYAMLv1 +
	"\n---" + tridentStorageClassCRDYAMLv1 +
	"\n---" + tridentVolumeCRDYAMLv1 +
	"\n---" + tridentVolumePublicationCRDYAMLv1 +
	"\n---" + tridentNodeCRDYAMLv1 +
	"\n---" + tridentTransactionCRDYAMLv1 +
	"\n---" + tridentSnapshotCRDYAMLv1 +
	"\n---" + tridentGroupSnapshotCRDYAMLv1 +
	"\n---" + tridentVolumeReferenceCRDYAMLv1 +
	"\n---" + tridentActionSnapshotRestoreCRDYAMLv1 +
	"\n---" + tridentConfiguratorCRDYAMLv1 + "\n"

func GetCSIDriverYAML(name, fsGroupPolicy string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"Name":                 name,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetCSIDriverYAML")
	defer func() { Log().Trace("<<<< GetCSIDriverYAML") }()

	switch strings.ToLower(fsGroupPolicy) {
	case "", strings.ToLower(string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy)):
		fsGroupPolicy = string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy)
	case strings.ToLower(string(storagev1.NoneFSGroupPolicy)):
		fsGroupPolicy = string(storagev1.NoneFSGroupPolicy)
	case strings.ToLower(string(storagev1.FileFSGroupPolicy)):
		fsGroupPolicy = string(storagev1.FileFSGroupPolicy)
	}

	csiDriver := strings.ReplaceAll(CSIDriverYAMLv1, "{NAME}", name)
	csiDriver = strings.ReplaceAll(csiDriver, "{FS_GROUP_POLICY}", fsGroupPolicy)
	csiDriver = yaml.ReplaceMultilineTag(csiDriver, "LABELS", constructLabels(labels))
	csiDriver = yaml.ReplaceMultilineTag(csiDriver, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", csiDriver).Trace("CSI Driver YAML")
	return csiDriver
}

const CSIDriverYAMLv1 = `
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: {NAME}
  {LABELS}
  {OWNER_REF}
spec:
  attachRequired: true
  fsGroupPolicy: {FS_GROUP_POLICY}
`

func constructNodeSelector(nodeLabels map[string]string) string {
	var nodeSelector string

	if nodeLabels != nil {
		for key, value := range nodeLabels {
			nodeSelector += fmt.Sprintf("- key: %s\n  operator: In\n  values:\n  - '%s'\n", key, value)
		}
	}
	return nodeSelector
}

func constructTolerations(tolerations []map[string]string) string {
	var tolerationsString string

	if tolerations != nil && len(tolerations) > 0 {
		tolerationsString += "tolerations:\n"
		for _, t := range tolerations {
			toleration := ""
			if t["key"] != "" {
				toleration += fmt.Sprintf("  key: \"%s\"\n", t["key"])
			}
			if t["value"] != "" {
				toleration += fmt.Sprintf("  value: \"%s\"\n", t["value"])
			}
			if t["effect"] != "" {
				toleration += fmt.Sprintf("  effect: \"%s\"\n", t["effect"])
			}
			if t["operator"] != "" {
				toleration += fmt.Sprintf("  operator: \"%s\"\n", t["operator"])
			}
			if t["tolerationSeconds"] != "" {
				toleration += fmt.Sprintf("  tolerationSeconds: %s\n", t["tolerationSeconds"])
			}
			if toleration != "" {
				toleration, _ = collection.ReplaceAtIndex(toleration, '-', 0)
				tolerationsString += toleration
			}
		}
	} else if len(tolerations) == 0 {
		tolerationsString = "tolerations: []\n"
	}

	return tolerationsString
}

func constructLabels(labels map[string]string) string {
	var labelData string

	if labels != nil {
		labelData += "labels:\n"
		for key, value := range labels {
			labelData += fmt.Sprintf("  %s: %s\n", key, value)
		}
	}

	return labelData
}

func constructOwnerRef(ownerRef map[string]string) string {
	var ownerRefData string
	if ownerRef != nil {
		isFirst := true
		ownerRefData += "ownerReferences:\n"
		for key, value := range ownerRef {
			if isFirst {
				ownerRefData += fmt.Sprintf("- %s: %s\n", key, value)
				isFirst = false
			} else {
				ownerRefData += fmt.Sprintf("  %s: %s\n", key, value)
			}
		}
	}

	return ownerRefData
}

func constructImagePullSecrets(imagePullSecrets []string) string {
	var imagePullSecretsData string
	if len(imagePullSecrets) > 0 {
		imagePullSecretsData += "imagePullSecrets:\n"
		for _, value := range imagePullSecrets {
			imagePullSecretsData += fmt.Sprintf("- name: %s\n", value)
		}
	}

	return imagePullSecretsData
}

func constructServiceAccountSecrets(serviceAccountSecrets []string) string {
	var serviceAccountSecretsData string
	if len(serviceAccountSecrets) > 0 {
		serviceAccountSecretsData += "secrets:\n"
		for _, value := range serviceAccountSecrets {
			serviceAccountSecretsData += fmt.Sprintf("- name: %s\n", value)
		}
	}

	return serviceAccountSecretsData
}

func constructServiceAccountAnnotation(cloudIdentity string) string {
	serviceAccountAnnotationData := "annotations:\n"
	serviceAccountAnnotationData += fmt.Sprintf("    %s", cloudIdentity)

	return serviceAccountAnnotationData
}

func getBurstValueForQPS(queriesPerSecond int) int {
	// The burst value is set to twice the QPS value, which seems to be a common practice
	return queriesPerSecond * 2
}
