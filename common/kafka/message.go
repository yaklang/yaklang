package kafka

type MessageType int

const (
	NodeRegistry MessageType = iota + 1
	ScanResult
	Heart
)
