package tests

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	prog, err := ssaapi.Parse(DynamicSecurityMetadataSource, ssaapi.WithLanguage(ssaapi.JAVA))
	assert.NoErrorf(t, err, "parse error: %v", err)
	prog.Show()
}
