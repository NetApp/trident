// Copyright 2025 NetApp, Inc. All Rights Reserved.

package resourcemonitor

import (
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldManageStorageClass(t *testing.T) {
	handler := &StorageClassHandler{
		driverHandler: NewFsxStorageDriverHandler(),
	}

	tests := []struct {
		name     string
		sc       *storagev1.StorageClass
		expected bool
	}{
		{
			name: "valid FSxN StorageClass",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: TridentCSIProvisioner,
				Parameters: map[string]string{
					FSxFilesystemIDParam:   "fs-123456789",
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expected: true,
		},
		{
			name: "missing fsxFilesystemID",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: TridentCSIProvisioner,
				Parameters: map[string]string{
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expected: false,
		},
		{
			name: "missing storageDriverName",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: TridentCSIProvisioner,
				Parameters: map[string]string{
					FSxFilesystemIDParam: "fs-123456789",
					CredentialsNameParam: "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expected: false,
		},
		{
			name: "missing credentialsName",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: TridentCSIProvisioner,
				Parameters: map[string]string{
					FSxFilesystemIDParam:   "fs-123456789",
					StorageDriverNameParam: "ontap-nas",
				},
			},
			expected: false,
		},
		{
			name: "wrong provisioner",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters: map[string]string{
					FSxFilesystemIDParam:   "fs-123456789",
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expected: false,
		},
		{
			name: "nil parameters",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: TridentCSIProvisioner,
				Parameters:  nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.shouldManageStorageClass(tt.sc)
			if result != tt.expected {
				t.Errorf("shouldManageStorageClass() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasRelevantChanges(t *testing.T) {
	handler := &StorageClassHandler{
		driverHandler: NewFsxStorageDriverHandler(),
	}

	tests := []struct {
		name     string
		oldSC    *storagev1.StorageClass
		newSC    *storagev1.StorageClass
		expected bool
	}{
		{
			name: "no changes",
			oldSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"key1": "value1",
				},
			},
			newSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"key1": "value1",
				},
			},
			expected: false,
		},
		{
			name: "parameter changed",
			oldSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"useREST":   "false",
					"aggregate": "aggr1",
				},
			},
			newSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"useREST":   "true",
					"aggregate": "aggr1",
				},
			},
			expected: false,
		},
		{
			name: "additionalFsxNFileSystemID annotation changed",
			oldSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						AdditionalFsxNFileSystemIDAnnotation: `["fs-111"]`,
					},
				},
				Parameters: map[string]string{
					"key1": "value1",
				},
			},
			newSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						AdditionalFsxNFileSystemIDAnnotation: `["fs-111", "fs-222"]`,
					},
				},
				Parameters: map[string]string{
					"key1": "value1",
				},
			},
			expected: true,
		},
		{
			name: "unrelated annotation changed",
			oldSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						"unrelated": "old-value",
					},
				},
				Parameters: map[string]string{
					"useREST": "true",
				},
			},
			newSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						"unrelated": "new-value",
					},
				},
				Parameters: map[string]string{
					"useREST": "true",
				},
			},
			expected: false,
		},
		{
			name: "unrelated parameter changed",
			oldSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"useREST":      "true",
					"unrelatedKey": "value1",
				},
			},
			newSC: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					"useREST":      "true",
					"unrelatedKey": "value2",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.hasRelevantChanges(tt.oldSC, tt.newSC)
			if result != tt.expected {
				t.Errorf("hasRelevantChanges() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTridentConfiguratorName(t *testing.T) {
	handler := &StorageClassHandler{
		driverHandler: NewFsxStorageDriverHandler(),
	}

	tests := []struct {
		name     string
		sc       *storagev1.StorageClass
		expected string
	}{
		{
			name: "with annotation",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						TridentConfiguratorNameAnnotation: "custom-tconf-name",
					},
				},
			},
			expected: "custom-tconf-name",
		},
		{
			name: "without annotation",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
			},
			expected: "tconf-test-sc",
		},
		{
			name: "empty annotation",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
					Annotations: map[string]string{
						TridentConfiguratorNameAnnotation: "",
					},
				},
			},
			expected: "tconf-test-sc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.getTridentConfiguratorName(tt.sc)
			if result != tt.expected {
				t.Errorf("getTridentConfiguratorName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateStorageClass(t *testing.T) {
	handler := &StorageClassHandler{
		driverHandler: NewFsxStorageDriverHandler(),
	}

	tests := []struct {
		name      string
		sc        *storagev1.StorageClass
		expectErr bool
	}{
		{
			name: "valid StorageClass",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					FSxFilesystemIDParam:   "fs-123456789",
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expectErr: false,
		},
		{
			name: "nil parameters",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: nil,
			},
			expectErr: true,
		},
		{
			name: "missing fsxFilesystemID",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expectErr: true,
		},
		{
			name: "empty fsxFilesystemID",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Parameters: map[string]string{
					FSxFilesystemIDParam:   "",
					StorageDriverNameParam: "ontap-nas",
					CredentialsNameParam:   "arn:aws:secretsmanager:us-west-2:123:secret:test",
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateStorageClass(tt.sc)
			if (err != nil) != tt.expectErr {
				t.Errorf("validateStorageClass() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]string
		b        map[string]string
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        map[string]string{},
			b:        map[string]string{},
			expected: true,
		},
		{
			name: "equal maps",
			a: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			b: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: true,
		},
		{
			name: "different values",
			a: map[string]string{
				"key1": "value1",
			},
			b: map[string]string{
				"key1": "value2",
			},
			expected: false,
		},
		{
			name: "different keys",
			a: map[string]string{
				"key1": "value1",
			},
			b: map[string]string{
				"key2": "value1",
			},
			expected: false,
		},
		{
			name: "different lengths",
			a: map[string]string{
				"key1": "value1",
			},
			b: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("mapsEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}
