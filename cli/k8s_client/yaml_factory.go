package k8sclient

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

func GetServiceAccountYAML(csi bool) string {

	if csi {
		return strings.Replace(serviceAccountYAML, "{NAME}", "trident-csi", 1)
	} else {
		return strings.Replace(serviceAccountYAML, "{NAME}", "trident", 1)
	}
}

const serviceAccountYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {NAME}
`

func GetClusterRoleYAML(flavor OrchestratorFlavor, version *utils.Version, csi bool) string {
	switch flavor {
	case FlavorOpenShift:
		if csi {
			return clusterRoleOpenShiftCSIYAML
		} else {
			return clusterRoleOpenShiftYAML
		}
	default:
		fallthrough
	case FlavorKubernetes:
		if csi {
			return clusterRoleKubernetesV1CSIYAML
		} else if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			return clusterRoleKubernetesV1YAML
		} else {
			return clusterRoleKubernetesV1Alpha1YAML
		}
	}
}

const clusterRoleOpenShiftYAML = `---
kind: ClusterRole
apiVersion: authorization.openshift.io/v1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["patch"]
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

const clusterRoleOpenShiftCSIYAML = `---
kind: ClusterRole
apiVersion: authorization.openshift.io/v1
metadata:
  name: trident-csi
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
`

const clusterRoleKubernetesV1YAML = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["patch"]
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

const clusterRoleKubernetesV1CSIYAML = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: trident-csi
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
`

const clusterRoleKubernetesV1Alpha1YAML = `---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: trident
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["patch"]
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

func GetClusterRoleBindingYAML(namespace string, flavor OrchestratorFlavor, version *utils.Version, csi bool) string {

	var name string
	var crbYAML string

	if csi {
		name = "trident-csi"
	} else {
		name = "trident"
	}

	switch flavor {
	case FlavorOpenShift:
		crbYAML = clusterRoleBindingOpenShiftYAMLTemplate
	default:
		fallthrough
	case FlavorKubernetes:
		if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			crbYAML = clusterRoleBindingKubernetesV1YAMLTemplate
		} else {
			crbYAML = clusterRoleBindingKubernetesV1Alpha1YAMLTemplate
		}
	}

	crbYAML = strings.Replace(crbYAML, "{NAMESPACE}", namespace, 1)
	crbYAML = strings.Replace(crbYAML, "{NAME}", name, -1)
	return crbYAML
}

const clusterRoleBindingOpenShiftYAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: authorization.openshift.io/v1
metadata:
  name: {NAME}
subjects:
  - kind: ServiceAccount
    name: {NAME}
    namespace: {NAMESPACE}
roleRef:
  name: {NAME}
`

const clusterRoleBindingKubernetesV1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {NAME}
subjects:
  - kind: ServiceAccount
    name: {NAME}
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: {NAME}
  apiGroup: rbac.authorization.k8s.io
`

const clusterRoleBindingKubernetesV1Alpha1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: {NAME}
subjects:
  - kind: ServiceAccount
    name: {NAME}
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: {NAME}
  apiGroup: rbac.authorization.k8s.io
`

func GetDeploymentYAML(pvcName, tridentImage, etcdImage, label string, debug bool) string {

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
	deploymentYAML = strings.Replace(deploymentYAML, "{LABEL}", label, -1)
	return deploymentYAML
}

const deploymentYAMLTemplate = `---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: trident
  labels:
    app: {LABEL}
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: {LABEL}
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

func GetCSIServiceYAML(label string) string {

	serviceYAML := strings.Replace(serviceYAMLTemplate, "{LABEL}", label, -1)
	return serviceYAML
}

const serviceYAMLTemplate = `---
apiVersion: v1
kind: Service
metadata:
  name: trident-csi
  labels:
    app: {LABEL}
spec:
  selector:
    app: {LABEL}
  ports:
    - name: dummy
      port: 12345
`

