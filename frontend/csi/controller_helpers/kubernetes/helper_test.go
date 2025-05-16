package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	k8sstoragev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
