package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const SyncUIInteraction = "ui_interaction"

type UIClientCapabilities struct {
	Protocol  string          `json:"protocol"`
	Questions bool            `json:"questions"`
	Catalog   json.RawMessage `json:"catalog,omitempty"`
}

type uiQuestion struct {
	Request interaction.QuestionRequest
	origin  *Config
	ctx     context.Context
	done    chan struct{}
	timer   *time.Timer
	answer  string
}

type UIUserMessage struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Metadata interaction.Submission `json:"metadata"`
}

// UISession is shared by the parent and its workers, but never across sessions.
// Durable snapshots use the existing event journal and are replayable by old
// clients through the ordinary text fallback emitted beside each UI artifact.
type UISession struct {
	mu        sync.Mutex
	client    UIClientCapabilities
	questions map[string]*uiQuestion
	receipts  map[string]UIUserMessage
	restored  bool
}

func newUISession() *UISession {
	return &UISession{questions: map[string]*uiQuestion{}, receipts: map[string]UIUserMessage{}}
}

func (c *Config) UISession() *UISession {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	if s.uiSession == nil {
		s.uiSession = newUISession()
	}
	return s.uiSession
}

func WithClientUI(raw string) ConfigOption {
	return func(c *Config) error {
		if len(raw) > 64*1024 {
			return fmt.Errorf("client UI catalog exceeds 64 KiB")
		}
		var client UIClientCapabilities
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &client); err != nil {
				return fmt.Errorf("invalid client UI capabilities: %w", err)
			}
			if client.Protocol != interaction.Protocol {
				return fmt.Errorf("unsupported client UI protocol %q", client.Protocol)
			}
		}
		s := c.UISession()
		s.mu.Lock()
		s.client = client
		s.mu.Unlock()
		return nil
	}
}

func (c *Config) UIQuestionsEnabled() bool {
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client.Protocol == interaction.Protocol && s.client.Questions
}

func (c *Config) RestoreUIInteractions() error {
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restored {
		return nil
	}
	if c.PersistentSessionId != "" && c.GetDB() != nil {
		events, err := yakit.QueryAIEvent(c.GetDB(), &ypb.AIEventFilter{
			SessionID: c.PersistentSessionId, EventType: []string{"ui_question", "user_message"},
		})
		if err != nil {
			return err
		}
		sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
		for _, e := range events {
			if e.Type == "ui_question" {
				var q interaction.QuestionRequest
				if json.Unmarshal(e.Content, &q) == nil && q.ID != "" {
					s.questions[q.ID] = &uiQuestion{Request: q}
				}
			} else {
				var message UIUserMessage
				if json.Unmarshal(e.Content, &message) == nil && message.ID != "" {
					s.receipts[message.ID] = message
				}
			}
		}
	}
	s.restored = true
	for _, q := range s.questions {
		if q.Request.State == "pending" && q.Request.Deadline > 0 && !q.Request.Touched {
			id := q.Request.ID
			q.timer = time.AfterFunc(time.Until(time.UnixMilli(q.Request.Deadline)), func() {
				if c.GetContext().Err() == nil {
					_ = c.UpdateQuestion(id, "expire")
				}
			})
		}
	}
	return nil
}

func (c *Config) emitQuestion(q *uiQuestion) error {
	_, err := c.EmitJSON("ui_question", "ui_question", q.Request)
	return err
}

// RequestQuestions returns immediately in async mode. Blocking mode waits only
// for this request; cancellation never implies authorization for another action.
func (c *Config) RequestQuestions(ctx context.Context, taskID string, questions []interaction.Question, blocking bool, timeoutSeconds int) (any, error) {
	if !c.UIQuestionsEnabled() {
		return nil, fmt.Errorf("client does not support structured questions")
	}
	if err := c.RestoreUIInteractions(); err != nil {
		return nil, err
	}
	mode := "async"
	if blocking {
		mode = "blocking"
	}
	request := interaction.QuestionRequest{ID: uuid.NewString(), TaskID: taskID, Mode: mode, Questions: questions, State: "pending"}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if timeoutSeconds < 0 || timeoutSeconds > 86400 {
		return nil, fmt.Errorf("timeoutSeconds must be between 0 and 86400")
	}
	if !blocking && timeoutSeconds > 0 {
		request.Deadline = time.Now().Add(time.Duration(timeoutSeconds) * time.Second).UnixMilli()
	}
	done := make(chan struct{})
	q := &uiQuestion{Request: request, origin: c, ctx: ctx, done: done}
	s := c.UISession()
	s.mu.Lock()
	pending := 0
	for _, old := range s.questions {
		if old.Request.State == "pending" {
			pending++
		}
	}
	if pending >= 3 {
		s.mu.Unlock()
		return nil, fmt.Errorf("three questions are already pending; continue independent work instead of asking again")
	}
	s.questions[request.ID] = q
	if err := c.emitQuestion(q); err != nil {
		delete(s.questions, request.ID)
		s.mu.Unlock()
		return nil, err
	}
	if request.Deadline > 0 {
		q.timer = time.AfterFunc(time.Until(time.UnixMilli(request.Deadline)), func() {
			if c.GetContext().Err() == nil {
				_ = c.UpdateQuestion(request.ID, "expire")
			}
		})
	}
	s.mu.Unlock()
	if !blocking {
		return map[string]any{"status": "accepted", "requestId": request.ID,
			"instruction": "This call does not wait for an answer. Continue independent work now. An answer may arrive later as an ordinary user message. Do not poll, repeat the question, or idle waiting."}, nil
	}
	select {
	case <-ctx.Done():
		_ = c.UpdateQuestion(request.ID, "cancel")
		return nil, ctx.Err()
	case <-done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return map[string]any{"status": q.Request.State, "answer": q.answer}, nil
	}
}

