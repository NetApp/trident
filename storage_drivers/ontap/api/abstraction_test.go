// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

type unixPermissionsTest struct {
	value          string
	expectedResult string
}

func TestConvertUnixPermissions(t *testing.T) {
	testValues := []unixPermissionsTest{
		{"---rwxrwxrwx", "777"},
		{"---rwxr-xr-x", "755"},
		{"---rw-rw-rw-", "666"},
		{"---r-xr-xr-x", "555"},
		{"---r--r--r--", "444"},
		{"----w--w--w-", "222"},
		{"------------", "000"},
		{"rwxrwxrwx", "777"},
	}

	for _, test := range testValues {
		assert.Equal(t,
			test.expectedResult,
			convertUnixPermissions(test.value),
			"Unexpected value",
		)
	}
}

func TestHasNextLink(t *testing.T) {
	////////////////////////////////////////////
	// negative tests
	assert.False(t,
		HasNextLink(nil),
		"Should NOT have a next link")

	assert.False(t,
		HasNextLink(APIResponse{}),
		"Should NOT have a next link")

	assert.False(t,
		HasNextLink(&models.VolumeResponse{
			Links: &models.VolumeResponseInlineLinks{},
		}),
		"Should NOT have a next link")

	////////////////////////////////////////////
	// positive tests
	assert.True(t,
		HasNextLink(
			&models.VolumeResponse{
				Links: &models.VolumeResponseInlineLinks{
					Next: &models.Href{
						Href: convert.ToPtr("/api/storage/volumes?start.uuid=00c881eb-f36c-11e8-996b-00a0986e75a0&fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM"),
					},
				},
			}),
		"SHOULD have a next link")

	assert.True(t,
		HasNextLink(
			&models.SnapshotResponse{
				Links: &models.SnapshotResponseInlineLinks{
					Next: &models.Href{
						Href: convert.ToPtr("/api/storage/snapshot?start.uuid=00c881eb-f36c-11e8-996b-00a0986e75a0&fields=%2A%2A&max_records=1&name=%2A&return_records=true&svm.name=SVM"),
					},
				},
			}),
		"SHOULD have a next link")
}

func TestIsSnapshotBusyError(t *testing.T) {
	snapshotName := "testSnapshot"
	sourceVolume := "sourceVolume"
	err := SnapshotBusyError(fmt.Sprintf("snapshot %s backing volume %s is busy", snapshotName, sourceVolume))
	assert.True(t,
		IsSnapshotBusyError(err),
		"Should be a SnapshotBusyError")
}

// Test SVMAggregateSpace struct and methods
func TestSVMAggregateSpace(t *testing.T) {
	t.Run("Constructor and getters", func(t *testing.T) {
		size := int64(1000)
		used := int64(500)
		footprint := int64(800)

		aggSpace := NewSVMAggregateSpace(size, used, footprint)

		assert.Equal(t, size, aggSpace.Size(), "Size should match")
		assert.Equal(t, used, aggSpace.Used(), "Used should match")
		assert.Equal(t, footprint, aggSpace.Footprint(), "Footprint should match")
	})

	t.Run("Edge cases", func(t *testing.T) {
		// Test with zero values
		aggSpace := NewSVMAggregateSpace(0, 0, 0)
		assert.Equal(t, int64(0), aggSpace.Size())
		assert.Equal(t, int64(0), aggSpace.Used())
		assert.Equal(t, int64(0), aggSpace.Footprint())

		// Test with negative values
		aggSpace = NewSVMAggregateSpace(-100, -50, -80)
		assert.Equal(t, int64(-100), aggSpace.Size())
		assert.Equal(t, int64(-50), aggSpace.Used())
		assert.Equal(t, int64(-80), aggSpace.Footprint())
	})
}

// Test APIResponse struct and methods
func TestAPIResponse(t *testing.T) {
	t.Run("Constructor and getters", func(t *testing.T) {
		status := "passed"
		reason := "success"
		errno := "0"

		response := NewAPIResponse(status, reason, errno)

		assert.Equal(t, status, response.Status(), "Status should match")
		assert.Equal(t, reason, response.Reason(), "Reason should match")
		assert.Equal(t, errno, response.Errno(), "Errno should match")
		assert.Empty(t, response.APIName(), "APIName should be empty")
	})

	t.Run("Empty values", func(t *testing.T) {
		response := NewAPIResponse("", "", "")
		assert.Empty(t, response.Status())
		assert.Empty(t, response.Reason())
		assert.Empty(t, response.Errno())
		assert.Empty(t, response.APIName())
	})
}

