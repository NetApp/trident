// Copyright 2024 NetApp, Inc. All Rights Reserved.

package yum

type Yum struct{}

func (y *Yum) MultipathToolsInstalled() bool {
	// TODO implement this function
	return true
}

func New() *Yum {
	return &Yum{}
}
