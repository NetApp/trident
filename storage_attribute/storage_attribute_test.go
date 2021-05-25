// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMatches(t *testing.T) {
	for i, test := range []struct {
		r        Request
		o        Offer
		expected bool
	}{
		{NewIntRequest(5), NewIntOffer(0, 10), true},
		{NewIntRequest(5), NewIntOffer(6, 10), false},
		{NewIntRequest(11), NewIntOffer(6, 10), false},
		{NewBoolRequest(false), NewBoolOffer(false), true},
		{NewBoolRequest(false), NewBoolOffer(true), true},
		{NewBoolRequest(true), NewBoolOffer(true), true},
		{NewBoolRequest(true), NewBoolOffer(false), false},
		{NewStringRequest("bar"), NewStringOffer("foo", "bar"), true},
		{NewStringRequest("baz"), NewStringOffer("foo", "bar"), false},
		{NewIntRequest(5), NewStringOffer("foo", "bar"), false},
		{NewIntRequest(5), NewBoolOffer(true), false},
		{NewBoolRequest(false), NewIntOffer(0, 10), false},
		{NewBoolRequest(false), NewLabelOffer(map[string]string{"performance": "gold"}), false},

		{NewLabelRequestMustCompile("performance = gold"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance=gold;protection-ha=low-minimal"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection-ha": "low-minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance=gold;protection=full"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance=gold;protection=minimal"),
			NewLabelOffer(map[string]string{"performance": "silver", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance=gold;protection=minimal"),
			NewLabelOffer(map[string]string{"performance": "silver", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("protection != full"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance = gold;protection != minimal"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance in (gold,silver)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance in (silver, bronze)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance in (gold, silver); protection in (minimal, full)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance notin (gold,silver)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance notin (silver, bronze)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("location notin (east, west)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("performance;protection"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("!performance; !protection"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("protection;!cloud"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{NewLabelRequestMustCompile("protection;foo"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{NewLabelRequestMustCompile("performance=gold;protection!=full;cloud in (aws, azure);!foo"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"},
				map[string]string{"cloud": "aws", "bar": "baz"}),
			true,
		},
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
		Replication: &boolOffer{
			Offer: false,
		},
		Snapshots: &boolOffer{
			Offer: true,
		},
		ProvisioningType: &stringOffer{
			Offers: []string{"foo", "bar"},
		},
		Labels: NewLabelOffer(
			map[string]string{"performance": "gold", "protection": "minimal"},
			map[string]string{"cloud": "aws"},
		),
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

	labelRequest, _ := NewLabelRequest("performance=gold")

	requestMap := map[string]Request{
		IOPS: &intRequest{
			Request: 5,
		},
		Replication: &boolRequest{
			Request: true,
		},
		Snapshots: &boolRequest{
			Request: true,
		},
		BackendType: &stringRequest{
			Request: "foo",
		},
		Labels: labelRequest,
	}

	data, err := MarshalRequestMap(requestMap)
	if err != nil {
		t.Fatal("Unable to marshal: ", err)
	}

	targetRequestMap, err := UnmarshalRequestMap(data)
	if err != nil {
		t.Fatal("Unable to unmarshal: ", err)
	}

	if !reflect.DeepEqual(requestMap, targetRequestMap) {
		t.Errorf("Maps are unequal.\n Expected: %s\nGot: %s\n", requestMap, targetRequestMap)
	}
}
