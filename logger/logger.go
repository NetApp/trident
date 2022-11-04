// Copyright 2021 NetApp, Inc. All Rights Reserved.

package logger

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

func Logc(ctx context.Context) *log.Entry {
	entry := log.WithFields(log.Fields{
		"requestID":     ctx.Value(ContextKeyRequestID),
		"requestSource": ctx.Value(ContextKeyRequestSource),
	})

	if val := ctx.Value(CRDControllerEvent); val != nil {
		entry = entry.WithField(string(CRDControllerEvent), val)
	}

	return entry
}

func GenerateRequestContext(ctx context.Context, requestID, requestSource string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	} else {
		if v := ctx.Value(ContextKeyRequestID); v != nil {
			requestID = fmt.Sprint(v)
		}
		if v := ctx.Value(ContextKeyRequestSource); v != nil {
			requestSource = fmt.Sprint(v)
		}
	}
	if requestID == "" {
		requestID = uuid.New().String()
	}
	if requestSource == "" {
		requestSource = "Unknown"
	}
	ctx = context.WithValue(ctx, ContextKeyRequestID, requestID)
	ctx = context.WithValue(ctx, ContextKeyRequestSource, requestSource)
	return ctx
}

// FormatMessageForLog capitalizes the first letter of the string, and adds punctuation.
func FormatMessageForLog(msg string) string {
	msg = strings.ToLower(msg)
	runes := []rune(msg)

	sentenceCased := strings.ToUpper(string(runes[0])) + string(runes[1:])
	if !strings.HasSuffix(sentenceCased, ".") {
		sentenceCased += "."
	}

	return sentenceCased
}
