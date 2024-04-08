// Copyright 2024 NetApp, Inc. All Rights Reserved.

package frontend

type Frontend interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
