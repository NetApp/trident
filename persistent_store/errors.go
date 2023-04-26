// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	KeyNotFoundErr = "Unable to find key"
	NotSupported   = "Unsupported operation"
)

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

func IsStatusNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	statusError, ok := err.(*errors.StatusError)
	if !ok {
		return false
	}
	return statusError.Status().Code == http.StatusNotFound
}

type AlreadyExistsError struct {
	resourceType string
	resourceName string
}

func NewAlreadyExistsError(resourceType, resourceName string) *AlreadyExistsError {
	return &AlreadyExistsError{
		resourceType: resourceType,
		resourceName: resourceName,
	}
}

func (ae *AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s %s already exists", ae.resourceType, ae.resourceName)
}

func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*AlreadyExistsError)
	return ok
}
