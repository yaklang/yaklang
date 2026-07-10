package notify

import (
	"context"
	"testing"
)

type fakeDriver struct {
	req *Request
}

func (f *fakeDriver) Do(ctx context.Context, req *Request) (*Response, error) {
	f.req = req
	return &Response{Platform: req.Platform, Action: req.Action, MessageID: "m1"}, nil
}

func (f *fakeDriver) Stream(ctx context.Context, req *Request, emit EventHandler) error {
	emit(Event{Type: EventConnected, Platform: req.Platform})
	return nil
}

func TestRegistryClientRoutesByParsedURL(t *testing.T) {
	reg := NewRegistry()
	fd := &fakeDriver{}
	reg.Register(Descriptor{
		Platform:     Platform("fake"),
		Capabilities: Capabilities{SendText: true},
		Actions:      []Action{ActionMessagesSend},
		New: func(DriverConfig) (Driver, error) {
			return fd, nil
		},
	})

	client := NewClient(WithRegistry(reg))
	resp, err := client.Do(context.Background(), &Request{URL: "notify://fake/messages:send"})
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if resp.MessageID != "m1" {
		t.Fatalf("message id = %q", resp.MessageID)
	}
	if fd.req.Platform != "fake" || fd.req.Action != ActionMessagesSend {
		t.Fatalf("routed request = %#v", fd.req)
	}
}

func TestRegistryRejectsUnsupportedAction(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Descriptor{
		Platform: Platform("fake"),
		Actions:  []Action{ActionMessagesSend},
		New: func(DriverConfig) (Driver, error) {
			return &fakeDriver{}, nil
		},
	})

	client := NewClient(WithRegistry(reg))
	_, err := client.Do(context.Background(), &Request{URL: "notify://fake/messages:patch"})
	if err == nil {
		t.Fatal("expected unsupported action error")
	}
}

func TestClientForwardsSendConfigToDriver(t *testing.T) {
	reg := NewRegistry()
	var got DriverConfig
	reg.Register(Descriptor{
		Platform: Platform("fake"),
		Actions:  []Action{ActionMessagesSend},
		New: func(cfg DriverConfig) (Driver, error) {
			got = cfg
			return &fakeDriver{}, nil
		},
	})

	sendCfg := NewSendConfig(WithAppID("app-1"), WithBaseURL("https://example.invalid"))
	client := NewClient(WithRegistry(reg), WithSendConfig(sendCfg))
	if _, err := client.Do(context.Background(), &Request{URL: "notify://fake/messages:send"}); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if got.SendConfig == nil {
		t.Fatal("expected send config to be forwarded")
	}
	if got.SendConfig.AppID != "app-1" || got.SendConfig.BaseURL != "https://example.invalid" {
		t.Fatalf("send config = %#v", got.SendConfig)
	}
}
