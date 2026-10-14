// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
)

const fakeGetObjectStoreResponse = `{"objectStore":{"objectStoreName":"store-a","objectStoreDriver":"fake","uuid":"1234","state":"online"}}`

func TestObjectStoreList(t *testing.T) {
	tests := []struct {
		name       string
		storeNames []string
		setupMocks func()
		wantErr    bool
		wantOutput string
	}{
		{
			name:       "get named object store",
			storeNames: []string{"store-a"},
			setupMocks: func() {
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore/store-a",
					httpmock.NewStringResponder(http.StatusOK, fakeGetObjectStoreResponse))
			},
			wantOutput: "store-a\n",
		},
		{
			name: "get all object stores skips ones deleted since listing",
			setupMocks: func() {
				responder, err := httpmock.NewJsonResponder(http.StatusOK, rest.ListObjectStoresResponse{
					ObjectStores: []string{"store-a", "store-b"},
				})
				if err != nil {
					panic(err)
				}
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore", responder)
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore/store-a",
					httpmock.NewStringResponder(http.StatusOK, fakeGetObjectStoreResponse))
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore/store-b",
					httpmock.NewStringResponder(http.StatusNotFound, `{"error":"object store store-b not found"}`))
			},
			wantOutput: "store-a\n",
		},
		{
			name:       "named object store not found",
			storeNames: []string{"store-b"},
			setupMocks: func() {
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore/store-b",
					httpmock.NewStringResponder(http.StatusNotFound, `{"error":"object store store-b not found"}`))
			},
			wantErr: true,
		},
		{
			name: "list error",
			setupMocks: func() {
				httpmock.RegisterResponder(http.MethodGet, BaseURL()+"/objectstore",
					httpmock.NewStringResponder(http.StatusInternalServerError, `{"error":"server error"}`))
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			origFormat := OutputFormat
			OutputFormat = FormatName
			defer func() { OutputFormat = origFormat }()

			test.setupMocks()

			var err error
			output := captureOutput(t, func() {
				err = objectStoreList(test.storeNames)
			})

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.wantOutput, output)
			}
		})
	}
}
