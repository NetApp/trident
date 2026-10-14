package concurrent_cache

import (
	"context"

	"github.com/netapp/trident/object"
)

// testObjectStore is a minimal object.Store for concurrent_cache tests.
type testObjectStore struct {
	name  string
	uuid  string
	state object.StoreState
}

func newTestObjectStore(name, uuid string) *testObjectStore {
	if uuid == "" {
		uuid = name
	}
	return &testObjectStore{name: name, uuid: uuid, state: object.StoreStateOnline}
}

func (s *testObjectStore) Name() string       { return s.name }
func (s *testObjectStore) DriverName() string { return "test" }
func (s *testObjectStore) StoreUUID() string  { return s.uuid }
func (s *testObjectStore) SetCommonConfig(config object.CommonConfig) {
	if config.StoreName != "" {
		s.name = config.StoreName
	}
	if config.UUID != "" {
		s.uuid = config.UUID
	}
	if config.State != "" {
		s.state = config.State
	}
}
func (s *testObjectStore) State() object.StoreState         { return s.state }
func (s *testObjectStore) SetState(state object.StoreState) { s.state = state }

func (s *testObjectStore) Initialize(context.Context) error   { return nil }
func (s *testObjectStore) Terminate(context.Context) error    { return nil }
func (s *testObjectStore) MatchParams(map[string]string) bool { return true }

func (s *testObjectStore) GetBucketByName(context.Context, string) (*object.Bucket, error) {
	return nil, nil
}
func (s *testObjectStore) CreateBucket(context.Context, *object.Bucket) (*object.Bucket, error) {
	return nil, nil
}
func (s *testObjectStore) DeleteBucket(context.Context, string) error { return nil }
func (s *testObjectStore) GrantBucketAccess(context.Context, []object.BucketAccessMode, *object.BucketAccess,
) (*object.S3BucketAccessSecret, error) {
	return nil, nil
}
func (s *testObjectStore) RevokeBucketAccess(context.Context, *object.BucketAccess) error {
	return nil
}

func (s *testObjectStore) Secrets() interface{} { return nil }

func (s *testObjectStore) SmartCopy() interface{} {
	copied := *s
	return &copied
}

func (s *testObjectStore) GetUniqueKey() string { return s.name }
