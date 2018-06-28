// Copyright 2018 NetApp, Inc. All Rights Reserved.

package ucpclient

import (
	"strings"
)

// UCPRoleCreateJSONTemplateFullPermissions is the YAML required to create a new UCP Role with full control
const UCPRoleCreateJSONTemplateFullPermissions = `
{
    "name": "{ROLE_NAME}",
	"system_role": false,
	"operations": {
	  "Config": {
		"Config Create": [],
		"Config Delete": [],
		"Config Update": [],
		"Config Use": [],
		"Config View": []
	  },
	  "Container": {
		"Container Attach": [],
		"Container Changes": [],
		"Container Connect": [],
		"Container Create": [
		  "IPC Mode",
		  "PID Mode",
		  "Host Networking Mode",
		  "Bridge Networking Mode",
		  "Privileged",
		  "Host Bind Mounts",
		  "Additional Kernel Capabilities",
		  "User Namespace Mode"
		],
		"Container Disconnect": [],
		"Container Exec": [],
		"Container Export": [],
		"Container Filesystem Read": [],
		"Container Filesystem Write": [],
		"Container Kill": [],
		"Container Logs": [],
		"Container Pause": [],
		"Container Remove": [],
		"Container Rename": [],
		"Container Resize": [],
		"Container Restart": [],
		"Container Start": [],
		"Container Stats": [],
		"Container Stop": [],
		"Container Top": [],
		"Container Unpause": [],
		"Container View": [],
		"Container Wait": []
	  },
	  "Image": {
		"Image Build": [],
		"Image Commit": [],
		"Image Create": [],
		"Image Export": [],
		"Image History": [],
		"Image Load": [],
		"Image Prune": [],
		"Image Push": [],
		"Image Remove": [
		  "Force Remove"
		],
		"Image Search": [],
		"Image Tag": [],
		"Image View": []
	  },
	  "Kubernetes Certificate Signing Request": {
		"CertificateSigningRequest Create": [],
		"CertificateSigningRequest Delete": [],
		"CertificateSigningRequest Delete Multiple": [],
		"CertificateSigningRequest Get": [],
		"CertificateSigningRequest List": [],
		"CertificateSigningRequest Patch": [],
		"CertificateSigningRequest Update": [],
		"CertificateSigningRequest Watch": []
	  },
	  "Kubernetes Cluster Role": {
		"ClusterRole Create": [],
		"ClusterRole Delete": [],
		"ClusterRole Delete Multiple": [],
		"ClusterRole Get": [],
		"ClusterRole List": [],
		"ClusterRole Patch": [],
		"ClusterRole Update": [],
		"ClusterRole Watch": []
	  },
	  "Kubernetes Cluster Role Binding": {
		"ClusterRoleBinding Create": [],
		"ClusterRoleBinding Delete": [],
		"ClusterRoleBinding Delete Multiple": [],
		"ClusterRoleBinding Get": [],
		"ClusterRoleBinding List": [],
		"ClusterRoleBinding Patch": [],
		"ClusterRoleBinding Update": [],
		"ClusterRoleBinding Watch": []
	  },
	  "Kubernetes Component Status": {
		"ComponentStatus Create": [],
		"ComponentStatus Delete": [],
		"ComponentStatus Delete Multiple": [],
		"ComponentStatus Get": [],
		"ComponentStatus List": [],
		"ComponentStatus Patch": [],
		"ComponentStatus Update": [],
		"ComponentStatus Watch": []
	  },
	  "Kubernetes Config Map": {
		"ConfigMap Create": [],
		"ConfigMap Delete": [],
		"ConfigMap Delete Multiple": [],
		"ConfigMap Get": [],
		"ConfigMap List": [],
		"ConfigMap Patch": [],
		"ConfigMap Update": [],
		"ConfigMap Watch": []
	  },
	  "Kubernetes Controller Revision": {
		"ControllerRevision Create": [],
		"ControllerRevision Delete": [],
		"ControllerRevision Delete Multiple": [],
		"ControllerRevision Get": [],
		"ControllerRevision List": [],
		"ControllerRevision Patch": [],
		"ControllerRevision Update": [],
		"ControllerRevision Watch": []
	  },
	  "Kubernetes Cron Job": {
		"CronJob Create": [],
		"CronJob Delete": [],
		"CronJob Delete Multiple": [],
		"CronJob Get": [],
		"CronJob List": [],
		"CronJob Patch": [],
		"CronJob Update": [],
		"CronJob Watch": []
	  },
	  "Kubernetes Custom Resource Definition": {
		"CustomResourceDefinition Create": [],
		"CustomResourceDefinition Delete": [],
		"CustomResourceDefinition Delete Multiple": [],
		"CustomResourceDefinition Get": [],
		"CustomResourceDefinition List": [],
		"CustomResourceDefinition Patch": [],
		"CustomResourceDefinition Update": [],
		"CustomResourceDefinition Watch": []
	  },
	  "Kubernetes Daemonset": {
		"Daemonset Create": [],
		"Daemonset Delete": [],
		"Daemonset Delete Multiple": [],
		"Daemonset Get": [],
		"Daemonset List": [],
		"Daemonset Patch": [],
		"Daemonset Update": [],
		"Daemonset Watch": []
	  },
	  "Kubernetes Deployment": {
		"Deployment Create": [],
		"Deployment Delete": [],
		"Deployment Delete Multiple": [],
		"Deployment Get": [],
		"Deployment List": [],
		"Deployment Patch": [],
		"Deployment Update": [],
		"Deployment Watch": []
	  },
	  "Kubernetes Endpoint": {
		"Endpoint Create": [],
		"Endpoint Delete": [],
		"Endpoint Delete Multiple": [],
		"Endpoint Get": [],
		"Endpoint List": [],
		"Endpoint Patch": [],
		"Endpoint Update": [],
		"Endpoint Watch": []
	  },
	  "Kubernetes Event": {
		"Event Create": [],
		"Event Delete": [],
		"Event Delete Multiple": [],
		"Event Get": [],
		"Event List": [],
		"Event Patch": [],
		"Event Update": [],
		"Event Watch": []
	  },
	  "Kubernetes External Admission Hook Configuration": {
		"ExternalAdmissionHookConfiguration Create": [],
		"ExternalAdmissionHookConfiguration Delete": [],
		"ExternalAdmissionHookConfiguration Delete Multiple": [],
		"ExternalAdmissionHookConfiguration Get": [],
		"ExternalAdmissionHookConfiguration List": [],
		"ExternalAdmissionHookConfiguration Patch": [],
		"ExternalAdmissionHookConfiguration Update": [],
		"ExternalAdmissionHookConfiguration Watch": []
	  },
	  "Kubernetes Horizontal Pod Autoscaler": {
		"HorizontalPodAutoscaler Create": [],
		"HorizontalPodAutoscaler Delete": [],
		"HorizontalPodAutoscaler Delete Multiple": [],
		"HorizontalPodAutoscaler Get": [],
		"HorizontalPodAutoscaler List": [],
		"HorizontalPodAutoscaler Patch": [],
		"HorizontalPodAutoscaler Update": [],
		"HorizontalPodAutoscaler Watch": []
	  },
	  "Kubernetes Ingress": {
		"Ingress Create": [],
		"Ingress Delete": [],
		"Ingress Delete Multiple": [],
		"Ingress Get": [],
		"Ingress List": [],
		"Ingress Patch": [],
		"Ingress Update": [],
		"Ingress Watch": []
	  },
	  "Kubernetes Initializer Configuration": {
		"InitializerConfiguration Create": [],
		"InitializerConfiguration Delete": [],
		"InitializerConfiguration Delete Multiple": [],
		"InitializerConfiguration Get": [],
		"InitializerConfiguration List": [],
		"InitializerConfiguration Patch": [],
		"InitializerConfiguration Update": [],
		"InitializerConfiguration Watch": []
	  },
	  "Kubernetes Job": {
		"Job Create": [],
		"Job Delete": [],
		"Job Delete Multiple": [],
		"Job Get": [],
		"Job List": [],
		"Job Patch": [],
		"Job Update": [],
		"Job Watch": []
	  },
	  "Kubernetes Limit Range": {
		"LimitRange Create": [],
		"LimitRange Delete": [],
		"LimitRange Delete Multiple": [],
		"LimitRange Get": [],
		"LimitRange List": [],
		"LimitRange Patch": [],
		"LimitRange Update": [],
		"LimitRange Watch": []
	  },
	  "Kubernetes Namespace": {
		"Namespace Create": [],
		"Namespace Delete": [],
		"Namespace Delete Multiple": [],
		"Namespace Get": [],
		"Namespace List": [],
		"Namespace Patch": [],
		"Namespace Update": [],
		"Namespace Watch": []
	  },
	  "Kubernetes Network Policy": {
		"NetworkPolicy Create": [],
		"NetworkPolicy Delete": [],
		"NetworkPolicy Delete Multiple": [],
		"NetworkPolicy Get": [],
		"NetworkPolicy List": [],
		"NetworkPolicy Patch": [],
		"NetworkPolicy Update": [],
		"NetworkPolicy Watch": []
	  },
	  "Kubernetes Node": {
		"Node Create": [],
		"Node Delete": [],
		"Node Delete Multiple": [],
		"Node Get": [],
		"Node List": [],
		"Node Patch": [],
		"Node Update": [],
		"Node Watch": []
	  },
	  "Kubernetes Persistent Volume": {
		"PersistentVolume Create": [],
		"PersistentVolume Delete": [],
		"PersistentVolume Delete Multiple": [],
		"PersistentVolume Get": [],
		"PersistentVolume List": [],
		"PersistentVolume Patch": [],
		"PersistentVolume Update": [],
		"PersistentVolume Watch": []
	  },
	  "Kubernetes Persistent Volume Claim": {
		"PersistentVolumeClaim Create": [],
		"PersistentVolumeClaim Delete": [],
		"PersistentVolumeClaim Delete Multiple": [],
		"PersistentVolumeClaim Get": [],
		"PersistentVolumeClaim List": [],
		"PersistentVolumeClaim Patch": [],
		"PersistentVolumeClaim Update": [],
		"PersistentVolumeClaim Watch": []
	  },
	  "Kubernetes Pod": {
		"Pod Create": [],
		"Pod Delete": [],
		"Pod Delete Multiple": [],
		"Pod Get": [],
		"Pod List": [],
		"Pod Patch": [],
		"Pod Update": [],
		"Pod Watch": []
	  },
	  "Kubernetes Pod Disruption Budget": {
		"PodDisruptionBudget Create": [],
		"PodDisruptionBudget Delete": [],
		"PodDisruptionBudget Delete Multiple": [],
		"PodDisruptionBudget Get": [],
		"PodDisruptionBudget List": [],
		"PodDisruptionBudget Patch": [],
		"PodDisruptionBudget Update": [],
		"PodDisruptionBudget Watch": []
	  },
	  "Kubernetes Pod Preset": {
		"PodPreset Create": [],
		"PodPreset Delete": [],
		"PodPreset Delete Multiple": [],
		"PodPreset Get": [],
		"PodPreset List": [],
		"PodPreset Patch": [],
		"PodPreset Update": [],
		"PodPreset Watch": []
	  },
	  "Kubernetes Pod Security Policy": {
		"PodSecurityPolicy Create": [],
		"PodSecurityPolicy Delete": [],
		"PodSecurityPolicy Delete Multiple": [],
		"PodSecurityPolicy Get": [],
		"PodSecurityPolicy List": [],
		"PodSecurityPolicy Patch": [],
		"PodSecurityPolicy Update": [],
		"PodSecurityPolicy Watch": []
	  },
	  "Kubernetes Pod Template": {
		"PodTemplate Create": [],
		"PodTemplate Delete": [],
		"PodTemplate Delete Multiple": [],
		"PodTemplate Get": [],
		"PodTemplate List": [],
		"PodTemplate Patch": [],
		"PodTemplate Update": [],
		"PodTemplate Watch": []
	  },
	  "Kubernetes Replica Set": {
		"ReplicaSet Create": [],
		"ReplicaSet Delete": [],
		"ReplicaSet Delete Multiple": [],
		"ReplicaSet Get": [],
		"ReplicaSet List": [],
		"ReplicaSet Patch": [],
		"ReplicaSet Update": [],
		"ReplicaSet Watch": []
	  },
	  "Kubernetes Replication Controller": {
		"ReplicationController Create": [],
		"ReplicationController Delete": [],
		"ReplicationController Delete Multiple": [],
		"ReplicationController Get": [],
		"ReplicationController List": [],
		"ReplicationController Patch": [],
		"ReplicationController Update": [],
		"ReplicationController Watch": []
	  },
	  "Kubernetes Resource Quota": {
		"ResourceQuota Create": [],
		"ResourceQuota Delete": [],
		"ResourceQuota Delete Multiple": [],
		"ResourceQuota Get": [],
		"ResourceQuota List": [],
		"ResourceQuota Patch": [],
		"ResourceQuota Update": [],
		"ResourceQuota Watch": []
	  },
	  "Kubernetes Role": {
		"Role Create": [],
		"Role Delete": [],
		"Role Delete Multiple": [],
		"Role Get": [],
		"Role List": [],
		"Role Patch": [],
		"Role Update": [],
		"Role Watch": []
	  },
	  "Kubernetes Role Binding": {
		"RoleBinding Create": [],
		"RoleBinding Delete": [],
		"RoleBinding Delete Multiple": [],
		"RoleBinding Get": [],
		"RoleBinding List": [],
		"RoleBinding Patch": [],
		"RoleBinding Update": [],
		"RoleBinding Watch": []
	  },
	  "Kubernetes Secret": {
		"Secret Create": [],
		"Secret Delete": [],
		"Secret Delete Multiple": [],
		"Secret Get": [],
		"Secret List": [],
		"Secret Patch": [],
		"Secret Update": [],
		"Secret Watch": []
	  },
	  "Kubernetes Service": {
		"Service Create": [],
		"Service Delete": [],
		"Service Delete Multiple": [],
		"Service Get": [],
		"Service List": [],
		"Service Patch": [],
		"Service Update": [],
		"Service Watch": []
	  },
	  "Kubernetes Service Account": {
		"ServiceAccount Create": [],
		"ServiceAccount Delete": [],
		"ServiceAccount Delete Multiple": [],
		"ServiceAccount Get": [],
		"ServiceAccount List": [],
		"ServiceAccount Patch": [],
		"ServiceAccount Update": [],
		"ServiceAccount Watch": []
	  },
	  "Kubernetes Stack": {
		"Stack Create": [],
		"Stack Delete": [],
		"Stack Delete Multiple": [],
		"Stack Get": [],
		"Stack List": [],
		"Stack Patch": [],
		"Stack Update": [],
		"Stack Watch": []
	  },
	  "Kubernetes Stateful Set": {
		"StatefulSet Create": [],
		"StatefulSet Delete": [],
		"StatefulSet Delete Multiple": [],
		"StatefulSet Get": [],
		"StatefulSet List": [],
		"StatefulSet Patch": [],
		"StatefulSet Update": [],
		"StatefulSet Watch": []
	  },
	  "Kubernetes Storage Class": {
		"StorageClass Create": [],
		"StorageClass Delete": [],
		"StorageClass Delete Multiple": [],
		"StorageClass Get": [],
		"StorageClass List": [],
		"StorageClass Patch": [],
		"StorageClass Update": [],
		"StorageClass Watch": []
	  },
	  "Kubernetes Third Party Resource": {
		"ThirdPartyResource Create": [],
		"ThirdPartyResource Delete": [],
		"ThirdPartyResource Delete Multiple": [],
		"ThirdPartyResource Get": [],
		"ThirdPartyResource List": [],
		"ThirdPartyResource Patch": [],
		"ThirdPartyResource Update": [],
		"ThirdPartyResource Watch": []
	  },
	  "Kubernetes User": {
		"User Create": [],
		"User Delete": [],
		"User Delete Multiple": [],
		"User Get": [],
		"User Impersonate": [],
		"User List": [],
		"User Patch": [],
		"User Update": [],
		"User Watch": []
	  },
	  "Network": {
		"Network Connect": [],
		"Network Create": [
		  "Attachable"
		],
		"Network Disconnect": [],
		"Network Remove": [],
		"Network View": []
	  },
	  "Node": {
		"Node Schedule": [],
		"Node Update": [],
		"Node View": []
	  },
	  "Secret": {
		"Secret Create": [],
		"Secret Delete": [],
		"Secret Update": [],
		"Secret Use": [],
		"Secret View": []
	  },
	  "Service": {
		"Service Create": [
		  "Credential Spec",
		  "Host Bind Mounts",
		  "Host Networking Mode",
		  "Bridge Networking Mode"
		],
		"Service Delete": [],
		"Service Logs": [],
		"Service Update": [
		  "Credential Spec",
		  "Host Bind Mounts",
		  "Host Networking Mode",
		  "Bridge Networking Mode"
		],
		"Service View": [],
		"Service View Tasks": []
	  },
	  "Volume": {
		"Volume Create/Attach": [
		  "Local Driver Options"
		],
		"Volume Remove": [],
		"Volume View": []
	  }
	}
}
`

