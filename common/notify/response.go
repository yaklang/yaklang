package notify

import "time"

type Response struct {
	Platform  Platform
	Action    Action
	MessageID string
	Resource  *Resource
	Raw       []byte
}

type Resource struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
	Path     string
	Raw      []byte
}

type EventType string

const (
	EventMessage    EventType = "message"
	EventCardAction EventType = "card_action"
	EventOnboarding EventType = "onboarding"
	EventConnected  EventType = "connected"
	EventError      EventType = "error"
)

type Event struct {
	Type       EventType
	Platform   Platform
	Message    *InboundMessage
	Onboarding *OnboardingStep
	Raw        []byte
	Err        error
}

type InboundMessage struct {
	Platform     PlatformType
	ID           string
	ChatID       string
	SenderID     string
	SenderName   string
	Text         string
	EventTime    time.Time
	ChatType     string
	ReplyContext any
	ReplyTo      string
	ThreadID     string
	RootID       string
	ParentID     string
	IsCardAction bool
	ActionValue  map[string]any
	MentionBot   bool
	Attachments  []IMAttachment
	Raw          []byte
}

type EventHandler func(Event)
