// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

const (
	KeyNotFoundErr        = "Unable to find key"
	KeyExistsErr          = "Key already exists"
	UnavailableClusterErr = "Unavailable etcd cluster"
	NotSupported          = "Unsupported operation"
)

// Error is used to turn etcd errors into something that callers can understand without
// having to import the client library
type Error struct {
	Message string
	Key     string
}

func NewPersistentStoreError(message, key string) *Error {
	return &Error{
		Message: message,
		Key:     key,
	}
}

func (e *Error) Error() string {
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
