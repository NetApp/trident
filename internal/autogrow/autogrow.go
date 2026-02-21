// Copyright 2025 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/utils/errors"
)

const (
	// DefaultGrowthAmountPercent is the default growth percentage applied when growthAmount is not specified
	// in the autogrow policy. This aligns with the CRD default value.
	DefaultGrowthAmountPercent = 10.0

	// ExtraBytesAboveResizeDelta is added on top of the backend's resize delta when we bump final capacity
	// so that the requested size is strictly greater than current + delta. Backends (e.g. ontap-san) treat
	// requests within the delta as a no-op; adding 1 MB ensures the backend actually performs the resize
	// and the node can complete filesystem expand (avoids PVC stuck in FileSystemResizeRequired).
	ExtraBytesAboveResizeDelta = 1000000 // 1 MB
)

// FinalCapacityRequest holds the inputs for final capacity calculation.
type FinalCapacityRequest struct {
	CurrentSize       resource.Quantity
	Policy            *tridentv1.TridentAutogrowPolicy
	ResizeDeltaBytes  int64
	CustomCeilingSize string // optional; e.g. subordinate source volume size to cap at
}

// FinalCapacityResponse holds the result of final capacity calculation.
// PolicyBasedCapacity is the capacity growth that would have been from policy alone (current + growth); FinalCapacity may differ after bump or cap.
// FinalCapacity is the final capacity (after optional bump and/or cap).
// ResizeDeltaBumpBytes > 0 when capacity was bumped to meet backend minimum growth (current+resizeDelta+ExtraBytesAboveResizeDelta).
// CappedAtPolicyMaxSize is true when final capacity was capped at policy maxSize (binding limit was policy, not custom ceiling).
// CappedAtCustomCeiling is true when final capacity was capped at custom ceiling (e.g. subordinate source size). When capped, exactly one of these is true.
// CappedAtBytes is the size in bytes we capped at (0 when not capped).
type FinalCapacityResponse struct {
	PolicyBasedCapacity   resource.Quantity // growth would have been this; final may differ after bump/cap
	FinalCapacity         resource.Quantity // final (after bump/cap)
	ResizeDeltaBumpBytes  int64
	CappedAtPolicyMaxSize bool
	CappedAtCustomCeiling bool
	CappedAtBytes         int64
}

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