func GetCSIStatefulSetYAML(pvcName, tridentImage, etcdImage, label string, debug bool) string {

	var debugLine string
	if debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	statefulSetYAML := strings.Replace(statefulSetYAMLTemplate, "{TRIDENT_IMAGE}", tridentImage, 1)
	statefulSetYAML = strings.Replace(statefulSetYAML, "{ETCD_IMAGE}", etcdImage, 1)
	statefulSetYAML = strings.Replace(statefulSetYAML, "{DEBUG}", debugLine, 1)
	statefulSetYAML = strings.Replace(statefulSetYAML, "{PVC_NAME}", pvcName, 1)
	statefulSetYAML = strings.Replace(statefulSetYAML, "{LABEL}", label, -1)
	return statefulSetYAML
}

const statefulSetYAMLTemplate = `---
apiVersion: apps/v1beta1
kind: StatefulSet
metadata:
  name: trident-csi
  labels:
    app: {LABEL}
spec:
  serviceName: "trident-csi"
  replicas: 1
  template:
    metadata:
      labels:
        app: {LABEL}
    spec:
      serviceAccount: trident-csi
      containers:
      - name: trident-main
        image: {TRIDENT_IMAGE}
        command:
        - /usr/local/bin/trident_orchestrator
        args:
        - -etcd_v3
        - http://127.0.0.1:8001
        - "--csi_node_name=$(KUBE_NODE_NAME)"
        - "--csi_endpoint=$(CSI_ENDPOINT)"
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
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: CSI_ENDPOINT
          value: unix://plugin/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /plugin
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
      - name: csi-attacher
        image: quay.io/k8scsi/csi-attacher:v0.2.0
        args:
        - "--v=9"
        - "--csi-address=$(ADDRESS)"
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      - name: csi-provisioner
        image: quay.io/k8scsi/csi-provisioner:v0.2.1
        args:
        - "--v=9"
        - "--provisioner=io.netapp.trident.csi"
        - "--csi-address=$(ADDRESS)"
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        volumeMounts:
        - name: socket-dir
          mountPath: /var/lib/csi/sockets/pluginproxy/
      volumes:
      - name: etcd-vol
        persistentVolumeClaim:
          claimName: {PVC_NAME}
      - name: socket-dir
        emptyDir:
`

func GetCSIDaemonSetYAML(tridentImage, label string, debug bool) string {

	var debugLine string
	if debug {
		debugLine = "- -debug"
	} else {
		debugLine = "#- -debug"
	}

	daemonSetYAML := strings.Replace(daemonSetYAMLTemplate, "{TRIDENT_IMAGE}", tridentImage, 1)
	daemonSetYAML = strings.Replace(daemonSetYAML, "{LABEL}", label, -1)
	daemonSetYAML = strings.Replace(daemonSetYAML, "{DEBUG}", debugLine, 1)
	return daemonSetYAML
}

