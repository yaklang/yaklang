package platformassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

const secret = "01234567890123456789012345678901"

func turn(id, user string) *aiv1.AssistantTurnRequest {
	return &aiv1.AssistantTurnRequest{TurnId: id, ConversationId: "conversation-" + user, OwnerUserId: user, Input: "List my projects", DelegationToken: "private-delegation-" + user}
}
func request(s *Server, q *aiv1.AssistantTurnRequest, auth string) *httptest.ResponseRecorder {
	data, _ := protojson.Marshal(q)
	r := httptest.NewRequest("POST", "/v1/turns", bytes.NewReader(data))
	r.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestPlatformHTTPBoundaries(t *testing.T) {
	s, err := New(Config{LegionURL: "http://127.0.0.1:1", ServiceSecret: secret, MaxPerUser: 1})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(s, turn("one", "alice"), "Bearer wrong"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	q := turn("one", "alice")
	q.Tools = []*aiv1.AssistantTool{{Name: "bash", ParametersJson: `{"type":"object"}`}}
	if w := request(s, q, "Bearer "+secret); w.Code != 400 {
		t.Fatal(w.Code)
	}
	s.active["active"] = "alice"
	s.users["alice"] = 1
	if w := request(s, turn("active", "bob"), "Bearer "+secret); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(s, turn("other", "alice"), "Bearer "+secret); w.Code != 429 {
		t.Fatal(w.Code)
	}
}
func TestPlatformConcurrentIdentityAndCancel(t *testing.T) {
	entered := make(chan string, 4)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		entered <- r.Header.Get("Authorization")
		<-r.Context().Done()
	}))
	defer callback.Close()
	s, err := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var cancels []context.CancelFunc
	for _, user := range []string{"alice", "bob"} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cancels = append(cancels, cancel)
		q := turn(user, user)
		body, _ := protojson.Marshal(q)
		r := httptest.NewRequest("POST", "/v1/turns", bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+secret)
		wg.Add(1)
		go func() { defer wg.Done(); s.ServeHTTP(httptest.NewRecorder(), r) }()
	}
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case id := <-entered:
			got[id] = true
		case <-time.After(20 * time.Second):
			t.Fatal("model callbacks did not start")
		}
	}
	if !got["Bearer private-delegation-alice"] || !got["Bearer private-delegation-bob"] {
		t.Fatal(got)
	}
	// Disconnect both admitted requests after observing their actual callbacks.
	for _, cancel := range cancels {
		cancel()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.active) != 0 || len(s.users) != 0 {
		t.Fatal("admission leaked")
	}
}
func TestPlatformRealEngineToolThenAnswer(t *testing.T) {
	previous := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() { yakit.SetCachedAIGlobalConfigForTest(previous) })
	yakit.SetCachedAIGlobalConfigForTest(&ypb.AIGlobalConfig{AIPresetPrompt: "PRIVATE_GLOBAL_CONTEXT_SENTINEL"})
	var mu sync.Mutex
	decisions, toolCalls := 0, 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-delegation-alice" {
			t.Error("delegation lost")
		}
		if r.URL.Path == "/v1/ai/assistant/runtime/tools" {
			body, _ := io.ReadAll(r.Body)
			q := new(aiv1.AssistantToolRequest)
			if protojson.Unmarshal(body, q) != nil || q.Name != "platform.list_projects" || q.ArgumentsJson != "{}" {
				t.Error("unexpected tool callback")
			}
			mu.Lock()
			toolCalls++
			mu.Unlock()
			data, _ := protojson.Marshal(&aiv1.AssistantToolResponse{ResultJson: `{"id":"operation-1","status":"completed","result_json":{"projects":[{"name":"Alice project"}]}}`})
			_, _ = w.Write(data)
			return
		}
		body, _ := io.ReadAll(r.Body)
		q := new(aiv1.AssistantModelRequest)
		_ = protojson.Unmarshal(body, q)
		if strings.Contains(q.Prompt, "private-delegation") || strings.Contains(q.Prompt, "PRIVATE_GLOBAL_CONTEXT_SENTINEL") {
			t.Error("secret in prompt")
		}
		mu.Lock()
		if aicommon.IsPrimaryDecisionPrompt(q.Prompt) {
			decisions++
		}
		n := decisions
		mu.Unlock()
		if n >= 2 && aicommon.IsPrimaryDecisionPrompt(q.Prompt) && !strings.Contains(q.Prompt, "operation-1") {
			t.Error("model did not receive operation result wrapper")
		}
		text := `{"@action":"object","next_action":{"type":"finish"}}`
		switch n {
		case 1:
			text = `{"@action":"object","human_readable_thought":"先查询当前用户的项目。","next_action":{"type":"directly_call_tool","directly_call_tool_name":"platform.list_projects","directly_call_tool_params":{}}}`
		case 2:
			text = `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"Alice project"}}`
		}
		if !aicommon.IsPrimaryDecisionPrompt(q.Prompt) && aicommon.IsDirectAnswerPrompt(q.Prompt) {
			text = `{"@action":"directly_answer","answer_payload":"Alice project"}`
		}
		data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: text})
		_, _ = w.Write(data)
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 20 * time.Second})
	q := turn("one", "alice")
	q.Tools = []*aiv1.AssistantTool{{Name: "platform.list_projects", Description: "List projects", ParametersJson: `{"type":"object","properties":{}}`}}
	w := request(s, q, "Bearer "+secret)
	mu.Lock()
	defer mu.Unlock()
	if toolCalls != 1 || decisions != 2 {
		t.Fatalf("tool calls %d decisions %d body %s", toolCalls, decisions, w.Body.String())
	}
	kinds := map[string]bool{}
	var thoughts []string
	for _, line := range strings.Split(strings.TrimSpace(w.Body.String()), "\n") {
		e := new(aiv1.AssistantRuntimeEvent)
		if err := protojson.Unmarshal([]byte(line), e); err != nil {
			t.Fatal(err)
		}
		if e.Kind == "thought_delta" {
			if kinds["completed"] || kinds["failed"] {
				t.Fatal("thought arrived after terminal event")
			}
			thoughts = append(thoughts, e.Text)
		}
		kinds[e.Kind] = true
	}
	if strings.Join(thoughts, "") != "先查询当前用户的项目。" {
		t.Fatalf("public summary missing or polluted: %q", thoughts)
	}
	if !kinds["tool_started"] || !kinds["tool_completed"] || !kinds["completed"] || !kinds["answer"] || !strings.Contains(w.Body.String(), "Alice project") {
		b, _ := json.Marshal(kinds)
		t.Fatal(string(b), w.Body.String())
	}
}
func TestPlatformCallbackRefusesRedirect(t *testing.T) {
	reached := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 307) }))
	defer origin.Close()
	s, _ := New(Config{LegionURL: origin.URL, ServiceSecret: secret})
	err := s.callback(context.Background(), "delegation", "/model", &aiv1.AssistantModelRequest{}, &aiv1.AssistantModelResponse{})
	if err == nil || reached {
		t.Fatal("redirect leaked delegation")
	}
}

