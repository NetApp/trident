// Copyright 2024 NetApp, Inc. All Rights Reserved.

package collection

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testStringSlice = []string{
	"foo",
	"bar",
	"earn",
	"baz",
	"con:1234",
	"silicon:1234",
	"bigstring",
	"verybigstring",
	"superbingstring",
}

var testIntSlice = []int{
	1,
	-1,
	9999,
	0,
	56355634,
}

type CustomString string

var testCustomStringSlice = []CustomString{
	"foo",
	"bar",
	"earn",
	"baz",
	"con:1234",
	"silicon:1234",
	"bigstring",
	"verybigstring",
	"superbingstring",
}

func TestContains(t *testing.T) {
	testCases := []struct {
		SliceName  interface{}
		SliceValue interface{}
		Contains   bool
	}{
		// Positive Cases
		{testStringSlice, "foo", true},
		{testStringSlice, "silicon:1234", true},
		{testCustomStringSlice, CustomString("foo"), true},
		{testCustomStringSlice, CustomString("silicon:1234"), true},
		{testIntSlice, -1, true},
		{testIntSlice, 0, true},
		{testIntSlice, 56355634, true},

		// Negative Cases
		{testStringSlice, "Foo", false},
		{testStringSlice, "foobar", false},
		{testStringSlice, "sIliCon:1234", false},
		{testStringSlice, "silicon:12344", false},
		{testStringSlice, "", false},
		{testCustomStringSlice, "foo", false},
		{testCustomStringSlice, "silicon:1234", false},
		{testCustomStringSlice, CustomString("Foo"), false},
		{testCustomStringSlice, CustomString("foobar"), false},
		{testCustomStringSlice, CustomString("sIliCon:1234"), false},
		{testCustomStringSlice, CustomString("silicon:12344"), false},
		{testCustomStringSlice, CustomString(""), false},
		{testIntSlice, "-1", false},
		{testIntSlice, -2, false},
		{testIntSlice, -9999, false},
		{testIntSlice, 100000, false},
	}

	for _, testCase := range testCases {
		contains := Contains(testCase.SliceName, testCase.SliceValue)
		assert.Equal(t, testCase.Contains, contains)
	}
}

