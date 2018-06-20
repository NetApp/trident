// Copyright 2018 NetApp, Inc. All Rights Reserved.

package csi

type TridentVolumeID struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
}

type TridentNodeID struct {
	Name string `json:"name"`
	IQN  string `json:"iqn"`
}
