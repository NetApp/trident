// Copyright 2018 NetApp, Inc. All Rights Reserved.

package ucpclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
)

var (
	defaultTridentRole = "trident"
)

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// BEGIN: UCP objects
//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// UCPCollectionGrants holds the results from calls to "/collectionGrants"
type UCPCollectionGrants struct {
	Grants []struct {
		CollectionPath string `json:"collectionPath"`
		SubjectID      string `json:"subjectID"`
		ObjectID       string `json:"objectID"`
		RoleID         string `json:"roleID"`
	} `json:"grants"`
	Subjects []struct {
		ID          string `json:"id"`
		SubjectType string `json:"subject_type"`
	} `json:"subjects"`
}

// UCPRole holds the results from calls to "/roles"
type UCPRole struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SystemRole bool   `json:"system_role"`
	Operations struct {
		Config struct {
			ConfigView []interface{} `json:"Config View"`
		} `json:"Config"`
		Container struct {
			ContainerLogs  []interface{} `json:"Container Logs"`
			ContainerStats []interface{} `json:"Container Stats"`
			ContainerTop   []interface{} `json:"Container Top"`
			ContainerView  []interface{} `json:"Container View"`
		} `json:"Container"`
		Image struct {
			ImageHistory []interface{} `json:"Image History"`
			ImageView    []interface{} `json:"Image View"`
		} `json:"Image"`
		KubernetesCertificateSigningRequest struct {
			CertificateSigningRequestGet   []interface{} `json:"CertificateSigningRequest Get"`
			CertificateSigningRequestList  []interface{} `json:"CertificateSigningRequest List"`
			CertificateSigningRequestWatch []interface{} `json:"CertificateSigningRequest Watch"`
		} `json:"Kubernetes Certificate Signing Request"`
		KubernetesClusterRole struct {
			ClusterRoleGet   []interface{} `json:"ClusterRole Get"`
			ClusterRoleList  []interface{} `json:"ClusterRole List"`
			ClusterRoleWatch []interface{} `json:"ClusterRole Watch"`
		} `json:"Kubernetes Cluster Role"`
		KubernetesClusterRoleBinding struct {
			ClusterRoleBindingGet   []interface{} `json:"ClusterRoleBinding Get"`
			ClusterRoleBindingList  []interface{} `json:"ClusterRoleBinding List"`
			ClusterRoleBindingWatch []interface{} `json:"ClusterRoleBinding Watch"`
		} `json:"Kubernetes Cluster Role Binding"`
		KubernetesComponentStatus struct {
			ComponentStatusGet   []interface{} `json:"ComponentStatus Get"`
			ComponentStatusList  []interface{} `json:"ComponentStatus List"`
			ComponentStatusWatch []interface{} `json:"ComponentStatus Watch"`
		} `json:"Kubernetes Component Status"`
		KubernetesConfigMap struct {
			ConfigMapGet   []interface{} `json:"ConfigMap Get"`
			ConfigMapList  []interface{} `json:"ConfigMap List"`
			ConfigMapWatch []interface{} `json:"ConfigMap Watch"`
		} `json:"Kubernetes Config Map"`
		KubernetesControllerRevision struct {
			ControllerRevisionGet   []interface{} `json:"ControllerRevision Get"`
			ControllerRevisionList  []interface{} `json:"ControllerRevision List"`
			ControllerRevisionWatch []interface{} `json:"ControllerRevision Watch"`
		} `json:"Kubernetes Controller Revision"`
		KubernetesCronJob struct {
			CronJobGet   []interface{} `json:"CronJob Get"`
			CronJobList  []interface{} `json:"CronJob List"`
			CronJobWatch []interface{} `json:"CronJob Watch"`
		} `json:"Kubernetes Cron Job"`
		KubernetesCustomResourceDefinition struct {
			CustomResourceDefinitionGet   []interface{} `json:"CustomResourceDefinition Get"`
			CustomResourceDefinitionList  []interface{} `json:"CustomResourceDefinition List"`
			CustomResourceDefinitionWatch []interface{} `json:"CustomResourceDefinition Watch"`
		} `json:"Kubernetes Custom Resource Definition"`
		KubernetesDaemonset struct {
			DaemonsetGet   []interface{} `json:"Daemonset Get"`
			DaemonsetList  []interface{} `json:"Daemonset List"`
			DaemonsetWatch []interface{} `json:"Daemonset Watch"`
		} `json:"Kubernetes Daemonset"`
		KubernetesDeployment struct {
			DeploymentGet   []interface{} `json:"Deployment Get"`
			DeploymentList  []interface{} `json:"Deployment List"`
			DeploymentWatch []interface{} `json:"Deployment Watch"`
		} `json:"Kubernetes Deployment"`
		KubernetesEndpoint struct {
			EndpointGet   []interface{} `json:"Endpoint Get"`
			EndpointList  []interface{} `json:"Endpoint List"`
			EndpointWatch []interface{} `json:"Endpoint Watch"`
		} `json:"Kubernetes Endpoint"`
		KubernetesEvent struct {
			EventGet   []interface{} `json:"Event Get"`
			EventList  []interface{} `json:"Event List"`
			EventWatch []interface{} `json:"Event Watch"`
		} `json:"Kubernetes Event"`
		KubernetesExternalAdmissionHookConfiguration struct {
			ExternalAdmissionHookConfigurationGet   []interface{} `json:"ExternalAdmissionHookConfiguration Get"`
			ExternalAdmissionHookConfigurationList  []interface{} `json:"ExternalAdmissionHookConfiguration List"`
			ExternalAdmissionHookConfigurationWatch []interface{} `json:"ExternalAdmissionHookConfiguration Watch"`
		} `json:"Kubernetes External Admission Hook Configuration"`
		KubernetesHorizontalPodAutoscaler struct {
			HorizontalPodAutoscalerGet   []interface{} `json:"HorizontalPodAutoscaler Get"`
			HorizontalPodAutoscalerList  []interface{} `json:"HorizontalPodAutoscaler List"`
			HorizontalPodAutoscalerWatch []interface{} `json:"HorizontalPodAutoscaler Watch"`
		} `json:"Kubernetes Horizontal Pod Autoscaler"`
		KubernetesIngress struct {
			IngressGet   []interface{} `json:"Ingress Get"`
			IngressList  []interface{} `json:"Ingress List"`
			IngressWatch []interface{} `json:"Ingress Watch"`
		} `json:"Kubernetes Ingress"`
		KubernetesInitializerConfiguration struct {
			InitializerConfigurationGet   []interface{} `json:"InitializerConfiguration Get"`
			InitializerConfigurationList  []interface{} `json:"InitializerConfiguration List"`
			InitializerConfigurationWatch []interface{} `json:"InitializerConfiguration Watch"`
		} `json:"Kubernetes Initializer Configuration"`
		KubernetesJob struct {
			JobGet   []interface{} `json:"Job Get"`
			JobList  []interface{} `json:"Job List"`
			JobWatch []interface{} `json:"Job Watch"`
		} `json:"Kubernetes Job"`
		KubernetesLimitRange struct {
			LimitRangeGet   []interface{} `json:"LimitRange Get"`
			LimitRangeList  []interface{} `json:"LimitRange List"`
			LimitRangeWatch []interface{} `json:"LimitRange Watch"`
		} `json:"Kubernetes Limit Range"`
		KubernetesNamespace struct {
			NamespaceGet   []interface{} `json:"Namespace Get"`
			NamespaceList  []interface{} `json:"Namespace List"`
			NamespaceWatch []interface{} `json:"Namespace Watch"`
		} `json:"Kubernetes Namespace"`
		KubernetesNetworkPolicy struct {
			NetworkPolicyGet   []interface{} `json:"NetworkPolicy Get"`
			NetworkPolicyList  []interface{} `json:"NetworkPolicy List"`
			NetworkPolicyWatch []interface{} `json:"NetworkPolicy Watch"`
		} `json:"Kubernetes Network Policy"`
		KubernetesNode struct {
			NodeGet   []interface{} `json:"Node Get"`
			NodeList  []interface{} `json:"Node List"`
			NodeWatch []interface{} `json:"Node Watch"`
		} `json:"Kubernetes Node"`
		KubernetesPersistentVolume struct {
			PersistentVolumeGet   []interface{} `json:"PersistentVolume Get"`
			PersistentVolumeList  []interface{} `json:"PersistentVolume List"`
			PersistentVolumeWatch []interface{} `json:"PersistentVolume Watch"`
		} `json:"Kubernetes Persistent Volume"`
		KubernetesPersistentVolumeClaim struct {
			PersistentVolumeClaimGet   []interface{} `json:"PersistentVolumeClaim Get"`
			PersistentVolumeClaimList  []interface{} `json:"PersistentVolumeClaim List"`
			PersistentVolumeClaimWatch []interface{} `json:"PersistentVolumeClaim Watch"`
		} `json:"Kubernetes Persistent Volume Claim"`
		KubernetesPod struct {
			PodGet   []interface{} `json:"Pod Get"`
			PodList  []interface{} `json:"Pod List"`
			PodWatch []interface{} `json:"Pod Watch"`
		} `json:"Kubernetes Pod"`
		KubernetesPodDisruptionBudget struct {
			PodDisruptionBudgetGet   []interface{} `json:"PodDisruptionBudget Get"`
			PodDisruptionBudgetList  []interface{} `json:"PodDisruptionBudget List"`
			PodDisruptionBudgetWatch []interface{} `json:"PodDisruptionBudget Watch"`
		} `json:"Kubernetes Pod Disruption Budget"`
		KubernetesPodPreset struct {
			PodPresetGet   []interface{} `json:"PodPreset Get"`
			PodPresetList  []interface{} `json:"PodPreset List"`
			PodPresetWatch []interface{} `json:"PodPreset Watch"`
		} `json:"Kubernetes Pod Preset"`
		KubernetesPodSecurityPolicy struct {
			PodSecurityPolicyGet   []interface{} `json:"PodSecurityPolicy Get"`
			PodSecurityPolicyList  []interface{} `json:"PodSecurityPolicy List"`
			PodSecurityPolicyWatch []interface{} `json:"PodSecurityPolicy Watch"`
		} `json:"Kubernetes Pod Security Policy"`
		KubernetesPodTemplate struct {
			PodTemplateGet   []interface{} `json:"PodTemplate Get"`
			PodTemplateList  []interface{} `json:"PodTemplate List"`
			PodTemplateWatch []interface{} `json:"PodTemplate Watch"`
		} `json:"Kubernetes Pod Template"`
		KubernetesReplicaSet struct {
			ReplicaSetGet   []interface{} `json:"ReplicaSet Get"`
			ReplicaSetList  []interface{} `json:"ReplicaSet List"`
			ReplicaSetWatch []interface{} `json:"ReplicaSet Watch"`
		} `json:"Kubernetes Replica Set"`
		KubernetesReplicationController struct {
			ReplicationControllerGet   []interface{} `json:"ReplicationController Get"`
			ReplicationControllerList  []interface{} `json:"ReplicationController List"`
			ReplicationControllerWatch []interface{} `json:"ReplicationController Watch"`
		} `json:"Kubernetes Replication Controller"`
		KubernetesResourceQuota struct {
			ResourceQuotaGet   []interface{} `json:"ResourceQuota Get"`
			ResourceQuotaList  []interface{} `json:"ResourceQuota List"`
			ResourceQuotaWatch []interface{} `json:"ResourceQuota Watch"`
		} `json:"Kubernetes Resource Quota"`
		KubernetesRole struct {
			RoleGet   []interface{} `json:"Role Get"`
			RoleList  []interface{} `json:"Role List"`
			RoleWatch []interface{} `json:"Role Watch"`
		} `json:"Kubernetes Role"`
		KubernetesRoleBinding struct {
			RoleBindingGet   []interface{} `json:"RoleBinding Get"`
			RoleBindingList  []interface{} `json:"RoleBinding List"`
			RoleBindingWatch []interface{} `json:"RoleBinding Watch"`
		} `json:"Kubernetes Role Binding"`
		KubernetesSecret struct {
			SecretGet   []interface{} `json:"Secret Get"`
			SecretList  []interface{} `json:"Secret List"`
			SecretWatch []interface{} `json:"Secret Watch"`
		} `json:"Kubernetes Secret"`
		KubernetesService struct {
			ServiceGet   []interface{} `json:"Service Get"`
			ServiceList  []interface{} `json:"Service List"`
			ServiceWatch []interface{} `json:"Service Watch"`
		} `json:"Kubernetes Service"`
		KubernetesServiceAccount struct {
			ServiceAccountGet   []interface{} `json:"ServiceAccount Get"`
			ServiceAccountList  []interface{} `json:"ServiceAccount List"`
			ServiceAccountWatch []interface{} `json:"ServiceAccount Watch"`
		} `json:"Kubernetes Service Account"`
		KubernetesStack struct {
			StackGet   []interface{} `json:"Stack Get"`
			StackList  []interface{} `json:"Stack List"`
			StackWatch []interface{} `json:"Stack Watch"`
		} `json:"Kubernetes Stack"`
		KubernetesStatefulSet struct {
			StatefulSetGet   []interface{} `json:"StatefulSet Get"`
			StatefulSetList  []interface{} `json:"StatefulSet List"`
			StatefulSetWatch []interface{} `json:"StatefulSet Watch"`
		} `json:"Kubernetes Stateful Set"`
		KubernetesStorageClass struct {
			StorageClassGet   []interface{} `json:"StorageClass Get"`
			StorageClassList  []interface{} `json:"StorageClass List"`
			StorageClassWatch []interface{} `json:"StorageClass Watch"`
		} `json:"Kubernetes Storage Class"`
		KubernetesThirdPartyResource struct {
			ThirdPartyResourceGet   []interface{} `json:"ThirdPartyResource Get"`
			ThirdPartyResourceList  []interface{} `json:"ThirdPartyResource List"`
			ThirdPartyResourceWatch []interface{} `json:"ThirdPartyResource Watch"`
		} `json:"Kubernetes Third Party Resource"`
		KubernetesUser struct {
			UserGet   []interface{} `json:"User Get"`
			UserList  []interface{} `json:"User List"`
			UserWatch []interface{} `json:"User Watch"`
		} `json:"Kubernetes User"`
		Network struct {
			NetworkView []interface{} `json:"Network View"`
		} `json:"Network"`
		Node struct {
			NodeView []interface{} `json:"Node View"`
		} `json:"Node"`
		Secret struct {
			SecretView []interface{} `json:"Secret View"`
		} `json:"Secret"`
		Service struct {
			ServiceLogs      []interface{} `json:"Service Logs"`
			ServiceView      []interface{} `json:"Service View"`
			ServiceViewTasks []interface{} `json:"Service View Tasks"`
		} `json:"Service"`
		Volume struct {
			VolumeView []interface{} `json:"Volume View"`
		} `json:"Volume"`
	} `json:"operations"`
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// END: UCP objects
//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// RestClient is a simple REST API client
type RestClient struct {
	BearerToken string
	Host        string
	Secure      bool
	Port        int
}

