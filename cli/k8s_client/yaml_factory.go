// Copyright 2023 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"strconv"
	"strings"

	commonconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
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
	serviceAccountName string, secrets []string, labels, controllingCRDetails map[string]string,
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

	saYAML = strings.ReplaceAll(saYAML, "{NAME}", serviceAccountName)
	saYAML = replaceMultilineYAMLTag(saYAML, "LABELS", constructLabels(labels))
	saYAML = replaceMultilineYAMLTag(saYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	// Log().ervice account YAML before the secrets are added.
	Log().WithField("yaml", saYAML).Trace("Service account YAML.")
	saYAML = strings.Replace(saYAML, "{SECRETS}", constructServiceAccountSecrets(secrets), 1)
	return saYAML
}

const serviceAccountYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {NAME}
  {LABELS}
  {OWNER_REF}
`

const serviceAccountWithSecretYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {NAME}
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
	clusterRoleYAML = replaceMultilineYAMLTag(clusterRoleYAML, "LABELS", constructLabels(labels))
	clusterRoleYAML = replaceMultilineYAMLTag(clusterRoleYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))
	Log().WithField("yaml", clusterRoleYAML).Trace("Cluster role YAML.")
	return clusterRoleYAML
}

// Specific permissions for sidecars
// csi-resizer needs 'list' for pods
// csi-snapshotter needs 'watch' for volumesnapshotclasses
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
    verbs: ["get", "list", "watch", "create", "update", "patch"]
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
  - apiGroups: ["storage.k8s.io"]
    resources: ["csinodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["trident.netapp.io"]
    resources: ["tridentversions", "tridentbackends", "tridentstorageclasses", "tridentvolumes","tridentnodes",
"tridenttransactions", "tridentsnapshots", "tridentbackendconfigs", "tridentbackendconfigs/status",
"tridentmirrorrelationships", "tridentmirrorrelationships/status", "tridentsnapshotinfos",
"tridentsnapshotinfos/status", "tridentvolumepublications", "tridentvolumereferences",
"tridentactionmirrorupdates", "tridentactionmirrorupdates/status",
"tridentactionsnapshotrestores", "tridentactionsnapshotrestores/status"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    verbs: ["use"]
    resourceNames:
      - {CLUSTER_ROLE_NAME}
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
	roleYAML = replaceMultilineYAMLTag(roleYAML, "LABELS", constructLabels(labels))
	roleYAML = replaceMultilineYAMLTag(roleYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
	rbYAML = replaceMultilineYAMLTag(rbYAML, "LABELS", constructLabels(labels))
	rbYAML = replaceMultilineYAMLTag(rbYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))
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
	crbYAML = replaceMultilineYAMLTag(crbYAML, "LABELS", constructLabels(labels))
	crbYAML = replaceMultilineYAMLTag(crbYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
	serviceYAML = replaceMultilineYAMLTag(serviceYAML, "LABELS", constructLabels(labels))
	serviceYAML = replaceMultilineYAMLTag(serviceYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
	resourceQuotaYAML = replaceMultilineYAMLTag(resourceQuotaYAML, "LABELS", constructLabels(labels))
	resourceQuotaYAML = replaceMultilineYAMLTag(resourceQuotaYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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

func GetCSIDeploymentYAML(args *DeploymentYAMLArguments) string {
	var debugLine, sideCarLogLevel, ipLocalhost string
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
	provisionerFeatureGates := ""
	if args.TopologyEnabled {
		provisionerFeatureGates = "- --feature-gates=Topology=True"
	}

	if args.Labels == nil {
		args.Labels = make(map[string]string)
	}
	args.Labels[DefaultContainerLabelKey] = "trident-main"

	autosupportDebugLine := "- -debug"
	if !IsLogLevelDebugOrHigher(args.LogLevel) {
		autosupportDebugLine = "#" + autosupportDebugLine
	}

	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DEPLOYMENT_NAME}", args.DeploymentName)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{CSI_SIDECAR_REGISTRY}", args.ImageRegistry)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LABEL_APP}", args.Labels[TridentAppLabelKey])
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{SIDECAR_LOG_LEVEL}", sideCarLogLevel)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_FORMAT}", args.LogFormat)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DEBUG}", debugLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_LEVEL}", args.LogLevel)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_WORKFLOWS}", args.LogWorkflows)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{LOG_LAYERS}", args.LogLayers)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{DISABLE_AUDIT_LOG}", strconv.FormatBool(args.DisableAuditLog))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{IP_LOCALHOST}", ipLocalhost)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_IMAGE}", args.AutosupportImage)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_PROXY}", autosupportProxyLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_CUSTOM_URL}", autosupportCustomURLLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_SERIAL_NUMBER}", autosupportSerialNumberLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_HOSTNAME}", autosupportHostnameLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_DEBUG}", autosupportDebugLine)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{AUTOSUPPORT_SILENCE}",
		strconv.FormatBool(args.SilenceAutosupport))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{PROVISIONER_FEATURE_GATES}", provisionerFeatureGates)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{HTTP_REQUEST_TIMEOUT}", args.HTTPRequestTimeout)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{IMAGE_PULL_POLICY}", args.ImagePullPolicy)
	deploymentYAML = replaceMultilineYAMLTag(deploymentYAML, "LABELS", constructLabels(args.Labels))
	deploymentYAML = replaceMultilineYAMLTag(deploymentYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	deploymentYAML = replaceMultilineYAMLTag(deploymentYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	deploymentYAML = replaceMultilineYAMLTag(deploymentYAML, "NODE_TOLERATIONS", constructTolerations(args.Tolerations))
	deploymentYAML = strings.ReplaceAll(deploymentYAML, "{ENABLE_FORCE_DETACH}", strconv.FormatBool(args.EnableForceDetach))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", deploymentYAML).Trace("CSI Deployment YAML.")
	deploymentYAML = replaceMultilineYAMLTag(deploymentYAML, "IMAGE_PULL_SECRETS",
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
        - "--metrics"
        {DEBUG}
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
        - name: CSI_ENDPOINT
          value: unix://plugin/csi.sock
        - name: TRIDENT_SERVER
          value: "{IP_LOCALHOST}:8000"
        volumeMounts:
        - name: socket-dir
          mountPath: /plugin
        - name: certs
          mountPath: /certs
          readOnly: true
      - name: trident-autosupport
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
        {AUTOSUPPORT_PROXY}
        {AUTOSUPPORT_CUSTOM_URL}
        {AUTOSUPPORT_SERIAL_NUMBER}
        {AUTOSUPPORT_HOSTNAME}
        {AUTOSUPPORT_DEBUG}
        resources:
          limits:
            memory: 1Gi
        volumeMounts:
        - name: asup-dir
          mountPath: /asup
      - name: csi-provisioner
        image: {CSI_SIDECAR_REGISTRY}/csi-provisioner:v3.4.1
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
        {PROVISIONER_FEATURE_GATES}
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-attacher
        image: {CSI_SIDECAR_REGISTRY}/csi-attacher:v4.2.0
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=60s"
        - "--retry-interval-start=10s"
        - "--csi-address=$(ADDRESS)"
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-resizer
        image: {CSI_SIDECAR_REGISTRY}/csi-resizer:v1.7.0
        imagePullPolicy: {IMAGE_PULL_POLICY}
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=300s"
        - "--csi-address=$(ADDRESS)"
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-snapshotter
        image: {CSI_SIDECAR_REGISTRY}/csi-snapshotter:v6.2.1
        imagePullPolicy: {IMAGE_PULL_POLICY}
        securityContext:
          capabilities:
            drop:
            - all
        args:
        - "--v={SIDECAR_LOG_LEVEL}"
        - "--timeout=300s"
        - "--csi-address=$(ADDRESS)"
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
      - name: asup-dir
        emptyDir:
          medium: ""
          sizeLimit: 1Gi
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

	if args.ImageRegistry == "" {
		args.ImageRegistry = commonconfig.KubernetesCSISidecarRegistry
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

	kubeletDir := strings.TrimRight(args.KubeletDir, "/")
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DAEMONSET_NAME}", args.DaemonsetName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{CSI_SIDECAR_REGISTRY}", args.ImageRegistry)
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
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "NODE_TOLERATIONS", constructTolerations(tolerations))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "LABELS", constructLabels(args.Labels))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", daemonSetYAML).Trace("CSI Daemonset Windows YAML.")
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "IMAGE_PULL_SECRETS",
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

	if args.ImageRegistry == "" {
		args.ImageRegistry = commonconfig.KubernetesCSISidecarRegistry
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

	kubeletDir := strings.TrimRight(args.KubeletDir, "/")
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{DAEMONSET_NAME}", args.DaemonsetName)
	daemonSetYAML = strings.ReplaceAll(daemonSetYAML, "{CSI_SIDECAR_REGISTRY}", args.ImageRegistry)
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
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "NODE_SELECTOR", constructNodeSelector(args.NodeSelector))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "NODE_TOLERATIONS", constructTolerations(tolerations))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "LABELS", constructLabels(args.Labels))
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", daemonSetYAML).Trace("CSI Daemonset Linux YAML.")
	daemonSetYAML = replaceMultilineYAMLTag(daemonSetYAML, "IMAGE_PULL_SECRETS",
		constructImagePullSecrets(args.ImagePullSecrets))

	return daemonSetYAML
}

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
    spec:
      serviceAccount: {SERVICE_ACCOUNT}
      hostNetwork: true
      hostIPC: true
      hostPID: true
      dnsPolicy: ClusterFirstWithHostNet
      priorityClassName: system-node-critical
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
        {DEBUG}
        startupProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 5
          timeoutSeconds: 1
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 3
          timeoutSeconds: 1
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readiness
            scheme: HTTPS
            port: {PROBE_PORT}
          failureThreshold: 5
          initialDelaySeconds: 10
          periodSeconds: 10
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
        image: {CSI_SIDECAR_REGISTRY}/csi-node-driver-registrar:v2.7.0
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
          timeoutSeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /liveness
            scheme: HTTPS
            port: healthz
          failureThreshold: 3
          timeoutSeconds: 3
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readiness
            scheme: HTTPS
            port: healthz
          failureThreshold: 5
          initialDelaySeconds: 10
          timeoutSeconds: 3
          periodSeconds: 10
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
        image: {CSI_SIDECAR_REGISTRY}/csi-node-driver-registrar:v2.7.0
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
        image: {CSI_SIDECAR_REGISTRY}/livenessprobe:v2.5.0
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
		"TridentImae":          args.TridentImage,
		"ServiceAccountName":   args.ServiceAccountName,
		"Labels":               args.Labels,
		"ControllingCRDetails": args.ControllingCRDetails,
	}).Trace(">>>> GetTridentVersionPodYAML")
	defer func() { Log().Trace("<<<< GetTridentVersionPodYAML") }()

	versionPodYAML := strings.ReplaceAll(tridentVersionPodYAML, "{NAME}", args.TridentVersionPodName)
	versionPodYAML = strings.ReplaceAll(versionPodYAML, "{TRIDENT_IMAGE}", args.TridentImage)
	versionPodYAML = strings.ReplaceAll(versionPodYAML, "{SERVICE_ACCOUNT}", args.ServiceAccountName)
	versionPodYAML = replaceMultilineYAMLTag(versionPodYAML, "LABELS", constructLabels(args.Labels))
	versionPodYAML = replaceMultilineYAMLTag(versionPodYAML, "OWNER_REF", constructOwnerRef(args.ControllingCRDetails))
	versionPodYAML = replaceMultilineYAMLTag(versionPodYAML, "NODE_TOLERATIONS", constructTolerations(args.Tolerations))

	// Log before secrets are inserted into YAML.
	Log().WithField("yaml", versionPodYAML).Trace("Trident Version Pod YAML.")
	versionPodYAML = replaceMultilineYAMLTag(versionPodYAML, "IMAGE_PULL_SECRETS",
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
	sccYAML = replaceMultilineYAMLTag(sccYAML, "LABELS", constructLabels(labels))
	sccYAML = replaceMultilineYAMLTag(sccYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
priority: 10
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
	secretYAML = replaceMultilineYAMLTag(secretYAML, "LABELS", constructLabels(labels))
	secretYAML = replaceMultilineYAMLTag(secretYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
	"\n---" + tridentVolumeReferenceCRDYAMLv1 +
	"\n---" + tridentActionSnapshotRestoreCRDYAMLv1 + "\n"

func GetCSIDriverYAML(name string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"Name":                 name,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetCSIDriverYAML")
	defer func() { Log().Trace("<<<< GetCSIDriverYAML") }()

	csiDriver := strings.ReplaceAll(CSIDriverYAMLv1, "{NAME}", name)
	csiDriver = replaceMultilineYAMLTag(csiDriver, "LABELS", constructLabels(labels))
	csiDriver = replaceMultilineYAMLTag(csiDriver, "OWNER_REF", constructOwnerRef(controllingCRDetails))

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
`

