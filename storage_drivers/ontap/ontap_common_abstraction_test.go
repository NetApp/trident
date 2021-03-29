// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/stretchr/testify/assert"
)

func NewAPIResponse(
	client, version, status, reason, errno string,
) *api.APIResponse {
	result := api.NewAPIResponse(status, reason, errno)
	return result
}

var (
	APIResponsePassed = NewAPIResponse("client", "version", "passed", "reason", "errno")
	APIResponseFailed = NewAPIResponse("client", "version", "failed", "reason", "errno")
)

func TestApiGetError(t *testing.T) {

	ctx := context.Background()

	var snapListResponseErr error
	assert.Nil(t, snapListResponseErr)

	apiErr := api.GetErrorAbstraction(ctx, nil, snapListResponseErr)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: nil response", apiErr.Error())

	apiErr = api.GetErrorAbstraction(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetErrorAbstraction(ctx, APIResponsePassed, nil)
	assert.Nil(t, apiErr)

	apiErr = api.GetErrorAbstraction(ctx, APIResponseFailed, nil)
	assert.NotNil(t, apiErr)
	assert.Equal(t, "API error: &{ failed reason errno}", apiErr.Error())
}
