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

func TestGcsBucketInfoTranslator_RoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[cosiapi.BucketInfoVar]string
		wantRpc *cosiproto.GcsBucketInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.BucketInfoVar]string{}, nil},
		{"info all set", map[cosiapi.BucketInfoVar]string{
			cosiapi.BucketInfoVar_GCS_BucketName: "my-bucket",
			cosiapi.BucketInfoVar_GCS_ProjectId:  "my-project",
		},
			&cosiproto.GcsBucketInfo{
				BucketName: "my-bucket",
				ProjectId:  "my-project",
			},
		},
		{"all set empty",
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_GCS_BucketName: "",
				cosiapi.BucketInfoVar_GCS_ProjectId:  "",
			},
			&cosiproto.GcsBucketInfo{
				BucketName: "",
				ProjectId:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GcsBucketInfoTranslator{}
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

func TestGcsBucketInfoTranslator_RpcToApi(t *testing.T) {
	tests := []struct {
		name    string
		rpc     *cosiproto.GcsBucketInfo
		wantApi map[cosiapi.BucketInfoVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.GcsBucketInfo{},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_GCS_BucketName: "",
				cosiapi.BucketInfoVar_GCS_ProjectId:  "",
			},
		},
		{"all fields set",
			&cosiproto.GcsBucketInfo{
				BucketName: "my-bucket",
				ProjectId:  "my-project",
			},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_GCS_BucketName: "my-bucket",
				cosiapi.BucketInfoVar_GCS_ProjectId:  "my-project",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GcsBucketInfoTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_GCS_"))
			}
		})
	}
}

func TestGcsCredentialTranslator_RoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[cosiapi.CredentialVar]string
		wantRpc *cosiproto.GcsCredentialInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.CredentialVar]string{}, nil},
		{"info all set", map[cosiapi.CredentialVar]string{
			cosiapi.CredentialVar_GCS_AccessId:       "FAKEACCESSID",
			cosiapi.CredentialVar_GCS_AccessSecret:   "FAKESECRET",
			cosiapi.CredentialVar_GCS_PrivateKeyName: "fake-key-name",
			cosiapi.CredentialVar_GCS_ServiceAccount: "fake-service-account@fake-project.iam.gserviceaccount.com",
		},
			&cosiproto.GcsCredentialInfo{
				AccessId:       "FAKEACCESSID",
				AccessSecret:   "FAKESECRET",
				PrivateKeyName: "fake-key-name",
				ServiceAccount: "fake-service-account@fake-project.iam.gserviceaccount.com",
			},
		},
		{"all set empty",
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_GCS_AccessId:       "",
				cosiapi.CredentialVar_GCS_AccessSecret:   "",
				cosiapi.CredentialVar_GCS_PrivateKeyName: "",
				cosiapi.CredentialVar_GCS_ServiceAccount: "",
			},
			&cosiproto.GcsCredentialInfo{
				AccessId:       "",
				AccessSecret:   "",
				PrivateKeyName: "",
				ServiceAccount: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GcsCredentialTranslator{}
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

func TestGcsCredentialTranslator_RpcToApi(t *testing.T) {
	tests := []struct {
		name    string
		rpc     *cosiproto.GcsCredentialInfo
		wantApi map[cosiapi.CredentialVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.GcsCredentialInfo{},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_GCS_AccessId:       "",
				cosiapi.CredentialVar_GCS_AccessSecret:   "",
				cosiapi.CredentialVar_GCS_PrivateKeyName: "",
				cosiapi.CredentialVar_GCS_ServiceAccount: "",
			},
		},
		{"all fields set",
			&cosiproto.GcsCredentialInfo{
				AccessId:       "FAKEACCESSID",
				AccessSecret:   "FAKESECRET",
				PrivateKeyName: "fake-key-name",
				ServiceAccount: "fake-service-account@fake-project.iam.gserviceaccount.com",
			},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_GCS_AccessId:       "FAKEACCESSID",
				cosiapi.CredentialVar_GCS_AccessSecret:   "FAKESECRET",
				cosiapi.CredentialVar_GCS_PrivateKeyName: "fake-key-name",
				cosiapi.CredentialVar_GCS_ServiceAccount: "fake-service-account@fake-project.iam.gserviceaccount.com",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := GcsCredentialTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_GCS_"))
			}
		})
	}
}
