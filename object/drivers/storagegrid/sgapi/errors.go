// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Terminal conditions in the bucket delete state machine. Both are failed
// preconditions: retrying will not resolve them without an operator decision.
var (
	// ErrBucketNotEmpty means the bucket holds objects and emptying was not
	// opted into.
	ErrBucketNotEmpty = errors.New("bucket is not empty")

	// ErrObjectsRemain means the emptying operation finished and objects are
	// still present, so no amount of retrying will make the delete succeed.
	ErrObjectsRemain = errors.New("objects remain in the bucket after emptying completed")
)

// ValidationError is a local pre-flight failure. The request never reached the
// grid, so there is no status code to report.
type ValidationError struct {
	Field  string
	Value  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("%s %s", e.Field, e.Reason)
	}
	return fmt.Sprintf("%s %q %s", e.Field, e.Value, e.Reason)
}

// IsValidationError reports whether err came from local validation.
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// IsBucketNameInvalid reports whether err is a local validation failure of the bucket name
// itself. Unlike other validation failures (e.g. a region taken from one backend's config), a
// bad bucket name is rejected identically by every backend, so retrying elsewhere cannot help.
func IsBucketNameInvalid(err error) bool {
	var v *ValidationError
	return errors.As(err, &v) && v.Field == "bucket name"
}

// AlertError is a request that returned a success status while reporting an
// error-severity alert, meaning part of the operation did not take effect.
type AlertError struct {
	Op     string
	Alerts []Alert
}

func (e *AlertError) Error() string {
	parts := make([]string, 0, len(e.Alerts))
	for _, a := range e.Alerts {
		parts = append(parts, a.summary())
	}
	return fmt.Sprintf("%s reported an error alert: %s", e.Op, strings.Join(parts, "; "))
}

// IsAlertError reports whether err is an error-severity alert on an otherwise
// successful response.
func IsAlertError(err error) bool {
	var a *AlertError
	return errors.As(err, &a)
}

// APIError is a non-2xx response from the grid. Most endpoints document only
// their success code plus a catch-all, so StatusCode should be treated as a
// coarse class and Key used for anything finer.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	// Code is the status echoed in the response envelope, which is not always
	// present and not always equal to StatusCode.
	Code             int
	Key              string
	Text             string
	Context          string
	DeveloperMessage string
	// Errors carries per-field detail for validation failures.
	Errors []Message
	// Body is the raw response, kept because the error catalogue is not
	// documented and unrecognised failures need to be inspectable.
	Body []byte
}

func newAPIError(method, path string, resp *Response) *APIError {
	e := &APIError{
		Method:     method,
		Path:       path,
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}
	if env := resp.Envelope; env != nil {
		e.Code = env.Code
		e.Errors = env.Errors
		if env.Message != nil {
			e.Key = env.Message.Key
			e.Text = env.Message.Text
			e.Context = env.Message.Context
			e.DeveloperMessage = env.Message.DeveloperMessage
		}
	}
	return e
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	if e.Key != "" {
		fmt.Fprintf(&b, " [%s]", e.Key)
	}
	switch {
	case e.Text != "":
		fmt.Fprintf(&b, ": %s", e.Text)
	case e.Key == "" && len(e.Body) > 0:
		fmt.Fprintf(&b, ": %s", strings.TrimSpace(string(e.Body)))
	}
	if e.Context != "" {
		fmt.Fprintf(&b, " (context %s)", e.Context)
	}
	for _, fe := range e.Errors {
		fmt.Fprintf(&b, "; %s", fe.summary())
	}
	return b.String()
}

// StatusOf returns the HTTP status of err, or 0 if it is not an APIError.
func StatusOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

// MessageKeyOf returns the stable message key of err, or "" if there is none.
func MessageKeyOf(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Key
	}
	return ""
}

// IsBadRequest reports a 400. On /authorize specifically this means single
// sign-on is enforced and password authentication is refused.
func IsBadRequest(err error) bool { return StatusOf(err) == http.StatusBadRequest }

// IsUnauthorized reports a 401, which is an expired token or bad credentials.
func IsUnauthorized(err error) bool { return StatusOf(err) == http.StatusUnauthorized }

// IsForbidden reports a 403, meaning the credentials lack a required permission.
func IsForbidden(err error) bool { return StatusOf(err) == http.StatusForbidden }

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return StatusOf(err) == http.StatusNotFound }

// IsConflict reports a 409: a bucket that is not empty, or a name already taken.
func IsConflict(err error) bool { return StatusOf(err) == http.StatusConflict }

// IsUnprocessable reports a 422 validation failure, which is terminal.
func IsUnprocessable(err error) bool { return StatusOf(err) == http.StatusUnprocessableEntity }

// IsRetryable reports whether err is a server-side failure worth retrying with
// backoff. It covers both a 5xx and a request that never got a response at all.
func IsRetryable(err error) bool {
	return StatusOf(err) >= http.StatusInternalServerError || IsTransportError(err)
}

// IsTransportError reports whether the request failed before a response arrived:
// a refused connection, an unresolvable host, a TLS handshake failure. Nothing is
// known about the operation's outcome, and the condition is usually transient, so
// this is the canonical case for retrying rather than failing.
//
// It is worth separating from a 5xx because an unreachable admin node during a
// rolling upgrade is routine, and classifying it as unknown would let a caller
// treat a retryable outage as a permanent failure.
func IsTransportError(err error) bool {
	if err == nil {
		return false
	}
	// A response arrived, so whatever went wrong is the grid's answer rather
	// than a transport failure, even though the error records the URL.
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
