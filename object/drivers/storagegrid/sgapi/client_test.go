// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testAccountID = "12345678901234567000"
	testToken     = "00000000-0000-0000-0000-000000000000"
	testBucket    = "bucket-4f2c1e88"
)

// grid is a stand-in for a StorageGRID admin node. Handlers are registered per
// path and receive the call index, so a test can make the same endpoint behave
// differently on successive attempts.
type grid struct {
	t *testing.T

	mu       sync.Mutex
	calls    map[string]int
	handlers map[string]func(w http.ResponseWriter, r *http.Request, call int)

	server *httptest.Server
}

func newGrid(t *testing.T) *grid {
	t.Helper()

	g := &grid{
		t:        t,
		calls:    make(map[string]int),
		handlers: make(map[string]func(http.ResponseWriter, *http.Request, int)),
	}
	g.server = httptest.NewServer(http.HandlerFunc(g.route))
	t.Cleanup(g.server.Close)

	// Every test needs a working login unless it overrides this.
	g.handle(http.MethodPost, "/api/v4/authorize", func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Cache-Control", "private, no-cache, max-age=57599")
		writeEnvelope(w, http.StatusOK, testToken, nil)
	})
	return g
}

func (g *grid) handle(method, path string, h func(http.ResponseWriter, *http.Request, int)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handlers[method+" "+path] = h
}

// status registers a handler that always answers with a bare status and an error
// envelope carrying key.
func (g *grid) status(method, path string, code int, key string) {
	g.handle(method, path, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeError(w, code, key)
	})
}

func (g *grid) route(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path

	g.mu.Lock()
	handler, ok := g.handlers[key]
	g.calls[key]++
	call := g.calls[key]
	g.mu.Unlock()

	if !ok {
		g.t.Errorf("unexpected request %s", key)
		writeError(w, http.StatusNotImplemented, "test.unhandled")
		return
	}
	handler(w, r, call)
}

func (g *grid) callCount(method, path string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[method+" "+path]
}