// UCPRoleCreateJSONTemplateMinimalPermissions is the YAML required to create a new UCP Role for Trident's usecase
const UCPRoleCreateJSONTemplateMinimalPermissions = `
{
    "name": "{ROLE_NAME}",
	"system_role": false,
	"operations": {
	  "Kubernetes Event": {
		"Event Create": [],
		"Event Get": [],
		"Event List": [],
		"Event Patch": [],
		"Event Update": [],
		"Event Watch": []
	  },
	  "Kubernetes Namespace": {
		"Namespace Create": [],
		"Namespace Get": [],
		"Namespace List": [],
		"Namespace Watch": []
	  },
	  "Kubernetes Persistent Volume": {
		"PersistentVolume Create": [],
		"PersistentVolume Delete": [],
		"PersistentVolume Delete Multiple": [],
		"PersistentVolume Get": [],
		"PersistentVolume List": [],
		"PersistentVolume Patch": [],
		"PersistentVolume Update": [],
		"PersistentVolume Watch": []
	  },
	  "Kubernetes Persistent Volume Claim": {
		"PersistentVolumeClaim Create": [],
		"PersistentVolumeClaim Delete": [],
		"PersistentVolumeClaim Delete Multiple": [],
		"PersistentVolumeClaim Get": [],
		"PersistentVolumeClaim List": [],
		"PersistentVolumeClaim Patch": [],
		"PersistentVolumeClaim Update": [],
		"PersistentVolumeClaim Watch": []
	  },
	  "Kubernetes Secret": {
		"Secret Create": [],
		"Secret Delete": [],
		"Secret Delete Multiple": [],
		"Secret Get": [],
		"Secret List": [],
		"Secret Patch": [],
		"Secret Update": [],
		"Secret Watch": []
	  },
	  "Kubernetes Storage Class": {
		"StorageClass Create": [],
		"StorageClass Delete": [],
		"StorageClass Delete Multiple": [],
		"StorageClass Get": [],
		"StorageClass List": [],
		"StorageClass Patch": [],
		"StorageClass Update": [],
		"StorageClass Watch": []
	  },
	  "Secret": {
		"Secret Create": [],
		"Secret Delete": [],
		"Secret Update": [],
		"Secret Use": [],
		"Secret View": []
	  },
	  "Volume": {
		"Volume Create/Attach": [
		  "Local Driver Options"
		],
		"Volume Remove": [],
		"Volume View": []
	  }
	}
}
`

// GetUCPRoleCreateJSON returns the YAML neccessary to create a new UCP Role for Trident's usecase
func GetUCPRoleCreateJSON(roleName string) string {

	if roleName == "" {
		roleName = "trident"
	}
	createYAML := strings.Replace(UCPRoleCreateJSONTemplateMinimalPermissions, "{ROLE_NAME}", roleName, 1)
	return createYAML
}