func GetPrivilegedPodSecurityPolicyYAML(pspName string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"Name":                 pspName,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetPrivilegedPodSecurityPolicyYAML")
	defer func() { Log().Trace("<<<< GetPrivilegedPodSecurityPolicyYAML") }()

	pspYAML := strings.ReplaceAll(PrivilegedPodSecurityPolicyYAML, "{PSP_NAME}", pspName)
	pspYAML = replaceMultilineYAMLTag(pspYAML, "LABELS", constructLabels(labels))
	pspYAML = replaceMultilineYAMLTag(pspYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", pspYAML).Trace("Privileged Pod Security Policy YAML")
	return pspYAML
}

const PrivilegedPodSecurityPolicyYAML = `
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: {PSP_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  privileged: true
  allowPrivilegeEscalation: true
  allowedCapabilities:
  - SYS_ADMIN
  hostIPC: true
  hostPID: true
  hostNetwork: true
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  runAsUser:
    rule: RunAsAny
  fsGroup:
    rule: RunAsAny
  volumes:
  - hostPath
  - projected
  - emptyDir
`

func GetUnprivilegedPodSecurityPolicyYAML(pspName string, labels, controllingCRDetails map[string]string) string {
	Log().WithFields(LogFields{
		"Name":                 pspName,
		"Labels":               labels,
		"ControllingCRDetails": controllingCRDetails,
	}).Trace(">>>> GetUnprivilegedPodSecurityPolicyYAML")
	defer func() { Log().Trace("<<<< GetUnprivilegedPodSecurityPolicyYAML") }()

	pspYAML := strings.ReplaceAll(UnprivilegedPodSecurityPolicyYAML, "{PSP_NAME}", pspName)
	pspYAML = replaceMultilineYAMLTag(pspYAML, "LABELS", constructLabels(labels))
	pspYAML = replaceMultilineYAMLTag(pspYAML, "OWNER_REF", constructOwnerRef(controllingCRDetails))

	Log().WithField("yaml", pspYAML).Trace("Unprivileged Pod Security Policy YAML")
	return pspYAML
}

const UnprivilegedPodSecurityPolicyYAML = `
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: {PSP_NAME}
  {LABELS}
  {OWNER_REF}
spec:
  privileged: false
  allowPrivilegeEscalation: false
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  runAsUser:
    rule: RunAsAny
  fsGroup:
    rule: RunAsAny
  volumes:
  - hostPath
  - projected
  - emptyDir
`

func shiftTextRight(text string, count int) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	newLines := make([]string, len(lines))
	for i, line := range lines {
		if line != "" {
			newLines[i] = strings.Repeat(" ", count) + line
		}
	}

	return strings.Join(newLines, "\n")
}

func replaceMultilineYAMLTag(originalYAML, tag, tagText string) string {
	for {
		tagWithSpaces, spaceCount := utils.GetYAMLTagWithSpaceCount(originalYAML, tag)

		if tagWithSpaces == "" {
			break
		}
		originalYAML = strings.Replace(originalYAML, tagWithSpaces, shiftTextRight(tagText, spaceCount), 1)
	}

	return originalYAML
}

func constructNodeSelector(nodeLabels map[string]string) string {
	var nodeSelector string

	if nodeLabels != nil {
		for key, value := range nodeLabels {
			nodeSelector += fmt.Sprintf("- key: %s\n  operator: In\n  values:\n  - %s\n", key, value)
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
				toleration, _ = utils.ReplaceAtIndex(toleration, '-', 0)
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
