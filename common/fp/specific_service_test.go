package fp

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWithFingerprintRule_Redis(t *testing.T) {
	t.SkipNow()

	//log.SetLevel(golog.DebugLevel)

	test := assert.New(t)
	matcher, err := NewFingerprintMatcher(nil, nil)
	if !test.Nil(err) {
		t.FailNow()
	}

	result, err := matcher.Match("10.3.0.127", 6379)
	if !test.Nil(err) {
		t.FailNow()
	}

	spew.Dump(result)
}