const daemonSetYAMLTemplate = `---
apiVersion: apps/v1beta2
kind: DaemonSet
metadata:
  name: trident-csi
  labels:
    app: {LABEL}
spec:
  selector:
    matchLabels:
      app: {LABEL}
  template:
    metadata:
      labels:
        app: {LABEL}
    spec:
      serviceAccount: trident-csi
      hostNetwork: true
      hostIPC: true
      containers:
      - name: trident-main
        securityContext:
          privileged: true
          capabilities:
            add: ["SYS_ADMIN"]
          allowPrivilegeEscalation: true
        image: {TRIDENT_IMAGE}
        command:
        - /usr/local/bin/trident_orchestrator
        args:
        - -no_persistence
        - "--csi_node_name=$(KUBE_NODE_NAME)"
        - "--csi_endpoint=$(CSI_ENDPOINT)"
        - "--rest=false"
        {DEBUG}
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.nodeName
        - name: CSI_ENDPOINT
          value: unix://plugin/csi.sock
        - name: PATH
          value: /netapp:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
        volumeMounts:
        - name: plugin-dir
          mountPath: /plugin
        - name: plugins-mount-dir
          mountPath: /var/lib/kubelet/plugins
        - name: pods-mount-dir
          mountPath: /var/lib/kubelet/pods
          mountPropagation: "Bidirectional"
        - name: dev-dir
          mountPath: /dev
        - name: sys-dir
          mountPath: /sys
        - name: host-dir
          mountPath: /host
          mountPropagation: "Bidirectional"
      - name: driver-registrar
        image: quay.io/k8scsi/driver-registrar:v0.2.0
        args:
        - "--v=9"
        - "--csi-address=$(ADDRESS)"
        env:
        - name: ADDRESS
          value: /plugin/csi.sock
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: plugin-dir
          mountPath: /plugin
      volumes:
      - name: plugin-dir
        hostPath:
          path: /var/lib/kubelet/plugins/io.netapp.trident.csi
          type: DirectoryOrCreate
      - name: plugins-mount-dir
        hostPath:
          path: /var/lib/kubelet/plugins
          type: DirectoryOrCreate
      - name: pods-mount-dir
        hostPath:
          path: /var/lib/kubelet/pods
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
`

func GetPVCYAML(pvcName, namespace, size, label string) string {

	pvcYAML := strings.Replace(persistentVolumeClaimYAMLTemplate, "{PVC_NAME}", pvcName, 1)
	pvcYAML = strings.Replace(pvcYAML, "{NAMESPACE}", namespace, 1)
	pvcYAML = strings.Replace(pvcYAML, "{SIZE}", size, 1)
	pvcYAML = strings.Replace(pvcYAML, "{LABEL}", label, -1)
	return pvcYAML
}

const persistentVolumeClaimYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  labels:
    app: {LABEL}
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
      app: {LABEL}
  storageClassName: ''
`

func GetNFSPVYAML(pvName, size, pvcName, pvcNamespace, nfsServer, nfsPath, label string) string {

	pvYAML := strings.Replace(persistentVolumeNFSYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAME}", pvcName, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAMESPACE}", pvcNamespace, 1)
	pvYAML = strings.Replace(pvYAML, "{SERVER}", nfsServer, 1)
	pvYAML = strings.Replace(pvYAML, "{PATH}", nfsPath, 1)
	pvYAML = strings.Replace(pvYAML, "{LABEL}", label, 1)
	return pvYAML
}

const persistentVolumeNFSYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: {LABEL}
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: {PVC_NAME}
    namespace: {PVC_NAMESPACE}
  nfs:
    server: {SERVER}
    path: {PATH}
`

func GetISCSIPVYAML(pvName, size, pvcName, pvcNamespace, targetPortal string, portals []string,
	iqn string, lun int32, label string) string {

	pvYAML := strings.Replace(persistentVolumeISCSIYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAME}", pvcName, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAMESPACE}", pvcNamespace, 1)
	pvYAML = strings.Replace(pvYAML, "{TARGET_PORTAL}", targetPortal, 1)
	pvYAML = strings.Replace(pvYAML, "{IQN}", iqn, 1)
	pvYAML = strings.Replace(pvYAML, "{LUN}", strconv.FormatInt(int64(lun), 10), 1)
	pvYAML = strings.Replace(pvYAML, "{LABEL}", label, 1)
	if 0 != len(portals) {
		pvYAML += "    portals:\n"
		for _, portal := range portals {
			pvYAML += "      - " + portal + "\n"
		}
	}
	return pvYAML
}

const persistentVolumeISCSIYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: {LABEL}
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: {PVC_NAME}
    namespace: {PVC_NAMESPACE}
  iscsi:
    targetPortal: {TARGET_PORTAL}
    iqn: {IQN}
    lun: {LUN}
    fsType: ext4
    readOnly: false
