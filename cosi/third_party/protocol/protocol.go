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

// Package protocol contains definitions and functions for transforming COSI gRPC spec definitions
// into COSI Kubernetes definitions.
package protocol

import (
	"fmt"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"
)

// ObjectProtocolTranslator translates object protocol types between the RPC driver-domain and
// Kubernetes API user-domain.
type ObjectProtocolTranslator struct{}

var (
	objectProtocolProtoApiMapping = map[cosiproto.ObjectProtocol_Type]cosiapi.ObjectProtocol{
		cosiproto.ObjectProtocol_S3:    cosiapi.ObjectProtocolS3,
		cosiproto.ObjectProtocol_AZURE: cosiapi.ObjectProtocolAzure,
		cosiproto.ObjectProtocol_GCS:   cosiapi.ObjectProtocolGcs,
	}
)

// RpcToApi translates object protocols from RPC to API.
func (ObjectProtocolTranslator) RpcToApi(in cosiproto.ObjectProtocol_Type) (cosiapi.ObjectProtocol, error) {
	a, ok := objectProtocolProtoApiMapping[in]
	if !ok {
		return cosiapi.ObjectProtocol(""), fmt.Errorf("unknown driver protocol %q", string(in))
	}
	return a, nil
}

// ApiToRpc translates object protocols from API to RPC.
func (ObjectProtocolTranslator) ApiToRpc(in cosiapi.ObjectProtocol) (cosiproto.ObjectProtocol_Type, error) {
	for p, a := range objectProtocolProtoApiMapping {
		if a == in {
			return p, nil
		}
	}
	return cosiproto.ObjectProtocol_UNKNOWN, fmt.Errorf("unknown api protocol %q", string(in))
}

// An RpcApiTranslator translates types between the RPC driver-domain and Kubernetes API user-domain
// for a particular protocol.
type RpcApiTranslator[RpcType any, ApiType comparable] interface {
	// ApiProtocol returns the user-domain API object protocol.
	ApiProtocol() cosiapi.ObjectProtocol

	// RpcToApi translates bucket info from RPC to API with no validation.
	// If the input is nil, the result map MUST be nil.
	// All possible API info fields SHOULD be present in the result, even if the corresponding RPC
	// info is omitted; an empty string value should be used for keys with no explicit config.
	RpcToApi(RpcType) map[ApiType]string

	// ApiToRpc translates bucket info from API to RPC with no validation.
	// If the input map is nil or empty, this is assumed to mean the protocol is not supported, and
	// the result MUST be nil.
	ApiToRpc(map[ApiType]string) RpcType

	// Validate checks that user-domain API fields meet requirements and expectations.
	Validate(map[ApiType]string, cosiapi.BucketAccessAuthenticationType) error
}
