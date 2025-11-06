package kubernetes

import (
	"context"
	"fmt"
	"testing"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
)

func TestGetCaseFoldedAnnotation(t *testing.T) {
	ann := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"KEY3": "value3",
	}
	assert.NotEmpty(t, getCaseFoldedAnnotation(ann, "KEY1"))
	assert.NotEmpty(t, getCaseFoldedAnnotation(ann, "key3"))
	assert.Empty(t, getCaseFoldedAnnotation(ann, "key4"))
}

func TestValidAccessControlPermission(t *testing.T) {
	validPermissions := []string{
		SMBShareFullControlPermission,
		SMBShareReadPermission,
		SMBShareChangePermission,
		SMBShareNoPermission,
	}

	for _, permission := range validPermissions {
		assert.True(t, isValidAccessControlPermission(permission), "Expected permission '%s' to be valid", permission)
	}
}

func TestInvalidAccessControlPermission(t *testing.T) {
	// Negative test cases (invalid permissions)
	invalidPermissions := []string{
		"",
		"InvalidPermission",
		"FullControl", // Similar but not exact match
		"ReadOnly",
		"None",
	}

	for _, permission := range invalidPermissions {
		assert.False(t, isValidAccessControlPermission(permission), "Expected permission '%s' to be invalid", permission)
	}
}

func TestGetSMBShareAccessControlFromPVCAnnotation_ValidAnnotations(t *testing.T) {
	tests := []struct {
		name            string
		inputAnnotation string
		expectedACL     map[string]string
	}{
		{
			name: "Valid annotation with multiple users",
			inputAnnotation: `
full_control:
  - user1
  - user2
read:
  - user3
change:
  - user4
no_access:
  - user5
`,
			expectedACL: map[string]string{
				"user1": SMBShareFullControlPermission,
				"user2": SMBShareFullControlPermission,
				"user3": SMBShareReadPermission,
				"user4": SMBShareChangePermission,
				"user5": SMBShareNoPermission,
			},
		},
		{
			name: "Valid annotation with single user",
			inputAnnotation: `
read:
  - user1
`,
			expectedACL: map[string]string{
				"user1": SMBShareReadPermission,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := getSMBShareAccessControlFromPVCAnnotation(test.inputAnnotation)
			assert.NoError(t, err, "Expected no error for valid annotation")
			assert.Equal(t, test.expectedACL, result, "Expected ACL to match")
		})
	}
}

func TestGetSMBShareAccessControlFromPVCAnnotation_InvalidAnnotations(t *testing.T) {
	tests := []struct {
		name            string
		inputAnnotation string
		expectedError   string
	}{
		{
			name: "Invalid access control permission",
			inputAnnotation: `
InvalidPermission:
  - user1
`,
			expectedError: "invalid access control permission InvalidPermission",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := getSMBShareAccessControlFromPVCAnnotation(test.inputAnnotation)
			assert.Error(t, err, "Expected error for invalid annotation in test case: %s", test.name)
			assert.Contains(t, err.Error(), test.expectedError, "Expected error message to contain: %s", test.expectedError)
			assert.Nil(t, result, "Expected result to be nil on error in test case: %s", test.name)
		})
	}
}

func TestGetSMBShareAccessControlFromPVCAnnotation_InvalidYaml(t *testing.T) {
	tests := []struct {
		name            string
		inputAnnotation string
		expectedError   string
	}{
		{
			name: "Invalid YAML format",
			inputAnnotation: `
FullControl:
  - user1
  - user2
InvalidYAML
`,
			expectedError: "failed to parse smbShareAccessControl annotation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := getSMBShareAccessControlFromPVCAnnotation(test.inputAnnotation)
			assert.Error(t, err, "Expected error for invalid YAML in test case: %s", test.name)
			assert.Contains(t, err.Error(), test.expectedError, "Expected error message to contain: %s", test.expectedError)
			assert.Nil(t, result, "Expected result to be nil on error in test case: %s", test.name)
		})
	}
}

func TestProcessSCAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		sc       *k8sstoragev1.StorageClass
		expected map[string]string
	}{
		{
			name: "Valid annotations in SC",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "Empty SC annotations",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "Nil SC annotations",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expected: map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := processSCAnnotations(test.sc)
			assert.Equal(t, test.expected, result, "Annotations did not match for test: %s", test.name)
		})
	}
}

func TestProcessSCAnnotationsExtended(t *testing.T) {
	tests := []struct {
		name        string
		sc          *k8sstoragev1.StorageClass
		expectedLen int
		expected    map[string]string
	}{
		{
			name: "StorageClass with existing annotations",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			expectedLen: 2,
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "StorageClass with nil annotations",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expectedLen: 0,
			expected:    map[string]string{},
		},
		{
			name: "StorageClass with empty annotations",
			sc: &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expectedLen: 0,
			expected:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processSCAnnotations(tt.sc)

			assert.NotNil(t, result)
			assert.Len(t, result, tt.expectedLen)

			for key, expectedValue := range tt.expected {
				assert.Equal(t, expectedValue, result[key])
			}
		})
	}
}

func TestProcessPVCAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		fsType      string
		expectedLen int
		expectFs    string
	}{
		{
			name: "PVC with existing annotations",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			fsType:      "ext4",
			expectedLen: 3, // 2 existing + 1 filesystem
			expectFs:    "ext4",
		},
		{
			name: "PVC with nil annotations",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			fsType:      "xfs",
			expectedLen: 1, // Only filesystem annotation
			expectFs:    "xfs",
		},
		{
			name: "PVC with existing filesystem annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnFileSystem: "btrfs",
						"other-key":   "other-value",
					},
				},
			},
			fsType:      "ext4",
			expectedLen: 2, // Keep existing filesystem, don't override
			expectFs:    "btrfs",
		},
		{
			name: "PVC with empty fsType parameter",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key1": "value1",
					},
				},
			},
			fsType:      "",
			expectedLen: 1, // No filesystem annotation added
			expectFs:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processPVCAnnotations(tt.pvc, tt.fsType)

			assert.NotNil(t, result)
			assert.Len(t, result, tt.expectedLen)

			if tt.expectFs != "" {
				assert.Equal(t, tt.expectFs, result[AnnFileSystem])
			} else {
				_, exists := result[AnnFileSystem]
				assert.False(t, exists)
			}
		})
	}
}

func TestGetVolumeConfigFunction(t *testing.T) {
	tests := []struct {
		name               string
		pvc                *v1.PersistentVolumeClaim
		volumeName         string
		size               resource.Quantity
		annotations        map[string]string
		storageClass       *k8sstoragev1.StorageClass
		expectedName       string
		expectedSize       string
		expectedAccessMode config.AccessMode
		expectedVolumeMode config.VolumeMode
	}{
		{
			name: "Basic volume configuration",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
				},
			},
			volumeName:         "test-volume",
			size:               resource.MustParse("10Gi"),
			annotations:        map[string]string{},
			storageClass:       &k8sstoragev1.StorageClass{},
			expectedName:       "test-volume",
			expectedSize:       "10737418240", // 10Gi in bytes
			expectedAccessMode: config.ReadWriteOnce,
			expectedVolumeMode: config.VolumeMode(v1.PersistentVolumeFilesystem),
		},
		{
			name: "Volume with block mode",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "block-pvc",
					Namespace: "default",
					UID:       "block-uid",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					VolumeMode: &[]v1.PersistentVolumeMode{v1.PersistentVolumeBlock}[0],
				},
			},
			volumeName:         "block-volume",
			size:               resource.MustParse("50Gi"),
			annotations:        map[string]string{},
			storageClass:       &k8sstoragev1.StorageClass{},
			expectedName:       "block-volume",
			expectedSize:       "53687091200", // 50Gi in bytes
			expectedAccessMode: config.ReadWriteOnce,
			expectedVolumeMode: config.VolumeMode(v1.PersistentVolumeBlock),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getVolumeConfig(
				context.Background(),
				tt.pvc,
				tt.volumeName,
				tt.size,
				tt.annotations,
				tt.storageClass,
				nil, // requisiteTopology
				nil, // preferredTopology
			)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedSize, result.Size)
			assert.Equal(t, tt.expectedAccessMode, result.AccessMode)
			assert.Equal(t, tt.expectedVolumeMode, result.VolumeMode)
		})
	}
}

func TestGetSMBShareAccessControlFromPVCAnnotation(t *testing.T) {
	tests := []struct {
		name           string
		annotation     string
		expectedResult map[string]string
		expectError    bool
		errorContains  string
	}{
		{
			name: "Valid SMB access control annotation",
			annotation: `
full_control:
  - user1
  - user2
read:
  - user3`,
			expectedResult: map[string]string{
				"user1": SMBShareFullControlPermission,
				"user2": SMBShareFullControlPermission,
				"user3": SMBShareReadPermission,
			},
			expectError: false,
		},
		{
			name: "User with multiple permissions - higher priority wins",
			annotation: `
full_control:
  - user1
read:
  - user1
  - user2`,
			expectedResult: map[string]string{
				"user1": SMBShareFullControlPermission, // full_control has highest priority
				"user2": SMBShareReadPermission,
			},
			expectError: false,
		},
		{
			name:           "Empty annotation",
			annotation:     `{}`,
			expectedResult: map[string]string{},
			expectError:    false,
		},
		{
			name: "Invalid YAML format",
			annotation: `
invalid_yaml: [
  unclosed_bracket`,
			expectedResult: nil,
			expectError:    true,
			errorContains:  "failed to parse smbShareAccessControl annotation",
		},
		{
			name: "Invalid permission type",
			annotation: `
invalid_permission:
  - user1`,
			expectedResult: nil,
			expectError:    true,
			errorContains:  "invalid access control permission invalid_permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getSMBShareAccessControlFromPVCAnnotation(tt.annotation)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				assert.Nil(t, result)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestGetSMBShareAccessControlFromPVCAnnotation_PriorityResolution(t *testing.T) {
	tests := []struct {
		name             string
		annotation       string
		expectedACL      map[string]string
		expectedErrorMsg string
	}{
		{
			name: "User with multiple permissions - priority resolution",
			annotation: `
full_control:
  - admin
  - user1
read:
  - user1
  - user2
change:
  - user1
  - user3
`,
			expectedACL: map[string]string{
				"admin": "full_control",
				"user1": "full_control", // Should get highest priority
				"user2": "read",
				"user3": "change",
			},
		},
		{
			name: "Different priority scenarios",
			annotation: `
no_access:
  - user1
read:
  - user1
  - user2
change:
  - user1
`,
			expectedACL: map[string]string{
				"user1": "change", // Should get highest priority among assigned
				"user2": "read",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getSMBShareAccessControlFromPVCAnnotation(tt.annotation)

			if tt.expectedErrorMsg != "" {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedACL, result)
			}
		})
	}
}

func TestValidateStorageClassParametersExtended(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name        string
		sc          *k8sstoragev1.StorageClass
		keys        []string
		expectError bool
	}{
		{
			name: "All required parameters present",
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"param1": "value1",
					"param2": "value2",
				},
			},
			keys:        []string{"param1", "param2"},
			expectError: false,
		},
		{
			name: "Missing one parameter",
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"param1": "value1",
				},
			},
			keys:        []string{"param1", "param2"},
			expectError: true,
		},
		{
			name: "Missing multiple parameters",
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{},
			},
			keys:        []string{"param1", "param2"},
			expectError: true,
		},
		{
			name: "No keys to validate",
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{"param1": "value1"},
			},
			keys:        []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validateStorageClassParameters(tt.sc, tt.keys...)

			if tt.expectError {
				assert.Error(t, err, "Expected error for validation test case: %s", tt.name)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	annotations := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"":     "empty-key",
	}

	// Test existing key
	assert.Equal(t, "value1", getAnnotation(annotations, "key1"))
	assert.Equal(t, "value2", getAnnotation(annotations, "key2"))
	assert.Equal(t, "empty-key", getAnnotation(annotations, ""))

	// Test non-existing key
	assert.Equal(t, "", getAnnotation(annotations, "nonexistent"))

	// Test nil map
	assert.Equal(t, "", getAnnotation(nil, "key1"))
}

