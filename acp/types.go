// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

//go:generate mockgen -destination=../mocks/mock_acp/mock_acp.go github.com/netapp/trident/acp TridentACP
//go:generate mockgen -destination=../mocks/mock_acp/mock_rest/mock_rest.go github.com/netapp/trident/acp REST

import (
	"context"

	"github.com/netapp/trident/utils/version"
)

// TridentACP is a set of methods for exposing Trident-ACP REST APIs to Trident.
type TridentACP interface {
	GetVersion(context.Context) (*version.Version, error)
	GetVersionWithBackoff(context.Context) (*version.Version, error)
	IsFeatureEnabled(context.Context, string) error
}

// REST is a set of methods for interacting with Trident-ACP REST APIs.
type REST interface {
	GetVersion(context.Context) (*version.Version, error)
	Entitled(context.Context, string) error
}
