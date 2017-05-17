// Copyright 2016 NetApp, Inc. All Rights Reserved.

package frontend

type FrontendPlugin interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
