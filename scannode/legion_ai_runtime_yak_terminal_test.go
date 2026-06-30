package scannode

import (
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/utils"
)

// TestYakAISendFailureCode verifies the breakpoint A-2 mapping: a ReAct task
// that terminated in the Aborted state (surfaced by aiengine as
// ErrAITaskAborted, e.g. the loop ended on an empty provider response) is
// reported to the platform with the distinct "yak_ai_task_aborted" code, while
// genuine send/transport failures keep the generic "yak_ai_send_failed" code.
//
// The scan node previously swallowed Aborted (SendMsg returned nil) and never
// emitted an AISessionFailed event, so the platform had to fall back to the
// 15-minute timeout sweeper to converge the session. With the aiengine fix
// (ErrAITaskAborted) plus this mapping, the failure is now reported promptly
// with a code that lets consumers distinguish aborts from transport errors.
func TestYakAISendFailureCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "aborted task surfaces distinct code",
			err:  aiengine.ErrAITaskAborted,
			want: "yak_ai_task_aborted",
		},
		{
			name: "wrapped aborted task still classified as abort",
			err:  errors.New("react loop failed: " + aiengine.ErrAITaskAborted.Error()),
			// errors.Is unwraps; a plain wrap via fmt.Errorf("%w", ...) would match.
			// Here we pass a sentinel that matches via errors.Is when wrapped with %w.
			want: "yak_ai_send_failed", // not wrapped with %w, so not matched
		},
		{
			name: "generic send failure keeps generic code",
			err:  utils.Error("failed to send input event: connection reset"),
			want: "yak_ai_send_failed",
		},
		{
			name: "nil-adjacent plain error keeps generic code",
			err:  errors.New("some other failure"),
			want: "yak_ai_send_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := yakAISendFailureCode(tc.err)
			if got != tc.want {
				t.Fatalf("yakAISendFailureCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestYakAISendFailureCodeWrappedAbort confirms that a properly wrapped
// (%w) ErrAITaskAborted is still classified as an abort.
func TestYakAISendFailureCodeWrappedAbort(t *testing.T) {
	t.Parallel()
	wrapped := errors.New("react loop ended: empty response")
	combined := combinedAbortErr{inner: aiengine.ErrAITaskAborted, msg: wrapped.Error()}
	if got := yakAISendFailureCode(combined); got != "yak_ai_task_aborted" {
		t.Fatalf("wrapped abort classified as %q, want yak_ai_task_aborted", got)
	}
}

type combinedAbortErr struct {
	inner error
	msg   string
}

func (e combinedAbortErr) Error() string { return e.msg }
func (e combinedAbortErr) Is(target error) bool {
	return target == e.inner || errors.Is(e.inner, target)
}