// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// rawCodec is a minimal grpc.Codec that round-trips raw byte slices, so the
// fake server can accept and reply to any method without proto codegen. We
// override the registered "proto" codec for the call by using grpc.CallContentSubtype.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return x, nil
	case *[]byte:
		return *x, nil
	default:
		// Fail fast on unexpected types so future changes to the test invocation
		// surface immediately instead of silently encoding empty payloads.
		return nil, fmt.Errorf("rawCodec.Marshal: unsupported type %T", v)
	}
}

func (rawCodec) Unmarshal(data []byte, v any) error {
	bp, ok := v.(*[]byte)
	if !ok {
		return fmt.Errorf("rawCodec.Unmarshal: unsupported type %T", v)
	}
	*bp = data
	return nil
}

func (rawCodec) Name() string { return "raw" }

func init() {
	encoding.RegisterCodec(rawCodec{})
}

// fakeUnaryHandler wraps a counter and returns ResourceExhausted for the first
// failUntil calls, then OK. Used as an UnknownServiceHandler.
type fakeUnaryHandler struct {
	failUntil int32
	calls     int32
}

func (h *fakeUnaryHandler) handle(_ any, stream grpc.ServerStream) error {
	n := atomic.AddInt32(&h.calls, 1)

	var raw []byte
	if err := stream.RecvMsg(&raw); err != nil {
		return err
	}

	if n <= h.failUntil {
		return status.Error(codes.ResourceExhausted, "fake quota exhausted")
	}
	return stream.SendMsg([]byte{})
}

// startFakeGRPCServer starts a bufconn-backed gRPC server with the given handler
// installed as the UnknownServiceHandler. Returns a dial function and a cleanup.
func startFakeGRPCServer(t *testing.T, h *fakeUnaryHandler) (dial func(...grpc.DialOption) *grpc.ClientConn, stop func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnknownServiceHandler(h.handle))
	go func() { _ = srv.Serve(lis) }()

	dial = func(extra ...grpc.DialOption) *grpc.ClientConn {
		opts := []grpc.DialOption{
			grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
				return lis.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
		opts = append(opts, extra...)
		// NewClient doesn't need a context; address is unused with the dialer.
		cc, err := grpc.NewClient("passthrough:///bufnet", opts...)
		require.NoError(t, err)
		return cc
	}
	stop = func() {
		srv.GracefulStop()
		_ = lis.Close()
	}
	return dial, stop
}

func invokeFake(ctx context.Context, cc *grpc.ClientConn) error {
	var resp []byte
	return cc.Invoke(ctx, "/fake.Service/Echo", []byte{}, &resp, grpc.CallContentSubtype("raw"))
}

// TestInterceptor_DecreasesOnceForBurstOf429s verifies the end-to-end gRPC path:
// when the upstream returns ResourceExhausted on a burst of concurrent calls,
// the interceptor halves the limit exactly once.
func TestInterceptor_DecreasesOnceForBurstOf429s(t *testing.T) {
	const burst = 25
	h := &fakeUnaryHandler{failUntil: int32(burst)}
	dial, stop := startFakeGRPCServer(t, h)
	defer stop()

	cfg := LimiterConfig{
		Ceiling:          rate.Limit(1000),
		Floor:            rate.Limit(1),
		Burst:            1000,
		DecreaseFactor:   0.5,
		DecreaseDebounce: time.Hour,
		ProbeInterval:    time.Hour,
		ProbeStep:        rate.Limit(1),
	}
	limiter := NewAdaptiveRateLimiter("integration-test", cfg, nil)

	cc := dial(grpc.WithUnaryInterceptor(limiter.UnaryInterceptor()))
	defer cc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, burst)
	for i := 0; i < burst; i++ {
		go func() { errCh <- invokeFake(ctx, cc) }()
	}
	for i := 0; i < burst; i++ {
		err := <-errCh
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	}

	dec, _ := limiter.Stats()
	assert.Equal(t, uint64(1), dec, "%d concurrent ResourceExhausted responses must collapse into exactly one decrease", burst)
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), limiter.Limit(),
		"limit must be halved exactly once after the burst")
}

// TestInterceptor_RecoversAfterUpstreamHeals verifies a full storm-then-heal cycle:
// initial 429s halve the limit, subsequent successes pace through but eventually
// climb back up after ProbeInterval ticks.
func TestInterceptor_RecoversAfterUpstreamHeals(t *testing.T) {
	h := &fakeUnaryHandler{failUntil: 5}
	dial, stop := startFakeGRPCServer(t, h)
	defer stop()

	cfg := LimiterConfig{
		Ceiling:          rate.Limit(500),
		Floor:            rate.Limit(1),
		Burst:            500,
		DecreaseFactor:   0.5,
		DecreaseDebounce: 10 * time.Millisecond,
		ProbeInterval:    1 * time.Millisecond, // intentionally tight so the test runs in <1s
		ProbeStep:        rate.Limit(50),
	}
	limiter := NewAdaptiveRateLimiter("integration-recovery", cfg, nil)

	cc := dial(grpc.WithUnaryInterceptor(limiter.UnaryInterceptor()))
	defer cc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		err := invokeFake(ctx, cc)
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	}

	dec, _ := limiter.Stats()
	assert.GreaterOrEqual(t, dec, uint64(1), "at least one decrease should have occurred")
	assert.Less(t, float64(limiter.Limit()), float64(cfg.Ceiling),
		"limit should be below ceiling after upstream throttling")

	for i := 0; i < 200; i++ {
		err := invokeFake(ctx, cc)
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
	}

	_, inc := limiter.Stats()
	assert.Greater(t, inc, uint64(0), "limit should have additively increased at least once after sustained success")
}
