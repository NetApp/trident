package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/utils/errors"
)

func TestUninstallTridentFunctions(t *testing.T) {
	originalClient := client
	originalTridentPodNamespace := TridentPodNamespace
	originalAppLabel := appLabel
	originalAppLabelKey := appLabelKey
	originalAppLabelValue := appLabelValue

	defer func() {
		client = originalClient
		TridentPodNamespace = originalTridentPodNamespace
		appLabel = originalAppLabel
		appLabelKey = originalAppLabelKey
		appLabelValue = originalAppLabelValue
	}()

	tests := []struct {
		name                        string
		functionToTest              string
		tridentPodNamespace         string
		setupClient                 bool
		mockClientConfig            mockUninstallClientConfig
		expectedError               string
		expectedAppLabelSet         bool
		expectedTridentInstalled    bool
		expectedNamespaceValidation bool
	}{
		// discoverTrident tests
		{
			name:                "discoverTrident_success_installed",
			functionToTest:      "discoverTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:  true,
				tridentNamespace:  "trident",
				checkInstallError: nil,
			},
			expectedError:            "",
			expectedTridentInstalled: true,
		},
		{
			name:                "discoverTrident_success_not_installed",
			functionToTest:      "discoverTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:  false,
				tridentNamespace:  "",
				checkInstallError: nil,
			},
			expectedError:            "",
			expectedTridentInstalled: false,
		},
		{
			name:                "discoverTrident_check_installation_fails",
			functionToTest:      "discoverTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:  false,
				tridentNamespace:  "",
				checkInstallError: errors.New("failed to check installation"),
			},
			expectedError:            "could not check if CSI Trident is installed",
			expectedTridentInstalled: false,
		},

		// validateUninstallationArguments tests
		{
			name:                        "validateUninstallationArguments_valid_namespace",
			functionToTest:              "validateUninstallationArguments",
			tridentPodNamespace:         "trident",
			setupClient:                 false,
			expectedError:               "",
			expectedNamespaceValidation: true,
		},
		{
			name:                        "validateUninstallationArguments_invalid_namespace_uppercase",
			functionToTest:              "validateUninstallationArguments",
			tridentPodNamespace:         "Trident",
			setupClient:                 false,
			expectedError:               "Trident is not a valid namespace name",
			expectedNamespaceValidation: false,
		},
		{
			name:                        "validateUninstallationArguments_invalid_namespace_underscore",
			functionToTest:              "validateUninstallationArguments",
			tridentPodNamespace:         "trident_namespace",
			setupClient:                 false,
			expectedError:               "trident_namespace is not a valid namespace name",
			expectedNamespaceValidation: false,
		},
		{
			name:                        "validateUninstallationArguments_invalid_namespace_hyphen_start",
			functionToTest:              "validateUninstallationArguments",
			tridentPodNamespace:         "-trident",
			setupClient:                 false,
			expectedError:               "-trident is not a valid namespace name",
			expectedNamespaceValidation: false,
		},

		// uninstallTrident tests
		{
			name:                "uninstallTrident_client_nil",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         false,
			expectedError:       "not able to connect to Kubernetes API server",
		},
		{
			name:                "uninstallTrident_discover_fails",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				checkInstallError: errors.New("discovery failed"),
			},
			expectedError: "could not check if CSI Trident is installed",
		},
		{
			name:                "uninstallTrident_deployment_namespace_mismatch",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:    true,
				tridentNamespace:    "trident",
				checkInstallError:   nil,
				deploymentExists:    true,
				deploymentNamespace: "different-namespace",
				deploymentName:      "trident-controller",
			},
			expectedError: "a Trident deployment was found in namespace 'different-namespace', not in specified namespace 'trident'",
		},
		{
			name:                "uninstallTrident_daemonset_namespace_mismatch",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:   true,
				tridentNamespace:   "trident",
				checkInstallError:  nil,
				deploymentExists:   false,
				daemonsetExists:    true,
				daemonsetNamespace: "different-namespace",
				daemonsetName:      "trident-csi",
			},
			expectedError: "a Trident DaemonSet was found in namespace 'different-namespace', not in specified namespace 'trident'",
		},
		{
			name:                "uninstallTrident_service_namespace_mismatch",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:  true,
				tridentNamespace:  "trident",
				checkInstallError: nil,
				deploymentExists:  false,
				daemonsetExists:   false,
				serviceExists:     true,
				serviceNamespace:  "different-namespace",
				serviceName:       "trident-csi",
			},
			expectedError: "a Trident service was found in namespace 'different-namespace', not in specified namespace 'trident'",
		},
		{
			name:                "uninstallTrident_resource_quota_namespace_mismatch",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:       true,
				tridentNamespace:       "trident",
				checkInstallError:      nil,
				deploymentExists:       false,
				daemonsetExists:        false,
				serviceExists:          false,
				resourceQuotaExists:    true,
				resourceQuotaNamespace: "different-namespace",
				resourceQuotaName:      "trident-csi",
			},
			expectedError: "a Trident resource quota was found in namespace 'different-namespace', not in specified namespace 'trident'",
		},
		{
			name:                "uninstallTrident_successful_complete_uninstall",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:       true,
				tridentNamespace:       "trident",
				checkInstallError:      nil,
				deploymentExists:       true,
				deploymentNamespace:    "trident",
				deploymentName:         "trident-controller",
				daemonsetExists:        true,
				daemonsetNamespace:     "trident",
				daemonsetName:          "trident-csi",
				serviceExists:          true,
				serviceNamespace:       "trident",
				serviceName:            "trident-csi",
				resourceQuotaExists:    true,
				resourceQuotaNamespace: "trident",
				resourceQuotaName:      "trident-csi",
				secretsExist:           true,
				secretsNamespace:       "trident",
			},
			expectedError:       "",
			expectedAppLabelSet: true,
		},
		{
			name:                "uninstallTrident_with_persistent_secrets",
			functionToTest:      "uninstallTrident",
			tridentPodNamespace: "trident",
			setupClient:         true,
			mockClientConfig: mockUninstallClientConfig{
				tridentInstalled:                 true,
				tridentNamespace:                 "trident",
				checkInstallError:                nil,
				deploymentExists:                 false,
				daemonsetExists:                  false,
				serviceExists:                    false,
				resourceQuotaExists:              false,
				secretsExist:                     true,
				secretsNamespace:                 "trident",
				secretsHavePersistentObjectLabel: true,
			},
			expectedError:       "",
			expectedAppLabelSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TridentPodNamespace = tt.tridentPodNamespace

			if tt.setupClient {
				ctrl := gomock.NewController(t)
				mockClient := setupMockUninstallClient(t, ctrl, tt.mockClientConfig)
				client = mockClient
			} else {
				client = nil
			}

			// Execute the appropriate function based on test case
			switch tt.functionToTest {
			case "discoverTrident":
				installed, err := discoverTrident()

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedTridentInstalled, installed)
				}

			case "validateUninstallationArguments":
				err := validateUninstallationArguments()

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)
				}

			case "uninstallTrident":
				err := uninstallTrident()

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)

					// Verify app labels were set for successful uninstall
					if tt.expectedAppLabelSet {
						assert.Equal(t, TridentCSILabel, appLabel)
						assert.Equal(t, TridentCSILabelKey, appLabelKey)
						assert.Equal(t, TridentCSILabelValue, appLabelValue)
					}
				}
			}
		})
	}
}