func (g *grid) client(t *testing.T) *Client {
	t.Helper()

	client, err := New(Config{
		Endpoint:  g.server.URL,
		AccountID: testAccountID,
		Username:  "root",
		Password:  "secret",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func writeEnvelope(w http.ResponseWriter, code int, data any, metadata *Metadata) {
	body := map[string]any{"status": "success", "apiVersion": "4.0"}
	if data != nil {
		body["data"] = data
	}
	if metadata != nil {
		body["metadata"] = metadata
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, key string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "error",
		"apiVersion": "4.0",
		"code":       code,
		"message": map[string]string{
			"text":    "something went wrong",
			"key":     key,
			"context": "bucketName",
		},
	})
}

func bucketPath(suffix string) string {
	path := "/api/v4/org/containers/" + testBucket
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}

// The account ID is 20 digits, which overflows a 64-bit integer. If it is ever
// serialized as a number it is silently rounded and the grid answers 401, so the
// wire format is worth asserting on directly.
func TestAuthorizeSendsAccountIDAsJSONString(t *testing.T) {
	g := newGrid(t)

	var raw []byte
	g.handle(http.MethodPost, "/api/v4/authorize", func(w http.ResponseWriter, r *http.Request, _ int) {
		raw, _ = io.ReadAll(r.Body)
		writeEnvelope(w, http.StatusOK, testToken, nil)
	})

	token, err := g.client(t).Authorize(context.Background())
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if token != testToken {
		t.Errorf("token = %q, want %q", token, testToken)
	}

	if want := `"accountId":"` + testAccountID + `"`; !strings.Contains(string(raw), want) {
		t.Errorf("request body %s does not contain %s", raw, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshalling request body: %v", err)
	}
	if _, ok := decoded["accountId"].(string); !ok {
		t.Errorf("accountId decoded as %T, want string", decoded["accountId"])
	}
}

func TestAuthorizeRequiresAccountID(t *testing.T) {
	client, err := New(Config{Endpoint: "https://grid.example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Without an account ID the grid would authenticate against the grid admin
	// realm and answer an indistinguishable 401, so this must fail locally.
	if _, err := client.Authorize(context.Background()); !IsValidationError(err) {
		t.Fatalf("Authorize error = %v, want a validation error", err)
	}
}

func TestAuthorizeBadRequestReportsSingleSignOn(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodPost, "/api/v4/authorize", http.StatusBadRequest, "auth.sso_enabled")

	_, err := g.client(t).Authorize(context.Background())
	if err == nil {
		t.Fatal("Authorize succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "single sign-on") {
		t.Errorf("error %q does not mention single sign-on", err)
	}
}

func TestParseTokenExpiry(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		header http.Header
		want   time.Time
	}{
		{
			name:   "max-age wins",
			header: http.Header{"Cache-Control": {"private, no-cache, max-age=57599"}},
			want:   now.Add(57599 * time.Second),
		},
		{
			name:   "expires is the fallback",
			header: http.Header{"Expires": {"Thu, 13 Aug 2026 20:00:00 GMT"}},
			want:   time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
		},
		{
			name:   "max-age is preferred over expires",
			header: http.Header{"Cache-Control": {"max-age=60"}, "Expires": {"Thu, 13 Aug 2026 20:00:00 GMT"}},
			want:   now.Add(60 * time.Second),
		},
		{
			name:   "no expiry information",
			header: http.Header{"Cache-Control": {"private, no-cache"}},
			want:   time.Time{},
		},
		{
			name:   "unparseable max-age is ignored",
			header: http.Header{"Cache-Control": {"max-age=soon"}},
			want:   time.Time{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTokenExpiry(tc.header, now)
			if !got.Equal(tc.want) {
				t.Errorf("parseTokenExpiry = %v, want %v", got, tc.want)
			}
		})
	}
}

// A 401 on a normal call is treated as an expired token: re-authorize once and
// replay.
func TestUnauthorizedReauthorizesOnceAndReplays(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodGet, bucketPath("region"), func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 1 {
			writeError(w, http.StatusUnauthorized, "auth.token_expired")
			return
		}
		writeEnvelope(w, http.StatusOK, map[string]string{"region": "us-east-1"}, nil)
	})

	region, err := g.client(t).GetRegion(context.Background(), testBucket)
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if region != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", region)
	}
	if got := g.callCount(http.MethodPost, "/api/v4/authorize"); got != 2 {
		t.Errorf("authorize called %d times, want 2", got)
	}
	if got := g.callCount(http.MethodGet, bucketPath("region")); got != 2 {
		t.Errorf("region read %d times, want 2", got)
	}
}

// A second 401 means bad credentials rather than an expired token, so it must not
// loop.
func TestRepeatedUnauthorizedIsTerminal(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodGet, bucketPath("region"), http.StatusUnauthorized, "auth.credentials_invalid")

	_, err := g.client(t).GetRegion(context.Background(), testBucket)
	if !IsUnauthorized(err) {
		t.Fatalf("GetRegion error = %v, want an unauthorized error", err)
	}
	if got := g.callCount(http.MethodPost, "/api/v4/authorize"); got != 2 {
		t.Errorf("authorize called %d times, want exactly 2", got)
	}
	if got := g.callCount(http.MethodGet, bucketPath("region")); got != 2 {
		t.Errorf("region read %d times, want exactly 2", got)
	}
}

// A caller-supplied token cannot be refreshed, so a 401 must be terminal without
// any attempt to authorize.
func TestStaticTokenIsNotRefreshed(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodGet, bucketPath("region"), http.StatusUnauthorized, "auth.token_expired")

	client, err := New(Config{Endpoint: g.server.URL, Token: testToken})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.GetRegion(context.Background(), testBucket); !IsUnauthorized(err) {
		t.Fatalf("GetRegion error = %v, want an unauthorized error", err)
	}
	if got := g.callCount(http.MethodPost, "/api/v4/authorize"); got != 0 {
		t.Errorf("authorize called %d times, want 0", got)
	}
}

