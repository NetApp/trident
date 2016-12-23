// Copyright 2016 NetApp, Inc. All Rights Reserved.

package persistent_store

const (
	KeyErrorMsg = "Unable to find key"
)

// Used to turn etcd errors into something that callers can understand without
// having to import the client library
type KeyError struct {
	Key string
}

func (e KeyError) Error() string {
	return KeyErrorMsg
}
