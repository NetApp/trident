// Copyright 2025 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/netapp/trident/config"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

// TestValidateAutogrowPolicySpec_ValidFormats tests validation with valid formats
func TestValidateAutogrowPolicySpec_ValidFormats(t *testing.T) {
	tests := []struct {
		name                         string
		usedThreshold                string
		growthAmount                 string
		maxSize                      string
		expectedUsedThresholdPercent bool
		expectedGrowthAmountPercent  bool
	}{
		{"Percent threshold and growth", "80%", "10%", "100Gi", true, true},
		{"Percent threshold, bytes growth", "80%", "5Gi", "100Gi", true, false},
		{"Valid max size", "80%", "", "1Ti", true, false},
		{"Min valid threshold 1%", "1%", "", "100Gi", true, false},
		{"Max valid threshold 99%", "99%", "", "100Gi", true, false},
		{"Threshold with spaces", " 80% ", "", "100Gi", true, false},
		{"MaxSize with spaces", "80%", "", " 100Gi ", true, false},
		{"GrowthAmount with spaces", "80%", " 10% ", "100Gi", true, true},
		{"Valid decimal threshold", "50.5%", "", "100Gi", true, false},
		{"Valid decimal max size", "80%", "", "100.5Gi", true, false},
		{"Valid decimal growth amount", "80%", "5.5Gi", "100Gi", true, false},
		{"Valid decimal growth amount percent", "80%", "10.5%", "100Gi", true, true},
		{"Empty maxSize (optional)", "80%", "10%", "", true, true},
		{"MaxSize zero treated as no limit", "80%", "10%", "0Gi", true, true},
		{"Empty growthAmount (optional)", "80%", "", "100Gi", true, false},
		{"Threshold with space before % sign", "80 %", "", "100Gi", true, false},
		{"GrowthAmount with space before % sign", "80%", "10 %", "100Gi", true, true},
		{"MaxSize greater than growthAmount (both absolute)", "80%", "10Gi", "100Gi", true, false},
		{"MaxSize much greater than growthAmount (both absolute)", "80%", "5Gi", "1Ti", true, false},
		{"Percent growthAmount with absolute maxSize (no comparison)", "80%", "10%", "100Gi", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAutogrowPolicySpec(
				tt.usedThreshold, tt.growthAmount, tt.maxSize)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Greater(t, result.UsedThresholdPercent, float32(0))
			assert.True(t, tt.expectedUsedThresholdPercent) // usedThreshold is always percentage

			if tt.growthAmount != "" {
				assert.NotNil(t, result.GrowthAmount)
				assert.Equal(t, tt.expectedGrowthAmountPercent, result.GrowthAmount.IsPercentage)
			} else {
				assert.Nil(t, result.GrowthAmount)
			}

			if tt.maxSize != "" {
				// Special case: "0Gi" or "0" should be treated as "no limit" (0 bytes)
				if tt.maxSize == "0Gi" || tt.maxSize == "0" {
					assert.Equal(t, uint64(0), result.MaxSizeBytes)
				} else {
					assert.Greater(t, result.MaxSizeBytes, uint64(0))
				}
			} else {
				assert.Equal(t, uint64(0), result.MaxSizeBytes)
			}
		})
	}
}

// TestValidateAutogrowPolicySpec_InvalidFormats tests validation with invalid formats
func TestValidateAutogrowPolicySpec_InvalidFormats(t *testing.T) {
	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
		errorContains string
	}{
		{"Invalid threshold", "invalid", "", "100Gi", "usedThreshold"},
		{"Invalid max size", "80%", "", "invalid", "maxSize"},
		{"Threshold over 99%", "100%", "", "100Gi", "between 1 and 99"},
		{"Threshold equals 0%", "0%", "", "100Gi", "between 1 and 99"},
		{"Empty threshold", "", "", "100Gi", "required"},
		{"Growth amount 0%", "80%", "0%", "100Gi", "at least 1%"},
		{"Invalid growthAmount", "80%", "invalid", "100Gi", "growthAmount"},
		{"GrowthAmount zero", "80%", "0Gi", "100Gi", "greater than 0"},
		{"MaxSize negative", "80%", "", "-100Gi", "must not be negative"},
		{"Invalid percentage threshold - not a number", "abc%", "", "100Gi", "valid number"},
		{"Invalid growthAmount percentage - not a number", "80%", "xyz%", "100Gi", "valid number"},
		{"GrowthAmount absolute with negative", "80%", "-5Gi", "100Gi", "growthAmount"},
		{"Threshold with space in middle of number", "8 0%", "", "100Gi", "valid number"},
		{"GrowthAmount with space in middle", "80%", "1 0%", "100Gi", "valid number"},
		{"MaxSize with space in middle", "80%", "", "10 0Gi", "maxSize"},
		{"UsedThreshold as absolute value", "50Gi", "", "100Gi", "must be a percentage"},
		{"UsedThreshold as absolute decimal", "50.5Gi", "", "100Gi", "must be a percentage"},
		{"MaxSize less than growthAmount (both absolute)", "80%", "10Gi", "5Gi", "maxSize"},
		{"MaxSize equal to growthAmount (both absolute)", "80%", "10Gi", "10Gi", "maxSize"},
		{"MaxSize smaller than growthAmount (both absolute)", "80%", "100Gi", "50Gi", "maxSize"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAutogrowPolicySpec(
				tt.usedThreshold, tt.growthAmount, tt.maxSize)

			assert.Error(t, err)
			if tt.errorContains != "" {
				assert.Contains(t, err.Error(), tt.errorContains)
			}
		})
	}
}