func (o *RestClient) protocol() string {
	if o.Secure {
		return "https"
	}
	return "http"
}

// Get performs an HTTP GET operation
func (o *RestClient) Get(query string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	myClient := &http.Client{Transport: tr}

	url := fmt.Sprintf("%v://%v:%v/%v", o.protocol(), o.Host, o.Port, query[1:])
	req, reqErr := http.NewRequest("GET", url, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	if o.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.BearerToken)
	}
	req.Header.Set("Accept", "application/json")
	log.WithFields(log.Fields{
		"URL":     url,
		"Headers": req.Header,
		"Content": req,
	}).Debug("GET Request.")

	res, resErr := myClient.Do(req)
	if resErr != nil {
		return nil, resErr
	}
	defer res.Body.Close()
	log.WithFields(log.Fields{
		"URL":     url,
		"Status":  res.Status,
		"Headers": res.Header,
		"Content": res,
	}).Debug("GET Response.")

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	log.WithFields(log.Fields{
		"Body": string(body),
	}).Debug("GET Response body.")

	return body, nil
}

// Put performs an HTTP PUT operation
func (o *RestClient) Put(query string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	myClient := &http.Client{Transport: tr}

	url := fmt.Sprintf("%v://%v:%v/%v", o.protocol(), o.Host, o.Port, query[1:])
	req, reqErr := http.NewRequest("PUT", url, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	if o.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.BearerToken)
	}
	req.Header.Set("Accept", "application/json")
	log.WithFields(log.Fields{
		"URL":     url,
		"Headers": req.Header,
		"Content": req,
	}).Debug("PUT Request.")

	res, resErr := myClient.Do(req)
	if resErr != nil {
		return nil, resErr
	}
	defer res.Body.Close()
	log.WithFields(log.Fields{
		"URL":     url,
		"Status":  res.Status,
		"Headers": res.Header,
		"Content": res,
	}).Debug("PUT Response.")

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	log.WithFields(log.Fields{
		"Body": string(body),
	}).Debug("PUT Response body.")

	return body, nil
}

