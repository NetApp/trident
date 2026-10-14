// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"encoding/json"
	"strings"
)

// Envelope is the response wrapper every StorageGRID endpoint shares. Data is
// left raw so each call can decode only what it needs.
type Envelope struct {
	ResponseTime string          `json:"responseTime,omitempty"`
	Status       string          `json:"status,omitempty"`
	APIVersion   string          `json:"apiVersion,omitempty"`
	Code         int             `json:"code,omitempty"`
	Deprecated   bool            `json:"deprecated,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Message      *Message        `json:"message,omitempty"`
	Errors       []Message       `json:"errors,omitempty"`
	Metadata     *Metadata       `json:"metadata,omitempty"`
}

// Message is the error and alert detail structure. Key is the only stable,
// localisation-independent discriminator; Text is localised and will change, so
// callers must branch on Key.
type Message struct {
	Text             string `json:"text,omitempty"`
	Key              string `json:"key,omitempty"`
	Context          string `json:"context,omitempty"`
	DeveloperMessage string `json:"developerMessage,omitempty"`
}

func (m Message) summary() string {
	switch {
	case m.Text != "" && m.Context != "":
		return m.Text + " (" + m.Context + ")"
	case m.Text != "":
		return m.Text
	case m.Key != "":
		return m.Key
	default:
		return "unspecified"
	}
}

// Metadata carries out-of-band notices about an otherwise successful response.
type Metadata struct {
	Alerts []Alert `json:"alerts,omitempty"`
}

// AlertSeverityError is the severity that turns a 2xx into a failure.
const AlertSeverityError = "error"

// Alert reports that part of an operation did not take effect even though the
// request itself succeeded.
type Alert struct {
	Severity string `json:"severity,omitempty"`
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	Context  string `json:"context,omitempty"`

	DeveloperMessage string `json:"developerMessage,omitempty"`
}

func (a Alert) summary() string {
	severity := a.Severity
	if severity == "" {
		severity = "unspecified"
	}
	return severity + ": " + Message{Text: a.Text, Key: a.Key, Context: a.Context}.summary()
}

func errorAlerts(alerts []Alert) []Alert {
	var out []Alert
	for _, a := range alerts {
		if strings.EqualFold(a.Severity, AlertSeverityError) {
			out = append(out, a)
		}
	}
	return out
}

// ObjectLock is the S3 Object Lock configuration of a bucket. It can only be
// enabled at creation time.
type ObjectLock struct {
	Enabled                 bool              `json:"enabled"`
	DefaultRetentionSetting *DefaultRetention `json:"defaultRetentionSetting,omitempty"`
}

// DefaultRetention is the default retention applied to objects in a bucket with
// Object Lock enabled.
type DefaultRetention struct {
	Mode  string `json:"mode,omitempty"`
	Years *int64 `json:"years,omitempty"`
	Days  *int64 `json:"days,omitempty"`
}

// ContainerCreate is the POST /org/containers request body. These three fields
// are all the endpoint accepts; everything else a BucketClass might configure is
// a separate PUT against a sub-resource after the bucket exists.
type ContainerCreate struct {
	Name         string      `json:"name"`
	Region       string      `json:"region,omitempty"`
	S3ObjectLock *ObjectLock `json:"s3ObjectLock,omitempty"`
}

// Container is one entry from the bucket list.
type Container struct {
	Name                string `json:"name"`
	CreationTime        string `json:"creationTime,omitempty"`
	Region              string `json:"region,omitempty"`
	ObjectCount         *int64 `json:"objectCount,omitempty"`
	DataBytes           *int64 `json:"dataBytes,omitempty"`
	VersioningEnabled   *bool  `json:"versioningEnabled,omitempty"`
	VersioningSuspended *bool  `json:"versioningSuspended,omitempty"`
}

// Versioning is the bucket versioning state. VersioningSuspended is omitted on
// write, where only VersioningEnabled is meaningful.
type Versioning struct {
	VersioningEnabled   bool `json:"versioningEnabled"`
	VersioningSuspended bool `json:"versioningSuspended,omitempty"`
}

// QuotaObjectBytes is the capacity quota. A nil value means no quota is set, and
// sending nil clears an existing one.
type QuotaObjectBytes struct {
	QuotaObjectBytes *int64 `json:"quotaObjectBytes"`
}

// QuotaObjectCount is the object count quota, with the same nil semantics as
// QuotaObjectBytes.
type QuotaObjectCount struct {
	QuotaObjectCount *int64 `json:"quotaObjectCount"`
}

// Consistency is a bucket's consistency level. The value is passed through to
// the grid rather than checked against a local allowlist: the schema documents an
// enum, but which spellings a given release accepts is one of the things the
// pilot is meant to establish.
type Consistency struct {
	Consistency string `json:"consistency"`
}

// DeleteObjectsStatus reports on the asynchronous server-side bucket emptying
// operation.
type DeleteObjectsStatus struct {
	IsDeletingObjects  bool   `json:"isDeletingObjects"`
	InitialObjectCount *int64 `json:"initialObjectCount,omitempty"`
	InitialObjectBytes *int64 `json:"initialObjectBytes,omitempty"`
}

// CreateBucketResult reports what the grid actually did, which is not always
// what was asked for.
type CreateBucketResult struct {
	Name string `json:"name"`
	// Region is as the grid resolved it, which for a request that omitted the
	// region may be the grid default rather than an empty string.
	Region string `json:"region,omitempty"`
	// RegionResolved is false when the create response did not report a region,
	// meaning it has to be read back separately.
	RegionResolved bool    `json:"regionResolved"`
	Alerts         []Alert `json:"alerts,omitempty"`
}

// BucketReads selects which optional per-property reads GetBucket performs. Each
// one costs a round trip, so a caller that validates nothing beyond the region
// leaves them all false.
type BucketReads struct {
	Versioning       bool
	QuotaObjectBytes bool
	QuotaObjectCount bool
	ObjectLock       bool
	Consistency      bool
}

// AllBucketReads selects every optional property read.
func AllBucketReads() BucketReads {
	return BucketReads{
		Versioning:       true,
		QuotaObjectBytes: true,
		QuotaObjectCount: true,
		ObjectLock:       true,
		Consistency:      true,
	}
}

// Any reports whether any optional read is selected.
func (r BucketReads) Any() bool {
	return r.Versioning || r.QuotaObjectBytes || r.QuotaObjectCount || r.ObjectLock || r.Consistency
}

// BucketState is the assembled view of a bucket.
type BucketState struct {
	Name             string      `json:"name"`
	Region           string      `json:"region"`
	Versioning       *Versioning `json:"versioning,omitempty"`
	QuotaObjectBytes *int64      `json:"quotaObjectBytes,omitempty"`
	QuotaObjectCount *int64      `json:"quotaObjectCount,omitempty"`
	ObjectLock       *ObjectLock `json:"objectLock,omitempty"`
	Consistency      string      `json:"consistency,omitempty"`
}