func TestCreateBucketReportsGridResolvedRegion(t *testing.T) {
	g := newGrid(t)

	var raw []byte
	g.handle(http.MethodPost, "/api/v4/org/containers", func(w http.ResponseWriter, r *http.Request, _ int) {
		raw, _ = io.ReadAll(r.Body)
		writeEnvelope(w, http.StatusCreated, map[string]any{
			"name":   testBucket,
			"region": "us-east-1",
		}, nil)
	})

	result, err := g.client(t).CreateBucket(context.Background(), ContainerCreate{Name: testBucket})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if result.Region != "us-east-1" || !result.RegionResolved {
		t.Errorf("region = %q resolved = %t, want us-east-1 resolved", result.Region, result.RegionResolved)
	}
	// The request omitted the region, so it must not appear on the wire.
	if strings.Contains(string(raw), "region") {
		t.Errorf("request body %s should not carry a region", raw)
	}
}

func TestCreateBucketWithoutResolvedRegion(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodPost, "/api/v4/org/containers", func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusCreated, map[string]any{"name": testBucket}, nil)
	})

	result, err := g.client(t).CreateBucket(context.Background(), ContainerCreate{Name: testBucket})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if result.RegionResolved {
		t.Error("RegionResolved is true, want false when the grid reported no region")
	}
}

// A 201 is not unconditionally a success: the bucket can be created while part of
// its configuration fails, and that is reported only through the alerts.
func TestCreateBucketErrorAlertIsAFailure(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodPost, "/api/v4/org/containers", func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusCreated,
			map[string]any{"name": testBucket, "region": "us-east-1"},
			&Metadata{Alerts: []Alert{{
				Severity: AlertSeverityError,
				Key:      "containers.container_create_object_lock_error",
				Text:     "default retention configuration failed",
			}}})
	})

	result, err := g.client(t).CreateBucket(context.Background(), ContainerCreate{Name: testBucket})
	if !IsAlertError(err) {
		t.Fatalf("CreateBucket error = %v, want an alert error", err)
	}
	// The result is still returned so that a retry can reconcile.
	if result == nil || len(result.Alerts) != 1 {
		t.Fatalf("result = %+v, want the alert to be reported", result)
	}
}

func TestCreateBucketNonErrorAlertIsNotAFailure(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodPost, "/api/v4/org/containers", func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusCreated,
			map[string]any{"name": testBucket},
			&Metadata{Alerts: []Alert{{Severity: "warning", Text: "heads up"}}})
	})

	result, err := g.client(t).CreateBucket(context.Background(), ContainerCreate{Name: testBucket})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if len(result.Alerts) != 1 {
		t.Errorf("alerts = %v, want the warning to be reported", result.Alerts)
	}
}

func TestCreateBucketValidatesLocally(t *testing.T) {
	g := newGrid(t)
	client := g.client(t)

	// Neither call may reach the grid, which the unhandled-route check enforces.
	if _, err := client.CreateBucket(context.Background(), ContainerCreate{Name: "no"}); !IsValidationError(err) {
		t.Errorf("short name error = %v, want a validation error", err)
	}
	if _, err := client.CreateBucket(context.Background(),
		ContainerCreate{Name: testBucket, Region: "not a region"}); !IsValidationError(err) {
		t.Errorf("bad region error = %v, want a validation error", err)
	}
}

func TestDeleteBucketOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		want    DeleteOutcome
		wantErr func(error) bool
	}{
		// Delete must be idempotent, so a bucket that is already gone is a
		// success rather than an error.
		{name: "deleted", status: http.StatusNoContent, want: BucketDeleted},
		{name: "already gone", status: http.StatusNotFound, want: BucketAlreadyGone},
		{name: "not empty", status: http.StatusConflict, want: BucketNotEmpty},
		{name: "forbidden", status: http.StatusForbidden, wantErr: IsForbidden},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantErr: IsRetryable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGrid(t)
			g.handle(http.MethodDelete, bucketPath(""), func(w http.ResponseWriter, _ *http.Request, _ int) {
				if tc.status == http.StatusNoContent {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				writeError(w, tc.status, "containers.delete")
			})

			outcome, err := g.client(t).DeleteBucket(context.Background(), testBucket)
			if tc.wantErr != nil {
				if !tc.wantErr(err) {
					t.Fatalf("error = %v, which is not the expected class", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DeleteBucket: %v", err)
			}
			if outcome != tc.want {
				t.Errorf("outcome = %v, want %v", outcome, tc.want)
			}
		})
	}
}

// Without the opt-in, a bucket holding objects is a failed precondition: emptying
// destroys data that was never explicitly nominated for destruction.
func TestDeleteStepNotEmptyWithoutOptIn(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodDelete, bucketPath(""), http.StatusConflict, "containers.not_empty")

	client := g.client(t)
	_, err := client.NewBucketDeleter(testBucket, false).Step(context.Background())
	if !errors.Is(err, ErrBucketNotEmpty) {
		t.Fatalf("error = %v, want ErrBucketNotEmpty", err)
	}
	// The emptying endpoints must not have been touched.
	if got := g.callCount(http.MethodPost, bucketPath("delete-objects")); got != 0 {
		t.Errorf("delete-objects posted %d times, want 0", got)
	}
}

// With the opt-in, each pass advances the state machine by one step: the first
// starts the emptying, the second finds it running, and the third deletes the
// now-empty bucket.
func TestDeleteStepEmptyingProgression(t *testing.T) {
	g := newGrid(t)

	deletes := 0
	g.handle(http.MethodDelete, bucketPath(""), func(w http.ResponseWriter, _ *http.Request, call int) {
		deletes = call
		if call < 3 {
			writeError(w, http.StatusConflict, "containers.not_empty")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	g.handle(http.MethodGet, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, call int) {
		writeEnvelope(w, http.StatusOK, map[string]any{
			"isDeletingObjects":  call > 1,
			"initialObjectCount": 42,
		}, nil)
	})
	g.handle(http.MethodPost, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"isDeletingObjects": true}, nil)
	})

	deleter := g.client(t).NewBucketDeleter(testBucket, true)
	ctx := context.Background()

	first, err := deleter.Step(ctx)
	if err != nil {
		t.Fatalf("first step: %v", err)
	}
	if first.Done || !first.Retryable {
		t.Errorf("first step = %+v, want a retryable step", first)
	}
	if got := g.callCount(http.MethodPost, bucketPath("delete-objects")); got != 1 {
		t.Errorf("emptying started %d times, want 1", got)
	}

	second, err := deleter.Step(ctx)
	if err != nil {
		t.Fatalf("second step: %v", err)
	}
	if second.Done || !second.Retryable {
		t.Errorf("second step = %+v, want a retryable step", second)
	}
	// Emptying is already running, so it must not be kicked off again.
	if got := g.callCount(http.MethodPost, bucketPath("delete-objects")); got != 1 {
		t.Errorf("emptying started %d times, want it left alone", got)
	}

	third, err := deleter.Step(ctx)
	if err != nil {
		t.Fatalf("third step: %v", err)
	}
	if !third.Done || third.Outcome != BucketDeleted {
		t.Errorf("third step = %+v, want a completed delete", third)
	}
	if deletes != 3 {
		t.Errorf("delete attempted %d times, want 3", deletes)
	}
}

