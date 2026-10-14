// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/object"
)

func NewTridentBucket(bucket *object.Bucket) (*TridentBucket, error) {
	tb := &TridentBucket{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentBucket",
		},
		ObjectMeta: metav1.ObjectMeta{Name: NameFix(bucket.Name)},
	}
	if err := tb.Apply(bucket); err != nil {
		return nil, err
	}
	return tb, nil
}

func (in *TridentBucket) Apply(bucket *object.Bucket) error {
	if NameFix(bucket.Name) != in.Name {
		return ErrNamesDontMatch
	}
	in.BucketName = bucket.Name
	in.StoreName = bucket.StoreName
	in.Info = tridentBucketInfoFromObject(bucket.Info)
	return nil
}

func (in *TridentBucket) Persistent() *object.Bucket {
	return &object.Bucket{
		Name:      in.BucketName,
		StoreName: in.StoreName,
		Info:      objectBucketInfoFromTrident(in.Info),
	}
}

// tridentBucketInfoFromObject converts object.BucketInfo to its CRD-persisted shape. Returns nil
// when there is nothing to persist, matching Info's `json:"info,omitempty"` tag.
func tridentBucketInfoFromObject(info object.BucketInfo) *TridentBucketInfo {
	if info.S3 == nil {
		return nil
	}
	return &TridentBucketInfo{
		S3: &TridentS3BucketInfo{
			Endpoint:        info.S3.Endpoint,
			Region:          info.S3.Region,
			AddressingStyle: info.S3.AddressingStyle,
		},
	}
}

// objectBucketInfoFromTrident is tridentBucketInfoFromObject's inverse.
func objectBucketInfoFromTrident(info *TridentBucketInfo) object.BucketInfo {
	if info == nil || info.S3 == nil {
		return object.BucketInfo{}
	}
	return object.BucketInfo{
		S3: &object.S3BucketInfo{
			Endpoint:        info.S3.Endpoint,
			Region:          info.S3.Region,
			AddressingStyle: info.S3.AddressingStyle,
		},
	}
}

func NewTridentBucketAccess(access *object.BucketAccess) (*TridentBucketAccess, error) {
	tba := &TridentBucketAccess{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentBucketAccess",
		},
		ObjectMeta: metav1.ObjectMeta{Name: NameFix(access.Name)},
	}
	if err := tba.Apply(access); err != nil {
		return nil, err
	}
	return tba, nil
}

func (in *TridentBucketAccess) Apply(access *object.BucketAccess) error {
	if NameFix(access.Name) != in.Name {
		return ErrNamesDontMatch
	}
	spec, err := json.Marshal(access)
	if err != nil {
		return err
	}
	in.Spec.Raw = spec
	in.BucketAccessName = access.Name
	in.StoreName = access.StoreName
	return nil
}

func (in *TridentBucketAccess) Persistent() (*object.BucketAccess, error) {
	access := &object.BucketAccess{}
	return access, json.Unmarshal(in.Spec.Raw, access)
}
