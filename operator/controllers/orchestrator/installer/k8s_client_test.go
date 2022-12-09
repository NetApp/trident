// Copyright 2022 NetApp, Inc. All Rights Reserved.

package installer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/utils"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

// JSONMatcher compares two byte arrays that represent JSON which may contain the same text with different encodings.
type JSONMatcher struct {
	expected []byte
}

// Matches takes a []byte as an interface, attempts to unmarshal it,
// then compares it using DeepEqual with an expected []byte field
func (m *JSONMatcher) Matches(x interface{}) bool {
	var actual, expected interface{}

	actualBytes, ok := x.([]byte)
	if !ok {
		return false
	}
	if err := json.Unmarshal(actualBytes, &actual); err != nil {
		return false
	}

	if err := json.Unmarshal(m.expected, &expected); err != nil {
		return false
	}

	return reflect.DeepEqual(actual, expected)
}

func (m *JSONMatcher) String() string {
	return ""
}

func TestCreateCustomResourceDefinition(t *testing.T) {
	crdName := "crd-name"
	crdYAML := "crd-yaml"
	k8sClientErr := fmt.Errorf("k8s client err")

	type input struct {
		crdName string
		crdYAML string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string)
	}{
		"expect to fail with k8s error": {
			input: input{
				crdName: crdName,
				crdYAML: crdYAML,
			},
			output: fmt.Errorf("could not create CRD %s; err: %v", crdName, k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string) {
				mockKubeClient.EXPECT().CreateObjectByYAML(crdYAML).Return(k8sClientErr)
			},
		},
		"expect to pass with no k8s error": {
			input: input{
				crdName: crdName,
				crdYAML: crdYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string) {
				mockKubeClient.EXPECT().CreateObjectByYAML(crdYAML).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input and output variables from the test case definition
				crdName, crdYAML := test.input.crdName, test.input.crdYAML
				expectedErr := test.output

				test.mocks(mockKubeClient, crdYAML)
				extendedK8sClient := &K8sClient{mockKubeClient}

				actualErr := extendedK8sClient.CreateCustomResourceDefinition(crdName, crdYAML)
				assert.Equal(t, actualErr, expectedErr)
			},
		)
	}
}

func TestPutCustomResourceDefinition(t *testing.T) {
	k8sClientErr := fmt.Errorf("k8s client err")
	crdName := "trident.netapp.io"
	crdYAML := `
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: trident.netapp.io
spec:
  group: trident.netapp.io
  versions:
    - name: v1
      served: true
      schema:
          openAPIV3Schema:
              type: object
              nullable: false`
	crd := &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &v1.JSONSchemaProps{
							Type:     "object",
							Nullable: false,
						},
					},
				},
			},
		},
	}

	type input struct {
		currentCRD *v1.CustomResourceDefinition
		createCRD  bool
		newCRDYAML string
		crdName    string
	}

	type output struct {
		errorExpected bool
	}

	tests := map[string]struct {
		input  input
		output output
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error when CRD needs created": {
			input: input{
				currentCRD: crd.DeepCopy(),
				createCRD:  true,
				newCRDYAML: crdYAML,
				crdName:    crdName,
			},
			output: output{
				errorExpected: true,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// args[0] = currentCRD, args[1] = createCRD, args[2] = newCRDYAML, args[3] = crdName
				crdYAML, _ := args[2].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(crdYAML).Return(k8sClientErr)
			},
		},
		"expect to fail with k8s error while waiting for the CRD to be established": {
			input: input{
				currentCRD: crd.DeepCopy(),
				createCRD:  true,
				newCRDYAML: crdYAML,
				crdName:    crdName,
			},
			output: output{
				errorExpected: true,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// args[0] = currentCRD, args[1] = createCRD, args[2] = newCRDYAML, args[3] = crdName
				crdYAML, _ := args[2].(string)
				crdName, _ := args[3].(string)
				k8sTimeout = 5 * time.Millisecond // set the timeout so the test doesn't hang
				mockKubeClient.EXPECT().CreateObjectByYAML(crdYAML).Return(nil)
				mockKubeClient.EXPECT().GetCRD(crdName).Return(nil, k8sClientErr).AnyTimes()
				mockKubeClient.EXPECT().DeleteObjectByYAML(crdYAML, false).Return(k8sClientErr)
			},
		},
		"expect to fail with k8s error while attempting to patch an existing CRD": {
			input: input{
				currentCRD: crd.DeepCopy(),
				createCRD:  false,
				newCRDYAML: crdYAML,
				crdName:    crdName,
			},
			output: output{
				errorExpected: true,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// args[0] = currentCRD, args[1] = createCRD, args[2] = newCRDYAML, args[3] = crdName
				crd := args[0].(*v1.CustomResourceDefinition)
				crdYAML, _ := args[2].(string)
				crdName, _ := args[3].(string)
				patchType := types.MergePatchType

				// Generate the deltas between the currentCRD and the new CRD YAML.
				patchBytes, err := k8sclient.GenericPatch(crd, []byte(crdYAML))
				if err != nil {
					t.Fatalf("can't generate GenericPatch")
				}
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchCRD(crdName, patchBytesMatcher, patchType).Return(k8sClientErr)
			},
		},
		"expect to pass with no k8s error while attempting to patch an existing CRD": {
			input: input{
				currentCRD: crd.DeepCopy(),
				createCRD:  false,
				newCRDYAML: crdYAML,
				crdName:    crdName,
			},
			output: output{
				errorExpected: false,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// args[0] = currentCRD, args[1] = createCRD, args[2] = newCRDYAML, args[3] = crdName
				crd := args[0].(*v1.CustomResourceDefinition)
				crdYAML, _ := args[2].(string)
				crdName, _ := args[3].(string)
				patchType := types.MergePatchType

				// Generate the deltas between the currentCRD and the new CRD YAML.
				patchBytes, err := k8sclient.GenericPatch(crd, []byte(crdYAML))
				if err != nil {
					t.Fatalf("can't generate GenericPatch")
				}
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchCRD(crdName, patchBytesMatcher, patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// Setup mock controller and kube client.
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// Extract the input and output variables from the test case definition.
				currentCRD, createCRD, newCRDYAML, crdName := test.input.currentCRD, test.input.createCRD,
					test.input.newCRDYAML, test.input.crdName

				errorExpected := test.output.errorExpected

				test.mocks(mockKubeClient, currentCRD, createCRD, newCRDYAML, crdName)
				extendedK8sClient := &K8sClient{mockKubeClient}

				actualErr := extendedK8sClient.PutCustomResourceDefinition(currentCRD, crdName, createCRD, newCRDYAML)

				if errorExpected {
					assert.NotNil(t, actualErr, "expected non-nil error")
				} else {
					assert.Nil(t, actualErr, "expected nil error")
				}
			},
		)
	}
}

func TestDeleteCustomResourceDefinition(t *testing.T) {
	crdName := "crd-name"
	crdYAML := "crd-yaml"
	k8sClientErr := fmt.Errorf("k8s client err")

	type input struct {
		crdName string
		crdYAML string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string)
	}{
		"expect to fail with k8s error": {
			input: input{
				crdName: crdName,
				crdYAML: crdYAML,
			},
			output: fmt.Errorf("could not delete CRD %s; err: %v", crdName, k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string) {
				mockKubeClient.EXPECT().DeleteObjectByYAML(crdYAML, false).Return(k8sClientErr)
			},
		},
		"expect to pass with no k8s error": {
			input: input{
				crdName: crdName,
				crdYAML: crdYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdYAML string) {
				mockKubeClient.EXPECT().DeleteObjectByYAML(crdYAML, false).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input and output variables from the test case definition
				crdName, crdYAML := test.input.crdName, test.input.crdYAML
				expectedErr := test.output

				test.mocks(mockKubeClient, crdYAML)
				extendedK8sClient := &K8sClient{mockKubeClient}

				actualErr := extendedK8sClient.DeleteCustomResourceDefinition(crdName, crdYAML)
				assert.Equal(t, actualErr, expectedErr)
			},
		)
	}
}

func TestWaitForCRDEstablished(t *testing.T) {
	var validCRD, invalidCRD *v1.CustomResourceDefinition

	// setup mock CRD objects to test with
	validCRD = &v1.CustomResourceDefinition{
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Type: v1.Established,
					// this should allow the function to return nil
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	invalidCRD = &v1.CustomResourceDefinition{
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Type: v1.Terminating,
					// this should allow the function to return nil
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	// setup variables used through tests
	versionCRDName := "trident-crd"
	timeout := 5 * time.Millisecond

	backoffRetryErr := fmt.Errorf("CRD was not established after %3.2f seconds", timeout.Seconds())

	type input struct {
		crdName string
		timeout time.Duration
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdName string)
	}{
		"expect to fail with k8s error during GetCRD": {
			input: input{
				crdName: versionCRDName,
				timeout: timeout,
			},
			output: fmt.Errorf("CRD was not established after %3.2f seconds", timeout.Seconds()),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdName string) {
				// mock calls here
				mockKubeClient.EXPECT().GetCRD(crdName).Return(nil, fmt.Errorf("any k8s error")).MinTimes(1)
			},
		},
		"expect to fail when unexpected crd conditions are found": {
			input: input{
				crdName: versionCRDName,
				timeout: timeout,
			},
			output: backoffRetryErr,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdName string) {
				// mock calls here
				mockKubeClient.EXPECT().GetCRD(crdName).Return(invalidCRD, nil).AnyTimes()
			},
		},
		"expect to pass when expected crd conditions are found": {
			input: input{
				crdName: versionCRDName,
				timeout: timeout,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, crdName string) {
				// mock calls here
				mockKubeClient.EXPECT().GetCRD(crdName).Return(validCRD, nil).AnyTimes()
			},
		},
	}
	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input and output variables from the test case definition
				crdName, timeout := test.input.crdName, test.input.timeout
				expectedErr := test.output

				test.mocks(mockKubeClient, crdName)
				extendedK8sClient := &K8sClient{mockKubeClient}

				actualErr := extendedK8sClient.WaitForCRDEstablished(crdName, timeout)
				assert.Equal(t, actualErr, expectedErr)
			},
		)
	}
}

func TestDeleteTransientVersionPod(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var versionPodLabel, name, invalidName, namespace string
	var validVersionPod, invalidVersionPod corev1.Pod
	var emptyVersionPodList, unwantedVersionPods []corev1.Pod

	versionPodLabel = "version-pod-label"
	name = "transient-version-pod"
	namespace = "default"
	invalidName = ""
	validVersionPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	invalidVersionPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}

	unwantedVersionPods = []corev1.Pod{
		invalidVersionPod,
		validVersionPod,
		invalidVersionPod,
	}

	k8sClientErr := fmt.Errorf("unable to get list of version pods")
	removeMultiplePodsErr := fmt.Errorf("unable to delete pod(s): %v", []string{
		fmt.Sprintf("%v/%v", invalidVersionPod.Namespace, invalidVersionPod.Name),
		fmt.Sprintf("%v/%v", invalidVersionPod.Namespace, invalidVersionPod.Name),
	})

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  string
		output error
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, versionPodLabel string)
	}{
		"expect to fail with k8s error during GetPodsByLabel": {
			input:  versionPodLabel,
			output: k8sClientErr,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, versionPodLabel string) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodsByLabel(versionPodLabel, true).Return(nil, fmt.Errorf("any client err"))
			},
		},
		"expect to pass with no k8s error and no transient pods found": {
			input:  versionPodLabel,
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, versionPodLabel string) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodsByLabel(versionPodLabel, true).Return(emptyVersionPodList, nil)
			},
		},
		"expect to fail with no k8s error and transient pods found but not removed": {
			input:  versionPodLabel,
			output: removeMultiplePodsErr,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, versionPodLabel string) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodsByLabel(versionPodLabel, true).Return(unwantedVersionPods, nil)

				// the invalid version pod names and namespaces will be in the returned error / output
				firstInvalidVersionPod := unwantedVersionPods[0]
				validVersionPod := unwantedVersionPods[1]
				secondInvalidVersionPod := unwantedVersionPods[2]
				mockKubeClient.EXPECT().DeletePod(firstInvalidVersionPod.Name,
					firstInvalidVersionPod.Namespace).Return(fmt.Errorf("k8s client error"))
				mockKubeClient.EXPECT().DeletePod(validVersionPod.Name,
					validVersionPod.Namespace).Return(nil)
				mockKubeClient.EXPECT().DeletePod(secondInvalidVersionPod.Name,
					secondInvalidVersionPod.Namespace).Return(fmt.Errorf("k8s client error"))
			},
		},
		"expect to pass with no k8s error and transient pods found and removed": {
			input:  versionPodLabel,
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, versionPodLabel string) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodsByLabel(versionPodLabel, true).Return([]corev1.Pod{validVersionPod}, nil)

				// the invalid version pod names and namespaces will be in the returned error / output
				mockKubeClient.EXPECT().DeletePod(validVersionPod.Name, validVersionPod.Namespace).Return(nil)
			},
		},
	}
	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input and output variables from the test case definition
				versionPodLabel, expectedErr := test.input, test.output

				test.mocks(mockKubeClient, versionPodLabel)
				extendedK8sClient := &K8sClient{mockKubeClient}

				actualErr := extendedK8sClient.DeleteTransientVersionPod(versionPodLabel)
				assert.Equal(t, actualErr, expectedErr)
			},
		)
	}
}

func TestGetCSIDriverInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validCSIDriver, invalidCSIDriver storagev1.CSIDriver
	var unwantedCSIDrivers, combinationCSIDrivers []storagev1.CSIDriver

	label = "tridentCSILabel"
	name = getCSIDriverName() // could be anything
	namespace = "default"
	invalidName = ""

	validCSIDriver = storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidCSIDriver = storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedCSIDrivers = []storagev1.CSIDriver{invalidCSIDriver, invalidCSIDriver}
	combinationCSIDrivers = append([]storagev1.CSIDriver{validCSIDriver}, append(combinationCSIDrivers,
		unwantedCSIDrivers...)...)

	// setup input and output test types for easy use
	type input struct {
		name         string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentCSIDriver  *storagev1.CSIDriver
		unwantedCSIDriver []storagev1.CSIDriver
		createCSIDriver   bool
		err               error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label, args[1] = name, args[2] = namespace
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: true,
			},
			output: output{
				currentCSIDriver:  nil,
				unwantedCSIDriver: nil,
				createCSIDriver:   true,
				err:               fmt.Errorf("unable to get list of CSI driver custom resources by label"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetCSIDriversByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no beta CSI driver found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				shouldUpdate: false,
			},
			output: output{
				currentCSIDriver:  nil,
				unwantedCSIDriver: nil,
				createCSIDriver:   true,
				err:               nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetCSIDriversByLabel(args[0]).Return([]storagev1.CSIDriver{}, nil)
				mockKubeClient.EXPECT().DeleteCSIDriver(args[1]).Return(nil)
			},
		},
		"expect to pass with valid current beta CSI driver found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: false,
			},
			output: output{
				currentCSIDriver:  &validCSIDriver,
				unwantedCSIDriver: unwantedCSIDrivers,
				createCSIDriver:   false,
				err:               nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetCSIDriversByLabel(args[0]).Return(combinationCSIDrivers, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, shouldUpdate := test.input.label, test.input.name, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCSIDriver, expectedUnwantedCSIDrivers, expectedCreateCSIDriver,
					expectedErr := test.output.currentCSIDriver, test.output.unwantedCSIDriver,
					test.output.createCSIDriver, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, name, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentCSIDriver, unwantedCSIDrivers, createCSIDriver, err := extendedK8sClient.GetCSIDriverInformation(name,
					label, shouldUpdate)

				assert.EqualValues(t, expectedCSIDriver, currentCSIDriver)
				assert.EqualValues(t, expectedUnwantedCSIDrivers, unwantedCSIDrivers)
				assert.Equal(t, len(expectedUnwantedCSIDrivers), len(unwantedCSIDrivers))
				assert.Equal(t, expectedCreateCSIDriver, createCSIDriver)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutCSIDriver(t *testing.T) {
	var validCSIDriver *storagev1.CSIDriver

	driverName := getCSIDriverName()
	validCSIDriver = &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: driverName,
		},
	}
	appLabel := TridentCSILabel
	newCSIDriverYAML := k8sclient.GetCSIDriverYAML(driverName, make(map[string]string), make(map[string]string))
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentCSIDriver *storagev1.CSIDriver
		createCSIDriver  bool
		newCSIDriverYAML string
		appLabel         string
		patchBytes       []byte
		patchType        types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newCSIDriverYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a CSI Driver and no k8s error occurs": {
			input: input{
				currentCSIDriver: nil,
				createCSIDriver:  true,
				newCSIDriverYAML: newCSIDriverYAML,
				appLabel:         appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a CSI Driver and a k8s error occurs": {
			input: input{
				currentCSIDriver: nil,
				createCSIDriver:  true,
				newCSIDriverYAML: newCSIDriverYAML,
				appLabel:         appLabel,
			},
			output: fmt.Errorf("could not create CSI driver; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to fail when updating a CSI Driver and a k8s error occurs": {
			input: input{
				currentCSIDriver: validCSIDriver,
				createCSIDriver:  false,
				newCSIDriverYAML: newCSIDriverYAML,
				appLabel:         appLabel,
			},
			output: fmt.Errorf("could not patch CSI driver; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchCSIDriverByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a CSI Driver and no k8s error occurs": {
			input: input{
				currentCSIDriver: &storagev1.CSIDriver{
					ObjectMeta: metav1.ObjectMeta{
						Name: driverName,
					},
				},
				createCSIDriver:  false,
				newCSIDriverYAML: newCSIDriverYAML,
				appLabel:         appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchCSIDriverByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentCSIDriver, createCSIDriver, newCSIDriverYAML, appLabel := test.input.currentCSIDriver,
					test.input.createCSIDriver, test.input.newCSIDriverYAML, test.input.appLabel

				if !createCSIDriver {
					if patchBytes, err = k8sclient.GenericPatch(currentCSIDriver,
						[]byte(newCSIDriverYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newCSIDriverYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutCSIDriver(currentCSIDriver, createCSIDriver, newCSIDriverYAML, appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentCSIDriverCR(t *testing.T) {
	// arrange variables for the tests
	var emptyCSIDriverList, unwantedCSIDrivers []storagev1.CSIDriver
	var undeletedCSIDrivers []string

	getCSIDriversErr := fmt.Errorf("unable to get list of CSI driver CRs by label")
	csiDriverName := "csiDriverName"
	appLabel := "appLabel"

	unwantedCSIDrivers = []storagev1.CSIDriver{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: csiDriverName,
			},
		},
	}

	for _, csiDriver := range unwantedCSIDrivers {
		undeletedCSIDrivers = append(undeletedCSIDrivers, fmt.Sprintf("%v", csiDriver.Name))
	}

	type input struct {
		csiDriverName string
		appLabel      string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetCSIDriversByLabel fails": {
			input: input{
				csiDriverName: csiDriverName,
				appLabel:      appLabel,
			},
			output: getCSIDriversErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetCSIDriversByLabel(appLabel).Return(nil, getCSIDriversErr)
			},
		},
		"expect to pass when GetCSIDriversByLabel returns no services": {
			input: input{
				csiDriverName: csiDriverName,
				appLabel:      appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// return an empty list and no error here
				mockK8sClient.EXPECT().GetCSIDriversByLabel(appLabel).Return(emptyCSIDriverList, nil)
				mockK8sClient.EXPECT().DeleteCSIDriver(csiDriverName).Return(nil)
			},
		},
		"expect to fail when GetCSIDriversByLabel succeeds but RemoveMultipleCSIDriverCRs fails": {
			input: input{
				csiDriverName: csiDriverName,
				appLabel:      appLabel,
			},
			output: fmt.Errorf("unable to delete CSI driver CR(s): %v", undeletedCSIDrivers),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetCSIDriversByLabel(appLabel).Return(unwantedCSIDrivers, nil)
				mockK8sClient.EXPECT().DeleteCSIDriver(csiDriverName).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedCSIDrivers))
			},
		},
		"expect to pass when GetCSIDriversByLabel succeeds and RemoveMultipleCSIDriverCRs succeeds": {
			input: input{
				csiDriverName: csiDriverName,
				appLabel:      appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetCSIDriversByLabel(appLabel).Return(unwantedCSIDrivers, nil)
				mockK8sClient.EXPECT().DeleteCSIDriver(csiDriverName).Return(nil).
					MaxTimes(len(unwantedCSIDrivers))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				driverName, appLabel := test.input.csiDriverName, test.input.appLabel
				err := extendedK8sClient.DeleteCSIDriverCR(driverName, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleCSIDriverCRs(t *testing.T) {
	// arrange variables for the tests
	var emptyCSIDriversCRs, undeletedCSIDriverCRs, unwantedCSIDriverCRs []storagev1.CSIDriver
	var undeletedCSIDriverCRNames []string

	undeletedCSIDriverCRErr := fmt.Errorf("could not delete CSI CR")
	undeletedCSIDriverName := "undeletedCSIDriver"
	undeletedCSIDriverCRs = []storagev1.CSIDriver{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedCSIDriverName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedCSIDriverName,
			},
		},
	}

	for _, cr := range undeletedCSIDriverCRs {
		undeletedCSIDriverCRNames = append(undeletedCSIDriverCRNames, cr.Name)
	}

	validCSIDRiverName := "csiDriverCRName"
	unwantedCSIDriverCRs = []storagev1.CSIDriver{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: validCSIDRiverName,
			},
		},
	}

	tests := map[string]struct {
		input  []storagev1.CSIDriver
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no csi driver crs": {
			input:  emptyCSIDriversCRs,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedCSIDriverCRs,
			output: fmt.Errorf("unable to delete CSI driver CR(s): %v", undeletedCSIDriverCRNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteCSIDriver(undeletedCSIDriverName).Return(undeletedCSIDriverCRErr).
					MaxTimes(len(undeletedCSIDriverCRs))
			},
		},
		"expect to pass with valid csi driver crs": {
			input:  unwantedCSIDriverCRs,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteCSIDriver(validCSIDRiverName).Return(nil).
					MaxTimes(len(unwantedCSIDriverCRs))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleCSIDriverCRs(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetClusterRoleInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validClusterRole, invalidClusterRole rbacv1.ClusterRole
	var unwantedClusterRoles, combinationClusterRoles, emptyClusterRoles []rbacv1.ClusterRole

	label = "tridentCSILabel"
	name = getControllerRBACResourceName(true) // could be anything
	namespace = "default"
	invalidName = ""

	validClusterRole = rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidClusterRole = rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedClusterRoles = []rbacv1.ClusterRole{invalidClusterRole, invalidClusterRole}
	combinationClusterRoles = append([]rbacv1.ClusterRole{validClusterRole}, append(combinationClusterRoles,
		unwantedClusterRoles...)...)

	// setup input and output test types for easy use
	type input struct {
		name         string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentClusterRole   *rbacv1.ClusterRole
		unwantedClusterRoles []rbacv1.ClusterRole
		createClusterRole    bool
		err                  error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label, args[1] = name, args[2] = namespace
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: true,
			},
			output: output{
				currentClusterRole:   nil,
				unwantedClusterRoles: nil,
				createClusterRole:    true,
				err:                  fmt.Errorf("unable to get list of cluster roles"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetClusterRolesByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no cluster role found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				shouldUpdate: false,
			},
			output: output{
				currentClusterRole:   nil,
				unwantedClusterRoles: nil,
				createClusterRole:    true,
				err:                  nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetClusterRolesByLabel(args[0]).Return(emptyClusterRoles, nil)
				mockKubeClient.EXPECT().DeleteClusterRole(args[1]).Return(nil)
			},
		},
		"expect to pass with valid current cluster role found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: false,
			},
			output: output{
				currentClusterRole:   &validClusterRole,
				unwantedClusterRoles: unwantedClusterRoles,
				createClusterRole:    false,
				err:                  nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetClusterRolesByLabel(args[0]).Return(combinationClusterRoles, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, shouldUpdate := test.input.label, test.input.name, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedClusterRole, expectedUnwantedClusterRoles, expectedCreateClusterRole,
					expectedErr := test.output.currentClusterRole, test.output.unwantedClusterRoles,
					test.output.createClusterRole, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, name, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentClusterRole, unwantedClusterRoles, createClusterRole, err := extendedK8sClient.GetClusterRoleInformation(name,
					label, shouldUpdate)

				assert.EqualValues(t, expectedClusterRole, currentClusterRole)
				assert.EqualValues(t, expectedUnwantedClusterRoles, unwantedClusterRoles)
				assert.Equal(t, len(expectedUnwantedClusterRoles), len(unwantedClusterRoles))
				assert.Equal(t, expectedCreateClusterRole, createClusterRole)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutClusterRole(t *testing.T) {
	var validClusterRole *rbacv1.ClusterRole

	clusterRoleName := getControllerRBACResourceName(true)
	validClusterRole = &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
	}
	appLabel := TridentCSILabel
	newClusterRoleYAML := k8sclient.GetClusterRoleYAML(
		"",
		clusterRoleName,
		make(map[string]string),
		make(map[string]string),
		true,
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentClusterRole *rbacv1.ClusterRole
		createClusterRole  bool
		newClusterRoleYAML string
		appLabel           string
		patchBytes         []byte
		patchType          types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newClusterRoleYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a ClusterRole and no k8s error occurs": {
			input: input{
				currentClusterRole: nil,
				createClusterRole:  true,
				newClusterRoleYAML: newClusterRoleYAML,
				appLabel:           appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a ClusterRole and a k8s error occurs": {
			input: input{
				currentClusterRole: nil,
				createClusterRole:  true,
				newClusterRoleYAML: newClusterRoleYAML,
				appLabel:           appLabel,
			},
			output: fmt.Errorf("could not create cluster role; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to fail when updating a ClusterRole and a k8s error occurs": {
			input: input{
				currentClusterRole: validClusterRole,
				createClusterRole:  false,
				newClusterRoleYAML: newClusterRoleYAML,
				appLabel:           appLabel,
			},
			output: fmt.Errorf("could not patch Trident Cluster role; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchClusterRoleByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a ClusterRole and no k8s error occurs": {
			input: input{
				currentClusterRole: validClusterRole,
				createClusterRole:  false,
				newClusterRoleYAML: newClusterRoleYAML,
				appLabel:           appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchClusterRoleByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentClusterRole, createClusterRole, newClusterRoleYAML, appLabel := test.input.currentClusterRole,
					test.input.createClusterRole, test.input.newClusterRoleYAML, test.input.appLabel

				if !createClusterRole {
					if patchBytes, err = k8sclient.GenericPatch(currentClusterRole,
						[]byte(newClusterRoleYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newClusterRoleYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutClusterRole(currentClusterRole, createClusterRole, newClusterRoleYAML,
					appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentClusterRole(t *testing.T) {
	// arrange variables for the tests
	var emptyClusterRoleList, unwantedClusterRoles []rbacv1.ClusterRole
	var undeletedClusterRoles []string

	getClusterRoleErr := fmt.Errorf("unable to get list of cluster roles")
	clusterRoleName := "clusterRoleName"
	appLabel := "appLabel"

	unwantedClusterRoles = []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleName,
			},
		},
	}

	for _, clusterRole := range unwantedClusterRoles {
		undeletedClusterRoles = append(undeletedClusterRoles, fmt.Sprintf("%v", clusterRole.Name))
	}

	type input struct {
		clusterRoleName string
		appLabel        string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetClusterRolesByLabel fails": {
			input: input{
				clusterRoleName: clusterRoleName,
				appLabel:        appLabel,
			},
			output: getClusterRoleErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRolesByLabel(appLabel).Return(nil, getClusterRoleErr)
			},
		},
		"expect to pass when GetClusterRolesByLabel returns no cluster role bindings": {
			input: input{
				clusterRoleName: clusterRoleName,
				appLabel:        appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRolesByLabel(appLabel).Return(emptyClusterRoleList, nil)
				mockK8sClient.EXPECT().DeleteClusterRole(clusterRoleName).Return(nil)
			},
		},
		"expect to fail when GetClusterRolesByLabel succeeds but DeleteClusterRole fails": {
			input: input{
				clusterRoleName: clusterRoleName,
				appLabel:        appLabel,
			},
			output: fmt.Errorf("unable to delete cluster role(s): %v", undeletedClusterRoles),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRolesByLabel(appLabel).Return(unwantedClusterRoles, nil)
				mockK8sClient.EXPECT().DeleteClusterRole(clusterRoleName).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedClusterRoles))
			},
		},
		"expect to pass when GetClusterRolesByLabel succeeds and DeleteClusterRole succeeds": {
			input: input{
				clusterRoleName: clusterRoleName,
				appLabel:        appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRolesByLabel(appLabel).Return(unwantedClusterRoles, nil)
				mockK8sClient.EXPECT().DeleteClusterRole(clusterRoleName).Return(nil).
					MaxTimes(len(unwantedClusterRoles))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				clusterRoleName, appLabel := test.input.clusterRoleName, test.input.appLabel
				err := extendedK8sClient.DeleteTridentClusterRole(clusterRoleName, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleClusterRoles(t *testing.T) {
	// arrange variables for the tests
	var emptyClusterRoles, unwantedClusterRoles, undeletedClusterRoles []rbacv1.ClusterRole
	var undeletedClusterRoleNames []string

	undeletedClusterRoleErr := fmt.Errorf("could not delete cluster role")
	undeletedClusterRoleName := "undeletedClusterRoleName"
	undeletedClusterRoles = []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedClusterRoleName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedClusterRoleName,
			},
		},
	}

	for _, cr := range undeletedClusterRoles {
		undeletedClusterRoleNames = append(undeletedClusterRoleNames, cr.Name)
	}

	unwantedClusterRoleName := "unwantedClusterRoleName"
	unwantedClusterRoles = []rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: unwantedClusterRoleName,
			},
		},
	}

	tests := map[string]struct {
		input  []rbacv1.ClusterRole
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no cluster roles": {
			input:  emptyClusterRoles,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedClusterRoles,
			output: fmt.Errorf("unable to delete cluster role(s): %v", undeletedClusterRoleNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteClusterRole(undeletedClusterRoleName).Return(undeletedClusterRoleErr).
					MaxTimes(len(undeletedClusterRoles))
			},
		},
		"expect to pass with valid cluster roles": {
			input:  unwantedClusterRoles,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteClusterRole(unwantedClusterRoleName).Return(nil).
					MaxTimes(len(unwantedClusterRoles))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleClusterRoles(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetClusterRoleBindingInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validClusterRoleBinding, invalidClusterRoleBinding rbacv1.ClusterRoleBinding
	var unwantedClusterRoleBindings, combinationClusterRoleBindings, emptyClusterRoleBindings []rbacv1.ClusterRoleBinding

	label = "tridentCSILabel"
	name = getClusterRoleBindingName(true) // could be anything
	namespace = "default"
	invalidName = ""

	validClusterRoleBinding = rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidClusterRoleBinding = rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedClusterRoleBindings = []rbacv1.ClusterRoleBinding{invalidClusterRoleBinding, invalidClusterRoleBinding}
	combinationClusterRoleBindings = append([]rbacv1.ClusterRoleBinding{validClusterRoleBinding},
		append(combinationClusterRoleBindings,
			unwantedClusterRoleBindings...)...)

	// setup input and output test types for easy use
	type input struct {
		name         string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentClusterRoleBinding   *rbacv1.ClusterRoleBinding
		unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding
		createClusterRoleBinding    bool
		err                         error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label, args[1] = name, args[2] = namespace
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: true,
			},
			output: output{
				currentClusterRoleBinding:   nil,
				unwantedClusterRoleBindings: nil,
				createClusterRoleBinding:    true,
				err:                         fmt.Errorf("unable to get list of cluster role bindings"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetClusterRoleBindingsByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no cluster role binding found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				shouldUpdate: false,
			},
			output: output{
				currentClusterRoleBinding:   nil,
				unwantedClusterRoleBindings: nil,
				createClusterRoleBinding:    true,
				err:                         nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetClusterRoleBindingsByLabel(args[0]).Return(emptyClusterRoleBindings, nil)
				mockKubeClient.EXPECT().DeleteClusterRoleBinding(args[1]).Return(nil)
			},
		},
		"expect to pass with valid current cluster role binding found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				shouldUpdate: false,
			},
			output: output{
				currentClusterRoleBinding:   &validClusterRoleBinding,
				unwantedClusterRoleBindings: unwantedClusterRoleBindings,
				createClusterRoleBinding:    false,
				err:                         nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetClusterRoleBindingsByLabel(args[0]).Return(combinationClusterRoleBindings,
					nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, shouldUpdate := test.input.label, test.input.name, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentClusterRoleBinding, expectedUnwantedClusterRoleBindings,
					expectedCreateClusterRoleBindings,
					expectedErr := test.output.currentClusterRoleBinding, test.output.unwantedClusterRoleBindings,
					test.output.createClusterRoleBinding, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, name, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding,
					err := extendedK8sClient.GetClusterRoleBindingInformation(name,
					label, shouldUpdate)

				assert.EqualValues(t, expectedCurrentClusterRoleBinding, currentClusterRoleBinding)
				assert.EqualValues(t, expectedUnwantedClusterRoleBindings, unwantedClusterRoleBindings)
				assert.Equal(t, len(expectedUnwantedClusterRoleBindings), len(unwantedClusterRoleBindings))
				assert.Equal(t, expectedCreateClusterRoleBindings, createClusterRoleBinding)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutClusterRoleBinding(t *testing.T) {
	var validClusterRoleBinding *rbacv1.ClusterRoleBinding

	clusterRoleBindingName := getClusterRoleBindingName(true)
	validClusterRoleBinding = &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
		},
	}
	appLabel := TridentCSILabel
	newClusterRoleYAML := k8sclient.GetClusterRoleBindingYAML(
		"namespace",
		clusterRoleBindingName,
		"",
		make(map[string]string),
		make(map[string]string),
		true,
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentClusterRoleBinding *rbacv1.ClusterRoleBinding
		createClusterRoleBinding  bool
		newClusterRoleBindingYAML string
		appLabel                  string
		patchBytes                []byte
		patchType                 types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newClusterRoleBindingYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a ClusterRole and no k8s error occurs": {
			input: input{
				currentClusterRoleBinding: nil,
				createClusterRoleBinding:  true,
				newClusterRoleBindingYAML: newClusterRoleYAML,
				appLabel:                  appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a ClusterRole and a k8s error occurs": {
			input: input{
				currentClusterRoleBinding: nil,
				createClusterRoleBinding:  true,
				newClusterRoleBindingYAML: newClusterRoleYAML,
				appLabel:                  appLabel,
			},
			output: fmt.Errorf("could not create cluster role binding; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to fail when updating a ClusterRole and a k8s error occurs": {
			input: input{
				currentClusterRoleBinding: validClusterRoleBinding,
				createClusterRoleBinding:  false,
				newClusterRoleBindingYAML: newClusterRoleYAML,
				appLabel:                  appLabel,
			},
			output: fmt.Errorf("could not patch cluster role binding; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchClusterRoleBindingByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a ClusterRoleBinding and no k8s error occurs": {
			input: input{
				currentClusterRoleBinding: validClusterRoleBinding,
				createClusterRoleBinding:  false,
				newClusterRoleBindingYAML: newClusterRoleYAML,
				appLabel:                  appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchClusterRoleBindingByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentClusterRoleBinding, createClusterRoleBinding, newClusterRoleBindingYAML, appLabel := test.input.currentClusterRoleBinding,
					test.input.createClusterRoleBinding, test.input.newClusterRoleBindingYAML, test.input.appLabel

				if !createClusterRoleBinding {
					if patchBytes, err = k8sclient.GenericPatch(currentClusterRoleBinding,
						[]byte(newClusterRoleBindingYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newClusterRoleBindingYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutClusterRoleBinding(currentClusterRoleBinding,
					createClusterRoleBinding, newClusterRoleBindingYAML, appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentClusterRoleBinding(t *testing.T) {
	// arrange variables for the tests
	var emptyClusterRoleBindingList, unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding
	var undeletedClusterRoleBindings []string

	getClusterRoleBindingsErr := fmt.Errorf("unable to get list of cluster role bindings")
	clusterRoleBindingName := "clusterRoleBindingName"
	appLabel := "appLabel"

	unwantedClusterRoleBindings = []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleBindingName,
			},
		},
	}

	for _, clusterRoleBinding := range unwantedClusterRoleBindings {
		undeletedClusterRoleBindings = append(undeletedClusterRoleBindings, fmt.Sprintf("%v", clusterRoleBinding.Name))
	}

	type input struct {
		clusterRoleBindingName string
		appLabel               string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetClusterRoleBindingsByLabel fails": {
			input: input{
				clusterRoleBindingName: clusterRoleBindingName,
				appLabel:               appLabel,
			},
			output: getClusterRoleBindingsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(appLabel).Return(nil, getClusterRoleBindingsErr)
			},
		},
		"expect to pass when GetClusterRoleBindingsByLabel returns no cluster role bindings": {
			input: input{
				clusterRoleBindingName: clusterRoleBindingName,
				appLabel:               appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(appLabel).Return(emptyClusterRoleBindingList, nil)
				mockK8sClient.EXPECT().DeleteClusterRoleBinding(clusterRoleBindingName).Return(nil)
			},
		},
		"expect to fail when GetClusterRoleBindingsByLabel succeeds but DeleteClusterRoleBinding fails": {
			input: input{
				clusterRoleBindingName: clusterRoleBindingName,
				appLabel:               appLabel,
			},
			output: fmt.Errorf("unable to delete cluster role binding(s): %v", undeletedClusterRoleBindings),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(appLabel).Return(unwantedClusterRoleBindings, nil)
				mockK8sClient.EXPECT().DeleteClusterRoleBinding(clusterRoleBindingName).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedClusterRoleBindings))
			},
		},
		"expect to pass when GetClusterRoleBindingsByLabel succeeds and DeleteClusterRoleBinding succeeds": {
			input: input{
				clusterRoleBindingName: clusterRoleBindingName,
				appLabel:               appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(appLabel).Return(unwantedClusterRoleBindings, nil)
				mockK8sClient.EXPECT().DeleteClusterRoleBinding(clusterRoleBindingName).Return(nil).
					MaxTimes(len(unwantedClusterRoleBindings))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				clusterRoleBindingName, appLabel := test.input.clusterRoleBindingName, test.input.appLabel
				err := extendedK8sClient.DeleteTridentClusterRoleBinding(clusterRoleBindingName, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleClusterRoleBindings(t *testing.T) {
	// arrange variables for the tests
	var emptyClusterRoleBindings, unwantedClusterRoleBindings, undeletedClusterRoleBindings []rbacv1.ClusterRoleBinding
	var undeletedClusterRoleBindingNames []string

	undeletedClusterRoleErr := fmt.Errorf("could not delete cluster role binding")
	undeletedClusterRoleBindingName := "undeletedClusterRoleBindingName"
	undeletedClusterRoleBindings = []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedClusterRoleBindingName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedClusterRoleBindingName,
			},
		},
	}

	for _, cr := range undeletedClusterRoleBindings {
		undeletedClusterRoleBindingNames = append(undeletedClusterRoleBindingNames, cr.Name)
	}

	unwantedClusterRoleBinding := "undeletedClusterRoleBindingName"
	unwantedClusterRoleBindings = []rbacv1.ClusterRoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: unwantedClusterRoleBinding,
			},
		},
	}

	tests := map[string]struct {
		input  []rbacv1.ClusterRoleBinding
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no cluster role bindings": {
			input:  emptyClusterRoleBindings,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedClusterRoleBindings,
			output: fmt.Errorf("unable to delete cluster role binding(s): %v", undeletedClusterRoleBindingNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteClusterRoleBinding(undeletedClusterRoleBindingName).Return(undeletedClusterRoleErr).
					MaxTimes(len(undeletedClusterRoleBindings))
			},
		},
		"expect to pass with valid cluster role bindings": {
			input:  unwantedClusterRoleBindings,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteClusterRoleBinding(unwantedClusterRoleBinding).Return(nil).
					MaxTimes(len(unwantedClusterRoleBindings))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call and asserts
				err := extendedK8sClient.RemoveMultipleClusterRoleBindings(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetMultipleRoleInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, invalidName, namespace string
	var invalidRole rbacv1.Role
	var validRoles, unwantedRoles, combinationRoles, emptyRoles []rbacv1.Role

	// initialize expected map variables
	expValidRolesMap := make(map[string]*rbacv1.Role)
	expReuseRoleMap := make(map[string]bool)

	windows = true
	roleNames := getNodeResourceNames()
	label = "tridentCSILabel"

	namespace = "default"
	invalidName = ""

	for _, name := range roleNames {
		validRole := rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		validRoles = append(validRoles, validRole)
		// initialize the map variables that will be compared with the values from the function under test
		expValidRolesMap[name] = &validRole
		expReuseRoleMap[name] = true
	}

	invalidRole = rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}

	unwantedRoles = []rbacv1.Role{invalidRole, invalidRole}
	// combinationRoles is a set of valid and invalid roles
	combinationRoles = append(combinationRoles, validRoles...)
	combinationRoles = append(combinationRoles, unwantedRoles...)

	// setup input and output test types for easy use
	type input struct {
		names        []string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentRole   map[string]*rbacv1.Role
		unwantedRoles []rbacv1.Role
		createRole    map[string]bool
		err           error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label, args[1] = name, args[2] = namespace
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				names:        roleNames,
				shouldUpdate: true,
			},
			output: output{
				currentRole:   make(map[string]*rbacv1.Role),
				unwantedRoles: nil,
				createRole:    make(map[string]bool),
				err:           fmt.Errorf("unable to get list of roles"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetRolesByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no role found and no k8s error": {
			input: input{
				label:        label,
				names:        []string{invalidName},
				shouldUpdate: false,
			},
			output: output{
				currentRole:   make(map[string]*rbacv1.Role),
				unwantedRoles: nil,
				createRole:    make(map[string]bool),
				err:           nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetRolesByLabel(args[0]).Return(emptyRoles, nil)
				for _, arg := range args[1].([]string) {
					mockKubeClient.EXPECT().DeleteRole(arg).Return(nil)
				}
			},
		},
		"expect to pass with valid current role found and no k8s error": {
			input: input{
				label:        label,
				names:        roleNames,
				shouldUpdate: false,
			},
			output: output{
				currentRole:   expValidRolesMap,
				unwantedRoles: unwantedRoles,
				createRole:    expReuseRoleMap,
				err:           nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetRolesByLabel(args[0]).Return(combinationRoles, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, roleNames, shouldUpdate := test.input.label, test.input.names, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedRole, expectedUnwantedRoles, expectedReuseRoleMap,
					expectedErr := test.output.currentRole, test.output.unwantedRoles,
					test.output.createRole, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, roleNames, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentRoleMap, unwantedRoles, reuseRoleMap, err := extendedK8sClient.GetMultipleRoleInformation(
					roleNames, label, shouldUpdate)

				// Using cmp.Equal is a better way to deep compare recursive structs/maps and cmp.Diff prints the verbose
				// difference between the two values in comparison for easy debug
				assert.True(t, cmp.Equal(expectedRole, currentRoleMap), cmp.Diff(expectedRole, currentRoleMap))
				assert.True(t, cmp.Equal(expectedUnwantedRoles, unwantedRoles), cmp.Diff(expectedUnwantedRoles, unwantedRoles))
				assert.Equal(t, len(expectedUnwantedRoles), len(unwantedRoles))
				assert.True(t, cmp.Equal(expectedReuseRoleMap, reuseRoleMap),
					cmp.Diff(expectedReuseRoleMap, reuseRoleMap))
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutRole(t *testing.T) {
	var validRole *rbacv1.Role

	roleName := getNodeResourceNames()[0]
	validRole = &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
	}
	appLabel := TridentCSILabel
	newRoleYAML := k8sclient.GetRoleYAML(
		"",
		"trident",
		roleName,
		make(map[string]string),
		make(map[string]string),
		true,
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentRole *rbacv1.Role
		reuseRole   bool
		newRoleYAML string
		appLabel    string
		patchBytes  []byte
		patchType   types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newRoleYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a role and no k8s error occurs": {
			input: input{
				currentRole: &rbacv1.Role{},
				reuseRole:   false,
				newRoleYAML: newRoleYAML,
				appLabel:    appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a role and a k8s error occurs": {
			input: input{
				currentRole: &rbacv1.Role{},
				reuseRole:   false,
				newRoleYAML: newRoleYAML,
				appLabel:    appLabel,
			},
			output: fmt.Errorf("could not create role; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to fail when updating a role and a k8s error occurs": {
			input: input{
				currentRole: validRole,
				reuseRole:   true,
				newRoleYAML: newRoleYAML,
				appLabel:    appLabel,
			},
			output: fmt.Errorf("could not patch Trident role; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchRoleByLabelAndName(appLabel, roleName, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a role and no k8s error occurs": {
			input: input{
				currentRole: validRole,
				reuseRole:   true,
				newRoleYAML: newRoleYAML,
				appLabel:    appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchRoleByLabelAndName(appLabel, roleName, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentRole, reuseRole, newRoleYAML, appLabel := test.input.currentRole,
					test.input.reuseRole, test.input.newRoleYAML, test.input.appLabel

				if reuseRole {
					if patchBytes, err = k8sclient.GenericPatch(currentRole,
						[]byte(newRoleYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newRoleYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutRole(currentRole, reuseRole, newRoleYAML,
					appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteMultipleTridentRoles(t *testing.T) {
	// arrange variables for the tests
	var emptyRoleList, unwantedRoles []rbacv1.Role
	var undeletedRoles []string

	getRoleErr := fmt.Errorf("unable to get list of roles")
	windows = true
	roleNames := getNodeResourceNames()
	appLabel := "appLabel"

	for _, roleName := range roleNames {
		unwantedRole := rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleName,
			},
		}
		unwantedRoles = append(unwantedRoles, unwantedRole)
	}

	for _, role := range unwantedRoles {
		undeletedRoles = append(undeletedRoles, fmt.Sprintf("%v", role.Name))
	}

	type input struct {
		roleNames []string
		appLabel  string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetRolesByLabel fails": {
			input: input{
				roleNames: roleNames,
				appLabel:  appLabel,
			},
			output: getRoleErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRolesByLabel(appLabel).Return(nil, getRoleErr)
			},
		},
		"expect to pass when GetRolesByLabel returns no role bindings": {
			input: input{
				roleNames: roleNames,
				appLabel:  appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRolesByLabel(appLabel).Return(emptyRoleList, nil)
				for _, name := range roleNames {
					mockK8sClient.EXPECT().DeleteRole(name).Return(nil)
				}
			},
		},
		"expect to fail when GetRolesByLabel succeeds but RemoveMultipleRoles fails": {
			input: input{
				roleNames: roleNames,
				appLabel:  appLabel,
			},
			output: fmt.Errorf("unable to delete role(s): %v", undeletedRoles),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRolesByLabel(appLabel).Return(unwantedRoles, nil)
				for _, name := range roleNames {
					mockK8sClient.EXPECT().DeleteRole(name).Return(fmt.Errorf("")).
						MaxTimes(len(unwantedRoles))
				}
			},
		},
		"expect to pass when GetRolesByLabel succeeds and RemoveMultipleRoles succeeds": {
			input: input{
				roleNames: roleNames,
				appLabel:  appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRolesByLabel(appLabel).Return(unwantedRoles, nil)
				for _, name := range roleNames {
					mockK8sClient.EXPECT().DeleteRole(name).Return(nil).
						MaxTimes(len(unwantedRoles))
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				roleNames, appLabel := test.input.roleNames, test.input.appLabel
				err := extendedK8sClient.DeleteMultipleTridentRoles(roleNames, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleRoles(t *testing.T) {
	// arrange variables for the tests
	var emptyRoles, unwantedRoles, undeletedRoles []rbacv1.Role
	var undeletedRoleNames []string

	undeletedRoleErr := fmt.Errorf("could not delete role")
	undeletedRoleName := "undeletedRoleName"
	undeletedRoles = []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedRoleName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedRoleName,
			},
		},
	}

	for _, cr := range undeletedRoles {
		undeletedRoleNames = append(undeletedRoleNames, cr.Name)
	}

	unwantedRoleName := "unwantedRoleName"
	unwantedRoles = []rbacv1.Role{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: unwantedRoleName,
			},
		},
	}

	tests := map[string]struct {
		input  []rbacv1.Role
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no roles": {
			input:  emptyRoles,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedRoles,
			output: fmt.Errorf("unable to delete role(s): %v", undeletedRoleNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteRole(undeletedRoleName).Return(undeletedRoleErr).
					MaxTimes(len(undeletedRoles))
			},
		},
		"expect to pass with valid roles": {
			input:  unwantedRoles,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteRole(unwantedRoleName).Return(nil).
					MaxTimes(len(unwantedRoles))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleRoles(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetMultipleRoleBindingInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, invalidName, namespace string
	var invalidRoleBinding rbacv1.RoleBinding
	var validRoleBindings, unwantedRoleBindings, combinationRoleBindings, emptyRoleBindings []rbacv1.RoleBinding

	// initialize expected map variables
	expValidRoleBindingsMap := make(map[string]*rbacv1.RoleBinding)
	expReuseRoleBindingsMap := make(map[string]bool)

	label = "tridentCSILabel"
	windows = true
	roleBindingNames := getNodeResourceNames() // could be anything
	namespace = "default"
	invalidName = ""

	for _, name := range roleBindingNames {
		validRoleBinding := rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		validRoleBindings = append(validRoleBindings, validRoleBinding)
		expValidRoleBindingsMap[name] = &validRoleBinding
		expReuseRoleBindingsMap[name] = true
	}

	invalidRoleBinding = rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}

	unwantedRoleBindings = []rbacv1.RoleBinding{invalidRoleBinding, invalidRoleBinding}
	combinationRoleBindings = append(combinationRoleBindings, validRoleBindings...)
	combinationRoleBindings = append(combinationRoleBindings, unwantedRoleBindings...)

	// setup input and output test types for easy use
	type input struct {
		names        []string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentRoleBindingMap map[string]*rbacv1.RoleBinding
		unwantedRoleBindings  []rbacv1.RoleBinding
		reuseRoleBindingMap   map[string]bool
		err                   error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label, args[1] = name, args[2] = namespace
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				names:        roleBindingNames,
				shouldUpdate: true,
			},
			output: output{
				currentRoleBindingMap: make(map[string]*rbacv1.RoleBinding),
				unwantedRoleBindings:  nil,
				reuseRoleBindingMap:   make(map[string]bool),
				err:                   fmt.Errorf("unable to get list of role bindings"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetRoleBindingsByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no role binding found and no k8s error": {
			input: input{
				label:        label,
				names:        []string{invalidName},
				shouldUpdate: false,
			},
			output: output{
				currentRoleBindingMap: make(map[string]*rbacv1.RoleBinding),
				unwantedRoleBindings:  nil,
				reuseRoleBindingMap:   make(map[string]bool),
				err:                   nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetRoleBindingsByLabel(args[0]).Return(emptyRoleBindings, nil)
				for _, arg := range args[1].([]string) {
					mockKubeClient.EXPECT().DeleteRoleBinding(arg).Return(nil)
				}
			},
		},
		"expect to pass with valid current role binding found and no k8s error": {
			input: input{
				label:        label,
				names:        roleBindingNames,
				shouldUpdate: false,
			},
			output: output{
				currentRoleBindingMap: expValidRoleBindingsMap,
				unwantedRoleBindings:  unwantedRoleBindings,
				reuseRoleBindingMap:   expReuseRoleBindingsMap,
				err:                   nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetRoleBindingsByLabel(args[0]).Return(combinationRoleBindings,
					nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, roleBindingNames, shouldUpdate := test.input.label, test.input.names, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentRoleBindingMap, expectedUnwantedRoleBindings,
					expectedReuseRoleBindingsMap,
					expectedErr := test.output.currentRoleBindingMap, test.output.unwantedRoleBindings,
					test.output.reuseRoleBindingMap, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, roleBindingNames, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap,
					err := extendedK8sClient.GetMultipleRoleBindingInformation(roleBindingNames,
					label, shouldUpdate)

				assert.True(t, cmp.Equal(expectedCurrentRoleBindingMap, currentRoleBindingMap),
					cmp.Diff(expectedCurrentRoleBindingMap, currentRoleBindingMap))
				assert.True(t, cmp.Equal(expectedUnwantedRoleBindings, unwantedRoleBindings),
					cmp.Diff(expectedUnwantedRoleBindings, unwantedRoleBindings))
				assert.Equal(t, len(expectedUnwantedRoleBindings), len(unwantedRoleBindings))
				assert.True(t, cmp.Equal(expectedReuseRoleBindingsMap, reuseRoleBindingMap),
					cmp.Diff(expectedReuseRoleBindingsMap, reuseRoleBindingMap))
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutRoleBinding(t *testing.T) {
	var validRoleBinding *rbacv1.RoleBinding

	roleBindingName := getNodeResourceNames()[0]
	validRoleBinding = &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleBindingName,
		},
	}
	appLabel := TridentCSILabel
	newRoleYAML := k8sclient.GetRoleBindingYAML(
		"namespace",
		roleBindingName, "",
		make(map[string]string),
		make(map[string]string),
		true,
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentRoleBinding *rbacv1.RoleBinding
		reuseRoleBinding   bool
		newRoleBindingYAML string
		appLabel           string
		patchBytes         []byte
		patchType          types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newRoleBindingYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a role and no k8s error occurs": {
			input: input{
				currentRoleBinding: &rbacv1.RoleBinding{},
				reuseRoleBinding:   false,
				newRoleBindingYAML: newRoleYAML,
				appLabel:           appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a role and a k8s error occurs": {
			input: input{
				currentRoleBinding: &rbacv1.RoleBinding{},
				reuseRoleBinding:   false,
				newRoleBindingYAML: newRoleYAML,
				appLabel:           appLabel,
			},
			output: fmt.Errorf("could not create role binding; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to fail when updating a role and a k8s error occurs": {
			input: input{
				currentRoleBinding: validRoleBinding,
				reuseRoleBinding:   true,
				newRoleBindingYAML: newRoleYAML,
				appLabel:           appLabel,
			},
			output: fmt.Errorf("could not patch role binding; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchRoleBindingByLabelAndName(appLabel, roleBindingName,
					patchBytesMatcher, patchType).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a roleBinding and no k8s error occurs": {
			input: input{
				currentRoleBinding: validRoleBinding,
				reuseRoleBinding:   true,
				newRoleBindingYAML: newRoleYAML,
				appLabel:           appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchRoleBindingByLabelAndName(appLabel, roleBindingName,
					patchBytesMatcher, patchType).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentRoleBinding, reuseRoleBinding, newRoleBindingYAML, appLabel := test.input.currentRoleBinding,
					test.input.reuseRoleBinding, test.input.newRoleBindingYAML, test.input.appLabel

				if reuseRoleBinding {
					if patchBytes, err = k8sclient.GenericPatch(currentRoleBinding,
						[]byte(newRoleBindingYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newRoleBindingYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutRoleBinding(currentRoleBinding,
					reuseRoleBinding, newRoleBindingYAML, appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteMultipleTridentRoleBindings(t *testing.T) {
	// arrange variables for the tests
	var emptyRoleBindingList, unwantedRoleBindings []rbacv1.RoleBinding
	var undeletedRoleBindings []string

	getRoleBindingsErr := fmt.Errorf("unable to get list of role bindings")
	windows = true
	roleBindingNames := getNodeResourceNames()
	appLabel := "appLabel"

	for _, roleBindingName := range roleBindingNames {
		unwantedRoleBinding := rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleBindingName,
			},
		}
		unwantedRoleBindings = append(unwantedRoleBindings, unwantedRoleBinding)
	}

	for _, roleBinding := range unwantedRoleBindings {
		undeletedRoleBindings = append(undeletedRoleBindings, fmt.Sprintf("%v", roleBinding.Name))
	}

	type input struct {
		roleBindingNames []string
		appLabel         string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetRoleBindingsByLabel fails": {
			input: input{
				roleBindingNames: roleBindingNames,
				appLabel:         appLabel,
			},
			output: getRoleBindingsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRoleBindingsByLabel(appLabel).Return(nil, getRoleBindingsErr)
			},
		},
		"expect to pass when GetRoleBindingsByLabel returns no role bindings": {
			input: input{
				roleBindingNames: roleBindingNames,
				appLabel:         appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRoleBindingsByLabel(appLabel).Return(emptyRoleBindingList, nil)
				for _, name := range roleBindingNames {
					mockK8sClient.EXPECT().DeleteRoleBinding(name).Return(nil)
				}
			},
		},
		"expect to fail when GetRoleBindingsByLabel succeeds but RemoveMultipleRoleBindings fails": {
			input: input{
				roleBindingNames: roleBindingNames,
				appLabel:         appLabel,
			},
			output: fmt.Errorf("unable to delete role binding(s): %v", undeletedRoleBindings),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRoleBindingsByLabel(appLabel).Return(unwantedRoleBindings, nil)
				for _, name := range roleBindingNames {
					mockK8sClient.EXPECT().DeleteRoleBinding(name).Return(fmt.Errorf("")).
						MaxTimes(len(unwantedRoleBindings))
				}
			},
		},
		"expect to pass when GetRoleBindingsByLabel succeeds and RemoveMultipleRoleBindings succeeds": {
			input: input{
				roleBindingNames: roleBindingNames,
				appLabel:         appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetRoleBindingsByLabel(appLabel).Return(unwantedRoleBindings, nil)
				for _, name := range roleBindingNames {
					mockK8sClient.EXPECT().DeleteRoleBinding(name).Return(nil).
						MaxTimes(len(unwantedRoleBindings))
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				roleBindingNames, appLabel := test.input.roleBindingNames, test.input.appLabel
				err := extendedK8sClient.DeleteMultipleTridentRoleBindings(roleBindingNames, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleRoleBindings(t *testing.T) {
	// arrange variables for the tests
	var emptyRoleBindings, unwantedRoleBindings, undeletedRoleBindings []rbacv1.RoleBinding
	var undeletedRoleBindingNames []string

	undeletedRoleErr := fmt.Errorf("could not delete role binding")
	undeletedRoleBindingName := "undeletedRoleBindingName"
	undeletedRoleBindings = []rbacv1.RoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedRoleBindingName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: undeletedRoleBindingName,
			},
		},
	}

	for _, cr := range undeletedRoleBindings {
		undeletedRoleBindingNames = append(undeletedRoleBindingNames, cr.Name)
	}

	unwantedRoleBinding := "undeletedRoleBindingName"
	unwantedRoleBindings = []rbacv1.RoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: unwantedRoleBinding,
			},
		},
	}

	tests := map[string]struct {
		input  []rbacv1.RoleBinding
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no role bindings": {
			input:  emptyRoleBindings,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedRoleBindings,
			output: fmt.Errorf("unable to delete role binding(s): %v", undeletedRoleBindingNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteRoleBinding(undeletedRoleBindingName).Return(undeletedRoleErr).
					MaxTimes(len(undeletedRoleBindings))
			},
		},
		"expect to pass with valid role bindings": {
			input:  unwantedRoleBindings,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteRoleBinding(unwantedRoleBinding).Return(nil).
					MaxTimes(len(unwantedRoleBindings))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call and asserts
				err := extendedK8sClient.RemoveMultipleRoleBindings(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetDaemonSetInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, namespace string
	var validDaemonSet appsv1.DaemonSet
	var unwantedDaemonSets []appsv1.DaemonSet

	label = "label"
	name = getDaemonSetName(false)
	namespace = "namespace"

	validDaemonSet = appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	unwantedDaemonSets = append(unwantedDaemonSets, validDaemonSet)

	// setup input and output test types for easy use
	type input struct {
		label     string
		namespace string
		windows   bool
	}

	type output struct {
		currentDaemonSet   *appsv1.DaemonSet
		unwantedDaemonSets []appsv1.DaemonSet
		createDaemonSet    bool
		err                error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail with k8s error": {
			input: input{
				label:     label,
				namespace: namespace,
				windows:   false,
			},
			output: output{
				currentDaemonSet:   nil,
				unwantedDaemonSets: nil,
				createDaemonSet:    true,
				err:                fmt.Errorf("unable to get list of daemonset"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				mockKubeClient.EXPECT().GetDaemonSetsByLabel(label, true).Return(nil,
					fmt.Errorf("unable to get list of daemonset"))
			},
		},
		"expect to pass with no daemonset found and no k8s error": {
			input: input{
				label:     label,
				namespace: "",
				windows:   false,
			},
			output: output{
				currentDaemonSet:   nil,
				unwantedDaemonSets: nil,
				createDaemonSet:    true,
				err:                nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				mockKubeClient.EXPECT().GetDaemonSetsByLabel(label, true).Return(nil, nil)
			},
		},
		"expect to pass with current daemonset found and no k8s error": {
			input: input{
				label:     label,
				namespace: namespace,
				windows:   false,
			},
			output: output{
				currentDaemonSet:   &validDaemonSet,
				unwantedDaemonSets: nil,
				createDaemonSet:    false,
				err:                nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				daemonSets := make([]appsv1.DaemonSet, 0)
				daemonSets = append(daemonSets, validDaemonSet)
				mockKubeClient.EXPECT().GetDaemonSetsByLabel(label, true).Return(daemonSets, nil)
			},
		},
		"expect to pass with no daemonset found in given namespace": {
			input: input{
				label:     label,
				namespace: "trident-namespace",
				windows:   true,
			},
			output: output{
				currentDaemonSet:   nil,
				unwantedDaemonSets: unwantedDaemonSets,
				createDaemonSet:    true,
				err:                nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				daemonSets := make([]appsv1.DaemonSet, 0)
				daemonSets = append(daemonSets, validDaemonSet)
				mockKubeClient.EXPECT().GetDaemonSetsByLabel(label, true).Return(daemonSets, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, namespace, isWindows := test.input.label, test.input.namespace, test.input.windows

				// extract the output variables from the test case definition
				expectedDaemonSet, expectedUnwantedDaemonSets, expectedCreateDaemonSet,
					expectedErr := test.output.currentDaemonSet, test.output.unwantedDaemonSets,
					test.output.createDaemonSet, test.output.err

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentDaemonSet, unwantedDaemonsSets, createDaemonSet, err := extendedK8sClient.GetDaemonSetInformation(
					label, namespace, isWindows)

				assert.EqualValues(t, expectedDaemonSet, currentDaemonSet)
				assert.EqualValues(t, expectedUnwantedDaemonSets, unwantedDaemonsSets)
				assert.Equal(t, len(expectedUnwantedDaemonSets), len(unwantedDaemonsSets))
				assert.Equal(t, expectedCreateDaemonSet, createDaemonSet)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutDaemonSet(t *testing.T) {
	daemonSetName := getDaemonSetName(false)
	nodeLabel := "app=node.csi.trident.netapp.io"
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: daemonSetName,
		},
	}
	version, _ := utils.ParseSemantic("1.21.3")
	daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:        daemonSetName,
		ImagePullSecrets:     make([]string, 0),
		Labels:               make(map[string]string),
		ControllingCRDetails: make(map[string]string),
		Debug:                false,
		Version:              version,
	}
	newDaemonSetYAML := k8sclient.GetCSIDaemonSetYAMLLinux(daemonSetArgs)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentDaemonSet *appsv1.DaemonSet
		createDaemonSet  bool
		newDaemonSetYAML string
		nodeLabel        string
		patchBytes       []byte
		patchType        types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newDaemonSetYAML, args[1] = nodeLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a DaemonSet and no k8s error occurs": {
			input: input{
				currentDaemonSet: nil,
				createDaemonSet:  true,
				newDaemonSetYAML: newDaemonSetYAML,
				nodeLabel:        nodeLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a DaemonSet and a k8s error occurs": {
			input: input{
				currentDaemonSet: nil,
				createDaemonSet:  true,
				newDaemonSetYAML: newDaemonSetYAML,
				nodeLabel:        nodeLabel,
			},
			output: fmt.Errorf("could not create Trident daemonset; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a DaemonSet and no k8s error occurs": {
			input: input{
				currentDaemonSet: daemonSet,
				createDaemonSet:  false,
				nodeLabel:        nodeLabel,
				newDaemonSetYAML: newDaemonSetYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				nodeLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDaemonSetByLabelAndName(nodeLabel, daemonSetName, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a DaemonSet and a k8s error occurs": {
			input: input{
				currentDaemonSet: daemonSet,
				createDaemonSet:  false,
				nodeLabel:        nodeLabel,
				newDaemonSetYAML: newDaemonSetYAML,
			},
			output: fmt.Errorf("could not patch Trident DaemonSet; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				nodeLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDaemonSetByLabelAndName(nodeLabel, daemonSetName, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentDaemonSet, createDaemonSet, newDaemonSetYAML, nodeLabel := test.input.currentDaemonSet,
					test.input.createDaemonSet, test.input.newDaemonSetYAML, test.input.nodeLabel

				if !createDaemonSet {
					if patchBytes, err = k8sclient.GenericPatch(currentDaemonSet,
						[]byte(newDaemonSetYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newDaemonSetYAML, nodeLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutDaemonSet(currentDaemonSet, createDaemonSet, newDaemonSetYAML,
					nodeLabel, daemonSetName)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutDaemonSet_Windows(t *testing.T) {
	daemonSetName := TridentCSIWindows
	nodeLabel := "app=node.csi.trident.netapp.io"
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: daemonSetName,
		},
	}
	version, _ := utils.ParseSemantic("1.21.3")
	daemonSetArgs := &k8sclient.DaemonsetYAMLArguments{
		DaemonsetName:        daemonSetName,
		ImagePullSecrets:     make([]string, 0),
		Labels:               make(map[string]string),
		ControllingCRDetails: make(map[string]string),
		Debug:                false,
		Version:              version,
	}
	newDaemonSetYAML := k8sclient.GetCSIDaemonSetYAMLWindows(daemonSetArgs)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentDaemonSet *appsv1.DaemonSet
		createDaemonSet  bool
		newDaemonSetYAML string
		nodeLabel        string
		patchBytes       []byte
		patchType        types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newDaemonSetYAML, args[1] = nodeLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a DaemonSet and no k8s error occurs": {
			input: input{
				currentDaemonSet: nil,
				createDaemonSet:  true,
				newDaemonSetYAML: newDaemonSetYAML,
				nodeLabel:        nodeLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a DaemonSet and a k8s error occurs": {
			input: input{
				currentDaemonSet: nil,
				createDaemonSet:  true,
				newDaemonSetYAML: newDaemonSetYAML,
				nodeLabel:        nodeLabel,
			},
			output: fmt.Errorf("could not create Trident daemonset; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a DaemonSet and no k8s error occurs": {
			input: input{
				currentDaemonSet: daemonSet,
				createDaemonSet:  false,
				nodeLabel:        nodeLabel,
				newDaemonSetYAML: newDaemonSetYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				nodeLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDaemonSetByLabelAndName(nodeLabel, daemonSetName, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a DaemonSet and a k8s error occurs": {
			input: input{
				currentDaemonSet: daemonSet,
				createDaemonSet:  false,
				nodeLabel:        nodeLabel,
				newDaemonSetYAML: newDaemonSetYAML,
			},
			output: fmt.Errorf("could not patch Trident DaemonSet; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				nodeLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDaemonSetByLabelAndName(nodeLabel, daemonSetName, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentDaemonSet, createDaemonSet, newDaemonSetYAML, nodeLabel := test.input.currentDaemonSet,
					test.input.createDaemonSet, test.input.newDaemonSetYAML, test.input.nodeLabel

				if !createDaemonSet {
					if patchBytes, err = k8sclient.GenericPatch(currentDaemonSet,
						[]byte(newDaemonSetYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newDaemonSetYAML, nodeLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutDaemonSet(currentDaemonSet, createDaemonSet, newDaemonSetYAML,
					nodeLabel, daemonSetName)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentDaemonSet(t *testing.T) {
	// arrange variables for the tests
	var emptyDaemonSetList, unwantedDaemonSets []appsv1.DaemonSet
	var undeletedDaemonSets []string

	getDaemonSetsErr := fmt.Errorf("unable to get list of daemonset")
	nodeLabel := "node-label"
	daemonSetName := "daemonSetName"
	daemonSetName2 := "daemonSetName2"
	namespace := "namespace"

	unwantedDaemonSets = []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName,
				Namespace: namespace,
			},
		},
	}

	unwantedDaemonSets2 := []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName,
				Namespace: namespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName2,
				Namespace: namespace,
			},
		},
	}

	for _, daemonSet := range unwantedDaemonSets {
		undeletedDaemonSets = append(undeletedDaemonSets, fmt.Sprintf("%v/%v", daemonSet.Namespace,
			daemonSet.Name))
	}

	tests := map[string]struct {
		input  string
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetDaemonSetsByLabel fails": {
			input:  nodeLabel,
			output: getDaemonSetsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDaemonSetsByLabel(nodeLabel, true).Return(nil, getDaemonSetsErr)
			},
		},
		"expect to pass when GetDaemonSetsByLabel returns no deployments": {
			input:  nodeLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDaemonSetsByLabel(nodeLabel, true).Return(emptyDaemonSetList, nil)
			},
		},
		"expect to fail when GetDaemonSetsByLabel succeeds but RemoveMultipleDeployments fails": {
			input:  nodeLabel,
			output: fmt.Errorf("unable to delete daemonset(s): %v", undeletedDaemonSets),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDaemonSetsByLabel(nodeLabel, true).Return(unwantedDaemonSets, nil)
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName,
					namespace, true).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedDaemonSets))
			},
		},
		"expect to pass when GetDaemonSetsByLabel succeeds and RemoveMultipleDeployments succeeds": {
			input:  nodeLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDaemonSetsByLabel(nodeLabel, true).Return(unwantedDaemonSets, nil)
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName,
					namespace, true).Return(nil).
					MaxTimes(len(unwantedDaemonSets))
			},
		},
		"expect to pass when multiple daemon sets are found and RemoveMultipleDeployments succeeds": {
			input:  nodeLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDaemonSetsByLabel(nodeLabel, true).Return(unwantedDaemonSets2, nil)
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName, namespace, true).Return(nil)
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName2, namespace, true).Return(nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.DeleteTridentDaemonSet(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleDaemonSets(t *testing.T) {
	// arrange variables for the tests
	var emptyDaemonSets, unwantedDaemonSets []appsv1.DaemonSet
	var undeletedDaemonSets []string

	deleteDaemonSetErr := fmt.Errorf("could not delete daemonset")
	daemonSetName := "undeletedDaemonSet"
	daemonSetNamespace := "daemonSetNamespace"

	unwantedDaemonSets = []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName,
				Namespace: daemonSetNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonSetName,
				Namespace: daemonSetNamespace,
			},
		},
	}

	for _, daemonSet := range unwantedDaemonSets {
		undeletedDaemonSets = append(undeletedDaemonSets, fmt.Sprintf("%v/%v", daemonSet.Namespace,
			daemonSet.Name))
	}

	tests := map[string]struct {
		input  []appsv1.DaemonSet
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no daemonsets": {
			input:  emptyDaemonSets,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  unwantedDaemonSets,
			output: fmt.Errorf("unable to delete daemonset(s): %v", undeletedDaemonSets),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName,
					daemonSetNamespace, true).Return(deleteDaemonSetErr).
					MaxTimes(len(unwantedDaemonSets))
			},
		},
		"expect to pass with valid daemonsets": {
			input:  unwantedDaemonSets,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteDaemonSet(daemonSetName,
					daemonSetNamespace, true).Return(nil).
					MaxTimes(len(unwantedDaemonSets))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleDaemonSets(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetDeploymentInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validDeployment, invalidDeployment appsv1.Deployment
	var unwantedDeployments, combinationDeployments []appsv1.Deployment

	label = "label"
	name = "name"
	namespace = "namespace"
	invalidName = ""

	validDeployment = appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidDeployment = appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedDeployments = []appsv1.Deployment{invalidDeployment, invalidDeployment}
	combinationDeployments = append([]appsv1.Deployment{validDeployment}, append(combinationDeployments,
		unwantedDeployments...)...)

	// setup input and output test types for easy use
	type input struct {
		label     string
		name      string
		namespace string
	}

	type output struct {
		currentDeployment   *appsv1.Deployment
		unwantedDeployments []appsv1.Deployment
		createDeployment    bool
		err                 error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail with k8s error": {
			input: input{
				label:     label,
				name:      name,
				namespace: namespace,
			},
			output: output{
				currentDeployment:   nil,
				unwantedDeployments: nil,
				createDeployment:    true,
				err:                 fmt.Errorf("unable to get list of deployments"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				mockKubeClient.EXPECT().GetDeploymentsByLabel(label, true).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no daemonset found and no k8s error": {
			input: input{
				label:     label,
				name:      invalidName,
				namespace: "",
			},
			output: output{
				currentDeployment:   nil,
				unwantedDeployments: nil,
				createDeployment:    true,
				err:                 nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				mockKubeClient.EXPECT().GetDeploymentsByLabel(label, true).Return(nil, nil)
			},
		},
		"expect to pass with current daemonset found and no k8s error": {
			input: input{
				label:     label,
				name:      name,
				namespace: namespace,
			},
			output: output{
				currentDeployment:   &validDeployment,
				unwantedDeployments: unwantedDeployments,
				createDeployment:    false,
				err:                 nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				// mock calls here
				mockKubeClient.EXPECT().GetDeploymentsByLabel(label, true).Return(combinationDeployments, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, namespace := test.input.label, test.input.name, test.input.namespace

				// extract the output variables from the test case definition
				expectedDaemonSet, expectedUnwantedDaemonSets, expectedCreateDaemonSet,
					expectedErr := test.output.currentDeployment, test.output.unwantedDeployments,
					test.output.createDeployment, test.output.err

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentDaemonSet, unwantedDaemonsSets, createDaemonSet, err := extendedK8sClient.GetDeploymentInformation(name,
					label, namespace)

				assert.EqualValues(t, expectedDaemonSet, currentDaemonSet)
				assert.EqualValues(t, expectedUnwantedDaemonSets, unwantedDaemonsSets)
				assert.Equal(t, len(expectedUnwantedDaemonSets), len(unwantedDaemonsSets))
				assert.Equal(t, expectedCreateDaemonSet, createDaemonSet)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutDeployment(t *testing.T) {
	deploymentName := getDeploymentName(true)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
		},
	}
	appLabel := TridentCSILabel
	version, _ := utils.ParseSemantic("1.21.3")
	deploymentArgs := &k8sclient.DeploymentYAMLArguments{
		DeploymentName:       deploymentName,
		ImagePullSecrets:     make([]string, 0),
		Labels:               make(map[string]string),
		ControllingCRDetails: make(map[string]string),
		Debug:                false,
		UseIPv6:              false,
		SilenceAutosupport:   true,
		Version:              version,
		TopologyEnabled:      false,
	}
	newDeploymentYAML := k8sclient.GetCSIDeploymentYAML(deploymentArgs)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentDeployment *appsv1.Deployment
		createDeployment  bool
		newDeploymentYAML string
		appLabel          string
		patchBytes        []byte
		patchType         types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newDeploymentYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a Deployment and no k8s error occurs": {
			input: input{
				currentDeployment: nil,
				createDeployment:  true,
				newDeploymentYAML: newDeploymentYAML,
				appLabel:          appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a Deployment and a k8s error occurs": {
			input: input{
				currentDeployment: nil,
				createDeployment:  true,
				newDeploymentYAML: newDeploymentYAML,
				appLabel:          appLabel,
			},
			output: fmt.Errorf("could not create Trident deployment; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a DaemonSet and no k8s error occurs": {
			input: input{
				currentDeployment: deployment,
				createDeployment:  false,
				newDeploymentYAML: newDeploymentYAML,
				appLabel:          appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDeploymentByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a Deployment and a k8s error occurs": {
			input: input{
				currentDeployment: deployment,
				createDeployment:  false,
				newDeploymentYAML: newDeploymentYAML,
				appLabel:          appLabel,
			},
			output: fmt.Errorf("could not patch Trident deployment; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchDeploymentByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentDeployment, createDeployment, newDeploymentYAML, appLabel := test.input.currentDeployment,
					test.input.createDeployment, test.input.newDeploymentYAML, test.input.appLabel

				if !createDeployment {
					if patchBytes, err = k8sclient.GenericPatch(currentDeployment,
						[]byte(newDeploymentYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newDeploymentYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutDeployment(currentDeployment, createDeployment, newDeploymentYAML,
					appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentDeployment(t *testing.T) {
	// arrange variables for the tests
	var emptyDeploymentList, unwantedDeployments []appsv1.Deployment
	var undeletedDeployments []string

	getDeploymentsErr := fmt.Errorf("unable to get list of deployments")
	appLabel := "trident-app-label"
	deploymentName := "deploymentName"
	deploymentNamespace := "deploymentNamespace"

	unwantedDeployments = []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: deploymentNamespace,
			},
		},
	}

	for _, deployment := range unwantedDeployments {
		undeletedDeployments = append(undeletedDeployments, fmt.Sprintf("%v/%v", deployment.Namespace,
			deployment.Name))
	}

	tests := map[string]struct {
		input  string
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetDeploymentsByLabel fails": {
			input:  appLabel,
			output: getDeploymentsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDeploymentsByLabel(appLabel, true).Return(nil, getDeploymentsErr)
			},
		},
		"expect to pass when GetDeploymentsByLabel returns no deployments": {
			input:  appLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDeploymentsByLabel(appLabel, true).Return(emptyDeploymentList, nil)
			},
		},
		"expect to fail when GetDeploymentsByLabel succeeds but RemoveMultipleDeployments fails": {
			input:  appLabel,
			output: fmt.Errorf("unable to delete deployment(s): %v", undeletedDeployments),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDeploymentsByLabel(appLabel, true).Return(unwantedDeployments, nil)
				mockK8sClient.EXPECT().DeleteDeployment(deploymentName,
					deploymentNamespace, true).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedDeployments))
			},
		},
		"expect to pass when GetDeploymentsByLabel succeeds and RemoveMultipleDeployments succeeds": {
			input:  appLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetDeploymentsByLabel(appLabel, true).Return(unwantedDeployments, nil)
				mockK8sClient.EXPECT().DeleteDeployment(deploymentName,
					deploymentNamespace, true).Return(nil).
					MaxTimes(len(unwantedDeployments))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.DeleteTridentDeployment(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleDeployments(t *testing.T) {
	// arrange variables for the tests
	var emptyDeploymentList, unwantedDeployments []appsv1.Deployment
	var undeletedDeployments []string

	deleteDeploymentErr := fmt.Errorf("could not delete deployment")
	deploymentName := "deploymentName"
	deploymentNamespace := "deploymentNamespace"

	unwantedDeployments = []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: deploymentNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: deploymentNamespace,
			},
		},
	}

	for _, deployment := range unwantedDeployments {
		undeletedDeployments = append(undeletedDeployments, fmt.Sprintf("%v/%v", deployment.Namespace,
			deployment.Name))
	}

	tests := map[string]struct {
		input  []appsv1.Deployment
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no deployments": {
			input:  emptyDeploymentList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  unwantedDeployments,
			output: fmt.Errorf("unable to delete deployment(s): %v", undeletedDeployments),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteDeployment(deploymentName,
					deploymentNamespace, true).Return(deleteDeploymentErr).
					MaxTimes(len(unwantedDeployments))
			},
		},
		"expect to pass with valid deployments": {
			input:  unwantedDeployments,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteDeployment(deploymentName,
					deploymentNamespace, true).Return(nil).
					MaxTimes(len(unwantedDeployments))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleDeployments(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetMultiplePodSecurityPolicyInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, invalidName, namespace string
	var validPSPs []policyv1beta1.PodSecurityPolicy
	var invalidPSP policyv1beta1.PodSecurityPolicy
	var unwantedPSPs, combinationPSPs, emptyPSPs []policyv1beta1.PodSecurityPolicy

	expCurrentPSPMap := make(map[string]*policyv1beta1.PodSecurityPolicy)
	expReusePSPMap := make(map[string]bool)

	label = "tridentCSILabel"
	pspNames := getRBACResourceNames()
	namespace = "default"
	invalidName = ""

	for _, name := range pspNames {
		validPSP := policyv1beta1.PodSecurityPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		validPSPs = append(validPSPs, validPSP)
		expCurrentPSPMap[name] = &validPSP
		expReusePSPMap[name] = true
	}

	invalidPSP = policyv1beta1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}

	unwantedPSPs = []policyv1beta1.PodSecurityPolicy{invalidPSP, invalidPSP}
	combinationPSPs = append(combinationPSPs, unwantedPSPs...)
	combinationPSPs = append(combinationPSPs, validPSPs...)

	// setup input and output test types for easy use
	type input struct {
		names        []string
		label        string
		shouldUpdate bool
	}

	type output struct {
		currentPSPMap map[string]*policyv1beta1.PodSecurityPolicy
		unwantedPSPs  []policyv1beta1.PodSecurityPolicy
		reusePSPMap   map[string]bool
		err           error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label (string), args[1] = name (string), args[2] = shouldUpdate (bool)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				names:        []string{invalidName},
				shouldUpdate: true,
			},
			output: output{
				currentPSPMap: make(map[string]*policyv1beta1.PodSecurityPolicy),
				unwantedPSPs:  nil,
				reusePSPMap:   make(map[string]bool),
				err:           fmt.Errorf("unable to get list of pod security policies"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodSecurityPoliciesByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no pod security policies found and no k8s error": {
			input: input{
				label:        label,
				names:        []string{invalidName},
				shouldUpdate: false,
			},
			output: output{
				currentPSPMap: make(map[string]*policyv1beta1.PodSecurityPolicy),
				unwantedPSPs:  nil,
				reusePSPMap:   make(map[string]bool),
				err:           nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// args[0] = label, args[1] = name, args[2] = shouldUpdate
				mockKubeClient.EXPECT().GetPodSecurityPoliciesByLabel(args[0]).Return(emptyPSPs, nil)
				for _, arg := range (args[1]).([]string) {
					mockKubeClient.EXPECT().DeletePodSecurityPolicy(arg).Return(nil)
				}
			},
		},
		"expect to pass with valid current pod security policies found and no k8s error": {
			input: input{
				label:        label,
				names:        pspNames,
				shouldUpdate: false,
			},
			output: output{
				currentPSPMap: expCurrentPSPMap,
				unwantedPSPs:  unwantedPSPs,
				reusePSPMap:   expReusePSPMap,
				err:           nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetPodSecurityPoliciesByLabel(args[0]).Return(combinationPSPs, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, names, shouldUpdate := test.input.label, test.input.names, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentPSPMap, expectedUnwantedPSPs,
					expectedReusePSPMap,
					expectedErr := test.output.currentPSPMap, test.output.unwantedPSPs,
					test.output.reusePSPMap, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = names, args[2] = shouldUpdate
				test.mocks(mockKubeClient, label, names, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentPSPMap, unwantedPSPs, reusePSPMap,
					err := extendedK8sClient.GetMultiplePodSecurityPolicyInformation(names,
					label, shouldUpdate)

				assert.True(t, cmp.Equal(expectedCurrentPSPMap, currentPSPMap), cmp.Diff(expectedCurrentPSPMap, currentPSPMap))
				assert.True(t, cmp.Equal(expectedUnwantedPSPs, unwantedPSPs), cmp.Diff(expectedUnwantedPSPs, unwantedPSPs))
				assert.True(t, cmp.Equal(expectedReusePSPMap, reusePSPMap), cmp.Diff(expectedReusePSPMap, reusePSPMap))
				assert.Equal(t, len(expectedUnwantedPSPs), len(unwantedPSPs))
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutPodSecurityPolicy(t *testing.T) {
	pspName := getPSPName()
	podSecurityPolicy := &policyv1beta1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: pspName,
		},
	}
	appLabel := TridentCSILabel
	newPSPYAML := k8sclient.GetPrivilegedPodSecurityPolicyYAML(
		pspName,
		make(map[string]string),
		make(map[string]string),
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentPSP *policyv1beta1.PodSecurityPolicy
		reusePSP   bool
		newPSPYAML string
		appLabel   string
		patchBytes []byte
		patchType  types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newPSPYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a Deployment and no k8s error occurs": {
			input: input{
				currentPSP: nil,
				reusePSP:   false,
				newPSPYAML: newPSPYAML,
				appLabel:   appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a PodSecurityPolicy and a k8s error occurs": {
			input: input{
				currentPSP: nil,
				reusePSP:   false,
				newPSPYAML: newPSPYAML,
				appLabel:   appLabel,
			},
			output: fmt.Errorf("could not create Trident pod security policy; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a PodSecurityPolicy and no k8s error occurs": {
			input: input{
				currentPSP: podSecurityPolicy,
				reusePSP:   true,
				newPSPYAML: newPSPYAML,
				appLabel:   appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchPodSecurityPolicyByLabelAndName(appLabel, pspName, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a PodSecurityPolicy and a k8s error occurs": {
			input: input{
				currentPSP: podSecurityPolicy,
				reusePSP:   true,
				newPSPYAML: newPSPYAML,
				appLabel:   appLabel,
			},
			output: fmt.Errorf("could not patch Trident Pod security policy; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchPodSecurityPolicyByLabelAndName(appLabel, pspName, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentPSP, reusePSP, newPSPYAML, appLabel := test.input.currentPSP,
					test.input.reusePSP, test.input.newPSPYAML, test.input.appLabel

				if reusePSP {
					if patchBytes, err = k8sclient.GenericPatch(currentPSP, []byte(newPSPYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newPSPYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutPodSecurityPolicy(currentPSP, reusePSP, newPSPYAML, appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentPodSecurityPolicy(t *testing.T) {
	// arrange variables for the tests
	var emptyPSPList, unwantedPSPs []policyv1beta1.PodSecurityPolicy
	var undeletedPSPs []string

	getPSPsErr := fmt.Errorf("unable to get list of Pod security policies")
	pspName := "pspName"
	appLabel := "appLabel"

	unwantedPSPs = []policyv1beta1.PodSecurityPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: pspName,
			},
		},
	}

	for _, policy := range unwantedPSPs {
		undeletedPSPs = append(undeletedPSPs, fmt.Sprintf("%v", policy.Name))
	}

	type input struct {
		pspName  string
		appLabel string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetPodSecurityPoliciesByLabel fails": {
			input: input{
				pspName:  pspName,
				appLabel: appLabel,
			},
			output: getPSPsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetPodSecurityPoliciesByLabel(appLabel).Return(nil, getPSPsErr)
			},
		},
		"expect to pass when GetPodSecurityPoliciesByLabel returns no policies": {
			input: input{
				pspName:  pspName,
				appLabel: appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetPodSecurityPoliciesByLabel(appLabel).Return(emptyPSPList, nil)
				mockK8sClient.EXPECT().DeletePodSecurityPolicy(pspName).Return(nil)
			},
		},
		"expect to fail when GetPodSecurityPoliciesByLabel succeeds but RemoveMultiplePodSecurityPolicies fails": {
			input: input{
				pspName:  pspName,
				appLabel: appLabel,
			},
			output: fmt.Errorf("unable to delete pod security policies: %v", undeletedPSPs),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetPodSecurityPoliciesByLabel(appLabel).Return(unwantedPSPs, nil)
				mockK8sClient.EXPECT().DeletePodSecurityPolicy(pspName).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedPSPs))
			},
		},
		"expect to pass when GetPodSecurityPoliciesByLabel succeeds and RemoveMultiplePodSecurityPolicies succeeds": {
			input: input{
				pspName:  pspName,
				appLabel: appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetPodSecurityPoliciesByLabel(appLabel).Return(unwantedPSPs, nil)
				mockK8sClient.EXPECT().DeletePodSecurityPolicy(pspName).Return(nil).
					MaxTimes(len(unwantedPSPs))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				secretName, appLabel := test.input.pspName, test.input.appLabel
				err := extendedK8sClient.DeleteTridentPodSecurityPolicy(secretName, appLabel)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultiplePodSecurityPolicies(t *testing.T) {
	// arrange variables for the tests
	var emptyPodSecurityPolicyList, undeletedPodSecurityPolicies, unwantedPodSecurityPolicies []policyv1beta1.PodSecurityPolicy
	var undeletedPodSecurityPolicyNames []string

	deletePodSecurityPolicyErr := fmt.Errorf("could not delete pod security policy")
	podName := "podName"

	undeletedPodSecurityPolicies = []policyv1beta1.PodSecurityPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: podName,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: podName,
			},
		},
	}

	for _, podSecurityPolicy := range undeletedPodSecurityPolicies {
		undeletedPodSecurityPolicyNames = append(undeletedPodSecurityPolicyNames, fmt.Sprintf("%v",
			podSecurityPolicy.Name))
	}

	unwantedPodSecurityPolicies = []policyv1beta1.PodSecurityPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: podName,
			},
		},
	}

	tests := map[string]struct {
		input  []policyv1beta1.PodSecurityPolicy
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no pod security policies": {
			input:  emptyPodSecurityPolicyList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedPodSecurityPolicies,
			output: fmt.Errorf("unable to delete pod security policies: %v", undeletedPodSecurityPolicyNames),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeletePodSecurityPolicy(podName).Return(deletePodSecurityPolicyErr).
					MaxTimes(len(undeletedPodSecurityPolicies))
			},
		},
		"expect to pass with valid pod security policies": {
			input:  unwantedPodSecurityPolicies,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeletePodSecurityPolicy(podName).Return(nil).
					MaxTimes(len(unwantedPodSecurityPolicies))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultiplePodSecurityPolicies(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetSecretInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validSecret, invalidSecret corev1.Secret
	var unwantedSecret, combinationSecrets, emptySecrets []corev1.Secret

	label = "tridentCSILabel"
	name = getServiceName() // could be anything
	namespace = "default"
	invalidName = ""

	validSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedSecret = []corev1.Secret{invalidSecret, invalidSecret}
	combinationSecrets = append([]corev1.Secret{validSecret}, append(combinationSecrets,
		unwantedSecret...)...)

	// setup input and output test types for easy use
	type input struct {
		label        string
		name         string
		namespace    string
		shouldUpdate bool
	}

	type output struct {
		currentSecret   *corev1.Secret
		unwantedSecrets []corev1.Secret
		createSecret    bool
		err             error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label (string), args[1] = name (string), args[2] = namespace (string), args[3] = shouldUpdate (bool)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: true,
			},
			output: output{
				currentSecret:   nil,
				unwantedSecrets: nil,
				createSecret:    true,
				err:             fmt.Errorf("unable to get list of secrets by label"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetSecretsByLabel(args[0], false).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no secrets found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentSecret:   nil,
				unwantedSecrets: nil,
				createSecret:    true,
				err:             nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetSecretsByLabel(args[0], false).Return(emptySecrets, nil)
				mockKubeClient.EXPECT().DeleteSecret(args[1], args[2]).Return(nil)
			},
		},
		"expect to pass with valid current secrets found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentSecret:   nil,
				unwantedSecrets: unwantedSecret,
				createSecret:    false,
				err:             nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetSecretsByLabel(args[0], false).Return(combinationSecrets, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, namespace, shouldUpdate := test.input.label, test.input.name,
					test.input.namespace, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentSecret, expectedUnwantedSecrets,
					expectedCreateSecret,
					expectedErr := test.output.currentSecret, test.output.unwantedSecrets,
					test.output.createSecret, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = namespace, args[3] = shouldUpdate
				test.mocks(mockKubeClient, label, name, namespace, shouldUpdate)

				extendedK8sClient := &K8sClient{mockKubeClient}
				currentSecret, unwantedSecrets, createSecret,
					err := extendedK8sClient.GetSecretInformation(name,
					label, namespace, shouldUpdate)

				assert.EqualValues(t, expectedCurrentSecret, currentSecret)
				assert.EqualValues(t, expectedUnwantedSecrets, unwantedSecrets)
				assert.Equal(t, len(expectedUnwantedSecrets), len(unwantedSecrets))
				assert.Equal(t, expectedCreateSecret, createSecret)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutSecret(t *testing.T) {
	secretName := getProtocolSecretName()
	namespace := "trident"
	newSecretYAML := k8sclient.GetSecretYAML(
		secretName,
		namespace,
		make(map[string]string),
		make(map[string]string),
		make(map[string]string),
		make(map[string]string),
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		createSecret  bool
		newSecretYAML string
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = newSecretYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a Secret and no k8s error occurs": {
			input: input{
				createSecret:  true,
				newSecretYAML: newSecretYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to pass when createSecret is false": {
			input: input{
				createSecret:  false,
				newSecretYAML: newSecretYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// no calls to mock here
			},
		},
		"expect to fail when creating a Secret and a k8s error occurs": {
			input: input{
				createSecret:  true,
				newSecretYAML: newSecretYAML,
			},
			output: fmt.Errorf("could not create Trident secret; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				createSecret, newSecretYAML := test.input.createSecret, test.input.newSecretYAML

				// extract the output err
				expectedErr := test.output

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newSecretYAML, createSecret, namespace)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutSecret(createSecret, newSecretYAML, secretName)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentSecret(t *testing.T) {
	// arrange variables for the tests
	var emptySecretList, unwantedSecrets []corev1.Secret
	var undeletedSecrets []string

	getSecretsErr := fmt.Errorf("unable to get list of secrets")
	secretName := "encryptSecretName"
	appLabel := "appLabel"
	namespace := "namespace"

	unwantedSecrets = []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
		},
	}

	for _, secret := range unwantedSecrets {
		undeletedSecrets = append(undeletedSecrets, fmt.Sprintf("%v/%v", secret.Namespace, secret.Name))
	}

	type input struct {
		secretName string
		appLabel   string
		namespace  string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetSecretsByLabel fails": {
			input: input{
				secretName: secretName,
				appLabel:   appLabel,
				namespace:  namespace,
			},
			output: getSecretsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetSecretsByLabel(appLabel, false).Return(nil, getSecretsErr)
			},
		},
		"expect to pass when GetSecretsByLabel returns no secrets": {
			input: input{
				secretName: secretName,
				appLabel:   appLabel,
				namespace:  namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetSecretsByLabel(appLabel, false).Return(emptySecretList, nil)
				mockK8sClient.EXPECT().DeleteSecret(secretName, namespace).Return(nil)
			},
		},
		"expect to fail when GetSecretsByLabel succeeds but RemoveMultipleSecrets fails": {
			input: input{
				secretName: secretName,
				appLabel:   appLabel,
				namespace:  namespace,
			},
			output: fmt.Errorf("unable to delete secret(s): %v", undeletedSecrets),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetSecretsByLabel(appLabel, false).Return(unwantedSecrets, nil)
				mockK8sClient.EXPECT().DeleteSecret(secretName,
					namespace).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedSecrets))
			},
		},
		"expect to pass when GetSecretsByLabel succeeds and RemoveMultipleServices succeeds": {
			input: input{
				secretName: secretName,
				appLabel:   appLabel,
				namespace:  namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetSecretsByLabel(appLabel, false).Return(unwantedSecrets, nil)
				mockK8sClient.EXPECT().DeleteSecret(secretName,
					namespace).Return(nil).
					MaxTimes(len(unwantedSecrets))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				secretName, appLabel, namespace := test.input.secretName, test.input.appLabel, test.input.namespace
				err := extendedK8sClient.DeleteTridentSecret(secretName, appLabel, namespace)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleSecrets(t *testing.T) {
	// arrange variables for the tests
	var emptySecretList, undeletedSecrets, unwantedSecrets []corev1.Secret
	var undeletedSecretDataList []string

	deleteSecretsErr := fmt.Errorf("could not delete secret")
	secretName := "encryptSecretName"
	secretNamespace := "secretNamespace"

	undeletedSecrets = []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		},
	}

	for _, secret := range undeletedSecrets {
		undeletedSecretDataList = append(undeletedSecretDataList, fmt.Sprintf("%v/%v", secret.Namespace,
			secret.Name))
	}

	unwantedSecrets = []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		},
	}

	tests := map[string]struct {
		input  []corev1.Secret
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no secrets": {
			input:  emptySecretList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedSecrets,
			output: fmt.Errorf("unable to delete secret(s): %v", undeletedSecretDataList),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteSecret(secretName,
					secretNamespace).Return(deleteSecretsErr).
					MaxTimes(len(undeletedSecrets))
			},
		},
		"expect to pass with valid secrets": {
			input:  unwantedSecrets,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteSecret(secretName,
					secretNamespace).Return(nil).
					MaxTimes(len(unwantedSecrets))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleSecrets(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetServiceInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validService, invalidService corev1.Service
	var unwantedServices, combinationsServices, emptyServices []corev1.Service

	label = "tridentCSILabel"
	name = getServiceName() // could be anything
	namespace = "default"
	invalidName = ""

	validService = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidService = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedServices = []corev1.Service{invalidService, invalidService}
	combinationsServices = append([]corev1.Service{validService}, append(combinationsServices,
		unwantedServices...)...)

	// setup input and output test types for easy use
	type input struct {
		label        string
		name         string
		namespace    string
		shouldUpdate bool
	}

	type output struct {
		currentService   *corev1.Service
		unwantedServices []corev1.Service
		createServices   bool
		err              error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label (string), args[1] = name (string), args[2] = namespace (string), args[3] = shouldUpdate (bool)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: true,
			},
			output: output{
				currentService:   nil,
				unwantedServices: nil,
				createServices:   true,
				err:              fmt.Errorf("unable to get list of services"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServicesByLabel(args[0], true).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no services found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentService:   nil,
				unwantedServices: nil,
				createServices:   true,
				err:              nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServicesByLabel(args[0], true).Return(emptyServices, nil)
				mockKubeClient.EXPECT().DeleteService(args[1], args[2]).Return(nil)
			},
		},
		"expect to pass with valid current services found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentService:   &validService,
				unwantedServices: unwantedServices,
				createServices:   false,
				err:              nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServicesByLabel(args[0], true).Return(combinationsServices, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, namespace, shouldUpdate := test.input.label, test.input.name,
					test.input.namespace, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentService, expectedUnwantedServices,
					expectedCreateService,
					expectedErr := test.output.currentService, test.output.unwantedServices,
					test.output.createServices, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = namespace, args[3] = shouldUpdate
				test.mocks(mockKubeClient, label, name, namespace, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentService, unwantedServices, createServices,
					err := extendedK8sClient.GetServiceInformation(name,
					label, namespace, shouldUpdate)

				assert.EqualValues(t, expectedCurrentService, currentService)
				assert.EqualValues(t, expectedUnwantedServices, unwantedServices)
				assert.Equal(t, len(expectedUnwantedServices), len(unwantedServices))
				assert.Equal(t, expectedCreateService, createServices)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestGetResourceQuotaInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, name, invalidName, namespace string
	var validResourceQuota, invalidResourceQuota corev1.ResourceQuota
	var unwantedResourceQuota, combinationResourceQuotas, emptyResourceQuotas []corev1.ResourceQuota

	label = "trident-csi"
	name = getResourceQuotaName()
	namespace = "default"
	invalidName = ""

	validResourceQuota = corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	invalidResourceQuota = corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}
	unwantedResourceQuota = []corev1.ResourceQuota{invalidResourceQuota, invalidResourceQuota}
	combinationResourceQuotas = append([]corev1.ResourceQuota{validResourceQuota}, append(combinationResourceQuotas,
		unwantedResourceQuota...)...)

	// setup input and output test types for easy use
	type input struct {
		label        string
		name         string
		namespace    string
		shouldUpdate bool
	}

	type output struct {
		currentResourceQuota   *corev1.ResourceQuota
		unwantedResourceQuotas []corev1.ResourceQuota
		createResourceQuota    bool
		err                    error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label (string), args[1] = name (string), args[2] = namespace (string), args[3] = shouldUpdate (bool)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: true,
			},
			output: output{
				currentResourceQuota:   nil,
				unwantedResourceQuotas: nil,
				createResourceQuota:    true,
				err:                    fmt.Errorf("unable to get list of resource quotas"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetResourceQuotasByLabel(args[0]).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no resource quotas found and no k8s error": {
			input: input{
				label:        label,
				name:         invalidName,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentResourceQuota:   nil,
				unwantedResourceQuotas: nil,
				createResourceQuota:    true,
				err:                    nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetResourceQuotasByLabel(args[0]).Return(emptyResourceQuotas, nil)
				// mockKubeClient.EXPECT().DeleteSecret(args[1], args[2]).Return(nil)
			},
		},
		"expect to pass with a valid current resource quota found and no k8s error": {
			input: input{
				label:        label,
				name:         name,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentResourceQuota:   &validResourceQuota,
				unwantedResourceQuotas: unwantedResourceQuota,
				createResourceQuota:    false,
				err:                    nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetResourceQuotasByLabel(args[0]).Return(combinationResourceQuotas, nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, name, namespace, shouldUpdate := test.input.label, test.input.name,
					test.input.namespace, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentResourceQuota, expectedUnwantedResourceQuotas,
					expectedCreateResourceQuota,
					expectedErr := test.output.currentResourceQuota, test.output.unwantedResourceQuotas,
					test.output.createResourceQuota, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = namespace, args[3] = shouldUpdate
				test.mocks(mockKubeClient, label, name, namespace, shouldUpdate)

				extendedK8sClient := &K8sClient{mockKubeClient}
				currentResourceQuota, unwantedResourceQuotas, createResourceQuota,
					err := extendedK8sClient.GetResourceQuotaInformation(name, label, namespace)

				assert.EqualValues(t, expectedCurrentResourceQuota, currentResourceQuota)
				assert.EqualValues(t, expectedUnwantedResourceQuotas, unwantedResourceQuotas)
				assert.Equal(t, len(expectedUnwantedResourceQuotas), len(unwantedResourceQuotas))
				assert.Equal(t, expectedCreateResourceQuota, createResourceQuota)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutResourceQuota(t *testing.T) {
	resourceQuotaName := getResourceQuotaName()
	resourceQuota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceQuotaName,
		},
	}
	appLabel := TridentCSILabel
	newResourceQuotaYAML := k8sclient.GetResourceQuotaYAML(
		resourceQuotaName,
		"trident",
		make(map[string]string, 0),
		make(map[string]string, 0),
	)
	k8sClientErr := fmt.Errorf("k8s client error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentResourceQuota *corev1.ResourceQuota
		createResourceQuota  bool
		newResourceQuotaYAML string
		appLabel             string
		patchBytes           []byte
		patchType            types.PatchType
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input       input
		errorExists bool
		// args[0] = newResourceQuotaYAML, args[1] = appLabel, args[2] = patchBytes, args[3] = patchType
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating a Resource Quota and no k8s error occurs": {
			input: input{
				currentResourceQuota: nil,
				createResourceQuota:  true,
				newResourceQuotaYAML: newResourceQuotaYAML,
				appLabel:             appLabel,
			},
			errorExists: false,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a Resource Quota and a k8s error occurs": {
			input: input{
				currentResourceQuota: nil,
				createResourceQuota:  true,
				newResourceQuotaYAML: newResourceQuotaYAML,
				appLabel:             appLabel,
			},
			errorExists: true,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientError)
			},
		},
		"expect to pass when createResourceQuota is false": {
			input: input{
				currentResourceQuota: resourceQuota,
				createResourceQuota:  false,
				newResourceQuotaYAML: newResourceQuotaYAML,
				appLabel:             appLabel,
			},
			errorExists: false,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// no calls to mock here
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchResourceQuotaByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a ResourceQuota and a k8s error occurs": {
			input: input{
				currentResourceQuota: resourceQuota,
				createResourceQuota:  false,
				newResourceQuotaYAML: newResourceQuotaYAML,
				appLabel:             appLabel,
			},
			errorExists: true,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchResourceQuotaByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentResourceQuota, createResourceQuota, newResourceQuotaYAML, appLabel := test.input.currentResourceQuota,
					test.input.createResourceQuota, test.input.newResourceQuotaYAML, test.input.appLabel

				if !createResourceQuota {
					if patchBytes, err = k8sclient.GenericPatch(currentResourceQuota,
						[]byte(newResourceQuotaYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.errorExists

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newResourceQuotaYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutResourceQuota(currentResourceQuota, createResourceQuota,
					newResourceQuotaYAML, appLabel)

				assert.Equal(t, expectedErr, err != nil)
			},
		)
	}
}

func TestDeleteTridentResourceQuota(t *testing.T) {
	// arrange variables for the tests
	var emptyResourceQuotaList, unwantedResourceQuotas []corev1.ResourceQuota
	var undeletedResourceQuotas []string

	getResourceQuotasErr := fmt.Errorf("unable to get list of resource quotas")
	resourceQuotaName := getResourceQuotaName()
	namespace := "namespace"
	appLabel := "trident-csi"

	unwantedResourceQuotas = []corev1.ResourceQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceQuotaName,
				Namespace: namespace,
			},
		},
	}

	for _, resourceQuota := range unwantedResourceQuotas {
		undeletedResourceQuotas = append(undeletedResourceQuotas, fmt.Sprintf("%v/%v", resourceQuota.Namespace,
			resourceQuota.Name))
	}

	tests := map[string]struct {
		input       string
		errorExists bool
		mocks       func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetResourceQuotasByLabel fails": {
			input:       appLabel,
			errorExists: true,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetResourceQuotasByLabel(appLabel).Return(nil, getResourceQuotasErr)
			},
		},
		"expect to pass when GetResourceQuotasByLabel returns no resource quotas": {
			input:       appLabel,
			errorExists: false,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetResourceQuotasByLabel(appLabel).Return(emptyResourceQuotaList, nil)
			},
		},
		"expect to fail when GetResourceQuotasByLabel succeeds but RemoveMultipleResourceQuotas fails": {
			input:       appLabel,
			errorExists: true,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetResourceQuotasByLabel(appLabel).Return(unwantedResourceQuotas, nil)
				mockK8sClient.EXPECT().DeleteResourceQuota(resourceQuotaName).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedResourceQuotas))
			},
		},
		"expect to pass when GetResourceQuotasByLabel succeeds and RemoveMultipleResourceQuotas succeeds": {
			input:       appLabel,
			errorExists: false,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetResourceQuotasByLabel(appLabel).Return(unwantedResourceQuotas, nil)
				mockK8sClient.EXPECT().DeleteResourceQuota(resourceQuotaName).Return(nil).
					MaxTimes(len(unwantedResourceQuotas))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.DeleteTridentResourceQuota(test.input)
				assert.Equal(t, test.errorExists, err != nil)
			},
		)
	}
}

func TestRemoveMultipleResourceQuotas(t *testing.T) {
	// arrange variables for the tests
	var emptyResourceQuotaList, unwantedResourceQuotas []corev1.ResourceQuota
	var undeletedResourceQuotas []string

	deleteDeploymentErr := fmt.Errorf("could not delete resource quota")
	resourceQuotaName := getResourceQuotaName()
	namespace := "namespace"

	unwantedResourceQuotas = []corev1.ResourceQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceQuotaName,
				Namespace: namespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceQuotaName,
				Namespace: namespace,
			},
		},
	}

	for _, resourceQuota := range unwantedResourceQuotas {
		undeletedResourceQuotas = append(undeletedResourceQuotas, fmt.Sprintf("%v/%v", resourceQuota.Namespace,
			resourceQuota.Name))
	}

	tests := map[string]struct {
		input       []corev1.ResourceQuota
		errorExists bool
		mocks       func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no resource quotas": {
			input:       emptyResourceQuotaList,
			errorExists: false,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:       unwantedResourceQuotas,
			errorExists: true,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteResourceQuota(resourceQuotaName).
					Return(deleteDeploymentErr).
					MaxTimes(len(unwantedResourceQuotas))
			},
		},
		"expect to pass with valid resource quotas": {
			input:       unwantedResourceQuotas,
			errorExists: false,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteResourceQuota(resourceQuotaName).
					Return(nil).
					MaxTimes(len(unwantedResourceQuotas))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleResourceQuotas(test.input)

				// check if we expect an error to exist; don't expect an exact match from the error string.
				assert.Equal(t, test.errorExists, err != nil)
			},
		)
	}
}

func TestPutService(t *testing.T) {
	serviceName := getServiceName()
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
	}
	appLabel := TridentCSILabel
	// version, _ := utils.ParseSemantic("1.21.3")
	newServiceYAML := k8sclient.GetCSIServiceYAML(
		serviceName,
		map[string]string{},
		map[string]string{},
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentService *corev1.Service
		createService  bool
		newServiceYAML string
		appLabel       string
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// arg[0] = newServiceYAML, arg[1] = appLabel, arg[2] = patchBytes, arg[3] = patchType,
		mocks func(*mockK8sClient.MockKubernetesClient, ...interface{})
	}{
		"expect to pass when creating a service and no k8s error occurs": {
			input: input{
				currentService: nil,
				createService:  true,
				newServiceYAML: newServiceYAML,
				appLabel:       appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().CreateObjectByYAML(args[0]).Return(nil)
			},
		},
		"expect to fail when creating a service and a k8s error occurs": {
			input: input{
				currentService: nil,
				createService:  true,
				newServiceYAML: newServiceYAML,
				appLabel:       appLabel,
			},
			output: fmt.Errorf("could not create Trident service; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().CreateObjectByYAML(args[0]).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a service and no k8s error occurs": {
			input: input{
				currentService: service,
				createService:  false,
				newServiceYAML: newServiceYAML,
				appLabel:       appLabel,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				// this ensures that the PatchServiceByLabel is called with the expected patchBytes
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchServiceByLabel(appLabel, patchBytesMatcher,
					patchType).Return(nil)
			},
		},
		"expect to fail when updating a service and a k8s error occurs": {
			input: input{
				currentService: service,
				createService:  false,
				newServiceYAML: newServiceYAML,
				appLabel:       appLabel,
			},
			output: fmt.Errorf("could not patch Trident Service; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				// this ensures that the PatchServiceByLabel is called with the expected patchBytes
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchServiceByLabel(appLabel, patchBytesMatcher,
					patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentService, createService, newServiceYAML, appLabel := test.input.currentService,
					test.input.createService, test.input.newServiceYAML, test.input.appLabel

				if !createService {
					if patchBytes, err = k8sclient.GenericPatch(currentService, []byte(newServiceYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr := test.output

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newServiceYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutService(currentService, createService, newServiceYAML, appLabel)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteTridentService(t *testing.T) {
	// arrange variables for the tests
	var emptyServiceList, unwantedServices []corev1.Service
	var undeletedServices []string

	getServicesErr := fmt.Errorf("unable to get list of services")
	serviceName := "serviceName"
	appLabel := "appLabel"
	namespace := "namespace"

	unwantedServices = []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
		},
	}

	for _, service := range unwantedServices {
		undeletedServices = append(undeletedServices, fmt.Sprintf("%v/%v", service.Namespace,
			service.Name))
	}

	type input struct {
		serviceName string
		appLabel    string
		namespace   string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetServicesByLabel fails": {
			input: input{
				serviceName: serviceName,
				appLabel:    appLabel,
				namespace:   namespace,
			},
			output: getServicesErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServicesByLabel(appLabel, true).Return(nil, getServicesErr)
			},
		},
		"expect to pass when GetServicesByLabel returns no services": {
			input: input{
				serviceName: serviceName,
				appLabel:    appLabel,
				namespace:   namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServicesByLabel(appLabel, true).Return(emptyServiceList, nil)
				mockK8sClient.EXPECT().DeleteService(serviceName, namespace).Return(nil)
			},
		},
		"expect to fail when GetServicesByLabel succeeds but RemoveMultipleServices fails": {
			input: input{
				serviceName: serviceName,
				appLabel:    appLabel,
				namespace:   namespace,
			},
			output: fmt.Errorf("unable to delete service(s): %v", undeletedServices),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServicesByLabel(appLabel, true).Return(unwantedServices, nil)
				mockK8sClient.EXPECT().DeleteService(serviceName,
					namespace).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedServices))
			},
		},
		"expect to pass when GetServicesByLabel succeeds and RemoveMultipleServices succeeds": {
			input: input{
				serviceName: serviceName,
				appLabel:    appLabel,
				namespace:   namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServicesByLabel(appLabel, true).Return(unwantedServices, nil)
				mockK8sClient.EXPECT().DeleteService(serviceName,
					namespace).Return(nil).
					MaxTimes(len(unwantedServices))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				serviceName, appLabel, namespace := test.input.serviceName, test.input.appLabel, test.input.namespace
				err := extendedK8sClient.DeleteTridentService(serviceName, appLabel, namespace)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleServices(t *testing.T) {
	// arrange variables for the tests
	var emptyServiceList, unwantedServices []corev1.Service
	var undeletedServices []string

	deleteSecretsErr := fmt.Errorf("could not delete service")
	serviceName := "serviceName"
	serviceNamespace := "serviceNamespace"

	unwantedServices = []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		},
	}

	for _, service := range unwantedServices {
		undeletedServices = append(undeletedServices, fmt.Sprintf("%v/%v", service.Namespace,
			service.Name))
	}

	tests := map[string]struct {
		input  []corev1.Service
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no services": {
			input:  emptyServiceList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  unwantedServices,
			output: fmt.Errorf("unable to delete service(s): %v", undeletedServices),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteService(serviceName,
					serviceNamespace).Return(deleteSecretsErr).
					MaxTimes(len(undeletedServices))
			},
		},
		"expect to pass with valid services": {
			input:  unwantedServices,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteService(serviceName,
					serviceNamespace).Return(nil).
					MaxTimes(len(unwantedServices))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleServices(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetMultipleServiceAccountInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var label, invalidName, namespace string
	var validServiceAccounts []corev1.ServiceAccount
	var invalidServiceAccount corev1.ServiceAccount
	var unwantedServiceAccounts, combinationServiceAccounts, emptyServiceAccounts []corev1.ServiceAccount

	expCurrentServiceAccountMap := make(map[string]*corev1.ServiceAccount)
	expServiceAccountSecretNamesMap := make(map[string][]string)
	expReuseServiceAccountMap := make(map[string]bool)

	label = "tridentCSILabel"
	serviceAccountNames := getRBACResourceNames()
	namespace = "default"
	invalidName = ""

	for _, name := range serviceAccountNames {
		validServiceAccount := corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Secrets: []corev1.ObjectReference{
				{
					Name: name + "-one",
				},
				{
					Name: name + "-two",
				},
			},
		}
		validServiceAccounts = append(validServiceAccounts, validServiceAccount)
		expCurrentServiceAccountMap[name] = &validServiceAccount
		expServiceAccountSecretNamesMap[name] = []string{name + "-one", name + "-two"}
		expReuseServiceAccountMap[name] = true
	}

	invalidServiceAccount = corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      invalidName,
			Namespace: namespace,
		},
	}

	unwantedServiceAccounts = []corev1.ServiceAccount{invalidServiceAccount, invalidServiceAccount}
	combinationServiceAccounts = append(combinationServiceAccounts, unwantedServiceAccounts...)
	combinationServiceAccounts = append(combinationServiceAccounts, validServiceAccounts...)

	// setup input and output test types for easy use
	type input struct {
		label        string
		names        []string
		namespace    string
		shouldUpdate bool
	}

	type output struct {
		currentServiceAccountMap     map[string]*corev1.ServiceAccount
		unwantedServiceAccounts      []corev1.ServiceAccount
		serviceAccountSecretNamesMap map[string][]string
		reuseServiceAccountMap       map[string]bool
		err                          error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = label (string), args[1] = name (string), args[2] = namespace (string), args[3] = shouldUpdate (bool)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				label:        label,
				names:        serviceAccountNames,
				namespace:    namespace,
				shouldUpdate: true,
			},
			output: output{
				currentServiceAccountMap:     make(map[string]*corev1.ServiceAccount),
				unwantedServiceAccounts:      nil,
				serviceAccountSecretNamesMap: make(map[string][]string),
				reuseServiceAccountMap:       make(map[string]bool),
				err:                          fmt.Errorf("unable to get list of service accounts"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServiceAccountsByLabel(args[0], false).Return(nil,
					fmt.Errorf(""))
			},
		},
		"expect to pass with no service accounts found and no k8s error": {
			input: input{
				label:        label,
				names:        []string{invalidName},
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentServiceAccountMap:     make(map[string]*corev1.ServiceAccount),
				unwantedServiceAccounts:      nil,
				serviceAccountSecretNamesMap: make(map[string][]string),
				reuseServiceAccountMap:       make(map[string]bool),
				err:                          nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServiceAccountsByLabel(args[0], false).Return(emptyServiceAccounts, nil)
				for _, arg := range (args[1]).([]string) {
					mockKubeClient.EXPECT().DeleteServiceAccount(arg, args[2], args[3]).Return(nil)
				}
			},
		},
		"expect to pass with valid current service accounts found and no k8s error": {
			input: input{
				label:        label,
				names:        serviceAccountNames,
				namespace:    namespace,
				shouldUpdate: false,
			},
			output: output{
				currentServiceAccountMap:     expCurrentServiceAccountMap,
				unwantedServiceAccounts:      unwantedServiceAccounts,
				serviceAccountSecretNamesMap: expServiceAccountSecretNamesMap,
				reuseServiceAccountMap:       expReuseServiceAccountMap,
				err:                          nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				mockKubeClient.EXPECT().GetServiceAccountsByLabel(args[0], false).Return(combinationServiceAccounts,
					nil)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				label, accountNames, namespace, shouldUpdate := test.input.label, test.input.names,
					test.input.namespace, test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedCurrentServiceAccountMap, expectedUnwantedServiceAccounts,
					expectedServiceAccountSecretNamesMap, expectedReuseServiceAccountMap,
					expectedErr := test.output.currentServiceAccountMap, test.output.unwantedServiceAccounts,
					test.output.serviceAccountSecretNamesMap, test.output.reuseServiceAccountMap, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = namespace, args[3] = shouldUpdate
				test.mocks(mockKubeClient, label, accountNames, namespace, shouldUpdate)
				extendedK8sClient := &K8sClient{mockKubeClient}
				currentServiceAccountMap, unwantedServiceAccounts, serviceAccountSecretNamesMap, reuseServiceAccountMap,
					err := extendedK8sClient.GetMultipleServiceAccountInformation(accountNames,
					label, namespace, shouldUpdate)

				assert.True(t, cmp.Equal(expectedCurrentServiceAccountMap, currentServiceAccountMap),
					cmp.Diff(expectedCurrentServiceAccountMap, currentServiceAccountMap))
				assert.True(t, cmp.Equal(expectedUnwantedServiceAccounts, unwantedServiceAccounts),
					cmp.Diff(expectedUnwantedServiceAccounts, unwantedServiceAccounts))
				assert.Equal(t, len(expectedUnwantedServiceAccounts), len(unwantedServiceAccounts))
				assert.True(t, cmp.Equal(expectedServiceAccountSecretNamesMap, serviceAccountSecretNamesMap),
					cmp.Diff(expectedServiceAccountSecretNamesMap, serviceAccountSecretNamesMap))
				assert.Equal(t, len(expectedServiceAccountSecretNamesMap), len(serviceAccountSecretNamesMap))
				assert.Equal(t, expectedReuseServiceAccountMap, reuseServiceAccountMap)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestPutServiceAccount(t *testing.T) {
	serviceAccountName := getServiceAccountName(true)
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountName,
		},
	}
	appLabel := TridentCSILabel
	newServiceAccountYAML := k8sclient.GetServiceAccountYAML(
		serviceAccountName,
		[]string{},
		map[string]string{},
		map[string]string{},
	)
	k8sClientErr := fmt.Errorf("k8s error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentServiceAccount *corev1.ServiceAccount
		reuseServiceAccount   bool
		newServiceAccountYAML string
		appLabel              string
	}

	type output struct {
		err               error
		newServiceAccount bool
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// arg[0] = newServiceAccountYAML, arg[1] = appLabel, arg[2] = patchBytes, arg[3] = patchType,
		mocks func(*mockK8sClient.MockKubernetesClient, ...interface{})
	}{
		"expect to pass when creating a service account and no k8s error occurs": {
			input: input{
				currentServiceAccount: &corev1.ServiceAccount{},
				reuseServiceAccount:   false,
				newServiceAccountYAML: newServiceAccountYAML,
				appLabel:              appLabel,
			},
			output: output{
				err:               nil,
				newServiceAccount: true,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML, _ := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(nil)
			},
		},
		"expect to fail when creating a service account and a k8s error occurs": {
			input: input{
				currentServiceAccount: &corev1.ServiceAccount{},
				reuseServiceAccount:   false,
				newServiceAccountYAML: newServiceAccountYAML,
				appLabel:              appLabel,
			},
			output: output{
				err:               fmt.Errorf("could not create service account; %v", k8sClientErr),
				newServiceAccount: false,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				objectYAML, _ := args[0].(string)
				mockKubeClient.EXPECT().CreateObjectByYAML(objectYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating a service account and no k8s error occurs": {
			input: input{
				currentServiceAccount: serviceAccount,
				reuseServiceAccount:   true,
				newServiceAccountYAML: newServiceAccountYAML,
				appLabel:              appLabel,
			},
			output: output{
				err:               nil,
				newServiceAccount: false,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				// this ensures that the PatchServiceByLabel is called with the expected patchBytes
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchServiceAccountByLabelAndName(appLabel, serviceAccountName,
					patchBytesMatcher, patchType).Return(nil)
			},
		},
		"expect to fail when updating a service account and a k8s error occurs": {
			input: input{
				currentServiceAccount: serviceAccount,
				reuseServiceAccount:   true,
				newServiceAccountYAML: newServiceAccountYAML,
				appLabel:              appLabel,
			},
			output: output{
				err:               fmt.Errorf("could not patch service account; %v", k8sClientErr),
				newServiceAccount: false,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				appLabel, _ := args[1].(string)
				patchBytes, _ := args[2].([]byte)
				patchType, _ := args[3].(types.PatchType)
				// this ensures that the PatchServiceByLabel is called with the expected patchBytes
				patchBytesMatcher := &JSONMatcher{patchBytes}
				mockKubeClient.EXPECT().PatchServiceAccountByLabelAndName(appLabel, serviceAccountName,
					patchBytesMatcher, patchType).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte
				var patchType types.PatchType

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentServiceAccount, reuseServiceAccount,
					newServiceAccountYAML, appLabel := test.input.currentServiceAccount, test.input.reuseServiceAccount,
					test.input.newServiceAccountYAML, test.input.appLabel

				if reuseServiceAccount {
					if patchBytes, err = k8sclient.GenericPatch(currentServiceAccount,
						[]byte(newServiceAccountYAML)); err != nil {
						t.Fatal(err)
					}
					patchType = types.MergePatchType
				}

				// extract the output err
				expectedErr, expectedServiceAccount := test.output.err, test.output.newServiceAccount

				// mock out calls found in every test case
				mockKubeClient.EXPECT().Namespace().AnyTimes()

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, newServiceAccountYAML, appLabel, patchBytes, patchType)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				newServiceAccount, err := extendedK8sClient.PutServiceAccount(currentServiceAccount,
					reuseServiceAccount, newServiceAccountYAML, appLabel)

				assert.Equal(t, expectedServiceAccount, newServiceAccount)
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteMultipleTridentServiceAccounts(t *testing.T) {
	// arrange variables for the tests
	var emptyServiceAccountList, unwantedServiceAccounts []corev1.ServiceAccount
	var undeletedServiceAccounts []string

	getServiceAccountsErr := fmt.Errorf("unable to get list of service accounts")
	serviceAccountNames := getRBACResourceNames()
	appLabel := "appLabel"
	namespace := "namespace"

	for _, serviceAccountName := range serviceAccountNames {
		unwantedServiceAccount := corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		}
		unwantedServiceAccounts = append(unwantedServiceAccounts, unwantedServiceAccount)
	}

	for _, serviceAccount := range unwantedServiceAccounts {
		undeletedServiceAccounts = append(undeletedServiceAccounts, fmt.Sprintf("%v/%v", serviceAccount.Namespace,
			serviceAccount.Name))
	}

	type input struct {
		serviceAccountNames []string
		appLabel            string
		namespace           string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetServiceAccountsByLabel fails": {
			input: input{
				serviceAccountNames: serviceAccountNames,
				appLabel:            appLabel,
				namespace:           namespace,
			},
			output: getServiceAccountsErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServiceAccountsByLabel(appLabel, false).Return(nil, getServiceAccountsErr)
			},
		},
		"expect to pass when GetServiceAccountsByLabel returns no secrets": {
			input: input{
				serviceAccountNames: serviceAccountNames,
				appLabel:            appLabel,
				namespace:           namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServiceAccountsByLabel(appLabel, false).Return(emptyServiceAccountList, nil)
				for _, name := range serviceAccountNames {
					mockK8sClient.EXPECT().DeleteServiceAccount(name, namespace, false).Return(nil)
				}
			},
		},
		"expect to fail when GetServiceAccountsByLabel succeeds but RemoveMultipleServiceAccounts fails": {
			input: input{
				serviceAccountNames: serviceAccountNames,
				appLabel:            appLabel,
				namespace:           namespace,
			},
			output: fmt.Errorf("unable to delete service account(s): %v", undeletedServiceAccounts),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServiceAccountsByLabel(appLabel, false).Return(unwantedServiceAccounts, nil)
				for _, name := range serviceAccountNames {
					mockK8sClient.EXPECT().DeleteServiceAccount(name,
						namespace, true).Return(fmt.Errorf("")).
						MaxTimes(len(unwantedServiceAccounts))
				}
			},
		},
		"expect to pass when GetServiceAccountsByLabel succeeds and RemoveMultipleServiceAccounts succeeds": {
			input: input{
				serviceAccountNames: serviceAccountNames,
				appLabel:            appLabel,
				namespace:           namespace,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetServiceAccountsByLabel(appLabel, false).Return(unwantedServiceAccounts, nil)
				for _, name := range serviceAccountNames {
					mockK8sClient.EXPECT().DeleteServiceAccount(name,
						namespace, true).Return(nil).
						MaxTimes(len(unwantedServiceAccounts))
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				serviceAccountNames, appLabel, namespace := test.input.serviceAccountNames, test.input.appLabel, test.input.namespace
				err := extendedK8sClient.DeleteMultipleTridentServiceAccounts(serviceAccountNames, appLabel, namespace)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleServiceAccounts(t *testing.T) {
	// arrange variables for the tests
	var emptyServiceAccountList, undeletedServiceAccounts, unwantedServiceAccounts []corev1.ServiceAccount
	var undeletedServiceDataList []string

	deleteSecretsErr := fmt.Errorf("could not delete service")
	serviceName := "serviceName"
	serviceNamespace := "serviceNamespace"

	undeletedServiceAccounts = []corev1.ServiceAccount{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		},
	}

	for _, service := range undeletedServiceAccounts {
		undeletedServiceDataList = append(undeletedServiceDataList, fmt.Sprintf("%v/%v", service.Namespace,
			service.Name))
	}

	unwantedServiceAccounts = []corev1.ServiceAccount{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		},
	}

	tests := map[string]struct {
		input  []corev1.ServiceAccount
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no service accounts": {
			input:  emptyServiceAccountList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedServiceAccounts,
			output: fmt.Errorf("unable to delete service account(s): %v", undeletedServiceDataList),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteServiceAccount(serviceName,
					serviceNamespace, true).Return(deleteSecretsErr).
					MaxTimes(len(undeletedServiceAccounts))
			},
		},
		"expect to pass with valid service accounts": {
			input:  unwantedServiceAccounts,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteServiceAccount(serviceName,
					serviceNamespace, true).Return(nil).
					MaxTimes(len(unwantedServiceAccounts))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleServiceAccounts(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestGetMultipleTridentOpenShiftSCCInformation(t *testing.T) {
	// declare and initialize variables used throughout the test cases
	var names, usernames []string

	names = getRBACResourceNames()
	usernames = getRBACResourceNames()

	expCurrentOpenShiftSCCJSONMap := make(map[string][]byte)
	expReuseOpenShiftSCCMap := make(map[string]bool)
	expRemoveExistingSCCMap := make(map[string]bool)

	for idx := 0; idx < len(names); idx++ {
		expCurrentOpenShiftSCCJSONMap[names[idx]] = []byte(k8sclient.GetOpenShiftSCCYAML(names[idx], usernames[idx],
			"default", nil, nil, false))
		expReuseOpenShiftSCCMap[names[idx]] = true
		expRemoveExistingSCCMap[names[idx]] = true
	}

	// setup input and output test types for easy use
	type input struct {
		names        []string
		usernames    []string
		shouldUpdate bool
	}

	type output struct {
		currentOpenShiftSCCJSONMap map[string][]byte
		reuseOpenShiftSCCMap       map[string]bool
		removeExistingSCCMap       map[string]bool
		err                        error
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output output
		// args[0] = username (string), args[1] = name (string)
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to fail with k8s error": {
			input: input{
				names:        names,
				usernames:    usernames,
				shouldUpdate: true,
			},
			output: output{
				currentOpenShiftSCCJSONMap: make(map[string][]byte),
				reuseOpenShiftSCCMap:       make(map[string]bool),
				removeExistingSCCMap:       make(map[string]bool),
				err:                        fmt.Errorf("unable to get OpenShift SCC for Trident"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				names, _ := args[0].([]string)
				usernames, _ := args[1].([]string)
				mockKubeClient.EXPECT().GetOpenShiftSCCByName(usernames[0], names[0]).Return(false, false, []byte{},
					fmt.Errorf(""))
			},
		},
		"expect to pass with no openshift scc found, no k8s error, and an scc user does not exist": {
			input: input{
				names:        names,
				usernames:    usernames,
				shouldUpdate: true,
			},
			output: output{
				currentOpenShiftSCCJSONMap: make(map[string][]byte),
				reuseOpenShiftSCCMap:       make(map[string]bool),
				removeExistingSCCMap:       expRemoveExistingSCCMap,
				err:                        nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				// SCCExist, SCCUserExist, jsonData, err
				// when SCCUserExist = false, removeExistingSCC = true
				names, _ := args[0].([]string)
				usernames, _ := args[1].([]string)
				for idx := 0; idx < len(names); idx++ {
					mockKubeClient.EXPECT().GetOpenShiftSCCByName(usernames[idx], names[idx]).Return(
						true, false, expCurrentOpenShiftSCCJSONMap[names[idx]], nil)
				}
			},
		},
		"expect to pass with no openshift scc found, no k8s error, and a it should update": {
			input: input{
				names:        names,
				usernames:    usernames,
				shouldUpdate: true,
			},
			output: output{
				currentOpenShiftSCCJSONMap: make(map[string][]byte),
				reuseOpenShiftSCCMap:       make(map[string]bool),
				removeExistingSCCMap:       expRemoveExistingSCCMap,
				err:                        nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				names, _ := args[0].([]string)
				usernames, _ := args[1].([]string)
				for idx := 0; idx < len(names); idx++ {
					mockKubeClient.EXPECT().GetOpenShiftSCCByName(usernames[idx], names[idx]).Return(
						true, true, expCurrentOpenShiftSCCJSONMap[names[idx]], nil)
				}
			},
		},
		"expect to pass with valid current services found and no k8s error": {
			input: input{
				names:        names,
				usernames:    usernames,
				shouldUpdate: false,
			},
			output: output{
				currentOpenShiftSCCJSONMap: expCurrentOpenShiftSCCJSONMap,
				reuseOpenShiftSCCMap:       expReuseOpenShiftSCCMap,
				removeExistingSCCMap:       make(map[string]bool),
				err:                        nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				names, _ := args[0].([]string)
				usernames, _ := args[1].([]string)
				for idx := 0; idx < len(names); idx++ {
					mockKubeClient.EXPECT().GetOpenShiftSCCByName(usernames[idx], names[idx]).Return(
						true, true, expCurrentOpenShiftSCCJSONMap[names[idx]], nil)
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				names, usernames, shouldUpdate := test.input.names, test.input.usernames,
					test.input.shouldUpdate

				// extract the output variables from the test case definition
				expectedOpenShiftSCCJSONMap, expectedReuseOpenShiftSCCMap, expectedRemoveExistingSCCMap,
					expectedErr := test.output.currentOpenShiftSCCJSONMap, test.output.reuseOpenShiftSCCMap,
					test.output.removeExistingSCCMap, test.output.err

				// mock out the k8s client calls needed to test this
				// args[0] = label, args[1] = name, args[2] = namespace, args[3] = shouldUpdate
				test.mocks(mockKubeClient, usernames, names)

				extendedK8sClient := &K8sClient{mockKubeClient}
				currentOpenShiftSCCJSONMap, reuseOpenShiftSCCMap, removeExistingSCCMap,
					err := extendedK8sClient.GetMultipleTridentOpenShiftSCCInformation(names, usernames,
					shouldUpdate)

				assert.True(t, cmp.Equal(expectedOpenShiftSCCJSONMap, currentOpenShiftSCCJSONMap),
					cmp.Diff(expectedOpenShiftSCCJSONMap, currentOpenShiftSCCJSONMap))
				assert.True(t, cmp.Equal(expectedReuseOpenShiftSCCMap, reuseOpenShiftSCCMap),
					cmp.Diff(expectedReuseOpenShiftSCCMap, reuseOpenShiftSCCMap))
				assert.True(t, cmp.Equal(expectedRemoveExistingSCCMap, removeExistingSCCMap),
					cmp.Diff(expectedRemoveExistingSCCMap, removeExistingSCCMap))
				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestExecPodForVersionInformation(t *testing.T) {
	podName := "trident-transient-pod"
	cmd := []string{"/bin/tridentctl", "version", "--client", "-o", "yaml"}
	timeout := 5 * time.Millisecond
	validExecOutput := []byte{
		116, 114, 105, 100, 101, 110, 116,
	}
	k8sClientErr := fmt.Errorf("any k8s client error")

	type input struct {
		podName string
		cmd     []string
		timeout time.Duration
	}

	type output struct {
		execOutput []byte
		err        error
	}

	tests := map[string]struct {
		input  input
		output output
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient, podName string, cmd []string)
	}{
		"expect to fail with no supplied cmd to exec": {
			input: input{
				podName: podName,
				cmd:     []string{},
				timeout: timeout,
			},
			output: output{
				execOutput: nil,
				err:        fmt.Errorf("no command supplied"),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, podName string, cmd []string) {
				// no mocks for this case
			},
		},
		"expect to fail with k8s error when exec the supplied cmd": {
			input: input{
				podName: podName,
				cmd:     cmd,
				timeout: timeout,
			},
			output: output{
				execOutput: []byte{},
				err:        fmt.Errorf("exec error; %v", k8sClientErr),
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, podName string, cmd []string) {
				mockKubeClient.EXPECT().Exec(podName, "", cmd).Return([]byte{}, k8sClientErr).AnyTimes()
			},
		},
		"expect to pass with no error when exec the supplied cmd": {
			input: input{
				podName: podName,
				cmd:     cmd,
				timeout: timeout,
			},
			output: output{
				execOutput: validExecOutput,
				err:        nil,
			},
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, podName string, cmd []string) {
				mockKubeClient.EXPECT().Exec(podName, "", cmd).Return(validExecOutput, nil).AnyTimes()
			},
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				podName, cmd, timeout := test.input.podName, test.input.cmd, test.input.timeout

				// extract the output variables from the test case definition
				expectedExecOutput, expectedErr := test.output.execOutput, test.output.err

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, podName, cmd)
				extendedK8sClient := &K8sClient{mockKubeClient}
				actualExecOutput, actualErr := extendedK8sClient.ExecPodForVersionInformation(podName, cmd, timeout)
				assert.Equal(t, actualExecOutput, expectedExecOutput)
				assert.Equal(t, actualErr, expectedErr)
			},
		)
	}
}

func TestGetCSISnapshotterVersion(t *testing.T) {
	var emptyDeployment, validDeployment, invalidDeployment *appsv1.Deployment

	validDeployment = &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "csi-snapshotter",
							Image: "csi-snapshotter:v4",
						},
					},
				},
			},
		},
	}

	invalidDeployment = &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "csi-snapshotter",
							Image: "csi-snapshotter",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{},
	}

	// input is currentDeployment and output is the snapshotCRDVersion
	tests := map[string]struct {
		input  *appsv1.Deployment
		output string
		mocks  func(mockKubeClient *mockK8sClient.MockKubernetesClient)
	}{
		"expect snapshot crd v1 with empty deployment": {
			input:  emptyDeployment,
			output: "v1",
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				mockKubeClient.EXPECT().GetSnapshotterCRDVersion().Return("v1")
			},
		},
		"expect snapshot crd v1 with deployment containers containing a snapshotter:v4 image": {
			input:  validDeployment,
			output: "v1",
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				mockKubeClient.EXPECT().GetSnapshotterCRDVersion().Return("")
			},
		},
		"expect empty snapshot version wwhen GetSnapshotterCRDVersion returns an empty string": {
			input:  invalidDeployment,
			output: "",
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient) {
				mockKubeClient.EXPECT().GetSnapshotterCRDVersion().Return("")
			},
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				snapshotCRDName := extendedK8sClient.GetCSISnapshotterVersion(test.input)
				assert.Equal(t, test.output, snapshotCRDName)
			},
		)
	}
}

func TestDeleteTridentStatefulSet(t *testing.T) {
	// arrange variables for the tests
	var emptyStatefulSets, unwantedStatefulSets []appsv1.StatefulSet
	var undeletedStatefulSets []string

	getStatefulSetErr := fmt.Errorf("unable to get list of statefulsets")
	appLabel := "trident-app-label"
	statefulSetName := "statefulSetName"
	namespace := "namespace"

	unwantedStatefulSets = []appsv1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      statefulSetName,
				Namespace: namespace,
			},
		},
	}

	for _, statefulSet := range unwantedStatefulSets {
		undeletedStatefulSets = append(undeletedStatefulSets, fmt.Sprintf("%v/%v", statefulSet.Namespace,
			statefulSet.Name))
	}

	tests := map[string]struct {
		input  string
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetStatefulSetsByLabel fails": {
			input:  appLabel,
			output: getStatefulSetErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetStatefulSetsByLabel(appLabel, true).Return(nil, getStatefulSetErr)
			},
		},
		"expect to pass when GetStatefulSetsByLabel returns no statefulsets": {
			input:  appLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetStatefulSetsByLabel(appLabel, true).Return(emptyStatefulSets, nil)
			},
		},
		"expect to fail when GetStatefulSetsByLabel succeeds but RemoveMultipleStatefulSets fails": {
			input:  appLabel,
			output: fmt.Errorf("unable to delete Statefulset(s): %v", undeletedStatefulSets),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetStatefulSetsByLabel(appLabel, true).Return(unwantedStatefulSets, nil)
				mockK8sClient.EXPECT().DeleteStatefulSet(statefulSetName,
					namespace).Return(fmt.Errorf("")).
					MaxTimes(len(unwantedStatefulSets))
			},
		},
		"expect to pass when GetStatefulSetsByLabel succeeds and RemoveMultipleStatefulSets succeeds": {
			input:  appLabel,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().GetStatefulSetsByLabel(appLabel, true).Return(unwantedStatefulSets, nil)
				mockK8sClient.EXPECT().DeleteStatefulSet(statefulSetName,
					namespace).Return(nil).
					MaxTimes(len(unwantedStatefulSets))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.DeleteTridentStatefulSet(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestPutOpenShiftSCC(t *testing.T) {
	// arrange variables for the tests
	openShiftSCCUserName := getOpenShiftSCCUserName()
	openShiftSCCName := getOpenShiftSCCName()
	newOpenShiftSCCYAML := k8sclient.GetOpenShiftSCCYAML(
		openShiftSCCUserName,
		openShiftSCCName,
		"trident",
		make(map[string]string),
		make(map[string]string),
		false,
	)

	currentOpenShiftSCCJSON, err := yaml.YAMLToJSON([]byte(k8sclient.GetOpenShiftSCCYAML(
		openShiftSCCUserName+"old",
		openShiftSCCName+"old",
		"trident",
		make(map[string]string),
		make(map[string]string),
		false,
	)))
	if err != nil {
		t.Fatal("GetOpenShiftSCCYAML() returned invalid YAML")
	}

	k8sClientErr := fmt.Errorf("k8s error")
	// genericErr := fmt.Errorf("error")

	// defining a custom input type makes testing different cases easier
	type input struct {
		currentOpenShiftSCCJSON []byte
		reuseOpenShiftSCC       bool
		newOpenShiftSCCYAML     string
	}

	// setup values for the test table with input, expected output, and mocks
	tests := map[string]struct {
		input  input
		output error
		// args[0] = openShiftSCCOldUserName, args[1] = openShiftSCCOldName, args[2] = newOpenShiftSCCYAML,
		// args[3] = patchBytes,
		mocks func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{})
	}{
		"expect to pass when creating OpenShift SCCs and no k8s error occurs when removing Trident users from" +
			" OpenShiftSCC": {
			input: input{
				currentOpenShiftSCCJSON: []byte{},
				reuseOpenShiftSCC:       false,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				openShiftSCCOldUserName, openShiftSCCOldName, newOpenShiftSCCYAML := args[0], args[1], args[2]
				mockKubeClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName,
					openShiftSCCOldName).Return(nil)
				mockKubeClient.EXPECT().CreateObjectByYAML(newOpenShiftSCCYAML).Return(nil)
			},
		},
		"expect to pass when creating OpenShift SCCs and a k8s error occurs when removing Trident users from" +
			" OpenShiftSCCC": {
			input: input{
				currentOpenShiftSCCJSON: []byte{},
				reuseOpenShiftSCC:       false,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				openShiftSCCOldUserName, openShiftSCCOldName, newOpenShiftSCCYAML := args[0], args[1], args[2]
				// even though this call fails, we can still continue to create OpenShiftSCC
				mockKubeClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName,
					openShiftSCCOldName).Return(k8sClientErr)
				mockKubeClient.EXPECT().CreateObjectByYAML(newOpenShiftSCCYAML).Return(nil)
			},
		},
		"expect to pass when creating OpenShift SCCs and no k8s error occurs": {
			input: input{
				currentOpenShiftSCCJSON: []byte{},
				reuseOpenShiftSCC:       false,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				openShiftSCCOldUserName, openShiftSCCOldName, newOpenShiftSCCYAML := args[0], args[1], args[2]
				// even though this call fails, we can still continue to create OpenShiftSCC
				mockKubeClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName,
					openShiftSCCOldName).Return(nil)
				mockKubeClient.EXPECT().CreateObjectByYAML(newOpenShiftSCCYAML).Return(nil)
			},
		},
		"expect to fail when creating OpenShift SCCs and a k8s error occurs": {
			input: input{
				currentOpenShiftSCCJSON: []byte{},
				reuseOpenShiftSCC:       false,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: fmt.Errorf("could not create OpenShift SCC; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				openShiftSCCOldUserName, openShiftSCCOldName, newOpenShiftSCCYAML := args[0], args[1], args[2]
				// even though this call fails, we can still continue to create OpenShiftSCC
				mockKubeClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName,
					openShiftSCCOldName).Return(nil)
				mockKubeClient.EXPECT().CreateObjectByYAML(newOpenShiftSCCYAML).Return(k8sClientErr)
			},
		},
		"expect to pass when updating OpenShift SCCs and no k8s error occurs": {
			input: input{
				currentOpenShiftSCCJSON: currentOpenShiftSCCJSON,
				reuseOpenShiftSCC:       true,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: nil,
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				var jsonMatcher *JSONMatcher
				// args[3] is patchBytes, but we need ot assert the type and use a JSON matcher to avoid false negatives
				if patchBytes, ok := args[3].([]byte); !ok {
					t.Fatal("invalid patchBytes!")
				} else {
					jsonMatcher = &JSONMatcher{patchBytes}
				}
				mockKubeClient.EXPECT().PatchOpenShiftSCC(jsonMatcher).Return(nil)
			},
		},
		"expect to fail when updating OpenShift SCCs and a k8s error occurs": {
			input: input{
				currentOpenShiftSCCJSON: currentOpenShiftSCCJSON,
				reuseOpenShiftSCC:       true,
				newOpenShiftSCCYAML:     newOpenShiftSCCYAML,
			},
			output: fmt.Errorf("could not patch Trident OpenShift SCC; %v", k8sClientErr),
			mocks: func(mockKubeClient *mockK8sClient.MockKubernetesClient, args ...interface{}) {
				// mock calls here
				var jsonMatcher *JSONMatcher
				// args[3] is patchBytes, but we need ot assert the type and use a JSON matcher to avoid false negatives
				if patchBytes, ok := args[3].([]byte); !ok {
					t.Fatal("invalid patchBytes!")
				} else {
					jsonMatcher = &JSONMatcher{patchBytes}
				}
				mockKubeClient.EXPECT().PatchOpenShiftSCC(jsonMatcher).Return(k8sClientErr)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				var err error
				var patchBytes []byte

				openShiftSCCOldUserName := "trident-csi"
				openShiftSCCOldName := "privileged"

				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// extract the input variables from the test case definition
				currentOpenShiftSCCJSON, reuseOpenShiftSCC, newOpenShiftSCCYAML := test.input.currentOpenShiftSCCJSON,
					test.input.reuseOpenShiftSCC, test.input.newOpenShiftSCCYAML

				if reuseOpenShiftSCC {
					// Convert new object from YAML to JSON format
					modifiedJSON, err := yaml.YAMLToJSON([]byte(newOpenShiftSCCYAML))
					if err != nil {
						t.Fatal(err)
					}

					if patchBytes, err = jsonpatch.MergePatch(currentOpenShiftSCCJSON, modifiedJSON); err != nil {
						t.Fatal(err)
					}
				}

				// extract the output err
				expectedErr := test.output

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient, openShiftSCCOldUserName, openShiftSCCOldName, newOpenShiftSCCYAML,
					patchBytes)
				extendedK8sClient := &K8sClient{mockKubeClient}

				// make the call
				err = extendedK8sClient.PutOpenShiftSCC(currentOpenShiftSCCJSON, reuseOpenShiftSCC,
					newOpenShiftSCCYAML)

				assert.Equal(t, expectedErr, err)
			},
		)
	}
}

func TestDeleteMultipleOpenShiftSCC(t *testing.T) {
	// arrange variables for the tests
	openShiftSCCNames := []string{"trident"}
	openShiftSCCUserNames := []string{"trident"}
	appLabel := "trident"
	getOpenShiftSCCByNameErr := fmt.Errorf("unable to get OpenShift SCC for Trident")
	deleteObjectByYAMLErr := fmt.Errorf("couldn't delete object by yaml")

	type input struct {
		usernames []string
		names     []string
		label     string
	}

	tests := map[string]struct {
		input  input
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to fail when GetOpenShiftSCCByName fails": {
			input: input{
				usernames: openShiftSCCUserNames,
				names:     openShiftSCCNames,
				label:     appLabel,
			},
			output: getOpenShiftSCCByNameErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				openShiftQueryYAML := k8sclient.GetOpenShiftSCCQueryYAML(getOpenShiftSCCName())
				mockK8sClient.EXPECT().DeleteObjectByYAML(openShiftQueryYAML, true).Return(nil)
				mockK8sClient.EXPECT().GetOpenShiftSCCByName(
					openShiftSCCUserNames[0], openShiftSCCUserNames[0]).Return(false, false, []byte{}, fmt.Errorf(""))
			},
		},
		"expect to fail when GetOpenShiftSCCByName succeeds but DeleteObjectByYAML fails": {
			input: input{
				usernames: openShiftSCCUserNames,
				names:     openShiftSCCNames,
				label:     appLabel,
			},
			output: deleteObjectByYAMLErr,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				openShiftQueryYAML := k8sclient.GetOpenShiftSCCQueryYAML(getOpenShiftSCCName())
				mockK8sClient.EXPECT().DeleteObjectByYAML(openShiftQueryYAML, true).Return(nil)
				mockK8sClient.EXPECT().GetOpenShiftSCCByName(openShiftSCCNames[0], openShiftSCCNames[0]).Return(true,
					false, []byte{}, nil)

				openShiftQueryYAML = k8sclient.GetOpenShiftSCCQueryYAML(openShiftSCCNames[0])
				mockK8sClient.EXPECT().DeleteObjectByYAML(openShiftQueryYAML, true).Return(deleteObjectByYAMLErr)
			},
		},
		"expect to fail when GetOpenShiftSCCByName succeeds and RemoveTridentUserFromOpenShiftSCC is called": {
			input: input{
				usernames: openShiftSCCUserNames,
				names:     openShiftSCCNames,
				label:     appLabel,
			},
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				openShiftQueryYAML := k8sclient.GetOpenShiftSCCQueryYAML(getOpenShiftSCCName())
				mockK8sClient.EXPECT().DeleteObjectByYAML(openShiftQueryYAML, true).Return(nil)
				mockK8sClient.EXPECT().GetOpenShiftSCCByName(openShiftSCCNames[0], openShiftSCCNames[0]).Return(true,
					false, []byte{}, nil)

				openShiftQueryYAML = k8sclient.GetOpenShiftSCCQueryYAML(openShiftSCCNames[0])
				mockK8sClient.EXPECT().DeleteObjectByYAML(openShiftQueryYAML, true).Return(nil)

				// values have to hard-coded here for the mock calls as they are hard-coded in the function
				gomock.InOrder(
					mockK8sClient.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident-installer",
						"privileged").Return(nil),
					mockK8sClient.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident-csi",
						"privileged").Return(nil),
					mockK8sClient.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident", "anyuid").Return(nil),
				)
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}

				usernames, names, label := test.input.usernames, test.input.names, test.input.label
				err := extendedK8sClient.DeleteMultipleOpenShiftSCC(usernames, names, label)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultiplePods(t *testing.T) {
	// arrange variables for the tests
	var emptyPodList, undeletedPods, unwantedPods []corev1.Pod
	var undeletedPodDataList []string

	deletePodErr := fmt.Errorf("could not delete pod")
	podName := "podName"
	podNamespace := "podNamespace"

	undeletedPods = []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: podNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: podNamespace,
			},
		},
	}

	for _, pod := range undeletedPods {
		undeletedPodDataList = append(undeletedPodDataList, fmt.Sprintf("%v/%v", pod.Namespace,
			pod.Name))
	}

	unwantedPods = []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: podNamespace,
			},
		},
	}

	tests := map[string]struct {
		input  []corev1.Pod
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no pods": {
			input:  emptyPodList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  undeletedPods,
			output: fmt.Errorf("unable to delete pod(s): %v", undeletedPodDataList),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeletePod(podName,
					podNamespace).Return(deletePodErr).
					MaxTimes(len(undeletedPods))
			},
		},
		"expect to pass with valid pods": {
			input:  unwantedPods,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeletePod(podName,
					podNamespace).Return(nil).
					MaxTimes(len(unwantedPods))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultiplePods(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}

func TestRemoveMultipleStatefulSets(t *testing.T) {
	// arrange variables for the tests
	var emptyStatefulSetList, unwantedStatefulSets []appsv1.StatefulSet
	var undeletedStatefulSets []string

	deleteStatefulSetsErr := fmt.Errorf("could not delete statefulset")
	statefulSetName := "statefulSetName"
	statefulSetNamespace := "statefulSetNamespace"

	unwantedStatefulSets = []appsv1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      statefulSetName,
				Namespace: statefulSetNamespace,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      statefulSetName,
				Namespace: statefulSetNamespace,
			},
		},
	}

	for _, service := range unwantedStatefulSets {
		undeletedStatefulSets = append(undeletedStatefulSets, fmt.Sprintf("%v/%v", service.Namespace,
			service.Name))
	}

	tests := map[string]struct {
		input  []appsv1.StatefulSet
		output error
		mocks  func(*mockK8sClient.MockKubernetesClient)
	}{
		"expect to pass with no stateful sets": {
			input:  emptyStatefulSetList,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				// do nothing as the lower level k8s call will never execute
			},
		},
		"expect to fail with k8s call error": {
			input:  unwantedStatefulSets,
			output: fmt.Errorf("unable to delete Statefulset(s): %v", undeletedStatefulSets),
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteStatefulSet(statefulSetName,
					statefulSetNamespace).Return(deleteStatefulSetsErr).
					MaxTimes(len(unwantedStatefulSets))
			},
		},
		"expect to pass with valid stateful sets": {
			input:  unwantedStatefulSets,
			output: nil,
			mocks: func(mockK8sClient *mockK8sClient.MockKubernetesClient) {
				mockK8sClient.EXPECT().DeleteStatefulSet(statefulSetName,
					statefulSetNamespace).Return(nil).
					MaxTimes(len(unwantedStatefulSets))
			},
		},
	}

	for name, test := range tests {
		t.Run(
			name, func(t *testing.T) {
				// setup mock controller and kube client
				mockCtrl := gomock.NewController(t)
				mockKubeClient := mockK8sClient.NewMockKubernetesClient(mockCtrl)

				// mock out the k8s client calls needed to test this
				test.mocks(mockKubeClient)
				extendedK8sClient := &K8sClient{mockKubeClient}
				err := extendedK8sClient.RemoveMultipleStatefulSets(test.input)
				assert.Equal(t, test.output, err)
			},
		)
	}
}
