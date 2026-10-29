package sfdb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func cycleGateLib(t *testing.T, content, ruleName string) *schema.SyntaxFlowRule {
	t.Helper()
	rule, err := CheckSyntaxFlowRuleContent(content)
	require.NoError(t, err)
	rule.RuleName = ruleName
	return rule
}

// TestCheckIncludeCycleForWriteDB exercises the write-time include-cycle gate
// against an isolated database: a candidate closing a cycle is rejected, an
// acyclic one passes, the candidate overlays the stored same-named rule
// (update semantics), and plain rules join the graph by title the way
// GetLibrary resolves them at runtime.
func TestCheckIncludeCycleForWriteDB(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}).Error)

	a := cycleGateLib(t, "desc(lib: \"a\");\n$input<include(\"b\")> as $o; alert $o;", "cycle-lib-a")
	require.NoError(t, db.Create(a).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Where("rule_name = ?", "cycle-lib-a").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
	})

	t.Run("rejects closing a cycle", func(t *testing.T) {
		b := cycleGateLib(t, "desc(lib: \"b\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-b")
		err := CheckIncludeCycleForWriteDB(db, b)
		require.Error(t, err)
		require.Contains(t, err.Error(), "a -> b -> a")
	})

	t.Run("allows acyclic candidate", func(t *testing.T) {
		c := cycleGateLib(t, "desc(lib: \"c\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-c")
		require.NoError(t, CheckIncludeCycleForWriteDB(db, c))
	})

	t.Run("candidate overlays the stored same-named rule", func(t *testing.T) {
		b := cycleGateLib(t, "desc(lib: \"b\");\n$input<include(\"a\")> as $o; alert $o;", "cycle-lib-b")
		require.NoError(t, db.Create(b).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Where("rule_name = ?", "cycle-lib-b").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
		})
		// The stored graph is cyclic (a -> b -> a), but the candidate replaces
		// a's node with an include-free version, so the write must pass.
		a2 := cycleGateLib(t, "desc(lib: \"a\");\na* as $o; alert $o;", "cycle-lib-a")
		require.NoError(t, CheckIncludeCycleForWriteDB(db, a2))
	})

	t.Run("non-library title joins the graph", func(t *testing.T) {
		// GetLibrary resolves by title too, so a plain rule titled "x" is a
		// cycle node even without allow-included.
		x := cycleGateLib(t, "desc(title: \"x\");\n$input<include(\"y\")> as $o; alert $o;", "cycle-rule-x")
		require.NoError(t, db.Create(x).Error)
		t.Cleanup(func() {
			require.NoError(t, db.Where("rule_name = ?", "cycle-rule-x").Unscoped().Delete(&schema.SyntaxFlowRule{}).Error)
		})

		y := cycleGateLib(t, "desc(lib: \"y\");\n$input<include(\"x\")> as $o; alert $o;", "cycle-lib-y")
		err := CheckIncludeCycleForWriteDB(db, y)
		require.Error(t, err)
		require.Contains(t, err.Error(), "x -> y -> x")
	})

	require.NoError(t, CheckIncludeCycleForWriteDB(db, nil))
}
