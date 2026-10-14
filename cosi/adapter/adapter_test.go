// Copyright 2026 NetApp, Inc. All Rights Reserved.

package adapter

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/cosi/provisioner"
	"github.com/netapp/trident/cosi/provisioner/fake"
)

func s3() []*cosiproto.ObjectProtocol {
	return []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_S3}}
}

// TestErrorMapping pins the sentinel-to-status-code contract. The reconcilers
// branch on these codes to decide between retrying and failing an object
// permanently, so a wrong mapping here is either an infinite retry loop or a
// transient failure reported to the user as terminal.
func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		want      codes.Code
		retryable bool
	}{
		{"not found", provisioner.ErrNotFound, codes.NotFound, true},
		{"already exists", provisioner.ErrAlreadyExists, codes.AlreadyExists, false},
		{"invalid argument", provisioner.ErrInvalidArgument, codes.InvalidArgument, false},
		{"unsupported", provisioner.ErrUnsupported, codes.Unimplemented, false},
		{"permission denied", provisioner.ErrPermissionDenied, codes.PermissionDenied, true},
		{"unauthenticated", provisioner.ErrUnauthenticated, codes.Unauthenticated, true},
		{"bucket not empty", provisioner.ErrBucketNotEmpty, codes.Aborted, true},
		// Retryable per the baseline scheme mirrored below; cosi/sidecar/bucketaccess.go
		// special-cases OutOfRange ahead of that baseline and treats it as terminal instead.
		{"multi-bucket access unsupported", provisioner.ErrMultiBucketAccessUnsupported, codes.OutOfRange, true},
		{"unclassified", errors.New("backend exploded"), codes.Internal, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Wrapped, because a backend adds context rather than returning a
			// bare sentinel.
			got := toStatus(fmt.Errorf("creating bucket: %w", tc.err))
			if status.Code(got) != tc.want {
				t.Errorf("got code %v, want %v", status.Code(got), tc.want)
			}
			if retryable := rpcErrorIsRetryable(status.Code(got)); retryable != tc.retryable {
				t.Errorf("got retryable %v, want %v", retryable, tc.retryable)
			}
		})
	}

	if toStatus(nil) != nil {
		t.Error("toStatus(nil) must be nil")
	}
}

// rpcErrorIsRetryable mirrors the vendored reconciler's baseline scheme in
// cosi/sidecar/driver.go, which is unexported. Keeping a copy here lets the
// mapping test assert the outcome a reconciler actually reaches.
func rpcErrorIsRetryable(c codes.Code) bool {
	switch c {
	case codes.InvalidArgument, codes.AlreadyExists, codes.Unimplemented:
		return false
	default:
		return true
	}
}

func TestCreateBucketIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := fake.New()
	a := New(f)

	req := &cosiproto.DriverCreateBucketRequest{
		Name:       "bucket-abc123",
		Protocols:  s3(),
		Parameters: map[string]string{"tier": "standard"},
	}

	first, err := a.DriverCreateBucket(ctx, req)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// The sidecar re-issues this call periodically. A second create must return
	// the same bucket rather than stranding another one on the backend.
	second, err := a.DriverCreateBucket(ctx, req)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}

	if first.GetBucketId() != second.GetBucketId() {
		t.Errorf("bucket ID changed across retries: %q then %q", first.GetBucketId(), second.GetBucketId())
	}
	if f.BucketCount() != 1 {
		t.Errorf("got %d buckets, want 1", f.BucketCount())
	}
	if info := first.GetProtocols().GetS3(); info == nil || info.GetEndpoint() == "" {
		t.Error("S3 bucket info missing from response")
	}
}

