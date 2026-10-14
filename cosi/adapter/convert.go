// Copyright 2026 NetApp, Inc. All Rights Reserved.

package adapter

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/cosi/provisioner"
)

// toStatus maps a provisioner error onto the gRPC status code the reconcilers
// branch on. An error that matches no sentinel becomes codes.Internal, which
// the reconcilers treat as retryable.
func toStatus(err error) error {
	if err == nil {
		return nil
	}

	var code codes.Code
	switch {
	case errors.Is(err, provisioner.ErrNotFound):
		code = codes.NotFound
	case errors.Is(err, provisioner.ErrAlreadyExists):
		code = codes.AlreadyExists
	case errors.Is(err, provisioner.ErrInvalidArgument):
		code = codes.InvalidArgument
	case errors.Is(err, provisioner.ErrUnsupported):
		code = codes.Unimplemented
	case errors.Is(err, provisioner.ErrPermissionDenied):
		code = codes.PermissionDenied
	case errors.Is(err, provisioner.ErrUnauthenticated):
		code = codes.Unauthenticated
	case errors.Is(err, provisioner.ErrBucketNotEmpty):
		code = codes.Aborted
	case errors.Is(err, provisioner.ErrMultiBucketAccessUnsupported):
		code = codes.OutOfRange
	default:
		code = codes.Internal
	}

	return status.Error(code, err.Error())
}

func protocolFromProto(p *cosiproto.ObjectProtocol) (provisioner.Protocol, error) {
	if p == nil {
		return "", fmt.Errorf("%w: nil protocol", provisioner.ErrInvalidArgument)
	}

	switch p.GetType() {
	case cosiproto.ObjectProtocol_S3:
		return provisioner.ProtocolS3, nil
	case cosiproto.ObjectProtocol_AZURE:
		return provisioner.ProtocolAzure, nil
	case cosiproto.ObjectProtocol_GCS:
		return provisioner.ProtocolGCS, nil
	default:
		return "", fmt.Errorf("%w: unknown protocol %q", provisioner.ErrInvalidArgument, p.GetType())
	}
}

func protocolsFromProto(in []*cosiproto.ObjectProtocol) ([]provisioner.Protocol, error) {
	if len(in) == 0 {
		return nil, nil
	}

	out := make([]provisioner.Protocol, 0, len(in))
	for _, p := range in {
		converted, err := protocolFromProto(p)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func protocolToProto(p provisioner.Protocol) (*cosiproto.ObjectProtocol, error) {
	var t cosiproto.ObjectProtocol_Type
	switch p {
	case provisioner.ProtocolS3:
		t = cosiproto.ObjectProtocol_S3
	case provisioner.ProtocolAzure:
		t = cosiproto.ObjectProtocol_AZURE
	case provisioner.ProtocolGCS:
		t = cosiproto.ObjectProtocol_GCS
	default:
		return nil, fmt.Errorf("%w: unknown protocol %q", provisioner.ErrInvalidArgument, p)
	}
	return &cosiproto.ObjectProtocol{Type: t}, nil
}

func protocolsToProto(in []provisioner.Protocol) ([]*cosiproto.ObjectProtocol, error) {
	out := make([]*cosiproto.ObjectProtocol, 0, len(in))
	for _, p := range in {
		converted, err := protocolToProto(p)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func authTypeFromProto(a *cosiproto.AuthenticationType) (provisioner.AuthenticationType, error) {
	if a == nil {
		return "", fmt.Errorf("%w: nil authentication type", provisioner.ErrInvalidArgument)
	}

	switch a.GetType() {
	case cosiproto.AuthenticationType_KEY:
		return provisioner.AuthenticationTypeKey, nil
	case cosiproto.AuthenticationType_SERVICE_ACCOUNT:
		return provisioner.AuthenticationTypeServiceAccount, nil
	default:
		return "", fmt.Errorf("%w: unknown authentication type %q", provisioner.ErrInvalidArgument, a.GetType())
	}
}

func accessModeFromProto(m *cosiproto.AccessMode) (provisioner.AccessMode, error) {
	if m == nil {
		return "", fmt.Errorf("%w: nil access mode", provisioner.ErrInvalidArgument)
	}

	switch m.GetMode() {
	case cosiproto.AccessMode_READ_WRITE:
		return provisioner.AccessModeReadWrite, nil
	case cosiproto.AccessMode_READ_ONLY:
		return provisioner.AccessModeReadOnly, nil
	case cosiproto.AccessMode_WRITE_ONLY:
		return provisioner.AccessModeWriteOnly, nil
	default:
		return "", fmt.Errorf("%w: unknown access mode %q", provisioner.ErrInvalidArgument, m.GetMode())
	}
}

func addressingStyleToProto(s provisioner.S3AddressingStyle) *cosiproto.S3AddressingStyle {
	style := cosiproto.S3AddressingStyle_UNKNOWN
	switch s {
	case provisioner.S3AddressingStylePath:
		style = cosiproto.S3AddressingStyle_PATH
	case provisioner.S3AddressingStyleVirtual:
		style = cosiproto.S3AddressingStyle_VIRTUAL
	}
	return &cosiproto.S3AddressingStyle{Style: style}
}

// bucketInfoToProto converts connection details. A nil field means the protocol
// is unsupported for the bucket, which the proto represents the same way, so
// nil is passed through rather than defaulted.
func bucketInfoToProto(info provisioner.BucketInfo) *cosiproto.ObjectProtocolAndBucketInfo {
	out := &cosiproto.ObjectProtocolAndBucketInfo{}

	if s3 := info.S3; s3 != nil {
		out.S3 = &cosiproto.S3BucketInfo{
			BucketId:        s3.BucketID,
			Endpoint:        s3.Endpoint,
			Region:          s3.Region,
			AddressingStyle: addressingStyleToProto(s3.AddressingStyle),
		}
	}
	if azure := info.Azure; azure != nil {
		out.Azure = &cosiproto.AzureBucketInfo{StorageAccount: azure.StorageAccount}
	}
	if gcs := info.GCS; gcs != nil {
		out.Gcs = &cosiproto.GcsBucketInfo{
			ProjectId:  gcs.ProjectID,
			BucketName: gcs.BucketName,
		}
	}

	return out
}

func credentialsToProto(creds provisioner.Credentials) *cosiproto.CredentialInfo {
	out := &cosiproto.CredentialInfo{}

	if s3 := creds.S3; s3 != nil {
		out.S3 = &cosiproto.S3CredentialInfo{
			AccessKeyId:     s3.AccessKeyID,
			AccessSecretKey: s3.AccessSecretKey,
		}
	}
	if azure := creds.Azure; azure != nil {
		out.Azure = &cosiproto.AzureCredentialInfo{
			AccessToken:     azure.AccessToken,
			ExpiryTimestamp: azure.ExpiryTimestamp,
		}
	}
	if gcs := creds.GCS; gcs != nil {
		out.Gcs = &cosiproto.GcsCredentialInfo{
			AccessId:       gcs.AccessID,
			AccessSecret:   gcs.AccessSecret,
			PrivateKeyName: gcs.PrivateKeyName,
			ServiceAccount: gcs.ServiceAccount,
		}
	}

	return out
}