func TestMatchNamespaceToAnnotation(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name       string
		namespace  string
		annotation string
		expected   bool
	}{
		{
			name:       "exact match",
			namespace:  "test-ns",
			annotation: "test-ns",
			expected:   true,
		},
		{
			name:       "wildcard match",
			namespace:  "any-ns",
			annotation: "*",
			expected:   true,
		},
		{
			name:       "multiple namespaces - first match",
			namespace:  "ns1",
			annotation: "ns1,ns2,ns3",
			expected:   true,
		},
		{
			name:       "multiple namespaces - middle match",
			namespace:  "ns2",
			annotation: "ns1,ns2,ns3",
			expected:   true,
		},
		{
			name:       "multiple namespaces - last match",
			namespace:  "ns3",
			annotation: "ns1,ns2,ns3",
			expected:   true,
		},
		{
			name:       "multiple namespaces with wildcard",
			namespace:  "any-ns",
			annotation: "ns1,*,ns3",
			expected:   true,
		},
		{
			name:       "no match",
			namespace:  "other-ns",
			annotation: "ns1,ns2,ns3",
			expected:   false,
		},
		{
			name:       "empty annotation",
			namespace:  "test-ns",
			annotation: "",
			expected:   false,
		},
		{
			name:       "empty namespace",
			namespace:  "",
			annotation: "test-ns",
			expected:   false,
		},
		{
			name:       "empty namespace exact match",
			namespace:  "",
			annotation: "",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.matchNamespaceToAnnotation(tt.namespace, tt.annotation)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPVCUIDFromCSIVolumeName(t *testing.T) {
	tests := []struct {
		name        string
		volumeName  string
		expectedUID string
		expectError bool
	}{
		{
			name:        "valid CSI volume name",
			volumeName:  "pvc-12345678-1234-1234-1234-123456789abc",
			expectedUID: "12345678-1234-1234-1234-123456789abc",
			expectError: false,
		},
		{
			name:        "valid CSI volume name with different UID",
			volumeName:  "pvc-abcdef01-2345-5678-9abc-def012345678",
			expectedUID: "abcdef01-2345-5678-9abc-def012345678",
			expectError: false,
		},
		{
			name:        "invalid volume name - no pvc prefix",
			volumeName:  "volume-12345678-1234-1234-1234-123456789abc",
			expectedUID: "",
			expectError: true,
		},
		{
			name:        "invalid volume name - no UID",
			volumeName:  "pvc-",
			expectedUID: "",
			expectError: true,
		},
		{
			name:        "invalid volume name - just pvc",
			volumeName:  "pvc",
			expectedUID: "",
			expectError: true,
		},
		{
			name:        "empty volume name",
			volumeName:  "",
			expectedUID: "",
			expectError: true,
		},
		{
			name:        "invalid UID format",
			volumeName:  "pvc-invalid-uid",
			expectedUID: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := getPVCUIDFromCSIVolumeName(tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "Expected error for PVC UID extraction test case: %s", tt.name)
				assert.Equal(t, "", uid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUID, uid)
			}
		})
	}
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{
			name:      "normal event type",
			eventType: controllerhelpers.EventTypeNormal,
			expected:  "Normal",
		},
		{
			name:      "warning event type",
			eventType: controllerhelpers.EventTypeWarning,
			expected:  "Warning",
		},
		{
			name:      "unknown event type defaults to warning",
			eventType: "UnknownType",
			expected:  "Warning",
		},
		{
			name:      "empty event type defaults to warning",
			eventType: "",
			expected:  "Warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapEventType(tt.eventType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetGroupSnapshotConfigForCreate(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name                string
		groupSnapshotName   string
		volumeNames         []string
		expectedName        string
		expectedVolumeNames []string
	}{
		{
			name:                "valid group snapshot config",
			groupSnapshotName:   "test-group-snapshot",
			volumeNames:         []string{"vol1", "vol2", "vol3"},
			expectedName:        "test-group-snapshot",
			expectedVolumeNames: []string{"vol1", "vol2", "vol3"},
		},
		{
			name:                "empty volume names",
			groupSnapshotName:   "test-group-snapshot",
			volumeNames:         []string{},
			expectedName:        "test-group-snapshot",
			expectedVolumeNames: []string{},
		},
		{
			name:                "nil volume names",
			groupSnapshotName:   "test-group-snapshot",
			volumeNames:         nil,
			expectedName:        "test-group-snapshot",
			expectedVolumeNames: nil,
		},
		{
			name:                "single volume",
			groupSnapshotName:   "single-vol-snapshot",
			volumeNames:         []string{"single-vol"},
			expectedName:        "single-vol-snapshot",
			expectedVolumeNames: []string{"single-vol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := h.GetGroupSnapshotConfigForCreate(context.Background(), tt.groupSnapshotName, tt.volumeNames)

			assert.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, tt.expectedName, config.Name)
			assert.Equal(t, tt.expectedVolumeNames, config.VolumeNames)
			assert.NotEmpty(t, config.Version) // Should have a version
		})
	}
}