// TestValidateAutogrowPolicySpec_NormalizedValues tests that normalized values are correct
func TestValidateAutogrowPolicySpec_NormalizedValues(t *testing.T) {
	tests := []struct {
		name                     string
		usedThreshold            string
		growthAmount             string
		maxSize                  string
		expectedUsedThreshold    float32
		expectedGrowthAmount     float64
		expectedMaxSize          uint64
		expectedUsedThresholdPct bool
		expectedGrowthAmountPct  bool
	}{
		{
			name:                     "Percent values",
			usedThreshold:            "80%",
			growthAmount:             "10%",
			maxSize:                  "100Gi",
			expectedUsedThreshold:    80.0,
			expectedGrowthAmount:     10.0,
			expectedMaxSize:          107374182400, // 100Gi in bytes
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  true,
		},
		{
			name:                     "Percent threshold with absolute growth",
			usedThreshold:            "80%",
			growthAmount:             "5Gi",
			maxSize:                  "100Gi",
			expectedUsedThreshold:    80.0,
			expectedGrowthAmount:     5368709120,   // 5Gi in bytes
			expectedMaxSize:          107374182400, // 100Gi in bytes
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  false,
		},
		{
			name:                     "Percent threshold with percent growth",
			usedThreshold:            "80%",
			growthAmount:             "10%",
			maxSize:                  "",
			expectedUsedThreshold:    80.0,
			expectedGrowthAmount:     10.0,
			expectedMaxSize:          0,
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  true,
		},
		{
			name:                     "Empty optional fields",
			usedThreshold:            "75%",
			growthAmount:             "",
			maxSize:                  "",
			expectedUsedThreshold:    75.0,
			expectedGrowthAmount:     0,
			expectedMaxSize:          0,
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  false,
		},
		{
			name:                     "Decimal percentage",
			usedThreshold:            "80.5%",
			growthAmount:             "10.25%",
			maxSize:                  "",
			expectedUsedThreshold:    80.5,
			expectedGrowthAmount:     10.25,
			expectedMaxSize:          0,
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  true,
		},
		{
			name:                     "MaxSize zero treated as no limit",
			usedThreshold:            "80%",
			growthAmount:             "10%",
			maxSize:                  "0Gi",
			expectedUsedThreshold:    80.0,
			expectedGrowthAmount:     10.0,
			expectedMaxSize:          0, // 0 means no limit
			expectedUsedThresholdPct: true,
			expectedGrowthAmountPct:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAutogrowPolicySpec(
				tt.usedThreshold, tt.growthAmount, tt.maxSize)

			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Check usedThreshold (always percentage now)
			assert.True(t, tt.expectedUsedThresholdPct)
			assert.Equal(t, float32(tt.expectedUsedThreshold), result.UsedThresholdPercent)

			// Check growthAmount
			if tt.growthAmount != "" {
				assert.NotNil(t, result.GrowthAmount)
				assert.Equal(t, tt.expectedGrowthAmountPct, result.GrowthAmount.IsPercentage)
				assert.Equal(t, tt.expectedGrowthAmount, result.GrowthAmount.Value)
			} else {
				assert.Nil(t, result.GrowthAmount)
			}

			// Check maxSize
			assert.Equal(t, tt.expectedMaxSize, result.MaxSizeBytes)
		})
	}
}

// TestValidateAutogrowPolicySpec_WhitespaceHandling tests trimming behavior
func TestValidateAutogrowPolicySpec_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
	}{
		{"Leading spaces", "  80%", "  10%", "  100Gi"},
		{"Trailing spaces", "80%  ", "10%  ", "100Gi  "},
		{"Both sides spaces", "  80%  ", "  10%  ", "  100Gi  "},
		{"Tabs", "\t80%\t", "\t10%\t", "\t100Gi\t"},
		{"Newlines", "\n80%\n", "\n10%\n", "\n100Gi\n"},
		{"Mixed whitespace", " \t80% \n", " \t10% \n", " \t100Gi \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAutogrowPolicySpec(
				tt.usedThreshold, tt.growthAmount, tt.maxSize)

			assert.NoError(t, err)
		})
	}
}