func TestGetErrorFunction(t *testing.T) {
	t.Run("Nil response", func(t *testing.T) {
		ctx := context.Background()
		err := GetError(ctx, nil, nil)
		assert.Error(t, err, "expected error when GetError is called with nil response")
		assert.Contains(t, err.Error(), "nil response")
	})

	t.Run("Input error takes precedence", func(t *testing.T) {
		ctx := context.Background()
		inputError := fmt.Errorf("input error")
		response := NewAPIResponse("passed", "success", "0")

		err := GetError(ctx, response, inputError)
		assert.Equal(t, inputError, err)
	})

	t.Run("Passed status returns no error", func(t *testing.T) {
		ctx := context.Background()
		response := NewAPIResponse("passed", "success", "0")

		err := GetError(ctx, response, nil)
		assert.NoError(t, err)
	})

	t.Run("Failed status returns error", func(t *testing.T) {
		ctx := context.Background()
		response := NewAPIResponse("failed", "operation failed", "1")

		err := GetError(ctx, response, nil)
		assert.Error(t, err, "expected error when GetError is called with failed status")
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("Other status returns error", func(t *testing.T) {
		ctx := context.Background()
		response := NewAPIResponse("unknown", "unknown status", "2")

		err := GetError(ctx, response, nil)
		assert.Error(t, err, "expected error when GetError is called with unknown status")
		assert.Contains(t, err.Error(), "API error")
	})
}

// Test all error types from abstraction_error.go
func TestAbstractionErrorTypes(t *testing.T) {
	t.Run("VolumeCreateJobExistsError", func(t *testing.T) {
		msg := "volume create job exists"
		err := VolumeCreateJobExistsError(msg)
		assert.Error(t, err, "expected VolumeCreateJobExistsError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsVolumeCreateJobExistsError(err))
		assert.False(t, IsVolumeCreateJobExistsError(nil))
		assert.False(t, IsVolumeCreateJobExistsError(fmt.Errorf("other error")))
	})

	t.Run("VolumeReadError", func(t *testing.T) {
		msg := "volume read error"
		err := VolumeReadError(msg)
		assert.Error(t, err, "expected VolumeReadError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsVolumeReadError(err))
		assert.False(t, IsVolumeReadError(nil))
		assert.False(t, IsVolumeReadError(fmt.Errorf("other error")))
	})

	t.Run("VolumeIdAttributesReadError", func(t *testing.T) {
		msg := "volume id attributes read error"
		err := VolumeIdAttributesReadError(msg)
		assert.Error(t, err, "expected VolumeIdAttributesReadError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsVolumeIdAttributesReadError(err))
		assert.False(t, IsVolumeIdAttributesReadError(nil))
		assert.False(t, IsVolumeIdAttributesReadError(fmt.Errorf("other error")))
	})

	t.Run("VolumeSpaceAttributesReadError", func(t *testing.T) {
		msg := "volume space attributes read error"
		err := VolumeSpaceAttributesReadError(msg)
		assert.Error(t, err, "expected VolumeSpaceAttributesReadError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsVolumeSpaceAttributesReadError(err))
		assert.False(t, IsVolumeSpaceAttributesReadError(nil))
		assert.False(t, IsVolumeSpaceAttributesReadError(fmt.Errorf("other error")))
	})

	t.Run("ApiError", func(t *testing.T) {
		msg := "api error"
		err := ApiError(msg)
		assert.Error(t, err, "expected ApiError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsApiError(err))
		assert.False(t, IsApiError(nil))
		assert.False(t, IsApiError(fmt.Errorf("other error")))
	})

	t.Run("NotFoundError", func(t *testing.T) {
		msg := "not found error"
		err := NotFoundError(msg)
		assert.Error(t, err, "expected NotFoundError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsNotFoundError(err))
		assert.False(t, IsNotFoundError(nil))
		assert.False(t, IsNotFoundError(fmt.Errorf("other error")))
	})

	t.Run("NotReadyError", func(t *testing.T) {
		msg := "not ready error"
		err := NotReadyError(msg)
		assert.Error(t, err, "expected NotReadyError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsNotReadyError(err))
		assert.False(t, IsNotReadyError(nil))
		assert.False(t, IsNotReadyError(fmt.Errorf("other error")))
	})

	t.Run("TooManyLunsError", func(t *testing.T) {
		msg := "too many luns error"
		err := TooManyLunsError(msg)
		assert.Error(t, err, "expected TooManyLunsError to be an error")
		assert.Equal(t, msg, err.Error())
		assert.True(t, IsTooManyLunsError(err))
		assert.False(t, IsTooManyLunsError(nil))
		assert.False(t, IsTooManyLunsError(fmt.Errorf("other error")))
	})
}

// Enhanced tests for existing functions
func TestConvertUnixPermissionsEdgeCases(t *testing.T) {
	edgeCases := []unixPermissionsTest{
		// Invalid/malformed strings
		{"", ""},
		{"invalid", "invalid"},
		{"rwx", "rwx"},                   // Too short
		{"rwxrwxrwxrwx", "rwxrwxrwxrwx"}, // Too long
		{"abcdefghijkl", "abcdefghijkl"}, // Non-permission characters
		{"r-xr-xr-x", "555"},             // Missing initial dashes but valid format gets converted
		// Unicode/special characters
		{"---rwx™wx™wx", "rwx™wx™wx"}, // Non-standard characters get trimmed
	}

	for _, test := range edgeCases {
		assert.Equal(t,
			test.expectedResult,
			convertUnixPermissions(test.value),
			fmt.Sprintf("Failed for input: %q", test.value),
		)
	}
}

func TestHasNextLinkEdgeCases(t *testing.T) {
	t.Run("Various nil and empty cases", func(t *testing.T) {
		// Test with different types that don't have next links
		assert.False(t, HasNextLink("string"))
		assert.False(t, HasNextLink(123))
		assert.False(t, HasNextLink([]string{}))
		assert.False(t, HasNextLink(map[string]interface{}{}))
	})

	t.Run("Response with empty href", func(t *testing.T) {
		// Based on actual behavior - empty string is considered a valid href
		assert.True(t,
			HasNextLink(&models.VolumeResponse{
				Links: &models.VolumeResponseInlineLinks{
					Next: &models.Href{
						Href: convert.ToPtr(""),
					},
				},
			}),
			"Should have a next link even with empty href")
	})

	t.Run("Response with nil href pointer", func(t *testing.T) {
		// Based on actual behavior - nil pointer is considered a valid href
		assert.True(t,
			HasNextLink(&models.VolumeResponse{
				Links: &models.VolumeResponseInlineLinks{
					Next: &models.Href{
						Href: nil,
					},
				},
			}),
			"Should have a next link even with nil href pointer")
	})
}