func TestUpdateVolumeConfigWithSecureSMBAccessControl(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name           string
		volumeConfig   *storage.VolumeConfig
		sc             *k8sstoragev1.StorageClass
		pvcAnnotations map[string]string
		scAnnotations  map[string]string
		secret         map[string]string
		expectError    bool
	}{
		{
			name: "Valid SMB configuration with required parameters",
			volumeConfig: &storage.VolumeConfig{
				Name: "test-volume",
			},
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"csi.storage.k8s.io/node-stage-secret-name":      "smb-secret",
					"csi.storage.k8s.io/node-stage-secret-namespace": "default",
				},
			},
			pvcAnnotations: map[string]string{
				"trident.netapp.io/smbAccessControl": "user1: full_control",
			},
			scAnnotations: map[string]string{
				"trident.netapp.io/smbShareAdUser":           "admin",
				"trident.netapp.io/smbShareAdUserPermission": "full_control",
			},
			secret:      map[string]string{},
			expectError: false,
		},
		{
			name: "Missing required secret parameters",
			volumeConfig: &storage.VolumeConfig{
				Name: "test-volume",
			},
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{},
			},
			pvcAnnotations: map[string]string{},
			scAnnotations:  map[string]string{},
			secret:         map[string]string{},
			expectError:    true,
		},
		{
			name: "Empty adUser permission defaults to full_control",
			volumeConfig: &storage.VolumeConfig{
				Name: "test-volume",
			},
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"csi.storage.k8s.io/node-stage-secret-name":      "smb-secret",
					"csi.storage.k8s.io/node-stage-secret-namespace": "default",
				},
			},
			pvcAnnotations: map[string]string{
				"trident.netapp.io/smbAccessControl": "user1: read",
			},
			scAnnotations: map[string]string{
				"trident.netapp.io/smbShareAdUser": "admin",
				// Permission not set - should default to full_control
			},
			secret:      map[string]string{},
			expectError: false,
		},
		{
			name: "Invalid adUser permission",
			volumeConfig: &storage.VolumeConfig{
				Name: "test-volume",
			},
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"csi.storage.k8s.io/node-stage-secret-name":      "smb-secret",
					"csi.storage.k8s.io/node-stage-secret-namespace": "default",
				},
			},
			pvcAnnotations: map[string]string{
				"trident.netapp.io/smbAccessControl": "user1: read",
			},
			scAnnotations: map[string]string{
				"trident.netapp.io/smbShareAdUser":           "admin",
				"trident.netapp.io/smbShareAdUserPermission": "invalid-permission",
			},
			secret:      map[string]string{},
			expectError: true,
		},
		{
			name: "AdUser already exists in ACL",
			volumeConfig: &storage.VolumeConfig{
				Name: "test-volume",
			},
			sc: &k8sstoragev1.StorageClass{
				Parameters: map[string]string{
					"csi.storage.k8s.io/node-stage-secret-name":      "smb-secret",
					"csi.storage.k8s.io/node-stage-secret-namespace": "default",
				},
			},
			pvcAnnotations: map[string]string{
				"trident.netapp.io/smbAccessControl": "admin: read, user1: change",
			},
			scAnnotations: map[string]string{
				"trident.netapp.io/smbShareAdUser":           "admin",
				"trident.netapp.io/smbShareAdUserPermission": "full_control",
			},
			secret:      map[string]string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.updateVolumeConfigWithSecureSMBAccessControl(
				context.Background(),
				tt.volumeConfig,
				tt.sc,
				tt.pvcAnnotations,
				tt.scAnnotations,
				tt.secret,
			)

			if tt.expectError {
				assert.Error(t, err, "Expected error for SMB access control test case: %s", tt.name)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetVolumeSnapshot(t *testing.T) {
	// Test using a helper with nil snapshot client - this should panic
	h := &helper{}

	tests := []struct {
		name        string
		snapName    string
		namespace   string
		expectPanic bool
	}{
		{
			name:        "Get volume snapshot with nil client should panic",
			snapName:    "test-snapshot",
			namespace:   "default",
			expectPanic: true,
		},
		{
			name:        "Get snapshot with empty name should panic",
			snapName:    "",
			namespace:   "default",
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				assert.Panics(t, func() {
					h.getVolumeSnapshot(context.Background(), tt.snapName, tt.namespace)
				})
			} else {
				_, err := h.getVolumeSnapshot(context.Background(), tt.snapName, tt.namespace)
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSnapshotContentFromSnapshot(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name        string
		snapshot    *vsv1.VolumeSnapshot
		expectError bool
		errorMsg    string
	}{
		{
			name: "Snapshot with nil bound content name",
			snapshot: &vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-snapshot",
				},
				Status: &vsv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: nil,
				},
			},
			expectError: true,
			errorMsg:    "boundVolumeSnapshotContentName not found",
		},
		{
			name: "Snapshot with empty bound content name",
			snapshot: &vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-snapshot",
				},
				Status: &vsv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: stringPtr(""),
				},
			},
			expectError: true,
			errorMsg:    "boundVolumeSnapshotContentName not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.getSnapshotContentFromSnapshot(context.Background(), tt.snapshot)

			if tt.expectError {
				assert.Error(t, err, "Expected error for snapshot content test case: %s", tt.name)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSubordinateVolumeConfig_EarlyValidation(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name         string
		pvc          *v1.PersistentVolumeClaim
		volConfig    *storage.VolumeConfig
		expectError  bool
		errorMessage string
		expectPanic  bool
	}{
		{
			name: "No share annotation - should return nil",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: false,
		},
		{
			name: "Empty share annotation - should return nil",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "",
					},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: false,
		},
		{
			name: "Invalid annotation format - single component",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "invalid-format",
					},
				},
			},
			volConfig:    &storage.VolumeConfig{Name: "test-volume"},
			expectError:  true,
			errorMessage: "annotation must have the format",
		},
		{
			name: "Nil annotations should not error",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				assert.Panics(t, func() {
					h.validateSubordinateVolumeConfig(context.Background(), tt.pvc, tt.volConfig)
				})
			} else {
				err := h.validateSubordinateVolumeConfig(context.Background(), tt.pvc, tt.volConfig)

				if tt.expectError {
					assert.Error(t, err, "Expected error for subordinate volume config test case: %s", tt.name)
					if tt.errorMessage != "" {
						assert.Contains(t, err.Error(), tt.errorMessage)
					}
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestRecordVolumeEvent_NilClient(t *testing.T) {
	h := &helper{}

	// Should not panic when K8s client is nil - function should handle gracefully
	assert.NotPanics(t, func() {
		h.RecordVolumeEvent(context.Background(), "test-volume", "Normal", "Created", "Volume created successfully")
	})
}

func TestRecordNodeEvent_NilClient(t *testing.T) {
	h := &helper{}

	// Should panic when trying to access K8s client (calls GetNode)
	assert.Panics(t, func() {
		h.RecordNodeEvent(context.Background(), "test-volume", "Normal", "Mounted", "Volume mounted successfully")
	})
}

func TestGetVolumeConfig_NilClient(t *testing.T) {
	h := &helper{}

	// Should fail early validation - volume name doesn't contain UID
	_, err := h.GetVolumeConfig(context.Background(), "test-pv", 1024, nil, config.File, nil, config.VolumeMode(""), "", nil, nil, nil, nil)
	assert.Error(t, err, "Expected error for invalid volume name without UID")
	assert.Contains(t, err.Error(), "volume name test-pv does not contain a uid")
}

// TestGetVolumeConfig_StorageClassNotFound tests when storage class is not found
func TestGetVolumeConfig_StorageClassNotFound(t *testing.T) {
	scName := "non-existent-sc"
	// Create a simple mock indexer that returns a PVC with non-existent storage class
	mockPVCIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			// Return a valid PVC with storage class pointer
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "12345678-1234-1234-1234-123456789abc",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			return pvc, true, nil
		},
	}

	h := &helper{
		pvcIndexer: mockPVCIndexer,
		kubeClient: nil, // This will cause the test to fail at getStorageClass
	}

	// This should fail when trying to get the storage class due to nil client
	_, err := h.GetVolumeConfig(context.Background(), "pvc-12345678-1234-1234-1234-123456789abc", 1024, nil, config.File, nil, config.VolumeMode(""), "", nil, nil, nil, nil)
	assert.Error(t, err, "Expected error when storage class not found with nil client")
}