// Delete performs an HTTP DELETE operation
func (o *RestClient) Delete(query string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	myClient := &http.Client{Transport: tr}

	url := fmt.Sprintf("%v://%v:%v/%v", o.protocol(), o.Host, o.Port, query[1:])
	req, reqErr := http.NewRequest("DELETE", url, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	if o.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.BearerToken)
	}
	req.Header.Set("Accept", "application/json")
	log.WithFields(log.Fields{
		"URL":     url,
		"Headers": req.Header,
		"Content": req,
	}).Debug("DELETE Request.")

	res, resErr := myClient.Do(req)
	if resErr != nil {
		return nil, resErr
	}
	defer res.Body.Close()
	log.WithFields(log.Fields{
		"URL":     url,
		"Status":  res.Status,
		"Headers": res.Header,
		"Content": res,
	}).Debug("DELETE Response.")

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	log.WithFields(log.Fields{
		"Body": string(body),
	}).Debug("DELETE Reponse body.")

	return body, nil
}

// PostForm performs an HTTP POST of 'application/x-www-form-urlencoded' data
func (o *RestClient) PostForm(query, jsonData string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	myClient := &http.Client{Transport: tr}

	url := fmt.Sprintf("%v://%v:%v/%v", o.protocol(), o.Host, o.Port, query[1:])
	req, reqErr := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonData)))
	if reqErr != nil {
		return nil, reqErr
	}
	if o.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.BearerToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	log.WithFields(log.Fields{
		"URL":     url,
		"Headers": req.Header,
		"Content": req,
	}).Debug("POST Request.")

	res, resErr := myClient.Do(req)
	if resErr != nil {
		return nil, resErr
	}
	defer res.Body.Close()
	log.WithFields(log.Fields{
		"URL":     url,
		"Status":  res.Status,
		"Headers": res.Header,
		"Content": res,
	}).Debug("POST Response.")

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	log.WithFields(log.Fields{
		"Body": string(body),
	}).Debug("POST Response body.")

	return body, nil
}

