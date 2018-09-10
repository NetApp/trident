// Copyright 2018 NetApp, Inc. All Rights Reserved.

package ucpclient

import (
	"strings"
)

// LegacyUCPRoleCreateJSONTemplateFullPermissions is the YAML required to create a new UCP Role with full control
const LegacyUCPRoleCreateJSONTemplateFullPermissions = `
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

// LegacyUCPRoleCreateJSONTemplateMinimalPermissions is the YAML required to create a new UCP Role for Trident's usecase
const LegacyUCPRoleCreateJSONTemplateMinimalPermissions = `
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
        "Kubernetes Certificate Signing Request Create": [],
        "Kubernetes Certificate Signing Request Delete": [],
        "Kubernetes Certificate Signing Request Delete Multiple": [],
        "Kubernetes Certificate Signing Request Get": [],
        "Kubernetes Certificate Signing Request List": [],
        "Kubernetes Certificate Signing Request Patch": [],
        "Kubernetes Certificate Signing Request Update": [],
        "Kubernetes Certificate Signing Request Watch": []
      },
      "Kubernetes Certificate Signing Request/approval": {
        "Kubernetes Certificate Signing Request/approval Create": [],
        "Kubernetes Certificate Signing Request/approval Delete": [],
        "Kubernetes Certificate Signing Request/approval Delete Multiple": [],
        "Kubernetes Certificate Signing Request/approval Get": [],
        "Kubernetes Certificate Signing Request/approval List": [],
        "Kubernetes Certificate Signing Request/approval Patch": [],
        "Kubernetes Certificate Signing Request/approval Update": [],
        "Kubernetes Certificate Signing Request/approval Watch": []
      },
      "Kubernetes Certificate Signing Request/status": {
        "Kubernetes Certificate Signing Request/status Create": [],
        "Kubernetes Certificate Signing Request/status Delete": [],
        "Kubernetes Certificate Signing Request/status Delete Multiple": [],
        "Kubernetes Certificate Signing Request/status Get": [],
        "Kubernetes Certificate Signing Request/status List": [],
        "Kubernetes Certificate Signing Request/status Patch": [],
        "Kubernetes Certificate Signing Request/status Update": [],
        "Kubernetes Certificate Signing Request/status Watch": []
      },
      "Kubernetes Cluster Role": {
        "Kubernetes Cluster Role Create": [],
        "Kubernetes Cluster Role Delete": [],
        "Kubernetes Cluster Role Delete Multiple": [],
        "Kubernetes Cluster Role Get": [],
        "Kubernetes Cluster Role List": [],
        "Kubernetes Cluster Role Patch": [],
        "Kubernetes Cluster Role Update": [],
        "Kubernetes Cluster Role Watch": []
      },
      "Kubernetes Cluster Role Binding": {
        "Kubernetes Cluster Role Binding Create": [],
        "Kubernetes Cluster Role Binding Delete": [],
        "Kubernetes Cluster Role Binding Delete Multiple": [],
        "Kubernetes Cluster Role Binding Get": [],
        "Kubernetes Cluster Role Binding List": [],
        "Kubernetes Cluster Role Binding Patch": [],
        "Kubernetes Cluster Role Binding Update": [],
        "Kubernetes Cluster Role Binding Watch": []
      },
      "Kubernetes Cluster Role Binding/status": {
        "Kubernetes Cluster Role Binding/status Create": [],
        "Kubernetes Cluster Role Binding/status Delete": [],
        "Kubernetes Cluster Role Binding/status Delete Multiple": [],
        "Kubernetes Cluster Role Binding/status Get": [],
        "Kubernetes Cluster Role Binding/status List": [],
        "Kubernetes Cluster Role Binding/status Patch": [],
        "Kubernetes Cluster Role Binding/status Update": [],
        "Kubernetes Cluster Role Binding/status Watch": []
      },
      "Kubernetes Cluster Role/status": {
        "Kubernetes Cluster Role/status Create": [],
        "Kubernetes Cluster Role/status Delete": [],
        "Kubernetes Cluster Role/status Delete Multiple": [],
        "Kubernetes Cluster Role/status Get": [],
        "Kubernetes Cluster Role/status List": [],
        "Kubernetes Cluster Role/status Patch": [],
        "Kubernetes Cluster Role/status Update": [],
        "Kubernetes Cluster Role/status Watch": []
      },
      "Kubernetes Component Status": {
        "Kubernetes Component Status Create": [],
        "Kubernetes Component Status Delete": [],
        "Kubernetes Component Status Delete Multiple": [],
        "Kubernetes Component Status Get": [],
        "Kubernetes Component Status List": [],
        "Kubernetes Component Status Patch": [],
        "Kubernetes Component Status Update": [],
        "Kubernetes Component Status Watch": []
      },
      "Kubernetes Component Status/status": {
        "Kubernetes Component Status/status Create": [],
        "Kubernetes Component Status/status Delete": [],
        "Kubernetes Component Status/status Delete Multiple": [],
        "Kubernetes Component Status/status Get": [],
        "Kubernetes Component Status/status List": [],
        "Kubernetes Component Status/status Patch": [],
        "Kubernetes Component Status/status Update": [],
        "Kubernetes Component Status/status Watch": []
      },
      "Kubernetes Config Map": {
        "Kubernetes Config Map Create": [],
        "Kubernetes Config Map Delete": [],
        "Kubernetes Config Map Delete Multiple": [],
        "Kubernetes Config Map Get": [],
        "Kubernetes Config Map List": [],
        "Kubernetes Config Map Patch": [],
        "Kubernetes Config Map Update": [],
        "Kubernetes Config Map Watch": []
      },
      "Kubernetes Config Map/status": {
        "Kubernetes Config Map/status Create": [],
        "Kubernetes Config Map/status Delete": [],
        "Kubernetes Config Map/status Delete Multiple": [],
        "Kubernetes Config Map/status Get": [],
        "Kubernetes Config Map/status List": [],
        "Kubernetes Config Map/status Patch": [],
        "Kubernetes Config Map/status Update": [],
        "Kubernetes Config Map/status Watch": []
      },
      "Kubernetes Controller Revision": {
        "Kubernetes Controller Revision Create": [],
        "Kubernetes Controller Revision Delete": [],
        "Kubernetes Controller Revision Delete Multiple": [],
        "Kubernetes Controller Revision Get": [],
        "Kubernetes Controller Revision List": [],
        "Kubernetes Controller Revision Patch": [],
        "Kubernetes Controller Revision Update": [],
        "Kubernetes Controller Revision Watch": []
      },
      "Kubernetes Controller Revision/status": {
        "Kubernetes Controller Revision/status Create": [],
        "Kubernetes Controller Revision/status Delete": [],
        "Kubernetes Controller Revision/status Delete Multiple": [],
        "Kubernetes Controller Revision/status Get": [],
        "Kubernetes Controller Revision/status List": [],
        "Kubernetes Controller Revision/status Patch": [],
        "Kubernetes Controller Revision/status Update": [],
        "Kubernetes Controller Revision/status Watch": []
      },
      "Kubernetes Cron Job": {
        "Kubernetes Cron Job Create": [],
        "Kubernetes Cron Job Delete": [],
        "Kubernetes Cron Job Delete Multiple": [],
        "Kubernetes Cron Job Get": [],
        "Kubernetes Cron Job List": [],
        "Kubernetes Cron Job Patch": [],
        "Kubernetes Cron Job Update": [],
        "Kubernetes Cron Job Watch": []
      },
      "Kubernetes Cron Job/status": {
        "Kubernetes Cron Job/status Create": [],
        "Kubernetes Cron Job/status Delete": [],
        "Kubernetes Cron Job/status Delete Multiple": [],
        "Kubernetes Cron Job/status Get": [],
        "Kubernetes Cron Job/status List": [],
        "Kubernetes Cron Job/status Patch": [],
        "Kubernetes Cron Job/status Update": [],
        "Kubernetes Cron Job/status Watch": []
      },
      "Kubernetes Custom Resource Definition": {
        "Kubernetes Custom Resource Definition Create": [],
        "Kubernetes Custom Resource Definition Delete": [],
        "Kubernetes Custom Resource Definition Delete Multiple": [],
        "Kubernetes Custom Resource Definition Get": [],
        "Kubernetes Custom Resource Definition List": [],
        "Kubernetes Custom Resource Definition Patch": [],
        "Kubernetes Custom Resource Definition Update": [],
        "Kubernetes Custom Resource Definition Watch": []
      },
      "Kubernetes Custom Resource Definition/status": {
        "Kubernetes Custom Resource Definition/status Create": [],
        "Kubernetes Custom Resource Definition/status Delete": [],
        "Kubernetes Custom Resource Definition/status Delete Multiple": [],
        "Kubernetes Custom Resource Definition/status Get": [],
        "Kubernetes Custom Resource Definition/status List": [],
        "Kubernetes Custom Resource Definition/status Patch": [],
        "Kubernetes Custom Resource Definition/status Update": [],
        "Kubernetes Custom Resource Definition/status Watch": []
      },
      "Kubernetes Daemonset": {
        "Kubernetes Daemonset Create": [],
        "Kubernetes Daemonset Delete": [],
        "Kubernetes Daemonset Delete Multiple": [],
        "Kubernetes Daemonset Get": [],
        "Kubernetes Daemonset List": [],
        "Kubernetes Daemonset Patch": [],
        "Kubernetes Daemonset Update": [],
        "Kubernetes Daemonset Watch": []
      },
      "Kubernetes Daemonset/status": {
        "Kubernetes Daemonset/status Create": [],
        "Kubernetes Daemonset/status Delete": [],
        "Kubernetes Daemonset/status Delete Multiple": [],
        "Kubernetes Daemonset/status Get": [],
        "Kubernetes Daemonset/status List": [],
        "Kubernetes Daemonset/status Patch": [],
        "Kubernetes Daemonset/status Update": [],
        "Kubernetes Daemonset/status Watch": []
      },
      "Kubernetes Deployment": {
        "Kubernetes Deployment Create": [],
        "Kubernetes Deployment Delete": [],
        "Kubernetes Deployment Delete Multiple": [],
        "Kubernetes Deployment Get": [],
        "Kubernetes Deployment List": [],
        "Kubernetes Deployment Patch": [],
        "Kubernetes Deployment Update": [],
        "Kubernetes Deployment Watch": []
      },
      "Kubernetes Deployment/rollback": {
        "Kubernetes Deployment/rollback Create": [],
        "Kubernetes Deployment/rollback Delete": [],
        "Kubernetes Deployment/rollback Delete Multiple": [],
        "Kubernetes Deployment/rollback Get": [],
        "Kubernetes Deployment/rollback List": [],
        "Kubernetes Deployment/rollback Patch": [],
        "Kubernetes Deployment/rollback Update": [],
        "Kubernetes Deployment/rollback Watch": []
      },
      "Kubernetes Deployment/scale": {
        "Kubernetes Deployment/scale Create": [],
        "Kubernetes Deployment/scale Delete": [],
        "Kubernetes Deployment/scale Delete Multiple": [],
        "Kubernetes Deployment/scale Get": [],
        "Kubernetes Deployment/scale List": [],
        "Kubernetes Deployment/scale Patch": [],
        "Kubernetes Deployment/scale Update": [],
        "Kubernetes Deployment/scale Watch": []
      },
      "Kubernetes Deployment/status": {
        "Kubernetes Deployment/status Create": [],
        "Kubernetes Deployment/status Delete": [],
        "Kubernetes Deployment/status Delete Multiple": [],
        "Kubernetes Deployment/status Get": [],
        "Kubernetes Deployment/status List": [],
        "Kubernetes Deployment/status Patch": [],
        "Kubernetes Deployment/status Update": [],
        "Kubernetes Deployment/status Watch": []
      },
      "Kubernetes Endpoint": {
        "Kubernetes Endpoint Create": [],
        "Kubernetes Endpoint Delete": [],
        "Kubernetes Endpoint Delete Multiple": [],
        "Kubernetes Endpoint Get": [],
        "Kubernetes Endpoint List": [],
        "Kubernetes Endpoint Patch": [],
        "Kubernetes Endpoint Update": [],
        "Kubernetes Endpoint Watch": []
      },
      "Kubernetes Endpoint/status": {
        "Kubernetes Endpoint/status Create": [],
        "Kubernetes Endpoint/status Delete": [],
        "Kubernetes Endpoint/status Delete Multiple": [],
        "Kubernetes Endpoint/status Get": [],
        "Kubernetes Endpoint/status List": [],
        "Kubernetes Endpoint/status Patch": [],
        "Kubernetes Endpoint/status Update": [],
        "Kubernetes Endpoint/status Watch": []
      },
      "Kubernetes Event": {
        "Kubernetes Event Create": [],
        "Kubernetes Event Delete": [],
        "Kubernetes Event Delete Multiple": [],
        "Kubernetes Event Get": [],
        "Kubernetes Event List": [],
        "Kubernetes Event Patch": [],
        "Kubernetes Event Update": [],
        "Kubernetes Event Watch": []
      },
      "Kubernetes Event/status": {
        "Kubernetes Event/status Create": [],
        "Kubernetes Event/status Delete": [],
        "Kubernetes Event/status Delete Multiple": [],
        "Kubernetes Event/status Get": [],
        "Kubernetes Event/status List": [],
        "Kubernetes Event/status Patch": [],
        "Kubernetes Event/status Update": [],
        "Kubernetes Event/status Watch": []
      },
      "Kubernetes External Admission Hook Configuration": {
        "Kubernetes External Admission Hook Configuration Create": [],
        "Kubernetes External Admission Hook Configuration Delete": [],
        "Kubernetes External Admission Hook Configuration Delete Multiple": [],
        "Kubernetes External Admission Hook Configuration Get": [],
        "Kubernetes External Admission Hook Configuration List": [],
        "Kubernetes External Admission Hook Configuration Patch": [],
        "Kubernetes External Admission Hook Configuration Update": [],
        "Kubernetes External Admission Hook Configuration Watch": []
      },
      "Kubernetes External Admission Hook Configuration/status": {
        "Kubernetes External Admission Hook Configuration/status Create": [],
        "Kubernetes External Admission Hook Configuration/status Delete": [],
        "Kubernetes External Admission Hook Configuration/status Delete Multiple": [],
        "Kubernetes External Admission Hook Configuration/status Get": [],
        "Kubernetes External Admission Hook Configuration/status List": [],
        "Kubernetes External Admission Hook Configuration/status Patch": [],
        "Kubernetes External Admission Hook Configuration/status Update": [],
        "Kubernetes External Admission Hook Configuration/status Watch": []
      },
      "Kubernetes Horizontal Pod Autoscaler": {
        "Kubernetes Horizontal Pod Autoscaler Create": [],
        "Kubernetes Horizontal Pod Autoscaler Delete": [],
        "Kubernetes Horizontal Pod Autoscaler Delete Multiple": [],
        "Kubernetes Horizontal Pod Autoscaler Get": [],
        "Kubernetes Horizontal Pod Autoscaler List": [],
        "Kubernetes Horizontal Pod Autoscaler Patch": [],
        "Kubernetes Horizontal Pod Autoscaler Update": [],
        "Kubernetes Horizontal Pod Autoscaler Watch": []
      },
      "Kubernetes Horizontal Pod Autoscaler/status": {
        "Kubernetes Horizontal Pod Autoscaler/status Create": [],
        "Kubernetes Horizontal Pod Autoscaler/status Delete": [],
        "Kubernetes Horizontal Pod Autoscaler/status Delete Multiple": [],
        "Kubernetes Horizontal Pod Autoscaler/status Get": [],
        "Kubernetes Horizontal Pod Autoscaler/status List": [],
        "Kubernetes Horizontal Pod Autoscaler/status Patch": [],
        "Kubernetes Horizontal Pod Autoscaler/status Update": [],
        "Kubernetes Horizontal Pod Autoscaler/status Watch": []
      },
      "Kubernetes Ingress": {
        "Kubernetes Ingress Create": [],
        "Kubernetes Ingress Delete": [],
        "Kubernetes Ingress Delete Multiple": [],
        "Kubernetes Ingress Get": [],
        "Kubernetes Ingress List": [],
        "Kubernetes Ingress Patch": [],
        "Kubernetes Ingress Update": [],
        "Kubernetes Ingress Watch": []
      },
      "Kubernetes Ingress/status": {
        "Kubernetes Ingress/status Create": [],
        "Kubernetes Ingress/status Delete": [],
        "Kubernetes Ingress/status Delete Multiple": [],
        "Kubernetes Ingress/status Get": [],
        "Kubernetes Ingress/status List": [],
        "Kubernetes Ingress/status Patch": [],
        "Kubernetes Ingress/status Update": [],
        "Kubernetes Ingress/status Watch": []
      },
      "Kubernetes Initializer Configuration": {
        "Kubernetes Initializer Configuration Create": [],
        "Kubernetes Initializer Configuration Delete": [],
        "Kubernetes Initializer Configuration Delete Multiple": [],
        "Kubernetes Initializer Configuration Get": [],
        "Kubernetes Initializer Configuration List": [],
        "Kubernetes Initializer Configuration Patch": [],
        "Kubernetes Initializer Configuration Update": [],
        "Kubernetes Initializer Configuration Watch": []
      },
      "Kubernetes Initializer Configuration/status": {
        "Kubernetes Initializer Configuration/status Create": [],
        "Kubernetes Initializer Configuration/status Delete": [],
        "Kubernetes Initializer Configuration/status Delete Multiple": [],
        "Kubernetes Initializer Configuration/status Get": [],
        "Kubernetes Initializer Configuration/status List": [],
        "Kubernetes Initializer Configuration/status Patch": [],
        "Kubernetes Initializer Configuration/status Update": [],
        "Kubernetes Initializer Configuration/status Watch": []
      },
      "Kubernetes Job": {
        "Kubernetes Job Create": [],
        "Kubernetes Job Delete": [],
        "Kubernetes Job Delete Multiple": [],
        "Kubernetes Job Get": [],
        "Kubernetes Job List": [],
        "Kubernetes Job Patch": [],
        "Kubernetes Job Update": [],
        "Kubernetes Job Watch": []
      },
      "Kubernetes Job/status": {
        "Kubernetes Job/status Create": [],
        "Kubernetes Job/status Delete": [],
        "Kubernetes Job/status Delete Multiple": [],
        "Kubernetes Job/status Get": [],
        "Kubernetes Job/status List": [],
        "Kubernetes Job/status Patch": [],
        "Kubernetes Job/status Update": [],
        "Kubernetes Job/status Watch": []
      },
      "Kubernetes Limit Range": {
        "Kubernetes Limit Range Create": [],
        "Kubernetes Limit Range Delete": [],
        "Kubernetes Limit Range Delete Multiple": [],
        "Kubernetes Limit Range Get": [],
        "Kubernetes Limit Range List": [],
        "Kubernetes Limit Range Patch": [],
        "Kubernetes Limit Range Update": [],
        "Kubernetes Limit Range Watch": []
      },
      "Kubernetes Limit Range/status": {
        "Kubernetes Limit Range/status Create": [],
        "Kubernetes Limit Range/status Delete": [],
        "Kubernetes Limit Range/status Delete Multiple": [],
        "Kubernetes Limit Range/status Get": [],
        "Kubernetes Limit Range/status List": [],
        "Kubernetes Limit Range/status Patch": [],
        "Kubernetes Limit Range/status Update": [],
        "Kubernetes Limit Range/status Watch": []
      },
      "Kubernetes Namespace": {
        "Kubernetes Namespace Create": [],
        "Kubernetes Namespace Delete": [],
        "Kubernetes Namespace Delete Multiple": [],
        "Kubernetes Namespace Get": [],
        "Kubernetes Namespace List": [],
        "Kubernetes Namespace Patch": [],
        "Kubernetes Namespace Update": [],
        "Kubernetes Namespace Watch": []
      },
      "Kubernetes Namespace/expansion": {
        "Kubernetes Namespace/expansion Create": [],
        "Kubernetes Namespace/expansion Delete": [],
        "Kubernetes Namespace/expansion Delete Multiple": [],
        "Kubernetes Namespace/expansion Get": [],
        "Kubernetes Namespace/expansion List": [],
        "Kubernetes Namespace/expansion Patch": [],
        "Kubernetes Namespace/expansion Update": [],
        "Kubernetes Namespace/expansion Watch": []
      },
      "Kubernetes Namespace/status": {
        "Kubernetes Namespace/status Create": [],
        "Kubernetes Namespace/status Delete": [],
        "Kubernetes Namespace/status Delete Multiple": [],
        "Kubernetes Namespace/status Get": [],
        "Kubernetes Namespace/status List": [],
        "Kubernetes Namespace/status Patch": [],
        "Kubernetes Namespace/status Update": [],
        "Kubernetes Namespace/status Watch": []
      },
      "Kubernetes Network Policy": {
        "Kubernetes Network Policy Create": [],
        "Kubernetes Network Policy Delete": [],
        "Kubernetes Network Policy Delete Multiple": [],
        "Kubernetes Network Policy Get": [],
        "Kubernetes Network Policy List": [],
        "Kubernetes Network Policy Patch": [],
        "Kubernetes Network Policy Update": [],
        "Kubernetes Network Policy Watch": []
      },
      "Kubernetes Network Policy/status": {
        "Kubernetes Network Policy/status Create": [],
        "Kubernetes Network Policy/status Delete": [],
        "Kubernetes Network Policy/status Delete Multiple": [],
        "Kubernetes Network Policy/status Get": [],
        "Kubernetes Network Policy/status List": [],
        "Kubernetes Network Policy/status Patch": [],
        "Kubernetes Network Policy/status Update": [],
        "Kubernetes Network Policy/status Watch": []
      },
      "Kubernetes Node": {
        "Kubernetes Node Create": [],
        "Kubernetes Node Delete": [],
        "Kubernetes Node Delete Multiple": [],
        "Kubernetes Node Get": [],
        "Kubernetes Node List": [],
        "Kubernetes Node Patch": [],
        "Kubernetes Node Update": [],
        "Kubernetes Node Watch": []
      },
      "Kubernetes Node/log": {
        "Kubernetes Node/log Create": [],
        "Kubernetes Node/log Delete": [],
        "Kubernetes Node/log Delete Multiple": [],
        "Kubernetes Node/log Get": [],
        "Kubernetes Node/log List": [],
        "Kubernetes Node/log Patch": [],
        "Kubernetes Node/log Update": [],
        "Kubernetes Node/log Watch": []
      },
      "Kubernetes Node/metrics": {
        "Kubernetes Node/metrics Create": [],
        "Kubernetes Node/metrics Delete": [],
        "Kubernetes Node/metrics Delete Multiple": [],
        "Kubernetes Node/metrics Get": [],
        "Kubernetes Node/metrics List": [],
        "Kubernetes Node/metrics Patch": [],
        "Kubernetes Node/metrics Update": [],
        "Kubernetes Node/metrics Watch": []
      },
      "Kubernetes Node/proxy": {
        "Kubernetes Node/proxy Create": [],
        "Kubernetes Node/proxy Delete": [],
        "Kubernetes Node/proxy Delete Multiple": [],
        "Kubernetes Node/proxy Get": [],
        "Kubernetes Node/proxy List": [],
        "Kubernetes Node/proxy Patch": [],
        "Kubernetes Node/proxy Update": [],
        "Kubernetes Node/proxy Watch": []
      },
      "Kubernetes Node/spec": {
        "Kubernetes Node/spec Create": [],
        "Kubernetes Node/spec Delete": [],
        "Kubernetes Node/spec Delete Multiple": [],
        "Kubernetes Node/spec Get": [],
        "Kubernetes Node/spec List": [],
        "Kubernetes Node/spec Patch": [],
        "Kubernetes Node/spec Update": [],
        "Kubernetes Node/spec Watch": []
      },
      "Kubernetes Node/stats": {
        "Kubernetes Node/stats Create": [],
        "Kubernetes Node/stats Delete": [],
        "Kubernetes Node/stats Delete Multiple": [],
        "Kubernetes Node/stats Get": [],
        "Kubernetes Node/stats List": [],
        "Kubernetes Node/stats Patch": [],
        "Kubernetes Node/stats Update": [],
        "Kubernetes Node/stats Watch": []
      },
      "Kubernetes Node/status": {
        "Kubernetes Node/status Create": [],
        "Kubernetes Node/status Delete": [],
        "Kubernetes Node/status Delete Multiple": [],
        "Kubernetes Node/status Get": [],
        "Kubernetes Node/status List": [],
        "Kubernetes Node/status Patch": [],
        "Kubernetes Node/status Update": [],
        "Kubernetes Node/status Watch": []
      },
      "Kubernetes Persistent Volume": {
        "Kubernetes Persistent Volume Create": [],
        "Kubernetes Persistent Volume Delete": [],
        "Kubernetes Persistent Volume Delete Multiple": [],
        "Kubernetes Persistent Volume Get": [],
        "Kubernetes Persistent Volume List": [],
        "Kubernetes Persistent Volume Patch": [],
        "Kubernetes Persistent Volume Update": [],
        "Kubernetes Persistent Volume Watch": []
      },
      "Kubernetes Persistent Volume Claim": {
        "Kubernetes Persistent Volume Claim Create": [],
        "Kubernetes Persistent Volume Claim Delete": [],
        "Kubernetes Persistent Volume Claim Delete Multiple": [],
        "Kubernetes Persistent Volume Claim Get": [],
        "Kubernetes Persistent Volume Claim List": [],
        "Kubernetes Persistent Volume Claim Patch": [],
        "Kubernetes Persistent Volume Claim Update": [],
        "Kubernetes Persistent Volume Claim Watch": []
      },
      "Kubernetes Persistent Volume Claim/status": {
        "Kubernetes Persistent Volume Claim/status Create": [],
        "Kubernetes Persistent Volume Claim/status Delete": [],
        "Kubernetes Persistent Volume Claim/status Delete Multiple": [],
        "Kubernetes Persistent Volume Claim/status Get": [],
        "Kubernetes Persistent Volume Claim/status List": [],
        "Kubernetes Persistent Volume Claim/status Patch": [],
        "Kubernetes Persistent Volume Claim/status Update": [],
        "Kubernetes Persistent Volume Claim/status Watch": []
      },
      "Kubernetes Persistent Volume/status": {
        "Kubernetes Persistent Volume/status Create": [],
        "Kubernetes Persistent Volume/status Delete": [],
        "Kubernetes Persistent Volume/status Delete Multiple": [],
        "Kubernetes Persistent Volume/status Get": [],
        "Kubernetes Persistent Volume/status List": [],
        "Kubernetes Persistent Volume/status Patch": [],
        "Kubernetes Persistent Volume/status Update": [],
        "Kubernetes Persistent Volume/status Watch": []
      },
      "Kubernetes Pod": {
        "Kubernetes Pod Create": [],
        "Kubernetes Pod Delete": [],
        "Kubernetes Pod Delete Multiple": [],
        "Kubernetes Pod Get": [],
        "Kubernetes Pod List": [],
        "Kubernetes Pod Patch": [],
        "Kubernetes Pod Update": [],
        "Kubernetes Pod Watch": []
      },
      "Kubernetes Pod Disruption Budget": {
        "Kubernetes Pod Disruption Budget Create": [],
        "Kubernetes Pod Disruption Budget Delete": [],
        "Kubernetes Pod Disruption Budget Delete Multiple": [],
        "Kubernetes Pod Disruption Budget Get": [],
        "Kubernetes Pod Disruption Budget List": [],
        "Kubernetes Pod Disruption Budget Patch": [],
        "Kubernetes Pod Disruption Budget Update": [],
        "Kubernetes Pod Disruption Budget Watch": []
      },
      "Kubernetes Pod Disruption Budget/status": {
        "Kubernetes Pod Disruption Budget/status Create": [],
        "Kubernetes Pod Disruption Budget/status Delete": [],
        "Kubernetes Pod Disruption Budget/status Delete Multiple": [],
        "Kubernetes Pod Disruption Budget/status Get": [],
        "Kubernetes Pod Disruption Budget/status List": [],
        "Kubernetes Pod Disruption Budget/status Patch": [],
        "Kubernetes Pod Disruption Budget/status Update": [],
        "Kubernetes Pod Disruption Budget/status Watch": []
      },
      "Kubernetes Pod Preset": {
        "Kubernetes Pod Preset Create": [],
        "Kubernetes Pod Preset Delete": [],
        "Kubernetes Pod Preset Delete Multiple": [],
        "Kubernetes Pod Preset Get": [],
        "Kubernetes Pod Preset List": [],
        "Kubernetes Pod Preset Patch": [],
        "Kubernetes Pod Preset Update": [],
        "Kubernetes Pod Preset Watch": []
      },
      "Kubernetes Pod Preset/status": {
        "Kubernetes Pod Preset/status Create": [],
        "Kubernetes Pod Preset/status Delete": [],
        "Kubernetes Pod Preset/status Delete Multiple": [],
        "Kubernetes Pod Preset/status Get": [],
        "Kubernetes Pod Preset/status List": [],
        "Kubernetes Pod Preset/status Patch": [],
        "Kubernetes Pod Preset/status Update": [],
        "Kubernetes Pod Preset/status Watch": []
      },
      "Kubernetes Pod Security Policy": {
        "Kubernetes Pod Security Policy Create": [],
        "Kubernetes Pod Security Policy Delete": [],
        "Kubernetes Pod Security Policy Delete Multiple": [],
        "Kubernetes Pod Security Policy Get": [],
        "Kubernetes Pod Security Policy List": [],
        "Kubernetes Pod Security Policy Patch": [],
        "Kubernetes Pod Security Policy Update": [],
        "Kubernetes Pod Security Policy Watch": []
      },
      "Kubernetes Pod Security Policy/status": {
        "Kubernetes Pod Security Policy/status Create": [],
        "Kubernetes Pod Security Policy/status Delete": [],
        "Kubernetes Pod Security Policy/status Delete Multiple": [],
        "Kubernetes Pod Security Policy/status Get": [],
        "Kubernetes Pod Security Policy/status List": [],
        "Kubernetes Pod Security Policy/status Patch": [],
        "Kubernetes Pod Security Policy/status Update": [],
        "Kubernetes Pod Security Policy/status Watch": []
      },
      "Kubernetes Pod Template": {
        "Kubernetes Pod Template Create": [],
        "Kubernetes Pod Template Delete": [],
        "Kubernetes Pod Template Delete Multiple": [],
        "Kubernetes Pod Template Get": [],
        "Kubernetes Pod Template List": [],
        "Kubernetes Pod Template Patch": [],
        "Kubernetes Pod Template Update": [],
        "Kubernetes Pod Template Watch": []
      },
      "Kubernetes Pod Template/status": {
        "Kubernetes Pod Template/status Create": [],
        "Kubernetes Pod Template/status Delete": [],
        "Kubernetes Pod Template/status Delete Multiple": [],
        "Kubernetes Pod Template/status Get": [],
        "Kubernetes Pod Template/status List": [],
        "Kubernetes Pod Template/status Patch": [],
        "Kubernetes Pod Template/status Update": [],
        "Kubernetes Pod Template/status Watch": []
      },
      "Kubernetes Pod/attach": {
        "Kubernetes Pod/attach Create": [],
        "Kubernetes Pod/attach Delete": [],
        "Kubernetes Pod/attach Delete Multiple": [],
        "Kubernetes Pod/attach Get": [],
        "Kubernetes Pod/attach List": [],
        "Kubernetes Pod/attach Patch": [],
        "Kubernetes Pod/attach Update": [],
        "Kubernetes Pod/attach Watch": []
      },
      "Kubernetes Pod/binding": {
        "Kubernetes Pod/binding Create": [],
        "Kubernetes Pod/binding Delete": [],
        "Kubernetes Pod/binding Delete Multiple": [],
        "Kubernetes Pod/binding Get": [],
        "Kubernetes Pod/binding List": [],
        "Kubernetes Pod/binding Patch": [],
        "Kubernetes Pod/binding Update": [],
        "Kubernetes Pod/binding Watch": []
      },
      "Kubernetes Pod/eviction": {
        "Kubernetes Pod/eviction Create": [],
        "Kubernetes Pod/eviction Delete": [],
        "Kubernetes Pod/eviction Delete Multiple": [],
        "Kubernetes Pod/eviction Get": [],
        "Kubernetes Pod/eviction List": [],
        "Kubernetes Pod/eviction Patch": [],
        "Kubernetes Pod/eviction Update": [],
        "Kubernetes Pod/eviction Watch": []
      },
      "Kubernetes Pod/exec": {
        "Kubernetes Pod/exec Create": [],
        "Kubernetes Pod/exec Delete": [],
        "Kubernetes Pod/exec Delete Multiple": [],
        "Kubernetes Pod/exec Get": [],
        "Kubernetes Pod/exec List": [],
        "Kubernetes Pod/exec Patch": [],
        "Kubernetes Pod/exec Update": [],
        "Kubernetes Pod/exec Watch": []
      },
      "Kubernetes Pod/log": {
        "Kubernetes Pod/log Create": [],
        "Kubernetes Pod/log Delete": [],
        "Kubernetes Pod/log Delete Multiple": [],
        "Kubernetes Pod/log Get": [],
        "Kubernetes Pod/log List": [],
        "Kubernetes Pod/log Patch": [],
        "Kubernetes Pod/log Update": [],
        "Kubernetes Pod/log Watch": []
      },
      "Kubernetes Pod/portforward": {
        "Kubernetes Pod/portforward Create": [],
        "Kubernetes Pod/portforward Delete": [],
        "Kubernetes Pod/portforward Delete Multiple": [],
        "Kubernetes Pod/portforward Get": [],
        "Kubernetes Pod/portforward List": [],
        "Kubernetes Pod/portforward Patch": [],
        "Kubernetes Pod/portforward Update": [],
        "Kubernetes Pod/portforward Watch": []
      },
      "Kubernetes Pod/proxy": {
        "Kubernetes Pod/proxy Create": [],
        "Kubernetes Pod/proxy Delete": [],
        "Kubernetes Pod/proxy Delete Multiple": [],
        "Kubernetes Pod/proxy Get": [],
        "Kubernetes Pod/proxy List": [],
        "Kubernetes Pod/proxy Patch": [],
        "Kubernetes Pod/proxy Update": [],
        "Kubernetes Pod/proxy Watch": []
      },
      "Kubernetes Pod/status": {
        "Kubernetes Pod/status Create": [],
        "Kubernetes Pod/status Delete": [],
        "Kubernetes Pod/status Delete Multiple": [],
        "Kubernetes Pod/status Get": [],
        "Kubernetes Pod/status List": [],
        "Kubernetes Pod/status Patch": [],
        "Kubernetes Pod/status Update": [],
        "Kubernetes Pod/status Watch": []
      },
      "Kubernetes Replica Set": {
        "Kubernetes Replica Set Create": [],
        "Kubernetes Replica Set Delete": [],
        "Kubernetes Replica Set Delete Multiple": [],
        "Kubernetes Replica Set Get": [],
        "Kubernetes Replica Set List": [],
        "Kubernetes Replica Set Patch": [],
        "Kubernetes Replica Set Update": [],
        "Kubernetes Replica Set Watch": []
      },
      "Kubernetes Replica Set/scale": {
        "Kubernetes Replica Set/scale Create": [],
        "Kubernetes Replica Set/scale Delete": [],
        "Kubernetes Replica Set/scale Delete Multiple": [],
        "Kubernetes Replica Set/scale Get": [],
        "Kubernetes Replica Set/scale List": [],
        "Kubernetes Replica Set/scale Patch": [],
        "Kubernetes Replica Set/scale Update": [],
        "Kubernetes Replica Set/scale Watch": []
      },
      "Kubernetes Replica Set/status": {
        "Kubernetes Replica Set/status Create": [],
        "Kubernetes Replica Set/status Delete": [],
        "Kubernetes Replica Set/status Delete Multiple": [],
        "Kubernetes Replica Set/status Get": [],
        "Kubernetes Replica Set/status List": [],
        "Kubernetes Replica Set/status Patch": [],
        "Kubernetes Replica Set/status Update": [],
        "Kubernetes Replica Set/status Watch": []
      },
      "Kubernetes Replication Controller": {
        "Kubernetes Replication Controller Create": [],
        "Kubernetes Replication Controller Delete": [],
        "Kubernetes Replication Controller Delete Multiple": [],
        "Kubernetes Replication Controller Get": [],
        "Kubernetes Replication Controller List": [],
        "Kubernetes Replication Controller Patch": [],
        "Kubernetes Replication Controller Update": [],
        "Kubernetes Replication Controller Watch": []
      },
      "Kubernetes Replication Controller/scale": {
        "Kubernetes Replication Controller/scale Create": [],
        "Kubernetes Replication Controller/scale Delete": [],
        "Kubernetes Replication Controller/scale Delete Multiple": [],
        "Kubernetes Replication Controller/scale Get": [],
        "Kubernetes Replication Controller/scale List": [],
        "Kubernetes Replication Controller/scale Patch": [],
        "Kubernetes Replication Controller/scale Update": [],
        "Kubernetes Replication Controller/scale Watch": []
      },
      "Kubernetes Replication Controller/status": {
        "Kubernetes Replication Controller/status Create": [],
        "Kubernetes Replication Controller/status Delete": [],
        "Kubernetes Replication Controller/status Delete Multiple": [],
        "Kubernetes Replication Controller/status Get": [],
        "Kubernetes Replication Controller/status List": [],
        "Kubernetes Replication Controller/status Patch": [],
        "Kubernetes Replication Controller/status Update": [],
        "Kubernetes Replication Controller/status Watch": []
      },
      "Kubernetes Resource Quota": {
        "Kubernetes Resource Quota Create": [],
        "Kubernetes Resource Quota Delete": [],
        "Kubernetes Resource Quota Delete Multiple": [],
        "Kubernetes Resource Quota Get": [],
        "Kubernetes Resource Quota List": [],
        "Kubernetes Resource Quota Patch": [],
        "Kubernetes Resource Quota Update": [],
        "Kubernetes Resource Quota Watch": []
      },
      "Kubernetes Resource Quota/status": {
        "Kubernetes Resource Quota/status Create": [],
        "Kubernetes Resource Quota/status Delete": [],
        "Kubernetes Resource Quota/status Delete Multiple": [],
        "Kubernetes Resource Quota/status Get": [],
        "Kubernetes Resource Quota/status List": [],
        "Kubernetes Resource Quota/status Patch": [],
        "Kubernetes Resource Quota/status Update": [],
        "Kubernetes Resource Quota/status Watch": []
      },
      "Kubernetes Role": {
        "Kubernetes Role Create": [],
        "Kubernetes Role Delete": [],
        "Kubernetes Role Delete Multiple": [],
        "Kubernetes Role Get": [],
        "Kubernetes Role List": [],
        "Kubernetes Role Patch": [],
        "Kubernetes Role Update": [],
        "Kubernetes Role Watch": []
      },
      "Kubernetes Role Binding": {
        "Kubernetes Role Binding Create": [],
        "Kubernetes Role Binding Delete": [],
        "Kubernetes Role Binding Delete Multiple": [],
        "Kubernetes Role Binding Get": [],
        "Kubernetes Role Binding List": [],
        "Kubernetes Role Binding Patch": [],
        "Kubernetes Role Binding Update": [],
        "Kubernetes Role Binding Watch": []
      },
      "Kubernetes Role Binding/status": {
        "Kubernetes Role Binding/status Create": [],
        "Kubernetes Role Binding/status Delete": [],
        "Kubernetes Role Binding/status Delete Multiple": [],
        "Kubernetes Role Binding/status Get": [],
        "Kubernetes Role Binding/status List": [],
        "Kubernetes Role Binding/status Patch": [],
        "Kubernetes Role Binding/status Update": [],
        "Kubernetes Role Binding/status Watch": []
      },
      "Kubernetes Role/status": {
        "Kubernetes Role/status Create": [],
        "Kubernetes Role/status Delete": [],
        "Kubernetes Role/status Delete Multiple": [],
        "Kubernetes Role/status Get": [],
        "Kubernetes Role/status List": [],
        "Kubernetes Role/status Patch": [],
        "Kubernetes Role/status Update": [],
        "Kubernetes Role/status Watch": []
      },
      "Kubernetes Secret": {
        "Kubernetes Secret Create": [],
        "Kubernetes Secret Delete": [],
        "Kubernetes Secret Delete Multiple": [],
        "Kubernetes Secret Get": [],
        "Kubernetes Secret List": [],
        "Kubernetes Secret Patch": [],
        "Kubernetes Secret Update": [],
        "Kubernetes Secret Watch": []
      },
      "Kubernetes Secret/status": {
        "Kubernetes Secret/status Create": [],
        "Kubernetes Secret/status Delete": [],
        "Kubernetes Secret/status Delete Multiple": [],
        "Kubernetes Secret/status Get": [],
        "Kubernetes Secret/status List": [],
        "Kubernetes Secret/status Patch": [],
        "Kubernetes Secret/status Update": [],
        "Kubernetes Secret/status Watch": []
      },
      "Kubernetes Service": {
        "Kubernetes Service Create": [],
        "Kubernetes Service Delete": [],
        "Kubernetes Service Delete Multiple": [],
        "Kubernetes Service Get": [],
        "Kubernetes Service List": [],
        "Kubernetes Service Patch": [],
        "Kubernetes Service Update": [],
        "Kubernetes Service Watch": []
      },
      "Kubernetes Service Account": {
        "Kubernetes Service Account Create": [],
        "Kubernetes Service Account Delete": [],
        "Kubernetes Service Account Delete Multiple": [],
        "Kubernetes Service Account Get": [],
        "Kubernetes Service Account Impersonate": [],
        "Kubernetes Service Account List": [],
        "Kubernetes Service Account Patch": [],
        "Kubernetes Service Account Update": [],
        "Kubernetes Service Account Watch": []
      },
      "Kubernetes Service Account/status": {
        "Kubernetes Service Account/status Create": [],
        "Kubernetes Service Account/status Delete": [],
        "Kubernetes Service Account/status Delete Multiple": [],
        "Kubernetes Service Account/status Get": [],
        "Kubernetes Service Account/status List": [],
        "Kubernetes Service Account/status Patch": [],
        "Kubernetes Service Account/status Update": [],
        "Kubernetes Service Account/status Watch": []
      },
      "Kubernetes Service Account/token": {
        "Kubernetes Service Account/token Create": [],
        "Kubernetes Service Account/token Delete": [],
        "Kubernetes Service Account/token Delete Multiple": [],
        "Kubernetes Service Account/token Get": [],
        "Kubernetes Service Account/token List": [],
        "Kubernetes Service Account/token Patch": [],
        "Kubernetes Service Account/token Update": [],
        "Kubernetes Service Account/token Watch": []
      },
      "Kubernetes Service/proxy": {
        "Kubernetes Service/proxy Create": [],
        "Kubernetes Service/proxy Delete": [],
        "Kubernetes Service/proxy Delete Multiple": [],
        "Kubernetes Service/proxy Get": [],
        "Kubernetes Service/proxy List": [],
        "Kubernetes Service/proxy Patch": [],
        "Kubernetes Service/proxy Update": [],
        "Kubernetes Service/proxy Watch": []
      },
      "Kubernetes Service/status": {
        "Kubernetes Service/status Create": [],
        "Kubernetes Service/status Delete": [],
        "Kubernetes Service/status Delete Multiple": [],
        "Kubernetes Service/status Get": [],
        "Kubernetes Service/status List": [],
        "Kubernetes Service/status Patch": [],
        "Kubernetes Service/status Update": [],
        "Kubernetes Service/status Watch": []
      },
      "Kubernetes Stack": {
        "Kubernetes Stack Create": [],
        "Kubernetes Stack Delete": [],
        "Kubernetes Stack Delete Multiple": [],
        "Kubernetes Stack Get": [],
        "Kubernetes Stack List": [],
        "Kubernetes Stack Patch": [],
        "Kubernetes Stack Update": [],
        "Kubernetes Stack Watch": []
      },
      "Kubernetes Stack/status": {
        "Kubernetes Stack/status Create": [],
        "Kubernetes Stack/status Delete": [],
        "Kubernetes Stack/status Delete Multiple": [],
        "Kubernetes Stack/status Get": [],
        "Kubernetes Stack/status List": [],
        "Kubernetes Stack/status Patch": [],
        "Kubernetes Stack/status Update": [],
        "Kubernetes Stack/status Watch": []
      },
      "Kubernetes Stateful Set": {
        "Kubernetes Stateful Set Create": [],
        "Kubernetes Stateful Set Delete": [],
        "Kubernetes Stateful Set Delete Multiple": [],
        "Kubernetes Stateful Set Get": [],
        "Kubernetes Stateful Set List": [],
        "Kubernetes Stateful Set Patch": [],
        "Kubernetes Stateful Set Update": [],
        "Kubernetes Stateful Set Watch": []
      },
      "Kubernetes Stateful Set/scale": {
        "Kubernetes Stateful Set/scale Create": [],
        "Kubernetes Stateful Set/scale Delete": [],
        "Kubernetes Stateful Set/scale Delete Multiple": [],
        "Kubernetes Stateful Set/scale Get": [],
        "Kubernetes Stateful Set/scale List": [],
        "Kubernetes Stateful Set/scale Patch": [],
        "Kubernetes Stateful Set/scale Update": [],
        "Kubernetes Stateful Set/scale Watch": []
      },
      "Kubernetes Stateful Set/status": {
        "Kubernetes Stateful Set/status Create": [],
        "Kubernetes Stateful Set/status Delete": [],
        "Kubernetes Stateful Set/status Delete Multiple": [],
        "Kubernetes Stateful Set/status Get": [],
        "Kubernetes Stateful Set/status List": [],
        "Kubernetes Stateful Set/status Patch": [],
        "Kubernetes Stateful Set/status Update": [],
        "Kubernetes Stateful Set/status Watch": []
      },
      "Kubernetes Storage Class": {
        "Kubernetes Storage Class Create": [],
        "Kubernetes Storage Class Delete": [],
        "Kubernetes Storage Class Delete Multiple": [],
        "Kubernetes Storage Class Get": [],
        "Kubernetes Storage Class List": [],
        "Kubernetes Storage Class Patch": [],
        "Kubernetes Storage Class Update": [],
        "Kubernetes Storage Class Watch": []
      },
      "Kubernetes Storage Class/status": {
        "Kubernetes Storage Class/status Create": [],
        "Kubernetes Storage Class/status Delete": [],
        "Kubernetes Storage Class/status Delete Multiple": [],
        "Kubernetes Storage Class/status Get": [],
        "Kubernetes Storage Class/status List": [],
        "Kubernetes Storage Class/status Patch": [],
        "Kubernetes Storage Class/status Update": [],
        "Kubernetes Storage Class/status Watch": []
      },
      "Kubernetes Third Party Resource": {
        "Kubernetes Third Party Resource Create": [],
        "Kubernetes Third Party Resource Delete": [],
        "Kubernetes Third Party Resource Delete Multiple": [],
        "Kubernetes Third Party Resource Get": [],
        "Kubernetes Third Party Resource List": [],
        "Kubernetes Third Party Resource Patch": [],
        "Kubernetes Third Party Resource Update": [],
        "Kubernetes Third Party Resource Watch": []
      },
      "Kubernetes Third Party Resource/status": {
        "Kubernetes Third Party Resource/status Create": [],
        "Kubernetes Third Party Resource/status Delete": [],
        "Kubernetes Third Party Resource/status Delete Multiple": [],
        "Kubernetes Third Party Resource/status Get": [],
        "Kubernetes Third Party Resource/status List": [],
        "Kubernetes Third Party Resource/status Patch": [],
        "Kubernetes Third Party Resource/status Update": [],
        "Kubernetes Third Party Resource/status Watch": []
      },
      "Kubernetes User": {
        "Kubernetes User Create": [],
        "Kubernetes User Delete": [],
        "Kubernetes User Delete Multiple": [],
        "Kubernetes User Get": [],
        "Kubernetes User Impersonate": [],
        "Kubernetes User List": [],
        "Kubernetes User Patch": [],
        "Kubernetes User Update": [],
        "Kubernetes User Watch": []
      },
      "Kubernetes User/status": {
        "Kubernetes User/status Create": [],
        "Kubernetes User/status Delete": [],
        "Kubernetes User/status Delete Multiple": [],
        "Kubernetes User/status Get": [],
        "Kubernetes User/status List": [],
        "Kubernetes User/status Patch": [],
        "Kubernetes User/status Update": [],
        "Kubernetes User/status Watch": []
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
      "Event Patch": [],
      "Event Update": [],
      "Event Watch": []
    },
      "Kubernetes Persistent Volume": {
        "Kubernetes Persistent Volume Create": [],
        "Kubernetes Persistent Volume Delete": [],
        "Kubernetes Persistent Volume Delete Multiple": [],
        "Kubernetes Persistent Volume Get": [],
        "Kubernetes Persistent Volume List": [],
        "Kubernetes Persistent Volume Patch": [],
        "Kubernetes Persistent Volume Update": [],
        "Kubernetes Persistent Volume Watch": []
      },
      "Kubernetes Persistent Volume Claim": {
        "Kubernetes Persistent Volume Claim Create": [],
        "Kubernetes Persistent Volume Claim Delete": [],
        "Kubernetes Persistent Volume Claim Delete Multiple": [],
        "Kubernetes Persistent Volume Claim Get": [],
        "Kubernetes Persistent Volume Claim List": [],
        "Kubernetes Persistent Volume Claim Patch": [],
        "Kubernetes Persistent Volume Claim Update": [],
        "Kubernetes Persistent Volume Claim Watch": []
      },
      "Kubernetes Persistent Volume Claim/status": {
        "Kubernetes Persistent Volume Claim/status Create": [],
        "Kubernetes Persistent Volume Claim/status Delete": [],
        "Kubernetes Persistent Volume Claim/status Delete Multiple": [],
        "Kubernetes Persistent Volume Claim/status Get": [],
        "Kubernetes Persistent Volume Claim/status List": [],
        "Kubernetes Persistent Volume Claim/status Patch": [],
        "Kubernetes Persistent Volume Claim/status Update": [],
        "Kubernetes Persistent Volume Claim/status Watch": []
      },
      "Kubernetes Persistent Volume/status": {
        "Kubernetes Persistent Volume/status Create": [],
        "Kubernetes Persistent Volume/status Delete": [],
        "Kubernetes Persistent Volume/status Delete Multiple": [],
        "Kubernetes Persistent Volume/status Get": [],
        "Kubernetes Persistent Volume/status List": [],
        "Kubernetes Persistent Volume/status Patch": [],
        "Kubernetes Persistent Volume/status Update": [],
        "Kubernetes Persistent Volume/status Watch": []
      },
      "Kubernetes Storage Class": {
        "Kubernetes Storage Class Create": [],
        "Kubernetes Storage Class Delete": [],
        "Kubernetes Storage Class Delete Multiple": [],
        "Kubernetes Storage Class Get": [],
        "Kubernetes Storage Class List": [],
        "Kubernetes Storage Class Patch": [],
        "Kubernetes Storage Class Update": [],
        "Kubernetes Storage Class Watch": []
      },
      "Kubernetes Storage Class/status": {
        "Kubernetes Storage Class/status Create": [],
        "Kubernetes Storage Class/status Delete": [],
        "Kubernetes Storage Class/status Delete Multiple": [],
        "Kubernetes Storage Class/status Get": [],
        "Kubernetes Storage Class/status List": [],
        "Kubernetes Storage Class/status Patch": [],
        "Kubernetes Storage Class/status Update": [],
        "Kubernetes Storage Class/status Watch": []
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
func GetUCPRoleCreateJSON(serverVersion, roleName string) string {

	// use the UCP version to determine the appropriate JSON to send for role creation
	var jsonToUse string
	switch serverVersion {
	case "ucp/3.0.0":
		jsonToUse = LegacyUCPRoleCreateJSONTemplateMinimalPermissions
	case "ucp/3.0.1":
		jsonToUse = LegacyUCPRoleCreateJSONTemplateMinimalPermissions
	default:
		jsonToUse = UCPRoleCreateJSONTemplateMinimalPermissions
	}

	createYAML := strings.Replace(jsonToUse, "{ROLE_NAME}", roleName, 1)
	return createYAML
}
