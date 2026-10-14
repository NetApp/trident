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

package translator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

func TestTranslateBucketInfo(t *testing.T) {
	tests := []struct {
		name                string // description of this test case
		pbi                 *cosiproto.ObjectProtocolAndBucketInfo
		validation          *ValidationConfig
		wantProtos          []cosiapi.ObjectProtocol
		wantInfoVarPrefixes []string
		wantErr             string
	}{
		{"no info, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{}, nil,
			[]cosiapi.ObjectProtocol{}, []string{}, "",
		},
		{"no info, validate S3",
			&cosiproto.ObjectProtocolAndBucketInfo{},
			&ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `missing response for expected "S3" protocol`,
		},
		{"no info, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `missing response for expected "Azure" protocol`,
		},
		{"no info, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `missing response for expected "GCS" protocol`,
		},
		{"s3 empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolS3,
			},
			[]string{
				"COSI_S3_",
			},
			"",
		},
		{"s3 empty, validate S3",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating S3 bucket info",
		},
		{"s3 empty, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `missing response for expected "Azure" protocol`,
		},
		{"s3 non-empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
					Endpoint: "cosi.corp.net",
				},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolS3,
			},
			[]string{
				"COSI_S3_",
			},
			"",
		},
		{"s3 non-empty, validate S3",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
					Endpoint: "cosi.corp.net",
					// some required info missing to ensure validation is being activated
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating S3 bucket info",
		},
		{"s3 non-empty, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
					Endpoint: "cosi.corp.net",
					// some required info missing to ensure validation is being activated
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `missing response for expected "GCS" protocol`,
		},
		{"azure empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Azure: &cosiproto.AzureBucketInfo{},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolAzure,
			},
			[]string{
				"COSI_AZURE_",
			},
			"",
		},
		{"azure empty, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Azure: &cosiproto.AzureBucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating Azure bucket info",
		},
		{"azure non-empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "something",
				},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolAzure,
			},
			[]string{
				"COSI_AZURE_",
			},
			"",
		},
		{"azure non-empty, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "", // empty string to verify validation is being activated
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating Azure bucket info",
		},
		{"GCS empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Gcs: &cosiproto.GcsBucketInfo{},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolGcs,
			},
			[]string{
				"COSI_GCS_",
			},
			"",
		},
		{"GCS empty, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Gcs: &cosiproto.GcsBucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating GCS bucket info",
		},
		{"GCS non-empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
				},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolGcs,
			},
			[]string{
				"COSI_GCS_",
			},
			"",
		},
		{"GCS non-empty, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
					// some required info missing to ensure validation is being activated
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, "errors translating GCS bucket info",
		},
		{"s3+azure+GCS empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3:    &cosiproto.S3BucketInfo{},
				Azure: &cosiproto.AzureBucketInfo{},
				Gcs:   &cosiproto.GcsBucketInfo{},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolS3,
				cosiapi.ObjectProtocolAzure,
				cosiapi.ObjectProtocolGcs,
			},
			[]string{
				"COSI_S3_",
				"COSI_AZURE_",
				"COSI_GCS_",
			},
			"",
		},
		{"s3+azure+GCS empty, validate S3",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3:    &cosiproto.S3BucketInfo{},
				Azure: &cosiproto.AzureBucketInfo{},
				Gcs:   &cosiproto.GcsBucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "S3" protocol is expected`,
		},
		{"s3+azure+GCS empty, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3:    &cosiproto.S3BucketInfo{},
				Azure: &cosiproto.AzureBucketInfo{},
				Gcs:   &cosiproto.GcsBucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "Azure" protocol is expected`,
		},
		{"s3+azure+GCS empty, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3:    &cosiproto.S3BucketInfo{},
				Azure: &cosiproto.AzureBucketInfo{},
				Gcs:   &cosiproto.GcsBucketInfo{},
			},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "GCS" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, no validation",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
				},
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "acct",
				},
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
				},
			},
			nil, // no validation
			[]cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolS3,
				cosiapi.ObjectProtocolAzure,
				cosiapi.ObjectProtocolGcs,
			},
			[]string{
				"COSI_S3_",
				"COSI_AZURE_",
				"COSI_GCS_",
			},
			"",
		},
		{"s3+azure+GCS non-empty, validate S3",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
				},
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "acct",
				},
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "S3" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, validate Azure",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
				},
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "acct",
				},
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "Azure" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, validate GCS",
			&cosiproto.ObjectProtocolAndBucketInfo{
				S3: &cosiproto.S3BucketInfo{
					BucketId: "something",
				},
				Azure: &cosiproto.AzureBucketInfo{
					StorageAccount: "acct",
				},
				Gcs: &cosiproto.GcsBucketInfo{
					BucketName: "something",
				},
			},
			&ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, nil, `only "GCS" protocol is expected`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protos, infoVars, err := BucketInfoToApi(tt.pbi, tt.validation)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				t.Log("got error:", err)
				assert.ErrorContains(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantProtos, protos)
			// If we check the exact results of details.allProtoBucketInfo, we will tie the unit
			// tests to the specific implementation of bucket info translators, tested elsewhere.
			// Instead, check only that prefixes match what we expect.
			if len(tt.wantProtos) > 0 {
				assert.NotZero(t, len(infoVars))
			} else {
				assert.Zero(t, len(infoVars))
			}
			// t.Log("got info map:", infoVars)
			for _, p := range tt.wantInfoVarPrefixes {
				found := false
				for k := range infoVars {
					assert.True(t, strings.HasPrefix(k, "COSI_")) // all vars must be prefixed COSI_
					if strings.HasPrefix(k, p) {
						found = true
					}
				}
				assert.Truef(t, found, "prefix %q not found in %v keys", p, infoVars)
			}
		})
	}
}

