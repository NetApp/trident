// Copyright 2025 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

const (
	// DefaultGrowthAmountPercent is the default growth percentage applied when growthAmount is not specified
	// in the autogrow policy. This aligns with the CRD default value.
	DefaultGrowthAmountPercent = 10.0

	// ExtraBytesAboveResizeDelta is added on top of the backend's resize delta when we bump final capacity
	// so that the requested size is strictly greater than current + delta. Backends (e.g. ontap-san) treat
	// requests within the delta as a no-op; adding 1 MiB ensures the backend actually performs the resize
	// and the node can complete filesystem expand (avoids PVC stuck in FileSystemResizeRequired).
	ExtraBytesAboveResizeDelta = 1048576 // 1 MiB
)

// ValidateAutogrowPolicySpec validates the autogrow policy spec according to design requirements
// and returns normalized values for easier consumption.
//
// Validation Rules:
//  1. UsedThreshold (required):
//     - Must be a percentage: "1%" to "99%" (e.g., "80%")
//  2. GrowthAmount (optional, defaults to DefaultGrowthAmountPercent):
//     - Percentage: "1%" or higher (e.g., "10%", "50%")
//     - Absolute: valid resource.Quantity > 0
//  3. MaxSize (optional):
//     - Must be valid resource.Quantity
//
// Parameters:
//   - usedThreshold: The threshold percentage at which autogrow should trigger (must be percentage)
//   - growthAmount: The amount to grow by (can be percentage or absolute)
//   - maxSize: The maximum size limit (optional)
//
// Returns:
//   - *NormalizedAutogrowPolicySpec: Validated and normalized policy values
//   - error: Descriptive validation error, nil if validation passed

// Amount represents a validated growth amount with its type
type Amount struct {
	Value        float64 // float64 for percentage or absolute bytes
	IsPercentage bool    // true if percentage, false if absolute
}

// NormalizedAutogrowPolicySpec contains validated and normalized autogrow policy values
type NormalizedAutogrowPolicySpec struct {
	UsedThresholdPercent float32 // Always percentage value (1-99)
	GrowthAmount         *Amount // nil if not specified
	MaxSizeBytes         uint64  // 0 if not specified
}

func ValidateAutogrowPolicySpec(
	usedThreshold string,
	growthAmount string,
	maxSize string,
) (*NormalizedAutogrowPolicySpec, error) {
	result := &NormalizedAutogrowPolicySpec{}

	// Trim whitespace from all fields to handle common user errors (copy-paste, YAML formatting)
	usedThreshold = strings.TrimSpace(usedThreshold)
	growthAmount = strings.TrimSpace(growthAmount)
	maxSize = strings.TrimSpace(maxSize)

	// Rule 1: Validate UsedThreshold (required field, must be percentage)
	if usedThreshold == "" {
		return nil, errors.New("usedThreshold is required")
	}

	if strings.HasSuffix(usedThreshold, "%") {
		// Percentage format: must be between 1-99 (decimals supported, e.g., "80.5%")
		// Trim the % first, then trim any spaces between the number and %
		percentStr := strings.TrimSuffix(usedThreshold, "%")
		percentStr = strings.TrimSpace(percentStr)
		percent, parseErr := strconv.ParseFloat(percentStr, 32)
		if parseErr != nil {
			return nil, fmt.Errorf("usedThreshold percentage must be a valid number, got %s", usedThreshold)
		}
		if percent < 1 || percent > 99 {
			return nil, fmt.Errorf("usedThreshold percentage must be between 1 and 99, got %s%%", percentStr)
		}
		result.UsedThresholdPercent = float32(percent)
	} else {
		// UsedThreshold must be a percentage
		return nil, fmt.Errorf("usedThreshold must be a percentage (e.g., '80%%'), got %s", usedThreshold)
	}

	// Rule 2: Validate GrowthAmount (optional, but if provided must be valid)
	if growthAmount != "" {
		if strings.HasSuffix(growthAmount, "%") {
			// Percentage format: must be >= 1 (decimals supported, e.g., "10.5%")
			// Trim the % first, then trim any spaces between the number and %
			percentStr := strings.TrimSuffix(growthAmount, "%")
			percentStr = strings.TrimSpace(percentStr)
			percent, parseErr := strconv.ParseFloat(percentStr, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("growthAmount percentage must be a valid number, got %s", growthAmount)
			}
			if percent < 1 {
				return nil, fmt.Errorf("growthAmount percentage must be at least 1%%, got %s%%", percentStr)
			}
			result.GrowthAmount = &Amount{
				Value:        percent,
				IsPercentage: true,
			}
		} else {
			// Absolute value format: must be a valid resource.Quantity (decimals supported), > 0
			qty, parseErr := resource.ParseQuantity(growthAmount)
			if parseErr != nil {
				return nil, fmt.Errorf("growthAmount must be either a percentage (e.g., '10%%') or a valid resource quantity (e.g., '5Gi'): %v", parseErr)
			}
			if qty.Sign() <= 0 {
				return nil, fmt.Errorf("growthAmount must be greater than 0, got %s", growthAmount)
			}
			result.GrowthAmount = &Amount{
				Value:        float64(qty.Value()),
				IsPercentage: false,
			}
		}
	}

	// Rule 3: Validate MaxSize (optional, but if provided must be valid resource.Quantity)
	// Note: maxSize of "0" or "0Gi" is treated as "no limit" (same as not specifying it)
	if maxSize != "" {
		qty, parseErr := resource.ParseQuantity(maxSize)
		if parseErr != nil {
			return nil, fmt.Errorf("maxSize is not a valid resource quantity: %v", parseErr)
		}
		if qty.Sign() < 0 {
			return nil, fmt.Errorf("maxSize must not be negative, got %s", maxSize)
		}
		result.MaxSizeBytes = uint64(qty.Value())
	}

	// Rule 4: If both maxSize and growthAmount are absolute values, maxSize must be greater than growthAmount
	if result.MaxSizeBytes > 0 && result.GrowthAmount != nil && !result.GrowthAmount.IsPercentage {
		if result.MaxSizeBytes <= uint64(result.GrowthAmount.Value) {
			return nil, fmt.Errorf("maxSize (%d bytes) must be greater than growthAmount (%d bytes) when both are absolute values",
				result.MaxSizeBytes, uint64(result.GrowthAmount.Value))
		}
	}

	return result, nil
}