`

func GetCHAPISCSIPVYAML(pvName, size, pvcName, pvcNamespace, secretName, targetPortal string,
	portals []string, iqn string, lun int32, label string) string {

	pvYAML := strings.Replace(persistentVolumeCHAPISCSIYAMLTemplate, "{PV_NAME}", pvName, 1)
	pvYAML = strings.Replace(pvYAML, "{SIZE}", size, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAME}", pvcName, 1)
	pvYAML = strings.Replace(pvYAML, "{PVC_NAMESPACE}", pvcNamespace, 1)
	pvYAML = strings.Replace(pvYAML, "{TARGET_PORTAL}", targetPortal, 1)
	pvYAML = strings.Replace(pvYAML, "{IQN}", iqn, 1)
	pvYAML = strings.Replace(pvYAML, "{LUN}", strconv.FormatInt(int64(lun), 10), 1)
	pvYAML = strings.Replace(pvYAML, "{SECRET_NAME}", secretName, 1)
	pvYAML = strings.Replace(pvYAML, "{LABEL}", label, 1)
	if 0 != len(portals) {
		pvYAML += "    portals:\n"
		for _, portal := range portals {
			pvYAML += "      - " + portal + "\n"
		}
	}
	return pvYAML
}

const persistentVolumeCHAPISCSIYAMLTemplate = `---
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: {LABEL}
  name: {PV_NAME}
spec:
  capacity:
    storage: {SIZE}
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: {PVC_NAME}
    namespace: {PVC_NAMESPACE}
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

func GetInstallerServiceAccountYAML() string {

	return installerServiceAccountYAML
}

const installerServiceAccountYAML = `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: trident-installer
`

func GetInstallerClusterRoleYAML(flavor OrchestratorFlavor, version *utils.Version) string {
	switch flavor {
	case FlavorOpenShift:
		return installerClusterRoleOpenShiftYAML
	default:
		fallthrough
	case FlavorKubernetes:
		if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			return strings.Replace(installerClusterRoleKubernetesYAMLTemplate, "{API_VERSION}", "rbac.authorization.k8s.io/v1", 1)
		} else {
			return strings.Replace(installerClusterRoleKubernetesYAMLTemplate, "{API_VERSION}", "rbac.authorization.k8s.io/v1alpha1", 1)
		}
	}
}

const installerClusterRoleOpenShiftYAML = `---
kind: ClusterRole
apiVersion: "authorization.openshift.io/v1"
metadata:
  name: trident-installer
rules:
- apiGroups: [""]
  resources: ["namespaces", "pods", "pods/exec", "persistentvolumes", "persistentvolumeclaims", "persistentvolumeclaims/status", "secrets", "serviceaccounts", "services", "events", "nodes", "configmaps"]
  verbs: ["*"]
- apiGroups: ["extensions"]
  resources: ["deployments", "daemonsets"]
  verbs: ["*"]
- apiGroups: ["apps"]
  resources: ["statefulsets", "daemonsets", "deployments"]
  verbs: ["*"]
- apiGroups: ["authorization.openshift.io", "rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["*"]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses", "volumeattachments"]
  verbs: ["*"]
- apiGroups: ["security.openshift.io"]
  resources: ["securitycontextconstraints"]
  verbs: ["*"]
`

const installerClusterRoleKubernetesYAMLTemplate = `---
kind: ClusterRole
apiVersion: {API_VERSION}
metadata:
  name: trident-installer
rules:
- apiGroups: [""]
  resources: ["namespaces", "pods", "pods/exec", "persistentvolumes", "persistentvolumeclaims", "persistentvolumeclaims/status", "secrets", "serviceaccounts", "services", "events", "nodes", "configmaps"]
  verbs: ["*"]
- apiGroups: ["extensions"]
  resources: ["deployments", "daemonsets"]
  verbs: ["*"]
- apiGroups: ["apps"]
  resources: ["statefulsets", "daemonsets", "deployments"]
  verbs: ["*"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["*"]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses", "volumeattachments"]
  verbs: ["*"]
`

