// Copyright 2025 NetApp, Inc. All Rights Reserved.

package yaml

import (
	"regexp"
	"strings"
)

func CountSpacesBeforeText(text string) int {
	return len(text) - len(strings.TrimLeft(text, " \t"))
}

func GetTagWithSpaceCount(text, tagName string) (string, int) {
	// This matches pattern in a multiline string of type "    {something}\n"
	tagsWithIndentationRegex := regexp.MustCompile(`(?m)^[\t ]*{` + tagName + `}$\n`)
	tag := tagsWithIndentationRegex.FindStringSubmatch(text)

	// Since we have two of `()` in the pattern, we want to use the tag identified by the second `()`.
	if len(tag) > 0 {
		tagWithSpaces := tagsWithIndentationRegex.FindString(text)
		indentation := CountSpacesBeforeText(tagWithSpaces)

		return tagWithSpaces, indentation
	}

	return "", 0
}

func ReplaceMultilineTag(originalYAML, tag, tagText string) string {
	for {
		tagWithSpaces, spaceCount := GetTagWithSpaceCount(originalYAML, tag)

		if tagWithSpaces == "" {
			break
		}
		originalYAML = strings.Replace(originalYAML, tagWithSpaces, shiftTextRight(tagText, spaceCount), 1)
	}

	return originalYAML
}

func shiftTextRight(text string, count int) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	newLines := make([]string, len(lines))
	for i, line := range lines {
		if line != "" {
			newLines[i] = strings.Repeat(" ", count) + line
		}
	}

	return strings.Join(newLines, "\n")
}
