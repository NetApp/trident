package cmd

import (
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/utils/errors"
)

var obliviateSecretTestMutex sync.RWMutex

const (
	secretTestShortTimeout  = 50 * time.Millisecond
	secretTestNormalTimeout = 100 * time.Millisecond
)

func withObliviateSecretTestMode(t *testing.T, testFunc func()) {
	obliviateSecretTestMutex.Lock()
	defer obliviateSecretTestMutex.Unlock()
	testFunc()
}

func getMockK8sClientForSecrets(t *testing.T) *mockK8sClient.MockKubernetesClient {
	ctrl := gomock.NewController(t)
	mockClient := mockK8sClient.NewMockKubernetesClient(ctrl)

	k8sClient = mockClient

	return mockClient
}

func createTestSecret(name, namespace string, withDeletionTimestamp bool) *v1.Secret {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				TridentCSILabel: "true",
			},
		},
	}
	if withDeletionTimestamp {
		now := metav1.NewTime(time.Now())
		secret.ObjectMeta.DeletionTimestamp = &now
	}
	return secret
}

func TestObliviateSecrets(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name: "success_no_secrets_found",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					GetSecretsByLabel(TridentCSILabel, true).
					Return([]v1.Secret{}, nil).
					Times(1)
			},
		},
		{
			name: "error_getting_secrets",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					GetSecretsByLabel(TridentCSILabel, true).
					Return(nil, errors.New("k8s connection failed")).
					Times(1)
			},
			expectedError: "k8s connection failed",
		},
		{
			name: "success_secrets_deleted",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				secrets := []v1.Secret{*createTestSecret("trident-secret-1", "trident", false)}

				mockClient.EXPECT().
					GetSecretsByLabel(TridentCSILabel, true).
					Return(secrets, nil).
					Times(1)

				mockClient.EXPECT().
					DeleteSecret("trident-secret-1", "trident").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("trident-secret-1").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name: "error_deleting_secrets",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				secrets := []v1.Secret{*createTestSecret("trident-secret-1", "trident", false)}

				mockClient.EXPECT().
					GetSecretsByLabel(TridentCSILabel, true).
					Return(secrets, nil).
					Times(1)

				mockClient.EXPECT().
					DeleteSecret("trident-secret-1", "trident").
					Return(errors.New("permission denied")).
					Times(1)
			},
			expectedError: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateSecretTestMode(t, func() {
				mockClient := getMockK8sClientForSecrets(t)
				tt.setupMocks(mockClient)

				originalTimeout := k8sTimeout
				k8sTimeout = secretTestNormalTimeout
				defer func() { k8sTimeout = originalTimeout }()

				err := obliviateSecrets()

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestDeleteSecrets(t *testing.T) {
	tests := []struct {
		name          string
		secrets       []v1.Secret
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:    "empty_secrets_list",
			secrets: []v1.Secret{},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				// No expectations for empty list
			},
		},
		{
			name:    "success_single_secret_deletion",
			secrets: []v1.Secret{*createTestSecret("test-secret", "default", false)},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					DeleteSecret("test-secret", "default").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name:    "secret_already_has_deletion_timestamp",
			secrets: []v1.Secret{*createTestSecret("test-secret", "default", true)},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name:    "secret_not_found_during_deletion_returns_error",
			secrets: []v1.Secret{*createTestSecret("test-secret", "default", false)},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					DeleteSecret("test-secret", "default").
					Return(errors.NotFoundError("secret not found")).
					Times(1)
			},
			expectedError: "secret not found",
		},
		{
			name:    "secret_deletion_api_error_stops_processing",
			secrets: []v1.Secret{*createTestSecret("test-secret", "default", false)},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					DeleteSecret("test-secret", "default").
					Return(errors.New("permission denied")).
					Times(1)
			},
			expectedError: "permission denied",
		},
		{
			name:    "timeout_waiting_for_deletion",
			secrets: []v1.Secret{*createTestSecret("timeout-secret", "default", false)},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					DeleteSecret("timeout-secret", "default").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("timeout-secret").
					Return(true, nil).
					AnyTimes() // Use AnyTimes to handle variable retry counts
			},
			expectedError: "secret timeout-secret was not deleted after",
		},
		{
			name: "multiple_secrets_success",
			secrets: []v1.Secret{
				*createTestSecret("secret-1", "ns1", false),
				*createTestSecret("secret-2", "ns2", false),
			},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					DeleteSecret("secret-1", "ns1").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("secret-1").
					Return(false, nil).
					Times(1)

				mockClient.EXPECT().
					DeleteSecret("secret-2", "ns2").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("secret-2").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name: "mixed_secrets_with_and_without_deletion_timestamp",
			secrets: []v1.Secret{
				*createTestSecret("secret-with-deletion", "ns1", true),
				*createTestSecret("secret-without-deletion", "ns2", false),
			},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("secret-with-deletion").
					Return(false, nil).
					Times(1)

				mockClient.EXPECT().
					DeleteSecret("secret-without-deletion", "ns2").
					Return(nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("secret-without-deletion").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name: "first_secret_fails_stops_processing_others",
			secrets: []v1.Secret{
				*createTestSecret("failing-secret", "ns1", false),
				*createTestSecret("would-not-be-processed", "ns2", false),
			},
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				// First secret fails, so second secret is never processed
				mockClient.EXPECT().
					DeleteSecret("failing-secret", "ns1").
					Return(errors.New("api error")).
					Times(1)
			},
			expectedError: "api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateSecretTestMode(t, func() {
				mockClient := getMockK8sClientForSecrets(t)
				tt.setupMocks(mockClient)

				originalTimeout := k8sTimeout
				k8sTimeout = secretTestShortTimeout
				defer func() { k8sTimeout = originalTimeout }()

				err := deleteSecrets(tt.secrets)

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestWaitForSecretDeletion(t *testing.T) {
	tests := []struct {
		name          string
		secretName    string
		timeout       time.Duration
		setupMocks    func(*mockK8sClient.MockKubernetesClient)
		expectedError string
	}{
		{
			name:       "success_secret_deleted_immediately",
			secretName: "test-secret",
			timeout:    1 * time.Second,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name:       "success_secret_not_found_error_treated_as_deleted",
			secretName: "test-secret",
			timeout:    1 * time.Second,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(false, errors.NotFoundError("not found")).
					Times(1)
			},
		},
		{
			name:       "success_secret_deleted_after_several_retries",
			secretName: "test-secret",
			timeout:    2 * time.Second,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(true, nil).
					Times(1)

				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(false, nil).
					Times(1)
			},
		},
		{
			name:       "success_exists_true_but_notfound_error_still_times_out",
			secretName: "test-secret",
			timeout:    100 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("test-secret").
					Return(true, errors.NotFoundError("not found")).
					AnyTimes()
			},
			expectedError: "secret test-secret was not deleted after 0.10 seconds",
		},
		{
			name:       "timeout_waiting_for_deletion",
			secretName: "persistent-secret",
			timeout:    100 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("persistent-secret").
					Return(true, nil).
					AnyTimes()
			},
			expectedError: "secret persistent-secret was not deleted after 0.10 seconds",
		},
		{
			name:       "api_error_during_check_causes_timeout",
			secretName: "error-secret",
			timeout:    100 * time.Millisecond,
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					CheckSecretExists("error-secret").
					Return(true, errors.New("api server unavailable")).
					AnyTimes()
			},
			expectedError: "secret error-secret was not deleted after 0.10 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateSecretTestMode(t, func() {
				mockClient := getMockK8sClientForSecrets(t)
				tt.setupMocks(mockClient)

				err := waitForSecretDeletion(tt.secretName, tt.timeout)

				if tt.expectedError != "" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), tt.expectedError)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestObliviateSecretCmdProperties(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "command_has_correct_properties",
			testFunc: func(t *testing.T) {
				assert.Equal(t, "secret", obliviateSecretCmd.Use)
				assert.Contains(t, obliviateSecretCmd.Short, "Reset Trident's Secret state")
				assert.Contains(t, obliviateSecretCmd.Short, "deletes all Trident Secrets present in a cluster")
				assert.NotNil(t, obliviateSecretCmd.RunE)
				assert.NotNil(t, obliviateSecretCmd.PersistentPreRun)
			},
		},
		{
			name: "persistent_pre_run_does_not_panic",
			testFunc: func(t *testing.T) {
				cmd := &cobra.Command{}
				assert.NotPanics(t, func() {
					obliviateSecretCmd.PersistentPreRun(cmd, []string{})
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestObliviateSecretCmdRunE_TunnelMode(t *testing.T) {
	tests := []struct {
		name                  string
		forceObliviate        bool
		expectedErrorContains string
	}{
		{
			name:           "tunnel_mode_force_obliviate_true",
			forceObliviate: true,

			expectedErrorContains: "",
		},
		{
			name:           "tunnel_mode_force_obliviate_false",
			forceObliviate: false,

			expectedErrorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateSecretTestMode(t, func() {
				originalMode := OperatingMode
				originalForce := forceObliviate
				defer func() {
					OperatingMode = originalMode
					forceObliviate = originalForce
				}()

				OperatingMode = ModeTunnel
				forceObliviate = tt.forceObliviate

				cmd := &cobra.Command{}

				err := obliviateSecretCmd.RunE(cmd, []string{})

				_ = err
			})
		})
	}
}

func TestObliviateSecretCmdRunE_DirectMode(t *testing.T) {
	tests := []struct {
		name                  string
		setupMocks            func(*mockK8sClient.MockKubernetesClient)
		expectedErrorContains string
	}{
		{
			name: "direct_mode_with_mocked_obliviate_call",
			setupMocks: func(mockClient *mockK8sClient.MockKubernetesClient) {
				mockClient.EXPECT().
					GetSecretsByLabel(TridentCSILabel, true).
					Return([]v1.Secret{}, nil).
					AnyTimes()
			},
			expectedErrorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateSecretTestMode(t, func() {
				originalMode := OperatingMode
				defer func() { OperatingMode = originalMode }()

				OperatingMode = "direct"

				mockClient := getMockK8sClientForSecrets(t)
				tt.setupMocks(mockClient)

				cmd := &cobra.Command{}

				err := obliviateSecretCmd.RunE(cmd, []string{})

				_ = err
			})
		})
	}
}