func GetInstallerClusterRoleBindingYAML(namespace string, flavor OrchestratorFlavor, version *utils.Version) string {

	var crbYAML string

	switch flavor {
	case FlavorOpenShift:
		crbYAML = installerClusterRoleBindingOpenShiftYAMLTemplate
	default:
		fallthrough
	case FlavorKubernetes:
		if version.AtLeast(utils.MustParseSemantic("v1.8.0")) {
			crbYAML = installerClusterRoleBindingKubernetesV1YAMLTemplate
		} else {
			crbYAML = installerClusterRoleBindingKubernetesV1Alpha1YAMLTemplate
		}
	}

	crbYAML = strings.Replace(crbYAML, "{NAMESPACE}", namespace, 1)
	return crbYAML
}

const installerClusterRoleBindingOpenShiftYAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: authorization.openshift.io/v1
metadata:
  name: trident-installer
subjects:
  - kind: ServiceAccount
    name: trident-installer
    namespace: {NAMESPACE}
roleRef:
  name: trident-installer
`

const installerClusterRoleBindingKubernetesV1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: trident-installer
subjects:
  - kind: ServiceAccount
    name: trident-installer
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: trident-installer
  apiGroup: rbac.authorization.k8s.io
`

const installerClusterRoleBindingKubernetesV1Alpha1YAMLTemplate = `---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1alpha1
metadata:
  name: trident-installer
subjects:
  - kind: ServiceAccount
    name: trident-installer
    namespace: {NAMESPACE}
roleRef:
  kind: ClusterRole
  name: trident-installer
  apiGroup: rbac.authorization.k8s.io
`

func GetInstallerPodYAML(label, tridentImage string, commandArgs []string) string {

	command := `["` + strings.Join(commandArgs, `", "`) + `"]`

	jobYAML := strings.Replace(installerPodTemplate, "{LABEL}", label, 1)
	jobYAML = strings.Replace(jobYAML, "{TRIDENT_IMAGE}", tridentImage, 1)
	jobYAML = strings.Replace(jobYAML, "{COMMAND}", command, 1)
	return jobYAML
}

const installerPodTemplate = `---
apiVersion: v1
kind: Pod
metadata:
  name: trident-installer
  labels:
    app: {LABEL}
spec:
  serviceAccount: trident-installer
  containers:
  - name: trident-installer
    image: {TRIDENT_IMAGE}
    workingDir: /
    command: {COMMAND}
    volumeMounts:
    - name: setup-dir
      mountPath: /setup
  restartPolicy: Never
  volumes:
  - name: setup-dir
    configMap:
      name: trident-installer
`

func GetUninstallerPodYAML(label, tridentImage string, commandArgs []string) string {

	command := `["` + strings.Join(commandArgs, `", "`) + `"]`

	jobYAML := strings.Replace(uninstallerPodTemplate, "{LABEL}", label, 1)
	jobYAML = strings.Replace(jobYAML, "{TRIDENT_IMAGE}", tridentImage, 1)
	jobYAML = strings.Replace(jobYAML, "{COMMAND}", command, 1)
	return jobYAML
}

const uninstallerPodTemplate = `---
apiVersion: v1
kind: Pod
metadata:
  name: trident-installer
  labels:
    app: {LABEL}
spec:
  serviceAccount: trident-installer
  containers:
  - name: trident-installer
    image: {TRIDENT_IMAGE}
    workingDir: /
    command: {COMMAND}
  restartPolicy: Never
`

func GetOpenShiftSCCQueryYAML(scc string) string {
	return strings.Replace(openShiftSCCQueryYAMLTemplate, "{SCC}", scc, 1)
}

const openShiftSCCQueryYAMLTemplate = `
---
kind: SecurityContextConstraints
apiVersion: security.openshift.io/v1
metadata:
  name: {SCC}
`