func TestPlatformRealEngineRejectsInjectedCapabilities(t *testing.T) {
	for _, attack := range []string{
		`{"type":"directly_call_tool","directly_call_tool_name":"bash","directly_call_tool_params":{"command":"touch /tmp/platform-assistant-forbidden"}}`,
		`{"type":"request_plan_and_execution","plan_payload":"execute shell"}`,
		`{"type":"loading_skills","skills":"/tmp/evil"}`,
	} {
		t.Run(attack, func(t *testing.T) {
			n := 0
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/ai/assistant/runtime/model" {
					t.Error("unapproved tool reached callback")
					http.Error(w, "denied", 403)
					return
				}
				n++
				text := `{"@action":"object","next_action":{"type":"finish"}}`
				if n == 1 {
					text = `{"@action":"object","next_action":` + attack + `}`
				}
				data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: text})
				_, _ = w.Write(data)
			}))
			defer callback.Close()
			s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
			w := request(s, turn("attack", "alice"), "Bearer "+secret)
			if n == 0 {
				t.Fatal("engine never called model")
			}
			if strings.Contains(w.Body.String(), "tool_started") {
				t.Fatal("unapproved tool execution", w.Body.String())
			}
		})
	}
}

func TestPlatformModelCallbackHonorsRequestAndTurnCancellation(t *testing.T) {
	for _, source := range []string{"request", "turn"} {
		t.Run(source, func(t *testing.T) {
			entered := make(chan struct{})
			disconnected := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(entered)
				<-r.Context().Done()
				close(disconnected)
			}))
			defer upstream.Close()
			s, _ := New(Config{LegionURL: upstream.URL, ServiceSecret: secret})
			turnCtx, cancelTurn := context.WithCancel(context.Background())
			defer cancelTurn()
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			done := make(chan error, 1)
			go func() {
				_, err := s.modelCallback(turnCtx, "delegation")(nil, aicommon.NewAIRequest("prompt", aicommon.WithAIRequest_Context(requestCtx)))
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("callback did not reach upstream")
			}
			if source == "request" {
				cancelRequest()
			} else {
				cancelTurn()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("canceled callback succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("callback ignored cancellation")
			}
			select {
			case <-disconnected:
			case <-time.After(time.Second):
				t.Fatal("upstream request was not canceled")
			}
		})
	}
}

