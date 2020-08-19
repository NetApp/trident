// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/netapp/trident/logger"
)

// Get default QoS information
func (c *Client) GetDefaultQoS(ctx context.Context) (*QoS, error) {

	var (
		defaultQoSReq    DefaultQoSRequest
		defaultQoSResult DefaultQoSResult
	)

	response, err := c.Request(ctx, "GetDefaultQoS", defaultQoSReq, NewReqID())
	if err != nil {
		Logc(ctx).Errorf("error detected in GetDefaultQoS API response: %+v", err)
		return nil, errors.New("device API error")
	}
	if err := json.Unmarshal(response, &defaultQoSResult); err != nil {
		Logc(ctx).Errorf("error detected unmarshalling json response: %+v", err)
		return nil, errors.New("json decode error")
	}

	return &QoS{
		BurstIOPS: defaultQoSResult.Result.BurstIOPS,
		MaxIOPS:   defaultQoSResult.Result.MaxIOPS,
		MinIOPS:   defaultQoSResult.Result.MinIOPS,
	}, err
}
