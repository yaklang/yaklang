package test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

func TestDetectUnsupportedPython2(t *testing.T) {
	py2 := map[string]string{
		"except binding":  "try:\n    pass\nexcept Exception, e:\n    pass\n",
		"print statement": "print 'hello'\n",
		"ur prefix":       "value = ur'hello'\n",
		"octal literal":   "mode = 0777\n",
		"long literal":    "size = 10L\n",
		"backtick repr":   "name = `value`\n",
		"not equal":       "if a <> b:\n    pass\n",
		"raise comma":     "raise ValueError, 'bad'\n",
		"lambda tuple":    "f = lambda (a, b): a + b\n",
		"print chevron":   "print >> stream, value\n",
	}
	for name, src := range py2 {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, pythonparser.DetectUnsupportedPython2(src), "expected Python 2 detection")
		})
	}

	py3 := map[string]string{
		"except as":    "try:\n    pass\nexcept Exception as e:\n    pass\n",
		"print call":   "print('hello')\n",
		"octal prefix": "mode = 0o777\n",
		"not equal":    "if a != b:\n    pass\n",
		"lambda":       "f = lambda a, b: a + b\n",
		"digit suffix": "size = 10\n",
		"empty source": "",
	}
	for name, src := range py3 {
		t.Run("py3/"+name, func(t *testing.T) {
			require.Nil(t, pythonparser.DetectUnsupportedPython2(src), "must not classify Python 3 as Python 2")
		})
	}
}

// A file the Python 3 grammar rejects must come back as a classified skip when
// it carries Python 2 syntax, and as a plain error otherwise.
func TestPython2SyntaxIsClassifiedUnsupported(t *testing.T) {
	_, err := python2ssa.Frontend("value = ur'hello'\n")
	require.Error(t, err)
	var unsupported *pythonparser.UnsupportedPython2Error
	require.True(t, errors.As(err, &unsupported), "expected UnsupportedPython2Error, got %T: %v", err, err)
	require.NotEmpty(t, unsupported.UnsupportedSyntaxReason())

	_, err = python2ssa.Frontend("def broken(:\n    pass\n")
	require.Error(t, err)
	require.False(t, errors.As(err, &unsupported), "a plain Python 3 syntax error must not be classified as Python 2")

	_, err = python2ssa.Frontend("try:\n    pass\nexcept Exception as e:\n    pass\n")
	require.NoError(t, err)
}
