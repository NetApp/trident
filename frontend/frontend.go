// Copyright 2018 NetApp, Inc. All Rights Reserved.

package frontend

//go:generate mockgen -destination=../mocks/mock_frontend/mock_plugin.go github.com/netapp/trident/frontend Plugin

type Plugin interface {
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
