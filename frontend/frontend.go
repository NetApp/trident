// Copyright 2018 NetApp, Inc. All Rights Reserved.

package frontend

type Plugin interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
