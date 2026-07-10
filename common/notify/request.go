package notify

import (
	"fmt"
	"net/url"
	"strings"
)

// Platform identifies a notify driver.
type Platform string

const (
	PlatformFeishu   Platform = "feishu"
	PlatformDingTalk Platform = "dingtalk"
	PlatformWeCom    Platform = "wecom"
	PlatformTelegram Platform = "telegram"
)

type PlatformType = Platform

func (p Platform) String() string {
	return string(p)
}

// Action is the resource:verb part of a notify URL.
type Action string

const (
	ActionMessagesSend      Action = "messages:send"
	ActionMessagesReply     Action = "messages:reply"
	ActionMessagesPatch     Action = "messages:patch"
	ActionReactionsAdd      Action = "reactions:add"
	ActionResourcesDownload Action = "resources:download"
	ActionPing              Action = "ping"
	ActionEventsReceive     Action = "events:receive"
	ActionOnboardingStart   Action = "onboarding:start"
)

// Request is the typed execution model for a notify:// request.
type Request struct {
	URL        string
	Platform   Platform
	Action     Action
	Credential CredentialRef
	Target     Target
	Message    *Message
	Resource   *ResourceRef
	Options    Options
	Native     NativeOptions
}

type CredentialRef struct {
	Platform Platform
	BotID    string
}

type Options map[string]any

type NativeOptions map[string]any

// ParseURL parses notify://<platform>/<resource>:<verb> into a Request.
func ParseURL(raw string) (*Request, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("notify: parse url: %w", err)
	}
	if u.Scheme != "notify" {
		return nil, fmt.Errorf("notify: invalid scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("notify: missing platform")
	}
	action := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
	if action == "" {
		return nil, fmt.Errorf("notify: missing action")
	}
	if !strings.Contains(action, ":") {
		return nil, fmt.Errorf("notify: action must be resource:verb")
	}
	return &Request{
		URL:      raw,
		Platform: Platform(u.Host),
		Action:   Action(action),
	}, nil
}