type Interface interface {
	GetRoleIDForServiceAccount(serviceAccount string) (string, error)
	GetRolesForRoleID(roleID string) (*UCPRole, error)
	GetRoles() ([]UCPRole, error)
	GetRolesForServiceAccount(serviceAccount string) (*UCPRole, error)
	FindRoleByName(ucpRoleName string) (*UCPRole, error)
	DeleteRoleByName(ucpRoleName string) (bool, error)
	CreateTridentRole() (bool, error)
	DeleteTridentRole() (bool, error)
	AddTridentRoleToServiceAccount(TridentPodNamespace string) (bool, error)
	RemoveTridentRoleFromServiceAccount(TridentPodNamespace string) (bool, error)
}

// UCP is an object for interacting with Docker's UCP API
type UCP struct {
	client *RestClient
}

// GetRoleIDForServiceAccount returns the Role ID for the supplied service account
func (o *UCP) GetRoleIDForServiceAccount(serviceAccount string) (string, error) {
	if serviceAccount == "" {
		return "", fmt.Errorf("error in GetRoleIDForServiceAccount, serviceAccount not specified")
	}

	// need to escape out the ':' characters
	body, err := o.client.Get(fmt.Sprintf("/collectionGrants?subjectID=%v", url.QueryEscape(serviceAccount)))
	if err != nil {
		return "", err
	}

	collectionGrants := &UCPCollectionGrants{}
	unmarshalError := json.Unmarshal(body, collectionGrants)
	if unmarshalError != nil {
		return "", unmarshalError
	}

	if collectionGrants != nil {
		if collectionGrants.Grants != nil {
			return collectionGrants.Grants[0].RoleID, nil
		}
	}

	return "", fmt.Errorf("could not determine Role ID for %v", serviceAccount)
}

