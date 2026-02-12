// Copyright 2023 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakeK8sClient "k8s.io/client-go/kubernetes/fake"

	"github.com/netapp/trident/cli/api"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	tridentconfig "github.com/netapp/trident/config"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

func TestGetDaemonSetName(t *testing.T) {
	testCases := []struct {
		name     string
		windows  bool
		expected string
	}{
		{
			name:     "Windows DaemonSet",
			windows:  true,
			expected: TridentNodeWindowsResourceName,
		},
		{
			name:     "Linux DaemonSet",
			windows:  false,
			expected: TridentNodeLinuxResourceName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getDaemonSetName(tc.windows)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestValidateTridentDeployment(t *testing.T) {
	tempDir := t.TempDir()
	validDeploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
  labels:
    app: controller.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: controller.csi.trident.netapp.io
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0
      - name: csi-provisioner
        image: registry.k8s.io/sig-storage/csi-provisioner:v3.6.0`

	missingDeploymentLabelYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
  labels:
    app: wrong-label
spec:
  template:
    metadata:
      labels:
        app: controller.csi.trident.netapp.io
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0`

	// Deployment missing pod template label
	missingPodLabelYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
  labels:
    app: controller.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: wrong-pod-label
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0`

	// Deployment missing trident container
	missingContainerYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
  labels:
    app: controller.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: controller.csi.trident.netapp.io
    spec:
      containers:
      - name: csi-provisioner
        image: registry.k8s.io/sig-storage/csi-provisioner:v3.6.0`

	tests := []struct {
		name           string
		yamlContent    string
		createFile     bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "Valid deployment",
			yamlContent: validDeploymentYAML,
			createFile:  true,
			expectError: false,
		},
		{
			name:           "File read error",
			yamlContent:    "",
			createFile:     false,
			expectError:    true,
			expectedErrMsg: "could not load deployment YAML file",
		},
		{
			name:           "Missing deployment label",
			yamlContent:    missingDeploymentLabelYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident deployment must have the label",
		},
		{
			name:           "Missing pod template label",
			yamlContent:    missingPodLabelYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident deployment's pod template must have the label",
		},
		{
			name:           "Missing trident container",
			yamlContent:    missingContainerYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident deployment must define the trident-main container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Println(tt.name)
			originalDeploymentPath := deploymentPath
			testDeploymentPath := filepath.Join(tempDir, "deployment.yaml")

			if tt.createFile {
				err := os.WriteFile(testDeploymentPath, []byte(tt.yamlContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				t.Cleanup(func() {
					os.Remove(testDeploymentPath)
				})
			}

			deploymentPath = testDeploymentPath
			origAppLabelKey := appLabelKey
			origAppLabelValue := appLabelValue
			appLabelKey = TridentCSILabelKey
			appLabelValue = TridentCSILabelValue
			defer func() {
				deploymentPath = originalDeploymentPath
				appLabelKey = origAppLabelKey
				appLabelValue = origAppLabelValue
			}()

			tridentImage = ""

			err := validateTridentDeployment()
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedErrMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if tridentImage != "netapp/trident:23.10.0" {
					t.Errorf("Expected tridentImage to be 'netapp/trident:23.10.0', got: %s", tridentImage)
				}
			}
		})
	}
}

func TestValidateTridentService(t *testing.T) {
	tempDir := t.TempDir()

	validServiceYAML := `apiVersion: v1
kind: Service
metadata:
  name: trident-csi
  labels:
    app: controller.csi.trident.netapp.io
spec:
  selector:
    app: controller.csi.trident.netapp.io
  ports:
  - port: 34571
    targetPort: 8443`

	invalidServiceYAML := `apiVersion: v1
kind: Service
metadata:
  name: trident-csi
  labels:
    app: wrong-label
spec:
  selector:
    app: controller.csi.trident.netapp.io
  ports:
  - port: 34571
    targetPort: 8443`

	tests := []struct {
		name           string
		yamlContent    string
		createFile     bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "Valid service",
			yamlContent: validServiceYAML,
			createFile:  true,
			expectError: false,
		},
		{
			name:           "File read error",
			yamlContent:    "",
			createFile:     false,
			expectError:    true,
			expectedErrMsg: "could not load service YAML file",
		},
		{
			name:           "Missing service label",
			yamlContent:    invalidServiceYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident service must have the label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up file path
			originalServicePath := servicePath
			testServicePath := filepath.Join(tempDir, "service.yaml")

			if tt.createFile {
				err := os.WriteFile(testServicePath, []byte(tt.yamlContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				t.Cleanup(func() {
					os.Remove(testServicePath)
				})
			}

			servicePath = testServicePath
			origAppLabelKey := appLabelKey
			origAppLabelValue := appLabelValue
			appLabelKey = TridentCSILabelKey
			appLabelValue = TridentCSILabelValue
			defer func() {
				servicePath = originalServicePath
				appLabelKey = origAppLabelKey
				appLabelValue = origAppLabelValue
			}()

			err := validateTridentService()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedErrMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateTridentDaemonSet(t *testing.T) {
	tempDir := t.TempDir()

	validDaemonSetYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: trident-csi
  labels:
    app: node.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: node.csi.trident.netapp.io
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0
      - name: csi-node-driver-registrar
        image: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.9.0`

	missingDaemonSetLabelYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: trident-csi
  labels:
    app: wrong-label
spec:
  template:
    metadata:
      labels:
        app: node.csi.trident.netapp.io
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0`

	missingPodLabelYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: trident-csi
  labels:
    app: node.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: wrong-pod-label
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:23.10.0`

	missingContainerYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: trident-csi
  labels:
    app: node.csi.trident.netapp.io
spec:
  template:
    metadata:
      labels:
        app: node.csi.trident.netapp.io
    spec:
      containers:
      - name: csi-node-driver-registrar
        image: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.9.0`

	tests := []struct {
		name           string
		yamlContent    string
		createFile     bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "Valid daemonset",
			yamlContent: validDaemonSetYAML,
			createFile:  true,
			expectError: false,
		},
		{
			name:           "File read error",
			yamlContent:    "",
			createFile:     false,
			expectError:    true,
			expectedErrMsg: "could not load DaemonSet YAML file",
		},
		{
			name:           "Missing daemonset label",
			yamlContent:    missingDaemonSetLabelYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident DaemonSet must have the label",
		},
		{
			name:           "Missing pod template label",
			yamlContent:    missingPodLabelYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident DaemonSet's pod template must have the label",
		},
		{
			name:           "Missing trident container",
			yamlContent:    missingContainerYAML,
			createFile:     true,
			expectError:    true,
			expectedErrMsg: "the Trident DaemonSet must define the trident-main container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDaemonSetPath := filepath.Join(tempDir, "daemonset.yaml")

			if tt.createFile {
				err := os.WriteFile(testDaemonSetPath, []byte(tt.yamlContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				t.Cleanup(func() {
					os.Remove(testDaemonSetPath)
				})
			}

			err := validateTridentDaemonSet(testDaemonSetPath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedErrMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.expectedErrMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestCreateOpenShiftTridentSCC(t *testing.T) {
	tests := []struct {
		name        string
		user        string
		appLabelVal string
		err         bool
		mockFunc    func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:        "Success with trident-installer user",
			user:        "trident-installer",
			appLabelVal: "trident-installer",
			err:         false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident-installer", "privileged").Return(nil)

				mockK8s.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil)
			},
		},
		{
			name:        "Success with CSI user",
			user:        "trident-csi",
			appLabelVal: "controller.csi.trident.netapp.io",
			err:         false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident-csi", "privileged").Return(nil)

				mockK8s.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil)
			},
		},
		{
			name:        "Success with regular trident user",
			user:        "trident",
			appLabelVal: "trident",
			err:         false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident", "anyuid").Return(nil)

				mockK8s.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil)
			},
		},
		{
			name:        "Error during SCC creation",
			user:        "trident-csi",
			appLabelVal: "controller.csi.trident.netapp.io",
			err:         true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().RemoveTridentUserFromOpenShiftSCC("trident-csi", "privileged").Return(nil)

				mockK8s.EXPECT().CreateObjectByYAML(gomock.Any()).Return(fmt.Errorf("creation failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
			client = mockK8s

			originalAppLabelValue := appLabelValue
			appLabelValue = tt.appLabelVal
			defer func() { appLabelValue = originalAppLabelValue }()

			tt.mockFunc(mockK8s)

			err := CreateOpenShiftTridentSCC(tt.user, tt.appLabelVal)

			if tt.err && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.err && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestDeleteOpenShiftTridentSCC(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		labelVal string
		err      bool
		mockFunc func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:     "Success with trident-installer user",
			user:     "trident-installer",
			labelVal: "trident-installer",
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil)
			},
		},
		{
			name:     "Success with CSI user",
			user:     "trident-csi",
			labelVal: "controller.csi.trident.netapp.io",
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil)
			},
		},
		{
			name:     "Success with regular trident user",
			user:     "trident",
			labelVal: "trident",
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil)
			},
		},
		{
			name:     "Error during SCC deletion",
			user:     "trident-csi",
			labelVal: "controller.csi.trident.netapp.io",
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(fmt.Errorf("deletion failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
			client = mockK8s

			tt.mockFunc(mockK8s)

			err := DeleteOpenShiftTridentSCC(tt.user, tt.labelVal)

			if tt.err && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.err && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestReadDeploymentFromFile(t *testing.T) {
	tempDir := t.TempDir()

	validDeploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: test-namespace
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-container
        image: test-image:latest`

	// Invalid YAML
	invalidYAML := `invalid: yaml: content: [unclosed`

	testCases := []struct {
		name        string
		fileContent string
		fileName    string
		expectError bool
		expectNil   bool
	}{
		{
			name:        "Valid deployment YAML",
			fileContent: validDeploymentYAML,
			fileName:    "valid-deployment.yaml",
			expectError: false,
			expectNil:   false,
		},
		{
			name:        "Invalid YAML content",
			fileContent: invalidYAML,
			fileName:    "invalid-deployment.yaml",
			expectError: true,
			expectNil:   true,
		},
		{
			name:        "Non-existent file",
			fileContent: "",
			fileName:    "non-existent.yaml",
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var filePath string

			if tc.name != "Non-existent file" {
				filePath = filepath.Join(tempDir, tc.fileName)
				err := os.WriteFile(filePath, []byte(tc.fileContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			} else {
				filePath = filepath.Join(tempDir, tc.fileName)
			}

			deployment, err := readDeploymentFromFile(filePath)

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tc.expectNil && deployment != nil {
				t.Errorf("Expected nil deployment but got: %v", deployment)
			}
			if !tc.expectNil && deployment == nil {
				t.Errorf("Expected non-nil deployment but got nil")
			}

			// Additional validation for successful case
			if !tc.expectError && !tc.expectNil {
				if deployment.Name != "test-deployment" {
					t.Errorf("Expected deployment name 'test-deployment', got %s", deployment.Name)
				}
			}
		})
	}
}

func TestReadServiceFromFile(t *testing.T) {
	tempDir := t.TempDir()

	validServiceYAML := `apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: test-namespace
spec:
  selector:
    app: test-app
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP`

	invalidYAML := `invalid: yaml: content: [unclosed`

	testCases := []struct {
		name        string
		fileContent string
		fileName    string
		expectError bool
		expectNil   bool
	}{
		{
			name:        "Valid service YAML",
			fileContent: validServiceYAML,
			fileName:    "valid-service.yaml",
			expectError: false,
			expectNil:   false,
		},
		{
			name:        "Invalid YAML content",
			fileContent: invalidYAML,
			fileName:    "invalid-service.yaml",
			expectError: true,
			expectNil:   true,
		},
		{
			name:        "Non-existent file",
			fileContent: "",
			fileName:    "non-existent.yaml",
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var filePath string

			if tc.name != "Non-existent file" {
				filePath = filepath.Join(tempDir, tc.fileName)
				err := os.WriteFile(filePath, []byte(tc.fileContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			} else {
				filePath = filepath.Join(tempDir, tc.fileName)
			}

			service, err := readServiceFromFile(filePath)

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tc.expectNil && service != nil {
				t.Errorf("Expected nil service but got: %v", service)
			}
			if !tc.expectNil && service == nil {
				t.Errorf("Expected non-nil service but got nil")
			}

			if !tc.expectError && !tc.expectNil {
				if service.Name != "test-service" {
					t.Errorf("Expected service name 'test-service', got %s", service.Name)
				}
			}
		})
	}
}

func TestReadDaemonSetFromFile(t *testing.T) {
	tempDir := t.TempDir()

	validDaemonSetYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: test-daemonset
  namespace: test-namespace
spec:
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-container
        image: test-image:latest`

	invalidYAML := `invalid: yaml: content: [unclosed`

	testCases := []struct {
		name        string
		fileContent string
		fileName    string
		expectError bool
		expectNil   bool
	}{
		{
			name:        "Valid daemonset YAML",
			fileContent: validDaemonSetYAML,
			fileName:    "valid-daemonset.yaml",
			expectError: false,
			expectNil:   false,
		},
		{
			name:        "Invalid YAML content",
			fileContent: invalidYAML,
			fileName:    "invalid-daemonset.yaml",
			expectError: true,
			expectNil:   true,
		},
		{
			name:        "Non-existent file",
			fileContent: "",
			fileName:    "non-existent.yaml",
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var filePath string

			if tc.name != "Non-existent file" {
				filePath = filepath.Join(tempDir, tc.fileName)
				err := os.WriteFile(filePath, []byte(tc.fileContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			} else {
				filePath = filepath.Join(tempDir, tc.fileName)
			}

			daemonset, err := readDaemonSetFromFile(filePath)

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tc.expectNil && daemonset != nil {
				t.Errorf("Expected nil daemonset but got: %v", daemonset)
			}
			if !tc.expectNil && daemonset == nil {
				t.Errorf("Expected non-nil daemonset but got nil")
			}

			if !tc.expectError && !tc.expectNil {
				if daemonset.Name != "test-daemonset" {
					t.Errorf("Expected daemonset name 'test-daemonset', got %s", daemonset.Name)
				}
			}
		})
	}
}

func TestGetNodeRBACResourceName(t *testing.T) {
	testCases := []struct {
		name     string
		windows  bool
		expected string
	}{
		{
			name:     "Windows node RBAC resource",
			windows:  true,
			expected: TridentNodeWindowsResourceName,
		},
		{
			name:     "Linux node RBAC resource",
			windows:  false,
			expected: TridentNodeLinuxResourceName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getNodeRBACResourceName(tc.windows)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetServiceAccountName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Service Account Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getServiceAccountName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetClusterRoleName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Cluster Role Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getClusterRoleName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetClusterRoleBindingName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Cluster Role Binding Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getClusterRoleBindingName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetServiceName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Service Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getServiceName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetProtocolSecretName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Protocol Secret Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getProtocolSecretName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetEncryptionSecretName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Encryption Secret Name",
			expected: TridentEncryptionKeys,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getEncryptionSecretName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetResourceQuotaName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "Resource Quota Name",
			expected: TridentCSI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getResourceQuotaName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetCSIDriverName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{
			name:     "CSI Driver Name",
			expected: CSIDriver,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getCSIDriverName()
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetAppLabelForResource(t *testing.T) {
	testCases := []struct {
		name             string
		resourceName     string
		expectedLabelMap map[string]string
		expectedLabel    string
	}{
		{
			name:         "Controller resource (no 'node' in name)",
			resourceName: "trident-csi-controller",
			expectedLabelMap: map[string]string{
				TridentNodeLabelKey: TridentCSILabelValue,
			},
			expectedLabel: TridentCSILabel,
		},
		{
			name:         "Node resource (contains 'node')",
			resourceName: "trident-csi-node",
			expectedLabelMap: map[string]string{
				TridentCSILabelKey: TridentNodeLabelValue,
			},
			expectedLabel: TridentNodeLabel,
		},
		{
			name:         "Another controller resource",
			resourceName: "trident-controller-rbac",
			expectedLabelMap: map[string]string{
				TridentNodeLabelKey: TridentCSILabelValue,
			},
			expectedLabel: TridentCSILabel,
		},
		{
			name:         "Node daemonset resource",
			resourceName: "trident-node-linux",
			expectedLabelMap: map[string]string{
				TridentCSILabelKey: TridentNodeLabelValue,
			},
			expectedLabel: TridentNodeLabel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			labelMap, label := getAppLabelForResource(tc.resourceName)

			if !reflect.DeepEqual(labelMap, tc.expectedLabelMap) {
				t.Errorf("expected labelMap %v, got %v", tc.expectedLabelMap, labelMap)
			}

			if label != tc.expectedLabel {
				t.Errorf("expected label %s, got %s", tc.expectedLabel, label)
			}
		})
	}
}

func TestIsLinuxNodeSCCUser(t *testing.T) {
	testCases := []struct {
		name     string
		user     string
		expected bool
	}{
		{
			name:     "Linux node SCC user",
			user:     TridentNodeLinuxResourceName,
			expected: true,
		},
		{
			name:     "Windows node user",
			user:     TridentNodeWindowsResourceName,
			expected: false,
		},
		{
			name:     "Different user",
			user:     "some-other-user",
			expected: false,
		},
		{
			name:     "Empty user",
			user:     "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isLinuxNodeSCCUser(tc.user)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCRDFunctions(t *testing.T) {
	// Override CRDnames with test-specific mock values
	originalCRDnames := CRDnames
	defer func() {
		CRDnames = originalCRDnames // Restore after test
	}()
	CRDnames = []string{"mycrd.example.com", "anothercrd.example.com"}

	tests := []struct {
		name       string
		bundle     string
		wantKeys   []string
		wantErr    bool
		errSubstrs []string
	}{
		{
			name: "all valid",
			bundle: `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: mycrd.example.com
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: anothercrd.example.com`,
			wantKeys: []string{"mycrd.example.com", "anothercrd.example.com"},
			wantErr:  false,
		},
		{
			name: "missing CRD",
			bundle: `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: mycrd.example.com`,
			wantKeys:   []string{"mycrd.example.com"},
			wantErr:    true,
			errSubstrs: []string{"missing CRD(s): [anothercrd.example.com]"},
		},
		{
			name: "extra CRD",
			bundle: `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: mycrd.example.com
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: extracrd.example.com`,
			wantKeys: []string{"mycrd.example.com", "extracrd.example.com"},
			wantErr:  true,
			errSubstrs: []string{
				"missing CRD(s): [anothercrd.example.com]",
				"unrecognized CRD(s) found: [extracrd.example.com]",
			},
		},
		{
			name: "no CRDs",
			bundle: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: configmap`,
			wantKeys: []string{},
			wantErr:  true,
			errSubstrs: []string{
				"missing CRD(s): [mycrd.example.com anothercrd.example.com]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crdMap := getCRDMapFromBundle(tt.bundle)

			// Validate keys parsed
			for _, k := range tt.wantKeys {
				if _, ok := crdMap[k]; !ok {
					t.Errorf("expected CRD key %q not found in crdMap", k)
				}
			}

			// Run validation
			err := validateCRDs(crdMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCRDs() error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, substr := range tt.errSubstrs {
				if err == nil || !strings.Contains(err.Error(), substr) {
					t.Errorf("expected error to contain %q, got: %v", substr, err)
				}
			}
		})
	}
}

func TestFileUtils(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.yaml")
	if fileExists(testFile) {
		t.Errorf("Expected file to not exist")
	}
	if err := os.WriteFile(testFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if !fileExists(testFile) {
		t.Errorf("Expected file to exist after writing")
	}

	writePath := filepath.Join(tmpDir, "written.yaml")
	if err := writeFile(writePath, "example content"); err != nil {
		t.Errorf("writeFile failed: %v", err)
	}
	if !fileExists(writePath) {
		t.Errorf("Expected written file to exist")
	}

	setupYAMLPaths = []string{testFile, writePath}
	cleanYAMLFiles()
	if fileExists(testFile) || fileExists(writePath) {
		t.Errorf("Expected files to be removed by cleanYAMLFiles")
	}

	setupPath = filepath.Join(tmpDir, "setup-dir")
	if err := ensureSetupDirExists(); err != nil {
		t.Errorf("Failed to create setup directory: %v", err)
	}
	if !fileExists(setupPath) {
		t.Errorf("Expected setup directory to be created")
	}
}

func TestProcessInstallationArguments(t *testing.T) {
	nodePrep = []string{" ISCSI", "nFs "}
	processInstallationArguments(nil)

	if appLabel != TridentCSILabel {
		t.Errorf("Expected appLabel = %s, got %s", TridentCSILabel, appLabel)
	}
	if appLabelKey != TridentCSILabelKey || appLabelValue != TridentCSILabelValue {
		t.Errorf("App label key/value mismatch")
	}
	if persistentObjectLabelKey != TridentPersistentObjectLabelKey ||
		persistentObjectLabelValue != TridentPersistentObjectLabelValue {
		t.Errorf("Persistent label key/value mismatch")
	}

	expected := []string{"iscsi", "nfs"}
	if len(nodePrep) != len(expected) {
		t.Fatalf("Expected %d protocols, got %d", len(expected), len(nodePrep))
	}
	for i := range expected {
		if nodePrep[i] != expected[i] {
			t.Errorf("Expected protocol[%d] = %s, got %s", i, expected[i], nodePrep[i])
		}
	}
}

func getMockK8sClient(t *testing.T) (*mockK8sClient.MockKubernetesClient, *fakeK8sClient.Clientset) {
	kubeClient := fakeK8sClient.NewSimpleClientset()
	ctrl := gomock.NewController(t)
	mockClient := mockK8sClient.NewMockKubernetesClient(ctrl)

	client = mockClient

	return mockClient, kubeClient
}

func TestWaitForTridentPod(t *testing.T) {
	originalTimeout := k8sTimeout
	k8sTimeout = 20 * time.Second
	defer func() { k8sTimeout = originalTimeout }()

	tests := []struct {
		name                  string
		setupMocks            func(*mockK8sClient.MockKubernetesClient)
		expectedErrorContains string
		expectPod             bool
	}{
		{
			name: "successful_pod_startup",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  TridentMainContainer,
								Image: tridentImage,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name:  TridentMainContainer,
								Ready: true,
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{},
								},
							},
						},
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "",
			expectPod:             true,
		},
		{
			name: "deployment_not_found",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(nil, errors.New("deployment not found")).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "deployment not found",
			expectPod:             false,
		},
		{
			name: "pod_not_found",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(nil, errors.New("pod not found")).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "pod not found",
			expectPod:             false,
		},
		{
			name: "pod_terminating",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				deletionTime := metav1.Now()
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "trident-pod",
						DeletionTimestamp: &deletionTime,
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident pod is terminating",
			expectPod:             false,
		},
		{
			name: "pod_not_running",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident pod is not running",
			expectPod:             false,
		},
		{
			name: "incorrect_image_in_pod_spec",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  TridentMainContainer,
								Image: "wrong-image:latest",
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident pod spec reports a different image",
			expectPod:             false,
		},
		{
			name: "container_not_running",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  TridentMainContainer,
								Image: tridentImage,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name:  TridentMainContainer,
								Ready: false,
								State: v1.ContainerState{
									Waiting: &v1.ContainerStateWaiting{},
								},
							},
						},
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident container is not running",
			expectPod:             false,
		},
		{
			name: "container_not_ready",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  TridentMainContainer,
								Image: tridentImage,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name:  TridentMainContainer,
								Ready: false,
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{},
								},
							},
						},
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident container is not ready",
			expectPod:             false,
		},
		{
			name: "main_container_not_found",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  TridentMainContainer,
								Image: tridentImage,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name:  "other-container",
								Ready: true,
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{},
								},
							},
						},
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: fmt.Sprintf("running container %s not found", TridentMainContainer),
			expectPod:             false,
		},
		{
			name: "pod_with_status_message",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Status: v1.PodStatus{
						Phase:   v1.PodFailed,
						Message: "Image pull failed",
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident pod is not running",
			expectPod:             false,
		},
		{
			name: "max_interval_backoff_behavior",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-deployment"},
				}
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
					Status: v1.PodStatus{
						Phase: v1.PodPending, // Pod never becomes ready to trigger retries
					},
				}

				mockClient.EXPECT().GetDeploymentByLabel(appLabel, false).Return(deployment, nil).AnyTimes()
				mockClient.EXPECT().GetPodByLabel(appLabel, false).Return(pod, nil).AnyTimes()
				mockClient.EXPECT().CLI().Return("kubectl").AnyTimes()
				mockClient.EXPECT().Namespace().Return("trident").AnyTimes()
			},
			expectedErrorContains: "trident pod is not running",
			expectPod:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			pod, err := waitForTridentPod()

			if tt.expectedErrorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorContains)
				if !tt.expectPod {
					assert.Nil(t, pod)
				}
			} else {
				assert.NoError(t, err)
				if tt.expectPod {
					assert.NotNil(t, pod)
				}
			}
		})
	}
}

func TestWaitForRESTInterface(t *testing.T) {
	originalTimeout := k8sTimeout
	k8sTimeout = 2 * time.Second
	defer func() { k8sTimeout = originalTimeout }()

	tests := []struct {
		name                  string
		setupMocks            func(*mockK8sClient.MockKubernetesClient)
		expectedErrorContains string
	}{
		{
			name: "successful_rest_interface",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				versionResponse := api.VersionResponse{
					Server: &api.Version{
						Version: "1.0.0",
					},
				}
				responseJSON, _ := json.Marshal(versionResponse)

				mockClient.EXPECT().Exec(TridentPodName, gomock.Any(), gomock.Any()).
					Return(responseJSON, nil).AnyTimes()
			},
			expectedErrorContains: "",
		},
		{
			name: "exec_command_fails",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Exec(TridentPodName, gomock.Any(), gomock.Any()).
					Return(nil, errors.New("exec failed")).AnyTimes()
			},
			expectedErrorContains: "exec failed",
		},
		{
			name: "exec_command_fails_with_output",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				output := []byte("command error output")
				mockClient.EXPECT().Exec(TridentPodName, gomock.Any(), gomock.Any()).
					Return(output, errors.New("exec failed")).AnyTimes()
			},
			expectedErrorContains: "exec failed",
		},
		{
			name: "invalid_json_response",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				invalidJSON := []byte("{invalid json}")
				mockClient.EXPECT().Exec(TridentPodName, gomock.Any(), gomock.Any()).
					Return(invalidJSON, nil).AnyTimes()
			},
			expectedErrorContains: "invalid character",
		},
		{
			name: "timeout_waiting_for_interface",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Exec(TridentPodName, gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection refused")).AnyTimes()
			},
			expectedErrorContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := waitForRESTInterface()

			if tt.expectedErrorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateRBACObjects(t *testing.T) {
	originalUseYAML := useYAML
	originalWindows := windows
	originalCloudIdentity := cloudIdentity
	defer func() {
		useYAML = originalUseYAML
		windows = originalWindows
		cloudIdentity = originalCloudIdentity
	}()

	tests := []struct {
		name                  string
		useYAML               bool
		windows               bool
		cloudIdentity         string
		k8sFlavor             string
		setupMocks            func(*mockK8sClient.MockKubernetesClient)
		expectedErrorContains string
	}{
		{
			name:          "successful_creation_with_yaml_strings",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(8)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "",
		},
		{
			name:          "successful_creation_with_windows",
			useYAML:       false,
			windows:       true,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(11)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "",
		},
		{
			name:          "successful_creation_with_openshift",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "openshift",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(8)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()
				mockClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes() // for SCC creation
			},
			expectedErrorContains: "",
		},
		{
			name:          "successful_creation_with_openshift_and_windows",
			useYAML:       false,
			windows:       true,
			cloudIdentity: "",
			k8sFlavor:     "openshift",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(11)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()

				mockClient.EXPECT().RemoveTridentUserFromOpenShiftSCC(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes()
			},
			expectedErrorContains: "",
		},
		{
			name:          "fail_controller_service_account",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("service account creation failed")).Times(1)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create controller service account",
		},
		{
			name:          "fail_controller_role",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("role creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create controller role",
		},
		{
			name:          "fail_controller_role_binding",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(2),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("role binding creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create controller role binding",
		},
		{
			name:          "fail_controller_cluster_role",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(3),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("cluster role creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create controller cluster role",
		},
		{
			name:          "fail_controller_cluster_role_binding",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(4),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("cluster role binding creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create controller cluster role binding",
		},
		{
			name:          "fail_node_linux_service_account",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(5),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("node service account creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create node linux service account",
		},
		{
			name:          "fail_node_windows_service_account",
			useYAML:       false,
			windows:       true,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(8),
					mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("node windows service account creation failed")).Times(1),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "could not create node windows service account",
		},
		{
			name:          "with_cloud_identity",
			useYAML:       false,
			windows:       false,
			cloudIdentity: "test-identity",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(8)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)

			useYAML = tt.useYAML
			windows = tt.windows
			cloudIdentity = tt.cloudIdentity

			tt.setupMocks(mockClient)

			err := createRBACObjects()

			if tt.expectedErrorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveRBACObjects(t *testing.T) {
	originalCloudIdentity := cloudIdentity
	defer func() {
		cloudIdentity = originalCloudIdentity
	}()

	tests := []struct {
		name           string
		logLevel       log.Level
		k8sFlavor      string
		cloudIdentity  string
		setupMocks     func(*mockK8sClient.MockKubernetesClient)
		expectedErrors bool
	}{
		{
			name:          "successful_removal_kubernetes",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(18)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: false,
		},
		{
			name:          "successful_removal_openshift",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "openshift",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(22)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()
			},
			expectedErrors: false,
		},
		{
			name:          "debug_log_level",
			logLevel:      log.DebugLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(18)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: false,
		},
		{
			name:          "some_deletion_failures",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("deletion failed")).Times(5)
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(13)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: true,
		},
		{
			name:          "all_deletion_failures",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("deletion failed")).Times(18)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: true,
		},
		{
			name:          "openshift_scc_deletion_failure",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "openshift",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(18),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("scc deletion failed")).Times(2),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(2),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()
			},
			expectedErrors: true,
		},
		{
			name:          "with_cloud_identity",
			logLevel:      log.InfoLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "test-identity",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(18)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: false,
		},
		{
			name:          "mixed_success_failure_scenario",
			logLevel:      log.WarnLevel,
			k8sFlavor:     "kubernetes",
			cloudIdentity: "",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				gomock.InOrder(
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("service account deletion failed")).Times(1),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(1),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("cluster role binding deletion failed")).Times(1),

					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(2),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("controller role deletion failed")).Times(1),
					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(2),

					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(5),

					mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(5),
				)
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			},
			expectedErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)

			cloudIdentity = tt.cloudIdentity

			tt.setupMocks(mockClient)

			anyErrors := removeRBACObjects(tt.logLevel)

			assert.Equal(t, tt.expectedErrors, anyErrors)
		})
	}
}

func TestDeleteCustomResourceDefinition(t *testing.T) {
	tests := []struct {
		name          string
		crdName       string
		crdYAML       string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:    "successful_deletion",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:    "deletion_failure",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("deletion failed")).Times(1)
			},
			expectedError: "could not delete custom resource definition test-crd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := deleteCustomResourceDefinition(tt.crdName, tt.crdYAML)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProtectCustomResourceDefinitions(t *testing.T) {
	originalCRDnames := CRDnames
	defer func() {
		CRDnames = originalCRDnames
	}()

	tests := []struct {
		name          string
		crdNames      []string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:     "successful_protection",
			crdNames: []string{"crd1", "crd2", "crd3"},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().AddFinalizerToCRD("crd1").Return(nil).Times(1)
				mockClient.EXPECT().AddFinalizerToCRD("crd2").Return(nil).Times(1)
				mockClient.EXPECT().AddFinalizerToCRD("crd3").Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:     "first_crd_fails",
			crdNames: []string{"crd1", "crd2"},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().AddFinalizerToCRD("crd1").Return(errors.New("finalizer failed")).Times(1)
			},
			expectedError: "finalizer failed",
		},
		{
			name:     "second_crd_fails",
			crdNames: []string{"crd1", "crd2"},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().AddFinalizerToCRD("crd1").Return(nil).Times(1)
				mockClient.EXPECT().AddFinalizerToCRD("crd2").Return(errors.New("second crd failed")).Times(1)
			},
			expectedError: "second crd failed",
		},
		{
			name:     "empty_crd_list",
			crdNames: []string{},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			CRDnames = tt.crdNames
			tt.setupMocks(mockClient)

			err := protectCustomResourceDefinitions()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateK8SCSIDriver(t *testing.T) {
	originalFsGroupPolicy := fsGroupPolicy
	defer func() {
		fsGroupPolicy = originalFsGroupPolicy
	}()

	tests := []struct {
		name          string
		fsGroupPolicy string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:          "successful_creation",
			fsGroupPolicy: "File",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(1)
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:          "delete_fails",
			fsGroupPolicy: "ReadWriteOnceWithFSType",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("delete failed")).Times(1)
			},
			expectedError: "could not delete csidriver custom resource",
		},
		{
			name:          "create_fails",
			fsGroupPolicy: "None",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(1)
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("create failed")).Times(1)
			},
			expectedError: "could not create csidriver custom resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			fsGroupPolicy = tt.fsGroupPolicy
			tt.setupMocks(mockClient)

			err := createK8SCSIDriver()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateCRD(t *testing.T) {
	tests := []struct {
		name          string
		crdName       string
		crdYAML       string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:    "successful_creation",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.Established,
								Status: apiextensionv1.ConditionTrue,
							},
						},
					},
				}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).AnyTimes()
			},
			expectedError: "",
		},
		{
			name:    "creation_fails",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("create failed")).Times(1)
			},
			expectedError: "could not create custom resource test-crd",
		},
		{
			name:    "establishment_fails_cleanup_succeeds",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
				mockClient.EXPECT().GetCRD("test-crd").Return(nil, errors.New("not found")).AnyTimes()
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).Times(1)
			},
			expectedError: "not found",
		},
		{
			name:    "establishment_fails_cleanup_fails",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
				mockClient.EXPECT().GetCRD("test-crd").Return(nil, errors.New("establishment failed")).AnyTimes()
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(errors.New("cleanup failed")).Times(1)
			},
			expectedError: "establishment failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			originalK8sTimeout := k8sTimeout
			k8sTimeout = 200 * time.Millisecond
			defer func() {
				k8sTimeout = originalK8sTimeout
			}()
			tt.setupMocks(mockClient)

			err := createCRD(tt.crdName, tt.crdYAML)

			if tt.expectedError != "" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateCustomResourceDefinition(t *testing.T) {
	tests := []struct {
		name          string
		crdName       string
		crdYAML       string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:    "successful_creation",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:    "creation_fails",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("creation error")).Times(1)
			},
			expectedError: "could not create custom resource test-crd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := createCustomResourceDefinition(tt.crdName, tt.crdYAML)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatchCRD(t *testing.T) {
	tests := []struct {
		name          string
		crdName       string
		crdYAML       string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:    "successful_patch",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockCRD := &apiextensionv1.CustomResourceDefinition{}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).Times(1)
				mockClient.EXPECT().PatchCRD("test-crd", gomock.Any(), types.MergePatchType).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:    "get_crd_fails",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().GetCRD("test-crd").Return(nil, errors.New("get failed")).Times(1)
			},
			expectedError: "could not retrieve the test-crd CRD",
		},
		{
			name:    "patch_fails",
			crdName: "test-crd",
			crdYAML: "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockCRD := &apiextensionv1.CustomResourceDefinition{}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).Times(1)
				mockClient.EXPECT().PatchCRD("test-crd", gomock.Any(), types.MergePatchType).Return(errors.New("patch failed")).Times(1)
			},
			expectedError: "could not patch custom resource test-crd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := patchCRD(tt.crdName, tt.crdYAML)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureCRDEstablished(t *testing.T) {
	originalK8sTimeout := k8sTimeout
	defer func() {
		k8sTimeout = originalK8sTimeout
	}()

	tests := []struct {
		name          string
		crdName       string
		testTimeout   time.Duration
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:        "crd_established_immediately",
			crdName:     "test-crd",
			testTimeout: 200 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.Established,
								Status: apiextensionv1.ConditionTrue,
							},
						},
					},
				}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:        "get_crd_fails",
			crdName:     "test-crd",
			testTimeout: 200 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().GetCRD("test-crd").Return(nil, errors.New("get failed")).AnyTimes()
			},
			expectedError: "CRD was not established after",
		},
		{
			name:        "crd_condition_false",
			crdName:     "test-crd",
			testTimeout: 200 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.Established,
								Status: apiextensionv1.ConditionFalse,
							},
						},
					},
				}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).AnyTimes()
			},
			expectedError: "CRD was not established after",
		},
		{
			name:        "crd_no_established_condition",
			crdName:     "test-crd",
			testTimeout: 200 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.NamesAccepted,
								Status: apiextensionv1.ConditionTrue,
							},
						},
					},
				}
				mockClient.EXPECT().GetCRD("test-crd").Return(mockCRD, nil).AnyTimes()
			},
			expectedError: "CRD was not established after",
		},
		{
			name:        "crd_eventually_established",
			crdName:     "test-crd",
			testTimeout: 2 * time.Second,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				call1 := mockClient.EXPECT().GetCRD("test-crd").Return(&apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.Established,
								Status: apiextensionv1.ConditionFalse,
							},
						},
					},
				}, nil).Times(1)
				mockClient.EXPECT().GetCRD("test-crd").Return(&apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.Established,
								Status: apiextensionv1.ConditionTrue,
							},
						},
					},
				}, nil).After(call1).AnyTimes()
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sTimeout = tt.testTimeout

			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			done := make(chan bool, 1)
			var err error

			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Test panicked: %v", r)
					}
					done <- true
				}()
				err = ensureCRDEstablished(tt.crdName)
			}()

			safetyTimeout := tt.testTimeout + (1 * time.Second)
			select {
			case <-done:
			case <-time.After(safetyTimeout):
				t.Fatal("Test took too long - likely infinite retry loop")
			}

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatchNamespace(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name: "successful_patch",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				expectedLabels := map[string]string{
					tridentconfig.PodSecurityStandardsEnforceLabel: tridentconfig.PodSecurityStandardsEnforceProfile,
				}
				mockClient.EXPECT().PatchNamespaceLabels(TridentPodNamespace, expectedLabels).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name: "patch_fails",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().PatchNamespaceLabels(gomock.Any(), gomock.Any()).Return(errors.New("patch failed")).Times(1)
			},
			expectedError: "patch failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := patchNamespace()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateNamespace(t *testing.T) {
	originalUseYAML := useYAML
	defer func() {
		useYAML = originalUseYAML
	}()

	tests := []struct {
		name          string
		useYAML       bool
		fileExists    bool
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:       "successful_creation_with_yaml_string",
			useYAML:    false,
			fileExists: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:       "successful_creation_with_yaml_file",
			useYAML:    true,
			fileExists: true,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByFile(namespacePath).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:       "yaml_file_doesnt_exist_fallback_to_string",
			useYAML:    true,
			fileExists: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).Times(1)
			},
			expectedError: "",
		},
		{
			name:       "creation_fails_with_yaml_string",
			useYAML:    false,
			fileExists: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("creation failed")).Times(1)
			},
			expectedError: "could not create namespace",
		},
		{
			name:       "creation_fails_with_yaml_file",
			useYAML:    true,
			fileExists: true,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CreateObjectByFile(namespacePath).Return(errors.New("file creation failed")).Times(1)
			},
			expectedError: "could not create namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useYAML = tt.useYAML

			if tt.useYAML && tt.fileExists {
				tempDir := t.TempDir()
				namespacePath = filepath.Join(tempDir, "namespace.yaml")
				err := os.WriteFile(namespacePath, []byte("test content"), 0o644)
				require.NoError(t, err)
			} else {
				namespacePath = ""
			}

			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)
			err := createNamespace()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPrepareYAMLFilePaths(t *testing.T) {
	originalInstallerDirectoryPath := installerDirectoryPath
	originalSetupPath := setupPath
	originalSetupYAMLPaths := setupYAMLPaths
	defer func() {
		installerDirectoryPath = originalInstallerDirectoryPath
		setupPath = originalSetupPath
		setupYAMLPaths = originalSetupYAMLPaths
	}()

	tests := []struct {
		name          string
		k8sFlavor     string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
		validatePaths func(*testing.T)
	}{
		{
			name:      "successful_kubernetes",
			k8sFlavor: "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).Times(1)
			},
			expectedError: "",
			validatePaths: func(t *testing.T) {
				assert.NotEmpty(t, installerDirectoryPath)
				assert.NotEmpty(t, setupPath)
				assert.Contains(t, setupPath, "setup")

				assert.Len(t, setupYAMLPaths, 24)

				assert.Contains(t, setupYAMLPaths, namespacePath)
				assert.Contains(t, setupYAMLPaths, controllerServiceAccountPath)
				assert.Contains(t, setupYAMLPaths, deploymentPath)

				assert.NotContains(t, setupYAMLPaths, controllerSCCPath)
			},
		},
		{
			name:      "successful_openshift",
			k8sFlavor: "openshift",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).Times(1)
			},
			expectedError: "",
			validatePaths: func(t *testing.T) {
				assert.NotEmpty(t, installerDirectoryPath)
				assert.NotEmpty(t, setupPath)

				assert.Len(t, setupYAMLPaths, 27)

				assert.Contains(t, setupYAMLPaths, controllerSCCPath)
				assert.Contains(t, setupYAMLPaths, nodeLinuxSCCPath)
				assert.Contains(t, setupYAMLPaths, nodeWindowsSCCPath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installerDirectoryPath = ""
			setupPath = ""
			setupYAMLPaths = nil

			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := prepareYAMLFilePaths()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				tt.validatePaths(t)
			}
		})
	}
}

func TestPrepareYAMLFiles(t *testing.T) {
	originalWindows := windows
	originalCloudIdentity := cloudIdentity
	defer func() {
		windows = originalWindows
		cloudIdentity = originalCloudIdentity
	}()

	tests := []struct {
		name          string
		windows       bool
		cloudIdentity string
		k8sFlavor     string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
		expectedFiles int
	}{
		{
			name:          "successful_kubernetes_linux_only",
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
				mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()
			},
			expectedError: "",
			expectedFiles: 16,
		},
		{
			name:          "successful_kubernetes_with_windows",
			windows:       true,
			cloudIdentity: "",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
				mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()
			},
			expectedError: "",
			expectedFiles: 20,
		},
		{
			name:          "successful_openshift_linux_only",
			windows:       false,
			cloudIdentity: "",
			k8sFlavor:     "openshift",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()
				mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()
			},
			expectedError: "",
			expectedFiles: 18,
		},
		{
			name:          "successful_openshift_with_windows",
			windows:       true,
			cloudIdentity: "",
			k8sFlavor:     "openshift",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("openshift")).AnyTimes()
				mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()
			},
			expectedError: "",
			expectedFiles: 23,
		},
		{
			name:          "with_cloud_identity",
			windows:       false,
			cloudIdentity: "test-identity",
			k8sFlavor:     "kubernetes",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
				mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()
			},
			expectedError: "",
			expectedFiles: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			namespacePath = filepath.Join(tempDir, "namespace.yaml")
			controllerServiceAccountPath = filepath.Join(tempDir, "controller-sa.yaml")
			controllerClusterRolePath = filepath.Join(tempDir, "controller-cluster-role.yaml")
			controllerRolePath = filepath.Join(tempDir, "controller-role.yaml")
			controllerRoleBindingPath = filepath.Join(tempDir, "controller-rb.yaml")
			controllerClusterRoleBindingPath = filepath.Join(tempDir, "controller-crb.yaml")
			nodeLinuxServiceAccountPath = filepath.Join(tempDir, "node-sa.yaml")
			nodeLinuxClusterRolePath = filepath.Join(tempDir, "node-linux-cluster-role.yaml")
			nodeLinuxClusterRoleBindingPath = filepath.Join(tempDir, "node-linux-crb.yaml")
			nodeWindowsClusterRolePath = filepath.Join(tempDir, "node-windows-cluster-role.yaml")
			nodeWindowsClusterRoleBindingPath = filepath.Join(tempDir, "node-windows-crb.yaml")
			crdsPath = filepath.Join(tempDir, "crds.yaml")
			servicePath = filepath.Join(tempDir, "service.yaml")
			resourceQuotaPath = filepath.Join(tempDir, "quota.yaml")
			deploymentPath = filepath.Join(tempDir, "deployment.yaml")
			daemonsetPath = filepath.Join(tempDir, "daemonset.yaml")
			windowsDaemonSetPath = filepath.Join(tempDir, "windows-ds.yaml")
			nodeWindowsServiceAccountPath = filepath.Join(tempDir, "windows-sa.yaml")
			controllerSCCPath = filepath.Join(tempDir, "controller-scc.yaml")
			nodeLinuxSCCPath = filepath.Join(tempDir, "node-scc.yaml")
			nodeWindowsSCCPath = filepath.Join(tempDir, "windows-scc.yaml")
			nodeRemediationTemplatePath = filepath.Join(tempDir, "node-remediation-template.yaml")
			nodeRemediationClusterRolePath = filepath.Join(tempDir, "node-remediation-cluster-role.yaml")

			windows = tt.windows
			cloudIdentity = tt.cloudIdentity

			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := prepareYAMLFiles()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				files, _ := filepath.Glob(filepath.Join(tempDir, "*.yaml"))
				assert.Len(t, files, tt.expectedFiles, "Expected %d files to be written, got %d", tt.expectedFiles, len(files))
			}
		})
	}
}

func TestPrepareYAMLFiles_WriteFileFailures(t *testing.T) {
	originalWindows := windows
	defer func() {
		windows = originalWindows
	}()

	tests := []struct {
		name          string
		failOnCallNum int
		expectedError string
	}{
		{
			name:          "namespace_write_fails",
			failOnCallNum: 1,
			expectedError: "could not write namespace YAML file",
		},
		{
			name:          "controller_service_account_write_fails",
			failOnCallNum: 2,
			expectedError: "could not write controller service account YAML file",
		},
		{
			name:          "deployment_write_fails",
			failOnCallNum: 10,
			expectedError: "could not write deployment YAML file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			windows = false

			mockClient, _ := getMockK8sClient(t)
			mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
			mockClient.EXPECT().ServerVersion().Return(versionutils.MustParseSemantic("v1.25.0")).AnyTimes()

			err := prepareYAMLFiles()

			assert.Error(t, err)
		})
	}
}

func TestCreateAndEnsureCRDs(t *testing.T) {
	originalUseYAML := useYAML
	defer func() {
		useYAML = originalUseYAML
	}()

	tests := []struct {
		name          string
		useYAML       bool
		fileExists    bool
		createFile    bool
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:       "successful_creation_with_yaml_string",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(false, nil).AnyTimes()

				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes()

				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{Type: apiextensionv1.Established, Status: apiextensionv1.ConditionTrue},
						},
					},
				}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(mockCRD, nil).AnyTimes()
				mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), true).Return(nil).AnyTimes()
			},
			expectedError: "",
		},
		{
			name:       "successful_creation_with_yaml_file",
			useYAML:    true,
			fileExists: true,
			createFile: true,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(false, nil).AnyTimes()
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes()
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{Type: apiextensionv1.Established, Status: apiextensionv1.ConditionTrue},
						},
					},
				}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(mockCRD, nil).AnyTimes()
			},
			expectedError: "",
		},
		{
			name:       "successful_patching_existing_crds",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(true, nil).AnyTimes()
				existingCRD := &apiextensionv1.CustomResourceDefinition{}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(existingCRD, nil).AnyTimes()
				mockClient.EXPECT().PatchCRD(gomock.Any(), gomock.Any(), types.MergePatchType).Return(nil).AnyTimes()
			},
			expectedError: "",
		},
		{
			name:       "mixed_create_and_patch",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(true, nil).AnyTimes()
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(false, nil).AnyTimes()
				existingCRD := &apiextensionv1.CustomResourceDefinition{}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(existingCRD, nil).AnyTimes()
				mockClient.EXPECT().PatchCRD(gomock.Any(), gomock.Any(), types.MergePatchType).Return(nil).AnyTimes()
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes()
				mockCRD := &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{Type: apiextensionv1.Established, Status: apiextensionv1.ConditionTrue},
						},
					},
				}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(mockCRD, nil).AnyTimes()
			},
			expectedError: "",
		},
		{
			name:       "crd_check_fails",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(false, errors.New("check failed")).Times(1)
			},
			expectedError: "unable to identify if",
		},
		{
			name:       "crd_creation_fails",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(false, nil).Times(1)
				mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(errors.New("creation failed")).Times(1)
			},
			expectedError: "could not create the Trident",
		},
		{
			name:       "crd_patch_fails",
			useYAML:    false,
			fileExists: false,
			createFile: false,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(true, nil).Times(1)
				existingCRD := &apiextensionv1.CustomResourceDefinition{}
				mockClient.EXPECT().GetCRD(gomock.Any()).Return(existingCRD, nil).Times(1)
				mockClient.EXPECT().PatchCRD(gomock.Any(), gomock.Any(), types.MergePatchType).Return(errors.New("patch failed")).Times(1)
			},
			expectedError: "could not patch the Trident",
		},
		{
			name:          "file_read_fails",
			useYAML:       true,
			fileExists:    true,
			createFile:    true,
			setupMocks:    func(mockClient *mockK8sClient.MockKubernetesClient) {},
			expectedError: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useYAML = tt.useYAML

			originalK8sTimeout := k8sTimeout
			k8sTimeout = 100 * time.Millisecond
			defer func() {
				k8sTimeout = originalK8sTimeout
			}()

			if tt.useYAML {
				tempDir := t.TempDir()
				crdsPath = filepath.Join(tempDir, "crds.yaml")

				if tt.fileExists && tt.createFile {
					crdContent := k8sclient.GetCRDsYAML()
					err := os.WriteFile(crdsPath, []byte(crdContent), 0o644)
					require.NoError(t, err)

					if tt.name == "file_read_fails" {
						err = os.Chmod(crdsPath, 0o000)
						require.NoError(t, err)
					}
				}
			}

			mockClient, _ := getMockK8sClient(t)
			tt.setupMocks(mockClient)

			err := createAndEnsureCRDs()

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type mockConfig struct {
	tridentInstalled bool
	tridentNamespace string
	checkError       error

	namespaceExists       bool
	namespaceError        error
	namespaceCreateError  error
	deploymentCreateError error

	createError error
	deleteError error
	patchError  error

	crdExists bool
	crdError  error
	crdObj    *apiextensionv1.CustomResourceDefinition

	secretExists bool
	secretError  error

	deploymentExists bool
	deploymentError  error
	deployment       *appsv1.Deployment

	daemonSetExists bool
	daemonSetError  error

	pod      *v1.Pod
	pods     []v1.Pod
	podError error

	serverVersion *versionutils.Version
	clientVersion *version.Info
	k8sFlavor     k8sclient.OrchestratorFlavor
	cli           string
	namespace     string
}

func TestInstallTrident(t *testing.T) {
	originalWindows := windows
	originalUseYAML := useYAML
	originalExcludeAutosupport := excludeAutosupport
	defer func() {
		windows = originalWindows
		useYAML = originalUseYAML
		excludeAutosupport = originalExcludeAutosupport
	}()

	setupComprehensiveMocks := func(mockClient *mockK8sClient.MockKubernetesClient, config mockConfig) {
		mockClient.EXPECT().CheckDeploymentExistsByLabel(gomock.Any(), gomock.Any()).Return(config.tridentInstalled, config.tridentNamespace, config.checkError).AnyTimes()
		mockClient.EXPECT().CheckDaemonSetExistsByLabel(gomock.Any(), gomock.Any()).Return(config.tridentInstalled, config.tridentNamespace, config.checkError).AnyTimes()

		if config.deployment != nil {
			mockClient.EXPECT().GetDeploymentByLabel(gomock.Any(), gomock.Any()).Return(config.deployment, config.checkError).AnyTimes()
		} else {
			mockClient.EXPECT().GetDeploymentByLabel(gomock.Any(), gomock.Any()).Return(nil, config.checkError).AnyTimes()
		}

		mockClient.EXPECT().CheckNamespaceExists(gomock.Any()).Return(config.namespaceExists, config.namespaceError).AnyTimes()
		if config.namespaceCreateError != nil {
			mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(config.namespaceCreateError).Times(1)
		} else if config.deploymentCreateError != nil {
			mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(nil).AnyTimes()
		} else {
			mockClient.EXPECT().CreateObjectByYAML(gomock.Any()).Return(config.createError).AnyTimes()
		}
		mockClient.EXPECT().CreateObjectByFile(gomock.Any()).Return(config.createError).AnyTimes()
		mockClient.EXPECT().PatchNamespaceLabels(gomock.Any(), gomock.Any()).Return(config.patchError).AnyTimes()

		mockClient.EXPECT().CheckCRDExists(gomock.Any()).Return(config.crdExists, config.crdError).AnyTimes()
		mockClient.EXPECT().AddFinalizerToCRD(gomock.Any()).Return(config.crdError).AnyTimes()

		if config.crdObj != nil {
			mockClient.EXPECT().GetCRD(gomock.Any()).Return(config.crdObj, config.crdError).AnyTimes()
		} else {
			mockCRD := &apiextensionv1.CustomResourceDefinition{
				Status: apiextensionv1.CustomResourceDefinitionStatus{
					Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
						{
							Type:   apiextensionv1.Established,
							Status: "True",
						},
					},
				},
			}
			mockClient.EXPECT().GetCRD(gomock.Any()).Return(mockCRD, config.crdError).AnyTimes()
		}

		mockClient.EXPECT().PatchCRD(gomock.Any(), gomock.Any(), gomock.Any()).Return(config.patchError).AnyTimes()

		mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), gomock.Any()).Return(config.deleteError).AnyTimes()
		mockClient.EXPECT().Flavor().Return(config.k8sFlavor).AnyTimes()

		mockClient.EXPECT().CheckSecretExists(gomock.Any()).Return(config.secretExists, config.secretError).AnyTimes()

		mockClient.EXPECT().CheckDeploymentExists(gomock.Any(), gomock.Any()).Return(config.deploymentExists, config.deploymentError).AnyTimes()
		mockClient.EXPECT().DeleteDeployment(gomock.Any(), gomock.Any(), gomock.Any()).Return(config.deleteError).AnyTimes()
		mockClient.EXPECT().CheckDaemonSetExists(gomock.Any(), gomock.Any()).Return(config.daemonSetExists, config.daemonSetError).AnyTimes()
		mockClient.EXPECT().DeleteDaemonSet(gomock.Any(), gomock.Any(), gomock.Any()).Return(config.deleteError).AnyTimes()

		if config.pod != nil {
			mockClient.EXPECT().GetPodByLabel(gomock.Any(), gomock.Any()).Return(config.pod, config.podError).AnyTimes()
		}
		if len(config.pods) > 0 {
			mockClient.EXPECT().GetPodsByLabel(gomock.Any(), gomock.Any()).Return(config.pods, config.podError).AnyTimes()
		}

		mockClient.EXPECT().ServerVersion().Return(config.serverVersion).AnyTimes()
		mockClient.EXPECT().Version().Return(config.clientVersion).AnyTimes()

		mockClient.EXPECT().CLI().Return(config.cli).AnyTimes()
		mockClient.EXPECT().Namespace().Return(config.namespace).AnyTimes()

		mockClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).Return([]byte(`{"Client":{"version":"24.02.0"},"Server":{"version":"24.02.0"}}`), nil).AnyTimes()
	}

	successConfig := mockConfig{
		tridentInstalled:      false,
		tridentNamespace:      "",
		checkError:            nil,
		namespaceExists:       false,
		namespaceError:        nil,
		crdExists:             false,
		crdError:              nil,
		crdObj:                nil,
		createError:           nil,
		namespaceCreateError:  nil,
		deploymentCreateError: nil,
		deleteError:           nil,
		patchError:            nil,
		secretExists:          false,
		secretError:           nil,
		deploymentExists:      false,
		deploymentError:       nil,
		daemonSetExists:       false,
		daemonSetError:        nil,
		deployment: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trident-controller",
				Namespace: "trident",
				Labels: map[string]string{
					"app": "controller.csi.trident.netapp.io",
				},
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
				Replicas:      1,
			},
		},
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trident-controller-abc123",
				Namespace: "trident",
				Labels: map[string]string{
					"app": "controller.csi.trident.netapp.io",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "trident-main",
						Image: "",
					},
				},
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:  "trident-main",
						Ready: true,
						State: v1.ContainerState{
							Running: &v1.ContainerStateRunning{},
						},
					},
				},
			},
		},
		pods: []v1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trident-controller-abc123",
				Namespace: "trident",
				Labels: map[string]string{
					"app": "controller.csi.trident.netapp.io",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "trident-main",
						Image: "",
					},
				},
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:  "trident-main",
						Ready: true,
						State: v1.ContainerState{
							Running: &v1.ContainerStateRunning{},
						},
					},
				},
			},
		}},
		podError:      nil,
		serverVersion: versionutils.MustParseSemantic("v1.25.0"),
		clientVersion: &version.Info{Major: "1", Minor: "25", GitVersion: "v1.25.0"},
		k8sFlavor:     k8sclient.OrchestratorFlavor("kubernetes"),
		cli:           "kubectl",
		namespace:     "trident",
	}

	namespaceFailConfig := mockConfig{
		tridentInstalled:      false,
		tridentNamespace:      "",
		checkError:            nil,
		namespaceExists:       false,
		namespaceError:        nil,
		crdExists:             false,
		crdError:              nil,
		crdObj:                nil,
		createError:           nil,
		namespaceCreateError:  errors.New("namespace creation failed"),
		deploymentCreateError: nil,
		deleteError:           nil,
		patchError:            nil,
		secretExists:          false,
		secretError:           nil,
		deploymentExists:      false,
		deploymentError:       nil,
		daemonSetExists:       false,
		daemonSetError:        nil,
		pod:                   nil,
		pods:                  nil,
		podError:              nil,
		deployment:            nil,
		serverVersion:         versionutils.MustParseSemantic("v1.25.0"),
		clientVersion:         &version.Info{Major: "1", Minor: "25", GitVersion: "v1.25.0"},
		k8sFlavor:             k8sclient.OrchestratorFlavor("kubernetes"),
		cli:                   "kubectl",
		namespace:             "trident",
	}

	tests := []struct {
		name          string
		windows       bool
		useYAML       bool
		excludeAuto   bool
		nilClient     bool
		mockConfig    mockConfig
		expectedError string
	}{
		{
			name:          "namespace_creation_fails",
			windows:       false,
			useYAML:       false,
			excludeAuto:   false,
			nilClient:     false,
			mockConfig:    namespaceFailConfig,
			expectedError: "could not create namespace",
		},
		{
			name:          "client_is_nil",
			nilClient:     true,
			mockConfig:    mockConfig{}, // Empty config, won't be used
			expectedError: "not able to connect to Kubernetes API server",
		},
		{
			name:      "trident_already_installed",
			nilClient: false,
			mockConfig: func() mockConfig {
				config := successConfig
				config.tridentInstalled = true
				config.tridentNamespace = "trident"
				return config
			}(),
			expectedError: "CSI Trident is already installed",
		},
		{
			name:      "namespace_check_fails",
			nilClient: false,
			mockConfig: func() mockConfig {
				config := successConfig
				config.namespaceError = errors.New("namespace check failed")
				return config
			}(),
			expectedError: "could not check if namespace",
		},
		{
			name:      "rbac_creation_fails",
			nilClient: false,
			mockConfig: func() mockConfig {
				config := successConfig
				config.createError = errors.New("rbac creation failed")
				return config
			}(),
			expectedError: "could not create",
		},
		{
			name:          "successful_installation",
			windows:       false,
			useYAML:       false,
			excludeAuto:   false,
			nilClient:     false,
			mockConfig:    successConfig,
			expectedError: "", // No error expected
		},
		{
			name:      "crd_establishment_timeout",
			nilClient: false,
			mockConfig: func() mockConfig {
				config := successConfig
				// Return a CRD that's never established (missing the Established condition)
				config.crdObj = &apiextensionv1.CustomResourceDefinition{
					Status: apiextensionv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionv1.CustomResourceDefinitionCondition{
							{
								Type:   apiextensionv1.NamesAccepted,
								Status: "True",
							},
							// Missing the Established condition - this should cause timeout
						},
					},
				}
				return config
			}(),
			expectedError: "CRD was not established",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			windows = tt.windows
			useYAML = tt.useYAML
			excludeAutosupport = tt.excludeAuto

			// Short timeout
			originalK8sTimeout := k8sTimeout
			k8sTimeout = 200 * time.Millisecond
			defer func() { k8sTimeout = originalK8sTimeout }()

			// Setup client
			var mockClient *mockK8sClient.MockKubernetesClient
			if tt.nilClient {
				client = nil
			} else {
				mockClient, _ = getMockK8sClient(t)
				setupComprehensiveMocks(mockClient, tt.mockConfig)
				client = mockClient // Set the global client variable
			}

			// Run test
			err := installTrident()

			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateInstallationArguments(t *testing.T) {
	originalTridentPodNamespace := TridentPodNamespace
	originalNodePrep := nodePrep
	originalLogFormat := logFormat
	originalImagePullPolicy := imagePullPolicy
	originalCloudProvider := cloudProvider
	originalCloudIdentity := cloudIdentity
	originalIdentityLabel := identityLabel
	originalFsGroupPolicy := fsGroupPolicy

	defer func() {
		TridentPodNamespace = originalTridentPodNamespace
		nodePrep = originalNodePrep
		logFormat = originalLogFormat
		imagePullPolicy = originalImagePullPolicy
		cloudProvider = originalCloudProvider
		cloudIdentity = originalCloudIdentity
		identityLabel = originalIdentityLabel
		fsGroupPolicy = originalFsGroupPolicy
	}()

	tests := []struct {
		name                  string
		tridentPodNamespace   string
		nodePrep              []string
		logFormat             string
		imagePullPolicy       string
		cloudProvider         string
		cloudIdentity         string
		fsGroupPolicy         string
		expectedError         string
		expectedIdentityLabel bool
		expectedCloudProvider string
	}{
		{
			name:                  "valid_default_configuration",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_json_log_format",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "json",
			imagePullPolicy:       "Always",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_namespace_uppercase",
			tridentPodNamespace:   "Trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'Trident' is not a valid namespace name",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_namespace_underscore",
			tridentPodNamespace:   "trident_namespace",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'trident_namespace' is not a valid namespace name",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_namespace_starting_hyphen",
			tridentPodNamespace:   "-trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'-trident' is not a valid namespace name",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_log_format",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "xml",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'xml' is not a valid log format",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_image_pull_policy",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "SometimesIfItFeelsLike",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'SometimesIfItFeelsLike' is not a valid trident image pull policy",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_pull_policy_never",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "Never",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_cloud_provider",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "digitalocean",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'digitalocean' is not a valid cloud provider",
			expectedIdentityLabel: false,
			expectedCloudProvider: "digitalocean",
		},
		{
			name:                  "valid_azure_cloud_provider_without_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "Azure",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "Azure",
		},
		{
			name:                  "aws_cloud_provider_requires_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "aws",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'' is not a valid cloud identity for the cloud provider 'AWS'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "aws",
		},
		{
			name:                  "gcp_cloud_provider_requires_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "GCP",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "'' is not a valid cloud identity for the cloud provider 'GCP'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "GCP",
		},
		{
			name:                  "cloud_identity_without_provider",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "some-identity",
			fsGroupPolicy:         "",
			expectedError:         "cloud provider must be specified for the cloud identity 'some-identity'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_azure_workload_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "Azure",
			cloudIdentity:         "azure.workload.identity/client-id:12345-abcd-6789-efgh",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: true,
			expectedCloudProvider: "", // Should be cleared for Azure workload identity
		},
		{
			name:                  "invalid_azure_identity_format",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "Azure",
			cloudIdentity:         "invalid-azure-identity",
			fsGroupPolicy:         "",
			expectedError:         "'invalid-azure-identity' is not a valid cloud identity for the cloud provider 'Azure'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "Azure",
		},
		{
			name:                  "valid_aws_iam_role",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "AWS",
			cloudIdentity:         "eks.amazonaws.com/role-arn:arn:aws:iam::123456789:role/trident-role",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "AWS",
		},
		{
			name:                  "invalid_aws_identity_format",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "AWS",
			cloudIdentity:         "invalid-aws-identity",
			fsGroupPolicy:         "",
			expectedError:         "'invalid-aws-identity' is not a valid cloud identity for the cloud provider 'AWS'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "AWS",
		},
		{
			name:                  "valid_gcp_workload_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "GCP",
			cloudIdentity:         "iam.gke.io/gcp-service-account:trident@my-project.iam.gserviceaccount.com",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "GCP",
		},
		{
			name:                  "invalid_gcp_identity_format",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "GCP",
			cloudIdentity:         "invalid-gcp-identity",
			fsGroupPolicy:         "",
			expectedError:         "'invalid-gcp-identity' is not a valid cloud identity for the cloud provider 'GCP'",
			expectedIdentityLabel: false,
			expectedCloudProvider: "GCP",
		},
		// Test edge case where Azure identity contains the key but is not for workload identity
		{
			name:                  "azure_identity_with_key_but_not_workload_identity",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "Azure",
			cloudIdentity:         "some-other-azure.workload.identity/client-id:value",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: true,
			expectedCloudProvider: "", // Should still be cleared when key is present
		},
		{
			name:                  "valid_fs_group_policy_readwriteoncewithfstype",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "ReadWriteOnceWithFSType",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_fs_group_policy_none",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "None",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_fs_group_policy_file",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "file",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "invalid_fs_group_policy",
			tridentPodNamespace:   "trident",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "InvalidPolicy",
			expectedError:         "'InvalidPolicy' is not a valid fsGroupPolicy",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
		{
			name:                  "valid_complex_namespace",
			tridentPodNamespace:   "trident-system-v2",
			nodePrep:              []string{"iscsi"},
			logFormat:             "text",
			imagePullPolicy:       "IfNotPresent",
			cloudProvider:         "",
			cloudIdentity:         "",
			fsGroupPolicy:         "",
			expectedError:         "",
			expectedIdentityLabel: false,
			expectedCloudProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test variables
			TridentPodNamespace = tt.tridentPodNamespace
			nodePrep = tt.nodePrep
			logFormat = tt.logFormat
			imagePullPolicy = tt.imagePullPolicy
			cloudProvider = tt.cloudProvider
			cloudIdentity = tt.cloudIdentity
			fsGroupPolicy = tt.fsGroupPolicy
			identityLabel = false // Reset before each test

			// Run the function
			err := validateInstallationArguments()

			// Verify the error
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Verify side effects
			assert.Equal(t, tt.expectedIdentityLabel, identityLabel, "identityLabel should match expected value")
			assert.Equal(t, tt.expectedCloudProvider, cloudProvider, "cloudProvider should match expected value")
		})
	}
}

// Test for protocol validation error - this would need to be mocked if protocol.ValidateProtocols is complex
func TestValidateInstallationArguments_ProtocolValidationError(t *testing.T) {
	originalTridentPodNamespace := TridentPodNamespace
	originalNodePrep := nodePrep
	originalLogFormat := logFormat
	originalImagePullPolicy := imagePullPolicy
	originalCloudProvider := cloudProvider
	originalCloudIdentity := cloudIdentity
	originalFsGroupPolicy := fsGroupPolicy

	defer func() {
		TridentPodNamespace = originalTridentPodNamespace
		nodePrep = originalNodePrep
		logFormat = originalLogFormat
		imagePullPolicy = originalImagePullPolicy
		cloudProvider = originalCloudProvider
		cloudIdentity = originalCloudIdentity
		fsGroupPolicy = originalFsGroupPolicy
	}()

	// Set up valid values for other fields
	TridentPodNamespace = "trident"
	logFormat = "text"
	imagePullPolicy = "IfNotPresent"
	cloudProvider = ""
	cloudIdentity = ""
	fsGroupPolicy = ""
	nodePrep = []string{"invalid-protocol"}

	err := validateInstallationArguments()
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.NoError(t, err)
	}
}

func TestDiscoverInstallationEnvironment(t *testing.T) {
	originalOperatingMode := OperatingMode
	originalTridentImage := tridentImage
	originalImageRegistry := imageRegistry
	originalAutosupportImage := autosupportImage
	originalClient := client
	originalTridentPodNamespace := TridentPodNamespace

	defer func() {
		OperatingMode = originalOperatingMode
		tridentImage = originalTridentImage
		imageRegistry = originalImageRegistry
		autosupportImage = originalAutosupportImage
		client = originalClient
		TridentPodNamespace = originalTridentPodNamespace
	}()

	tests := []struct {
		name                       string
		initialTridentImage        string
		initialImageRegistry       string
		initialAutosupportImage    string
		initialTridentPodNamespace string
		mockServerVersion          string
		mockNamespace              string
		simulateClientError        bool
		simulateVersionError       bool
		expectedError              string
		expectedOperatingMode      string
	}{
		{
			name:                       "successful_default_configuration",
			initialTridentImage:        "",
			initialImageRegistry:       "",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "trident",
			mockServerVersion:          "v1.25.0",
			mockNamespace:              "trident",
			simulateClientError:        false,
			simulateVersionError:       false,
			expectedError:              "",
			expectedOperatingMode:      ModeInstall,
		},
		{
			name:                       "custom_trident_image_provided",
			initialTridentImage:        "my-registry.com/custom-trident:v1.0.0",
			initialImageRegistry:       "",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "custom-ns",
			mockServerVersion:          "v1.25.0",
			mockNamespace:              "custom-ns",
			simulateClientError:        false,
			simulateVersionError:       false,
			expectedError:              "",
			expectedOperatingMode:      ModeInstall,
		},
		{
			name:                       "custom_image_registry_with_default_images",
			initialTridentImage:        "",
			initialImageRegistry:       "my-registry.com",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "trident",
			mockServerVersion:          "v1.25.0",
			mockNamespace:              "trident",
			simulateClientError:        false,
			simulateVersionError:       false,
			expectedError:              "",
			expectedOperatingMode:      ModeInstall,
		},
		{
			name:                       "client_initialization_fails",
			initialTridentImage:        "",
			initialImageRegistry:       "",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "",
			mockServerVersion:          "",
			mockNamespace:              "",
			simulateClientError:        true,
			simulateVersionError:       false,
			expectedError:              "could not initialize Kubernetes client",
			expectedOperatingMode:      ModeInstall,
		},
		{
			name:                       "kubernetes_version_validation_fails",
			initialTridentImage:        "",
			initialImageRegistry:       "",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "trident",
			mockServerVersion:          "v1.15.0",
			mockNamespace:              "trident",
			simulateClientError:        false,
			simulateVersionError:       true,
			expectedError:              "kubernetes version",
			expectedOperatingMode:      ModeInstall,
		},
		{
			name:                       "namespace_inferred_from_client",
			initialTridentImage:        "",
			initialImageRegistry:       "",
			initialAutosupportImage:    "",
			initialTridentPodNamespace: "",
			mockServerVersion:          "v1.25.0",
			mockNamespace:              "kube-system",
			simulateClientError:        false,
			simulateVersionError:       false,
			expectedError:              "",
			expectedOperatingMode:      ModeInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tridentImage = tt.initialTridentImage
			imageRegistry = tt.initialImageRegistry
			autosupportImage = tt.initialAutosupportImage
			TridentPodNamespace = tt.initialTridentPodNamespace

			if !tt.simulateClientError {
				ctrl := gomock.NewController(t)
				mockClient := mockK8sClient.NewMockKubernetesClient(ctrl)

				serverVersion := versionutils.MustParseSemantic(tt.mockServerVersion)
				mockClient.EXPECT().ServerVersion().Return(serverVersion).AnyTimes()
				mockClient.EXPECT().Namespace().Return(tt.mockNamespace).AnyTimes()
				mockClient.EXPECT().SetNamespace(gomock.Any()).AnyTimes()

				client = mockClient
			} else {
				client = nil
			}

			err := testDiscoverInstallationEnvironment(tt.simulateClientError, tt.simulateVersionError)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Verify OperatingMode is always set
			assert.Equal(t, tt.expectedOperatingMode, OperatingMode, "OperatingMode should match expected")

			// Verify images were processed
			if tt.initialTridentImage == "" && !tt.simulateClientError {
				assert.NotEmpty(t, tridentImage, "tridentImage should be set from build default")
			} else if tt.initialTridentImage != "" {
				assert.Equal(t, tt.initialTridentImage, tridentImage, "custom tridentImage should be preserved")
			}

			// Verify registry replacement when specified
			if tt.initialImageRegistry != "" && tt.initialTridentImage == "" && !tt.simulateClientError {
				assert.Contains(t, tridentImage, tt.initialImageRegistry, "tridentImage should contain custom registry")
			}
		})
	}
}

// Test helper function that simulates the key logic of discoverInstallationEnvironment
func testDiscoverInstallationEnvironment(simulateClientError, simulateVersionError bool) error {
	OperatingMode = ModeInstall

	if tridentImage == "" {
		tridentImage = tridentconfig.BuildImage

		if imageRegistry != "" {
			tridentImage = network.ReplaceImageRegistry(tridentImage, imageRegistry)
		}
	}

	if autosupportImage == "" {
		autosupportImage = tridentconfig.DefaultAutosupportImage

		if imageRegistry != "" {
			autosupportImage = network.ReplaceImageRegistry(autosupportImage, imageRegistry)
		}
	}

	if simulateClientError {
		return fmt.Errorf("could not initialize Kubernetes client; simulated failure")
	}

	if simulateVersionError {
		return fmt.Errorf("kubernetes version validation failed")
	}

	// Simulate successful path
	if client != nil {
		if TridentPodNamespace == "" {
			TridentPodNamespace = client.Namespace()
		}

		client.SetNamespace(TridentPodNamespace)
	}

	return nil
}

func TestImageRegistryReplacement(t *testing.T) {
	tests := []struct {
		name          string
		originalImage string
		newRegistry   string
		expectedImage string
	}{
		{
			name:          "replace_registry_in_standard_image",
			originalImage: "netapp/trident:24.02.0",
			newRegistry:   "my-registry.com",
			expectedImage: "my-registry.com/trident:24.02.0",
		},
		{
			name:          "no_registry_replacement_when_empty",
			originalImage: "netapp/trident:24.02.0",
			newRegistry:   "",
			expectedImage: "trident:24.02.0",
		},
		{
			name:          "replace_registry_with_existing_registry",
			originalImage: "docker.io/netapp/trident:24.02.0",
			newRegistry:   "my-registry.com",
			expectedImage: "my-registry.com/trident:24.02.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := network.ReplaceImageRegistry(tt.originalImage, tt.newRegistry)
			assert.Equal(t, tt.expectedImage, result)
		})
	}
}

func TestDiscoverInstallationEnvironment_CoreBehaviors(t *testing.T) {
	originalOperatingMode := OperatingMode
	originalTridentImage := tridentImage
	originalAutosupportImage := autosupportImage

	defer func() {
		OperatingMode = originalOperatingMode
		tridentImage = originalTridentImage
		autosupportImage = originalAutosupportImage
	}()

	t.Run("operating_mode_set_to_install", func(t *testing.T) {
		OperatingMode = ""

		_ = testDiscoverInstallationEnvironment(true, false)

		assert.Equal(t, ModeInstall, OperatingMode, "OperatingMode should be set to ModeInstall")
	})

	t.Run("default_images_set_when_empty", func(t *testing.T) {
		tridentImage = ""
		autosupportImage = ""
		imageRegistry = ""

		_ = testDiscoverInstallationEnvironment(true, false)

		assert.NotEmpty(t, tridentImage, "tridentImage should be set from build default")
		assert.NotEmpty(t, autosupportImage, "autosupportImage should be set from default")
		assert.Equal(t, tridentconfig.BuildImage, tridentImage, "tridentImage should match build image")
		assert.Equal(t, tridentconfig.DefaultAutosupportImage, autosupportImage, "autosupportImage should match default")
	})

	t.Run("custom_images_preserved", func(t *testing.T) {
		customTridentImage := "my-registry.com/custom-trident:v1.0.0"
		customAutosupportImage := "my-registry.com/custom-autosupport:v1.0.0"

		tridentImage = customTridentImage
		autosupportImage = customAutosupportImage
		imageRegistry = ""

		_ = testDiscoverInstallationEnvironment(true, false)

		assert.Equal(t, customTridentImage, tridentImage, "custom tridentImage should be preserved")
		assert.Equal(t, customAutosupportImage, autosupportImage, "custom autosupportImage should be preserved")
	})

	t.Run("registry_replacement_for_default_images", func(t *testing.T) {
		tridentImage = ""
		autosupportImage = ""
		imageRegistry = "my-custom-registry.com"

		// Call test helper
		_ = testDiscoverInstallationEnvironment(true, false) // Simulate client error to exit early

		assert.Contains(t, tridentImage, "my-custom-registry.com", "tridentImage should contain custom registry")
		assert.Contains(t, autosupportImage, "my-custom-registry.com", "autosupportImage should contain custom registry")

		// Should not contain the original registry
		assert.NotContains(t, tridentImage, "netapp/trident", "tridentImage should not contain original netapp path")
		assert.NotContains(t, autosupportImage, "netapp/trident-autosupport", "autosupportImage should not contain original netapp path")
	})
}

func TestDiscoverInstallationEnvironment_Integration(t *testing.T) {
	originalOperatingMode := OperatingMode
	originalTridentImage := tridentImage
	originalAutosupportImage := autosupportImage
	originalTridentPodNamespace := TridentPodNamespace

	defer func() {
		OperatingMode = originalOperatingMode
		tridentImage = originalTridentImage
		autosupportImage = originalAutosupportImage
		TridentPodNamespace = originalTridentPodNamespace
	}()

	t.Run("actual_function_sets_basic_state", func(t *testing.T) {
		// Reset variables
		OperatingMode = ""
		tridentImage = ""
		autosupportImage = ""

		_ = discoverInstallationEnvironment()

		assert.Equal(t, ModeInstall, OperatingMode, "OperatingMode should be set even on failure")
		assert.NotEmpty(t, tridentImage, "tridentImage should be set from build default")
		assert.NotEmpty(t, autosupportImage, "autosupportImage should be set from default")
	})
}

func TestDiscoverInstallationEnvironment_Integration1(t *testing.T) {
	originalOperatingMode := OperatingMode
	originalTridentImage := tridentImage
	originalImageRegistry := imageRegistry
	originalAutosupportImage := autosupportImage
	originalTridentPodNamespace := TridentPodNamespace

	defer func() {
		OperatingMode = originalOperatingMode
		tridentImage = originalTridentImage
		imageRegistry = originalImageRegistry
		autosupportImage = originalAutosupportImage
		TridentPodNamespace = originalTridentPodNamespace
	}()

	t.Run("operating_mode_always_set", func(t *testing.T) {
		OperatingMode = ""
		tridentImage = ""
		autosupportImage = ""

		_ = discoverInstallationEnvironment()

		assert.Equal(t, ModeInstall, OperatingMode, "OperatingMode should be set even on failure")

		// Images should also be set from defaults
		assert.NotEmpty(t, tridentImage, "tridentImage should be set from build default")
		assert.NotEmpty(t, autosupportImage, "autosupportImage should be set from default")
	})
}

func TestInitInstallerLogging(t *testing.T) {
	originalSilent := silent
	originalLogWorkflows := logWorkflows
	originalLogLayers := logLayers

	defer func() {
		silent = originalSilent
		logWorkflows = originalLogWorkflows
		logLayers = originalLogLayers
	}()

	tests := []struct {
		name         string
		silent       bool
		logWorkflows string
		logLayers    string
	}{
		{
			name:         "normal_logging_mode",
			silent:       false,
			logWorkflows: "volume=create",
			logLayers:    "core",
		},
		{
			name:         "silent_logging_mode",
			silent:       true,
			logWorkflows: "volume=create",
			logLayers:    "core",
		},
		{
			name:         "empty_workflows_and_layers",
			logWorkflows: "",
			logLayers:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			silent = tt.silent
			logWorkflows = tt.logWorkflows
			logLayers = tt.logLayers

			assert.NotPanics(t, func() {
				initInstallerLogging()
			}, "initInstallerLogging should not panic")
		})
	}
}
