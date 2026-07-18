package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityTodoDoneRequiresEvidence(t *testing.T) {
	store := NewVerificationTodoStore()
	add := store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "add", ID: "probe_idor", Content: "验证用户接口是否存在越权"}})
	require.True(t, add[0].Success)
	require.True(t, store.Items[0].EvidenceRequired)

	withoutEvidence := store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "done", ID: "probe_idor"}})
	require.False(t, withoutEvidence[0].Success)
	require.Equal(t, VerificationTodoStatusPending, store.Items[0].Status)

	withEvidence := store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "done", ID: "probe_idor", Evidence: "GET /api/user/2 returned 403"}})
	require.True(t, withEvidence[0].Success)
	require.Equal(t, VerificationTodoStatusDone, store.Items[0].Status)
	require.Equal(t, "GET /api/user/2 returned 403", store.Items[0].Evidence)
}

func TestGenericTodoKeepsLegacyDoneSemantics(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "add", ID: "write_summary", Content: "整理总结"}})
	result := store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "done", ID: "write_summary"}})
	require.True(t, result[0].Success)
}
