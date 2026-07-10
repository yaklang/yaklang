package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/notify"
)

func TestDescriptorDeclaresFeishuCardV2(t *testing.T) {
	desc := Descriptor()
	if !desc.Capabilities.SendCard || !desc.Capabilities.UpdateCard || !desc.Capabilities.CardActions {
		t.Fatalf("feishu card capabilities incomplete: %#v", desc.Capabilities)
	}
	if !desc.Capabilities.StreamCard {
		t.Fatalf("feishu should support stream card updates: %#v", desc.Capabilities)
	}
	if !desc.Capabilities.SupportsNativeCard("feishu.card.v2") {
		t.Fatalf("missing feishu.card.v2 native schema: %#v", desc.Capabilities.NativeCardSchemas)
	}
}

func TestDescriptorDeclaresFeishuActions(t *testing.T) {
	desc := Descriptor()
	required := map[string]bool{
		"messages:send":      false,
		"messages:reply":     false,
		"messages:patch":     false,
		"reactions:add":      false,
		"resources:download": false,
		"events:receive":     false,
		"onboarding:start":   false,
	}
	for _, action := range desc.Actions {
		if _, ok := required[string(action)]; ok {
			required[string(action)] = true
		}
	}
	for action, found := range required {
		if !found {
			t.Fatalf("feishu descriptor missing action %s", action)
		}
	}
}

func TestDriverDoSendNativeCard(t *testing.T) {
	var gotBody map[string]any
	var gotReceiveIDType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/im/v1/messages":
			gotReceiveIDType = r.URL.Query().Get("receive_id_type")
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &gotBody); err != nil {
				t.Fatalf("decode send body: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_native"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	cardBody := []byte(`{"schema":"2.0","body":{"elements":[{"tag":"markdown","content":"# hi"}]}}`)
	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("cli_a"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(srv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformFeishu,
		Action:   notify.ActionMessagesSend,
		Target: notify.Target{
			ID:   "oc_x",
			Kind: notify.TargetChat,
			Native: map[string]any{
				"receive_id_type": "chat_id",
			},
		},
		Message: &notify.Message{
			Type: notify.MessageNative,
			NativeCard: &notify.NativeCard{
				Platform: notify.PlatformFeishu,
				Schema:   "feishu.card.v2",
				Body:     cardBody,
			},
		},
	})
	if err != nil {
		t.Fatalf("Do send native card: %v", err)
	}
	if resp.MessageID != "om_native" {
		t.Fatalf("message id = %q", resp.MessageID)
	}
	if gotReceiveIDType != "chat_id" {
		t.Fatalf("receive_id_type = %q", gotReceiveIDType)
	}
	if gotBody["receive_id"] != "oc_x" {
		t.Fatalf("receive_id = %v", gotBody["receive_id"])
	}
	if gotBody["msg_type"] != "interactive" {
		t.Fatalf("msg_type = %v", gotBody["msg_type"])
	}
	content, ok := gotBody["content"].(string)
	if !ok {
		t.Fatalf("content type = %T", gotBody["content"])
	}
	var gotCard, wantCard map[string]any
	if err := json.Unmarshal([]byte(content), &gotCard); err != nil {
		t.Fatalf("decode card content: %v", err)
	}
	if err := json.Unmarshal(cardBody, &wantCard); err != nil {
		t.Fatalf("decode expected card: %v", err)
	}
	if !reflect.DeepEqual(gotCard, wantCard) {
		t.Fatalf("card content = %#v, want %#v", gotCard, wantCard)
	}
}

func TestDriverDoPing(t *testing.T) {
	var gotBody map[string]any
	var gotPath, gotAuth string
	srv := mockFeishuGateway(t, &gotBody, &gotPath, &gotAuth, "unused")

	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("cli_a"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(srv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformFeishu,
		Action:   notify.ActionPing,
	})
	if err != nil {
		t.Fatalf("Do ping: %v", err)
	}
	if resp.Platform != notify.PlatformFeishu || resp.Action != notify.ActionPing {
		t.Fatalf("response = %#v", resp)
	}
	if gotPath != "/open-apis/auth/v3/tenant_access_token/internal" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestDriverStreamEventsReceive(t *testing.T) {
	driver := &Driver{
		start: func(ctx context.Context, handler func(*notify.InboundMessage)) error {
			handler(&notify.InboundMessage{
				Platform: notify.PlatformFeishu,
				ID:       "om_msg",
				Text:     "hello",
			})
			handler(&notify.InboundMessage{
				Platform:     notify.PlatformFeishu,
				ID:           "om_card",
				IsCardAction: true,
			})
			return nil
		},
	}
	var events []notify.Event
	err := driver.Stream(context.Background(), &notify.Request{
		Platform: notify.PlatformFeishu,
		Action:   notify.ActionEventsReceive,
	}, func(ev notify.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Type != notify.EventMessage || events[0].Message.ID != "om_msg" {
		t.Fatalf("message event = %#v", events[0])
	}
	if events[1].Type != notify.EventCardAction || events[1].Message.ID != "om_card" {
		t.Fatalf("card event = %#v", events[1])
	}
}

func TestDriverDoDownloadResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"mock-tt","expire":7200}`))
		case r.URL.Path == "/open-apis/im/v1/messages/om_test/resources/img_v3_001":
			if got := r.URL.Query().Get("type"); got != "image" {
				t.Fatalf("type query = %q", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	driver, err := Descriptor().New(notify.DriverConfig{
		SendConfig: notify.NewSendConfig(
			notify.WithAppID("cli_a"),
			notify.WithAppSecret("secret"),
			notify.WithBaseURL(srv.URL),
			notify.WithTimeout(5*time.Second),
		),
	})
	if err != nil {
		t.Fatalf("new driver: %v", err)
	}
	resp, err := driver.Do(context.Background(), &notify.Request{
		Platform: notify.PlatformFeishu,
		Action:   notify.ActionResourcesDownload,
		Resource: &notify.ResourceRef{
			ID:        "img_v3_001",
			MessageID: "om_test",
			Type:      "image",
		},
	})
	if err != nil {
		t.Fatalf("download resource: %v", err)
	}
	if resp.Resource == nil {
		t.Fatal("resource is nil")
	}
	defer os.Remove(resp.Resource.Path)
	if resp.Resource.MimeType != "image/png" || resp.Resource.Size != 6 {
		t.Fatalf("resource = %#v", resp.Resource)
	}
	if _, err := os.Stat(resp.Resource.Path); err != nil {
		t.Fatalf("resource file missing: %v", err)
	}
}

func TestDriverStreamOnboardingStart(t *testing.T) {
	driver := &Driver{
		onboard: func(timeoutSeconds int, opts map[string]string, handler notify.OnboardingHandler) error {
			if timeoutSeconds != 42 {
				t.Fatalf("timeout = %d", timeoutSeconds)
			}
			if opts["is_lark"] != "true" {
				t.Fatalf("opts = %#v", opts)
			}
			return handler(&notify.OnboardingStep{State: "qr", QrURL: "https://qr.example"})
		},
	}
	var events []notify.Event
	err := driver.Stream(context.Background(), &notify.Request{
		Platform: notify.PlatformFeishu,
		Action:   notify.ActionOnboardingStart,
		Options: notify.Options{
			"timeout_seconds": 42,
			"is_lark":         "true",
		},
	}, func(ev notify.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Stream onboarding: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Type != notify.EventOnboarding || events[0].Onboarding == nil || events[0].Onboarding.QrURL != "https://qr.example" {
		t.Fatalf("event = %#v", events[0])
	}
}