// The bounded platform profile must stop after the verified final answer. A
// second model call used to repeat the answer or fail an otherwise complete turn.
func TestResiliencePlatformFinalAnswerNeedsNoFollowupModelCall(t *testing.T) {
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			http.Error(w, "unexpected follow-up", 500)
			return
		}
		data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"Current page is Projects."}}`})
		w.Write(data)
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
	w := request(s, turn("final-answer", "alice"), "Bearer "+secret)
	if calls != 1 || strings.Contains(w.Body.String(), `"kind":"failed"`) || !strings.Contains(w.Body.String(), `"kind":"completed"`) {
		t.Fatalf("calls=%d body=%s", calls, w.Body.String())
	}
	if strings.Count(w.Body.String(), "Current page is Projects.") != 1 {
		t.Fatalf("answer duplicated: %s", w.Body.String())
	}
}

func TestPlatformCorrectsPlainTextWithoutReplayingTools(t *testing.T) {
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		text := "Current page is Projects."
		if calls > 1 {
			text = `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"Current page is Projects."}}`
		}
		data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: text})
		w.Write(data)
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
	w := request(s, turn("format-retry", "alice"), "Bearer "+secret)
	if calls != 2 || strings.Contains(w.Body.String(), `"kind":"failed"`) || !strings.Contains(w.Body.String(), `"kind":"completed"`) || strings.Count(w.Body.String(), "Current page is Projects.") != 1 {
		t.Fatalf("calls=%d body=%s", calls, w.Body.String())
	}
}
func TestPlatformDoesNotMultiplyFailedModelCallbacks(t *testing.T) {
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "model failed", 500) }))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
	w := request(s, turn("model-failure", "alice"), "Bearer "+secret)
	if calls != 1 || !strings.Contains(w.Body.String(), `"kind":"failed"`) {
		t.Fatalf("calls=%d body=%s", calls, w.Body.String())
	}
}

func TestPlatformMalformedResponseRetriesAreBounded(t *testing.T) {
	calls := 0
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: "plain answer"})
		w.Write(data)
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
	w := request(s, turn("invalid-format", "alice"), "Bearer "+secret)
	if calls != 3 || !strings.Contains(w.Body.String(), "runtime_invalid_model_response") || strings.Contains(w.Body.String(), `"kind":"completed"`) {
		t.Fatalf("calls=%d body=%s", calls, w.Body.String())
	}
}

func TestPlatformThoughtForwarderFiltersAndBounds(t *testing.T) {
	var events []*aiv1.AssistantRuntimeEvent
	forward := thoughtForwarder(func(event *aiv1.AssistantRuntimeEvent) { events = append(events, event) })
	finish := func(id string) {
		data, _ := json.Marshal(map[string]string{"event_writer_id": id})
		forward(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "stream-finished", Content: data})
	}
	for _, stream := range []struct{ node, source string }{
		{"re-act-loop-answer-payload", "human_readable_thought"},
		{"re-act-loop-thought", "reason_content"},
		{"re-act-loop-thought", "modify_code_reason"},
		{"re-act-loop-thought", ""},
	} {
		forward(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_STREAM, EventUUID: "private", VizSource: stream.source, NodeId: stream.node, StreamDelta: []byte("private or answer")})
		finish("private")
	}
	if len(events) != 0 {
		t.Fatal("non-public streams were forwarded")
	}
	public := func(id, text string) {
		forward(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_STREAM, EventUUID: id, VizSource: "human_readable_thought", NodeId: "re-act-loop-thought", StreamDelta: []byte(text)})
	}
	public("one", "查询")
	public("one", "项目")
	if len(events) != 2 {
		t.Fatal("summary deltas were not forwarded immediately")
	}
	finish("one")
	finish("one")
	public("two", strings.Repeat("中", maxThoughtBytes))
	finish("two")
	public("three", "must be ignored")
	finish("three")
	total := 0
	for _, event := range events {
		if event.Kind != "thought_delta" || !utf8.ValidString(event.Text) {
			t.Fatal("invalid thought event")
		}
		total += len(event.Text)
	}
	if len(events) != 3 || events[0].Text+events[1].Text != "查询项目" || total > maxThoughtBytes || total < maxThoughtBytes-3 {
		t.Fatalf("unexpected bounded output: events=%d bytes=%d", len(events), total)
	}
}

func TestPlatformFlatActionPublicSummary(t *testing.T) {
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := protojson.Marshal(&aiv1.AssistantModelResponse{Text: `{"@action":"directly_answer","identifier":"explain_page","human_readable_thought":"根据页面上下文回答。","answer_payload":"当前页面是项目列表。"}`})
		_, _ = w.Write(data)
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
	w := request(s, turn("flat-summary", "alice"), "Bearer "+secret)
	thoughts, answer, completed := "", "", false
	for _, line := range strings.Split(strings.TrimSpace(w.Body.String()), "\n") {
		event := new(aiv1.AssistantRuntimeEvent)
		if err := protojson.Unmarshal([]byte(line), event); err != nil {
			t.Fatal(err)
		}
		switch event.Kind {
		case "thought_delta":
			if completed {
				t.Fatalf("invalid summary: %v", event)
			}
			thoughts += event.Text
		case "answer":
			answer += event.Text
		case "completed":
			completed = true
		case "failed":
			t.Fatalf("flat action failed: %s", event.ErrorCode)
		}
	}
	if thoughts != "根据页面上下文回答。" || !completed || answer != "当前页面是项目列表。" {
		t.Fatalf("thoughts=%q completed=%t answer=%q", thoughts, completed, answer)
	}
}

func TestResiliencePlatformStreamingBeforeModelCompletion(t *testing.T) {
	gate := make(chan struct{})
	defer func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/x-ndjson" {
			t.Error("stream negotiation absent")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		emit := func(kind, text string) {
			data, _ := protojson.Marshal(&aiv1.AssistantRuntimeEvent{Kind: kind, Text: text})
			w.Write(append(data, '\n'))
			w.(http.Flusher).Flush()
		}
		emit("model_delta", `{"@action":"object","human_readable_thought":"正在检查页面。","next_action":{"type":"directly_answer","answer_payload":"当前页面`)
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		emit("model_delta", `是项目。"}}`)
		emit("completed", "")
	}))
	defer callback.Close()
	s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	events := make(chan *aiv1.AssistantRuntimeEvent, 100)
	done := make(chan error, 1)
	go func() {
		done <- s.run(ctx, turn("stream", "alice"), func(e *aiv1.AssistantRuntimeEvent) { events <- e })
	}()
	thought, answer := false, false
	for !thought || !answer {
		select {
		case e := <-events:
			thought = thought || e.Kind == "thought_delta"
			answer = answer || e.Kind == "answer"
		case err := <-done:
			t.Fatalf("finished before release: %v", err)
		case <-ctx.Done():
			t.Fatal("public output buffered until model completion")
		}
	}
	close(gate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("turn did not finish")
	}
}

