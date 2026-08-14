package aicommon

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type eofGatedReader struct {
	reader  *strings.Reader
	waiting chan struct{}
	release chan struct{}
	once    sync.Once
}

type synchronizedTestCapture struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (c *synchronizedTestCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *synchronizedTestCapture) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buf.String()
}

func newEOFGatedReader(payload string) *eofGatedReader {
	return &eofGatedReader{
		reader:  strings.NewReader(payload),
		waiting: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *eofGatedReader) Read(p []byte) (int, error) {
	if r.reader.Len() > 0 {
		return r.reader.Read(p)
	}
	r.once.Do(func() { close(r.waiting) })
	<-r.release
	return 0, io.EOF
}

func TestActionWaitParseResultRejectsTruncatedCanonicalObject(t *testing.T) {
	raw := `{
		"@action":"batch",
		"calls":[
			{"tool_name":"first","params":{"path":"/a"}},
			{"tool_name":"second","params":`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action, err := ExtractActionFromStream(ctx, strings.NewReader(raw), "batch")
	require.NoError(t, err, "the streaming constructor must remain asynchronous")
	require.ErrorContains(t, action.WaitParseResult(ctx), "complete canonical object")
	// A flattened callback can observe @action before the complete root object.
	// Its presence must not turn the partial response into an executable action.
	require.True(t, action.ValidCheck("batch"))
	_, exists := action.LookupCanonicalParam("calls")
	require.False(t, exists)
}

func TestActionParseErrorPropagatesThroughSynchronousExtractors(t *testing.T) {
	raw := `{"@action":"batch","calls":[{"tool_name":"first"},`

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action, err := ExtractValidActionFromStream(ctx, strings.NewReader(raw), "batch")
	require.Nil(t, action)
	require.ErrorContains(t, err, "complete canonical object")

	action, err = ExtractAction(raw, "batch")
	require.Nil(t, action)
	require.ErrorContains(t, err, "complete canonical object")
}

func TestActionWaitParseResultAcceptsCompleteCanonicalObject(t *testing.T) {
	action, err := ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"batch","calls":[]}`),
		"batch",
	)
	require.NoError(t, err)
	require.NoError(t, action.WaitParseResult(context.Background()))

	items, exists, err := action.GetCanonicalObjectArray("calls")
	require.NoError(t, err)
	require.True(t, exists)
	require.Empty(t, items)
}

func TestActionWaitParseResultJoinsStreamMirrorBeforeUnsynchronizedTeeBufferRead(t *testing.T) {
	const payload = `{"@action":"batch","calls":[]}`
	source := newEOFGatedReader(payload)
	var mirrored bytes.Buffer

	// Registering an AI tag makes ActionMaker use CreateUTF8StreamMirror. Its
	// copy goroutine drives this TeeReader and therefore owns mirrored until it
	// observes source EOF.
	action, err := ExtractActionFromStream(
		context.Background(),
		io.TeeReader(source, &mirrored),
		"batch",
		WithActionTagToKeyAndNonce("TEST_TAG", "test_tag", "nonce"),
	)
	require.NoError(t, err)

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- action.WaitParseResult(context.Background())
	}()

	// The complete JSON object has been copied, but the upstream reader has not
	// closed. WaitParseResult must not transfer ownership of mirrored yet.
	<-source.waiting
	select {
	case err := <-waitDone:
		require.Failf(t, "WaitParseResult returned before mirror EOF", "error: %v", err)
	default:
	}

	close(source.release)
	require.NoError(t, <-waitDone)
	// Unsynchronized callers may read the tee target only after this completion
	// barrier. ReAct instead uses a synchronized capture so scalar execution need
	// not wait for EOF; both ownership contracts are covered independently.
	require.Equal(t, payload, mirrored.String())
}

func TestActionFieldGetterStillReturnsBeforeParseCompletion(t *testing.T) {
	reader, writer := io.Pipe()
	action, err := ExtractActionFromStream(context.Background(), reader, "directly_call_tool")
	require.NoError(t, err)

	writeErr := make(chan error, 1)
	go func() {
		// Leave a later field unfinished. The comma after tool_name commits that
		// field, while the root object and the parser itself remain incomplete.
		_, err := io.WriteString(writer, `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","pending": `)
		writeErr <- err
	}()

	nameDone := make(chan string, 1)
	go func() {
		nameDone <- action.GetString("directly_call_tool_name")
	}()

	select {
	case name := <-nameDone:
		require.Equal(t, "read_file", name)
	case <-time.After(time.Second):
		t.Fatal("field getter waited for the incomplete root object")
	}
	require.NoError(t, <-writeErr)

	parseCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, action.WaitParseResult(parseCtx), context.DeadlineExceeded,
		"the field must be observable while the action parser is still running")

	require.NoError(t, writer.Close())
	require.ErrorContains(t, action.WaitParseResult(context.Background()), "complete canonical object")
}