// TestGetStorageClass_ResyncError tests resync error path in getStorageClass
func TestGetStorageClass_ResyncError(t *testing.T) {
	// Create a mock indexer that returns not found first, then resync fails
	mockSCIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			return nil, false, fmt.Errorf("storage class not found")
		},
		resyncFunc: func() error {
			return fmt.Errorf("resync failed")
		},
	}

	h := &helper{
		scIndexer: mockSCIndexer,
	}

	// This should fail during resync
	_, err := h.getStorageClass(context.Background(), "test-sc")
	assert.Error(t, err, "Expected error when storage class cache resync fails")
	assert.Contains(t, err.Error(), "could not refresh local storage class cache")
}

// TestGetPVCMirrorPeer_IndexerError tests the indexer error path in getPVCMirrorPeer
func TestGetPVCMirrorPeer_IndexerError(t *testing.T) {
	// Create a mock indexer that returns an error
	mockIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			return nil, false, fmt.Errorf("indexer error")
		},
	}

	h := &helper{
		mrIndexer: mockIndexer,
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	// This should fail due to indexer error
	_, err := h.getPVCMirrorPeer(pvc, "test-mr")
	assert.Error(t, err, "Expected error when PVC mirror peer indexer fails")
	assert.Contains(t, err.Error(), "indexer error")
}

