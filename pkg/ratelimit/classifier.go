// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DecreaseOnError reports whether an upstream error should trigger a
// multiplicative decrease in the adaptive limiter.
type DecreaseOnError func(error) bool

// MessageSubstringDecreaseOnError returns a predicate that decreases for matching
// gRPC ResourceExhausted errors. Matches are case-insensitive, and any exclude
// substring takes precedence over an include substring.
func MessageSubstringDecreaseOnError(include, exclude []string) DecreaseOnError {
	includeLower := lowerStrings(include)
	excludeLower := lowerStrings(exclude)
	return func(err error) bool {
		if err == nil {
			return false
		}
		s, ok := status.FromError(err)
		if !ok || s.Code() != codes.ResourceExhausted {
			return false
		}
		msg := strings.ToLower(s.Message())
		for _, sub := range excludeLower {
			if strings.Contains(msg, sub) {
				return false
			}
		}
		for _, sub := range includeLower {
			if strings.Contains(msg, sub) {
				return true
			}
		}
		return false
	}
}

func lowerStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(v)
	}
	return out
}
