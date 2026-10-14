// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
)

func TestObjectStoreCreate(t *testing.T) {
	tests := []struct {
		name       string
		responder  httpmock.Responder
		wantErr    bool
		wantOutput string
	}{
		{
			name: "success",
			responder: func(_ *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(http.StatusCreated, rest.AddObjectStoreResponse{
					ObjectStoreID: "object-store-1",
				})
			},
			wantOutput: "object-store-1\n",
		},
		{
			name: "server error",
			responder: func(_ *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(http.StatusBadRequest, `{"error":"create failed"}`), nil
			},
			wantErr: true,
		},
		{
			name: "invalid response",
			responder: func(_ *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(http.StatusCreated, "invalid JSON"), nil
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			httpmock.RegisterResponder(http.MethodPost, BaseURL()+"/objectstore", test.responder)

			var err error
			output := captureOutput(t, func() {
				err = objectStoreCreate([]byte(`{"objectStoreDriver":"fake","objectStoreName":"store-a"}`))
			})

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.wantOutput, output)
		})
	}
}
