// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// Get cluster hardware info
func (c *Client) GetClusterHardwareInfo(ctx context.Context) (*ClusterHardwareInfo, error) {
	var (
		clusterHardwareInfoReq    struct{}
		clusterHardwareInfoResult GetClusterHardwareInfoResult
	)

	response, err := c.Request(ctx, "GetClusterHardwareInfo", clusterHardwareInfoReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("Error detected in GetClusterHardwareInfo API response: %+v", err)
		return nil, errors.New("device API error")
	}

	if err := json.Unmarshal(response, &clusterHardwareInfoResult); err != nil {
		Logc(ctx).Errorf("Error detected unmarshalling json response: %+v", err)
		return nil, errors.New("json decode error")
	}
	return &clusterHardwareInfoResult.Result.ClusterHardwareInfo, err
}
