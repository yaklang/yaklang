package notify

type SendResult struct {
	MessageID string
	Raw       []byte
	Platform  PlatformType
}