// GetRolesForRoleID returns all the Roles for the supplied Role ID
func (o *UCP) GetRolesForRoleID(roleID string) (*UCPRole, error) {
	body, err := o.client.Get(fmt.Sprintf("/roles/%v", roleID))
	if err != nil {
		return nil, err
	}

	ucpRoles := &UCPRole{}
	unmarshalError := json.Unmarshal(body, ucpRoles)
	if unmarshalError != nil {
		return nil, unmarshalError
	}

	if ucpRoles != nil {
		return ucpRoles, nil
	}

	return nil, fmt.Errorf("could not determine Roles for Role ID %v", roleID)
}

// GetRoles returns all the UCP roles
func (o *UCP) GetRoles() ([]UCPRole, error) {
	body, err := o.client.Get(fmt.Sprintf("/roles"))
	if err != nil {
		return nil, err
	}

	ucpRoles := make([]UCPRole, 0)
	unmarshalError := json.Unmarshal(body, &ucpRoles)
	if unmarshalError != nil {
		return nil, unmarshalError
	}

	if ucpRoles != nil {
		return ucpRoles, nil
	}

	return nil, fmt.Errorf("could not determine Roles")
}

// GetRolesForServiceAccount retrieves the UCP Roles for the supplied Kubernetes service account
func (o *UCP) GetRolesForServiceAccount(serviceAccount string) (*UCPRole, error) {
	if serviceAccount == "" {
		return nil, fmt.Errorf("error in GetRolesForServiceAccount, serviceAccount not specified")
	}

	roleID, clientError := o.GetRoleIDForServiceAccount(serviceAccount)
	log.WithFields(log.Fields{
		"roleID":      roleID,
		"clientError": clientError,
	}).Debug("GetRoleIDForServiceAccount result.")

	if clientError == nil {
		return o.GetRolesForRoleID(roleID)
	}
	return nil, clientError
}

