package loopinfra

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestDispatchContextMode_ParsingAndValidation(t *testing.T) {
	const name = "dispatch-context-mode-parser"
	require.NoError(t, reactloops.RegisterLoopFactory(name, func(inv aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
		return reactloops.NewMinimalReActLoop(inv.GetConfig(), inv), nil
	}))
	for _, encoded := range []bool{false, true} {
		t.Run(fmt.Sprintf("encoded=%v/mixed_batch", encoded), func(t *testing.T) {
			jobs := fmt.Sprintf(`[{"identifier":"a","goal":"first","loop_name":%q,"context_mode":"fork"},{"identifier":"b","goal":"second","loop_name":%q,"context_mode":"task_only"}]`, name, name)
			if encoded {
				raw, err := json.Marshal(jobs)
				require.NoError(t, err)
				jobs = string(raw)
			}
			action, err := aicommon.ExtractAction(`{"@action":"dispatch_sub_react_agents","dispatches":`+jobs+`}`, "dispatch_sub_react_agents")
			require.NoError(t, err)
			parsed, err := reactloops.ParseDispatchJobs(action)
			require.NoError(t, err)
			require.Len(t, parsed, 2)
			require.Equal(t, "fork", parsed[0].ContextMode)
			require.Equal(t, "first", parsed[0].Goal)
			require.Equal(t, "task_only", parsed[1].ContextMode)
			require.Equal(t, "second", parsed[1].Goal)
		})
		for _, tc := range []struct {
			mode, want string
			valid      bool
		}{
			{"", "fork", true}, {`,"context_mode":"fork"`, "fork", true},
			{`,"context_mode":"task_only"`, "task_only", true},
			{`,"context_mode":"clean"`, "", false}, {`,"context_mode":"all"`, "", false},
			{`,"context_mode":null`, "", false}, {`,"context_mode":false`, "", false},
			{`,"context_mode":[]`, "", false},
			{`,"context_mode":""`, "", false},
		} {
			t.Run(fmt.Sprintf("encoded=%v/%s", encoded, tc.mode), func(t *testing.T) {
				jobs := fmt.Sprintf(`[{"goal":"check file","loop_name":%q%s}]`, name, tc.mode)
				if encoded {
					raw, err := json.Marshal(jobs)
					require.NoError(t, err)
					jobs = string(raw)
				}
				action, err := aicommon.ExtractAction(`{"@action":"dispatch_sub_react_agents","dispatches":`+jobs+`}`, "dispatch_sub_react_agents")
				require.NoError(t, err)
				parsed, err := reactloops.ParseDispatchJobs(action)
				if !tc.valid {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Len(t, parsed, 1)
				require.Equal(t, tc.want, parsed[0].ContextMode)
			})
		}
	}
	for _, raw := range []string{"null", "{}", "[]", "[null]", "[42]"} {
		t.Run("invalid_dispatches/"+raw, func(t *testing.T) {
			action, err := aicommon.ExtractAction(`{"@action":"dispatch_sub_react_agents","dispatches":`+raw+`}`, "dispatch_sub_react_agents")
			require.NoError(t, err)
			_, err = reactloops.ParseDispatchJobs(action)
			require.Error(t, err)
		})
	}
}