// If emptying has finished and the delete still reports a non-empty bucket,
// retrying will never succeed, so the deleter must stop.
func TestDeleteStepStopsWhenObjectsRemain(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodDelete, bucketPath(""), http.StatusConflict, "containers.not_empty")
	g.handle(http.MethodGet, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"isDeletingObjects": false}, nil)
	})
	g.handle(http.MethodPost, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"isDeletingObjects": true}, nil)
	})
	g.handle(http.MethodGet, bucketPath("object-lock"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"enabled": true}, nil)
	})

	deleter := g.client(t).NewBucketDeleter(testBucket, true)
	ctx := context.Background()

	if _, err := deleter.Step(ctx); err != nil {
		t.Fatalf("first step: %v", err)
	}

	_, err := deleter.Step(ctx)
	if !errors.Is(err, ErrObjectsRemain) {
		t.Fatalf("error = %v, want ErrObjectsRemain", err)
	}
	// Object Lock is the most useful explanation available, so it is named.
	if !strings.Contains(err.Error(), "Object Lock") {
		t.Errorf("error %q does not mention Object Lock", err)
	}
}

// The emptying operation reports partial failures through alerts rather than the
// status code.
func TestStartDeleteObjectsErrorAlertIsAFailure(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodPost, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"isDeletingObjects": false},
			&Metadata{Alerts: []Alert{{
				Severity: AlertSeverityError,
				Key:      "containers.empty_bucket_ldr_partial_failure",
				Text:     "unable to delete objects from the bucket",
			}}})
	})

	_, _, err := g.client(t).StartDeleteObjects(context.Background(), testBucket)
	if !IsAlertError(err) {
		t.Fatalf("error = %v, want an alert error", err)
	}
}

func TestGetBucketReadsRegionFirstAndSkipsUnrequested(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodGet, bucketPath("region"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]string{"region": "us-east-1"}, nil)
	})

	// Only the region is registered, so any other read would fail the test as an
	// unexpected request.
	state, err := g.client(t).GetBucket(context.Background(), testBucket, BucketReads{})
	if err != nil {
		t.Fatalf("GetBucket: %v", err)
	}
	if state.Region != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", state.Region)
	}
	if state.Versioning != nil || state.ObjectLock != nil {
		t.Errorf("state = %+v, want no optional properties", state)
	}
}

func TestGetBucketAllReads(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodGet, bucketPath("region"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]string{"region": "us-east-1"}, nil)
	})
	g.handle(http.MethodGet, bucketPath("versioning"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"versioningEnabled": true}, nil)
	})
	g.handle(http.MethodGet, bucketPath("quota-object-bytes"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"quotaObjectBytes": 100000000000}, nil)
	})
	g.handle(http.MethodGet, bucketPath("quota-object-count"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"quotaObjectCount": nil}, nil)
	})
	g.handle(http.MethodGet, bucketPath("object-lock"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"enabled": false}, nil)
	})
	g.handle(http.MethodGet, bucketPath("consistency"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"consistency": "read-after-new-write"}, nil)
	})

	state, err := g.client(t).GetBucket(context.Background(), testBucket, AllBucketReads())
	if err != nil {
		t.Fatalf("GetBucket: %v", err)
	}
	if state.Versioning == nil || !state.Versioning.VersioningEnabled {
		t.Errorf("versioning = %+v, want enabled", state.Versioning)
	}
	if state.QuotaObjectBytes == nil || *state.QuotaObjectBytes != 100000000000 {
		t.Errorf("quota bytes = %v, want 100000000000", state.QuotaObjectBytes)
	}
	// A nil quota is meaningful: it means no quota is set.
	if state.QuotaObjectCount != nil {
		t.Errorf("quota count = %v, want nil", *state.QuotaObjectCount)
	}
	if state.Consistency != "read-after-new-write" {
		t.Errorf("consistency = %q, want read-after-new-write", state.Consistency)
	}
}