// TestValidateAutogrowPolicySpec_EdgeCases tests boundary conditions
func TestValidateAutogrowPolicySpec_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
		shouldPass    bool
	}{
		{"Minimum threshold 1%", "1%", "", "", true},
		{"Maximum threshold 99%", "99%", "", "", true},
		{"Below minimum 0.99%", "0.99%", "", "", false},
		{"Above maximum 99.01%", "99.01%", "", "", false},
		{"Minimum growth 1%", "80%", "1%", "", true},
		{"Below minimum growth 0.99%", "80%", "0.99%", "", false},
		{"Very small absolute threshold", "1Ki", "", "", false},    // UsedThreshold must be percentage
		{"Very large absolute threshold", "1000Ti", "", "", false}, // UsedThreshold must be percentage
		{"Very small growth amount", "80%", "1Ki", "", true},
		{"Very large growth amount", "80%", "1000Ti", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAutogrowPolicySpec(
				tt.usedThreshold, tt.growthAmount, tt.maxSize)

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// Helper function to create a test policy
func createTestPolicy(usedThreshold, growthAmount, maxSize string) *tridentv1.TridentAutogrowPolicy {
	return &tridentv1.TridentAutogrowPolicy{
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: usedThreshold,
			GrowthAmount:  growthAmount,
			MaxSize:       maxSize,
		},
	}
}

// TestCalculateFinalCapacity_NilPolicy tests nil policy handling
func TestCalculateFinalCapacity_NilPolicy(t *testing.T) {
	currentSize := resource.MustParse("10Gi")

	result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: nil, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

	assert.Error(t, err)
	assert.True(t, errors.IsAutogrowPolicyNilError(err))
	assert.Equal(t, resource.Quantity{}, result.FinalCapacity)
}

// TestCalculateFinalCapacity_InvalidPolicySpec tests invalid policy validation
func TestCalculateFinalCapacity_InvalidPolicySpec(t *testing.T) {
	tests := []struct {
		name          string
		currentSize   string
		policy        *tridentv1.TridentAutogrowPolicy
		errorContains string
	}{
		{
			name:          "Empty usedThreshold",
			currentSize:   "10Gi",
			policy:        createTestPolicy("", "10%", ""),
			errorContains: "usedThreshold is required",
		},
		{
			name:          "Invalid usedThreshold",
			currentSize:   "10Gi",
			policy:        createTestPolicy("invalid", "10%", ""),
			errorContains: "invalid autogrow policy config",
		},
		{
			name:          "UsedThreshold not percentage",
			currentSize:   "10Gi",
			policy:        createTestPolicy("10Gi", "10%", ""),
			errorContains: "must be a percentage",
		},
		{
			name:          "UsedThreshold below 1%",
			currentSize:   "10Gi",
			policy:        createTestPolicy("0%", "10%", ""),
			errorContains: "between 1 and 99",
		},
		{
			name:          "UsedThreshold above 99%",
			currentSize:   "10Gi",
			policy:        createTestPolicy("100%", "10%", ""),
			errorContains: "between 1 and 99",
		},
		{
			name:          "Invalid growthAmount",
			currentSize:   "10Gi",
			policy:        createTestPolicy("80%", "invalid", ""),
			errorContains: "invalid autogrow policy config",
		},
		{
			name:          "GrowthAmount 0%",
			currentSize:   "10Gi",
			policy:        createTestPolicy("80%", "0%", ""),
			errorContains: "at least 1%",
		},
		{
			name:          "Invalid maxSize",
			currentSize:   "10Gi",
			policy:        createTestPolicy("80%", "10%", "invalid"),
			errorContains: "invalid autogrow policy config",
		},
		{
			name:          "Negative maxSize",
			currentSize:   "10Gi",
			policy:        createTestPolicy("80%", "10%", "-100Gi"),
			errorContains: "must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: tt.policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Equal(t, resource.Quantity{}, result.FinalCapacity)
		})
	}
}

