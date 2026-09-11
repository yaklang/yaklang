package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTemplates(t *testing.T) {
	tmpl := GetTemplates()
	assert.NotNil(t, tmpl)
	assert.Contains(t, tmpl.JspDefaultHeader, "trimDirectiveWhitespaces")
	assert.Contains(t, tmpl.JspDefaultMemberDecl, "defineClass")
	assert.Contains(t, tmpl.PhpDefaultServiceDecl, "eval")
	assert.Contains(t, tmpl.AspxDefaultServiceDecl, "Assembly")
	assert.Contains(t, tmpl.BehinderPhpEchoEncoder, "encrypt")
	assert.Contains(t, tmpl.BehinderAspEchoEncoder, "Encrypt")
	assert.Equal(t, "assert|eval(base64_decode('", tmpl.BehinderPhpAssertPrefix)
	assert.Equal(t, "'));", tmpl.BehinderPhpAssertSuffix)
}
