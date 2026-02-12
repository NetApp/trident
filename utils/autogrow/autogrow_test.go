// Copyright 2026 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test constants for Autogrow policy names
const (
	// Common test Autogrow policy names (short form)
	testAGPolicyGold   = "gold"
	testAGPolicySilver = "silver"
	testAGPolicyBronze = "bronze"

	// Full-form Autogrow policy names (with -policy suffix)
	testAGPolicyGoldFull     = "gold-policy"
	testAGPolicySilverFull   = "silver-policy"
	testAGPolicyBronzeFull   = "bronze-policy"
	testAGPolicyPlatinumFull = "platinum-policy"

	// Generic test identifiers
	testVolumeName  = "test-volume"
	testAGPolicyPVC = "pvc-policy"
	testAGPolicySC  = "sc-policy"
)

func TestAutogrowPolicyNoneConstant(t *testing.T) {
	// Verify the constant value is lowercase "none" as documented
	assert.Equal(t, "none", AutogrowPolicyNone)
}

func TestNameFix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Already valid lowercase",
			input:    "gold-policy",
			expected: "gold-policy",
		},
		{
			name:     "Uppercase to lowercase",
			input:    "GOLD-POLICY",
			expected: "gold-policy",
		},
		{
			name:     "Mixed case",
			input:    "Gold-Policy",
			expected: "gold-policy",
		},
		{
			name:     "Underscore replaced with hyphen",
			input:    "gold_policy",
			expected: "gold-policy",
		},
		{
			name:     "Spaces replaced with hyphens",
			input:    "my policy",
			expected: "my-policy",
		},
		{
			name:     "Multiple spaces",
			input:    "my  policy  name",
			expected: "my--policy--name",
		},
		{
			name:     "Special characters replaced",
			input:    "policy@123!",
			expected: "policy-123-",
		},
		{
			name:     "Numbers preserved",
			input:    "policy123",
			expected: "policy123",
		},
		{
			name:     "Dots preserved",
			input:    "my.policy.name",
			expected: "my.policy.name",
		},
		{
			name:     "Complex mixed input",
			input:    "My_Policy Name@123",
			expected: "my-policy-name-123",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NameFix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveEffectiveAutogrowPolicy(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		volumeName     string
		pvcAnnotation  string
		scAnnotation   string
		expectedResult string
		description    string
	}{
		// Priority 1: PVC annotation tests
		{
			name:           "PVC annotation with valid policy name",
			volumeName:     testVolumeName + "-1",
			pvcAnnotation:  testAGPolicyGoldFull,
			scAnnotation:   testAGPolicySilverFull,
			expectedResult: testAGPolicyGoldFull,
			description:    "PVC annotation should override SC annotation",
		},
		{
			name:           "PVC annotation is 'none' - explicitly disabled",
			volumeName:     testVolumeName + "-2",
			pvcAnnotation:  AutogrowPolicyNone,
			scAnnotation:   testAGPolicyGoldFull,
			expectedResult: "",
			description:    "PVC 'none' should block inheritance from SC and return empty",
		},
		{
			name:           "PVC annotation with different policy name",
			volumeName:     testVolumeName + "-3",
			pvcAnnotation:  testAGPolicyBronzeFull,
			scAnnotation:   "",
			expectedResult: testAGPolicyBronzeFull,
			description:    "PVC annotation should be returned when SC is empty",
		},

		// Priority 2: StorageClass annotation tests (PVC annotation empty)
		{
			name:           "SC annotation with valid policy name (PVC empty)",
			volumeName:     testVolumeName + "-4",
			pvcAnnotation:  "",
			scAnnotation:   testAGPolicySilverFull,
			expectedResult: testAGPolicySilverFull,
			description:    "SC annotation should be returned when PVC is empty",
		},
		{
			name:           "SC annotation is 'none' - explicitly disabled",
			volumeName:     testVolumeName + "-5",
			pvcAnnotation:  "",
			scAnnotation:   AutogrowPolicyNone,
			expectedResult: "",
			description:    "SC 'none' should disable autogrow and return empty",
		},
		{
			name:           "SC annotation with different policy name",
			volumeName:     testVolumeName + "-6",
			pvcAnnotation:  "",
			scAnnotation:   testAGPolicyPlatinumFull,
			expectedResult: testAGPolicyPlatinumFull,
			description:    "SC annotation should be returned when PVC is empty",
		},

		// Priority 3: No policy configured (both empty)
		{
			name:           "Both annotations empty",
			volumeName:     testVolumeName + "-7",
			pvcAnnotation:  "",
			scAnnotation:   "",
			expectedResult: "",
			description:    "Empty result when no policy is configured",
		},

		// Edge cases
		{
			name:           "Empty volume name with valid policies",
			volumeName:     "",
			pvcAnnotation:  "test-policy",
			scAnnotation:   testAGPolicySC,
			expectedResult: "test-policy",
			description:    "Function should work with empty volume name",
		},
		{
			name:           "Policy name 'none-policy' is valid (contains 'none' substring)",
			volumeName:     testVolumeName + "-none-substr",
			pvcAnnotation:  "none-policy",
			scAnnotation:   testAGPolicyGold,
			expectedResult: "none-policy",
			description:    "Policy names containing 'none' as substring should be valid",
		},
		{
			name:           "Policy name 'mynone' is valid (contains 'none' suffix)",
			volumeName:     testVolumeName + "-none-suffix",
			pvcAnnotation:  "mynone",
			scAnnotation:   testAGPolicyGold,
			expectedResult: "mynone",
			description:    "Policy names with 'none' suffix should be valid",
		},
		{
			name:           "Very long policy name",
			volumeName:     testVolumeName + "-long",
			pvcAnnotation:  "this-is-a-very-long-policy-name-that-exceeds-typical-lengths-for-testing-purposes-abcdefghijklmnopqrstuvwxyz",
			scAnnotation:   "short",
			expectedResult: "this-is-a-very-long-policy-name-that-exceeds-typical-lengths-for-testing-purposes-abcdefghijklmnopqrstuvwxyz",
			description:    "Long policy names should be preserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveEffectiveAutogrowPolicy(ctx, tt.volumeName, tt.pvcAnnotation, tt.scAnnotation)
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

// TestResolveEffectiveAutogrowPolicy_CaseInsensitiveNone tests that "none" is case-insensitive
func TestResolveEffectiveAutogrowPolicy_CaseInsensitiveNone(t *testing.T) {
	ctx := context.Background()

	noneVariations := []string{"none", "None", "NONE", "nOnE", "NoNe"}

	t.Run("PVC none variations all disable autogrow", func(t *testing.T) {
		for _, variant := range noneVariations {
			result := ResolveEffectiveAutogrowPolicy(ctx, "vol", variant, testAGPolicyGold)
			assert.Equal(t, "", result, "PVC '%s' should disable autogrow", variant)
		}
	})

	t.Run("SC none variations all disable autogrow when PVC empty", func(t *testing.T) {
		for _, variant := range noneVariations {
			result := ResolveEffectiveAutogrowPolicy(ctx, "vol", "", variant)
			assert.Equal(t, "", result, "SC '%s' should disable autogrow", variant)
		}
	})
}

// TestResolveEffectiveAutogrowPolicy_NameNormalization tests that policy names are normalized
func TestResolveEffectiveAutogrowPolicy_NameNormalization(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		pvcAnnotation  string
		scAnnotation   string
		expectedResult string
	}{
		{
			name:           "Uppercase PVC policy normalized to lowercase",
			pvcAnnotation:  "GOLD-POLICY",
			scAnnotation:   "",
			expectedResult: "gold-policy",
		},
		{
			name:           "Mixed case PVC policy normalized",
			pvcAnnotation:  "Gold-Policy",
			scAnnotation:   "",
			expectedResult: "gold-policy",
		},
		{
			name:           "Underscore in PVC policy replaced with hyphen",
			pvcAnnotation:  "gold_policy",
			scAnnotation:   "",
			expectedResult: "gold-policy",
		},
		{
			name:           "Spaces in PVC policy replaced with hyphens",
			pvcAnnotation:  "my policy",
			scAnnotation:   "",
			expectedResult: "my-policy",
		},
		{
			name:           "Uppercase SC policy normalized to lowercase",
			pvcAnnotation:  "",
			scAnnotation:   "SILVER-POLICY",
			expectedResult: "silver-policy",
		},
		{
			name:           "Mixed case SC policy normalized",
			pvcAnnotation:  "",
			scAnnotation:   "Silver-Policy",
			expectedResult: "silver-policy",
		},
		{
			name:           "Complex normalization",
			pvcAnnotation:  "My_Policy Name",
			scAnnotation:   "",
			expectedResult: "my-policy-name",
		},
		{
			name:           "Leading/trailing spaces normalized to hyphens",
			pvcAnnotation:  "  spaced-policy  ",
			scAnnotation:   "",
			expectedResult: "--spaced-policy--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveEffectiveAutogrowPolicy(ctx, "vol", tt.pvcAnnotation, tt.scAnnotation)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestResolveEffectiveAutogrowPolicy_WithTODOContext tests behavior with context.TODO()
func TestResolveEffectiveAutogrowPolicy_WithTODOContext(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		name           string
		pvcAnnotation  string
		scAnnotation   string
		expectedResult string
	}{
		{
			name:           "TODO context with PVC policy",
			pvcAnnotation:  "test-policy",
			scAnnotation:   "",
			expectedResult: "test-policy",
		},
		{
			name:           "TODO context with PVC none",
			pvcAnnotation:  AutogrowPolicyNone,
			scAnnotation:   testAGPolicyGold,
			expectedResult: "",
		},
		{
			name:           "TODO context with SC policy",
			pvcAnnotation:  "",
			scAnnotation:   testAGPolicySC,
			expectedResult: testAGPolicySC,
		},
		{
			name:           "TODO context with SC none",
			pvcAnnotation:  "",
			scAnnotation:   AutogrowPolicyNone,
			expectedResult: "",
		},
		{
			name:           "TODO context with both empty",
			pvcAnnotation:  "",
			scAnnotation:   "",
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveEffectiveAutogrowPolicy(ctx, testVolumeName, tt.pvcAnnotation, tt.scAnnotation)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestResolveEffectiveAutogrowPolicy_HierarchyExamples tests the documented examples
func TestResolveEffectiveAutogrowPolicy_HierarchyExamples(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		pvcAnnotation  string
		scAnnotation   string
		expectedResult string
		docExample     string
	}{
		{
			name:           "Example 1: PVC none blocks SC gold",
			pvcAnnotation:  AutogrowPolicyNone,
			scAnnotation:   testAGPolicyGold,
			expectedResult: "",
			docExample:     "PVC: \"none\", SC: \"gold\" → Returns \"\" (disabled, blocks SC)",
		},
		{
			name:           "Example 2: Empty PVC inherits from SC gold",
			pvcAnnotation:  "",
			scAnnotation:   testAGPolicyGold,
			expectedResult: testAGPolicyGold,
			docExample:     "PVC: \"\", SC: \"gold\" → Returns \"gold\" (inherits from SC)",
		},
		{
			name:           "Example 3: PVC silver overrides SC gold",
			pvcAnnotation:  testAGPolicySilver,
			scAnnotation:   testAGPolicyGold,
			expectedResult: testAGPolicySilver,
			docExample:     "PVC: \"silver\", SC: \"gold\" → Returns \"silver\" (PVC overrides)",
		},
		{
			name:           "Example 4: PVC My Policy normalized",
			pvcAnnotation:  "My Policy",
			scAnnotation:   "",
			expectedResult: "my-policy",
			docExample:     "PVC: \"My Policy\", SC: \"\" → Returns \"my-policy\" (normalized)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveEffectiveAutogrowPolicy(ctx, "doc-example-volume", tt.pvcAnnotation, tt.scAnnotation)
			assert.Equal(t, tt.expectedResult, result, "Failed for documented example: %s", tt.docExample)
		})
	}
}

// TestResolveEffectiveAutogrowPolicy_PriorityOrder verifies the strict priority ordering
func TestResolveEffectiveAutogrowPolicy_PriorityOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("PVC always takes priority over SC when PVC is non-empty", func(t *testing.T) {
		result := ResolveEffectiveAutogrowPolicy(ctx, "vol", testAGPolicyPVC, testAGPolicySC)
		assert.Equal(t, testAGPolicyPVC, result)

		result = ResolveEffectiveAutogrowPolicy(ctx, "vol", testAGPolicyPVC, "")
		assert.Equal(t, testAGPolicyPVC, result)

		result = ResolveEffectiveAutogrowPolicy(ctx, "vol", testAGPolicyPVC, AutogrowPolicyNone)
		assert.Equal(t, testAGPolicyPVC, result)
	})

	t.Run("SC is only used when PVC is empty", func(t *testing.T) {
		result := ResolveEffectiveAutogrowPolicy(ctx, "vol", "", testAGPolicySC)
		assert.Equal(t, testAGPolicySC, result)
	})

	t.Run("Empty result only when both are empty", func(t *testing.T) {
		result := ResolveEffectiveAutogrowPolicy(ctx, "vol", "", "")
		assert.Equal(t, "", result)
	})
}

// TestResolveEffectiveAutogrowPolicy_NoneKeywordBehavior tests the special "none" keyword handling
func TestResolveEffectiveAutogrowPolicy_NoneKeywordBehavior(t *testing.T) {
	ctx := context.Background()

	t.Run("PVC none blocks SC inheritance", func(t *testing.T) {
		scPolicies := []string{testAGPolicyGold, testAGPolicySilver, testAGPolicyBronze, "default", AutogrowPolicyNone, ""}
		for _, sc := range scPolicies {
			result := ResolveEffectiveAutogrowPolicy(ctx, "vol", AutogrowPolicyNone, sc)
			assert.Equal(t, "", result, "PVC 'none' should always return empty, SC was: %s", sc)
		}
	})

	t.Run("SC none disables autogrow when PVC is empty", func(t *testing.T) {
		result := ResolveEffectiveAutogrowPolicy(ctx, "vol", "", AutogrowPolicyNone)
		assert.Equal(t, "", result)
	})

	t.Run("Policy names containing 'none' are NOT treated as special", func(t *testing.T) {
		// These contain 'none' but are not equal to 'none'
		nonePolicies := []string{"none-policy", "mynone", "nonesuch", "before-none-after"}
		for _, policy := range nonePolicies {
			result := ResolveEffectiveAutogrowPolicy(ctx, "vol", policy, testAGPolicyGold)
			assert.Equal(t, policy, result, "Policy '%s' should be returned (normalized)", policy)
		}
	})
}
