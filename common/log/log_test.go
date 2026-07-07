package log

import (
	"testing"

	"github.com/kataras/golog"
	"github.com/stretchr/testify/suite"
)

func TestLog(t *testing.T) { suite.Run(t, &LogTest{}) }

type LogTest struct{ suite.Suite }

func (l *LogTest) TestParseLevel() {
	assert := l.Require()
	level, err := ParseLevel("info")
	assert.Nil(err)
	assert.Equal(golog.InfoLevel, level)
	level, err = ParseLevel("aaa")
	assert.Error(err, ErrUnknowLevel.Error())
	level, err = ParseLevel("disable")
	assert.Nil(err)
	assert.Equal(golog.DisableLevel, level)
}