// FindRoleByName looks up the UCP Role object by name
func (o *UCP) FindRoleByName(ucpRoleName string) (*UCPRole, error) {

	foundUcpRole := false
	var ucpRole *UCPRole

	allRoles, clientError := o.GetRoles()
	log.WithFields(log.Fields{
		"ucpRoles":    allRoles,
		"clientError": clientError,
	}).Debug("GetRoles result.")

	if clientError != nil {
		return nil, clientError
	}

	for i, role := range allRoles {
		log.WithFields(log.Fields{
			"role.ID":     role.ID,
			"role.Name":   role.Name,
			"ucpRoleName": ucpRoleName,
		}).Debug("Comparing.")

		if strings.Compare(role.Name, ucpRoleName) == 0 {
			ucpRole = &allRoles[i] // do not use &role from range, it's effectively the last range value (not what you want!)
			foundUcpRole = true
			log.WithFields(log.Fields{
				"Role": ucpRoleName,
				"ID":   ucpRole.ID,
			}).Debug("Found.")
		}
	}

	if foundUcpRole {
		log.WithFields(log.Fields{
			"Role": ucpRole.Name,
			"ID":   ucpRole.ID,
		}).Debug("Using.")
	}

	return ucpRole, nil
}

// DeleteRoleByName looks up the UCP Role object by name and deletes it
func (o *UCP) DeleteRoleByName(ucpRoleName string) (bool, error) {
	ucpRole, clientError := o.FindRoleByName(ucpRoleName)
	if clientError != nil {
		return false, clientError
	}
	if ucpRole == nil {
		return false, fmt.Errorf("could not find UCP Role %v", ucpRoleName)
	}

	body, err := o.client.Delete(fmt.Sprintf("/roles/%v", ucpRole.ID))
	log.WithFields(log.Fields{
		"body": string(body),
		"err":  err,
	}).Debug("DeleteRoleByName result.")

	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateTridentRole creates Trident's UCP role
func (o *UCP) CreateTridentRole() (bool, error) {
	ucpTridentRoleName := defaultTridentRole
	jsonString := GetUCPRoleCreateJSON(ucpTridentRoleName)

	// verify what we have can be encoded into JSON
	ucpRole := &UCPRole{}
	unmarshalError := json.Unmarshal([]byte(jsonString), ucpRole)
	if unmarshalError != nil {
		return false, unmarshalError
	}

	_, err := o.client.PostForm("/roles", jsonString)
	if err != nil {
		return false, err
	}

	return true, nil
}

// DeleteTridentRole delete Trident's UCP role
func (o *UCP) DeleteTridentRole() (bool, error) {
	return o.DeleteRoleByName(defaultTridentRole)
}

func (o *UCP) getServiceAccountForNamespace(TridentPodNamespace string) string {
	serviceAccount := fmt.Sprintf("system:serviceaccount:%s:trident", TridentPodNamespace)
	return serviceAccount
}

// AddTridentRoleToServiceAccount adds the role to the account
func (o *UCP) AddTridentRoleToServiceAccount(TridentPodNamespace string) (bool, error) {
	ucpRoleName := defaultTridentRole
	serviceAccount := url.QueryEscape(o.getServiceAccountForNamespace(TridentPodNamespace))

	// lookup the UCP Role so we can get its ID
	ucpRole, clientError := o.FindRoleByName(ucpRoleName)
	if clientError != nil {
		return false, clientError
	}
	if ucpRole == nil {
		return false, fmt.Errorf("could not find UCP Role %v", ucpRoleName)
	}

	// need to escape out the ':' characters
	_, err := o.client.Put(fmt.Sprintf("/collectionGrants/%v/kubernetesnamespaces/%v?type=grantobject", serviceAccount, ucpRole.ID))
	if err != nil {
		return false, err
	}

	return true, nil
}

// RemoveTridentRoleFromServiceAccount removes the role from the service account
func (o *UCP) RemoveTridentRoleFromServiceAccount(TridentPodNamespace string) (bool, error) {
	ucpRoleName := defaultTridentRole
	serviceAccount := url.QueryEscape(o.getServiceAccountForNamespace(TridentPodNamespace))

	// lookup the UCP Role so we can get its ID
	ucpRole, clientError := o.FindRoleByName(ucpRoleName)
	if clientError != nil {
		return false, clientError
	}
	if ucpRole == nil {
		return false, fmt.Errorf("could not find UCP Role %v", ucpRoleName)
	}

	// need to escape out the ':' characters
	_, err := o.client.Delete(fmt.Sprintf("/collectionGrants/%v/kubernetesnamespaces/%v?type=grantobject", serviceAccount, ucpRole.ID))
	if err != nil {
		return false, err
	}

	return true, nil
}

// NewClient factory method to create a new client object for use against a UCP server
func NewClient(ucpHost, bearerToken string) (*UCP, error) {
	if ucpHost == "" || bearerToken == "" {
		return nil, fmt.Errorf("ucp-bearer-token and ucp-host must BOTH be specified")
	}
	ucpClient := &UCP{
		client: &RestClient{
			BearerToken: bearerToken,
			Host:        ucpHost,
			Secure:      true,
			Port:        443,
		},
	}
	return ucpClient, nil
}
