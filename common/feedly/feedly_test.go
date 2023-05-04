package feedly

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLoad(t *testing.T) {
	outlines, err := LoadOutlinesFromBindata()
	if err != nil {
		t.FailNow()
	}

	test := assert.New(t)
	test.True(len(outlines) > 0)
}
