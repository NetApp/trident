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
	"testing"

	"github.com/stretchr/testify/assert"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

func TestObjectProtocolTranslator(t *testing.T) {
	tests := []struct {
		name    string                 // description of this test case
		api     cosiapi.ObjectProtocol // input
		wantRpc cosiproto.ObjectProtocol_Type
		wantErr bool
	}{
		{"empty string", cosiapi.ObjectProtocol(""), cosiproto.ObjectProtocol_UNKNOWN, true},
		{"unknown proto", cosiapi.ObjectProtocol("evilcorp-proto"), cosiproto.ObjectProtocol_UNKNOWN, true},
		{"S3", cosiapi.ObjectProtocolS3, cosiproto.ObjectProtocol_S3, false},
		{"Azure", cosiapi.ObjectProtocolAzure, cosiproto.ObjectProtocol_AZURE, false},
		{"GCS", cosiapi.ObjectProtocolGcs, cosiproto.ObjectProtocol_GCS, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := ObjectProtocolTranslator{}

			gotRpc, gotErr := o.ApiToRpc(tt.api)
			if tt.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
			assert.Equal(t, tt.wantRpc, gotRpc)

			// Test the round trip. Tests UNKNOWN proto as input for tests where err is expected.
			rtGot, rtErr := o.RpcToApi(tt.wantRpc)
			if tt.wantErr {
				assert.Error(t, rtErr)
				assert.Equal(t, cosiapi.ObjectProtocol(""), rtGot)
			} else {
				assert.NoError(t, rtErr)
				assert.Equal(t, tt.api, rtGot)
			}
		})
	}
}
