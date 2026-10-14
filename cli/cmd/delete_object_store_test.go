// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestObjectStoreDelete(t *testing.T) {
	tests := []struct {
		name          string
		storeNames    []string
		setupMocks    func()
		wantErr       bool
		errorContains string
	}{
		{
			name:       "delete multiple object stores",
			storeNames: []string{"store-a", "", "store-b"},
			setupMocks: func() {
				httpmock.RegisterResponder(http.MethodDelete, BaseURL()+"/objectstore/store-a",
					httpmock.NewStringResponder(http.StatusOK, ""))
				httpmock.RegisterResponder(http.MethodDelete, BaseURL()+"/objectstore/store-b",
					httpmock.NewStringResponder(http.StatusOK, ""))
			},
		},
		{
			name:          "no object store specified",
			setupMocks:    func() {},
			wantErr:       true,
			errorContains: "object store name not specified",
		},
		{
			name:       "object store not found",
			storeNames: []string{"store-a"},
			setupMocks: func() {
				httpmock.RegisterResponder(http.MethodDelete, BaseURL()+"/objectstore/store-a",
					httpmock.NewStringResponder(http.StatusNotFound, `{"error":"object store store-a not found"}`))
			},
			wantErr:       true,
			errorContains: "could not delete object store store-a",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			test.setupMocks()

			err := objectStoreDelete(test.storeNames)

			if test.wantErr {
				assert.ErrorContains(t, err, test.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