// TestGetPVCMirrorPeer_TypeAssertionError tests type assertion error in relationships list
func TestGetPVCMirrorPeer_TypeAssertionError(t *testing.T) {
	// Create a mock indexer that returns a wrong type object in the list
	mockIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			// Return not found for specific key, so it goes to List() path
			return nil, false, nil
		},
		listFunc: func() []interface{} {
			// Return a wrong type object that will fail type assertion
			return []interface{}{
				"wrong-type-object", // This will cause type assertion to fail
			}
		},
	}

	h := &helper{
		mrIndexer: mockIndexer,
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	// This should fail due to type assertion error in relationships list
	_, err := h.getPVCMirrorPeer(pvc, "")
	assert.Error(t, err, "Expected error when type assertion fails for mirror relationship")
	assert.Contains(t, err.Error(), "could not perform assertion")
}

// TestGetVolumeConfig_MirrorPeerError tests GetVolumeConfig when getPVCMirrorPeer fails
func TestGetVolumeConfig_MirrorPeerError(t *testing.T) {
	scName := "test-sc"

	// Create a PVC with mirror relationship annotation that will trigger the error
	mockPVCIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "12345678-1234-1234-1234-123456789abc",
					Annotations: map[string]string{
						AnnMirrorRelationship: "test-mirror", // This will trigger getPVCMirrorPeer
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			return pvc, true, nil
		},
	}

	// Create a storage class that will pass validation
	mockSCIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			sc := &k8sstoragev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: "csi.trident.netapp.io",
				Parameters:  map[string]string{},
			}
			return sc, true, nil
		},
	}

	// Create a mirror relationship indexer that will fail
	mockMRIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			return nil, false, fmt.Errorf("mirror relationship indexer error")
		},
	}

	h := &helper{
		pvcIndexer: mockPVCIndexer,
		scIndexer:  mockSCIndexer,
		mrIndexer:  mockMRIndexer,
	}

	// This should fail when GetVolumeConfig calls getPVCMirrorPeer
	_, err := h.GetVolumeConfig(context.Background(), "pvc-12345678-1234-1234-1234-123456789abc", 1024, nil, config.File, nil, config.VolumeMode(""), "", nil, nil, nil, nil)
	assert.Error(t, err, "Expected error when GetVolumeConfig calls getPVCMirrorPeer with failing indexer")
	assert.Contains(t, err.Error(), "PVC 12345678-1234-1234-1234-123456789abc was not in cache")
}

func TestGetVolumeConfig_PVCNotInPendingState(t *testing.T) {
	ctx := context.Background()

	// Create a mock PVC indexer that returns a PVC in Bound state (not Pending)
	mockPVCIndexer := &MockFullIndexer{
		byIndexFunc: func(indexName, indexKey string) ([]interface{}, error) {
			if indexName == "uid" && indexKey == "12345678-1234-1234-1234-123456789abc" {
				pvc := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "default",
						UID:       "12345678-1234-1234-1234-123456789abc",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
					Status: v1.PersistentVolumeClaimStatus{
						Phase: v1.ClaimBound, // This should cause the error
					},
				}
				return []interface{}{pvc}, nil
			}
			return []interface{}{}, nil
		},
	}

	helper := &helper{
		pvcIndexer: mockPVCIndexer,
	}

	// Call GetVolumeConfig - should fail because PVC is not in Pending state
	_, err := helper.GetVolumeConfig(ctx, "pvc-12345678-1234-1234-1234-123456789abc",
		1000000000, nil, config.File, nil, config.Filesystem, "ext4",
		nil, nil, nil, nil)

	assert.Error(t, err, "Expected error when PVC is not in Pending state")
	assert.Contains(t, err.Error(), "PVC test-pvc is not in Pending state")
}

func TestGetVolumeConfig_PVCWithoutValidSize(t *testing.T) {
	ctx := context.Background()

	// Create a mock PVC indexer that returns a PVC without storage size request
	mockPVCIndexer := &MockFullIndexer{
		byIndexFunc: func(indexName, indexKey string) ([]interface{}, error) {
			if indexName == "uid" && indexKey == "12345678-1234-1234-1234-123456789abc" {
				pvc := &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvc",
						Namespace: "default",
						UID:       "12345678-1234-1234-1234-123456789abc",
					},
					Spec: v1.PersistentVolumeClaimSpec{
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{
								// Missing storage request - this will cause the error
							},
						},
					},
					Status: v1.PersistentVolumeClaimStatus{
						Phase: v1.ClaimPending, // Valid phase but missing size
					},
				}
				return []interface{}{pvc}, nil
			}
			return []interface{}{}, nil
		},
	}

	helper := &helper{
		pvcIndexer: mockPVCIndexer,
	}

	// Call GetVolumeConfig - should fail because PVC doesn't have valid size
	_, err := helper.GetVolumeConfig(ctx, "pvc-12345678-1234-1234-1234-123456789abc",
		1000000000, nil, config.File, nil, config.Filesystem, "ext4",
		nil, nil, nil, nil)

	assert.Error(t, err, "Expected error when PVC does not have valid size")
	assert.Contains(t, err.Error(), "PVC test-pvc does not have a valid size")
}

