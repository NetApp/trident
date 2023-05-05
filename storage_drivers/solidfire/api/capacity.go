// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// Get cluster capacity stats
func (c *Client) GetClusterCapacity(ctx context.Context) (capacity *ClusterCapacity, err error) {
	var (
		clusterCapReq    GetClusterCapacityRequest
		clusterCapResult GetClusterCapacityResult
	)

	response, err := c.Request(ctx, "GetClusterCapacity", clusterCapReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error detected in GetClusterCapacity API response: %+v", err)
		return nil, errors.New("device API error")
	}
	if err := json.Unmarshal(response, &clusterCapResult); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling json response: %+v", err)
		return nil, errors.New("json decode error")
	}
	return &clusterCapResult.Result.ClusterCapacity, err
}