// A bucket this tenant does not own cannot be read, which is what makes the
// region read the existence check.
func TestGetBucketNotFound(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodGet, bucketPath("region"), http.StatusNotFound, "containers.not_found")

	_, err := g.client(t).GetBucket(context.Background(), testBucket, AllBucketReads())
	if !IsNotFound(err) {
		t.Fatalf("error = %v, want a not-found error", err)
	}
}

func TestSetQuotaNilClearsIt(t *testing.T) {
	g := newGrid(t)

	var raw []byte
	g.handle(http.MethodPut, bucketPath("quota-object-bytes"), func(w http.ResponseWriter, r *http.Request, _ int) {
		raw, _ = io.ReadAll(r.Body)
		writeEnvelope(w, http.StatusOK, map[string]any{"quotaObjectBytes": nil}, nil)
	})

	if err := g.client(t).SetQuotaObjectBytes(context.Background(), testBucket, nil); err != nil {
		t.Fatalf("SetQuotaObjectBytes: %v", err)
	}
	if want := `"quotaObjectBytes":null`; !strings.Contains(string(raw), want) {
		t.Errorf("request body %s does not contain %s", raw, want)
	}
}

func TestVersionsAcceptsBothShapes(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{name: "bare array", data: []int{2, 3, 4}},
		{name: "wrapped", data: map[string]any{"versions": []int{2, 3, 4}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGrid(t)
			g.handle(http.MethodGet, "/api/v4/versions", func(w http.ResponseWriter, _ *http.Request, _ int) {
				writeEnvelope(w, http.StatusOK, tc.data, nil)
			})

			versions, err := g.client(t).Versions(context.Background())
			if err != nil {
				t.Fatalf("Versions: %v", err)
			}
			if len(versions) != 3 || versions[2] != 4 {
				t.Errorf("versions = %v, want [2 3 4]", versions)
			}
			// The check is unauthenticated, which is what isolates connectivity
			// problems from credential problems.
			if got := g.callCount(http.MethodPost, "/api/v4/authorize"); got != 0 {
				t.Errorf("authorize called %d times, want 0", got)
			}
		})
	}
}

// Bucket names arrive from COSI as DNS subdomains and can contain characters that
// must not be read as path structure.
func TestBucketPathsAreEscaped(t *testing.T) {
	g := newGrid(t)

	// The router keys on the decoded path, since that is what the server sees
	// once the escaping has been undone.
	var gotPath string
	g.handle(http.MethodDelete, "/api/v4/org/containers/odd/name", func(w http.ResponseWriter, r *http.Request, _ int) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	})

	// The name is invalid for a bucket, so exercise the escaping directly.
	if got := escapePath("org", "containers", "odd/name"); got != "/org/containers/odd%2Fname" {
		t.Fatalf("escapePath = %q, want the slash escaped", got)
	}

	client, err := New(Config{Endpoint: g.server.URL, Token: testToken})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.do(context.Background(), request{
		method: http.MethodDelete,
		path:   escapePath("org", "containers", "odd/name"),
	}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotPath != "/api/v4/org/containers/odd%2Fname" {
		t.Errorf("server saw %q, want the escaped path", gotPath)
	}
}

// Message keys are the only stable discriminator, since the text is localised.
func TestAPIErrorSurfacesMessageKey(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodGet, bucketPath("region"), http.StatusConflict, "containers.name_taken")

	_, err := g.client(t).GetRegion(context.Background(), testBucket)
	if got := MessageKeyOf(err); got != "containers.name_taken" {
		t.Errorf("message key = %q, want containers.name_taken", got)
	}
	if got := StatusOf(err); got != http.StatusConflict {
		t.Errorf("status = %d, want %d", got, http.StatusConflict)
	}
	if !strings.Contains(err.Error(), "containers.name_taken") {
		t.Errorf("error %q does not include the message key", err)
	}
}