// TestCalculateFinalCapacity_PercentageGrowth tests percentage-based growth calculations
func TestCalculateFinalCapacity_PercentageGrowth(t *testing.T) {
	tests := []struct {
		name              string
		currentSize       string
		growthPercent     string
		expectedFinalSize string
		maxSize           string
	}{
		{
			name:              "10% growth on 10Gi",
			currentSize:       "10Gi",
			growthPercent:     "10%",
			expectedFinalSize: "11Gi",
			maxSize:           "",
		},
		{
			name:              "20% growth on 100Gi",
			currentSize:       "100Gi",
			growthPercent:     "20%",
			expectedFinalSize: "120Gi",
			maxSize:           "",
		},
		{
			name:              "50% growth on 20Gi",
			currentSize:       "20Gi",
			growthPercent:     "50%",
			expectedFinalSize: "30Gi",
			maxSize:           "",
		},
		{
			name:              "1% growth (minimum)",
			currentSize:       "100Gi",
			growthPercent:     "1%",
			expectedFinalSize: "101Gi",
			maxSize:           "",
		},
		{
			name:              "100% growth (double size)",
			currentSize:       "50Gi",
			growthPercent:     "100%",
			expectedFinalSize: "100Gi",
			maxSize:           "",
		},
		{
			name:              "Decimal percentage 10.5%",
			currentSize:       "100Gi",
			growthPercent:     "10.5%",
			expectedFinalSize: "110Gi", // Approximate due to rounding
			maxSize:           "",
		},
		{
			name:              "Small size 1Gi with 10%",
			currentSize:       "1Gi",
			growthPercent:     "10%",
			expectedFinalSize: "1181116006", // 1.1Gi in bytes (1.073741824 * 1.1)
			maxSize:           "",
		},
		{
			name:              "Large size 1Ti with 10%",
			currentSize:       "1Ti",
			growthPercent:     "10%",
			expectedFinalSize: "1209462790553", // 1.1Ti in bytes  (1099511627776 * 1.1)
			maxSize:           "",
		},
		{
			name:              "10% growth with maxSize not exceeded",
			currentSize:       "10Gi",
			growthPercent:     "10%",
			expectedFinalSize: "11Gi",
			maxSize:           "20Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthPercent, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedFinalSize)
			// Allow small rounding differences for decimal percentages
			assert.InDelta(t, expected.Value(), result.FinalCapacity.Value(), float64(expected.Value())*0.01)
		})
	}
}

// TestCalculateFinalCapacity_AbsoluteGrowth tests absolute growth calculations
func TestCalculateFinalCapacity_AbsoluteGrowth(t *testing.T) {
	tests := []struct {
		name              string
		currentSize       string
		growthAmount      string
		expectedFinalSize string
		maxSize           string
	}{
		{
			name:              "Add 5Gi to 10Gi",
			currentSize:       "10Gi",
			growthAmount:      "5Gi",
			expectedFinalSize: "15Gi",
			maxSize:           "",
		},
		{
			name:              "Add 10Gi to 100Gi",
			currentSize:       "100Gi",
			growthAmount:      "10Gi",
			expectedFinalSize: "110Gi",
			maxSize:           "",
		},
		{
			name:              "Add 1Mi to 1Gi",
			currentSize:       "1Gi",
			growthAmount:      "1Mi",
			expectedFinalSize: "1074790400", // 1Gi + 1Mi in bytes (1073741824 + 1048576)
			maxSize:           "",
		},
		{
			name:              "Add 100Gi to 1Ti",
			currentSize:       "1Ti",
			growthAmount:      "100Gi",
			expectedFinalSize: "1206885810176", // 1Ti + 100Gi in bytes (1099511627776 + 107374182400)
			maxSize:           "",
		},
		{
			name:              "Small absolute growth 1Ki",
			currentSize:       "10Gi",
			growthAmount:      "1Ki",
			expectedFinalSize: "10737419264", // 10Gi + 1Ki in bytes
			maxSize:           "",
		},
		{
			name:              "Decimal absolute growth 5.5Gi",
			currentSize:       "10Gi",
			growthAmount:      "5.5Gi",
			expectedFinalSize: "15.5Gi",
			maxSize:           "",
		},
		{
			name:              "Absolute growth with maxSize not exceeded",
			currentSize:       "10Gi",
			growthAmount:      "5Gi",
			expectedFinalSize: "15Gi",
			maxSize:           "20Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedFinalSize)
			assert.Equal(t, expected.Value(), result.FinalCapacity.Value())
		})
	}
}

// TestCalculateFinalCapacity_DefaultGrowth tests default 10% when growthAmount is empty
func TestCalculateFinalCapacity_DefaultGrowth(t *testing.T) {
	tests := []struct {
		name              string
		currentSize       string
		expectedFinalSize string
	}{
		{
			name:              "Default 10% on 10Gi",
			currentSize:       "10Gi",
			expectedFinalSize: "11Gi",
		},
		{
			name:              "Default 10% on 100Gi",
			currentSize:       "100Gi",
			expectedFinalSize: "110Gi",
		},
		{
			name:              "Default 10% on 1Ti",
			currentSize:       "1Ti",
			expectedFinalSize: "1209462790553", // 1.1Ti in bytes (1099511627776 * 1.1)
		},
		{
			name:              "Default 10% on 50Mi",
			currentSize:       "50Mi",
			expectedFinalSize: "55Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", "", "") // Empty growthAmount triggers default

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedFinalSize)
			assert.InDelta(t, expected.Value(), result.FinalCapacity.Value(), float64(expected.Value())*0.01)
		})
	}
}