func (c *Config) UpdateQuestion(id, operation string) error {
	if err := c.RestoreUIInteractions(); err != nil {
		return err
	}
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.questions[id]
	if q == nil {
		return fmt.Errorf("question request does not belong to this session")
	}
	if q.Request.State == "submitted" {
		return nil
	}
	switch operation {
	case "touch":
		q.Request.Touched = true
		q.Request.Deadline = 0
		if q.timer != nil {
			q.timer.Stop()
		}
	case "expire":
		if q.Request.State != "pending" || q.Request.Touched || q.Request.Deadline == 0 || time.Now().UnixMilli() < q.Request.Deadline {
			return nil
		}
		q.Request.State = "expired"
	case "skip":
		q.Request.State = "skipped"
	case "cancel":
		q.Request.State = "cancelled"
	default:
		return fmt.Errorf("unknown question operation %q", operation)
	}
	if q.Request.State != "pending" && q.timer != nil {
		q.timer.Stop()
	}
	if err := c.emitQuestion(q); err != nil {
		return err
	}
	if q.Request.State != "pending" && q.done != nil {
		close(q.done)
		q.done = nil
	}
	return nil
}

func (c *Config) HandleUIControl(event *ypb.AIInputEvent) error {
	var control struct {
		RequestID string `json:"requestId"`
		Operation string `json:"operation"`
	}
	err := json.Unmarshal([]byte(event.SyncJsonInput), &control)
	if err == nil {
		err = c.UpdateQuestion(control.RequestID, control.Operation)
	}
	payload := map[string]any{"requestId": control.RequestID, "operation": control.Operation, "ok": err == nil}
	if err != nil {
		payload["error"] = err.Error()
	}
	c.EmitSyncJSON("ui_receipt", "ui_receipt", payload, event.SyncID)
	return err
}

// AcceptQuestionAnswer owns deduplication, validation, journaling and wakeup.
// deliver chooses current-task steering or new-task admission at the server.
func (c *Config) AcceptQuestionAnswer(event *ypb.AIInputEvent, deliver func(string, string, *Config) error) error {
	if err := c.RestoreUIInteractions(); err != nil {
		return err
	}
	var receipt interaction.Submission
	if len(event.UserInteraction) > 128*1024 {
		return fmt.Errorf("interaction submission exceeds 128 KiB")
	}
	if err := json.Unmarshal([]byte(event.UserInteraction), &receipt); err != nil {
		return err
	}
	if strings.TrimSpace(receipt.ID) == "" || len(receipt.ID) > 128 || receipt.Source != "questionnaire" {
		return fmt.Errorf("invalid submission id or source")
	}
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, exists := s.receipts[receipt.ID]; exists {
		if previous.Metadata.RequestID != receipt.RequestID {
			return fmt.Errorf("submission id already belongs to another request")
		}
		c.EmitSyncJSON("ui_receipt", "ui_receipt", map[string]any{"id": receipt.ID, "ok": true}, event.SyncID)
		return nil
	}
	q := s.questions[receipt.RequestID]
	if q == nil {
		return fmt.Errorf("question request does not belong to this session")
	}
	if q.Request.State == "submitted" {
		return fmt.Errorf("question already answered")
	}
	text, err := q.Request.AnswerText(receipt.Answers)
	if err != nil {
		return err
	}
	if text != "" {
		origin := q.origin
		if q.ctx == nil || q.ctx.Err() != nil {
			origin = nil
		}
		if err := deliver(text, q.Request.TaskID, origin); err != nil {
			return err
		}
		message := UIUserMessage{ID: receipt.ID, Text: text, Metadata: receipt}
		// Admission succeeded: retries in this runtime must never deliver twice,
		// even if the transport disconnects while publishing the receipt.
		s.receipts[receipt.ID] = message
		if _, err := c.EmitJSON("user_message", "user_message", message); err != nil {
			return err
		}
		q.Request.State = "submitted"
	} else {
		q.Request.State = "skipped"
	}
	q.answer = text
	q.Request.Touched = true
	q.Request.Deadline = 0
	if q.timer != nil {
		q.timer.Stop()
	}
	if err := c.emitQuestion(q); err != nil {
		return err
	}
	if q.done != nil {
		close(q.done)
		q.done = nil
	}
	c.EmitSyncJSON("ui_receipt", "ui_receipt", map[string]any{"id": receipt.ID, "ok": true}, event.SyncID)
	return nil
}
