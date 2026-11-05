package test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

// Cookiecutter and Django app templates use a .py suffix but contain Jinja
// tags. The parse failure must come back as a classified skip, not an engine
// error, and an inline tag inside a Python string must stay valid Python.
func TestTemplateFileIsClassifiedUnsupported(t *testing.T) {
	_, err := python2ssa.Frontend("{% include 'partials/common.py' %}\n")
	require.Error(t, err)
	var unsupported *pythonparser.UnsupportedTemplateError
	require.True(t, errors.As(err, &unsupported), "expected UnsupportedTemplateError, got %T: %v", err, err)
	require.Contains(t, unsupported.UnsupportedSyntaxReason(), "template syntax")

	_, err = python2ssa.Frontend("from ${app_name} import views\n")
	require.Error(t, err)
	require.True(t, errors.As(err, &unsupported), "expected scaffold placeholder detection")

	_, err = python2ssa.Frontend("value = '{{ name }}'\n")
	require.NoError(t, err, "an inline template tag inside a string is valid Python")
}
