package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCaseFoldedAnnotation(t *testing.T) {
	ann := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"KEY3": "value3",
	}
	assert.NotEmpty(t, getCaseFoldedAnnotation(ann, "KEY1"))
	assert.NotEmpty(t, getCaseFoldedAnnotation(ann, "key3"))
	assert.Empty(t, getCaseFoldedAnnotation(ann, "key4"))
}