// Mock configuration for uninstall client
type mockUninstallClientConfig struct {
	tridentInstalled                 bool
	tridentNamespace                 string
	checkInstallError                error
	deploymentExists                 bool
	deploymentNamespace              string
	deploymentName                   string
	daemonsetExists                  bool
	daemonsetNamespace               string
	daemonsetName                    string
	serviceExists                    bool
	serviceNamespace                 string
	serviceName                      string
	resourceQuotaExists              bool
	resourceQuotaNamespace           string
	resourceQuotaName                string
	secretsExist                     bool
	secretsNamespace                 string
	secretsHavePersistentObjectLabel bool
	deleteErrors                     bool
}

// Helper function to setup mock client for uninstall tests
func setupMockUninstallClient(t *testing.T, ctrl *gomock.Controller, config mockUninstallClientConfig) *mockK8sClient.MockKubernetesClient {
	mockClient := mockK8sClient.NewMockKubernetesClient(ctrl)

	// Mock isCSITridentInstalled call (through discoverTrident)
	if config.checkInstallError != nil {
		// Mock the methods that isCSITridentInstalled would call
		mockClient.EXPECT().CheckDeploymentExistsByLabel(gomock.Any(), gomock.Any()).Return(false, "", config.checkInstallError).AnyTimes()
	} else {
		mockClient.EXPECT().CheckDeploymentExistsByLabel(gomock.Any(), gomock.Any()).Return(config.tridentInstalled, config.tridentNamespace, nil).AnyTimes()
		mockClient.EXPECT().CheckDaemonSetExistsByLabel(gomock.Any(), gomock.Any()).Return(config.tridentInstalled, config.tridentNamespace, nil).AnyTimes()
	}
	mockClient.EXPECT().Flavor().Return(k8sclient.OrchestratorFlavor("kubernetes")).AnyTimes()
	// Mock deployment operations
	if config.deploymentExists {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.deploymentName,
				Namespace: config.deploymentNamespace,
			},
		}
		mockClient.EXPECT().GetDeploymentByLabel(gomock.Any(), gomock.Any()).Return(deployment, nil).AnyTimes()

		if config.deploymentNamespace == TridentPodNamespace {
			if config.deleteErrors {
				mockClient.EXPECT().DeleteDeploymentByLabel(gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			} else {
				mockClient.EXPECT().DeleteDeploymentByLabel(gomock.Any()).Return(nil).AnyTimes()
			}
		}
	} else {
		mockClient.EXPECT().GetDeploymentByLabel(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	}

	// Mock daemonset operations
	if config.daemonsetExists {
		daemonsets := []appsv1.DaemonSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.daemonsetName,
				Namespace: config.daemonsetNamespace,
			},
		}}
		mockClient.EXPECT().GetDaemonSetsByLabel(gomock.Any(), gomock.Any()).Return(daemonsets, nil).AnyTimes()

		if config.daemonsetNamespace == TridentPodNamespace {
			if config.deleteErrors {
				mockClient.EXPECT().DeleteDaemonSetByLabelAndName(gomock.Any(), gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			} else {
				mockClient.EXPECT().DeleteDaemonSetByLabelAndName(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}
		}
	} else {
		mockClient.EXPECT().GetDaemonSetsByLabel(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	}

	// Mock service operations
	if config.serviceExists {
		service := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.serviceName,
				Namespace: config.serviceNamespace,
			},
		}
		mockClient.EXPECT().GetServiceByLabel(gomock.Any(), gomock.Any()).Return(service, nil).AnyTimes()

		if config.serviceNamespace == TridentPodNamespace {
			if config.deleteErrors {
				mockClient.EXPECT().DeleteServiceByLabel(gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			} else {
				mockClient.EXPECT().DeleteServiceByLabel(gomock.Any()).Return(nil).AnyTimes()
			}
		}
	} else {
		mockClient.EXPECT().GetServiceByLabel(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	}

	// Mock resource quota operations
	if config.resourceQuotaExists {
		resourceQuota := &v1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.resourceQuotaName,
				Namespace: config.resourceQuotaNamespace,
			},
		}
		mockClient.EXPECT().GetResourceQuotaByLabel(gomock.Any()).Return(resourceQuota, nil).AnyTimes()

		if config.resourceQuotaNamespace == TridentPodNamespace {
			if config.deleteErrors {
				mockClient.EXPECT().DeleteResourceQuotaByLabel(gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			} else {
				mockClient.EXPECT().DeleteResourceQuotaByLabel(gomock.Any()).Return(nil).AnyTimes()
			}
		}
	} else {
		mockClient.EXPECT().GetResourceQuotaByLabel(gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	}

	// Mock secrets operations
	if config.secretsExist {
		var secrets []v1.Secret
		if config.secretsHavePersistentObjectLabel {
			secrets = []v1.Secret{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persistent-secret",
					Namespace: config.secretsNamespace,
					Labels: map[string]string{
						TridentPersistentObjectLabelKey: TridentPersistentObjectLabelValue,
					},
				},
			}}
		} else {
			secrets = []v1.Secret{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "regular-secret",
					Namespace: config.secretsNamespace,
				},
			}}
			if !config.deleteErrors {
				mockClient.EXPECT().DeleteSecret(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			} else {
				mockClient.EXPECT().DeleteSecret(gomock.Any(), gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			}
		}
		mockClient.EXPECT().GetSecretsByLabel(gomock.Any(), gomock.Any()).Return(secrets, nil).AnyTimes()
	} else {
		mockClient.EXPECT().GetSecretsByLabel(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()
	}

	mockClient.EXPECT().DeleteObjectByYAML(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return mockClient
}