// CalculateFinalCapacity computes the new volume capacity from the current size and autogrow policy.
// It (1) validates the policy spec; (2) applies the growth amount (percentage or absolute), using a
// default percentage if growth is not specified; (3) when resizeDeltaBytes > 0, if the computed
// growth would be less than that, bumps the requested size to current + resizeDeltaBytes +
// ExtraBytesAboveResizeDelta so the backend actually resizes (avoids within-delta no-op); (4) caps
// the result at maxSize when set so we grow as much as possible within the policy.
// When customCeilingSize is non-empty (e.g. for subordinate volumes capped by source volume size), the
// effective cap is min(policy maxSize, floor(customCeilingSize)); subordinate volumes cannot exceed their source.
// All parse-and-floor logic for caps lives here so final capacity calculation is in one place.
//
// Errors: policy validation failure; current size already at or above maxSize; or, after capping at
// maxSize, the resulting growth would still be less than resizeDeltaBytes+ExtraBytesAboveResizeDelta
// (backend would not resize, volume stuck).
//
// Parameters:
//   - req: FinalCapacityRequest with CurrentSize, Policy, ResizeDeltaBytes, CustomCeilingSize (optional).
//
// Returns:
//   - FinalCapacityResponse: FinalCapacity and flags; other fields indicate bump/cap details.
//   - error: As described above
func CalculateFinalCapacity(req FinalCapacityRequest) (FinalCapacityResponse, error) {
	zero := FinalCapacityResponse{}
	if req.Policy == nil {
		return zero, errors.AutogrowPolicyNilError()
	}

	// --- Step 1: Validate policy spec ---
	validated, err := ValidateAutogrowPolicySpec(
		req.Policy.Spec.UsedThreshold,
		req.Policy.Spec.GrowthAmount,
		req.Policy.Spec.MaxSize,
	)
	if err != nil {
		return zero, fmt.Errorf("invalid autogrow policy config: %w", err)
	}

	// --- Step 2: Resolve growth amount (default 10% if not specified) ---
	growth := validated.GrowthAmount
	if growth == nil {
		growth = &Amount{Value: DefaultGrowthAmountPercent, IsPercentage: true}
	}

	// --- Step 3: Compute capacity from policy (current + growth) ---
	currentSize := req.CurrentSize
	currentBytes := currentSize.Value()
	var capacityAfterGrowth resource.Quantity
	if growth.IsPercentage {
		growthBytes := int64(float64(currentBytes) * growth.Value / 100.0)
		capacityAfterGrowth = *resource.NewQuantity(currentBytes+growthBytes, currentSize.Format)
	} else {
		growthBytes := int64(growth.Value)
		capacityAfterGrowth = *resource.NewQuantity(currentBytes+growthBytes, currentSize.Format)
	}
	policyBasedCapacity := capacityAfterGrowth
	finalCapacity := capacityAfterGrowth

	// --- Step 4: Bump if growth is below backend resize delta ---
	// Some backends treat resize within a delta as no-op; bump so we exceed current+delta and the backend actually resizes.
	result := FinalCapacityResponse{PolicyBasedCapacity: policyBasedCapacity, FinalCapacity: finalCapacity}
	if req.ResizeDeltaBytes > 0 {
		growthRequested := finalCapacity.Value() - currentBytes
		if growthRequested > 0 && growthRequested <= req.ResizeDeltaBytes {
			bumpBytes := req.ResizeDeltaBytes + ExtraBytesAboveResizeDelta
			finalCapacity = *resource.NewQuantity(currentBytes+bumpBytes, currentSize.Format)
			result.FinalCapacity = finalCapacity
			result.ResizeDeltaBumpBytes = bumpBytes
		}
	}

	// --- Step 5: Cap at effective limit (min of policy maxSize and custom ceiling) ---
	// Parse and use floor instead of ceiling so we never exceed maxSize or custom ceiling (e.g. "1.7Gi" → 1825361100 bytes).
	policyMaxBytes := capacity.ParseQuantityFloorBytes(req.Policy.Spec.MaxSize)
	customCeilingBytes := capacity.ParseQuantityFloorBytes(req.CustomCeilingSize)
	effectiveCapLimitBytes := effectiveCapLimit(policyMaxBytes, customCeilingBytes)
	effectiveLimitFromCustomCeiling := customCeilingBytes > 0 && effectiveCapLimitBytes == customCeilingBytes

	if effectiveCapLimitBytes > 0 {
		capLimitQty := *resource.NewQuantity(int64(effectiveCapLimitBytes), currentSize.Format)
		if currentSize.Cmp(capLimitQty) >= 0 {
			return zero, errors.AutogrowAlreadyAtMaxSizeError(capLimitQty.String())
		}
		if finalCapacity.Cmp(capLimitQty) > 0 {
			finalCapacity = capLimitQty
			result.FinalCapacity = finalCapacity
			result.CappedAtBytes = int64(effectiveCapLimitBytes)
			result.CappedAtPolicyMaxSize = !effectiveLimitFromCustomCeiling
			result.CappedAtCustomCeiling = effectiveLimitFromCustomCeiling
		}
	}

	// --- Step 6: Stuck-resize check when capped and backend has a resize delta ---
	// If we capped and the resulting growth is below the backend's minimum, the resize would be a no-op; reject and ask to increase limit.
	capped := result.CappedAtPolicyMaxSize || result.CappedAtCustomCeiling
	if capped && req.ResizeDeltaBytes > 0 {
		bumpBytes := req.ResizeDeltaBytes + ExtraBytesAboveResizeDelta
		growthAfterCap := result.FinalCapacity.Value() - currentBytes
		if growthAfterCap < bumpBytes {
			minRequestedQty := *resource.NewQuantity(currentBytes+bumpBytes, currentSize.Format)
			remedy := "increase the effective limit (e.g. source volume size in case of subordinate volume) to at least %s"
			if result.CappedAtPolicyMaxSize {
				remedy = "increase autogrow policy maxSize to at least %s"
			}
			return zero, errors.AutogrowStuckResizeAtMaxSizeError(
				"volume cannot be autogrown: when capped at %s the volume would only grow by %d bytes, "+
					"which is less than the minimum growth of %s required by the backend; "+
					"minimum size that would trigger a resize is %s; "+remedy,
				result.FinalCapacity.String(), growthAfterCap, capacity.BytesToDecimalSizeString(bumpBytes), minRequestedQty.String(), minRequestedQty.String())
		}
	}

	return result, nil
}

// effectiveCapLimit returns the effective cap in bytes: min(policyMaxBytes, customCeilingBytes) when both are set;
// otherwise the one that is set, or 0 if neither.
func effectiveCapLimit(policyMaxBytes, customCeilingBytes uint64) uint64 {
	if policyMaxBytes == 0 {
		return customCeilingBytes
	}
	if customCeilingBytes == 0 {
		return policyMaxBytes
	}
	if customCeilingBytes < policyMaxBytes {
		return customCeilingBytes
	}
	return policyMaxBytes
}
