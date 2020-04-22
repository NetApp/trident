// Copyright 2020 NetApp, Inc. All Rights Reserved.

package controllers

type Controller interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