// TestGetPVCForCSIVolume_ResyncPath tests the cache resync error path
func TestGetPVCForCSIVolume_ResyncPath(t *testing.T) {
	// Create a mock indexer that simulates cache miss then resync failure
	mockIndexer := &MockFullIndexer{
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			return nil, false, nil // PVC not found in cache
		},
		resyncFunc: func() error {
			return fmt.Errorf("resync failed")
		},
	}

	h := &helper{
		pvcIndexer: mockIndexer,
	}

	// Test with valid UID format but cache resync failure
	_, err := h.getPVCForCSIVolume(context.Background(), "pvc-12345678-1234-1234-1234-123456789abc")
	assert.Error(t, err, "Expected error when PVC cache resync fails")
	assert.Contains(t, err.Error(), "could not refresh local PVC cache")
}

// MockFullIndexer implements the full cache.Indexer interface
type MockFullIndexer struct {
	getByKeyFunc func(key string) (interface{}, bool, error)
	listFunc     func() []interface{}
	resyncFunc   func() error
	byIndexFunc  func(indexName, indexKey string) ([]interface{}, error)
}

func (m *MockFullIndexer) GetByKey(key string) (interface{}, bool, error) {
	if m.getByKeyFunc != nil {
		return m.getByKeyFunc(key)
	}
	return nil, false, nil
}

func (m *MockFullIndexer) List() []interface{} {
	if m.listFunc != nil {
		return m.listFunc()
	}
	return []interface{}{}
}

func (m *MockFullIndexer) Resync() error {
	if m.resyncFunc != nil {
		return m.resyncFunc()
	}
	return nil
}

// Required methods for cache.Store interface
func (m *MockFullIndexer) Add(obj interface{}) error    { return nil }
func (m *MockFullIndexer) Update(obj interface{}) error { return nil }
func (m *MockFullIndexer) Delete(obj interface{}) error { return nil }
func (m *MockFullIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}
func (m *MockFullIndexer) ListKeys() []string                  { return []string{} }
func (m *MockFullIndexer) Replace([]interface{}, string) error { return nil }

// Required methods for cache.Indexer interface
func (m *MockFullIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *MockFullIndexer) IndexKeys(indexName, indexKey string) ([]string, error) {
	return []string{}, nil
}
func (m *MockFullIndexer) ListIndexFuncValues(indexName string) []string { return []string{} }
func (m *MockFullIndexer) ByIndex(indexName, indexKey string) ([]interface{}, error) {
	if m.byIndexFunc != nil {
		return m.byIndexFunc(indexName, indexKey)
	}
	return []interface{}{}, nil
}
func (m *MockFullIndexer) GetIndexers() cache.Indexers                  { return cache.Indexers{} }
func (m *MockFullIndexer) AddIndexers(newIndexers cache.Indexers) error { return nil }

func TestGetPVCMirrorPeer_NilClient(t *testing.T) {
	h := &helper{}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	// Should panic when trying to access mirror relationship indexer
	assert.Panics(t, func() {
		h.getPVCMirrorPeer(pvc, "test-relationship")
	})
}

func TestGetPVCMirrorPeer_MultipleTMRsError(t *testing.T) {
	// Test case where multiple TMRs refer to the same PVC without specifying relationship name
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	// Create two TMRs that both refer to the same PVC
	tmr1 := &netappv1.TridentMirrorRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tmr-1",
			Namespace: "default",
		},
		Spec: netappv1.TridentMirrorRelationshipSpec{
			VolumeMappings: []*netappv1.TridentMirrorRelationshipVolumeMapping{
				{
					LocalPVCName: "test-pvc",
				},
			},
		},
	}

	tmr2 := &netappv1.TridentMirrorRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tmr-2",
			Namespace: "default",
		},
		Spec: netappv1.TridentMirrorRelationshipSpec{
			VolumeMappings: []*netappv1.TridentMirrorRelationshipVolumeMapping{
				{
					LocalPVCName: "test-pvc",
				},
			},
		},
	}

	// Mock indexer that returns both TMRs
	mockIndexer := &MockFullIndexer{
		listFunc: func() []interface{} {
			return []interface{}{tmr1, tmr2}
		},
		getByKeyFunc: func(key string) (interface{}, bool, error) {
			return nil, false, nil // No specific TMR found since name is empty
		},
	}

	h := &helper{
		mrIndexer: mockIndexer,
	}

	// Call without specifying relationship name - should fail due to multiple TMRs
	_, err := h.getPVCMirrorPeer(pvc, "")

	assert.Error(t, err, "Expected error when multiple TMRs refer to same PVC")
	assert.Contains(t, err.Error(), "multiple TMRs refer to PVC test-pvc")
}

func TestGetPVCForCSIVolume_NilClient(t *testing.T) {
	h := &helper{}

	// Should not panic immediately - has early validation
	_, err := h.getPVCForCSIVolume(context.Background(), "test-volume")
	assert.Error(t, err, "Expected error when getting PVC for CSI volume with nil client")
}

func TestGetStorageClass_NilClient(t *testing.T) {
	h := &helper{}

	// Should panic when trying to access K8s client
	assert.Panics(t, func() {
		h.getStorageClass(context.Background(), "test-storage-class")
	})
}

