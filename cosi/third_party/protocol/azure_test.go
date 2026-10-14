/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package protocol

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

func TestAzureBucketInfoTranslator_RoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[cosiapi.BucketInfoVar]string
		wantRpc *cosiproto.AzureBucketInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.BucketInfoVar]string{}, nil},
		{"info all set", map[cosiapi.BucketInfoVar]string{
			cosiapi.BucketInfoVar_Azure_StorageAccount: "mystorageaccount",
		},
			&cosiproto.AzureBucketInfo{
				StorageAccount: "mystorageaccount",
			},
		},
		{"all set empty",
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_Azure_StorageAccount: "",
			},
			&cosiproto.AzureBucketInfo{
				StorageAccount: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := AzureBucketInfoTranslator{}
			rpc := s.ApiToRpc(tt.vars)
			assert.Equal(t, tt.wantRpc, rpc)

			api := s.RpcToApi(rpc)
			if len(tt.vars) == 0 {
				assert.Nil(t, api)
			} else {
				assert.Equal(t, tt.vars, api)
			}
		})
	}
}

func TestAzureBucketInfoTranslator_RpcToApi(t *testing.T) {
	tests := []struct {
		name    string
		rpc     *cosiproto.AzureBucketInfo
		wantApi map[cosiapi.BucketInfoVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.AzureBucketInfo{},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_Azure_StorageAccount: "",
			},
		},
		{"all fields set",
			&cosiproto.AzureBucketInfo{
				StorageAccount: "mystorageaccount",
			},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_Azure_StorageAccount: "mystorageaccount",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := AzureBucketInfoTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_AZURE_"))
			}
		})
	}
}

func TestAzureCredentialTranslator_RoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[cosiapi.CredentialVar]string
		wantRpc *cosiproto.AzureCredentialInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.CredentialVar]string{}, nil},
		{"info all set", map[cosiapi.CredentialVar]string{
			cosiapi.CredentialVar_Azure_AccessToken:     "FAKEACCESSTOKEN",
			cosiapi.CredentialVar_Azure_ExpiryTimestamp: "2023-12-31T23:59:59Z",
		},
			&cosiproto.AzureCredentialInfo{
				AccessToken:     "FAKEACCESSTOKEN",
				ExpiryTimestamp: "2023-12-31T23:59:59Z",
			},
		},
		{"info with token only", map[cosiapi.CredentialVar]string{
			cosiapi.CredentialVar_Azure_AccessToken:     "FAKEACCESSTOKEN",
			cosiapi.CredentialVar_Azure_ExpiryTimestamp: "",
		},
			&cosiproto.AzureCredentialInfo{
				AccessToken:     "FAKEACCESSTOKEN",
				ExpiryTimestamp: "",
			},
		},
		{"all set empty",
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_Azure_AccessToken:     "",
				cosiapi.CredentialVar_Azure_ExpiryTimestamp: "",
			},
			&cosiproto.AzureCredentialInfo{
				AccessToken:     "",
				ExpiryTimestamp: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := AzureCredentialTranslator{}
			rpc := s.ApiToRpc(tt.vars)
			assert.Equal(t, tt.wantRpc, rpc)

			api := s.RpcToApi(rpc)
			if len(tt.vars) == 0 {
				assert.Nil(t, api)
			} else {
				assert.Equal(t, tt.vars, api)
			}
		})
	}
}

func TestAzureCredentialTranslator_RpcToApi(t *testing.T) {
	tests := []struct {
		name    string
		rpc     *cosiproto.AzureCredentialInfo
		wantApi map[cosiapi.CredentialVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.AzureCredentialInfo{},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_Azure_AccessToken:     "",
				cosiapi.CredentialVar_Azure_ExpiryTimestamp: "",
			},
		},
		{"all fields set",
			&cosiproto.AzureCredentialInfo{
				AccessToken:     "FAKEACCESSTOKEN",
				ExpiryTimestamp: "2023-12-31T23:59:59Z",
			},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_Azure_AccessToken:     "FAKEACCESSTOKEN",
				cosiapi.CredentialVar_Azure_ExpiryTimestamp: "2023-12-31T23:59:59Z",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := AzureCredentialTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_AZURE_"))
			}
		})
	}
}
