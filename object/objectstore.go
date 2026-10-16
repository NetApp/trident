package object

import "context"

type StoreState string

const (
	StoreStateOnline   StoreState = "online"
	StoreStateOffline  StoreState = "offline"
	StoreStateDeleting StoreState = "deleting"
)

// CommonConfig carries values supplied outside driver JSON.
// REST create includes objectStoreName in the request body. CR create sets
// StoreName from metadata.name with SetCommonConfig after the driver is built.
type CommonConfig struct {
	StoreName  string
	DriverName string
	UUID       string
	State      StoreState
}

type Store interface {
	Name() string
	DriverName() string
	// SetCommonConfig applies name, UUID, and state supplied outside driver JSON.
	// Core sets online and offline; drivers store the state they are given.
	SetCommonConfig(CommonConfig)
	StoreUUID() string
	State() StoreState
	SetState(StoreState)
	Initialize(context.Context) error
	Terminate(context.Context) error
	// MatchParams checks if all provided params match this store.
	MatchParams(map[string]string) bool
	GetBucketByName(context.Context, string) (*Bucket, error)
	// CreateBucket must be idempotent on Bucket.Name: a repeat call for an existing name
	// should succeed with Info populated.
	CreateBucket(context.Context, *Bucket) error
	DeleteBucket(context.Context, string) error
	GrantBucketAccess(context.Context, []BucketAccessMode, *BucketAccess) (*S3BucketAccessSecret, error)
	RevokeBucketAccess(context.Context, *BucketAccess) error
	Secrets() interface{}
	SmartCopy() interface{}
	GetUniqueKey() string
}
