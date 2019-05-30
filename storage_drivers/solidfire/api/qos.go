// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"

	log "github.com/sirupsen/logrus"
)

// Get default QoS information
func (c *Client) GetDefaultQoS() (*QoS, error) {
	var (
		defaultQoSReq    DefaultQoSRequest
		defaultQoSResult DefaultQoSResult
	)

	response, err := c.Request("GetDefaultQoS", defaultQoSReq, NewReqID())
	if err != nil {
		log.Errorf("error detected in GetDefaultQoS API response: %+v", err)
		return nil, errors.New("device API error")
	}
	if err := json.Unmarshal([]byte(response), &defaultQoSResult); err != nil {
		log.Errorf("error detected unmarshalling json response: %+v", err)
		return nil, errors.New("json decode error")
	}

	return &QoS{
		BurstIOPS: defaultQoSResult.Result.BurstIOPS,
		MaxIOPS:   defaultQoSResult.Result.MaxIOPS,
		MinIOPS:   defaultQoSResult.Result.MinIOPS,
	}, err
}