func TestCreateBucketConflictingParameters(t *testing.T) {
	ctx := context.Background()
	a := New(fake.New())

	base := &cosiproto.DriverCreateBucketRequest{
		Name:       "bucket-abc123",
		Protocols:  s3(),
		Parameters: map[string]string{"tier": "standard"},
	}
	if _, err := a.DriverCreateBucket(ctx, base); err != nil {
		t.Fatalf("create: %v", err)
	}

	conflicting := &cosiproto.DriverCreateBucketRequest{
		Name:       "bucket-abc123",
		Protocols:  s3(),
		Parameters: map[string]string{"tier": "archive"},
	}
	_, err := a.DriverCreateBucket(ctx, conflicting)
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("got %v, want AlreadyExists", status.Code(err))
	}
}

func TestGetBucketNotFound(t *testing.T) {
	a := New(fake.New())

	_, err := a.DriverGetBucket(context.Background(), &cosiproto.DriverGetBucketRequest{
		BucketId: "fake-bucket-missing",
	})

	// The reconciler special-cases NotFound for static provisioning, so this
	// must not arrive as a generic internal error.
	if status.Code(err) != codes.NotFound {
		t.Errorf("got %v, want NotFound", status.Code(err))
	}
}

func TestDeleteBucketIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := fake.New()
	a := New(f)

	created, err := a.DriverCreateBucket(ctx, &cosiproto.DriverCreateBucketRequest{
		Name:      "bucket-abc123",
		Protocols: s3(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req := &cosiproto.DriverDeleteBucketRequest{BucketId: created.GetBucketId()}
	for i := range 2 {
		if _, err := a.DriverDeleteBucket(ctx, req); err != nil {
			t.Fatalf("delete %d: %v", i+1, err)
		}
	}

	if f.BucketCount() != 0 {
		t.Errorf("got %d buckets, want 0", f.BucketCount())
	}
}

func TestGrantAccessReissuesSameCredentials(t *testing.T) {
	ctx := context.Background()
	f := fake.New()
	a := New(f)

	created, err := a.DriverCreateBucket(ctx, &cosiproto.DriverCreateBucketRequest{
		Name:      "bucket-abc123",
		Protocols: s3(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req := &cosiproto.DriverGrantBucketAccessRequest{
		AccountName:        "ba-xyz789",
		Protocol:           &cosiproto.ObjectProtocol{Type: cosiproto.ObjectProtocol_S3},
		AuthenticationType: &cosiproto.AuthenticationType{Type: cosiproto.AuthenticationType_KEY},
		Buckets: []*cosiproto.DriverGrantBucketAccessRequest_AccessedBucket{{
			BucketId:   created.GetBucketId(),
			AccessMode: &cosiproto.AccessMode{Mode: cosiproto.AccessMode_READ_WRITE},
		}},
	}

	first, err := a.DriverGrantBucketAccess(ctx, req)
	if err != nil {
		t.Fatalf("first grant: %v", err)
	}
	second, err := a.DriverGrantBucketAccess(ctx, req)
	if err != nil {
		t.Fatalf("second grant: %v", err)
	}

	// Rotating credentials on a retry breaks a running workload that already
	// mounted the Secret.
	if first.GetCredentials().GetS3().GetAccessKeyId() != second.GetCredentials().GetS3().GetAccessKeyId() {
		t.Error("access key changed across retries")
	}
	if first.GetAccountId() != second.GetAccountId() {
		t.Error("account ID changed across retries")
	}
	if f.AccountCount() != 1 {
		t.Errorf("got %d accounts, want 1", f.AccountCount())
	}
	if len(second.GetBuckets()) != 1 || second.GetBuckets()[0].GetBucketId() != created.GetBucketId() {
		t.Error("granted bucket list did not round-trip")
	}
}

func TestGrantAccessServiceAccountReturnsNoCredentials(t *testing.T) {
	ctx := context.Background()
	a := New(fake.New())

	resp, err := a.DriverGrantBucketAccess(ctx, &cosiproto.DriverGrantBucketAccessRequest{
		AccountName:        "ba-xyz789",
		Protocol:           &cosiproto.ObjectProtocol{Type: cosiproto.ObjectProtocol_S3},
		AuthenticationType: &cosiproto.AuthenticationType{Type: cosiproto.AuthenticationType_SERVICE_ACCOUNT},
		ServiceAccountName: "workload-sa",
	})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	if resp.GetCredentials().GetS3() != nil {
		t.Error("service account grant must not issue key credentials")
	}
}

func TestUnknownProtocolIsTerminal(t *testing.T) {
	a := New(fake.New())

	_, err := a.DriverCreateBucket(context.Background(), &cosiproto.DriverCreateBucketRequest{
		Name:      "bucket-abc123",
		Protocols: []*cosiproto.ObjectProtocol{{Type: cosiproto.ObjectProtocol_UNKNOWN}},
	})

	// Retrying an unknown protocol can never succeed, so it must not be
	// classified as retryable.
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", status.Code(err))
	}
}

func TestDriverGetInfo(t *testing.T) {
	resp, err := New(fake.New()).DriverGetInfo(context.Background(), &cosiproto.DriverGetInfoRequest{})
	if err != nil {
		t.Fatalf("driver info: %v", err)
	}

	if resp.GetName() != fake.DriverName {
		t.Errorf("got name %q, want %q", resp.GetName(), fake.DriverName)
	}
	if len(resp.GetSupportedProtocols()) != 1 ||
		resp.GetSupportedProtocols()[0].GetType() != cosiproto.ObjectProtocol_S3 {
		t.Error("supported protocols did not round-trip")
	}
}

// serverShim adapts the client-shaped Adapter to the server-shaped interface by
// dropping the grpc.CallOption variadic.
type serverShim struct {
	cosiproto.UnimplementedProvisionerServer
	a *Adapter
}

func (s *serverShim) DriverCreateBucket(
	ctx context.Context, in *cosiproto.DriverCreateBucketRequest,
) (*cosiproto.DriverCreateBucketResponse, error) {
	return s.a.DriverCreateBucket(ctx, in)
}

func (s *serverShim) DriverGetBucket(
	ctx context.Context, in *cosiproto.DriverGetBucketRequest,
) (*cosiproto.DriverGetBucketResponse, error) {
	return s.a.DriverGetBucket(ctx, in)
}

func (s *serverShim) DriverDeleteBucket(
	ctx context.Context, in *cosiproto.DriverDeleteBucketRequest,
) (*cosiproto.DriverDeleteBucketResponse, error) {
	return s.a.DriverDeleteBucket(ctx, in)
}

func (s *serverShim) DriverGrantBucketAccess(
	ctx context.Context, in *cosiproto.DriverGrantBucketAccessRequest,
) (*cosiproto.DriverGrantBucketAccessResponse, error) {
	return s.a.DriverGrantBucketAccess(ctx, in)
}

func (s *serverShim) DriverRevokeBucketAccess(
	ctx context.Context, in *cosiproto.DriverRevokeBucketAccessRequest,
) (*cosiproto.DriverRevokeBucketAccessResponse, error) {
	return s.a.DriverRevokeBucketAccess(ctx, in)
}

// TestProvisionerCanServeGRPC guards the claim that this layering keeps the
// door open to running upstream's out-of-process sidecar later: the same
// Provisioner backs the gRPC server interface with pure delegation and no
// change to any backend. If a future proto revision breaks that, this stops
// compiling, which is the point.
func TestProvisionerCanServeGRPC(t *testing.T) {
	var server cosiproto.ProvisionerServer = &serverShim{a: New(fake.New())}

	resp, err := server.DriverCreateBucket(context.Background(), &cosiproto.DriverCreateBucketRequest{
		Name:      "bucket-abc123",
		Protocols: s3(),
	})
	if err != nil {
		t.Fatalf("create over server interface: %v", err)
	}
	if resp.GetBucketId() == "" {
		t.Error("expected a bucket ID")
	}
}
