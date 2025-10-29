package java

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestJavaInterfaceMapper_WithAnnotation(t *testing.T) {
	ssatest.Check(t, javaIfEmbed, func(prog *ssaapi.Program) error {
		prog.Show()
		assert.Greater(t, prog.SyntaxFlowChain(`Select.__ref__?{have: getUserById} as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`getUserById.annotation?{.Select} as $sink`).Show().Len(), 0)

		assert.Greater(t, prog.SyntaxFlowChain(`Insert.__ref__?{have: insertUser} as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`Options.__ref__?{have: insertUser} as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`insertUser.annotation?{.Insert && .Options} as $sink`).Show().Len(), 0)

		assert.Greater(t, prog.SyntaxFlowChain(`Update.__ref__?{have: updateUser} as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`updateUser.annotation?{.Update} as $sink`).Show().Len(), 0)

		assert.Greater(t, prog.SyntaxFlowChain(`Delete.__ref__?{have: deleteUser} as $sink`).Show().Len(), 0)
		assert.Greater(t, prog.SyntaxFlowChain(`deleteUser.annotation?{.Delete} as $sink`).Show().Len(), 0)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
