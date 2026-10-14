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
	"errors"
	"fmt"
	"reflect"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/cosi/third_party/protocol"
)

// ValidationConfig controls behavior of validation logic when translating bucket/credential info.
type ValidationConfig struct {
	// ExpectedProtocol the singular object protocol response that is expected.
	// Any responses for other protocols other will result in a validation error.
	ExpectedProtocol cosiapi.ObjectProtocol

	// Validation logic uses BucketAccess authenticationType for internal logic. Set accordingly.
	AuthenticationType cosiapi.BucketAccessAuthenticationType
}

// TranslateBucketInfo translates all bucket info for all protocols from an RPC response.
// If validation is configured (non-nil), only the expected protocol is allowed to have bucket info.
func BucketInfoToApi(
	bi *cosiproto.ObjectProtocolAndBucketInfo,
	validation *ValidationConfig,
) (
	returnedProtos []cosiapi.ObjectProtocol,
	allProtoBucketInfo map[string]string,
	err error,
) {
	if bi == nil {
		return nil, nil, fmt.Errorf("bucket info response is nil")
	}

	errs := []error{}
	returnedProtos = []cosiapi.ObjectProtocol{}
	allProtoBucketInfo = map[string]string{}

	if bi.S3 != nil {
		returnedProtos = append(returnedProtos, cosiapi.ObjectProtocolS3)
	}
	info, err := translate(protocol.S3BucketInfoTranslator{}, bi.S3, validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating %s bucket info: %w", cosiapi.ObjectProtocolS3, err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoBucketInfo)
	}

	if bi.Azure != nil {
		returnedProtos = append(returnedProtos, cosiapi.ObjectProtocolAzure)
	}
	info, err = translate(protocol.AzureBucketInfoTranslator{}, bi.Azure, validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating %s bucket info: %w", cosiapi.ObjectProtocolAzure, err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoBucketInfo)
	}

	if bi.Gcs != nil {
		returnedProtos = append(returnedProtos, cosiapi.ObjectProtocolGcs)
	}
	info, err = translate(protocol.GcsBucketInfoTranslator{}, bi.Gcs, validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating %s bucket info: %w", cosiapi.ObjectProtocolGcs, err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoBucketInfo)
	}

	if len(errs) > 0 {
		return nil, nil, fmt.Errorf("errors translating bucket info: %w", errors.Join(errs...))
	}
	return returnedProtos, allProtoBucketInfo, nil
}

// TranslateCredentials translates all credential info for all protocols from an RPC response.
// Validation must be configured, and only the expected protocol is allowed to have credentials.
func CredentialsToApi(
	ci *cosiproto.CredentialInfo,
	validation ValidationConfig, // no pointer, validation required
) (
	allProtoCredentials map[string]string,
	err error,
) {
	errs := []error{}
	allProtoCredentials = map[string]string{}

	if ci == nil {
		return nil, fmt.Errorf("credential info is nil")
	}

	info, err := translate(protocol.S3CredentialTranslator{}, ci.S3, &validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating S3 bucket credentials: %w", err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoCredentials)
	}

	info, err = translate(protocol.AzureCredentialTranslator{}, ci.Azure, &validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating Azure bucket credentials: %w", err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoCredentials)
	}

	info, err = translate(protocol.GcsCredentialTranslator{}, ci.Gcs, &validation)
	if err != nil {
		errs = append(errs, fmt.Errorf("errors translating GCS bucket credentials: %w", err))
	} else {
		MergeApiInfoIntoStringMap(info, allProtoCredentials)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors translating access credentials: %w", errors.Join(errs...))
	}
	return allProtoCredentials, nil
}

func translate[RpcType any, ApiType comparable, T protocol.RpcApiTranslator[RpcType, ApiType]](
	translator T,
	rpcInfo RpcType,
	validation *ValidationConfig,
) (
	map[ApiType]string,
	error,
) {
	thisProto := translator.ApiProtocol()

	tv := reflect.ValueOf(translator)
	if (tv.Kind() == reflect.Interface || tv.Kind() == reflect.Ptr) && tv.IsNil() {
		return nil, fmt.Errorf("cannot translate %q protocol", thisProto)
	}

	if validation != nil && validation.ExpectedProtocol != thisProto {
		rv := reflect.ValueOf(rpcInfo)
		if rv.Kind() != reflect.Ptr {
			return nil, fmt.Errorf("RpcType must be a pointer type")
		}
		if rv.IsNil() {
			// Non-expected protocol is nil as required. Nothing to translate.
			return map[ApiType]string{}, nil
		}
		// Non-expected protocol is unexpectedly set.
		return nil, fmt.Errorf("received %q protocol response when only %q protocol is expected",
			thisProto, validation.ExpectedProtocol)
	}

	if validation != nil && reflect.ValueOf(rpcInfo).IsNil() {
		return nil, fmt.Errorf("missing response for expected %q protocol", thisProto)
	}

	out := translator.RpcToApi(rpcInfo)

	if validation != nil {
		err := translator.Validate(out, validation.AuthenticationType)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Translate COSI API authentication type to RPC authentication type.
func AuthenticationTypeToRpc(a cosiapi.BucketAccessAuthenticationType) (cosiproto.AuthenticationType_Type, error) {
	switch a {
	case cosiapi.BucketAccessAuthenticationTypeKey:
		return cosiproto.AuthenticationType_KEY, nil
	case cosiapi.BucketAccessAuthenticationTypeServiceAccount:
		return cosiproto.AuthenticationType_SERVICE_ACCOUNT, nil
	default:
		return cosiproto.AuthenticationType_UNKNOWN, fmt.Errorf("unknown authentication type %q", string(a))
	}
}

// Translate COSI API access mode to RPC access mode.
func AccessModeToRpc(m cosiapi.BucketAccessMode) (cosiproto.AccessMode_Mode, error) {
	switch m {
	case cosiapi.BucketAccessModeReadOnly:
		return cosiproto.AccessMode_READ_ONLY, nil
	case cosiapi.BucketAccessModeReadWrite:
		return cosiproto.AccessMode_READ_WRITE, nil
	case cosiapi.BucketAccessModeWriteOnly:
		return cosiproto.AccessMode_WRITE_ONLY, nil
	default:
		return cosiproto.AccessMode_UNKNOWN, fmt.Errorf("unknown access mode %q", string(m))
	}
}

func MergeApiInfoIntoStringMap[T cosiapi.BucketInfoVar | cosiapi.CredentialVar | string](
	varKey map[T]string, target map[string]string,
) {
	if target == nil {
		target = map[string]string{}
	}
	for k, v := range varKey {
		target[string(k)] = v
	}
}
