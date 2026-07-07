package guard

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestApache(t *testing.T) {
	test := assert.New(t)
	raw, err := searchApacheProcess(context.Background())
	if err != nil {
		test.FailNow("Get Apache Process failed", err.Error())
	}
	_ = raw
	spew.Dump(getApachePid(context.Background()))

	ds := GetApacheDetail(context.Background())
	spew.Dump(ds)
}
