package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	step := &DefaultStep{}
	assert.Equal(t, "default step", step.GetName())
	assert.Equal(t, false, step.IsRequired())
	assert.Nil(t, step.Apply(nil))
}

func TestDefaultValues(t *testing.T) {
	step := &DefaultStep{Name: "test", Required: true}
	assert.Equal(t, "test", step.GetName())
	assert.Equal(t, true, step.IsRequired())
	assert.Nil(t, step.Apply(nil))
}
