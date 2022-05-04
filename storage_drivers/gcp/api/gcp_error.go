// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import (
	"fmt"
)

type Error struct {
	statusCode int
	code       int
	message    string
}

func (e Error) IsPassed() bool {
	return e.statusCode < 300
}

func (e Error) Error() string {
	if e.IsPassed() {
		return fmt.Sprintf("API passed (%d)", e.statusCode)
	}
	return fmt.Sprintf("API failed (%d). Code: %d. Message: %s", e.statusCode, e.code, e.message)
}

func (e Error) Message() string {
	return e.message
}

func (e Error) Code() int {
	return e.code
}
