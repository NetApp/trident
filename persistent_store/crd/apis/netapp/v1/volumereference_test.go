// Copyright 2022 NetApp, Inc. All Rights Reserved.

package v1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTridentVolumeReferenceGetObjectMeta(t *testing.T) {
	tests := []struct {
		name       string
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		want       metav1.ObjectMeta
		Spec       TridentVolumeReferenceSpec
	}{
		{
			name:       "GetMetaObject",
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Name: "fakeMetaObject"},
			want:       metav1.ObjectMeta{Name: "fakeMetaObject"},
			Spec:       TridentVolumeReferenceSpec{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}
			if got := in.GetObjectMeta(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TridentVolumeReference.GetObjectMeta() = %v, expected %v", got, tt.want)
			}
		})
	}
}

func TestTridentVolumeReferenceGetFinalizers(t *testing.T) {
	tests := []struct {
		name           string
		want           metav1.ObjectMeta
		wantFinalizers []string
		TypeMeta       metav1.TypeMeta
		ObjectMeta     metav1.ObjectMeta
		Spec           TridentVolumeReferenceSpec
	}{
		{
			name:           "GetFinalizers",
			TypeMeta:       metav1.TypeMeta{},
			ObjectMeta:     metav1.ObjectMeta{Finalizers: []string{"fakeFinalizer"}},
			wantFinalizers: []string{"fakeFinalizer"},
			Spec:           TridentVolumeReferenceSpec{},
		},
		{
			name:           "EmptyFinalizer",
			TypeMeta:       metav1.TypeMeta{},
			ObjectMeta:     metav1.ObjectMeta{},
			wantFinalizers: []string{},
			Spec:           TridentVolumeReferenceSpec{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}

			if got := in.GetFinalizers(); !reflect.DeepEqual(got, tt.wantFinalizers) {
				t.Errorf("TridentVolumeReference.GetFinalizers() = %v, expected = %v", got, tt.wantFinalizers)
			}
		})
	}
}

func TestTridentVolumeReferenceHasTridentFinalizers(t *testing.T) {
	tests := []struct {
		name       string
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Spec       TridentVolumeReferenceSpec
		want       bool
	}{
		{
			name:       "TridentFinalizerPresent",
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"trident.netapp.io"}},
			Spec:       TridentVolumeReferenceSpec{},
			want:       true,
		},
		{
			name:       "TridentFinalizerAbsent",
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"fakeFinalizer"}},
			Spec:       TridentVolumeReferenceSpec{},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}
			if got := in.HasTridentFinalizers(); got != tt.want {
				t.Errorf("TridentVolumeReference.HasTridentFinalizers() = %v, expected %v", got, tt.want)
			}
		})
	}
}

func TestTridentVolumeReferenceAddTridentFinalizers(t *testing.T) {
	tests := []struct {
		name       string
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Spec       TridentVolumeReferenceSpec
		want       bool
	}{
		{
			name:       "AddTridentFinalizer",
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"fakeFinalizer"}},
			Spec:       TridentVolumeReferenceSpec{},
			want:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}
			in.AddTridentFinalizers()

			if got := in.HasTridentFinalizers(); got != tt.want {
				t.Errorf("TridentVolumeReference.HasTridentFinalizers() = %v, expected %v", got, tt.want)
			}
		})
	}
}

func TestTridentVolumeReferenceRemoveTridentFinalizers(t *testing.T) {
	tests := []struct {
		name       string
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Spec       TridentVolumeReferenceSpec
		want       bool
	}{
		{
			name:       "RemoveTridentFinalizer",
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"trident.netapp.io", "fakeFinalizer"}},
			Spec:       TridentVolumeReferenceSpec{},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}
			in.RemoveTridentFinalizers()

			if got := in.HasTridentFinalizers(); got != tt.want {
				t.Errorf("TridentVolumeReference.HasTridentFinalizers() = %v, expected %v", got, tt.want)
			}
		})
	}
}

func TestTridentVolumeReferenceCacheKey(t *testing.T) {
	tests := []struct {
		name       string
		want       string
		TypeMeta   metav1.TypeMeta
		ObjectMeta metav1.ObjectMeta
		Spec       TridentVolumeReferenceSpec
	}{
		{
			name:       "cacheKey",
			TypeMeta:   metav1.TypeMeta{},
			Spec:       TridentVolumeReferenceSpec{PVCNamespace: "fakePVCNs", PVCName: "fakePVC"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "fakeNs"},
			want:       "fakeNs_fakePVCNs/fakePVC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &TridentVolumeReference{
				TypeMeta:   tt.TypeMeta,
				ObjectMeta: tt.ObjectMeta,
				Spec:       tt.Spec,
			}
			if got := in.CacheKey(); got != tt.want {
				t.Errorf("TridentVolumeReference.CacheKey() = %v, expected %v", got, tt.want)
			}
		})
	}
}