// An unrecognised failure must still be inspectable, since the error catalogue is
// not documented.
func TestAPIErrorRetainsUnparseableBody(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodGet, bucketPath("region"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>gateway error</html>"))
	})

	_, err := g.client(t).GetRegion(context.Background(), testBucket)
	if !IsRetryable(err) {
		t.Fatalf("error = %v, want a retryable error", err)
	}
	if !strings.Contains(err.Error(), "gateway error") {
		t.Errorf("error %q does not include the raw body", err)
	}
}

func TestRedactHidesCredentials(t *testing.T) {
	in := []byte(`{"accountId":"1","username":"root","password":"hunter2","secretAccessKey":"abc"}`)
	got := string(redact(in))

	for _, secret := range []string{"hunter2", "abc"} {
		if strings.Contains(got, secret) {
			t.Errorf("redact left %q in %s", secret, got)
		}
	}
	if !strings.Contains(got, `"username":"root"`) {
		t.Errorf("redact removed non-secret fields: %s", got)
	}
}

func TestDebugTraceRedactsThePassword(t *testing.T) {
	g := newGrid(t)

	var trace strings.Builder
	client, err := New(Config{
		Endpoint:  g.server.URL,
		AccountID: testAccountID,
		Password:  "hunter2",
		Debug:     &trace,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Authorize(context.Background()); err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if strings.Contains(trace.String(), "hunter2") {
		t.Errorf("trace leaked the password: %s", trace.String())
	}
	if !strings.Contains(trace.String(), "POST /api/v4/authorize") {
		t.Errorf("trace does not record the request: %s", trace.String())
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "no endpoint", cfg: Config{}},
		{name: "no host", cfg: Config{Endpoint: "https://"}},
		{name: "bad ca cert", cfg: Config{Endpoint: "https://grid.example.com", CACert: []byte("not a pem")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); !IsValidationError(err) {
				t.Errorf("New error = %v, want a validation error", err)
			}
		})
	}
}

func TestNewDefaultsToHTTPS(t *testing.T) {
	client, err := New(Config{Endpoint: "admin.example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := client.baseURL.String(); got != "https://admin.example.com" {
		t.Errorf("base URL = %q, want https://admin.example.com", got)
	}
}

func TestReleaseTokenIsANoOpWithoutASession(t *testing.T) {
	g := newGrid(t)
	if err := g.client(t).ReleaseToken(context.Background()); err != nil {
		t.Errorf("ReleaseToken: %v", err)
	}
	if got := g.callCount(http.MethodDelete, "/api/v4/authorize"); got != 0 {
		t.Errorf("authorize deleted %d times, want 0", got)
	}
}

func TestReleaseToken(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodDelete, "/api/v4/authorize", func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := g.client(t)
	if _, err := client.Authorize(context.Background()); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if err := client.ReleaseToken(context.Background()); err != nil {
		t.Fatalf("ReleaseToken: %v", err)
	}
	if got := g.callCount(http.MethodDelete, "/api/v4/authorize"); got != 1 {
		t.Errorf("authorize deleted %d times, want 1", got)
	}
}

// The cached token is reused rather than re-fetched for every call.
func TestTokenIsCachedAcrossCalls(t *testing.T) {
	g := newGrid(t)
	g.handle(http.MethodGet, bucketPath("region"), func(w http.ResponseWriter, r *http.Request, _ int) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization = %q, want the bearer token", got)
		}
		writeEnvelope(w, http.StatusOK, map[string]string{"region": "us-east-1"}, nil)
	})

	client := g.client(t)
	for i := 0; i < 3; i++ {
		if _, err := client.GetRegion(context.Background(), testBucket); err != nil {
			t.Fatalf("GetRegion %d: %v", i, err)
		}
	}
	if got := g.callCount(http.MethodPost, "/api/v4/authorize"); got != 1 {
		t.Errorf("authorize called %d times, want 1", got)
	}
}

