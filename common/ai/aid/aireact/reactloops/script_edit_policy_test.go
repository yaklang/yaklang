package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func newScriptPolicyTestLoop(query string) *ReActLoop {
	loop := &ReActLoop{vars: omap.NewEmptyOrderedMap[string, any]()}
	loop.Set("user_query", query)
	return loop
}

func TestDetectEditThenExecuteIntent(t *testing.T) {
	require.True(t, DetectEditThenExecuteIntent("在这个脚本开头增加一段注释，增加了内容之后重新再执行"))
	require.True(t, DetectEditThenExecuteIntent("modify this script and run again"))
	require.False(t, DetectEditThenExecuteIntent("编写一个 python 脚本并执行"))
	require.False(t, DetectEditThenExecuteIntent("执行这个脚本"))
}

func TestApplyScriptEditExecutionPolicy(t *testing.T) {
	loop := newScriptPolicyTestLoop("在这个脚本开头增加一段注释，增加了内容之后重新再执行")
	adjusted := ApplyScriptEditExecutionPolicy(loop, []string{"bash", "read_file"})

	require.Equal(t, []string{"modify_file", "bash", "read_file"}, adjusted)
	require.Equal(t, "true", loop.Get(LoopStateRequireEditBeforeExecution))
	require.Equal(t, "", loop.Get(LoopStateEditBeforeExecutionCompleted))
}

func TestShouldBlockBashUntilEdit(t *testing.T) {
	loop := newScriptPolicyTestLoop("修改这个脚本并重新执行")
	ApplyScriptEditExecutionPolicy(loop, nil)

	require.True(t, ShouldBlockBashUntilEdit(loop, "bash"))
	MarkEditBeforeExecutionCompleted(loop, "modify_file")
	require.False(t, ShouldBlockBashUntilEdit(loop, "bash"))
}

func TestMarkEditBeforeExecutionCompletedIgnoresNonEditTools(t *testing.T) {
	loop := newScriptPolicyTestLoop("修改这个脚本并重新执行")
	ApplyScriptEditExecutionPolicy(loop, nil)

	MarkEditBeforeExecutionCompleted(loop, "read_file")
	require.Equal(t, "", loop.Get(LoopStateEditBeforeExecutionCompleted))

	MarkEditBeforeExecutionCompleted(loop, "write_file")
	require.Equal(t, "true", loop.Get(LoopStateEditBeforeExecutionCompleted))
}