func TestTranslateCredentials(t *testing.T) {
	tests := []struct {
		name                string // description of this test case
		pbi                 *cosiproto.CredentialInfo
		validation          ValidationConfig
		wantInfoVarPrefixes []string
		wantErr             string
	}{
		{"no info, validate S3",
			&cosiproto.CredentialInfo{},
			ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `missing response for expected "S3" protocol`,
		},
		{"no info, validate Azure",
			&cosiproto.CredentialInfo{},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `missing response for expected "Azure" protocol`,
		},
		{"no info, validate GCS",
			&cosiproto.CredentialInfo{},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `missing response for expected "GCS" protocol`,
		},
		{"s3 empty, validate S3",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating S3 bucket credentials",
		},
		{"s3 empty, validate Azure",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `missing response for expected "Azure" protocol`,
		},
		{"s3 non-empty, validate S3",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{
					AccessKeyId: "accesskey",
					// some required info missing to ensure validation is being activated
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating S3 bucket credentials",
		},
		{"s3 non-empty, validate GCS",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{
					AccessKeyId: "accesskey",
					// some required info missing to ensure validation is being activated
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `missing response for expected "GCS" protocol`,
		},
		{"azure empty, validate Azure",
			&cosiproto.CredentialInfo{
				Azure: &cosiproto.AzureCredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating Azure bucket credentials",
		},
		{"azure non-empty, validate Azure",
			&cosiproto.CredentialInfo{
				Azure: &cosiproto.AzureCredentialInfo{
					AccessToken: "", // empty string to verify validation is being activated
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating Azure bucket credentials",
		},
		{"GCS empty, validate GCS",
			&cosiproto.CredentialInfo{
				Gcs: &cosiproto.GcsCredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating GCS bucket credentials",
		},
		{"GCS non-empty, validate GCS",
			&cosiproto.CredentialInfo{
				Gcs: &cosiproto.GcsCredentialInfo{
					AccessId: "accessId",
					// some required info missing to ensure validation is being activated
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, "errors translating GCS bucket credentials",
		},
		{"s3+azure+GCS empty, validate S3",
			&cosiproto.CredentialInfo{
				S3:    &cosiproto.S3CredentialInfo{},
				Azure: &cosiproto.AzureCredentialInfo{},
				Gcs:   &cosiproto.GcsCredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "S3" protocol is expected`,
		},
		{"s3+azure+GCS empty, validate Azure",
			&cosiproto.CredentialInfo{
				S3:    &cosiproto.S3CredentialInfo{},
				Azure: &cosiproto.AzureCredentialInfo{},
				Gcs:   &cosiproto.GcsCredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "Azure" protocol is expected`,
		},
		{"s3+azure+GCS empty, validate GCS",
			&cosiproto.CredentialInfo{
				S3:    &cosiproto.S3CredentialInfo{},
				Azure: &cosiproto.AzureCredentialInfo{},
				Gcs:   &cosiproto.GcsCredentialInfo{},
			},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "GCS" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, validate S3",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{
					AccessKeyId: "something",
				},
				Azure: &cosiproto.AzureCredentialInfo{
					AccessToken: "",
				},
				Gcs: &cosiproto.GcsCredentialInfo{
					AccessId: "something",
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolS3, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "S3" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, validate Azure",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{
					AccessKeyId: "something",
				},
				Azure: &cosiproto.AzureCredentialInfo{
					AccessToken: "",
				},
				Gcs: &cosiproto.GcsCredentialInfo{
					AccessId: "something",
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolAzure, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "Azure" protocol is expected`,
		},
		{"s3+azure+GCS non-empty, validate GCS",
			&cosiproto.CredentialInfo{
				S3: &cosiproto.S3CredentialInfo{
					AccessKeyId: "something",
				},
				Azure: &cosiproto.AzureCredentialInfo{
					AccessToken: "",
				},
				Gcs: &cosiproto.GcsCredentialInfo{
					AccessId: "something",
				},
			},
			ValidationConfig{cosiapi.ObjectProtocolGcs, cosiapi.BucketAccessAuthenticationTypeKey},
			nil, `only "GCS" protocol is expected`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := CredentialsToApi(tt.pbi, tt.validation)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				t.Log("got error:", err)
				assert.ErrorContains(t, err, tt.wantErr)
			}
			// If we check the exact results of details.allProtoBucketInfo, we will tie the unit
			// tests to the specific implementation of bucket info translators, tested elsewhere.
			// Instead, check only that prefixes match what we expect.
			if tt.wantErr == "" {
				assert.NotZero(t, len(creds))
			}
			// t.Log("got info map:", infoVars)
			for _, p := range tt.wantInfoVarPrefixes {
				found := false
				for k := range creds {
					assert.True(t, strings.HasPrefix(k, "COSI_")) // all vars must be prefixed COSI_
					if strings.HasPrefix(k, p) {
						found = true
					}
				}
				assert.Truef(t, found, "prefix %q not found in %v keys", p, creds)
			}
		})
	}
}