// TestCalculateFinalCapacity_MaxSizeExceeded tests maxSize capping and the already-at-maxSize error.
// When growth would exceed maxSize we cap at maxSize (grow as much as possible). We only error when
// current size is already at or above maxSize (AutogrowAlreadyAtMaxSizeError).
func TestCalculateFinalCapacity_MaxSizeExceeded(t *testing.T) {
	tests := []struct {
		name                      string
		currentSize               string
		growthAmount              string
		maxSize                   string
		expectAlreadyAtMaxSizeErr bool   // when true, expect AutogrowAlreadyAtMaxSizeError
		expectedResult            string // when !expectAlreadyAtMaxSizeErr, expected result (e.g. "120Gi" when capped)
	}{
		{
			name:           "Percentage growth exceeds maxSize — cap at maxSize",
			currentSize:    "100Gi",
			growthAmount:   "50%",   // Would be 150Gi
			maxSize:        "120Gi", // Cap at 120Gi
			expectedResult: "120Gi",
		},
		{
			name:           "Absolute growth exceeds maxSize — cap at maxSize",
			currentSize:    "100Gi",
			growthAmount:   "30Gi",  // Would be 130Gi
			maxSize:        "120Gi", // Cap at 120Gi
			expectedResult: "120Gi",
		},
		{
			name:           "Default growth exceeds maxSize — cap at maxSize",
			currentSize:    "100Gi",
			growthAmount:   "",      // Default 10% = 110Gi
			maxSize:        "105Gi", // Cap at 105Gi
			expectedResult: "105Gi",
		},
		{
			name:           "Growth equals maxSize exactly (unchanged)",
			currentSize:    "100Gi",
			growthAmount:   "10Gi", // Would be 110Gi
			maxSize:        "110Gi",
			expectedResult: "110Gi",
		},
		{
			name:           "Growth just under maxSize (unchanged)",
			currentSize:    "100Gi",
			growthAmount:   "9Gi", // Would be 109Gi
			maxSize:        "110Gi",
			expectedResult: "109Gi",
		},
		{
			name:           "Large percentage exceeds maxSize — cap at maxSize",
			currentSize:    "10Gi",
			growthAmount:   "200%", // Would be 30Gi
			maxSize:        "20Gi", // Cap at 20Gi
			expectedResult: "20Gi",
		},
		{
			name:                      "Already at maxSize — error",
			currentSize:               "120Gi",
			growthAmount:              "10%",
			maxSize:                   "120Gi",
			expectAlreadyAtMaxSizeErr: true,
		},
		{
			name:                      "Already above maxSize — error",
			currentSize:               "121Gi",
			growthAmount:              "10%",
			maxSize:                   "120Gi",
			expectAlreadyAtMaxSizeErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			if tt.expectAlreadyAtMaxSizeErr {
				assert.Error(t, err)
				assert.True(t, errors.IsAutogrowAlreadyAtMaxSizeError(err))
				assert.Equal(t, resource.Quantity{}, result.FinalCapacity)
			} else {
				assert.NoError(t, err)
				expected := resource.MustParse(tt.expectedResult)
				assert.Equal(t, 0, result.FinalCapacity.Cmp(expected), "expected %s, got %s", tt.expectedResult, result.FinalCapacity.String())
			}
		})
	}
}

// TestCalculateFinalCapacity_DecimalMaxSizeNeverExceeded ensures that when maxSize is a decimal (e.g. "1.7Gi"),
// the final capacity never exceeds the user's limit. ParseQuantity can round up; we use floor when capping so we never patch beyond max.
func TestCalculateFinalCapacity_DecimalMaxSizeNeverExceeded(t *testing.T) {
	tests := []struct {
		name             string
		currentSizeBytes int64
		growthAmount     string
		maxSize          string
		wantCapBytes     int64
		wantCappedAtMax  bool
	}{
		{
			name:             "Growth would exceed decimal max — capped at floor of max, not rounded over",
			currentSizeBytes: 1546188225, // 1.44 GiB
			growthAmount:     "20%",
			maxSize:          "1.7Gi",
			wantCapBytes:     1825361100, // floor(1.7 * 1024^3)
			wantCappedAtMax:  true,
		},
		{
			name:             "Growth below decimal max — result unchanged, not capped",
			currentSizeBytes: 1073741824, // 1 GiB
			growthAmount:     "20%",
			maxSize:          "1.7Gi",
			wantCapBytes:     1288490188, // 1.2 GiB
			wantCappedAtMax:  false,
		},
		{
			name:             "Another decimal max (3.7Gi) — when growth would exceed, cap at floor of max",
			currentSizeBytes: 3758096384, // 3.5 GiB
			growthAmount:     "20%",
			maxSize:          "3.7Gi",
			wantCapBytes:     3972844748, // floor(3.7 * 1024^3)
			wantCappedAtMax:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := *resource.NewQuantity(tt.currentSizeBytes, resource.BinarySI)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)
			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCapBytes, result.FinalCapacity.Value(), "final capacity must not exceed maxSize (expected %d)", tt.wantCapBytes)
			assert.Equal(t, tt.wantCappedAtMax, result.CappedAtPolicyMaxSize)
		})
	}
}

