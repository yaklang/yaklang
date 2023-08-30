package numrange

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestParseNumRange(t *testing.T) {
	assert.Equal(t, &NumRange{Op: '=', Num1: 1}, must(ParseNumRange("1")))
	assert.Equal(t, &NumRange{Op: '!', Num1: 1}, must(ParseNumRange("!1")))
	assert.Equal(t, &NumRange{Op: '<', Num1: 1}, must(ParseNumRange("<1")))
	assert.Equal(t, &NumRange{Op: '>', Num1: 1}, must(ParseNumRange(">1")))
	assert.Equal(t, &NumRange{Op: '-', Num1: 1, Num2: 2}, must(ParseNumRange("1-2")))
	assert.Equal(t, &NumRange{Op: '-', Num1: 1, Num2: 2}, must(ParseNumRange("1<>2")))
}

func TestNumRange_Match(t *testing.T) {
	assert.Equal(t, true, must(ParseNumRange("1")).Match(1))
	assert.Equal(t, false, must(ParseNumRange("1")).Match(2))
	assert.Equal(t, true, must(ParseNumRange("!1")).Match(2))
	assert.Equal(t, false, must(ParseNumRange("!1")).Match(1))
	assert.Equal(t, true, must(ParseNumRange("<1")).Match(0))
	assert.Equal(t, false, must(ParseNumRange("<1")).Match(1))
	assert.Equal(t, true, must(ParseNumRange(">1")).Match(2))
	assert.Equal(t, false, must(ParseNumRange(">1")).Match(1))
	assert.Equal(t, true, must(ParseNumRange("1-2")).Match(1))
	assert.Equal(t, true, must(ParseNumRange("1-2")).Match(2))
	assert.Equal(t, false, must(ParseNumRange("1-2")).Match(3))
	assert.Equal(t, true, must(ParseNumRange("1<>2")).Match(1))
	assert.Equal(t, true, must(ParseNumRange("1<>2")).Match(2))
	assert.Equal(t, false, must(ParseNumRange("1<>2")).Match(3))
}

func must[T any](val T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return val
}
