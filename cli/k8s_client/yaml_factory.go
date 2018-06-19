package k8s_client

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/netapp/trident/utils"
)

func GetNamespaceYAML(namespace string) string {
	return strings.Replace(namespaceYAMLTemplate, "{NAMESPACE}", namespace, 1)
}

const namespaceYAMLTemplate = `---
apiVersion: v1
kind: Namespace
metadata:
  name: {NAMESPACE}
`

func GetServiceAccountYAML() string {
	return serviceAccountYAML
}

const serviceAccountYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: trident
`

func GetClusterRoleYAML(flavor OrchestratorFlavor, version *utils.Version) string {
	switch flavor {
	case FlavorOpenShift:
		return clusterRoleOpenShiftYAML
	default:
		fallthrough
	case FlavorKubernetes:
		if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			return clusterRoleKubernetesV1YAML
		} else {
			return clusterRoleKubernetesV1Alpha1YAML
		}
	}
}

const clusterRoleOpenShiftYAML = `---
kind: ClusterRole
apiVersion: v1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete"]
`

const clusterRoleKubernetesV1YAML = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete"]
`

const clusterRoleKubernetesV1Alpha1YAML = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete"]
`

func GetClusterRoleBindingYAML(namespace string, flavor OrchestratorFlavor, version *utils.Version) string {
	switch flavor {
	case FlavorOpenShift:
		return strings.Replace(clusterRoleBindingOpenShiftYAMLTemplate, "{NAMESPACE}", namespace, 1)
	default:
		fallthrough
	case FlavorKubernetes:
		if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			return strings.Replace(clusterRoleBindingKubernetesV1YAMLTemplate,
				"{NAMESPACE}", namespace, 1)
		} else {
			return strings.Replace(clusterRoleBindingKubernetesV1Alpha1YAMLTemplate,
				"{NAMESPACE}", namespace, 1)
		}
	}
}

const clusterRoleBindingOpenShiftYAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: v1 
metadata:
  name: trident
subjects:
  - kind: ServiceAccount
    name: trident
    namespace: {NAMESPACE}
roleRef:
  name: trident
`

const clusterRoleBindingKubernetesV1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: trident
subjects:
  - kind: ServiceAccount
    name: trident
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: trident
  apiGroup: rbac.authorization.k8s.io
`

const clusterRoleBindingKubernetesV1Alpha1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: trident
subjects:
  - kind: ServiceAccount
    name: trident
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: trident
  apiGroup: rbac.authorization.k8s.io
`

func GetDeploymentYAML(pvcName, tridentImage, etcdImage string, debug bool) string {

	var debugLine string
	if debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	deploymentYAML := strings.Replace(deploymentYAMLTemplate, "{TRIDENT_IMAGE}", tridentImage, 1)
	deploymentYAML = strings.Replace(deploymentYAML, "{ETCD_IMAGE}", etcdImage, 1)
	deploymentYAML = strings.Replace(deploymentYAML, "{DEBUG}", debugLine, 1)
	deploymentYAML = strings.Replace(deploymentYAML, "{PVC_NAME}", pvcName, 1)
	return deploymentYAML
}

const deploymentYAMLTemplate = `---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: trident
  labels:
    app: trident.netapp.io
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: trident.netapp.io
    spec:
      serviceAccount: trident
      containers:
      - name: trident-main
        image: {TRIDENT_IMAGE}
        command:
        - /usr/local/bin/trident_orchestrator
        args:
        - -etcd_v3
        - http://127.0.0.1:8001
        - -k8s_pod
        #- -k8s_api_server
        #- __KUBERNETES_SERVER__:__KUBERNETES_PORT__
        {DEBUG}
        livenessProbe:
          exec:
            command:
            - tridentctl
            - -s
            - 127.0.0.1:8000
            - get
            - backend
          failureThreshold: 2
          initialDelaySeconds: 120
          periodSeconds: 120
          timeoutSeconds: 90
      - name: etcd
        image: {ETCD_IMAGE}
        command:
        - /usr/local/bin/etcd
        args:
        - -name
        - etcd1
        - -advertise-client-urls
        - http://127.0.0.1:8001
        - -listen-client-urls
        - http://127.0.0.1:8001
        - -initial-advertise-peer-urls
        - http://127.0.0.1:8002
        - -listen-peer-urls
        - http://127.0.0.1:8002
        - -data-dir
        - /var/etcd/data
        - -initial-cluster
        - etcd1=http://127.0.0.1:8002
        volumeMounts:
        - name: etcd-vol
          mountPath: /var/etcd/data
        livenessProbe:
          exec:
            command:
            - etcdctl
            - -endpoint=http://127.0.0.1:8001/
            - cluster-health
          failureThreshold: 2
          initialDelaySeconds: 15
          periodSeconds: 15
          timeoutSeconds: 10
      volumes:
      - name: etcd-vol
        persistentVolumeClaim:
          claimName: {PVC_NAME}
`

