package platformassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
			text = `{"@action":"object","next_action":{"type":"directly_call_tool","directly_call_tool_name":"platform.list_projects","directly_call_tool_params":{}}}`
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
	if toolCalls != 1 {
		t.Fatalf("tool calls %d decisions %d body %s", toolCalls, decisions, w.Body.String())
	}
	kinds := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(w.Body.String()), "\n") {
		e := new(aiv1.AssistantRuntimeEvent)
		if err := protojson.Unmarshal([]byte(line), e); err != nil {
			t.Fatal(err)
		}
		kinds[e.Kind] = true
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
