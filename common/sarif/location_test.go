package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_simple_location(t *testing.T) {
	l := NewLocation().
		WithId(1).
		WithMessage(
			NewMessage().
				WithID("messageId1").
				WithText("message text")).
		WithAnnotations(
			[]*Region{NewRegion().WithByteLength(10)},
		).
		WithRelationships([]*LocationRelationship{NewLocationRelationship(1)})

	assert.Equal(t, `{"id":1,"message":{"text":"message text","id":"messageId1"},"annotations":[{"byteLength":10}],"relationships":[{"target":1}]}`, getJsonString(l))
}
