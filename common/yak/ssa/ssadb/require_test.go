package ssadb

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"testing"
)

func TestRequire(t *testing.T) {
	id, code := RequireIrCode(consts.GetGormProjectDatabase())
	spew.Dump(code)
	assert.Greater(t, uint64(id), uint64(0))
}