func TestContainsString(t *testing.T) {
	testCases := []struct {
		Text           string
		ExpectedResult bool
	}{
		// Positive Cases
		{"foo", true},
		{"silicon:1234", true},

		// Negative Cases
		{"Foo", false},
		{"foobar", false},
		{"sIliCon:1234", false},
		{"silicon:12344", false},
		{"", false},
	}

	for _, testCase := range testCases {
		contains := ContainsString(testStringSlice, testCase.Text)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestContainsStringCaseInsensitive(t *testing.T) {
	testCases := []struct {
		Text           string
		ExpectedResult bool
	}{
		// Positive Cases
		{"foo", true},
		{"Foo", true},
		{"silicon:1234", true},
		{"sIliCon:1234", true},

		// Negative Cases
		{"foobar", false},
		{"silicon:12344", false},
		{"", false},
	}

	for _, testCase := range testCases {
		contains := ContainsStringCaseInsensitive(testStringSlice, testCase.Text)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestContainsStringConditionally(t *testing.T) {
	testCases := []struct {
		Text           string
		MatchFunc      func(string, string) bool
		ExpectedResult bool
	}{
		{"foo", func(val1, val2 string) bool { return val1 == val2 }, true},
		{"Foo", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Foo", func(val1, val2 string) bool { return strings.EqualFold(val1, val2) }, true},
		{"ear", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Ear", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Ear", func(val1, val2 string) bool { return strings.EqualFold(val1, val2) }, false},
		{"ear", func(val1, val2 string) bool { return strings.Contains(val1, val2) }, true},
		{"Ear", func(val1, val2 string) bool { return strings.Contains(val1, val2) }, false},
		{
			"Ear",
			func(val1, val2 string) bool { return strings.Contains(strings.ToLower(val1), strings.ToLower(val2)) },
			true,
		},
	}

	for _, testCase := range testCases {
		contains := ContainsStringConditionally(testStringSlice, testCase.Text, testCase.MatchFunc)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestContainsElements(t *testing.T) {
	testCases := []struct {
		SliceName    interface{}
		SliceValues  interface{}
		ContainsAll  bool
		ContainsSome bool
	}{
		// ContainsAll
		{testStringSlice, testStringSlice, true, true},
		{testStringSlice, testStringSlice[1:4], true, true},
		{testStringSlice, append(testStringSlice, "foo"), true, true},
		{append(testStringSlice, "foo"), testStringSlice, true, true},
		{testStringSlice, []string{"foo", "bar"}, true, true},
		{testStringSlice, []string{"silicon:1234"}, true, true},

		{testCustomStringSlice, testCustomStringSlice, true, true},
		{testCustomStringSlice, testCustomStringSlice[1:4], true, true},
		{testCustomStringSlice, append(testCustomStringSlice, "foo"), true, true},
		{append(testCustomStringSlice, "foo"), testCustomStringSlice, true, true},
		{testCustomStringSlice, []CustomString{"foo", "bar"}, true, true},
		{testCustomStringSlice, []CustomString{"silicon:1234"}, true, true},

		{testIntSlice, testIntSlice, true, true},
		{testIntSlice, testIntSlice[1:3], true, true},
		{testIntSlice, append(testIntSlice, 1), true, true},
		{append(testIntSlice, 1), testIntSlice, true, true},
		{testIntSlice, []int{1, 56355634}, true, true},
		{testIntSlice, []int{9999}, true, true},

		// ContainsSome
		{testStringSlice, append(testStringSlice, "new"), false, true},
		{[]string{"foo", "bar"}, testStringSlice, false, true},
		{testStringSlice, []string{"foo", "bar1"}, false, true},
		{testStringSlice, []string{"", "foo"}, false, true},

		{testCustomStringSlice, append(testCustomStringSlice, "new"), false, true},
		{[]CustomString{"foo", "bar"}, testCustomStringSlice, false, true},
		{testCustomStringSlice, []CustomString{"foo", "bar1"}, false, true},
		{testCustomStringSlice, []CustomString{"", "foo"}, false, true},

		{testIntSlice, append(testIntSlice, 2), false, true},
		{[]int{1, 56355634}, testIntSlice, false, true},
		{testIntSlice, []int{1, 563556341}, false, true},
		{testIntSlice, []int{2, 56355634}, false, true},

		// Negative Cases
		{testStringSlice, []string{}, false, false},
		{testStringSlice, "", false, false},
		{[]string{}, testStringSlice, false, false},
		{"", testStringSlice, false, false},
		{"", "", false, false},
		{testStringSlice, "foo", false, false},
		{"foo", testStringSlice, false, false},
		{"foo", "foo", false, false},
		{"foo", "", false, false},
		{testStringSlice, []string{"foo1", "bar1"}, false, false},
		{testStringSlice, []string{"sIliCon:1234"}, false, false},
		{testStringSlice, []string{"foobar"}, false, false},
		{testStringSlice, testCustomStringSlice, false, false},
		{testStringSlice, testIntSlice, false, false},

		{testCustomStringSlice, []CustomString{}, false, false},
		{testCustomStringSlice, "", false, false},
		{[]CustomString{}, testCustomStringSlice, false, false},
		{"", testCustomStringSlice, false, false},
		{"", "", false, false},
		{testCustomStringSlice, "foo", false, false},
		{"foo", testCustomStringSlice, false, false},
		{"foo", "foo", false, false},
		{"foo", "", false, false},
		{testCustomStringSlice, []CustomString{"foo1", "bar1"}, false, false},
		{testCustomStringSlice, []CustomString{"sIliCon:1234"}, false, false},
		{testCustomStringSlice, []CustomString{"foobar"}, false, false},
		{testCustomStringSlice, testStringSlice, false, false},
		{testCustomStringSlice, testIntSlice, false, false},

		{testIntSlice, []int{}, false, false},
		{testIntSlice, "", false, false},
		{testIntSlice, 0, false, false},
		{[]int{}, testIntSlice, false, false},
		{"", testIntSlice, false, false},
		{0, testIntSlice, false, false},
		{"", "", false, false},
		{0, 0, false, false},
		{testIntSlice, 1, false, false},
		{1, testIntSlice, false, false},
		{1, 1, false, false},
		{1, "", false, false},
		{1, 0, false, false},
		{testIntSlice, []int{2, -2}, false, false},
		{testIntSlice, []int{100000}, false, false},
		{testIntSlice, []int{-9999}, false, false},
		{testIntSlice, testStringSlice, false, false},
		{testIntSlice, testCustomStringSlice, false, false},
	}

	for _, testCase := range testCases {
		containsAll, containsSome := ContainsElements(testCase.SliceName, testCase.SliceValues)
		assert.Equal(t, testCase.ContainsAll, containsAll, "containsAll mismatch")
		assert.Equal(t, testCase.ContainsSome, containsSome, "containsSome mismatch")
	}
}

func TestRemoveString(t *testing.T) {
	slice := []string{
		"foo",
		"bar",
		"baz",
	}
	updatedSlice := slice

	updatedSlice = RemoveString(updatedSlice, "foo")
	if ContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "foo")
	}

	updatedSlice = RemoveString(updatedSlice, "bar")
	if ContainsString(updatedSlice, "bar") {
		t.Errorf("Slice should NOT contain string %v", "bar")
	}

	updatedSlice = RemoveString(updatedSlice, "baz")
	if ContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "baz")
	}

	if len(updatedSlice) != 0 {
		t.Errorf("Slice should be empty")
	}
}

func TestRemoveStringConditionally(t *testing.T) {
	updatedSlice := make([]string, len(testStringSlice))
	copy(updatedSlice, testStringSlice)

	updatedSlice = RemoveStringConditionally(updatedSlice, "foo",
		func(val1, val2 string) bool { return val1 == val2 })
	if ContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "foo")
	}

	lenBefore := len(updatedSlice)
	updatedSlice = RemoveStringConditionally(updatedSlice, "random",
		func(val1, val2 string) bool { return val1 == val2 })
	lenAfter := len(updatedSlice)
	if lenBefore != lenAfter {
		t.Errorf("Slice should have NOT removed element(s)")
	}

	updatedSlice = RemoveStringConditionally(updatedSlice, "earnest",
		func(main, val string) bool { return strings.Contains(main, val) })
	if ContainsString(updatedSlice, "ear") {
		t.Errorf("Slice should NOT contain string %v", "ear")
	}
	if ContainsString(updatedSlice, "earn") {
		t.Errorf("Slice should NOT contain string %v", "earn")
	}

	updatedSlice = RemoveStringConditionally(updatedSlice, "con:3421",
		func(main, val string) bool {
			mainIpAddress := strings.Split(main, ":")[0]
			valIpAddress := strings.Split(val, ":")[0]
			return mainIpAddress == valIpAddress
		})
	if ContainsString(updatedSlice, "con:1234") {
		t.Errorf("Slice should NOT contain string %v", "con:1234")
	}
	if !ContainsString(updatedSlice, "silicon:1234") {
		t.Errorf("Slice should contain string %v", "silicon:1234")
	}

	updatedSlice = RemoveStringConditionally(updatedSlice, "bigstring",
		func(main, val string) bool { return len(val) > len(main) })
	if ContainsString(updatedSlice, "verybigstring") {
		t.Errorf("Slice should NOT contain string %v", "verybigstring")
	}
	if ContainsString(updatedSlice, "superbingstring") {
		t.Errorf("Slice should NOT contain string %v", "superbingstring")
	}
}

func TestSplitString(t *testing.T) {
	ctx := context.TODO()

	stringList := SplitString(ctx, "a,b, c", ",")
	assert.Equal(t, []string{"a", "b", " c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", "")
	assert.Equal(t, []string{"a", ",", "b", ",", "c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", ";")
	assert.Equal(t, []string{"a,b,c"}, stringList)

	stringList = SplitString(ctx, "", ",")
	assert.Equal(t, []string(nil), stringList)

	stringList = SplitString(ctx, "", ";")
	assert.Equal(t, []string(nil), stringList)

	stringList = SplitString(ctx, " ", ";")
	assert.Equal(t, []string{" "}, stringList)

	stringList = SplitString(ctx, ";a;b", ";")
	assert.Equal(t, []string{"", "a", "b"}, stringList)
}

func TestReplaceAtIndex(t *testing.T) {
	actual, err := ReplaceAtIndex("foo", 'f', 0)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 0)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("fof", 'o', 2)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', -1)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 3)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 50)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)
}

func TestEqualValues(t *testing.T) {
	tests := map[string]struct {
		s1    []string
		s2    []string
		equal bool
	}{
		"identical slices": {
			s1:    []string{"a", "b", "c"},
			s2:    []string{"a", "b", "c"},
			equal: true,
		},
		"same elements different order": {
			s1:    []string{"c", "a", "b"},
			s2:    []string{"a", "b", "c"},
			equal: true,
		},
		"both nil": {
			s1:    nil,
			s2:    nil,
			equal: true,
		},
		"both empty": {
			s1:    []string{},
			s2:    []string{},
			equal: true,
		},
		"nil and empty are equal": {
			s1:    nil,
			s2:    []string{},
			equal: true,
		},
		"empty and nil are equal": {
			s1:    []string{},
			s2:    nil,
			equal: true,
		},
		"different lengths": {
			s1:    []string{"a", "b"},
			s2:    []string{"a", "b", "c"},
			equal: false,
		},
		"same length different values": {
			s1:    []string{"a", "b", "c"},
			s2:    []string{"a", "b", "d"},
			equal: false,
		},
		"one nil one populated": {
			s1:    nil,
			s2:    []string{"a"},
			equal: false,
		},
		"one empty one populated": {
			s1:    []string{},
			s2:    []string{"a"},
			equal: false,
		},
		"duplicate elements equal cardinality": {
			s1:    []string{"a", "a", "b"},
			s2:    []string{"a", "b", "a"},
			equal: true,
		},
		"duplicate elements unequal cardinality": {
			s1:    []string{"a", "a", "b"},
			s2:    []string{"a", "b", "b"},
			equal: false,
		},
		"single element equal": {
			s1:    []string{"x"},
			s2:    []string{"x"},
			equal: true,
		},
		"single element not equal": {
			s1:    []string{"x"},
			s2:    []string{"y"},
			equal: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.equal, EqualValues(tc.s1, tc.s2))
		})
	}

	// Also test with int type to verify generics work.
	t.Run("int slices same elements different order", func(t *testing.T) {
		assert.True(t, EqualValues([]int{3, 1, 2}, []int{1, 2, 3}))
	})
	t.Run("int slices different values", func(t *testing.T) {
		assert.False(t, EqualValues([]int{1, 2, 3}, []int{1, 2, 4}))
	})
	t.Run("int nil and empty are equal", func(t *testing.T) {
		assert.True(t, EqualValues[int](nil, []int{}))
	})
}

