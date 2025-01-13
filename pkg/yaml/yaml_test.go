// Copyright 2025 NetApp, Inc. All Rights Reserved.

package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const inputString = `
text
{REPLACE-1}
  something: else
  {REPLACE-2}
  {NO-REPLACE-1}x
  c{NO-REPLACE-2}
    something: else
    {NO_REPLACE_3}y
    {REPLACE-3}
    {REPLACE-4}
`

func TestGetTagWithSpaceCount(t *testing.T) {
	tagWithSpaces, spaces := GetTagWithSpaceCount(inputString, "REPLACE-1")
	assert.Equal(t, tagWithSpaces, "{REPLACE-1}\n")
	assert.Equal(t, spaces, 0)

	inputStringCopy := strings.ReplaceAll(inputString, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetTagWithSpaceCount(inputString, "REPLACE-2")
	assert.Equal(t, tagWithSpaces, "  {REPLACE-2}\n")
	assert.Equal(t, spaces, 2)

	inputStringCopy = strings.ReplaceAll(inputStringCopy, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetTagWithSpaceCount(inputString, "REPLACE-3")
	assert.Equal(t, tagWithSpaces, "    {REPLACE-3}\n")
	assert.Equal(t, spaces, 4)

	inputStringCopy = strings.ReplaceAll(inputStringCopy, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetTagWithSpaceCount(inputString, "REPLACE-4")
	assert.Equal(t, tagWithSpaces, "    {REPLACE-4}\n")
	assert.Equal(t, spaces, 4)
}

func TestCountSpacesBeforeText(t *testing.T) {
	type TextWithSpaces struct {
		Text   string
		Spaces int
	}

	multipleTextsWithSpaces := []TextWithSpaces{
		{"text", 0},
		{"  text", 2},
		{"  text  ", 2},
		{"      text", 6},
		{"      text      ", 6},
	}

	for _, textWithSpaces := range multipleTextsWithSpaces {
		spaceCount := CountSpacesBeforeText(textWithSpaces.Text)
		assert.Equal(t, spaceCount, textWithSpaces.Spaces)
	}
}
