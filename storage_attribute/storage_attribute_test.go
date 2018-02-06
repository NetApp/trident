// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"encoding/json"
	"log"
	"reflect"
	"testing"
)

func TestMatches(t *testing.T) {
	for i, test := range []struct {
		r        Request
		o        Offer
		expected bool
	}{
		// This is all basically trivial, but it doesn't hurt to test.
		{NewIntRequest(5), NewIntOffer(0, 10), true},
		{NewIntRequest(5), NewIntOffer(6, 10), false},
		{NewIntRequest(11), NewIntOffer(6, 10), false},
		{NewBoolRequest(false), NewBoolOffer(false), true},
		{NewBoolRequest(false), NewBoolOffer(true), true},
		{NewBoolRequest(true), NewBoolOffer(true), true},
		{NewBoolRequest(true), NewBoolOffer(false), false},
		{NewStringRequest("bar"), NewStringOffer("foo", "bar"),
			true},
		{NewStringRequest("baz"), NewStringOffer("foo", "bar"),
			false},
		{NewIntRequest(5), NewStringOffer("foo", "bar"), false},
		{NewIntRequest(5), NewBoolOffer(true), false},
		{NewBoolRequest(false), NewIntOffer(0, 10), false},
	} {
		if test.o.Matches(test.r) != test.expected {
			t.Errorf("Test case %d failed", i)
		}
	}
}

func TestUnmarshalOffer(t *testing.T) {
	var (
		targetOfferMap map[string]Offer
	)
	offerMap := map[string]Offer{
		IOPS: &intOffer{
			Min: 5,
			Max: 10,
		},
		Snapshots: &boolOffer{
			Offer: true,
		},
		ProvisioningType: &stringOffer{
			Offers: []string{"foo", "bar"},
		},
	}
	data, err := json.Marshal(offerMap)
	if err != nil {
		t.Fatal("Unable to marshal:  ", err)
	}
	targetOfferMap, err = UnmarshalOfferMap(data)
	if err != nil {
		t.Fatal("Unable to unmarshal: ", err)
	}
	if !reflect.DeepEqual(offerMap, targetOfferMap) {
		t.Errorf("Maps are unequal.\n Expected: %s\nGot: %s\n", offerMap,
			targetOfferMap)
	}
}

func TestUnmarshalRequest(t *testing.T) {
	var (
		targetRequestMap map[string]Request
	)
	requestMap := map[string]Request{
		IOPS: &intRequest{
			Request: 5,
		},
		Snapshots: &boolRequest{
			Request: true,
		},
		BackendType: &stringRequest{
			Request: "foo",
		},
	}
	//data, err := json.Marshal(requestMap)
	data, err := MarshalRequestMap(requestMap)
	if err != nil {
		t.Fatal("Unable to marshal:  ", err)
	}
	log.Print("Marshaled data: ", string(data))
	targetRequestMap, err = UnmarshalRequestMap(data)
	if err != nil {
		t.Fatal("Unable to unmarshal: ", err)
	}
	if !reflect.DeepEqual(requestMap, targetRequestMap) {
		t.Errorf("Maps are unequal.\n Expected: %s\nGot: %s\n", requestMap,
			targetRequestMap)
	}
}
