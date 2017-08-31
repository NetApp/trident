// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

const (
	KeyNotFoundErr        = "Unable to find key"
	KeyExistsErr          = "Key already exists"
	UnavailableClusterErr = "Unavailable etcd cluster"
)

// Used to turn etcd errors into something that callers can understand without
// having to import the client library
type PersistentStoreError struct {
	Message string
	Key     string
}

func NewPersistentStoreError(message, key string) *PersistentStoreError {
	return &PersistentStoreError{
		Message: message,
		Key:     key,
	}
}

func (e *PersistentStoreError) Error() string {
	return e.Message
}

func MatchKeyNotFoundErr(err error) bool {
	if err != nil && err.Error() == KeyNotFoundErr {
		return true
	}
	return false
}

func MatchUnavailableClusterErr(err error) bool {
	if err != nil && err.Error() == UnavailableClusterErr {
		return true
	}
	return false
}
