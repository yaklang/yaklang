package hostsparser

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCIDRBlock(t *testing.T) {
	test := assert.New(t)
	b, err := newCIDRBlock(context.Background(), "46.2.1.3/24")
	if err != nil {
		test.FailNow(err.Error())
	}
	spew.Dump(b)
}
