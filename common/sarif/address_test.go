package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_new_simple_address(t *testing.T) {

	address := NewAddress().
		WithIndex(1).
		WithName("google").
		WithFullyQualifiedName("https://www.google.com")

	assert.Equal(t, `{"index":1,"name":"google","fullyQualifiedName":"https://www.google.com"}`, getJsonString(address))
}

func Test_create_new_absolute_address(t *testing.T) {

	address := NewAddress().
		WithIndex(1).
		WithName("google").
		WithAbsoluteAddress(1).
		WithKind("url").
		WithLength(10)

	assert.Equal(t, `{"index":1,"absoluteAddress":1,"length":10,"name":"google","kind":"url"}`, getJsonString(address))
}