func GetPVCYAML(pvcName, namespace, size string) string {

	pvcYAML := strings.Replace(persistentVolumeClaimYAMLTemplate, "{PVC_NAME}", pvcName, 1)
	pvcYAML = strings.Replace(pvcYAML, "{NAMESPACE}", namespace, 1)
	pvcYAML = strings.Replace(pvcYAML, "{SIZE}", size, 1)
	return pvcYAML
}

const persistentVolumeClaimYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  labels:
    app: trident.netapp.io
  name: {PVC_NAME}
  namespace: {NAMESPACE}
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: {SIZE}
  selector:
    matchLabels:
      app: trident.netapp.io
`

func GetNFSPVYAML(pvName, size, nfsServer, nfsPath string) string {

	pvYAML := strings.Replace(persistentVolumeNFSYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{SERVER}", nfsServer, 1)
	pvYAML = strings.Replace(pvYAML, "{PATH}", nfsPath, 1)
	return pvYAML
}

const persistentVolumeNFSYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: trident.netapp.io
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  nfs:
    server: {SERVER}
    path: {PATH}
`

func GetISCSIPVYAML(pvName, size, targetPortal, iqn string, lun int32) string {

	pvYAML := strings.Replace(persistentVolumeISCSIYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{TARGET_PORTAL}", targetPortal, 1)
	pvYAML = strings.Replace(pvYAML, "{IQN}", iqn, 1)
	pvYAML = strings.Replace(pvYAML, "{LUN}", strconv.FormatInt(int64(lun), 10), 1)
	return pvYAML
}

const persistentVolumeISCSIYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: trident.netapp.io
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  iscsi:
    targetPortal: {TARGET_PORTAL}
    iqn: {IQN}
    lun: {LUN}
    fsType: ext4
    readOnly: false
`

func GetCHAPISCSIPVYAML(pvName, size, targetPortal, iqn string, lun int32, secretName string) string {

	pvYAML := strings.Replace(persistentVolumeCHAPISCSIYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{TARGET_PORTAL}", targetPortal, 1)
	pvYAML = strings.Replace(pvYAML, "{IQN}", iqn, 1)
	pvYAML = strings.Replace(pvYAML, "{LUN}", strconv.FormatInt(int64(lun), 10), 1)
	pvYAML = strings.Replace(pvYAML, "{SECRET_NAME}", secretName, 1)
	return pvYAML
}

const persistentVolumeCHAPISCSIYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: trident.netapp.io
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  iscsi:
    targetPortal: {TARGET_PORTAL}
    iqn: {IQN}
    lun: {LUN}
    fsType: ext4
    readOnly: false
    chapAuthDiscovery: true
    chapAuthSession: true
    secretRef:
      name: {SECRET_NAME}
`

func GetCHAPSecretYAML(secretName, userName, initiatorSecret, targetSecret string) string {

	encodedUserName := base64.StdEncoding.EncodeToString([]byte(userName))
	encodedInitiatorSecret := base64.StdEncoding.EncodeToString([]byte(initiatorSecret))
	encodedTargetSecret := base64.StdEncoding.EncodeToString([]byte(targetSecret))

	secretYAML := strings.Replace(chapSecretYAMLTemplate, "{SECRET_NAME}", secretName, 1)
	secretYAML = strings.Replace(secretYAML, "{USER_NAME}", encodedUserName, -1)
	secretYAML = strings.Replace(secretYAML, "{INITIATOR_SECRET}", encodedInitiatorSecret, -1)
	secretYAML = strings.Replace(secretYAML, "{TARGET_SECRET}", encodedTargetSecret, -1)
	return secretYAML
}

const chapSecretYAMLTemplate = `---
apiVersion: v1
kind: Secret
metadata:
  name: {SECRET_NAME}
type: "kubernetes.io/iscsi-chap"
data:
  discovery.sendtargets.auth.username: {USER_NAME}
  discovery.sendtargets.auth.password: {INITIATOR_SECRET}
  discovery.sendtargets.auth.username_in: {USER_NAME}
  discovery.sendtargets.auth.password_in: {TARGET_SECRET}
  node.session.auth.username: {USER_NAME}
  node.session.auth.password: {INITIATOR_SECRET}
  node.session.auth.username_in: {USER_NAME}
  node.session.auth.password_in: {TARGET_SECRET}
`