// CalculateFinalCapacity calculates the new volume capacity based on current size and autogrow policy.
// It validates the policy spec, applies the growth amount (percentage or absolute), optionally enforces
// a minimum growth (resizeDeltaBytes) for backends that treat resize within a delta as a no-op, and
// enforces maxSize. Returns an error if the result (after any delta bump) would exceed maxSize.
//
// Parameters:
//   - currentSize: Current volume capacity (as resource.Quantity)
//   - policy: TridentAutogrowPolicy object containing the policy spec
//   - resizeDeltaBytes: If > 0, ensures requested growth is at least this many bytes (then capped by maxSize if set)
//
// Returns:
//   - resource.Quantity: The calculated final capacity
//   - error: If policy validation fails, calculation fails, or result exceeds maxSize (when not applying delta)
func CalculateFinalCapacity(
	currentSize resource.Quantity,
	policy *tridentv1.TridentAutogrowPolicy,
	resizeDeltaBytes int64,
) (resource.Quantity, error) {
	if policy == nil {
		return resource.Quantity{}, errors.New("autogrow policy is nil")
	}

	// Step 1: Validate policy spec
	validated, err := ValidateAutogrowPolicySpec(
		policy.Spec.UsedThreshold,
		policy.Spec.GrowthAmount,
		policy.Spec.MaxSize,
	)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("invalid autogrow policy config: %w", err)
	}

	// Step 2: Apply default if growthAmount not specified
	growth := validated.GrowthAmount
	if growth == nil {
		// Default growth percentage
		growth = &Amount{
			Value:        DefaultGrowthAmountPercent,
			IsPercentage: true,
		}
	}

	// Step 3: Calculate final capacity from policy
	var finalCapacity resource.Quantity
	currentBytes := currentSize.Value()

	if growth.IsPercentage {
		// Percentage growth: currentSize * (1 + percent/100)
		growthBytes := int64(float64(currentBytes) * growth.Value / 100.0)
		finalBytes := currentBytes + growthBytes
		finalCapacity = *resource.NewQuantity(finalBytes, currentSize.Format)
	} else {
		// Absolute growth: currentSize + growthAmount
		growthBytes := int64(growth.Value)
		finalBytes := currentBytes + growthBytes
		finalCapacity = *resource.NewQuantity(finalBytes, currentSize.Format)
	}
	policyBasedCapacity := finalCapacity

	// Step 4: For backends that treat resize within a delta as no-op, ensure we request at least current + delta
	// + ExtraBytesAboveResizeDelta so the requested size is strictly above the tolerance band and the backend
	// actually resizes (avoids PVC stuck in FileSystemResizeRequired).
	appliedResizeDelta := false
	if resizeDeltaBytes > 0 {
		requestedGrowth := finalCapacity.Value() - currentBytes
		if requestedGrowth > 0 && requestedGrowth < resizeDeltaBytes {
			minFinalBytes := currentBytes + resizeDeltaBytes + ExtraBytesAboveResizeDelta
			finalCapacity = *resource.NewQuantity(minFinalBytes, currentSize.Format)
			appliedResizeDelta = true
		}
	}

	// Step 5: Enforce maxSize — return error if final capacity exceeds it (including after applying resize delta).
	if validated.MaxSizeBytes > 0 {
		maxQty := *resource.NewQuantity(int64(validated.MaxSizeBytes), currentSize.Format)
		if finalCapacity.Cmp(maxQty) > 0 {
			if appliedResizeDelta {
				return resource.Quantity{}, fmt.Errorf(
					"policy growthAmount would give %s (current + growth) but driver requires at least %d bytes growth (resize delta + 1 MiB); "+
						"so minimum size would be %s which exceeds maxSize %s; increase maxSize or volume cannot be resized",
					policyBasedCapacity.String(), resizeDeltaBytes+ExtraBytesAboveResizeDelta, finalCapacity.String(), maxQty.String())
			}
			return resource.Quantity{}, fmt.Errorf("calculated size %s exceeds maxSize %s",
				finalCapacity.String(), maxQty.String())
		}
	}

	return finalCapacity, nil
}
