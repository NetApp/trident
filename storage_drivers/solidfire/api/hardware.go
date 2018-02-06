// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"errors"

	log "github.com/sirupsen/logrus"
)

// Get cluster hardware info
func (c *Client) GetClusterHardwareInfo() (*ClusterHardwareInfo, error) {
	var (
		clusterHardwareInfoReq    struct{}
		clusterHardwareInfoResult GetClusterHardwareInfoResult
	)

	response, err := c.Request("GetClusterHardwareInfo", clusterHardwareInfoReq, NewReqID())
	if err != nil {
		log.Errorf("Error detected in GetClusterHardwareInfo API response: %+v", err)
		return nil, errors.New("device API error")
	}

	if err := json.Unmarshal([]byte(response), &clusterHardwareInfoResult); err != nil {
		log.Errorf("Error detected unmarshalling json response: %+v", err)
		return nil, errors.New("json decode error")
	}
	return &clusterHardwareInfoResult.Result.ClusterHardwareInfo, err
}