// TestCalculateFinalCapacity_MaxSizeZero tests maxSize=0 (no limit)
func TestCalculateFinalCapacity_MaxSizeZero(t *testing.T) {
	tests := []struct {
		name         string
		currentSize  string
		growthAmount string
		maxSize      string
	}{
		{
			name:         "MaxSize 0Gi means no limit",
			currentSize:  "100Gi",
			growthAmount: "200%", // 300Gi total
			maxSize:      "0Gi",
		},
		{
			name:         "Empty maxSize means no limit",
			currentSize:  "100Gi",
			growthAmount: "500%", // 600Gi total
			maxSize:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err)
			assert.Greater(t, result.FinalCapacity.Value(), currentSize.Value())
		})
	}
}

// TestCalculateFinalCapacity_WhitespaceHandling tests trimming in policy fields
func TestCalculateFinalCapacity_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name              string
		currentSize       string
		usedThreshold     string
		growthAmount      string
		maxSize           string
		expectedFinalSize string
	}{
		{
			name:              "Spaces around percentage",
			currentSize:       "10Gi",
			usedThreshold:     " 80% ",
			growthAmount:      " 10% ",
			maxSize:           " 20Gi ",
			expectedFinalSize: "11Gi",
		},
		{
			name:              "Tabs and newlines",
			currentSize:       "10Gi",
			usedThreshold:     "\t80%\n",
			growthAmount:      "\t5Gi\n",
			maxSize:           "\t20Gi\n",
			expectedFinalSize: "15Gi",
		},
		{
			name:              "Mixed whitespace",
			currentSize:       "100Gi",
			usedThreshold:     " \t80% \n",
			growthAmount:      " \t20% \n",
			maxSize:           "",
			expectedFinalSize: "120Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy(tt.usedThreshold, tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedFinalSize)
			assert.Equal(t, expected.Value(), result.FinalCapacity.Value())
		})
	}
}

// TestCalculateFinalCapacity_EdgeCases tests boundary conditions
func TestCalculateFinalCapacity_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		currentSize  string
		growthAmount string
		maxSize      string
		shouldFail   bool
		description  string
	}{
		{
			name:         "Very small current size 1Ki",
			currentSize:  "1Ki",
			growthAmount: "10%",
			maxSize:      "",
			shouldFail:   false,
			description:  "Should handle small sizes",
		},
		{
			name:         "Very large current size 10Ti",
			currentSize:  "10Ti",
			growthAmount: "10%",
			maxSize:      "",
			shouldFail:   false,
			description:  "Should handle large sizes",
		},
		{
			name:         "Minimum growth percentage 1%",
			currentSize:  "100Gi",
			growthAmount: "1%",
			maxSize:      "",
			shouldFail:   false,
			description:  "Should handle minimum percentage",
		},
		{
			name:         "Large growth percentage 500%",
			currentSize:  "10Gi",
			growthAmount: "500%",
			maxSize:      "",
			shouldFail:   false,
			description:  "Should handle large percentages",
		},
		{
			name:         "Decimal percentage 0.5%",
			currentSize:  "1000Gi",
			growthAmount: "0.5%",
			maxSize:      "",
			shouldFail:   true,
			description:  "Should reject sub-1% growth",
		},
		{
			name:         "Very small absolute growth 1 byte",
			currentSize:  "10Gi",
			growthAmount: "1",
			maxSize:      "",
			shouldFail:   false,
			description:  "Should handle 1 byte growth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			if tt.shouldFail {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Greater(t, result.FinalCapacity.Value(), currentSize.Value(), tt.description)
			}
		})
	}
}

// TestCalculateFinalCapacity_MixedUnits tests calculations with different unit combinations
func TestCalculateFinalCapacity_MixedUnits(t *testing.T) {
	tests := []struct {
		name         string
		currentSize  string
		growthAmount string
		description  string
	}{
		{
			name:         "Gi current + Mi growth",
			currentSize:  "1Gi",
			growthAmount: "512Mi",
			description:  "Mix Gi and Mi",
		},
		{
			name:         "Ti current + Gi growth",
			currentSize:  "1Ti",
			growthAmount: "100Gi",
			description:  "Mix Ti and Gi",
		},
		{
			name:         "Mi current + Ki growth",
			currentSize:  "100Mi",
			growthAmount: "1024Ki",
			description:  "Mix Mi and Ki",
		},
		{
			name:         "Decimal Gi growth",
			currentSize:  "10Gi",
			growthAmount: "2.5Gi",
			description:  "Decimal growth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, "")

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: 0, CustomCeilingSize: ""})

			assert.NoError(t, err, tt.description)
			assert.Greater(t, result.FinalCapacity.Value(), currentSize.Value(), tt.description)

			// Verify the growth amount was added correctly
			growthQty := resource.MustParse(tt.growthAmount)
			expected := currentSize.DeepCopy()
			expected.Add(growthQty)
			assert.Equal(t, expected.Value(), result.FinalCapacity.Value())
		})
	}
}

