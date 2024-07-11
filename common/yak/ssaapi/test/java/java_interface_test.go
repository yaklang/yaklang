package java

import (
	_ "embed"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed sample/mybatis_mapper_interface.java
var javaIfEmbed string

func TestJavaInterfaceMapper(t *testing.T) {
	ssatest.Check(t, javaIfEmbed, func(prog *ssaapi.Program) error {
		prog.Show()
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.get* as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.getUserById as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.insert* as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.update* as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.delete* as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.*User as $sink`).Show().Len(), 2)
		assert.Greater(t, prog.SyntaxFlowChain(`UserMapper.*User* as $sink`).Show().Len(), 3)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
