package loop_yaklangcode

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type runStatusMap struct {
	m map[string]string
}

func (r *runStatusMap) Set(k string, v any) {
	if r.m == nil {
		r.m = make(map[string]string)
	}
	r.m[k] = fmt.Sprint(v)
}

func (r *runStatusMap) Get(k string) string {
	if r.m == nil {
		return ""
	}
	return r.m[k]
}

func TestResetYakRunStatusAfterCodeChange(t *testing.T) {
	st := &runStatusMap{m: map[string]string{
		loopVarYakRunOK:           "true",
		loopVarYakRunOutput:       "ok log",
		loopVarYakRunLastFeedback: "old feedback",
	}}
	resetYakRunStatusAfterCodeChange(st)
	require.Equal(t, "", st.Get(loopVarYakRunOK))
	require.Equal(t, "", st.Get(loopVarYakRunOutput))
	require.Equal(t, "", st.Get(loopVarYakRunLastFeedback))
}

func TestResetYakRunStatusAfterCodeChange_NilSafe(t *testing.T) {
	require.NotPanics(t, func() {
		resetYakRunStatusAfterCodeChange(nil)
	})
}