// TestCalculateFinalCapacity_ResizeDelta tests that resizeDeltaBytes bumps small growth and is capped by maxSize.
// Uses config.SANResizeDelta (50 MB) + ExtraBytesAboveResizeDelta (1 MB) to simulate SAN-style backend behavior.
func TestCalculateFinalCapacity_ResizeDelta(t *testing.T) {
	delta := int64(config.SANResizeDelta)                // 50 MB — backend minimum growth to trigger resize
	deltaAfterBump := delta + ExtraBytesAboveResizeDelta // 50 MB + 1 MB — applied when policy growth <= delta
	// Expected result when current 100Gi is bumped by deltaAfterBump (used for "growth below/equal delta" cases).
	base100Gi := resource.MustParse("100Gi")
	expected100GiBumped := resource.NewQuantity(base100Gi.Value()+deltaAfterBump, resource.BinarySI).String()

	tests := []struct {
		name                   string
		currentSizeStr         string
		currentSizeMinusBump   bool // if true, current = parse(currentSizeStr).Value() - deltaAfterBump (for cap-at-maxSize case)
		growthAmount           string
		maxSize                string
		resizeDelta            int64
		expectStuckResizeError bool   // expect AutogrowStuckResizeAtMaxSizeError
		expectedResult         string // when no error: expected capacity string
	}{
		{
			name:           "Zero delta no effect",
			currentSizeStr: "100Gi",
			growthAmount:   "5Gi",
			maxSize:        "200Gi",
			resizeDelta:    0,
			expectedResult: "105Gi",
		},
		{
			name:           "Growth above delta unchanged",
			currentSizeStr: "100Gi",
			growthAmount:   "50Gi",
			maxSize:        "200Gi",
			resizeDelta:    delta,
			expectedResult: "150Gi",
		},
		{
			name:           "Growth below delta bumps to current+delta+1MB",
			currentSizeStr: "100Gi",
			growthAmount:   "10Mi",
			maxSize:        "200Gi",
			resizeDelta:    delta,
			expectedResult: expected100GiBumped,
		},
		{
			name:           "Growth equal to delta bumps to current+delta+1MB",
			currentSizeStr: "100Gi",
			growthAmount:   "50M", // 50e6 bytes = delta; backend treats <= delta as no-op
			maxSize:        "200Gi",
			resizeDelta:    delta,
			expectedResult: expected100GiBumped,
		},
		{
			name:                 "Bumped result exceeds maxSize — cap at maxSize when growth after cap >= bumpBytes",
			currentSizeStr:       "100Gi",
			currentSizeMinusBump: true,
			growthAmount:         "10Mi",
			maxSize:              "100Gi",
			resizeDelta:          delta,
			expectedResult:       "100Gi",
		},
		{
			name:                   "Capped at maxSize but growth below resize delta — error (stuck resize)",
			currentSizeStr:         "980M",
			growthAmount:           "10M",
			maxSize:                "1G",
			resizeDelta:            delta,
			expectStuckResizeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var currentSize resource.Quantity
			if tt.currentSizeMinusBump {
				maxQty := resource.MustParse(tt.currentSizeStr)
				currentSize = *resource.NewQuantity(maxQty.Value()-deltaAfterBump, resource.BinarySI)
			} else {
				currentSize = resource.MustParse(tt.currentSizeStr)
			}
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: tt.resizeDelta, CustomCeilingSize: ""})

			if tt.expectStuckResizeError {
				assert.Error(t, err)
				assert.True(t, errors.IsAutogrowStuckResizeAtMaxSizeError(err))
				assert.True(t, result.FinalCapacity.IsZero())
				return
			}
			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedResult)
			assert.Equal(t, 0, result.FinalCapacity.Cmp(expected), "expected %s, got %s", tt.expectedResult, result.FinalCapacity.String())
		})
	}
}

