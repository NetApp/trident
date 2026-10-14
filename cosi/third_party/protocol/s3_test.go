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

func TestS3BucketInfoTranslator_RoundTrips(t *testing.T) {
	// APIs are more loosely typed, so round trip testing starting from API can be expected for all
	// translators.

	// TODO: can we set up fuzzing to test this for protos more generically?
	// TODO: assert output nil-ness for empty/nil inputs all translators in a separate test

	tests := []struct {
		name    string
		vars    map[cosiapi.BucketInfoVar]string // input
		wantRpc *cosiproto.S3BucketInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.BucketInfoVar]string{}, nil},
		{"info all set", map[cosiapi.BucketInfoVar]string{
			cosiapi.BucketInfoVar_S3_BucketId:        "bc-qwerty",
			cosiapi.BucketInfoVar_S3_Endpoint:        "s3.corp.net",
			cosiapi.BucketInfoVar_S3_Region:          "us-west-1",
			cosiapi.BucketInfoVar_S3_AddressingStyle: "path",
		},
			&cosiproto.S3BucketInfo{
				BucketId:        "bc-qwerty",
				Endpoint:        "s3.corp.net",
				Region:          "us-west-1",
				AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_PATH},
			},
		},
		{"addressing style unset",
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_S3_BucketId:        "bc-asdfgh",
				cosiapi.BucketInfoVar_S3_Endpoint:        "object.s3.com",
				cosiapi.BucketInfoVar_S3_Region:          "us-east-1",
				cosiapi.BucketInfoVar_S3_AddressingStyle: "",
			},
			&cosiproto.S3BucketInfo{
				BucketId:        "bc-asdfgh",
				Endpoint:        "object.s3.com",
				Region:          "us-east-1",
				AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_UNKNOWN},
			},
		},
		{"all set empty", // not valid for bucket access, but fine for Bucket status
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_S3_BucketId:        "",
				cosiapi.BucketInfoVar_S3_Endpoint:        "",
				cosiapi.BucketInfoVar_S3_Region:          "",
				cosiapi.BucketInfoVar_S3_AddressingStyle: "",
			},
			&cosiproto.S3BucketInfo{
				BucketId:        "",
				Endpoint:        "",
				Region:          "",
				AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_UNKNOWN},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := S3BucketInfoTranslator{}
			rpc := s.ApiToRpc(tt.vars)
			assert.Equal(t, tt.wantRpc, rpc)

			// should round trip back to the input
			api := s.RpcToApi(rpc)
			if len(tt.vars) == 0 { // input may be nil or empty
				assert.Nil(t, api) // but output must be nil by interface definition
			} else {
				assert.Equal(t, tt.vars, api)
			}
		})
	}
}

func TestS3BucketInfoTranslator_RpcToApi(t *testing.T) {
	// RPC is more strictly defined with more corner cases that can't be easily exercised with round
	// trip testing above

	tests := []struct {
		name    string
		rpc     *cosiproto.S3BucketInfo // input
		wantApi map[cosiapi.BucketInfoVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.S3BucketInfo{},
			map[cosiapi.BucketInfoVar]string{ // should have all fields present w/ no value
				cosiapi.BucketInfoVar_S3_BucketId:        "",
				cosiapi.BucketInfoVar_S3_Endpoint:        "",
				cosiapi.BucketInfoVar_S3_Region:          "",
				cosiapi.BucketInfoVar_S3_AddressingStyle: "",
			},
		},
		{"addressing style nil",
			&cosiproto.S3BucketInfo{
				BucketId:        "bc-qwerty",
				Endpoint:        "s3.corp.net",
				Region:          "us-west-1",
				AddressingStyle: nil,
			},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_S3_BucketId:        "bc-qwerty",
				cosiapi.BucketInfoVar_S3_Endpoint:        "s3.corp.net",
				cosiapi.BucketInfoVar_S3_Region:          "us-west-1",
				cosiapi.BucketInfoVar_S3_AddressingStyle: "",
			},
		},
		{"addressing style empty",
			&cosiproto.S3BucketInfo{
				BucketId:        "bc-qwerty",
				Endpoint:        "s3.corp.net",
				Region:          "us-west-1",
				AddressingStyle: &cosiproto.S3AddressingStyle{},
			},
			map[cosiapi.BucketInfoVar]string{
				cosiapi.BucketInfoVar_S3_BucketId:        "bc-qwerty",
				cosiapi.BucketInfoVar_S3_Endpoint:        "s3.corp.net",
				cosiapi.BucketInfoVar_S3_Region:          "us-west-1",
				cosiapi.BucketInfoVar_S3_AddressingStyle: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := S3BucketInfoTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_S3_"))
			}
		})
	}
}

func TestS3CredentialTranslator_RoundTrips(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[cosiapi.CredentialVar]string
		wantRpc *cosiproto.S3CredentialInfo
	}{
		{"nil info", nil, nil},
		{"empty info", map[cosiapi.CredentialVar]string{}, nil},
		{"info all set", map[cosiapi.CredentialVar]string{
			cosiapi.CredentialVar_S3_AccessKeyId:     "FAKEACCESSKEY",
			cosiapi.CredentialVar_S3_AccessSecretKey: "FAKESECRETKEY",
		},
			&cosiproto.S3CredentialInfo{
				AccessKeyId:     "FAKEACCESSKEY",
				AccessSecretKey: "FAKESECRETKEY",
			},
		},
		{"all set empty",
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_S3_AccessKeyId:     "",
				cosiapi.CredentialVar_S3_AccessSecretKey: "",
			},
			&cosiproto.S3CredentialInfo{
				AccessKeyId:     "",
				AccessSecretKey: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := S3CredentialTranslator{}
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

func TestS3CredentialTranslator_RpcToApi(t *testing.T) {
	tests := []struct {
		name    string
		rpc     *cosiproto.S3CredentialInfo
		wantApi map[cosiapi.CredentialVar]string
	}{
		{"nil input", nil, nil},
		{"empty input",
			&cosiproto.S3CredentialInfo{},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_S3_AccessKeyId:     "",
				cosiapi.CredentialVar_S3_AccessSecretKey: "",
			},
		},
		{"all fields set",
			&cosiproto.S3CredentialInfo{
				AccessKeyId:     "FAKEACCESSKEY",
				AccessSecretKey: "FAKESECRETKEY",
			},
			map[cosiapi.CredentialVar]string{
				cosiapi.CredentialVar_S3_AccessKeyId:     "FAKEACCESSKEY",
				cosiapi.CredentialVar_S3_AccessSecretKey: "FAKESECRETKEY",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := S3CredentialTranslator{}
			api := s.RpcToApi(tt.rpc)
			assert.Equal(t, tt.wantApi, api)

			for k := range api {
				assert.True(t, strings.HasPrefix(string(k), "COSI_S3_"))
			}
		})
	}
}
