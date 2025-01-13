// Copyright 2025 NetApp, Inc. All Rights Reserved.

package convert

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
)

func TestToTitle(t *testing.T) {
	logging.Log().Debug("Running TestTitle...")

	testCases := []struct {
		Text           string
		ExpectedResult string
	}{
		{"foo", "Foo"},
		{"Foo", "Foo"},
		{"foo bar", "Foo Bar"},
		{"Foo bar", "Foo Bar"},
	}

	for _, testCase := range testCases {
		result := ToTitle(testCase.Text)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestToPtr(t *testing.T) {
	i := 42
	pi := ToPtr(i)
	assert.Equal(t, i, *pi)

	s := "test"
	ps := ToPtr(s)
	assert.Equal(t, s, *ps)

	a := [2]int{1, 2}
	pa := ToPtr(a)
	assert.Equal(t, a, *pa)
}

func TestToSlicePtrs(t *testing.T) {
	slice := []string{"hello", "world"}
	result := ToSlicePtrs(slice)
	assert.True(t, len(slice) == len(result))
	assert.Equal(t, slice[0], *result[0])
	assert.Equal(t, slice[1], *result[1])
}

func TestToPrintableBoolPtr(t *testing.T) {
	var bPtr *bool
	pval := ToPrintableBoolPtr(bPtr)
	assert.Equal(t, "none", pval)

	tmp := false
	bPtr = &tmp
	pval = ToPrintableBoolPtr(bPtr)
	assert.Equal(t, "false", pval)

	tmp = true
	pval = ToPrintableBoolPtr(bPtr)
	assert.Equal(t, "true", pval)
}

func TestToFormattedBool(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{"Valid uppercase bool value", "TRUE", "true", false},
		{"Valid Camelcase bool value", "True", "true", false},
		{"Invalid bool value", "TrUe", "TrUe", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, responseErr := ToFormattedBool(test.input)

			assert.Equal(t, test.expected, response)
			if test.expectErr {
				assert.Error(t, responseErr)
			} else {
				assert.Nil(t, responseErr)
			}
		})
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		b        string
		expected bool
	}{
		{
			b:        "true",
			expected: true,
		},
		{
			b:        "false",
			expected: false,
		},
		{
			b:        "not a value",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.b, func(t *testing.T) {
			actual := ToBool(test.b)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestObjectToBase64String_Fails(t *testing.T) {
	// Object is nil.
	encodedObj, err := ObjectToBase64String(nil)
	assert.Empty(t, encodedObj)
	assert.Error(t, err)

	// Object is an unmarshal-able type.
	encodedObj, err = ObjectToBase64String(func() {})
	assert.Empty(t, encodedObj)
	assert.Error(t, err)
}

func TestObjectToBase64String_Succeeds(t *testing.T) {
	type testObject struct {
		Foo string `json:"foo"`
		Bar string `json:"bar"`
		Baz string `json:"baz,omitempty"`
	}

	// Object is non-nil, but empty.
	encodedObj, err := ObjectToBase64String(testObject{})
	assert.NotNil(t, encodedObj)
	assert.NoError(t, err)

	// Object is an object with fields filled in.
	obj := testObject{
		Foo: "foo_test",
		Bar: "bar_test",
		Baz: "baz_test",
	}
	encodedObj, err = ObjectToBase64String(obj)
	assert.NotNil(t, encodedObj)
	assert.NoError(t, err)
}

func TestBase64StringToObject_Fails(t *testing.T) {
	type testObject struct {
		Foo string `json:"foo"`
		Bar string `json:"bar"`
		Baz string `json:"baz,omitempty"`
	}

	// Encoded object is an empty string.
	actualObject := testObject{}
	err := Base64StringToObject("", &actualObject)
	assert.Empty(t, actualObject.Foo)
	assert.Empty(t, actualObject.Bar)
	assert.Empty(t, actualObject.Baz)
	assert.Error(t, err)

	// Encoded object is an invalid value for a base64 string.
	actualObject = testObject{}
	err = Base64StringToObject("%", &actualObject)
	assert.Empty(t, actualObject.Foo)
	assert.Empty(t, actualObject.Bar)
	assert.Empty(t, actualObject.Baz)
	assert.Error(t, err)

	// Encoded object contains non-ASCII characters for a base64 string.
	actualObject = testObject{}
	err = Base64StringToObject("ß-11234567890987654321234567890", &actualObject)
	assert.Empty(t, actualObject.Foo)
	assert.Empty(t, actualObject.Bar)
	assert.Empty(t, actualObject.Baz)
	assert.Error(t, err)
}

func TestBase64StringToObject_Succeeds(t *testing.T) {
	type testObject struct {
		Foo string `json:"foo"`
		Bar string `json:"bar"`
		Baz string `json:"baz,omitempty"`
	}

	// Encoded object is an empty string.
	actualObject := testObject{}
	expectedObject := testObject{Foo: "foo_test", Bar: "bar_test", Baz: "baz_test"}
	err := Base64StringToObject(
		"eyJmb28iOiJmb29fdGVzdCIsImJhciI6ImJhcl90ZXN0IiwiYmF6IjoiYmF6X3Rlc3QifQ==",
		&actualObject,
	)
	assert.EqualValues(t, expectedObject, actualObject)
	assert.NoError(t, err)

	// Encoded object is an empty string.
	actualObject = testObject{}
	expectedObject = testObject{Foo: "foo_test", Bar: "bar_test", Baz: "baz_test"}
	err = Base64StringToObject(
		"eyJmb28iOiJmb29fdGVzdCIsImJhciI6ImJhcl90ZXN0IiwiYmF6IjoiYmF6X3Rlc3QifQ==",
		&actualObject,
	)
	assert.EqualValues(t, expectedObject, actualObject)
	assert.NoError(t, err)
}

func TestEncodeAndDecodeToAndFromBase64(t *testing.T) {
	type testObject struct {
		Foo string `json:"foo"`
		Bar string `json:"bar"`
		Baz string `json:"baz,omitempty"`
	}

	// Create a test object and encoded it.
	originalObject := testObject{Foo: "foo_test", Bar: "bar_test", Baz: "baz_test"}
	encodedObject, err := ObjectToBase64String(originalObject)
	assert.NoError(t, err)
	assert.NotNil(t, encodedObject)

	// Decode the encoded test object and ensure the values extracted object and its values are equivalent to
	// those present in the original object.
	var actualObject testObject
	err = Base64StringToObject(encodedObject, &actualObject)
	assert.NoError(t, err)
	assert.NotNil(t, encodedObject)
	assert.Equal(t, originalObject.Foo, actualObject.Foo)
	assert.Equal(t, originalObject.Bar, actualObject.Bar)
	assert.Equal(t, originalObject.Baz, actualObject.Baz)
}

func TestParseIntInRange(t *testing.T) {
	tests := []struct {
		name               string
		val                string
		min, max, expected int64
		errContains        string
	}{
		{
			name:     "positive in range",
			val:      "5",
			min:      0,
			max:      10,
			expected: 5,
		},
		{
			name:     "negative in range",
			val:      "-5",
			min:      -10,
			max:      0,
			expected: -5,
		},
		{
			name:        "beyond range",
			val:         "11",
			min:         0,
			max:         10,
			expected:    0,
			errContains: "is out of range",
		},
		{
			name:        "below range",
			val:         "-11",
			min:         -10,
			max:         0,
			expected:    0,
			errContains: "is out of range",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			i, err := parseIntInRange(test.val, test.min, test.max)
			assert.Equal(t, test.expected, i)
			if test.errContains == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
			}
		})
	}
}

func TestRedactSecretsFromString(t *testing.T) {
	passphrase := "chap-initiator-secret"
	outboundPassphrase := "chap-target-initiator-secret"
	username := "chap-username"
	outboundUsername := "chap-target-username"

	sampleZapiRequestFormatString := `sending to '10.211.55.19' xml: \n<?xml version="1.0" encoding="UTF-8"?>\n\t\t
<netapp xmlns="http://www.netapp.com/filer/admin\" version="1.21" vfiler="datavserver">\n
<iscsi-initiator-set-default-auth>\n     <auth-type>CHAP</auth-type>\n
<outbound-passphrase>%s</outbound-passphrase>\n
<outbound-user-name>%s</outbound-user-name>\n
<passphrase>%s</passphrase>\n     <user-name>%s</user-name>\n
</iscsi-initiator-set-default-auth>\n          </netapp>`
	sampleZapiRequest := fmt.Sprintf(sampleZapiRequestFormatString, outboundPassphrase, outboundUsername, passphrase,
		username)
	secretFields := []string{"outbound-passphrase", "outbound-user-name", "passphrase", "user-name"}
	requestReplacements := make(map[string]string)
	for _, f := range secretFields {
		fmtString := "<%s>%s</%s>"
		requestReplacements[fmt.Sprintf(fmtString, f, ".*", f)] = fmt.Sprintf(fmtString, f, config.REDACTED, f)
	}

	expectedRedactedZapiRequest := fmt.Sprintf(sampleZapiRequestFormatString, config.REDACTED, config.REDACTED,
		config.REDACTED, config.REDACTED)

	xmlPassphraseString := "<outbound-user-name></outbound-user-name></outbound-user-name>"

	xmlExpected := fmt.Sprintf("<outbound-user-name>%s</outbound-user-name>", config.REDACTED)
	xmlReplacements := map[string]string{"<outbound-user-name>.*</outbound-user-name>": xmlExpected}

	xmlTagFormatString := "<%s>%s</%s>"
	outboundPassphraseTag := fmt.Sprintf(xmlTagFormatString, "outbound-passphrase", "%s", "outbound-passphrase")

	noRegexPassphrase := "fdsxchj4d@"
	noRegexPassphraseXml := fmt.Sprintf(outboundPassphraseTag, noRegexPassphrase)
	noRegexExpectedXml := fmt.Sprintf(outboundPassphraseTag, config.REDACTED)
	noRegexReplacements := map[string]string{
		noRegexPassphraseXml: noRegexExpectedXml,
	}

	uncompilableRegexPassphrase := "<?$!abjghjd()^>[</?$!abjghjd()^>"
	uncompilableRegexExpectedString := "regex matching the secret could not compile, so the entire string has been" +
		" redacted"
	uncompilableRegexPassphraseTag := fmt.Sprintf(outboundPassphraseTag, uncompilableRegexPassphrase)
	uncompilableRegexReplacements := map[string]string{
		uncompilableRegexPassphraseTag: "won't be used",
	}

	testCases := []struct {
		description   string
		input         string
		expected      string
		replacements  map[string]string
		useRegex      bool
		assertMessage string
	}{
		{
			"ZAPI request in bug report", sampleZapiRequest, expectedRedactedZapiRequest,
			requestReplacements, true, "expected that the sample usernames" +
				" and passwords from the bug report are redacted in the output string",
		},
		{
			"Passphrase is equal to the closing XML tag", xmlPassphraseString, xmlExpected,
			xmlReplacements, true, "expected that only the passphrase portion of the" +
				" xml string to be redacted when the passphrase is equal to the ending tag",
		},
		{
			"Replacement works properly when a regular expression is not used", noRegexPassphraseXml,
			noRegexExpectedXml, noRegexReplacements, false, "expect only the" +
				" passphrase to be redacted when regex matching is not used",
		},
		{
			"Safe string is returned when regex is invalid", uncompilableRegexPassphrase,
			uncompilableRegexExpectedString, uncompilableRegexReplacements, true,
			"expect the invalid regex error string when the provided regex cannot be compiled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actualString := RedactSecretsFromString(tc.input, tc.replacements, tc.useRegex)
			assert.Equal(t, tc.expected, actualString, tc.assertMessage)
		})
	}
}

func TestTruncateString(t *testing.T) {
	type TestData struct {
		Input  string
		Length int
		Output string
	}

	data := []TestData{
		{"", 0, ""},
		{"", 1, ""},
		{"text", 10, "text"},
		{"text", 3, "tex"},
		{" text ", 10, " text "},
		{
			"a123456789b123456789c123456789d123456789e123456789f123456789g123456789",
			63,
			"a123456789b123456789c123456789d123456789e123456789f123456789g12",
		},
	}

	for _, d := range data {
		result := TruncateString(d.Input, d.Length)
		assert.Equal(t, d.Output, result)
	}
}
