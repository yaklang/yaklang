package notify

type MessageType string

const (
	MessageText     MessageType = "text"
	MessageMarkdown MessageType = "markdown"
	MessageCard     MessageType = "card"
	MessageNative   MessageType = "native"
)

type MsgType string

const (
	MsgText     MsgType = "text"
	MsgMarkdown MsgType = "markdown"
	MsgCard     MsgType = "card"
	MsgImage    MsgType = "image"
	MsgFile     MsgType = "file"
)

type Message struct {
	Type       MessageType
	Text       string
	Markdown   string
	Card       *Card
	NativeCard *NativeCard
	Files      []Attachment
}

type Card struct {
	Title    string
	Content  string
	Markdown string
	Config   map[string]any
	Elements []map[string]any
	Buttons  []CardButton
}

type CardButton struct {
	Text  string
	Style string
	Value map[string]any
}

type Button = CardButton

type NativeCard struct {
	Platform Platform
	Schema   string
	Body     []byte
}

type TargetKind string

const (
	TargetUser   TargetKind = "user"
	TargetChat   TargetKind = "chat"
	TargetThread TargetKind = "thread"
)

type Target struct {
	ID       string
	Kind     TargetKind
	ThreadID string
	ReplyTo  string
	Native   map[string]any
}

type Attachment struct {
	Name     string
	MimeType string
	Path     string
	Size     int64
}

type ResourceRef struct {
	ID        string
	MessageID string
	Name      string
	Type      string
}

type IMAttachment struct {
	Type      MsgType
	FileKey   string
	FileName  string
	MessageID string
	MimeType  string
	Size      int64
	LocalPath string
}
