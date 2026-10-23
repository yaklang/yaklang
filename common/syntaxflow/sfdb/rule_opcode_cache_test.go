package sfdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMigrateSyntaxFlowReplacesStaleNonemptyOpcodes(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}).Error)
	rule := &schema.SyntaxFlowRule{
		RuleName: "stale-opcode-cache", Content: `desc(title: "cache") * as $x`,
		OpCodes: `{"version":"outdated","schema_version":1,"opcode":[]}`,
	}
	require.NoError(t, MigrateSyntaxFlowWithDB(db, "", rule))
	originalHash, originalID := rule.Hash, rule.ID
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	_, resave, err := vm.Load(rule)
	require.NoError(t, err)
	require.True(t, resave)
	require.EqualValues(t, 1, vm.CompileCount())
	require.NoError(t, MigrateSyntaxFlowWithDB(db, "", rule))
	var stored schema.SyntaxFlowRule
	require.NoError(t, db.Where("rule_name = ?", rule.RuleName).First(&stored).Error)
	require.Equal(t, originalHash, stored.Hash, "derived opcodes do not change rule identity")
	require.Equal(t, originalID, stored.ID)
	require.Equal(t, rule.OpCodes, stored.OpCodes, "nonempty stale cache must actually be replaced")
	warmVM := sfvm.NewSyntaxFlowVirtualMachine()
	_, resave, err = warmVM.Load(&stored)
	require.NoError(t, err)
	require.False(t, resave, "a later process must use the persisted new opcodes")
	require.Zero(t, warmVM.CompileCount())
}
