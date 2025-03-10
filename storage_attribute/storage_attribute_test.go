// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storageattribute

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

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

		{
			NewLabelRequestMustCompile("performance = gold"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance=gold;protection-ha=low-minimal"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection-ha": "low-minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance=gold;protection=full"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance=gold;protection=minimal"),
			NewLabelOffer(map[string]string{"performance": "silver", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance=gold;protection=minimal"),
			NewLabelOffer(map[string]string{"performance": "silver", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("protection != full"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance = gold;protection != minimal"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance in (gold,silver)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance in (silver, bronze)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance in (gold, silver); protection in (minimal, full)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance notin (gold,silver)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance notin (silver, bronze)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("location notin (east, west)"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("performance;protection"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("!performance; !protection"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("protection;!cloud"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			true,
		},
		{
			NewLabelRequestMustCompile("protection;foo"),
			NewLabelOffer(map[string]string{"performance": "gold", "protection": "minimal"}),
			false,
		},
		{
			NewLabelRequestMustCompile("performance=gold;protection!=full;cloud in (aws, azure);!foo"),
			NewLabelOffer(
				map[string]string{"performance": "gold", "protection": "minimal"},
				map[string]string{"cloud": "aws", "bar": "baz"},
			),
			true,
		},
	} {
		if test.o.Matches(test.r) != test.expected {
			t.Errorf("Test case %d failed", i)
		}
	}
}

func TestUnmarshalOffer(t *testing.T) {
	var targetOfferMap map[string]Offer
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
		t.Errorf("Maps are unequal.\n Expected: %s\nGot: %s\n", offerMap, targetOfferMap)
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

	// Test for replication with prefix. This is needed in case of upgrade from 22.04,
	// we remove the prefix from replication to support it in later versions
	requestMap2 := map[string]Request{
		"trident.netapp.io/replication": &boolRequest{
			Request: true,
		},
	}

	data2, err := MarshalRequestMap(requestMap2)
	if err != nil {
		t.Fatal("Unable to marshal: ", err)
	}

	targetRequestMap2, err := UnmarshalRequestMap(data2)
	if err != nil {
		t.Fatal("Unable to unmarshal: ", err)
	}

	_, ok1 := targetRequestMap2["replication"]
	_, ok2 := targetRequestMap2["trident.netapp.io/replication"]

	assert.True(t, ok1)
	assert.False(t, ok2)

	// Adding coverage for default case
	attrTypes["temp"] = "float32"
	_, _ = CreateAttributeRequestFromAttributeValue("temp", "")
}

func TestNewBoolOfferFromOffers(t *testing.T) {
	for i, test := range []struct {
		actual   Offer
		expected Offer
	}{
		{NewBoolOfferFromOffers(NewBoolOffer(true), NewBoolOffer(true)), NewBoolOffer(true)},
		{NewBoolOfferFromOffers(NewBoolOffer(true), NewBoolOffer(false)), NewBoolOffer(true)},
		{NewBoolOfferFromOffers(NewBoolOffer(false), NewBoolOffer(true)), NewBoolOffer(true)},
		{NewBoolOfferFromOffers(NewBoolOffer(false), NewBoolOffer(false)), NewBoolOffer(false)},
	} {
		assert.Equal(t, test.expected, test.actual, fmt.Sprintf("Test case %d failed", i))
	}
}

func TestNewStringOfferFromOffers(t *testing.T) {
	for i, test := range []struct {
		actual   Offer
		expected Offer
	}{
		{NewStringOfferFromOffers(NewStringOffer("foo", "bar", "foo")), NewStringOffer("bar", "foo")},
		{NewStringOfferFromOffers(NewStringOffer("foo", "bar", "baz")), NewStringOffer("bar", "baz", "foo")},
	} {
		if sOffer, ok := test.actual.(*stringOffer); ok {
			sort.Strings(sOffer.Offers)
			test.actual = sOffer
		}
		assert.Equal(t, test.expected, test.actual, fmt.Sprintf("Test case %d failed", i))
	}
}

func TestGetType(t *testing.T) {
	for i, test := range []struct {
		actual   Request
		expected Type
	}{
		{NewIntRequest(6), "int"},
		{NewBoolRequest(true), "bool"},
		{NewStringRequest("bar"), "string"},
		{NewLabelRequestMustCompile("performance = gold"), "label"},
	} {
		assert.Equal(t, test.expected, test.actual.GetType(), fmt.Sprintf("Test case %d failed", i))
	}
}

func TestValue(t *testing.T) {
	for i, test := range []struct {
		actual   Request
		expected interface{}
	}{
		{NewIntRequest(6), 6},
		{NewBoolRequest(false), false},
		{NewStringRequest("baz"), "baz"},
		{NewLabelRequestMustCompile("performance = gold"), "performance = gold"},
	} {
		assert.Equal(t, test.expected, test.actual.Value(), fmt.Sprintf("Test case %d failed", i))
	}
}

func TestLabels(t *testing.T) {
	offer := NewLabelOffer(map[string]string{"performance": "silver", "protection": "minimal"})

	actualMap := offer.(*labelOffer).Labels()
	expectedMap := map[string]string{"performance": "silver", "protection": "minimal"}

	assert.Equal(t, expectedMap, actualMap)
}

func TestCreateBackendStoragePoolsMapFromEncodedString(t *testing.T) {
	actualMap, _ := CreateBackendStoragePoolsMapFromEncodedString("backend1:pool1,pool2;backend2:pool3")
	targetMap := map[string][]string{"backend1": {"pool1", "pool2"}, "backend2": {"pool3"}}

	assert.Equal(t, targetMap, actualMap, "Test case failed")
}

func TestCreateBackendStoragePoolsMapFromEncodedStringNegative(t *testing.T) {
	_, actualErr := CreateBackendStoragePoolsMapFromEncodedString("backend1;backend2:pool3")
	expectedErr := fmt.Errorf("the encoded backend-storage pool string does not have the right format")

	assert.Equal(t, expectedErr, actualErr, "Test case failed")
}

func TestBoolString(t *testing.T) {
	boolOffer := boolOffer{
		Offer: true,
	}
	assert.Equal(t, "true", boolOffer.ToString())
	assert.Equal(t, "{Offer:  true}", boolOffer.String())
}

func TestString(t *testing.T) {
	stringOffer := stringOffer{
		Offers: []string{"foo", "bar", "baz"},
	}

	assert.Equal(t, "foo,bar,baz", stringOffer.ToString())
	assert.Equal(t, "{Offers: foo,bar,baz}", stringOffer.String())
}

func TestLabelString(t *testing.T) {
	labelOffer := labelOffer{
		Offers: map[string]string{"performance": "silver", "protection": "minimal"},
	}
	assert.Equal(t, "map[performance:silver protection:minimal]", labelOffer.ToString())
	assert.Equal(t, "{Offers: map[performance:silver protection:minimal]}", labelOffer.String())
}

func TestIntString(t *testing.T) {
	intOffer := intOffer{
		Min: 6,
		Max: 10,
	}
	assert.Equal(t, "{Min: 6, Max: 10}", intOffer.ToString())
}

func TestNewLabelRequestNegative(t *testing.T) {
	for i, test := range []struct {
		requestString string
		expectedErr   error
	}{
		{"", fmt.Errorf("label selector may not be empty")},
		{"performance=gold+protection=full", fmt.Errorf("invalid label selector: performance=gold+protection=full")},
	} {
		_, actualErr := NewLabelRequest(test.requestString)
		assert.Equal(t, test.expectedErr, actualErr, fmt.Sprintf("Test case %d failed", i))
	}
}

func TestUnmarshalRequestMapNil(t *testing.T) {
	_, actualErr := UnmarshalRequestMap(nil)
	expectedErr := error(nil)

	assert.Equal(t, expectedErr, actualErr, "Test case failed")
}

func TestUnmarshalRequestMapNegative(t *testing.T) {
	for i, test := range []struct {
		requestMap  json.RawMessage
		expectedErr error
	}{
		{[]byte("bar"), fmt.Errorf(
			"unable to unmarshal map: invalid character 'b' looking for beginning of value")},
		{[]byte(`{"ExistentBool":"true"}`), fmt.Errorf("unrecognized storage attribute: ExistentBool")},
	} {
		_, actualErr := UnmarshalRequestMap(test.requestMap)
		assert.Equal(t, test.expectedErr, actualErr, fmt.Sprintf("Test case %d failed", i))
	}
}

func TestMarshalRequestMapNil(t *testing.T) {
	_, actualErr := MarshalRequestMap(nil)
	expectedErr := error(nil)

	assert.Equal(t, expectedErr, actualErr, "Test case failed")
}

func TestCreateAttributeRequestFromAttributeValueNegative(t *testing.T) {
	for i, test := range []struct {
		name        string
		val         string
		expectedErr error
	}{
		{"snapshots", "No", fmt.Errorf(
			"storage attribute value (No) doesn't match the specified type (" +
				"bool); strconv.ParseBool: parsing \"No\": invalid syntax")},
		{"IOPS", "123A", fmt.Errorf(
			"storage attribute value (123A) doesn't match the specified type (int); strconv." +
				"ParseInt: parsing \"123A\": invalid syntax")},
		{"labels", "!performance, !protection", fmt.Errorf(
			"storage attribute value (!performance, !protection) doesn't match the specified type (" +
				"label); invalid label selector: !performance, !protection")},
	} {
		_, actualErr := CreateAttributeRequestFromAttributeValue(test.name, test.val)
		assert.Equal(t, test.expectedErr, actualErr, fmt.Sprintf("Test case %d failed", i))
	}
}

func TestUnmarshalOfferMapNegative(t *testing.T) {
	for i, test := range []struct {
		offerMap    json.RawMessage
		expectedErr error
	}{
		{[]byte("bar"), fmt.Errorf(
			"unable to unmarshal map: invalid character 'b' looking for beginning of value")},
		{[]byte(`{"ExistentBool":"true"}`), fmt.Errorf("unknown storage attribute: ExistentBool")},
		{[]byte(`{"IOPS":"123A"}`), fmt.Errorf(
			"unable to fully unmarshal request IOPS: json: cannot unmarshal string into Go value of type " +
				"storageattribute.intOffer")},
	} {
		_, actualErr := UnmarshalOfferMap(test.offerMap)
		assert.Equal(t, test.expectedErr, actualErr, fmt.Sprintf("Test case %d failed", i))
	}
}