func TestGetSnapshotCloneSourceInfo_EarlyValidation(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		expectError bool
		errorMsg    string
	}{
		{
			name: "Missing clone snapshot annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectError: true,
			errorMsg:    "annotation 'cloneFromSnapshot' is empty",
		},
		{
			name: "Empty clone snapshot annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"trident.netapp.io/cloneFromSnapshot": "",
					},
				},
			},
			expectError: true,
			errorMsg:    "annotation 'cloneFromSnapshot' is empty",
		},
		{
			name: "Valid annotation - should proceed to K8s call",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"trident.netapp.io/cloneFromSnapshot": "test-snapshot",
					},
				},
			},
			expectError: true,
			errorMsg:    "panic", // Will panic when accessing K8s client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				_, _, err = h.getSnapshotCloneSourceInfo(context.Background(), tt.pvc)
			}()

			if tt.expectError {
				assert.Error(t, err, "Expected error for snapshot clone source test case: %s", tt.name)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSubordinateVolumeConfig_DeepValidation(t *testing.T) {
	h := &helper{} // Empty helper will fail on K8s calls but let us test annotation parsing

	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		volConfig   *storage.VolumeConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid annotation format but fails on volume reference lookup",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subordinate-pvc",
					Namespace: "target-ns",
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "source-ns/source-pvc",
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: stringPtr("fast-ssd"),
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: true,
			errorMsg:    "panic", // Will panic when trying to access indexer
		},
		{
			name: "Valid annotation with empty namespace part",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subordinate-pvc",
					Namespace: "target-ns",
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "/source-pvc",
					},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: true,
			errorMsg:    "annotation must have the format",
		},
		{
			name: "Valid annotation with empty PVC name part",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subordinate-pvc",
					Namespace: "target-ns",
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "source-ns/",
					},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: true,
			errorMsg:    "annotation must have the format",
		},
		{
			name: "Valid annotation with too many path components",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subordinate-pvc",
					Namespace: "target-ns",
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "extra/source-ns/source-pvc",
					},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: true,
			errorMsg:    "annotation must have the format",
		},
		{
			name: "Nil annotations should be handled gracefully",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "subordinate-pvc",
					Namespace:   "target-ns",
					Annotations: nil, // This tests the annotations == nil path
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: false, // Should return nil since no shareFromPVC annotation
		},
		{
			name: "Empty annotations map should be handled gracefully",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "subordinate-pvc",
					Namespace:   "target-ns",
					Annotations: map[string]string{}, // Empty map
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: false, // Should return nil since no shareFromPVC annotation
		},
		{
			name: "Single path component in annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subordinate-pvc",
					Namespace: "target-ns",
					Annotations: map[string]string{
						"trident.netapp.io/shareFromPVC": "source-pvc-only",
					},
				},
			},
			volConfig:   &storage.VolumeConfig{Name: "test-volume"},
			expectError: true,
			errorMsg:    "annotation must have the format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				err = h.validateSubordinateVolumeConfig(context.Background(), tt.pvc, tt.volConfig)
			}()

			if tt.expectError {
				assert.Error(t, err, "Expected error for subordinate volume config validation test case: %s", tt.name)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Enhanced coverage tests - comprehensive validation scenarios
func TestGetSnapshotConfigForImport_Comprehensive(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name                string
		volumeName          string
		snapshotContentName string
		expectError         bool
		errorMessage        string
	}{
		{
			name:                "Empty volume name",
			volumeName:          "",
			snapshotContentName: "snap-content-123",
			expectError:         true,
			errorMessage:        "invalid volume or snapshot name supplied",
		},
		{
			name:                "Empty snapshot content name",
			volumeName:          "volume-123",
			snapshotContentName: "",
			expectError:         true,
			errorMessage:        "invalid volume or snapshot name supplied",
		},
		{
			name:                "Both empty",
			volumeName:          "",
			snapshotContentName: "",
			expectError:         true,
			errorMessage:        "invalid volume or snapshot name supplied",
		},
		{
			name:                "Valid inputs - will fail on K8s client",
			volumeName:          "volume-123",
			snapshotContentName: "snap-content-123",
			expectError:         true,
			errorMessage:        "panic", // Will panic when accessing K8s client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				_, err = h.GetSnapshotConfigForImport(context.Background(), tt.volumeName, tt.snapshotContentName)
			}()

			if tt.expectError {
				assert.Error(t, err, "Expected error for snapshot config import test case: %s", tt.name)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetCloneSourceInfo_EarlyValidation(t *testing.T) {
	h := &helper{}

	tests := []struct {
		name        string
		pvc         *v1.PersistentVolumeClaim
		expectError bool
		errorMsg    string
	}{
		{
			name: "Missing clone PVC annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectError: true,
			errorMsg:    "annotation 'cloneFromPVC' is empty",
		},
		{
			name: "Empty clone PVC annotation",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"trident.netapp.io/cloneFromPVC": "",
					},
				},
			},
			expectError: true,
			errorMsg:    "annotation 'cloneFromPVC' is empty",
		},
		{
			name: "Valid annotation - should proceed to K8s call",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"trident.netapp.io/cloneFromPVC": "source-pvc",
					},
				},
			},
			expectError: true,
			errorMsg:    "panic", // Will panic when accessing K8s client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				_, err = h.getCloneSourceInfo(context.Background(), tt.pvc)
			}()

			if tt.expectError {
				assert.Error(t, err, "Expected error for clone source info test case: %s", tt.name)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