func TestListBucketsSendsIncludeAsCSV(t *testing.T) {
	g := newGrid(t)

	var query string
	g.handle(http.MethodGet, "/api/v4/org/containers", func(w http.ResponseWriter, r *http.Request, _ int) {
		query = r.URL.Query().Get("include")
		writeEnvelope(w, http.StatusOK, []map[string]any{{"name": testBucket, "region": "us-east-1"}}, nil)
	})

	buckets, err := g.client(t).ListBuckets(context.Background(), []string{"region", "versioning"})
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if query != "region,versioning" {
		t.Errorf("include = %q, want region,versioning", query)
	}
	if len(buckets) != 1 || buckets[0].Name != testBucket {
		t.Errorf("buckets = %+v, want one entry for %s", buckets, testBucket)
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	g := newGrid(t)
	g.status(http.MethodDelete, bucketPath(""), http.StatusConflict, "containers.not_empty")
	g.handle(http.MethodGet, bucketPath("delete-objects"), func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeEnvelope(w, http.StatusOK, map[string]any{"isDeletingObjects": true}, nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	deleter := g.client(t).NewBucketDeleter(testBucket, true)

	_, err := deleter.Run(ctx, time.Millisecond, func(*StepStatus) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}

func TestValidateBucketName(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
	}{
		{name: "typical COSI generated name", bucket: "bucket-4f2c1e88-9a3b-4d21-bd77-0f1a2c3d4e5f"},
		{name: "minimum length", bucket: "abc"},
		{name: "dotted", bucket: "my.bucket.name"},
		{name: "empty", bucket: "", wantErr: true},
		{name: "too short", bucket: "ab", wantErr: true},
		{name: "too long", bucket: strings.Repeat("a", MaxBucketNameLength+1), wantErr: true},
		{name: "COSI maximum overflows StorageGRID", bucket: strings.Repeat("a", 253), wantErr: true},
		{name: "uppercase", bucket: "MyBucket", wantErr: true},
		{name: "underscore", bucket: "my_bucket", wantErr: true},
		{name: "leading hyphen", bucket: "-bucket", wantErr: true},
		{name: "trailing dot", bucket: "bucket.", wantErr: true},
		{name: "consecutive dots", bucket: "my..bucket", wantErr: true},
		{name: "dot next to hyphen", bucket: "my.-bucket", wantErr: true},
		{name: "IP address", bucket: "192.168.0.1", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBucketName(tc.bucket)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateBucketName(%q) = nil, want an error", tc.bucket)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateBucketName(%q) = %v, want nil", tc.bucket, err)
			}
		})
	}
}

func TestValidateRegionName(t *testing.T) {
	tests := []struct {
		region  string
		wantErr bool
	}{
		{region: "us-east-1"},
		{region: "US-EAST-1"},
		{region: "ab"},
		{region: strings.Repeat("a", MaxRegionNameLength)},
		{region: "", wantErr: true},
		{region: "a", wantErr: true},
		{region: strings.Repeat("a", MaxRegionNameLength+1), wantErr: true},
		{region: "us_east_1", wantErr: true},
		{region: "us east 1", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.region), func(t *testing.T) {
			err := ValidateRegionName(tc.region)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateRegionName(%q) = nil, want an error", tc.region)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateRegionName(%q) = %v, want nil", tc.region, err)
			}
		})
	}
}

// A bad bucket name fails the same way on every backend; a bad region only reflects one
// backend's config. Only the former may be treated as terminal.
func TestIsBucketNameInvalid(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "bucket name", err: ValidateBucketName("UPPER"), want: true},
		{name: "wrapped bucket name", err: fmt.Errorf("create: %w", ValidateBucketName("")), want: true},
		{name: "region", err: ValidateRegionName("us_east_1"), want: false},
		{name: "not a validation error", err: errors.New("boom"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBucketNameInvalid(tc.err); got != tc.want {
				t.Errorf("IsBucketNameInvalid(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
