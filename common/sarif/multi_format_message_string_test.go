package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_multi_format_message_string(t *testing.T) {

	msg := NewMultiformatMessageString("mock plain text")

	assert.Equal(t, `{"text":"mock plain text"}`, getJsonString(msg))
}

func Test_create_multi_format_message_string_with_markdown(t *testing.T) {

	msg := NewMultiformatMessageString("mock plain text").
		WithMarkdown("mock markdown text")

	assert.Equal(t, `{"text":"mock plain text","markdown":"mock markdown text"}`, getJsonString(msg))
}