func TestAppendToStringList(t *testing.T) {
	tests := []struct {
		stringList    string
		newItem       string
		sep           string
		newStringList string
	}{
		{"", "xfs", ",", "xfs"},
		{"", "xfs", "/", "xfs"},
		{"", "xfs", " ", "xfs"},
		{"", "xfs", "", "xfs"},
		{"", "", ",", ""},
		{"", "", "/", ""},
		{"", "", " ", ""},
		{"", "", "", ""},
		{"ext3", "xfs", ",", "ext3,xfs"},
		{"ext3", "xfs", "/", "ext3/xfs"},
		{"ext3", "xfs", " ", "ext3 xfs"},
		{"ext3", "xfs", "", "ext3xfs"},
		{"ext3", "xfs,raw", ",", "ext3,xfs,raw"},
		{"ext3", "xfs, raw", "/", "ext3/xfs, raw"},
		{"ext3", "xfs, raw", " ", "ext3 xfs, raw"},
		{"ext3", "xfs, raw", "", "ext3xfs, raw"},
		{"ext3,ext4", "xfs", ",", "ext3,ext4,xfs"},
		{"ext3,ext4", "xfs", "/", "ext3,ext4/xfs"},
		{"ext3,ext4", "xfs", " ", "ext3,ext4 xfs"},
		{"ext3,ext4", "xfs", "", "ext3,ext4xfs"},
		{"ext3, ext4", "xfs", ",", "ext3, ext4,xfs"},
		{"ext3, ext4", "xfs", "/", "ext3, ext4/xfs"},
		{"ext3, ext4", "xfs", " ", "ext3, ext4 xfs"},
		{"ext3, ext4", "xfs", "", "ext3, ext4xfs"},
	}

	for _, test := range tests {
		newStringList := AppendToStringList(test.stringList, test.newItem, test.sep)
		assert.Equal(t, test.newStringList, newStringList)
	}
}