// TestCalculateFinalCapacity_CustomCeilingBytes tests the optional custom ceiling (e.g. subordinate source size).
// Effective max = min(policy maxSize, floor(customCeilingSize)); parse and floor done in CalculateFinalCapacity.
func TestCalculateFinalCapacity_CustomCeilingBytes(t *testing.T) {
	tests := []struct {
		name                        string
		currentSize                 string
		growthAmount                string
		maxSize                     string
		customCeiling               string // empty = no ceiling
		expectedResult              string
		expectCappedAtPolicyMaxSize bool
		expectCappedAtCustomCeiling bool
		expectedEffectiveCap        string // expected CappedAtBytes when capped; empty = 0
		expectAlreadyAtMaxSizeErr   bool
		expectStuckResizeError      bool
		resizeDelta                 int64
	}{
		{
			name:                        "customCeiling empty — no effect",
			currentSize:                 "100Gi",
			growthAmount:                "10%",
			maxSize:                     "200Gi",
			customCeiling:               "",
			expectedResult:              "110Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: false,
			expectedEffectiveCap:        "",
		},
		{
			name:                        "Custom ceiling below policy maxSize — cap at ceiling (subordinate at source size)",
			currentSize:                 "100Gi",
			growthAmount:                "50%", // 150Gi
			maxSize:                     "200Gi",
			customCeiling:               "120Gi",
			expectedResult:              "120Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: true,
			expectedEffectiveCap:        "120Gi",
		},
		{
			name:                        "Custom ceiling above policy maxSize — effective max is policy maxSize",
			currentSize:                 "100Gi",
			growthAmount:                "150%", // 250Gi
			maxSize:                     "200Gi",
			customCeiling:               "300Gi",
			expectedResult:              "200Gi",
			expectCappedAtPolicyMaxSize: true,
			expectCappedAtCustomCeiling: false,
			expectedEffectiveCap:        "200Gi",
		},
		{
			name:                        "Policy has no maxSize, custom ceiling set — cap at ceiling",
			currentSize:                 "100Gi",
			growthAmount:                "50%",
			maxSize:                     "",
			customCeiling:               "120Gi",
			expectedResult:              "120Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: true,
			expectedEffectiveCap:        "120Gi",
		},
		{
			name:                        "Growth below custom ceiling — not capped",
			currentSize:                 "100Gi",
			growthAmount:                "10%", // 110Gi
			maxSize:                     "200Gi",
			customCeiling:               "120Gi",
			expectedResult:              "110Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: false,
			expectedEffectiveCap:        "",
		},
		{
			name:                      "Already at custom ceiling — error",
			currentSize:               "120Gi",
			growthAmount:              "10%",
			maxSize:                   "200Gi",
			customCeiling:             "120Gi",
			expectAlreadyAtMaxSizeErr: true,
		},
		{
			name:                        "Capped at custom ceiling, CappedAtBytes set",
			currentSize:                 "50Gi",
			growthAmount:                "200%", // 150Gi
			maxSize:                     "200Gi",
			customCeiling:               "100Gi",
			expectedResult:              "100Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: true,
			expectedEffectiveCap:        "100Gi",
		},
		{
			name:                        "Capped at custom ceiling with resize delta — growth after cap >= bumpBytes (ok)",
			currentSize:                 "100Gi",
			growthAmount:                "20%", // 120Gi, cap at 110Gi -> growth 10Gi > bumpBytes
			maxSize:                     "200Gi",
			customCeiling:               "110Gi",
			expectedResult:              "110Gi",
			expectCappedAtPolicyMaxSize: false,
			expectCappedAtCustomCeiling: true,
			expectedEffectiveCap:        "110Gi",
			resizeDelta:                 int64(config.SANResizeDelta),
		},
		{
			name:                   "Stuck resize when capped at custom ceiling — growth after cap < bumpBytes",
			currentSize:            "1000Mi", // close to 1Gi so cap leaves growth < 50MB+1MB
			growthAmount:           "10M",
			maxSize:                "2Gi",
			customCeiling:          "1Gi",
			expectStuckResizeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSize := resource.MustParse(tt.currentSize)
			policy := createTestPolicy("80%", tt.growthAmount, tt.maxSize)

			resizeDelta := tt.resizeDelta
			if tt.expectStuckResizeError && resizeDelta == 0 {
				resizeDelta = int64(config.SANResizeDelta)
			}

			result, err := CalculateFinalCapacity(FinalCapacityRequest{CurrentSize: currentSize, Policy: policy, ResizeDeltaBytes: resizeDelta, CustomCeilingSize: tt.customCeiling})

			if tt.expectAlreadyAtMaxSizeErr {
				assert.Error(t, err)
				assert.True(t, errors.IsAutogrowAlreadyAtMaxSizeError(err))
				assert.True(t, result.FinalCapacity.IsZero())
				return
			}
			if tt.expectStuckResizeError {
				assert.Error(t, err)
				assert.True(t, errors.IsAutogrowStuckResizeAtMaxSizeError(err))
				assert.True(t, result.FinalCapacity.IsZero())
				return
			}

			assert.NoError(t, err)
			expected := resource.MustParse(tt.expectedResult)
			assert.Equal(t, 0, result.FinalCapacity.Cmp(expected), "expected %s, got %s", tt.expectedResult, result.FinalCapacity.String())
			assert.Equal(t, tt.expectCappedAtPolicyMaxSize, result.CappedAtPolicyMaxSize)
			if tt.expectedEffectiveCap != "" {
				expectedCap := resource.MustParse(tt.expectedEffectiveCap)
				assert.Equal(t, expectedCap.Value(), result.CappedAtBytes)
			} else {
				assert.Equal(t, int64(0), result.CappedAtBytes)
			}
			if tt.expectCappedAtCustomCeiling {
				assert.True(t, result.CappedAtCustomCeiling, "expected CappedAtCustomCeiling")
			} else if !tt.expectAlreadyAtMaxSizeErr && !tt.expectStuckResizeError {
				assert.False(t, result.CappedAtCustomCeiling)
			}
		})
	}
}