func TestResiliencePlatformPartialModelFailureIsNotRetried(t *testing.T) {
	for _, terminal := range []string{"failed", "eof", "completed"} {
		t.Run(terminal, func(t *testing.T) {
			calls := 0
			callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/x-ndjson")
				data, _ := protojson.Marshal(&aiv1.AssistantRuntimeEvent{Kind: "model_delta", Text: `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"partial`})
				w.Write(append(data, '\n'))
				w.(http.Flusher).Flush()
				if terminal == "completed" {
					w.Write([]byte("{\"kind\":\"completed\"}\n"))
				}
				if terminal == "failed" {
					w.Write([]byte("{\"kind\":\"failed\",\"error_code\":\"assistant_model_upstream_error\"}\n"))
				}
			}))
			defer callback.Close()
			s, _ := New(Config{LegionURL: callback.URL, ServiceSecret: secret, TurnTimeout: 10 * time.Second})
			w := request(s, turn("partial", "alice"), "Bearer "+secret)
			if calls != 1 || !strings.Contains(w.Body.String(), `"kind":"failed"`) || strings.Contains(w.Body.String(), `"kind":"completed"`) {
				t.Fatalf("calls=%d body=%s", calls, w.Body.String())
			}
		})
	}
}

func TestResiliencePlatformStreamingCancellation(t *testing.T) {
	entered := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte("{\"kind\":\"model_delta\",\"text\":\"prefix\"}\n"))
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, _ := New(Config{LegionURL: upstream.URL, ServiceSecret: secret})
	callback, lastError := s.streamingModelCallback(ctx, "delegation")
	response, err := callback(aicommon.NewConfig(ctx), aicommon.NewAIRequest("prompt", aicommon.WithAIRequest_Context(ctx)))
	if err != nil {
		t.Fatal(err)
	}
	_, reader := response.GetUnboundStreamReaderEx(nil, nil, nil)
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(reader); done <- err }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream reader leaked after cancellation")
	}
	if lastError() == nil {
		t.Fatal("canceled partial response treated as success")
	}
}
