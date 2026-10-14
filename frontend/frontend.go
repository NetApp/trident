// Copyright 2018 NetApp, Inc. All Rights Reserved.

package frontend

//go:generate mockgen -destination=../mocks/mock_frontend/mock_plugin.go github.com/netapp/trident/frontend Plugin

type Plugin interface {
	// Activate starts the frontend. An error here is logged by the caller and, unless wrapped
	// with utils/errors.TerminalReconciliationError, does not stop Trident: one frontend failing
	// to start must not take the others down with it.
	Activate() error
	Deactivate() error
	GetName() string
	Version() string
}