func TestActionScalarDirectFieldsRemainReadableBeforeStreamEOF(t *testing.T) {
	reader, writer := io.Pipe()
	capture := new(synchronizedTestCapture)

	// The parser is deliberately held open after both scalar direct-call fields
	// have been committed.
	action, err := ExtractActionFromStream(
		context.Background(),
		io.TeeReader(reader, capture),
		"directly_call_tool",
	)
	require.NoError(t, err)

	writeErr := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"path":"/tmp/a"},"pending": `)
		writeErr <- err
	}()

	fieldsDone := make(chan struct{})
	go func() {
		defer close(fieldsDone)
		require.Equal(t, "read_file", action.GetString("directly_call_tool_name"))
		require.Equal(t, "/tmp/a", action.GetInvokeParams("directly_call_tool_params").GetString("path"))
	}()
	select {
	case <-fieldsDone:
	case <-time.After(time.Second):
		t.Fatal("scalar direct-call fields waited for stream EOF")
	}
	require.NoError(t, <-writeErr)

	// Snapshot while the tee writer is still live. This is the operation that
	// used to race in ReAct exec.go.
	snapshot := capture.String()
	require.Contains(t, snapshot, `"directly_call_tool_name":"read_file"`)

	parseCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, action.WaitParseResult(parseCtx), context.DeadlineExceeded)
	require.NoError(t, writer.Close())
	require.Error(t, action.WaitParseResult(context.Background()))
}

// DirectlyCallTool's long-standing latency contract is card-first: once the
// scalar tool name is known, the loading card is emitted before the later
// reason/params fields have to finish streaming. This is intentionally a
// ToolCaller-level contract; the outer AI transaction may still wait for its
// provider callback before dispatching the Action handler.
func TestToolCallerDirectlyCallTool_EmitsStartBeforeLaterActionFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events := make(chan *schema.AiOutputEvent, 16)
	cfg := NewTestConfig(ctx, WithEventHandler(func(event *schema.AiOutputEvent) {
		events <- event
	}))
	tool, err := aitool.New(
		"streaming_direct_test",
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return params.GetString("path"), nil
		}),
	)
	require.NoError(t, err)

	caller, err := NewToolCaller(
		ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_CallToolID("streaming-direct-call"),
		WithToolCaller_Reason("streaming direct-call test"),
	)
	require.NoError(t, err)

	reader, writer := io.Pipe()
	action, err := ExtractActionFromStream(ctx, reader, "directly_call_tool")
	require.NoError(t, err)
	writePrefixDone := make(chan error, 1)
	go func() {
		_, writeErr := io.WriteString(writer, `{"@action":"directly_call_tool","directly_call_tool_name":"streaming_direct_test","pending":`)
		writePrefixDone <- writeErr
	}()

	require.Equal(t, "streaming_direct_test", action.GetString("directly_call_tool_name"))
	require.NoError(t, <-writePrefixDone)

	prepareEntered := make(chan struct{})
	releasePrepare := make(chan struct{})
	prepareErr := errors.New("stop after card-ordering assertion")
	callDone := make(chan error, 1)
	go func() {
		_, _, callErr := caller.DirectlyCallTool(tool, action, func(_ *Action, _ string) (aitool.InvokeParams, bool, *aitool.Tool, error) {
			close(prepareEntered)
			<-releasePrepare
			// Stop at the direct-call preparation boundary. The purpose of this
			// test is to lock the legacy card-first ordering; executable scalar
			// examples cover the subsequent real tool callback separately.
			return nil, false, tool, prepareErr
		})
		callDone <- callErr
	}()

	select {
	case <-prepareEntered:
	case <-ctx.Done():
		t.Fatal("prepare callback was not reached")
	}

	var startEvent *schema.AiOutputEvent
	for startEvent == nil {
		select {
		case event := <-events:
			if event != nil && event.Type == schema.EVENT_TOOL_CALL_START {
				startEvent = event
			}
		case <-ctx.Done():
			t.Fatal("tool loading card was not emitted before later fields")
		}
	}
	require.Equal(t, "streaming-direct-call", startEvent.NodeId)
	select {
	case err := <-callDone:
		t.Fatalf("tool call completed before prepare was released: %v", err)
	default:
	}

	require.NoError(t, writer.Close())
	close(releasePrepare)
	require.ErrorIs(t, <-callDone, prepareErr)
	require.Error(t, action.WaitParseResult(context.Background()))
}

func TestActionWaitParseResultReportsCallerCancellationBeforeParserCompletion(t *testing.T) {
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	action, err := ExtractActionFromStream(ctx, reader, "batch")
	require.NoError(t, err)
	cancel()
	require.ErrorIs(t, action.WaitParseResult(ctx), context.Canceled)

	// Release the parser goroutine and make sure the eventual parser completion
	// remains observable independently from the cancelled construction context.
	require.NoError(t, writer.Close())
	parseCtx, parseCancel := context.WithTimeout(context.Background(), time.Second)
	defer parseCancel()
	require.NoError(t, action.WaitParseResult(parseCtx))
}

func TestDecodeStrictObjectArray(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantErr string
		wantLen int
	}{
		{name: "null", raw: nil, wantErr: "got null"},
		{name: "typed null", raw: []any(nil), wantErr: "got null"},
		{name: "scalar string", raw: "tool", wantErr: "got string"},
		{name: "scalar number", raw: 1, wantErr: "got int"},
		{name: "object is not array", raw: map[string]any{"tool_name": "one"}, wantErr: "got map"},
		{name: "null item", raw: []any{nil}, wantErr: "item 0 must be an object, got null"},
		{name: "string item", raw: []any{map[string]any{"tool_name": "one"}, "two"}, wantErr: "item 1 must be an object, got string"},
		{name: "array item", raw: []any{[]any{}}, wantErr: "item 0 must be an object, got slice"},
		{name: "non-string object keys", raw: []any{map[int]any{1: "one"}}, wantErr: "string keys"},
		{name: "empty", raw: []any{}, wantLen: 0},
		{name: "invoke params", raw: []aitool.InvokeParams{{"tool_name": "one"}}, wantLen: 1},
		{name: "objects", raw: []any{map[string]any{"tool_name": "one"}, map[string]any{"tool_name": "two"}}, wantLen: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, err := DecodeStrictObjectArray(test.raw)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, items)
				return
			}
			require.NoError(t, err)
			require.Len(t, items, test.wantLen)
		})
	}
}

func TestCanonicalObjectArrayDoesNotUseFlattenedNestedFields(t *testing.T) {
	raw := `{
		"@action":"batch",
		"calls":[
			{"tool_name":"first","params":{"path":"/a"}},
			{"tool_name":"second","params":{"path":"/b"}}
		]
	}`
	action, err := ExtractAction(raw, "batch")
	require.NoError(t, err)

	// Legacy flattening exposes the first nested field under its simple key.
	require.Equal(t, "first", action.GetString("tool_name"))
	// A canonical lookup only sees fields on the action's root object.
	_, exists := action.LookupCanonicalParam("tool_name")
	require.False(t, exists)

	items, exists, err := action.GetCanonicalObjectArray("calls")
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, items, 2)
	require.Equal(t, "first", items[0].GetString("tool_name"))
	require.Equal(t, "second", items[1].GetString("tool_name"))

	items[0]["tool_name"] = "mutated"
	canonicalRaw, exists := action.LookupCanonicalParam("calls")
	require.True(t, exists)
	canonical, err := DecodeStrictObjectArray(canonicalRaw)
	require.NoError(t, err)
	require.Equal(t, "first", canonical[0].GetString("tool_name"), "strict decoding must return independent item maps")
}

func TestGetCanonicalObjectArrayDistinguishesOmittedAndInvalid(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		exists  bool
		wantErr string
	}{
		{name: "omitted", payload: `{"@action":"batch"}`, exists: false},
		{name: "null", payload: `{"@action":"batch","calls":null}`, exists: true, wantErr: "got null"},
		{name: "scalar", payload: `{"@action":"batch","calls":"one"}`, exists: true, wantErr: "got string"},
		{name: "object", payload: `{"@action":"batch","calls":{"tool_name":"one"}}`, exists: true, wantErr: "got map"},
		{name: "non-object item", payload: `{"@action":"batch","calls":[{"tool_name":"one"},2]}`, exists: true, wantErr: "item 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, err := ExtractAction(test.payload, "batch")
			require.NoError(t, err)
			items, exists, err := action.GetCanonicalObjectArray("calls")
			require.Equal(t, test.exists, exists)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, items)
				return
			}
			require.NoError(t, err)
			require.Nil(t, items)
		})
	}
}
